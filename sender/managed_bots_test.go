package sender_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 9.6 managed bots and prepared keyboard buttons (B-25 slice 1, TEST-PLAN §3, rows T-s-5 … T-s-10).

// api96FakeToken is a made-up token in Telegram's format; it is not a real credential.
const api96FakeToken = "7000001:AAH-fake-managed-token"

func api96ReplyRaw(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

// T-s-5: both token methods call their own method with body {"user_id":42}, return the token, and the token
// never shows in fmt verbs or a slog JSON record.
func TestManagedBotToken96_GetAndReplace(t *testing.T) {
	rows := []struct {
		method string
		call   func(*sender.Client) (tg.SecretToken, error)
	}{
		{"getManagedBotToken", func(c *sender.Client) (tg.SecretToken, error) {
			return c.GetManagedBotToken(context.Background(), 42)
		}},
		{"replaceManagedBotToken", func(c *sender.Client) (tg.SecretToken, error) {
			return c.ReplaceManagedBotToken(context.Background(), 42)
		}},
	}
	for _, r := range rows {
		t.Run(r.method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/"+r.method, func(w http.ResponseWriter, _ *http.Request) {
				// raw bytes: a tg.SecretToken passed to a JSON encoder would be written as [REDACTED]
				api96ReplyRaw(w, `{"ok":true,"result":"`+api96FakeToken+`"}`)
			})
			client := testutil.NewTestClient(t, server.BaseURL())

			tok, err := r.call(client)
			require.NoError(t, err)
			assert.Equal(t, api96FakeToken, tok.Value())
			require.Equal(t, 1, server.CaptureCount())
			server.LastCapture().AssertPath(t, "/bot"+testutil.TestToken+"/"+r.method)
			assert.JSONEq(t, `{"user_id":42}`, server.LastCapture().BodyString())

			printed := fmt.Sprintf("%v|%+v|%#v|%s", tok, tok, tok, tok)
			var buf bytes.Buffer
			slog.New(slog.NewJSONHandler(&buf, nil)).Info("token", "token", tok)
			for _, out := range []string{printed, buf.String()} {
				for _, part := range []string{api96FakeToken, "7000001", "AAH-fake-managed-token"} {
					assert.NotContains(t, out, part)
				}
			}
		})
	}
}

// T-s-6: a user id <= 0 is a *tg.ValidationError on user_id, and no request is sent.
func TestManagedBots96_UserIDValidation(t *testing.T) {
	calls := map[string]func(*sender.Client, int64) error{
		"GetManagedBotToken": func(c *sender.Client, id int64) error {
			_, err := c.GetManagedBotToken(context.Background(), id)
			return err
		},
		"ReplaceManagedBotToken": func(c *sender.Client, id int64) error {
			_, err := c.ReplaceManagedBotToken(context.Background(), id)
			return err
		},
		"SavePreparedKeyboardButton": func(c *sender.Client, id int64) error {
			_, err := c.SavePreparedKeyboardButton(context.Background(), sender.SavePreparedKeyboardButtonRequest{
				UserID: id, Button: tg.KeyboardButton{Text: "x", RequestUsers: &tg.KeyboardButtonRequestUsers{RequestID: 1}},
			})
			return err
		},
	}
	for name, call := range calls {
		for _, id := range []int64{0, -1} {
			t.Run(fmt.Sprintf("%s %d", name, id), func(t *testing.T) {
				server := testutil.NewMockServer(t)
				client := testutil.NewTestClient(t, server.BaseURL())
				err := call(client, id)
				var ve *tg.ValidationError
				require.ErrorAs(t, err, &ve)
				assert.Equal(t, "user_id", ve.Field)
				assert.Equal(t, 0, server.CaptureCount())
			})
		}
	}
}

// T-s-7: replaceManagedBotToken is not retried (it revokes the token); SendMessage on the same client and mock is
// retried, so the row can see a retry.
func TestReplaceManagedBotToken96_NotRetried(t *testing.T) {
	server := testutil.NewMockServer(t)
	var replaceCalls, sendCalls atomic.Int32
	server.On("/bot"+testutil.TestToken+"/replaceManagedBotToken", func(w http.ResponseWriter, _ *http.Request) {
		if replaceCalls.Add(1) == 1 {
			testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway")
			return
		}
		api96ReplyRaw(w, `{"ok":true,"result":"`+api96FakeToken+`"}`)
	})
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, _ *http.Request) {
		if sendCalls.Add(1) == 1 {
			testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway")
			return
		}
		testutil.ReplyMessage(w, 4242)
	})
	sleeper := &testutil.FakeSleeper{}
	client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper)

	_, err := client.ReplaceManagedBotToken(context.Background(), 42)
	require.Error(t, err)
	assert.Equal(t, 1, server.CaptureCount(), "exactly one request: no retry")
	assert.Empty(t, sleeper.Calls(), "no backoff sleep")

	// control: SendMessage is wrapped in withRetry
	server.ResetCaptures()
	_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: int64(-100123), Text: "hi"})
	require.NoError(t, err)
	assert.Equal(t, 2, server.CaptureCount(), "the control call is retried once")
}

// T-s-8: savePreparedKeyboardButton sends user_id and the whole button (any button type: Telegram checks it)
// and returns the prepared button.
func TestSavePreparedKeyboardButton96(t *testing.T) {
	rows := []struct {
		name   string
		button tg.KeyboardButton
		body   string
	}{
		{"request_managed_bot", tg.KeyboardButton{Text: "New bot", RequestManagedBot: &tg.KeyboardButtonRequestManagedBot{
			RequestID: 7, SuggestedName: "Helper", SuggestedUsername: "helper_test_bot",
		}}, `{"user_id":42,"button":{"text":"New bot","request_managed_bot":{"request_id":7,"suggested_name":"Helper","suggested_username":"helper_test_bot"}}}`},
		{"request_users", tg.KeyboardButton{Text: "Pick users", RequestUsers: &tg.KeyboardButtonRequestUsers{RequestID: 9}},
			`{"user_id":42,"button":{"text":"Pick users","request_users":{"request_id":9}}}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/savePreparedKeyboardButton", func(w http.ResponseWriter, _ *http.Request) {
				testutil.ReplyOK(w, map[string]any{"id": "prep-btn-1"})
			})
			client := testutil.NewTestClient(t, server.BaseURL())

			got, err := client.SavePreparedKeyboardButton(context.Background(), sender.SavePreparedKeyboardButtonRequest{UserID: 42, Button: r.button})
			require.NoError(t, err)
			assert.Equal(t, &tg.PreparedKeyboardButton{ID: "prep-btn-1"}, got)
			require.Equal(t, 1, server.CaptureCount())
			server.LastCapture().AssertPath(t, "/bot"+testutil.TestToken+"/savePreparedKeyboardButton")
			assert.JSONEq(t, r.body, server.LastCapture().BodyString())
		})
	}
}

// T-s-10: a non-string result is an error whose text does not contain the result's bytes.
func TestManagedBotToken96_ObjectResult(t *testing.T) {
	const marker = "MARKER-96-should-not-leak"
	for _, method := range []string{"getManagedBotToken", "replaceManagedBotToken"} {
		t.Run(method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/"+method, func(w http.ResponseWriter, _ *http.Request) {
				api96ReplyRaw(w, `{"ok":true,"result":{"token":"`+marker+`"}}`)
			})
			client := testutil.NewTestClient(t, server.BaseURL())
			var err error
			if method == "getManagedBotToken" {
				_, err = client.GetManagedBotToken(context.Background(), 42)
			} else {
				_, err = client.ReplaceManagedBotToken(context.Background(), 42)
			}
			require.Error(t, err)
			assert.False(t, strings.Contains(err.Error(), marker), "error text: %q", err.Error())
			assert.Equal(t, 1, server.CaptureCount(), "exactly one request seen")
		})
	}
}

// ==================== Bot API 10.0: managed bot access settings (B-25 slice 2a, TEST-PLAN §3) ====================

func TestAPI100_GetManagedBotAccessSettings(t *testing.T) {
	// API100 T-s-10
	u1 := tg.User{ID: testutil.TestUserID, FirstName: "Test", Username: testutil.TestUsername}
	u2 := tg.User{ID: 555000111, IsBot: true, FirstName: "Managed", Username: "managed_test_bot"}
	rows := []struct {
		name   string
		result string
		want   *tg.BotAccessSettings
	}{
		{"(a) restricted with users", `{"is_access_restricted":true,"added_users":[` +
			`{"id":987654321,"is_bot":false,"first_name":"Test","username":"testuser"},` +
			`{"id":555000111,"is_bot":true,"first_name":"Managed","username":"managed_test_bot"}]}`,
			&tg.BotAccessSettings{IsAccessRestricted: true, AddedUsers: []tg.User{u1, u2}}},
		{"(b) not restricted", `{"is_access_restricted":false}`, &tg.BotAccessSettings{}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/getManagedBotAccessSettings", func(w http.ResponseWriter, _ *http.Request) {
				api96ReplyRaw(w, `{"ok":true,"result":`+r.result+`}`)
			})
			client := testutil.NewTestClient(t, server.BaseURL())
			got, err := client.GetManagedBotAccessSettings(context.Background(), 42)
			require.NoError(t, err)
			assert.Equal(t, r.want, got)
			require.Equal(t, 1, server.CaptureCount())
			server.LastCapture().AssertPath(t, "/bot"+testutil.TestToken+"/getManagedBotAccessSettings")
			assert.JSONEq(t, `{"user_id":42}`, server.LastCapture().BodyString())
		})
	}
	t.Run("(c) malformed result", func(t *testing.T) {
		server := testutil.NewMockServer(t)
		server.On("/bot"+testutil.TestToken+"/getManagedBotAccessSettings", func(w http.ResponseWriter, _ *http.Request) {
			api96ReplyRaw(w, `{"ok":true,"result":"x"}`)
		})
		_, err := testutil.NewTestClient(t, server.BaseURL()).GetManagedBotAccessSettings(context.Background(), 42)
		// a malformed result: callJSON's wrap names the method and keeps the decode error (A-C1-03)
		require.Error(t, err)
		assert.True(t, strings.HasPrefix(err.Error(), "galigo: getManagedBotAccessSettings: failed to parse response: "), err.Error())
		var ute *json.UnmarshalTypeError
		assert.True(t, errors.As(err, &ute), "error %v", err)
	})
}

func TestAPI100_GetManagedBotAccessSettings_Validation(t *testing.T) {
	// API100 T-s-11
	for _, id := range []int64{0, -1} {
		server := testutil.NewMockServer(t)
		client := testutil.NewTestClient(t, server.BaseURL())
		_, err := client.GetManagedBotAccessSettings(context.Background(), id)
		var ve *tg.ValidationError
		require.True(t, errors.As(err, &ve), "user %d: error %v", id, err)
		assert.Equal(t, "user_id", ve.Field)
		assert.Equal(t, "galigo: validation: user_id - must be positive", err.Error()) // the whole text (A-C1-02)
		assert.Equal(t, 0, server.CaptureCount())
	}
}

func TestAPI100_SetManagedBotAccessSettings(t *testing.T) {
	// API100 T-s-12
	ten := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	eleven := append(append([]int64{}, ten...), 11)
	rows := []struct {
		name string
		req  sender.SetManagedBotAccessSettingsRequest
		body string
	}{
		{"(a) false, nil ids", sender.SetManagedBotAccessSettingsRequest{UserID: 42},
			`{"user_id":42,"is_access_restricted":false}`},
		{"(b) 10 ids", sender.SetManagedBotAccessSettingsRequest{UserID: 42, IsAccessRestricted: true, AddedUserIDs: ten},
			`{"user_id":42,"is_access_restricted":true,"added_user_ids":[1,2,3,4,5,6,7,8,9,10]}`},
		{"(c) empty ids", sender.SetManagedBotAccessSettingsRequest{UserID: 42, IsAccessRestricted: true, AddedUserIDs: []int64{}},
			`{"user_id":42,"is_access_restricted":true,"added_user_ids":[]}`},
		{"(d) 11 ids, sent as given", sender.SetManagedBotAccessSettingsRequest{UserID: 42, IsAccessRestricted: true, AddedUserIDs: eleven},
			`{"user_id":42,"is_access_restricted":true,"added_user_ids":[1,2,3,4,5,6,7,8,9,10,11]}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/setManagedBotAccessSettings", func(w http.ResponseWriter, _ *http.Request) {
				testutil.ReplyBool(w, true)
			})
			client := testutil.NewTestClient(t, server.BaseURL())
			require.NoError(t, client.SetManagedBotAccessSettings(context.Background(), r.req))
			require.Equal(t, 1, server.CaptureCount())
			server.LastCapture().AssertPath(t, "/bot"+testutil.TestToken+"/setManagedBotAccessSettings")
			assert.JSONEq(t, r.body, server.LastCapture().BodyString())
		})
	}
	t.Run("(e) user 0 and -1", func(t *testing.T) {
		for _, id := range []int64{0, -1} {
			server := testutil.NewMockServer(t)
			client := testutil.NewTestClient(t, server.BaseURL())
			err := client.SetManagedBotAccessSettings(context.Background(), sender.SetManagedBotAccessSettingsRequest{UserID: id})
			var ve *tg.ValidationError
			require.True(t, errors.As(err, &ve), "user %d: error %v", id, err)
			assert.Equal(t, "user_id", ve.Field)
			assert.Equal(t, "galigo: validation: user_id - must be positive", err.Error()) // the whole text (A-C1-02)
			assert.Equal(t, 0, server.CaptureCount())
		}
	})
}
