package sender_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-50: approveChatJoinRequest and declineChatJoinRequest (Bot API 5.4), TEST-PLAN rows U1 … U11.

// b50User is above 2^32, so a 32-bit truncation shows in the body.
const b50User = int64(5000000001)

// b50Methods calls each new method; every row runs for both.
var b50Methods = []struct {
	method string
	call   func(context.Context, *sender.Client, tg.ChatID, int64) error
}{
	{"approveChatJoinRequest", func(ctx context.Context, c *sender.Client, chat tg.ChatID, user int64) error {
		return c.ApproveChatJoinRequest(ctx, chat, user)
	}},
	{"declineChatJoinRequest", func(ctx context.Context, c *sender.Client, chat tg.ChatID, user int64) error {
		return c.DeclineChatJoinRequest(ctx, chat, user)
	}},
}

// b50Texts are the 400 texts Telegram returned on 2026-11-11 when a bot approved (USER_ALREADY_PARTICIPANT) and
// declined (HIDE_REQUESTER_MISSING) a request for its own id: a chat member with no pending request. U4 gives
// each method both texts, so neither method may turn either text into success.
var b50Texts = []string{"Bad Request: USER_ALREADY_PARTICIPANT", "Bad Request: HIDE_REQUESTER_MISSING"}

// U1: the request for each method and chat id kind.
func TestB50_U1_Body(t *testing.T) {
	chats := []struct {
		name string
		chat tg.ChatID
		want string
	}{
		{"int64", int64(-1001234567890), `{"chat_id":-1001234567890,"user_id":5000000001}`},
		{"int", int(-42), `{"chat_id":-42,"user_id":5000000001}`},
		{"username", "@b50chan", `{"chat_id":"@b50chan","user_id":5000000001}`},
	}
	for _, m := range b50Methods {
		for _, c := range chats {
			t.Run(m.method+"/"+c.name, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				api101bOK(server, "approveChatJoinRequest", "declineChatJoinRequest")
				client := testutil.NewTestClient(t, server.BaseURL())
				require.NoError(t, m.call(context.Background(), client, c.chat, b50User))
				require.Equal(t, 1, server.CaptureCount())
				capt := server.LastCapture()
				capt.AssertPath(t, api101bPath(m.method))
				capt.AssertMethod(t, http.MethodPost)
				capt.AssertContentType(t, "application/json")
				assert.JSONEq(t, c.want, string(capt.Body))
			})
		}
	}
}

// U2: invalid ids give UnbanChatMember's error for the same input, and no request is sent.
func TestB50_U2_Validation(t *testing.T) {
	inputs := []struct {
		name string
		chat tg.ChatID
		user int64
	}{
		{"chat_nil", nil, 1},
		{"chat_int64_0", int64(0), 1},
		{"chat_int_0", int(0), 1},
		{"chat_empty", "", 1},
		{"chat_float", float64(1), 1},
		{"user_0", int64(1), 0},
		{"user_neg", int64(1), -1},
		{"both_bad", nil, 0},
	}
	for _, m := range b50Methods {
		for _, in := range inputs {
			t.Run(m.method+"/"+in.name, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				client := testutil.NewTestClient(t, server.BaseURL())
				want := client.UnbanChatMember(context.Background(), in.chat, in.user)
				require.Error(t, want, "oracle")
				got := m.call(context.Background(), client, in.chat, in.user)
				require.Error(t, got)
				assert.Equal(t, want.Error(), got.Error())
				assert.Equal(t, 0, server.CaptureCount())
			})
		}
	}
}

// U3: the smallest valid values are sent as given.
func TestB50_U3_ValidEdges(t *testing.T) {
	chats := []struct {
		chat tg.ChatID
		want string
	}{
		{int64(1), `{"chat_id":1,"user_id":1}`},
		{"@a", `{"chat_id":"@a","user_id":1}`},
	}
	for _, m := range b50Methods {
		for _, c := range chats {
			server := testutil.NewMockServer(t)
			api101bOK(server, "approveChatJoinRequest", "declineChatJoinRequest")
			client := testutil.NewTestClient(t, server.BaseURL())
			require.NoError(t, m.call(context.Background(), client, c.chat, 1), "%s %v", m.method, c.chat)
			require.Equal(t, 1, server.CaptureCount(), "%s %v", m.method, c.chat)
			assert.JSONEq(t, c.want, string(server.LastCapture().Body), "%s %v", m.method, c.chat)
		}
	}
}

// U4: an API error is a *tg.APIError with the method name, for each method and each measured text.
func TestB50_U4_APIError(t *testing.T) {
	for _, m := range b50Methods {
		for _, text := range b50Texts {
			t.Run(m.method+"/"+text, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				server.On(api101bPath(m.method), func(w http.ResponseWriter, _ *http.Request) {
					testutil.ReplyError(w, 400, text, nil)
				})
				client := testutil.NewTestClient(t, server.BaseURL())
				err := m.call(context.Background(), client, api100Chat, b50User)
				var apiErr *tg.APIError
				require.True(t, errors.As(err, &apiErr), "error %v", err)
				assert.Equal(t, 400, apiErr.Code)
				assert.Equal(t, text, apiErr.Description)
				assert.Equal(t, m.method, apiErr.Method)
				assert.Equal(t, 1, server.CaptureCount())
			})
		}
	}
}

// U5: neither method retries a 502 or a 429; SendMessage on the same client is the control.
func TestB50_U5_NoRetry(t *testing.T) {
	sets := []struct {
		name  string
		first func(http.ResponseWriter)
		check func(*testing.T, string, error)
	}{
		{"502", func(w http.ResponseWriter) { testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway") },
			func(t *testing.T, _ string, err error) { require.Error(t, err) }},
		// the 429 error is the *tg.APIError with its details, and carries its sentinel through errors.Is (AC4)
		{"429", func(w http.ResponseWriter) { testutil.ReplyRateLimit(w, 1) },
			func(t *testing.T, method string, err error) {
				require.ErrorIs(t, err, tg.ErrTooManyRequests)
				var apiErr *tg.APIError
				require.True(t, errors.As(err, &apiErr), "error %v", err)
				assert.Equal(t, 429, apiErr.Code)
				assert.Equal(t, method, apiErr.Method)
				assert.Equal(t, "Too Many Requests: retry after 1", apiErr.Description)
				assert.Equal(t, time.Second, apiErr.RetryAfter)
			}},
	}
	for _, set := range sets {
		t.Run(set.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			paths := []string{"sendMessage"}
			for _, m := range b50Methods {
				paths = append(paths, m.method)
			}
			for _, p := range paths {
				var n atomic.Int32
				server.On(api101bPath(p), func(w http.ResponseWriter, _ *http.Request) {
					if n.Add(1) == 1 {
						set.first(w)
						return
					}
					testutil.ReplyMessage(w, 4242)
				})
			}
			sleeper := &testutil.FakeSleeper{}
			client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRateLimit(1000, 100),
				sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))
			for _, m := range b50Methods {
				server.ResetCaptures()
				set.check(t, m.method, m.call(context.Background(), client, api100Chat, b50User))
				assert.Equal(t, 1, server.CaptureCount(), "%s: exactly one request", m.method)
			}
			assert.Equal(t, 0, sleeper.CallCount(), "no backoff sleep before the control")

			// control, last: SendMessage is wrapped in withRetry
			server.ResetCaptures()
			_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api100Chat, Text: "hi"})
			require.NoError(t, err)
			assert.Equal(t, 2, server.CaptureCount(), "the control call is retried once")
			assert.GreaterOrEqual(t, sleeper.CallCount(), 1, "the sleeper is wired")
		})
	}
}

// U5b: neither method retries a network timeout; SendMessage is the control.
func TestB50_U5b_NoRetryNetworkError(t *testing.T) {
	rt := &api101bFailTransport{}
	sleeper := &testutil.FakeSleeper{}
	client := testutil.NewRetryTestClient(t, "http://galigo.invalid", sleeper, sender.WithHTTPClient(&http.Client{Transport: rt}),
		sender.WithRateLimit(1000, 100), sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))
	for _, m := range b50Methods {
		rt.n.Store(0)
		err := m.call(context.Background(), client, api100Chat, b50User)
		var ne net.Error
		require.True(t, errors.As(err, &ne) && ne.Timeout(), "%s: error %v", m.method, err)
		assert.Equal(t, int32(1), rt.n.Load(), "%s: exactly one round trip", m.method)
	}
	assert.Equal(t, 0, sleeper.CallCount())

	rt.n.Store(0)
	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api100Chat, Text: "hi"})
	require.Error(t, err)
	assert.GreaterOrEqual(t, rt.n.Load(), int32(2), "the control call is retried")
}

// U6: the result is not decoded (as UnbanChatMember); a broken body is an error.
func TestB50_U6_ResultNotDecoded(t *testing.T) {
	bodies := []struct {
		name, body string
		wantErr    bool
	}{
		{"false", `{"ok":true,"result":false}`, false},
		{"string", `{"ok":true,"result":"x"}`, false},
		{"broken", `{"ok":true,"result":`, true},
	}
	for _, m := range b50Methods {
		for _, b := range bodies {
			t.Run(m.method+"/"+b.name, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				server.On(api101bPath(m.method), func(w http.ResponseWriter, _ *http.Request) { api96ReplyRaw(w, b.body) })
				client := testutil.NewTestClient(t, server.BaseURL())
				err := m.call(context.Background(), client, api100Chat, b50User)
				if b.wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				assert.Equal(t, 1, server.CaptureCount())
			})
		}
	}
}

// U7: the call waits on the client's limiter (global burst 0 refuses it).
func TestB50_U7_Limiter(t *testing.T) {
	server := testutil.NewMockServer(t)
	api101bOK(server, "approveChatJoinRequest", "declineChatJoinRequest")
	client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRateLimit(0, 0))
	for _, m := range b50Methods {
		err := m.call(context.Background(), client, api100Chat, b50User)
		require.Error(t, err, m.method)
		assert.Contains(t, err.Error(), "exceeds limiter's burst", m.method)
	}
	assert.Equal(t, 0, server.CaptureCount())
}

// U8: the limiter key is the chat id (the same chat as int64 and int shares it; the user id is not the key).
func TestB50_U8_LimiterKey(t *testing.T) {
	cases := []struct {
		name                string
		first, again, other tg.ChatID
	}{
		{"numeric_group", int64(-100), int(-100), int64(-200)},
		{"username", "@b50a", "@b50a", "@b50b"},
	}
	for _, m := range b50Methods {
		for _, c := range cases {
			t.Run(m.method+"/"+c.name, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				api101bOK(server, "approveChatJoinRequest", "declineChatJoinRequest")
				client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRateLimit(1000, 100),
					sender.WithPerChatRateLimit(0.001, 1), sender.WithGroupRateLimit(0.001, 1))
				call := func(chat tg.ChatID) error {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					return m.call(ctx, client, chat, b50User)
				}
				require.NoError(t, call(c.first))
				require.Equal(t, 1, server.CaptureCount())
				err := call(c.again)
				require.Error(t, err, "same chat: blocked")
				assert.Contains(t, err.Error(), "would exceed context deadline")
				assert.Equal(t, 1, server.CaptureCount(), "no request for the blocked call")
				require.NoError(t, call(c.other), "another chat has its own limiter")
				assert.Equal(t, 2, server.CaptureCount())
			})
		}
	}
}

// U8 (c): the new methods share the chat's bucket with the existing methods (UnbanChatMember).
func TestB50_U8_SharedBucket(t *testing.T) {
	for _, m := range b50Methods {
		t.Run(m.method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			api101bOK(server, "approveChatJoinRequest", "declineChatJoinRequest")
			client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRateLimit(1000, 100),
				sender.WithPerChatRateLimit(0.001, 1), sender.WithGroupRateLimit(0.001, 1))
			require.NoError(t, client.UnbanChatMember(context.Background(), int64(-300), b50User))
			require.Equal(t, 1, server.CaptureCount())
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := m.call(ctx, client, int64(-300), b50User)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "would exceed context deadline")
			assert.Equal(t, 1, server.CaptureCount(), "no request for the blocked call")
		})
	}
}

// U8 (d): a negative numeric chat id uses the group limiter, not the per-chat one.
func TestB50_U8_GroupLimiter(t *testing.T) {
	for _, m := range b50Methods {
		t.Run(m.method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			api101bOK(server, "approveChatJoinRequest", "declineChatJoinRequest")
			client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRateLimit(1000, 100),
				sender.WithPerChatRateLimit(1000, 100), sender.WithGroupRateLimit(0.001, 1))
			require.NoError(t, m.call(context.Background(), client, int64(-400), b50User))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := m.call(ctx, client, int64(-400), b50User)
			require.Error(t, err, "the group limiter blocks the second call")
			assert.Contains(t, err.Error(), "would exceed context deadline")
			assert.Equal(t, 1, server.CaptureCount())
		})
	}
}

// U9: both methods go through the client's circuit breaker, which they share.
func TestB50_U9_Breaker(t *testing.T) {
	fail := func(server *testutil.MockTelegramServer) {
		for _, m := range b50Methods {
			server.On(api101bPath(m.method), func(w http.ResponseWriter, _ *http.Request) {
				testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway")
			})
		}
	}
	const chat = int64(777) // positive: the group limiter is not used (P2-TP)
	newClient := func(t *testing.T, server *testutil.MockTelegramServer) *sender.Client {
		return testutil.NewBreakerTestClient(t, server.BaseURL(), sender.WithRateLimit(1000, 100),
			sender.WithPerChatRateLimit(1000, 100), sender.WithGroupRateLimit(1000, 100))
	}
	for _, m := range b50Methods {
		t.Run(m.method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			fail(server)
			client := newClient(t, server)
			for i := 1; i <= 2; i++ {
				err := m.call(context.Background(), client, chat, b50User)
				require.Error(t, err)
				assert.False(t, errors.Is(err, gobreaker.ErrOpenState), "call %d: %v", i, err)
				assert.Equal(t, i, server.CaptureCount(), "call %d sends a request", i)
			}
			err := m.call(context.Background(), client, chat, b50User)
			assert.True(t, errors.Is(err, gobreaker.ErrOpenState), "call 3: %v", err)
			assert.Equal(t, 2, server.CaptureCount(), "call 3 sends no request")
		})
	}
	t.Run("shared", func(t *testing.T) {
		server := testutil.NewMockServer(t)
		fail(server)
		client := newClient(t, server)
		for range 2 {
			require.Error(t, b50Methods[0].call(context.Background(), client, chat, b50User))
		}
		err := b50Methods[1].call(context.Background(), client, chat, b50User)
		assert.True(t, errors.Is(err, gobreaker.ErrOpenState), "error %v", err)
		assert.Equal(t, 2, server.CaptureCount(), "no request while the breaker is open")
	})
}

// U10: a context cancelled before the call stops it before a request is sent.
func TestB50_U10_Context(t *testing.T) {
	for _, m := range b50Methods {
		t.Run(m.method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			api101bOK(server, "approveChatJoinRequest", "declineChatJoinRequest")
			client := testutil.NewTestClient(t, server.BaseURL())
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := m.call(ctx, client, api100Chat, b50User)
			assert.True(t, errors.Is(err, context.Canceled), "error %v", err)
			assert.Equal(t, 0, server.CaptureCount())
		})
	}
}

// U10b: the caller's context reaches the request: cancelling it while the server holds the reply ends the call.
func TestB50_U10b_ContextMidflight(t *testing.T) {
	for _, m := range b50Methods {
		t.Run(m.method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			arrived := make(chan struct{})
			release := make(chan struct{})
			t.Cleanup(func() { close(release) }) // runs before the server closes (cleanups run last-in first-out)
			server.On(api101bPath(m.method), func(w http.ResponseWriter, _ *http.Request) {
				close(arrived)
				<-release
				api96ReplyRaw(w, `{"ok":true,"result":true}`)
			})
			client := testutil.NewTestClient(t, server.BaseURL())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- m.call(ctx, client, api100Chat, b50User) }()

			select {
			case <-arrived:
			case err := <-done:
				t.Fatalf("the call returned before the request arrived: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("the request did not arrive")
			}
			cancel()
			select {
			case err := <-done:
				assert.True(t, errors.Is(err, context.Canceled), "error %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("the call did not end after cancel")
			}
			assert.Equal(t, 1, server.CaptureCount())
		})
	}
}

// U11: the two methods have exactly the planned signatures (an extra variadic parameter would still compile at every
// call site).
func TestB50_U11_Signatures(t *testing.T) {
	want := reflect.TypeFor[func(*sender.Client, context.Context, tg.ChatID, int64) error]().String()
	assert.Equal(t, want, reflect.TypeOf((*sender.Client).ApproveChatJoinRequest).String())
	assert.Equal(t, want, reflect.TypeOf((*sender.Client).DeclineChatJoinRequest).String())
}
