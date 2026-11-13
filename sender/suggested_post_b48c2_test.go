package sender_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"strings"
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

// B-48c2: approveSuggestedPost and declineSuggestedPost (Bot API 9.2), TEST-PLAN rows U1 … U12. Shared helpers:
// api101bPath, api101bOK, api101bFailTransport (sender/joinreq101b_test.go), api96ReplyRaw (sender/managed_bots_test.go),
// api100Chat (sender/chat_info_test.go: -1001234567890, above 2^32).

// b48c2Methods calls each new method with no optional value; every row marked "each" runs for both.
var b48c2Methods = []struct {
	method string
	call   func(context.Context, *sender.Client, int64, int) error
}{
	{"approveSuggestedPost", func(ctx context.Context, c *sender.Client, chat int64, msg int) error {
		return c.ApproveSuggestedPost(ctx, sender.ApproveSuggestedPostRequest{ChatID: chat, MessageID: msg})
	}},
	{"declineSuggestedPost", func(ctx context.Context, c *sender.Client, chat int64, msg int) error {
		return c.DeclineSuggestedPost(ctx, sender.DeclineSuggestedPostRequest{ChatID: chat, MessageID: msg})
	}},
}

// b48c2Errors are the API errors U4 gives each method. The first two are the 400 texts the server returned on
// 2026-11-12 in the test supergroup (E29/lookups/probe-b48c2-raw-console.txt): a plain message, and a message id
// with no message. The others are synthetic: an error of another code, or one that maps to a sentinel, reaches the
// caller unchanged, with its sentinel.
var b48c2Errors = []struct {
	code     int
	text     string
	sentinel error // nil: the text and code map to no sentinel
}{
	{400, "Bad Request: message can't be used in the method", nil},
	{400, "Bad Request: suggested post not found", nil},
	{400, "Bad Request: not enough rights to manage direct messages", tg.ErrNoRights},
	{403, "Forbidden: bot is not a member of the channel chat", tg.ErrForbidden},
	{403, "Forbidden: bot was kicked from the channel chat", tg.ErrBotKicked}, // the text maps first (tg.DetectSentinel)
	{404, "Not Found", tg.ErrNotFound},
}

// b48c2Sentinels are every tg sentinel (as tg/errors_test.go TestSentinelErrors_AreDistinct lists them).
var b48c2Sentinels = []error{
	tg.ErrUnauthorized, tg.ErrForbidden, tg.ErrNotFound, tg.ErrTooManyRequests, tg.ErrMessageNotFound,
	tg.ErrMessageNotModified, tg.ErrMessageCantBeEdited, tg.ErrMessageCantBeDeleted, tg.ErrMessageTooOld,
	tg.ErrBotBlocked, tg.ErrBotKicked, tg.ErrChatNotFound, tg.ErrUserDeactivated, tg.ErrNoRights,
	tg.ErrCallbackExpired, tg.ErrInvalidCallbackData, tg.ErrRateLimited, tg.ErrCircuitOpen, tg.ErrMaxRetries,
	tg.ErrResponseTooLarge, tg.ErrInvalidToken, tg.ErrPathTraversal, tg.ErrInvalidConfig,
}

// b48c2OnlySentinel asserts that err matches want and no other tg sentinel (want nil: none).
func b48c2OnlySentinel(t *testing.T, err, want error) {
	t.Helper()
	if want != nil {
		require.Contains(t, b48c2Sentinels, want, "the expected sentinel is in the list (A-T3-03)")
	}
	for _, s := range b48c2Sentinels {
		if errors.Is(s, want) { // the sentinels are distinct errors.New values, so this is identity
			assert.ErrorIs(t, err, s)
		} else {
			assert.NotErrorIs(t, err, s)
		}
	}
}

func b48c2OK(server *testutil.MockTelegramServer) {
	api101bOK(server, "approveSuggestedPost", "declineSuggestedPost")
}

// U1: the request of each method with no optional value.
func TestB48C2_U1_Body(t *testing.T) {
	for _, m := range b48c2Methods {
		t.Run(m.method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			b48c2OK(server)
			client := testutil.NewTestClient(t, server.BaseURL())
			require.NoError(t, m.call(context.Background(), client, api100Chat, 42))
			require.Equal(t, 1, server.CaptureCount())
			capt := server.LastCapture()
			capt.AssertPath(t, api101bPath(m.method))
			capt.AssertMethod(t, http.MethodPost)
			capt.AssertContentType(t, "application/json")
			assert.JSONEq(t, `{"chat_id":-1001234567890,"message_id":42}`, string(capt.Body))
		})
	}
}

// U1b: the optional values are sent when set; only 0 / "" leaves them out.
func TestB48C2_U1b_Optional(t *testing.T) {
	cases := []struct {
		name, method, want, raw string
		call                    func(context.Context, *sender.Client) error
	}{
		{"send_date", "approveSuggestedPost", `{"chat_id":-1001234567890,"message_id":42,"send_date":1900000000}`, "",
			func(ctx context.Context, c *sender.Client) error {
				return c.ApproveSuggestedPost(ctx, sender.ApproveSuggestedPostRequest{ChatID: api100Chat, MessageID: 42, SendDate: 1900000000})
			}},
		{"send_date_large", "approveSuggestedPost", `{"chat_id":-1001234567890,"message_id":42,"send_date":2200000000}`, "",
			func(ctx context.Context, c *sender.Client) error { // above 2^31-1: an int32 narrowing would change it
				return c.ApproveSuggestedPost(ctx, sender.ApproveSuggestedPostRequest{ChatID: api100Chat, MessageID: 42, SendDate: 2200000000})
			}},
		{"send_date_negative", "approveSuggestedPost", `{"chat_id":-1001234567890,"message_id":42,"send_date":-1}`, "",
			func(ctx context.Context, c *sender.Client) error {
				return c.ApproveSuggestedPost(ctx, sender.ApproveSuggestedPostRequest{ChatID: api100Chat, MessageID: 42, SendDate: -1})
			}},
		{"comment", "declineSuggestedPost", `{"chat_id":-1001234567890,"message_id":42,"comment":"ok 🙂"}`, `"comment":"ok 🙂"`,
			func(ctx context.Context, c *sender.Client) error {
				return c.DeclineSuggestedPost(ctx, sender.DeclineSuggestedPostRequest{ChatID: api100Chat, MessageID: 42, Comment: "ok 🙂"})
			}},
		{"comment_spaces", "declineSuggestedPost", `{"chat_id":-1001234567890,"message_id":42,"comment":"  OK Привет  "}`, `"comment":"  OK Привет  "`,
			func(ctx context.Context, c *sender.Client) error { // E-0117: a value a normaliser (trim, case fold; O-T3-01) would change is sent as given
				return c.DeclineSuggestedPost(ctx, sender.DeclineSuggestedPostRequest{ChatID: api100Chat, MessageID: 42, Comment: "  OK Привет  "})
			}},
		{"comment_long", "declineSuggestedPost", `{"chat_id":-1001234567890,"message_id":42,"comment":"` + strings.Repeat("x", 129) + `"}`, "",
			func(ctx context.Context, c *sender.Client) error {
				return c.DeclineSuggestedPost(ctx, sender.DeclineSuggestedPostRequest{ChatID: api100Chat, MessageID: 42, Comment: strings.Repeat("x", 129)})
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			b48c2OK(server)
			client := testutil.NewTestClient(t, server.BaseURL())
			require.NoError(t, tc.call(context.Background(), client))
			require.Equal(t, 1, server.CaptureCount())
			capt := server.LastCapture()
			capt.AssertPath(t, api101bPath(tc.method))
			assert.JSONEq(t, tc.want, string(capt.Body))
			if tc.raw != "" {
				assert.Contains(t, string(capt.Body), tc.raw, "the comment is sent as raw UTF-8")
			}
		})
	}
}

// U2: a bad id is refused locally with the field it names, before any request (E-0074, E-0143).
func TestB48C2_U2_Validation(t *testing.T) {
	inputs := []struct {
		name  string
		chat  int64
		msg   int
		field string
		text  string
	}{
		{"chat_0", 0, 42, "chat_id", "galigo: validation: chat_id - required"},
		{"message_0", -100, 0, "message_id", "galigo: validation: message_id - must be positive"},
		{"message_negative", -100, -1, "message_id", "galigo: validation: message_id - must be positive"},
		{"both_0", 0, 0, "chat_id", "galigo: validation: chat_id - required"},
	}
	for _, m := range b48c2Methods {
		for _, in := range inputs {
			t.Run(m.method+"/"+in.name, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				b48c2OK(server)
				client := testutil.NewTestClient(t, server.BaseURL())
				err := m.call(context.Background(), client, in.chat, in.msg)
				var ve *tg.ValidationError
				require.True(t, errors.As(err, &ve), "error %v", err)
				assert.Equal(t, in.field, ve.Field)
				assert.Equal(t, in.text, err.Error())
				assert.Equal(t, 0, server.CaptureCount())
			})
		}
	}
}

// U3: valid edge ids are sent as given (a negative channel id, a positive id above 2^32, the smallest message id).
func TestB48C2_U3_ValidEdges(t *testing.T) {
	cases := []struct {
		chat int64
		msg  int
		want string
	}{
		{1, 1, `{"chat_id":1,"message_id":1}`},
		{-1, 1, `{"chat_id":-1,"message_id":1}`},
		{5000000001, 2, `{"chat_id":5000000001,"message_id":2}`},
		{api100Chat, 1, `{"chat_id":-1001234567890,"message_id":1}`},
	}
	if strconv.IntSize == 64 { // a message id above 2^31 (A-C1-01); int is 32 bits on 386, where no such id fits
		cases = append(cases, struct {
			chat int64
			msg  int
			want string
		}{1, int(b48c2BigMessageID), `{"chat_id":1,"message_id":3000000000}`})
	}
	for _, m := range b48c2Methods {
		for _, c := range cases {
			server := testutil.NewMockServer(t)
			b48c2OK(server)
			client := testutil.NewTestClient(t, server.BaseURL())
			require.NoError(t, m.call(context.Background(), client, c.chat, c.msg), "%s %d", m.method, c.chat)
			require.Equal(t, 1, server.CaptureCount(), "%s %d", m.method, c.chat)
			assert.JSONEq(t, c.want, string(server.LastCapture().Body), "%s %d", m.method, c.chat)
		}
	}
}

// b48c2BigMessageID is a message id above 2^31; a variable, so the file still builds where int has 32 bits.
var b48c2BigMessageID int64 = 3000000000

// U4: an API error is the *tg.APIError with its code, text and method, and keeps its sentinel and no other.
func TestB48C2_U4_APIError(t *testing.T) {
	for _, m := range b48c2Methods {
		for _, e := range b48c2Errors {
			t.Run(m.method+"/"+e.text, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				server.On(api101bPath(m.method), func(w http.ResponseWriter, _ *http.Request) {
					testutil.ReplyError(w, e.code, e.text, nil)
				})
				client := testutil.NewTestClient(t, server.BaseURL())
				err := m.call(context.Background(), client, api100Chat, 42)
				assert.IsType(t, &tg.APIError{}, err, "the *tg.APIError itself, not wrapped (AC5; A-T3-01)")
				var apiErr *tg.APIError
				require.True(t, errors.As(err, &apiErr), "error %v", err)
				assert.Equal(t, e.code, apiErr.Code)
				assert.Equal(t, e.text, apiErr.Description)
				assert.Equal(t, m.method, apiErr.Method)
				b48c2OnlySentinel(t, err, e.sentinel) // the expected sentinel and no other (none for the measured texts)
				assert.Equal(t, 1, server.CaptureCount())
			})
		}
	}
}

// U5: neither method retries a 502 or a 429; SendMessage on the same client is the control.
func TestB48C2_U5_NoRetry(t *testing.T) {
	sets := []struct {
		name  string
		first func(http.ResponseWriter)
		check func(*testing.T, string, error)
	}{
		{"502", func(w http.ResponseWriter) { testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway") },
			func(t *testing.T, method string, err error) {
				assert.IsType(t, &tg.APIError{}, err, "not wrapped (AC5)")
				var apiErr *tg.APIError
				require.True(t, errors.As(err, &apiErr), "error %v", err)
				assert.Equal(t, http.StatusBadGateway, apiErr.Code)
				assert.Equal(t, method, apiErr.Method)
				assert.Equal(t, "Bad Gateway", apiErr.Description)
				b48c2OnlySentinel(t, err, nil)
			}},
		{"429", func(w http.ResponseWriter) { testutil.ReplyRateLimit(w, 1) },
			func(t *testing.T, method string, err error) {
				b48c2OnlySentinel(t, err, tg.ErrTooManyRequests)
				assert.IsType(t, &tg.APIError{}, err, "not wrapped (AC5)")
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
			for _, m := range b48c2Methods {
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
			for _, m := range b48c2Methods {
				server.ResetCaptures()
				set.check(t, m.method, m.call(context.Background(), client, api100Chat, 42))
				assert.Equal(t, 1, server.CaptureCount(), "%s: exactly one request", m.method)
			}
			assert.Equal(t, 0, sleeper.CallCount(), "no backoff sleep before the control")

			server.ResetCaptures()
			_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api100Chat, Text: "hi"})
			require.NoError(t, err)
			assert.Equal(t, 2, server.CaptureCount(), "the control call is retried once")
			assert.GreaterOrEqual(t, sleeper.CallCount(), 1, "the sleeper is wired")
		})
	}
}

// U5b: neither method retries a network timeout; SendMessage is the control.
func TestB48C2_U5b_NoRetryNetworkError(t *testing.T) {
	rt := &api101bFailTransport{}
	sleeper := &testutil.FakeSleeper{}
	client := testutil.NewRetryTestClient(t, "http://galigo.invalid", sleeper, sender.WithHTTPClient(&http.Client{Transport: rt}),
		sender.WithRateLimit(1000, 100), sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))
	for _, m := range b48c2Methods {
		rt.n.Store(0)
		err := m.call(context.Background(), client, api100Chat, 42)
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

// U6: the result is not decoded; a broken body is an error.
func TestB48C2_U6_ResultNotDecoded(t *testing.T) {
	bodies := []struct {
		name, body string
		wantErr    bool
	}{
		{"false", `{"ok":true,"result":false}`, false},
		{"string", `{"ok":true,"result":"x"}`, false},
		{"broken", `{"ok":true,"result":`, true},
	}
	for _, m := range b48c2Methods {
		for _, b := range bodies {
			t.Run(m.method+"/"+b.name, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				server.On(api101bPath(m.method), func(w http.ResponseWriter, _ *http.Request) { api96ReplyRaw(w, b.body) })
				client := testutil.NewTestClient(t, server.BaseURL())
				err := m.call(context.Background(), client, api100Chat, 42)
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

// U7: the call waits on the client's global limiter (burst 0 refuses it).
func TestB48C2_U7_Limiter(t *testing.T) {
	server := testutil.NewMockServer(t)
	b48c2OK(server)
	client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRateLimit(0, 0))
	for _, m := range b48c2Methods {
		err := m.call(context.Background(), client, api100Chat, 42)
		require.Error(t, err, m.method)
		assert.Contains(t, err.Error(), "exceeds limiter's burst", m.method)
	}
	assert.Equal(t, 0, server.CaptureCount())
}

// U8: the limiter key is the chat id (a negative id uses the group limiter, a positive one the per-chat limiter; the
// message id is not the key).
func TestB48C2_U8_LimiterKey(t *testing.T) {
	cases := []struct {
		name                string
		first, again, other int64
	}{
		{"group", -100, -100, -200},
		{"per_chat", 100, 100, 200},
	}
	for _, m := range b48c2Methods {
		for _, c := range cases {
			t.Run(m.method+"/"+c.name, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				b48c2OK(server)
				client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRateLimit(1000, 100),
					sender.WithPerChatRateLimit(0.001, 1), sender.WithGroupRateLimit(0.001, 1))
				call := func(chat int64) error {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					return m.call(ctx, client, chat, 42)
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

// U8c: each method waits on the bucket an existing method (UnbanChatMember) already uses for the same chat, for a
// group and a per-chat id, small and beyond int32 (E-0121: a different key string for the same chat would get a fresh
// bucket).
func TestB48C2_U8c_SharedBucket(t *testing.T) {
	for _, m := range b48c2Methods {
		for _, chat := range []int64{-300, 300, api100Chat, 5000000001} {
			t.Run(fmt.Sprintf("%s/%d", m.method, chat), func(t *testing.T) {
				server := testutil.NewMockServer(t)
				b48c2OK(server)
				client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRateLimit(1000, 100),
					sender.WithPerChatRateLimit(0.001, 1), sender.WithGroupRateLimit(0.001, 1))
				require.NoError(t, client.UnbanChatMember(context.Background(), chat, 7))
				require.Equal(t, 1, server.CaptureCount())
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				err := m.call(ctx, client, chat, 42)
				require.Error(t, err, "the chat's bucket is already empty")
				assert.Contains(t, err.Error(), "would exceed context deadline")
				assert.Equal(t, 1, server.CaptureCount(), "no request for the blocked call")
			})
		}
	}
}

// U8d: a negative chat id uses the group limiter (slow here), not the per-chat one (fast here).
func TestB48C2_U8d_GroupLimiter(t *testing.T) {
	for _, m := range b48c2Methods {
		t.Run(m.method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			b48c2OK(server)
			client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRateLimit(1000, 100),
				sender.WithPerChatRateLimit(1000, 100), sender.WithGroupRateLimit(0.001, 1))
			require.NoError(t, m.call(context.Background(), client, -400, 42))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := m.call(ctx, client, -400, 42)
			require.Error(t, err, "the group limiter blocks the second call")
			assert.Contains(t, err.Error(), "would exceed context deadline")
			assert.Equal(t, 1, server.CaptureCount())
		})
	}
}

// U9: both methods go through the client's circuit breaker, which they share.
func TestB48C2_U9_Breaker(t *testing.T) {
	fail := func(server *testutil.MockTelegramServer) {
		for _, m := range b48c2Methods {
			server.On(api101bPath(m.method), func(w http.ResponseWriter, _ *http.Request) {
				testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway")
			})
		}
	}
	const chat = int64(777) // positive: the group limiter is not used
	newClient := func(t *testing.T, server *testutil.MockTelegramServer) *sender.Client {
		return testutil.NewBreakerTestClient(t, server.BaseURL(), sender.WithRateLimit(1000, 100),
			sender.WithPerChatRateLimit(1000, 100), sender.WithGroupRateLimit(1000, 100))
	}
	for _, m := range b48c2Methods {
		t.Run(m.method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			fail(server)
			client := newClient(t, server)
			for i := 1; i <= 2; i++ {
				err := m.call(context.Background(), client, chat, 42)
				require.Error(t, err)
				assert.False(t, errors.Is(err, gobreaker.ErrOpenState), "call %d: %v", i, err)
				assert.Equal(t, i, server.CaptureCount(), "call %d sends a request", i)
			}
			err := m.call(context.Background(), client, chat, 42)
			assert.True(t, errors.Is(err, gobreaker.ErrOpenState), "call 3: %v", err)
			assert.Equal(t, 2, server.CaptureCount(), "call 3 sends no request")
		})
	}
	t.Run("shared", func(t *testing.T) {
		server := testutil.NewMockServer(t)
		fail(server)
		client := newClient(t, server)
		for range 2 {
			require.Error(t, b48c2Methods[0].call(context.Background(), client, chat, 42))
		}
		err := b48c2Methods[1].call(context.Background(), client, chat, 42)
		assert.True(t, errors.Is(err, gobreaker.ErrOpenState), "error %v", err)
		assert.Equal(t, 2, server.CaptureCount(), "no request while the breaker is open")
	})
}

// U10: a context cancelled before the call stops it before a request is sent.
func TestB48C2_U10_Context(t *testing.T) {
	for _, m := range b48c2Methods {
		t.Run(m.method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			b48c2OK(server)
			client := testutil.NewTestClient(t, server.BaseURL())
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := m.call(ctx, client, api100Chat, 42)
			assert.True(t, errors.Is(err, context.Canceled), "error %v", err)
			assert.Equal(t, 0, server.CaptureCount())
		})
	}
}

// U10b: the caller's context reaches the request: cancelling it while the server holds the reply ends the call.
func TestB48C2_U10b_ContextMidflight(t *testing.T) {
	for _, m := range b48c2Methods {
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
			go func() { done <- m.call(ctx, client, api100Chat, 42) }()

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

// U11: the two methods have exactly the planned signatures.
func TestB48C2_U11_Signatures(t *testing.T) {
	assert.Equal(t, reflect.TypeFor[func(*sender.Client, context.Context, sender.ApproveSuggestedPostRequest) error]().String(),
		reflect.TypeOf((*sender.Client).ApproveSuggestedPost).String())
	assert.Equal(t, reflect.TypeFor[func(*sender.Client, context.Context, sender.DeclineSuggestedPostRequest) error]().String(),
		reflect.TypeOf((*sender.Client).DeclineSuggestedPost).String())
}

// U12: the request types' fields, in order (name, Go type, JSON tag, not embedded).
func TestB48C2_U12_RequestShapes(t *testing.T) {
	type row struct{ name, typ, tag string }
	for _, s := range []struct {
		v    any
		rows []row
	}{
		{sender.ApproveSuggestedPostRequest{}, []row{{"ChatID", "int64", "chat_id"}, {"MessageID", "int", "message_id"}, {"SendDate", "int64", "send_date,omitempty"}}},
		{sender.DeclineSuggestedPostRequest{}, []row{{"ChatID", "int64", "chat_id"}, {"MessageID", "int", "message_id"}, {"Comment", "string", "comment,omitempty"}}},
	} {
		rt := reflect.TypeOf(s.v)
		require.Equal(t, len(s.rows), rt.NumField(), rt.Name())
		for i := range rt.NumField() {
			f := rt.Field(i)
			assert.False(t, f.Anonymous, "%s.%s must be a named field", rt.Name(), f.Name)
			assert.Equal(t, s.rows[i], row{f.Name, f.Type.String(), f.Tag.Get("json")}, "%s field %d", rt.Name(), i)
		}
	}
}
