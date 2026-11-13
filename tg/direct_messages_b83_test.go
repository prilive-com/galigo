package tg_test

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// B-83: the Bot API 9.2 direct-messages read side: DirectMessagesTopic, Message.DirectMessagesTopic,
// Chat.IsDirectMessages, ChatFullInfo.IsDirectMessages and ChatFullInfo.ParentChat. No test builds a new type with a
// keyed composite literal (only zero values): values come from decoded JSON and are read by selectors, and asserts use
// testify with typed expected values (int64(…), true, false, assert.Nil), so a Go-type mutant compiles and fails by
// assertion. Ids are fake. Fixtures hold only keys galigo models, so a strict decode tests the new keys. The helpers
// api96Strict (tg/api96_test.go), api102aMarshal and api102aKeys (tg/api102a_test.go) are reused.
const (
	b83User    = `{"id":3000000001,"is_bot":false,"first_name":"Ann"}`
	b83Topic   = `{"topic_id":4503599627370495,"user":` + b83User + `}` // topic id 2^52-1, the spec's maximum
	b83DMChat  = `{"id":-1009876543210,"type":"supergroup","title":"Test DM","is_direct_messages":true}`
	b83Channel = `{"id":-1001234567890,"type":"channel","title":"Test Channel","username":"testchannel"}`
	b83Msg     = `{"message_id":42,"direct_messages_topic":` + b83Topic + `,"date":1700000000,"chat":` + b83DMChat + `,"text":"hi"}`
	b83MaxID   = int64(4503599627370495)
)

// b83Decode decodes in strictly (DisallowUnknownFields) into v and checks that v re-encodes to the same JSON.
func b83Decode(t *testing.T, in string, v any) {
	t.Helper()
	require.NoError(t, api96Strict(t, []byte(in), v))
	assert.JSONEq(t, in, api102aMarshal(t, v))
}

// b83AssertTopic checks a decoded topic against b83Topic. It takes any, so a holder mutant with a value type
// (DirectMessagesTopic DirectMessagesTopic) still builds and fails here at run time (TEST-PLAN §2, §8).
func b83AssertTopic(t *testing.T, v any) {
	t.Helper()
	got, ok := v.(*tg.DirectMessagesTopic)
	require.True(t, ok, "want *tg.DirectMessagesTopic, got %T", v)
	require.NotNil(t, got)
	assert.Equal(t, b83MaxID, got.TopicID)
	require.NotNil(t, got.User)
	assert.Equal(t, int64(3000000001), got.User.ID)
	assert.Equal(t, "Ann", got.User.FirstName)
}

// T1: DirectMessagesTopic.
func TestB83_Topic(t *testing.T) {
	t.Run("max52", func(t *testing.T) {
		var v tg.DirectMessagesTopic
		b83Decode(t, b83Topic, &v)
		b83AssertTopic(t, &v)
	})
	t.Run("above_2_31", func(t *testing.T) {
		var v tg.DirectMessagesTopic
		b83Decode(t, `{"topic_id":3000000000,"user":`+b83User+`}`, &v)
		assert.Equal(t, int64(3000000000), v.TopicID)
		require.NotNil(t, v.User)
	})
	t.Run("small", func(t *testing.T) {
		var v tg.DirectMessagesTopic
		b83Decode(t, `{"topic_id":1,"user":`+b83User+`}`, &v)
		assert.Equal(t, int64(1), v.TopicID)
	})
	t.Run("no_user", func(t *testing.T) {
		var v tg.DirectMessagesTopic
		b83Decode(t, `{"topic_id":5}`, &v)
		assert.Equal(t, int64(5), v.TopicID)
		assert.Nil(t, v.User)
	})
}

// T2: the holders of the topic and the flag in a Message, and the receiver paths through tg.Update.
func TestB83_Message(t *testing.T) {
	t.Run("topic_and_chat", func(t *testing.T) {
		var m tg.Message
		b83Decode(t, b83Msg, &m)
		b83AssertTopic(t, m.DirectMessagesTopic)
		require.NotNil(t, m.Chat)
		assert.Equal(t, true, m.Chat.IsDirectMessages)
		assert.Equal(t, int64(-1009876543210), m.Chat.ID)
	})
	t.Run("sender_chat", func(t *testing.T) {
		var m tg.Message
		b83Decode(t, `{"message_id":43,"sender_chat":`+b83DMChat+`,"date":1700000000,"chat":`+b83Channel+`,"text":"hi"}`, &m)
		require.NotNil(t, m.SenderChat)
		assert.Equal(t, true, m.SenderChat.IsDirectMessages)
		require.NotNil(t, m.Chat)
		assert.Equal(t, false, m.Chat.IsDirectMessages)
		assert.Nil(t, m.DirectMessagesTopic)
	})
	t.Run("nested_reply", func(t *testing.T) {
		var m tg.Message
		b83Decode(t, `{"message_id":44,"date":1700000000,"chat":`+b83DMChat+`,"reply_to_message":`+b83Msg+`,"text":"re"}`, &m)
		require.NotNil(t, m.ReplyToMessage)
		b83AssertTopic(t, m.ReplyToMessage.DirectMessagesTopic)
		assert.Nil(t, m.DirectMessagesTopic)
	})
	t.Run("update_message", func(t *testing.T) {
		for _, tc := range []struct {
			name, in string
			get      func(u *tg.Update) *tg.Message
		}{
			{"message", `{"update_id":1,"message":` + b83Msg + `}`, func(u *tg.Update) *tg.Message { return u.Message }},
			{"edited_message", `{"update_id":2,"edited_message":` + b83Msg + `}`, func(u *tg.Update) *tg.Message { return u.EditedMessage }},
		} {
			t.Run(tc.name, func(t *testing.T) {
				var u tg.Update
				b83Decode(t, tc.in, &u)
				m := tc.get(&u)
				require.NotNil(t, m)
				b83AssertTopic(t, m.DirectMessagesTopic)
				require.NotNil(t, m.Chat)
				assert.Equal(t, true, m.Chat.IsDirectMessages)
			})
		}
	})
	t.Run("update_member", func(t *testing.T) {
		member := `{"chat":` + b83DMChat + `,"from":` + b83User + `,"date":1700000000,` +
			`"old_chat_member":{"status":"left","user":` + b83User + `},"new_chat_member":{"status":"member","user":` + b83User + `}}`
		for _, tc := range []struct {
			name, in string
			get      func(u *tg.Update) *tg.ChatMemberUpdated
		}{
			{"my_chat_member", `{"update_id":3,"my_chat_member":` + member + `}`, func(u *tg.Update) *tg.ChatMemberUpdated { return u.MyChatMember }},
			{"chat_member", `{"update_id":4,"chat_member":` + member + `}`, func(u *tg.Update) *tg.ChatMemberUpdated { return u.ChatMember }},
		} {
			t.Run(tc.name, func(t *testing.T) {
				var u tg.Update
				// Strict decoding stops at ChatMemberUpdated: its UnmarshalJSON (tg/update.go) decodes its subtree
				// with a plain json.Unmarshal, so an unknown key under "chat" here is not refused. A misspelt tag still
				// fails this row by the value checks below; the strict proof of the key is in T1-T3.
				require.NoError(t, api96Strict(t, []byte(tc.in), &u))
				cm := tc.get(&u)
				require.NotNil(t, cm)
				require.NotNil(t, cm.Chat)
				assert.Equal(t, true, cm.Chat.IsDirectMessages)
				assert.Equal(t, int64(-1009876543210), cm.Chat.ID)
				// the member fields are json:"-", so the whole update does not round-trip; the chat value does
				assert.JSONEq(t, b83DMChat, api102aMarshal(t, cm.Chat))
			})
		}
	})
}

// T3: ChatFullInfo (a getChat result).
func TestB83_ChatFullInfo(t *testing.T) {
	t.Run("dm_chat", func(t *testing.T) {
		var c tg.ChatFullInfo
		b83Decode(t, `{"id":-1009876543210,"type":"supergroup","title":"Test DM","is_direct_messages":true,`+
			`"accent_color_id":1,"max_reaction_count":11,"parent_chat":`+b83Channel+`}`, &c)
		assert.Equal(t, true, c.IsDirectMessages)
		require.NotNil(t, c.ParentChat)
		assert.Equal(t, int64(-1001234567890), c.ParentChat.ID)
		assert.Equal(t, "channel", c.ParentChat.Type)
		assert.Equal(t, "testchannel", c.ParentChat.Username)
		assert.Equal(t, false, c.ParentChat.IsDirectMessages)
	})
	t.Run("parent_is_dm_flag", func(t *testing.T) {
		var c tg.ChatFullInfo
		b83Decode(t, `{"id":-1009876543211,"type":"supergroup","accent_color_id":1,"max_reaction_count":11,"parent_chat":`+b83DMChat+`}`, &c)
		assert.Equal(t, false, c.IsDirectMessages)
		require.NotNil(t, c.ParentChat)
		assert.Equal(t, true, c.ParentChat.IsDirectMessages)
	})
}

// T4: absent keys decode to zero; zero values encode no new key.
func TestB83_Absent(t *testing.T) {
	t.Run("message", func(t *testing.T) {
		var m tg.Message
		b83Decode(t, `{"message_id":1,"date":1700000000,"chat":`+b83Channel+`,"text":"t"}`, &m)
		assert.Nil(t, m.DirectMessagesTopic)
		require.NotNil(t, m.Chat)
		assert.Equal(t, false, m.Chat.IsDirectMessages)
		assert.NotContains(t, api102aKeys(t, tg.Message{}), "direct_messages_topic")
		assert.NotContains(t, api102aKeys(t, tg.Chat{}), "is_direct_messages")
	})
	t.Run("chat_full_info", func(t *testing.T) {
		var c tg.ChatFullInfo
		b83Decode(t, `{"id":-1001,"type":"supergroup","accent_color_id":1,"max_reaction_count":11}`, &c)
		assert.Equal(t, false, c.IsDirectMessages)
		assert.Nil(t, c.ParentChat)
		keys := api102aKeys(t, tg.ChatFullInfo{})
		assert.NotContains(t, keys, "is_direct_messages")
		assert.NotContains(t, keys, "parent_chat")
	})
	t.Run("explicit_false", func(t *testing.T) {
		var ch tg.Chat
		require.NoError(t, api96Strict(t, []byte(`{"id":-1001,"type":"supergroup","is_direct_messages":false}`), &ch))
		assert.Equal(t, false, ch.IsDirectMessages)
		var c tg.ChatFullInfo
		require.NoError(t, api96Strict(t, []byte(`{"id":-1001,"type":"supergroup","is_direct_messages":false,"accent_color_id":1,"max_reaction_count":11}`), &c))
		assert.Equal(t, false, c.IsDirectMessages)
	})
	t.Run("topic_zero", func(t *testing.T) {
		assert.JSONEq(t, `{"topic_id":0}`, api102aMarshal(t, tg.DirectMessagesTopic{}))
	})
}

// T5: shapes by reflection (E-0132, E-0126): name, Go type, tag, place, not embedded.
func TestB83_Shapes(t *testing.T) {
	type field struct{ name, typ, tag string }
	t.Run("DirectMessagesTopic", func(t *testing.T) {
		rt := reflect.TypeOf(tg.DirectMessagesTopic{})
		var got []field
		for i := range rt.NumField() {
			f := rt.Field(i)
			assert.False(t, f.Anonymous, f.Name)
			got = append(got, field{f.Name, f.Type.String(), f.Tag.Get("json")})
		}
		assert.Equal(t, []field{{"TopicID", "int64", "topic_id"}, {"User", "*tg.User", "user,omitempty"}}, got)
	})
	for _, tc := range []struct {
		v                  any
		name, typ, tag     string
		before, afterField string
	}{
		{tg.Message{}, "DirectMessagesTopic", "*tg.DirectMessagesTopic", "direct_messages_topic,omitempty", "MessageThreadID", "From"},
		{tg.Chat{}, "IsDirectMessages", "bool", "is_direct_messages,omitempty", "IsForum", "Photo"},
		{tg.ChatFullInfo{}, "IsDirectMessages", "bool", "is_direct_messages,omitempty", "IsForum", "AccentColorID"},
		{tg.ChatFullInfo{}, "ParentChat", "*tg.Chat", "parent_chat,omitempty", "PersonalChat", "AvailableReactions"},
	} {
		rt := reflect.TypeOf(tc.v)
		t.Run(rt.Name()+"."+tc.name, func(t *testing.T) {
			f, ok := rt.FieldByName(tc.name)
			require.True(t, ok)
			require.Len(t, f.Index, 1)
			assert.False(t, f.Anonymous)
			assert.Equal(t, tc.typ, f.Type.String())
			assert.Equal(t, tc.tag, f.Tag.Get("json"))
			i := f.Index[0]
			require.Greater(t, i, 0)
			require.Less(t, i, rt.NumField()-1)
			assert.Equal(t, tc.before, rt.Field(i-1).Name)
			assert.Equal(t, tc.afterField, rt.Field(i+1).Name)
		})
	}
}

// T6: the keys live only on their planned holders (a walk of every struct reachable from the roots).
func TestB83_KeyOwners(t *testing.T) {
	watched := map[string]bool{"is_direct_messages": true, "parent_chat": true, "direct_messages_topic": true, "topic_id": true}
	seen := map[reflect.Type]bool{}
	var owners []string
	queue := []reflect.Type{reflect.TypeOf(tg.Update{}), reflect.TypeOf(tg.ChatFullInfo{})}
	for len(queue) > 0 {
		rt := queue[0]
		queue = queue[1:]
		for rt.Kind() == reflect.Pointer || rt.Kind() == reflect.Slice || rt.Kind() == reflect.Array || rt.Kind() == reflect.Map {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct || seen[rt] {
			continue
		}
		seen[rt] = true
		for i := range rt.NumField() {
			f := rt.Field(i)
			if key, _, _ := strings.Cut(f.Tag.Get("json"), ","); watched[key] {
				owners = append(owners, rt.Name()+"."+f.Name)
			}
			queue = append(queue, f.Type)
		}
	}
	sort.Strings(owners)
	assert.Equal(t, []string{
		"Chat.IsDirectMessages", "ChatFullInfo.IsDirectMessages", "ChatFullInfo.ParentChat",
		"DirectMessagesTopic.TopicID", "Message.DirectMessagesTopic",
	}, owners)
	for _, v := range []any{tg.Update{}, tg.Message{}, tg.Chat{}, tg.ChatFullInfo{}, tg.ChatMemberUpdated{}, tg.DirectMessagesTopic{}} {
		assert.True(t, seen[reflect.TypeOf(v)], "the walk did not reach %T", v)
	}
}
