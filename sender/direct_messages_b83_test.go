package sender_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-83 (E-0149, the client layer): GetChat (decoded by callJSON) and SendMessage (decoded by parseMessage inside
// withRetry) return the Bot API 9.2 direct-messages fields when Telegram sends them, and zero values when it does
// not. Each row also compares the whole returned value with its reply fixture (O-T2-01), so a value the client loses
// or changes after decode fails. The rows serve each holder and state the type rows prove (O-T3-01), with different
// values in different holders. Ids are fake.
const (
	b83SenderDMChat  = `{"id":-1009876543210,"type":"supergroup","title":"Test DM","is_direct_messages":true}`
	b83SenderChannel = `{"id":-1001234567890,"type":"channel","title":"Test Channel","username":"testchannel"}`
	b83SenderTopic   = `{"topic_id":4503599627370495,"user":{"id":3000000001,"is_bot":false,"first_name":"Ann"}}`
	b83SenderMsg     = `{"message_id":42,"direct_messages_topic":` + b83SenderTopic +
		`,"date":1700000000,"chat":` + b83SenderDMChat + `,"text":"hi"}`
)

// S1: getChat.
func TestB83_Client_GetChat(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		check      func(t *testing.T, chat *tg.ChatFullInfo)
	}{
		{"with", `{"id":-1009876543210,"type":"supergroup","title":"Test DM","is_direct_messages":true,` +
			`"accent_color_id":1,"max_reaction_count":11,"parent_chat":` + b83SenderChannel + `}`,
			func(t *testing.T, chat *tg.ChatFullInfo) {
				assert.Equal(t, true, chat.IsDirectMessages)
				require.NotNil(t, chat.ParentChat)
				assert.Equal(t, int64(-1001234567890), chat.ParentChat.ID)
				assert.Equal(t, "channel", chat.ParentChat.Type)
				assert.Equal(t, "Test Channel", chat.ParentChat.Title)
				assert.Equal(t, "testchannel", chat.ParentChat.Username)
				assert.Equal(t, false, chat.ParentChat.IsDirectMessages)
			}},
		// the parent holds the flag, the outer chat does not (T3 parent_is_dm_flag at the client layer; O-T3-01)
		{"parent_is_dm", `{"id":-1009876543211,"type":"supergroup","accent_color_id":1,"max_reaction_count":11,` +
			`"parent_chat":` + b83SenderDMChat + `}`,
			func(t *testing.T, chat *tg.ChatFullInfo) {
				assert.Equal(t, false, chat.IsDirectMessages)
				require.NotNil(t, chat.ParentChat)
				assert.Equal(t, true, chat.ParentChat.IsDirectMessages)
				assert.Equal(t, int64(-1009876543210), chat.ParentChat.ID)
			}},
		{"without", `{"id":-1009876543210,"type":"supergroup","title":"Test Group","accent_color_id":1,"max_reaction_count":11}`,
			func(t *testing.T, chat *tg.ChatFullInfo) {
				assert.Equal(t, false, chat.IsDirectMessages)
				assert.Nil(t, chat.ParentChat)
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/getChat", func(w http.ResponseWriter, r *http.Request) {
				testutil.ReplyOK(w, json.RawMessage(tc.body))
			})
			chat, err := testutil.NewTestClient(t, server.BaseURL()).GetChat(context.Background(), int64(-1009876543210))
			require.NoError(t, err)
			require.NotNil(t, chat)
			assert.JSONEq(t, tc.body, b83SenderJSON(t, chat))
			tc.check(t, chat)
		})
	}
}

// S2: sendMessage.
func TestB83_Client_SendMessage(t *testing.T) {
	for _, tc := range []struct {
		name, reply string
		check       func(t *testing.T, m *tg.Message)
	}{
		{"topic_and_chat", b83SenderMsg, func(t *testing.T, m *tg.Message) {
			require.NotNil(t, m.DirectMessagesTopic)
			assert.Equal(t, int64(4503599627370495), m.DirectMessagesTopic.TopicID)
			require.NotNil(t, m.DirectMessagesTopic.User)
			assert.Equal(t, int64(3000000001), m.DirectMessagesTopic.User.ID)
			assert.Equal(t, "Ann", m.DirectMessagesTopic.User.FirstName)
			assert.Equal(t, false, m.DirectMessagesTopic.User.IsBot)
			require.NotNil(t, m.Chat)
			assert.Equal(t, true, m.Chat.IsDirectMessages)
			assert.Equal(t, int64(-1009876543210), m.Chat.ID)
			assert.Equal(t, "supergroup", m.Chat.Type)
			assert.Equal(t, "Test DM", m.Chat.Title)
		}},
		// the optional user is absent and must stay nil (O-T3-01)
		{"no_user", `{"message_id":44,"direct_messages_topic":{"topic_id":5},"date":1700000000,"chat":` +
			b83SenderDMChat + `,"text":"hi"}`, func(t *testing.T, m *tg.Message) {
			require.NotNil(t, m.DirectMessagesTopic)
			assert.Equal(t, int64(5), m.DirectMessagesTopic.TopicID)
			assert.Nil(t, m.DirectMessagesTopic.User)
		}},
		// the topic is only in the replied message (O-T3-01)
		{"nested_reply", `{"message_id":45,"date":1700000000,"chat":` + b83SenderDMChat +
			`,"reply_to_message":` + b83SenderMsg + `,"text":"re"}`, func(t *testing.T, m *tg.Message) {
			assert.Nil(t, m.DirectMessagesTopic)
			require.NotNil(t, m.ReplyToMessage)
			require.NotNil(t, m.ReplyToMessage.DirectMessagesTopic)
			assert.Equal(t, int64(4503599627370495), m.ReplyToMessage.DirectMessagesTopic.TopicID)
		}},
		// the flag is only on the sender chat, not on the outer chat (O-T3-01)
		{"sender_chat", `{"message_id":43,"sender_chat":` + b83SenderDMChat + `,"date":1700000000,"chat":` +
			b83SenderChannel + `,"text":"hi"}`, func(t *testing.T, m *tg.Message) {
			require.NotNil(t, m.SenderChat)
			assert.Equal(t, true, m.SenderChat.IsDirectMessages)
			require.NotNil(t, m.Chat)
			assert.Equal(t, false, m.Chat.IsDirectMessages)
			assert.Nil(t, m.DirectMessagesTopic)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
				testutil.ReplyOK(w, json.RawMessage(tc.reply))
			})
			m, err := testutil.NewTestClient(t, server.BaseURL()).SendMessage(context.Background(),
				sender.SendMessageRequest{ChatID: int64(-1009876543210), Text: "hi", DirectMessagesTopicID: 4503599627370495})
			require.NoError(t, err)
			require.NotNil(t, m)
			assert.JSONEq(t, tc.reply, b83SenderJSON(t, m))
			tc.check(t, m)
		})
	}
}

// b83SenderJSON encodes a returned value for the whole-value compare.
func b83SenderJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}
