package sender_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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

// B-32: a null or empty result is an error in getManagedBotToken, replaceManagedBotToken, savePreparedKeyboardButton,
// answerGuestQuery and getManagedBotAccessSettings; a real value is returned as given; a decode error keeps its text
// and cause; the new errors are not breaker failures and are not retried; other callJSON methods are unchanged.

// b32Server answers method with the given raw reply bodies, one per call, in order.
func b32Server(t *testing.T, method string, bodies ...string) *testutil.MockTelegramServer {
	t.Helper()
	server := testutil.NewMockServer(t)
	var n atomic.Int32
	server.On("/bot"+testutil.TestToken+"/"+method, func(w http.ResponseWriter, _ *http.Request) {
		i := int(n.Add(1)) - 1
		if i >= len(bodies) {
			t.Errorf("%s: unexpected request %d", method, i+1)
			testutil.ReplyServerError(w, 500, "unexpected request")
			return
		}
		api96ReplyRaw(w, bodies[i])
	})
	return server
}

// b32Client has retries enabled, so a repeat of a B-32 call would show in the request count.
func b32Client(t *testing.T, server *testutil.MockTelegramServer, opts ...sender.Option) *sender.Client {
	t.Helper()
	return testutil.NewTestClient(t, server.BaseURL(), append([]sender.Option{sender.WithRetries(2)}, opts...)...)
}

// b32Once checks that exactly one request reached method.
func b32Once(t *testing.T, server *testutil.MockTelegramServer, method string) {
	t.Helper()
	require.Equal(t, 1, server.CaptureCount())
	server.LastCapture().AssertPath(t, "/bot"+testutil.TestToken+"/"+method)
}

func b32Body(result string) string { return `{"ok":true,"result":` + result + `}` }

const b32Absent = `{"ok":true}`

// b32AssertNewErr checks the exact text of a new error and that it is neither an API nor a validation error.
func b32AssertNewErr(t *testing.T, err error, want string) {
	t.Helper()
	require.EqualError(t, err, want)
	var apiErr *tg.APIError
	assert.False(t, errors.As(err, &apiErr), "a *tg.APIError: %v", err)
	var valErr *tg.ValidationError
	assert.False(t, errors.As(err, &valErr), "a *tg.ValidationError: %v", err)
	assert.Nil(t, errors.Unwrap(err), "the new error wraps nothing: %v", err)
	assert.Equal(t, errors.New(want), err, "a plain error (fmt.Errorf with no %%w) with this text")
}

// b32API is one API refusal class: its reply body and what the caller must get back.
type b32API struct {
	body        string
	code        int
	description string
	retryAfter  time.Duration
	sentinel    error // nil: no mapped sentinel
}

// b32APIClasses: a 403 that tg.DetectSentinel maps to tg.ErrBotBlocked, a 429 with retry_after, a 502 (no sentinel).
var b32APIClasses = map[string]b32API{
	"api_error": {`{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked by the user"}`,
		403, "Forbidden: bot was blocked by the user", 0, tg.ErrBotBlocked},
	"api_429": {`{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 7","parameters":{"retry_after":7}}`,
		429, "Too Many Requests: retry after 7", 7 * time.Second, tg.ErrTooManyRequests},
	"api_502": {`{"ok":false,"error_code":502,"description":"Bad Gateway"}`, 502, "Bad Gateway", 0, nil},
}

// b32AssertAPIErr checks that an API refusal still reaches the caller as a *tg.APIError with every field the sender
// fills, the same text and the mapped sentinel.
func b32AssertAPIErr(t *testing.T, err error, method string, c b32API) {
	t.Helper()
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr), "want a *tg.APIError: %v", err)
	assert.Equal(t, c.code, apiErr.Code)
	assert.Equal(t, c.description, apiErr.Description)
	assert.Equal(t, method, apiErr.Method)
	assert.Equal(t, c.retryAfter, apiErr.RetryAfter)
	assert.EqualError(t, err, tg.NewAPIErrorWithRetry(method, c.code, c.description, c.retryAfter).Error())
	if c.sentinel != nil {
		assert.True(t, errors.Is(err, c.sentinel), "want %v: %v", c.sentinel, err)
	}
}

// b32AssertParseErr checks the unchanged decode error: an absent result keeps its full text and wraps a
// *json.SyntaxError; a wrong type keeps the method prefix and wraps a *json.UnmarshalTypeError (with its Field when
// field is not ""; Go's own type-error wording is not pinned).
func b32AssertParseErr(t *testing.T, err error, method string, wrongType bool, field string) {
	t.Helper()
	require.Error(t, err)
	prefix := "galigo: " + method + ": failed to parse response: "
	if !wrongType {
		assert.EqualError(t, err, prefix+"unexpected end of JSON input")
		var syn *json.SyntaxError
		assert.True(t, errors.As(err, &syn), "want a *json.SyntaxError: %v", err)
		return
	}
	assert.True(t, strings.HasPrefix(err.Error(), prefix), "want prefix %q, got %q", prefix, err.Error())
	var typ *json.UnmarshalTypeError
	require.True(t, errors.As(err, &typ), "want a *json.UnmarshalTypeError: %v", err)
	if field != "" {
		assert.Equal(t, field, typ.Field)
	}
}

func b32TokenCall(method string) func(*sender.Client) (tg.SecretToken, error) {
	if method == "getManagedBotToken" {
		return func(c *sender.Client) (tg.SecretToken, error) { return c.GetManagedBotToken(context.Background(), 42) }
	}
	return func(c *sender.Client) (tg.SecretToken, error) {
		return c.ReplaceManagedBotToken(context.Background(), 42)
	}
}

func b32Button(c *sender.Client) (*tg.PreparedKeyboardButton, error) {
	return c.SavePreparedKeyboardButton(context.Background(), sender.SavePreparedKeyboardButtonRequest{
		UserID: 42, Button: tg.KeyboardButton{Text: "x", RequestUsers: &tg.KeyboardButtonRequestUsers{RequestID: 1}},
	})
}

func b32Guest(c *sender.Client) (*tg.SentGuestMessage, error) {
	return c.AnswerGuestQuery(context.Background(), sender.AnswerGuestQueryRequest{GuestQueryID: "gq-b32", Result: api100Article()})
}

func b32Access(c *sender.Client) (*tg.BotAccessSettings, error) {
	return c.GetManagedBotAccessSettings(context.Background(), 42)
}

func TestB32_Token(t *testing.T) {
	for _, method := range []string{"getManagedBotToken", "replaceManagedBotToken"} {
		call := b32TokenCall(method)
		empty := "galigo: " + method + ": empty token in response"
		rows := []struct {
			name, body string
			want       string // the value of a row with no error
			newErr     bool   // the new empty-token error
			parse      int    // 0 none, 1 absent (syntax), 2 wrong type, 3 API error
		}{
			{name: "null", body: b32Body(`null`), newErr: true},
			{name: "empty", body: b32Body(`""`), newErr: true},
			{name: "space", body: b32Body(`" "`), want: " "},
			{name: "real", body: b32Body(`"` + api96FakeToken + `"`), want: api96FakeToken},
			{name: "absent", body: b32Absent, parse: 1},
			{name: "wrong_type", body: b32Body(`{}`), parse: 2},
			{name: "api_error", parse: 3},
			{name: "api_429", parse: 3},
			{name: "api_502", parse: 3},
		}
		t.Run(method, func(t *testing.T) {
			for _, r := range rows {
				t.Run(r.name, func(t *testing.T) {
					body := r.body
					if r.parse == 3 {
						body = b32APIClasses[r.name].body
					}
					server := b32Server(t, method, body)
					tok, err := call(b32Client(t, server))
					switch {
					case r.newErr:
						b32AssertNewErr(t, err, empty)
						assert.Equal(t, tg.SecretToken(""), tok)
					case r.parse == 3:
						b32AssertAPIErr(t, err, method, b32APIClasses[r.name])
						assert.Equal(t, tg.SecretToken(""), tok)
					case r.parse != 0:
						b32AssertParseErr(t, err, method, r.parse == 2, "")
						assert.Equal(t, tg.SecretToken(""), tok)
					default:
						require.NoError(t, err)
						assert.Equal(t, r.want, tok.Value())
					}
					b32Once(t, server, method)
				})
			}
		})
	}
}

func TestB32_PreparedButton(t *testing.T) {
	const empty = "galigo: savePreparedKeyboardButton: empty id in response"
	rows := []struct {
		name, body string
		want       *tg.PreparedKeyboardButton // non-nil: no error and this value
		parse      int                        // 0 the new error, 1 absent (syntax), 2 wrong type, 3 API error
		field      string                     // the UnmarshalTypeError Field of a wrong-type row
	}{
		{name: "null", body: b32Body(`null`)},
		{name: "empty_object", body: b32Body(`{}`)},
		{name: "empty_id", body: b32Body(`{"id":""}`)},
		{name: "null_id", body: b32Body(`{"id":null}`)},
		{name: "space", body: b32Body(`{"id":" "}`), want: &tg.PreparedKeyboardButton{ID: " "}},
		{name: "real", body: b32Body(`{"id":"Prep-B32"}`), want: &tg.PreparedKeyboardButton{ID: "Prep-B32"}},
		{name: "absent", body: b32Absent, parse: 1},
		{name: "wrong_type", body: b32Body(`"x"`), parse: 2},
		{name: "field_type", body: b32Body(`{"id":1}`), parse: 2, field: "id"},
		{name: "api_error", parse: 3},
		{name: "api_429", parse: 3},
		{name: "api_502", parse: 3},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			body := r.body
			if r.parse == 3 {
				body = b32APIClasses[r.name].body
			}
			server := b32Server(t, "savePreparedKeyboardButton", body)
			got, err := b32Button(b32Client(t, server))
			switch {
			case r.want != nil:
				require.NoError(t, err)
				assert.Equal(t, r.want, got)
			case r.parse == 3:
				b32AssertAPIErr(t, err, "savePreparedKeyboardButton", b32APIClasses[r.name])
				assert.Nil(t, got)
			case r.parse != 0:
				b32AssertParseErr(t, err, "savePreparedKeyboardButton", r.parse == 2, r.field)
				assert.Nil(t, got)
			default:
				b32AssertNewErr(t, err, empty)
				assert.Nil(t, got)
			}
			b32Once(t, server, "savePreparedKeyboardButton")
		})
	}
}

func TestB32_GuestQuery(t *testing.T) {
	const empty = "galigo: answerGuestQuery: empty inline_message_id in response"
	rows := []struct {
		name, body string
		want       *tg.SentGuestMessage // non-nil: no error and this value
		parse      int                  // 0 the new error, 1 absent (syntax), 2 wrong type, 3 API error
		field      string               // the UnmarshalTypeError Field of a wrong-type row
	}{
		{name: "null", body: b32Body(`null`)},
		{name: "empty_object", body: b32Body(`{}`)},
		{name: "empty_id", body: b32Body(`{"inline_message_id":""}`)},
		{name: "null_id", body: b32Body(`{"inline_message_id":null}`)},
		{name: "space", body: b32Body(`{"inline_message_id":" "}`), want: &tg.SentGuestMessage{InlineMessageID: " "}},
		{name: "real", body: b32Body(`{"inline_message_id":"Im-B32"}`), want: &tg.SentGuestMessage{InlineMessageID: "Im-B32"}},
		{name: "absent", body: b32Absent, parse: 1},
		{name: "wrong_type", body: b32Body(`"x"`), parse: 2},
		{name: "field_type", body: b32Body(`{"inline_message_id":1}`), parse: 2, field: "inline_message_id"},
		{name: "api_error", parse: 3},
		{name: "api_429", parse: 3},
		{name: "api_502", parse: 3},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			body := r.body
			if r.parse == 3 {
				body = b32APIClasses[r.name].body
			}
			server := b32Server(t, "answerGuestQuery", body)
			got, err := b32Guest(b32Client(t, server))
			switch {
			case r.want != nil:
				require.NoError(t, err)
				assert.Equal(t, r.want, got)
			case r.parse == 3:
				b32AssertAPIErr(t, err, "answerGuestQuery", b32APIClasses[r.name])
				assert.Nil(t, got)
			case r.parse != 0:
				b32AssertParseErr(t, err, "answerGuestQuery", r.parse == 2, r.field)
				assert.Nil(t, got)
			default:
				b32AssertNewErr(t, err, empty)
				assert.Nil(t, got)
			}
			b32Once(t, server, "answerGuestQuery")
		})
	}
}

func TestB32_AccessSettings(t *testing.T) {
	rows := []struct {
		name, body string
		want       *tg.BotAccessSettings // non-nil: no error and this value
		parse      int                   // 0 the null error, 1 absent (syntax), 2 wrong type, 3 API error
		field      string                // the UnmarshalTypeError Field of a wrong-type row
	}{
		{name: "null", body: b32Body(`null`)},
		{name: "empty_object", body: b32Body(`{}`), want: &tg.BotAccessSettings{}},
		{name: "false", body: b32Body(`{"is_access_restricted":false}`), want: &tg.BotAccessSettings{}},
		{name: "null_field", body: b32Body(`{"is_access_restricted":null}`), want: &tg.BotAccessSettings{}},
		{name: "real", body: b32Body(`{"is_access_restricted":true,"added_users":[{"id":7,"is_bot":false,"first_name":"Ann"}]}`),
			want: &tg.BotAccessSettings{IsAccessRestricted: true, AddedUsers: []tg.User{{ID: 7, FirstName: "Ann"}}}},
		{name: "absent", body: b32Absent, parse: 1},
		{name: "field_type", body: b32Body(`{"is_access_restricted":"x"}`), parse: 2, field: "is_access_restricted"},
		{name: "users_type", body: b32Body(`{"added_users":"x"}`), parse: 2, field: "added_users"},
		{name: "api_error", parse: 3},
		{name: "api_429", parse: 3},
		{name: "api_502", parse: 3},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			body := r.body
			if r.parse == 3 {
				body = b32APIClasses[r.name].body
			}
			server := b32Server(t, "getManagedBotAccessSettings", body)
			got, err := b32Access(b32Client(t, server))
			switch {
			case r.want != nil:
				require.NoError(t, err)
				assert.Equal(t, r.want, got)
			case r.parse == 3:
				b32AssertAPIErr(t, err, "getManagedBotAccessSettings", b32APIClasses[r.name])
				assert.Nil(t, got)
			case r.parse != 0:
				b32AssertParseErr(t, err, "getManagedBotAccessSettings", r.parse == 2, r.field)
				assert.Nil(t, got)
			default:
				b32AssertNewErr(t, err, "galigo: getManagedBotAccessSettings: null result in response")
				assert.Nil(t, got)
			}
			b32Once(t, server, "getManagedBotAccessSettings")
		})
	}
}

// TestB32_OtherNullUnchanged: a callJSON method outside B-32 keeps its old outcome for a null result (no shared
// check in callJSON; such methods are backlog item B-96).
func TestB32_OtherNullUnchanged(t *testing.T) {
	t.Run("answerWebAppQuery", func(t *testing.T) {
		server := b32Server(t, "answerWebAppQuery", b32Body(`null`))
		got, err := b32Client(t, server).AnswerWebAppQuery(context.Background(), sender.AnswerWebAppQueryRequest{
			WebAppQueryID: "wq-b32", Result: api100Article()})
		require.NoError(t, err)
		assert.Equal(t, &tg.SentWebAppMessage{}, got)
		b32Once(t, server, "answerWebAppQuery")
	})
}

// b32CountingBreaker returns breaker settings whose ReadyToTrip counts its calls and keeps the default decision.
// gobreaker v2.4.0 calls ReadyToTrip only on a failure, so the count is the number of breaker failures.
func b32CountingBreaker(calls *atomic.Int32) sender.Option {
	s := sender.DefaultCircuitBreakerSettings()
	def := s.ReadyToTrip
	s.ReadyToTrip = func(c gobreaker.Counts) bool {
		calls.Add(1)
		return def(c)
	}
	return sender.WithCircuitBreakerSettings(s)
}

func TestB32_NoBreakerTrip(t *testing.T) {
	t.Run("control_5xx_trips", func(t *testing.T) {
		server := testutil.NewMockServer(t)
		server.On("/bot"+testutil.TestToken+"/getManagedBotToken", func(w http.ResponseWriter, _ *http.Request) {
			testutil.ReplyServerError(w, 502, "Bad Gateway")
		})
		var trips atomic.Int32
		client := b32Client(t, server, b32CountingBreaker(&trips))
		for i := 0; i < 3; i++ {
			_, err := client.GetManagedBotToken(context.Background(), 42)
			var apiErr *tg.APIError
			require.True(t, errors.As(err, &apiErr), "call %d: %v", i+1, err)
			assert.Equal(t, 502, apiErr.Code, "call %d", i+1)
			assert.False(t, errors.Is(err, gobreaker.ErrOpenState), "call %d: %v", i+1, err)
		}
		assert.Equal(t, int32(3), trips.Load(), "one ReadyToTrip call per breaker failure")
		_, err := client.GetManagedBotToken(context.Background(), 42)
		assert.True(t, errors.Is(err, gobreaker.ErrOpenState), "call 4: %v", err)
		require.Equal(t, 3, server.CaptureCount(), "the breaker, not another gate, refused call 4")
	})

	type step struct {
		body, want string // want "": no error
	}
	tokenSteps := func(method string) []step {
		e := "galigo: " + method + ": empty token in response"
		return []step{{b32Body(`null`), e}, {b32Body(`""`), e}, {b32Body(`"` + api96FakeToken + `"`), ""}}
	}
	const btn = "galigo: savePreparedKeyboardButton: empty id in response"
	const gst = "galigo: answerGuestQuery: empty inline_message_id in response"
	leaves := []struct {
		method string
		steps  []step
		call   func(*sender.Client) error
	}{
		{"getManagedBotToken", tokenSteps("getManagedBotToken"), func(c *sender.Client) error {
			_, err := b32TokenCall("getManagedBotToken")(c)
			return err
		}},
		{"replaceManagedBotToken", tokenSteps("replaceManagedBotToken"), func(c *sender.Client) error {
			_, err := b32TokenCall("replaceManagedBotToken")(c)
			return err
		}},
		{"savePreparedKeyboardButton", []step{{b32Body(`null`), btn}, {b32Body(`{}`), btn}, {b32Body(`{"id":""}`), btn},
			{b32Body(`{"id":null}`), btn}, {b32Body(`{"id":"Prep-B32"}`), ""}}, func(c *sender.Client) error {
			_, err := b32Button(c)
			return err
		}},
		{"answerGuestQuery", []step{{b32Body(`null`), gst}, {b32Body(`{}`), gst}, {b32Body(`{"inline_message_id":""}`), gst},
			{b32Body(`{"inline_message_id":null}`), gst}, {b32Body(`{"inline_message_id":"Im-B32"}`), ""}}, func(c *sender.Client) error {
			_, err := b32Guest(c)
			return err
		}},
		{"getManagedBotAccessSettings", []step{{b32Body(`null`), "galigo: getManagedBotAccessSettings: null result in response"},
			{b32Body(`{"is_access_restricted":true}`), ""}}, func(c *sender.Client) error {
			_, err := b32Access(c)
			return err
		}},
	}
	for _, l := range leaves {
		t.Run(l.method, func(t *testing.T) {
			bodies := make([]string, len(l.steps))
			for i, s := range l.steps {
				bodies[i] = s.body
			}
			server := b32Server(t, l.method, bodies...)
			var trips atomic.Int32
			client := b32Client(t, server, b32CountingBreaker(&trips))
			for i, s := range l.steps {
				err := l.call(client)
				if s.want == "" {
					require.NoError(t, err, "step %d", i+1)
				} else {
					require.EqualError(t, err, s.want, "step %d", i+1)
				}
			}
			assert.Equal(t, int32(0), trips.Load(), "breaker failures")
			require.Equal(t, len(l.steps), server.CaptureCount())
		})
	}
}
