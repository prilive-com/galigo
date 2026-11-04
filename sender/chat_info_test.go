package sender_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// ==================== GetChat ====================

func TestGetChat(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getChat", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"id":    int64(-1001234567890),
			"type":  "supergroup",
			"title": "Test Group",
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	chat, err := client.GetChat(context.Background(), int64(-1001234567890))
	require.NoError(t, err)
	assert.Equal(t, int64(-1001234567890), chat.ID)
	assert.Equal(t, "supergroup", chat.Type)
	assert.Equal(t, "Test Group", chat.Title)
}

func TestGetChat_WithUsername(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getChat", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"id":    int64(-100123),
			"type":  "channel",
			"title": "Channel",
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	chat, err := client.GetChat(context.Background(), "@testchannel")
	require.NoError(t, err)
	assert.Equal(t, "channel", chat.Type)
}

func TestGetChat_Validation_NilChatID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.GetChat(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat_id")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestGetChat_Validation_ZeroChatID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.GetChat(context.Background(), int64(0))
	require.Error(t, err)
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

func TestGetChat_Error_NotFound(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getChat", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBadRequest(w, "chat not found")
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	_, err := client.GetChat(context.Background(), int64(999999))

	require.Error(t, err)
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 400, apiErr.Code)
}

// ==================== GetChatAdministrators ====================

func TestGetChatAdministrators(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getChatAdministrators", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, []map[string]any{
			{"status": "creator", "user": map[string]any{"id": 123, "first_name": "Owner", "is_bot": false}},
			{"status": "administrator", "user": map[string]any{"id": 456, "first_name": "Admin", "is_bot": false}, "can_delete_messages": true},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	admins, err := client.GetChatAdministrators(context.Background(), int64(-100123))
	require.NoError(t, err)
	require.Len(t, admins, 2)
	assert.True(t, tg.IsOwner(admins[0]))
	assert.True(t, tg.IsAdmin(admins[1]))
	assert.False(t, tg.IsOwner(admins[1]))
}

func TestGetChatAdministrators_Validation_InvalidChatID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.GetChatAdministrators(context.Background(), int64(0))
	require.Error(t, err)
	assert.Equal(t, 0, server.CaptureCount())
}

// ==================== GetChatMemberCount ====================

func TestGetChatMemberCount(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getChatMemberCount", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, 42)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	count, err := client.GetChatMemberCount(context.Background(), int64(-100123))
	require.NoError(t, err)
	assert.Equal(t, 42, count)
}

// ==================== GetChatMember ====================

func TestGetChatMember_AllStatuses(t *testing.T) {
	statuses := []string{"creator", "administrator", "member", "restricted", "left", "kicked"}

	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/getChatMember", func(w http.ResponseWriter, r *http.Request) {
				testutil.ReplyOK(w, map[string]any{
					"status": status,
					"user":   map[string]any{"id": 123, "first_name": "Test", "is_bot": false},
				})
			})

			client := testutil.NewTestClient(t, server.BaseURL())
			member, err := client.GetChatMember(context.Background(), int64(-100123), 123)

			require.NoError(t, err)
			assert.Equal(t, status, member.Status())
		})
	}
}

func TestGetChatMember_Validation_InvalidUserID(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.GetChatMember(context.Background(), int64(-100123), 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user_id must be positive")
	assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
}

// ==================== Bucket: Error ====================

func TestChatInfo_Error_Forbidden(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getChatAdministrators", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyForbidden(w, "bot is not a member of the chat")
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	_, err := client.GetChatAdministrators(context.Background(), int64(-100123))

	require.Error(t, err)
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 403, apiErr.Code)
}

// ==================== Bot API 10.0: getChatAdministrators return_bots (B-25 slice 2a, TEST-PLAN §3) ====================

const (
	api100Chat       = int64(-1001234567890)
	api100SecondChat = int64(-1005555555555)
	api100ActorChat  = int64(-1009876543210)

	// an administrator bot (can_delete_messages) and a creator user
	api100Admins = `[{"status":"administrator","user":{"id":7001,"is_bot":true,"first_name":"Test Bot"},` +
		`"can_be_edited":false,"is_anonymous":false,"can_manage_chat":true,"can_delete_messages":true,` +
		`"can_manage_video_chats":false,"can_restrict_members":false,"can_promote_members":false,` +
		`"can_change_info":false,"can_invite_users":false},` +
		`{"status":"creator","user":{"id":42,"is_bot":false,"first_name":"Owner"},"is_anonymous":false}]`
)

func api100WantAdmins() []tg.ChatMember {
	admin := tg.ChatMemberAdministrator{CanManageChat: true, CanDeleteMessages: true}
	admin.User = &tg.User{ID: 7001, IsBot: true, FirstName: "Test Bot"}
	owner := tg.ChatMemberOwner{}
	owner.User = &tg.User{ID: 42, FirstName: "Owner"}
	return []tg.ChatMember{admin, owner}
}

// api100LimiterRow checks the per-chat limiter key of call (TEST-PLAN §3 header): the call on chat A makes one
// limiter; GetChat on A (keyed by extractChatID) adds none; the call on a second chat adds one.
func api100LimiterRow(t *testing.T, method, reply string, call func(*sender.Client, tg.ChatID) error) {
	t.Helper()
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/"+method, func(w http.ResponseWriter, _ *http.Request) {
		api96ReplyRaw(w, `{"ok":true,"result":`+reply+`}`)
	})
	server.On("/bot"+testutil.TestToken+"/getChat", func(w http.ResponseWriter, _ *http.Request) {
		api96ReplyRaw(w, `{"ok":true,"result":{"id":-1001234567890,"type":"supergroup"}}`)
	})
	client := testutil.NewTestClient(t, server.BaseURL(),
		sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))

	require.NoError(t, call(client, api100Chat))
	assert.Equal(t, 1, client.ChatLimiterCount(), "after the call on chat A")
	_, err := client.GetChat(context.Background(), api100Chat)
	require.NoError(t, err)
	assert.Equal(t, 1, client.ChatLimiterCount(), "after getChat on chat A")
	require.NoError(t, call(client, api100SecondChat))
	assert.Equal(t, 2, client.ChatLimiterCount(), "after the call on the second chat")
}

func TestAPI100_GetChatAdministratorsWithBots(t *testing.T) {
	// API100 T-s-3
	rows := []struct {
		name string
		body string
		call func(*sender.Client, tg.ChatID) ([]tg.ChatMember, error)
	}{
		{"WithBots", `{"chat_id":-1001234567890,"return_bots":true}`, func(c *sender.Client, id tg.ChatID) ([]tg.ChatMember, error) {
			return c.GetChatAdministratorsWithBots(context.Background(), id)
		}},
		{"base", `{"chat_id":-1001234567890}`, func(c *sender.Client, id tg.ChatID) ([]tg.ChatMember, error) {
			return c.GetChatAdministrators(context.Background(), id)
		}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/getChatAdministrators", func(w http.ResponseWriter, _ *http.Request) {
				api96ReplyRaw(w, `{"ok":true,"result":`+api100Admins+`}`)
			})
			client := testutil.NewTestClient(t, server.BaseURL())
			got, err := r.call(client, api100Chat)
			require.NoError(t, err)
			assert.Equal(t, api100WantAdmins(), got)
			require.Equal(t, 1, server.CaptureCount())
			server.LastCapture().AssertPath(t, "/bot"+testutil.TestToken+"/getChatAdministrators")
			assert.JSONEq(t, r.body, server.LastCapture().BodyString())

			api100LimiterRow(t, "getChatAdministrators", api100Admins, func(c *sender.Client, id tg.ChatID) error {
				_, err := r.call(c, id)
				return err
			})
		})
	}

	// the public signature of GetChatAdministrators stays as base (t2 O-T1-10)
	assert.Equal(t, reflect.TypeOf((func(*sender.Client, context.Context, tg.ChatID) ([]tg.ChatMember, error))(nil)),
		reflect.TypeOf((*sender.Client).GetChatAdministrators))
}

func TestAPI100_GetChatAdministrators_ParseErrors(t *testing.T) {
	// API100 T-s-4: the moved parse loop keeps today's error texts; both methods give the same text
	want := json.Unmarshal([]byte(`"x"`), new([]json.RawMessage))
	require.Error(t, want)
	rows := []struct {
		name, result, text string
	}{
		{"not an array", `"x"`, "galigo: getChatAdministrators: failed to parse response: " + want.Error()},
		{"unknown status", `[{"status":"bogus"}]`, `galigo: getChatAdministrators: unknown chat member status: "bogus"`},
	}
	calls := map[string]func(*sender.Client) ([]tg.ChatMember, error){
		"GetChatAdministrators": func(c *sender.Client) ([]tg.ChatMember, error) {
			return c.GetChatAdministrators(context.Background(), api100Chat)
		},
		"GetChatAdministratorsWithBots": func(c *sender.Client) ([]tg.ChatMember, error) {
			return c.GetChatAdministratorsWithBots(context.Background(), api100Chat)
		},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			texts := map[string]string{}
			for name, call := range calls {
				server := testutil.NewMockServer(t)
				server.On("/bot"+testutil.TestToken+"/getChatAdministrators", func(w http.ResponseWriter, _ *http.Request) {
					api96ReplyRaw(w, `{"ok":true,"result":`+r.result+`}`)
				})
				_, err := call(testutil.NewTestClient(t, server.BaseURL()))
				require.Error(t, err, name)
				assert.Equal(t, r.text, err.Error(), name)
				if r.name == "not an array" {
					var ute *json.UnmarshalTypeError
					assert.True(t, errors.As(err, &ute), "%s: %v", name, err)
				}
				texts[name] = err.Error()
			}
			assert.Equal(t, texts["GetChatAdministrators"], texts["GetChatAdministratorsWithBots"])
		})
	}
}

func TestAPI100_GetChatAdministratorsWithBots_Validation(t *testing.T) {
	// API100 T-s-15: the same validation error as GetChatAdministrators, and no request
	for _, id := range []tg.ChatID{"", int64(0), nil} {
		server := testutil.NewMockServer(t)
		client := testutil.NewTestClient(t, server.BaseURL())
		_, wantErr := client.GetChatAdministrators(context.Background(), id)
		require.Error(t, wantErr)
		_, err := client.GetChatAdministratorsWithBots(context.Background(), id)
		require.Error(t, err)
		assert.Equal(t, wantErr.Error(), err.Error(), "chat id %#v", id)
		assert.Equal(t, 0, server.CaptureCount(), "chat id %#v", id)
	}
}
