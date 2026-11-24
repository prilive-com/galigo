package receiver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// Fake tokens. b71Token has the piece "FakeB71Token"; clean71Token has no 4-byte window in the texts the rows use.
// over71Token shares 4-byte windows with galigo's label "telegram" and the marker "[REDACTED]": only the server's
// text may be scrubbed, and only once.
const (
	b71Token     = "123456789:AAFakeB71TokenForUnitUse00000000"
	clean71Token = "987654321:AAxyzTokenNoWindows0000000000000"
	over71Token  = "123456789:AAtelegramREDACTED000000000000"
	// status71Token shares a 4-byte window with "unexpected status code": that text is galigo's own, not the
	// server's, and is never scrubbed.
	status71Token = "123456789:AAstatusQ0000000000000000000"

	b71DescWhole = `{"ok":false,"error_code":400,"description":"Bad Request: echo /bot` + b71Token + `/getMe"}`
	b71DescPiece = `{"ok":false,"error_code":400,"description":"Bad Request: echo key FakeB71Token"}`
	b71Decode    = `{"ok":false,"error_code":123456789123456789123,"description":"x"}`
	b71Clean     = `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`
	// b71Nested puts the whole token in a nested chat member status, which tg.UnmarshalChatMember quotes.
	b71Nested = `{"ok":true,"result":[{"update_id":1,"my_chat_member":{"chat":{"id":1,"type":"private"},"from":{"id":1,"is_bot":false,"first_name":"a"},"date":1,"old_chat_member":{"status":"/bot` + b71Token + `/getUpdates"},"new_chat_member":{"status":"member"}}}]}`
	// b71NestedNum: the same with a valid status and an out-of-range user id: the number decode error inside
	// tg.UnmarshalChatMember.
	b71NestedNum = `{"ok":true,"result":[{"update_id":1,"my_chat_member":{"chat":{"id":1,"type":"private"},"from":{"id":1,"is_bot":false,"first_name":"a"},"date":1,"old_chat_member":{"status":"member","user":{"id":123456789123456789123,"is_bot":false,"first_name":"a"}},"new_chat_member":{"status":"member"}}}]}`
	b71DescOver  = `{"ok":false,"error_code":400,"description":"Bad Request: echo REDACTED0000"}`
)

var (
	errB71ReadWhole = errors.New("malformed trailer line: /bot" + b71Token)
	errB71ReadPiece = errors.New("malformed trailer line: key FakeB71Token")
	errB71Dial      = errors.New("dial refused: /bot" + over71Token)
)

// b71ThenNoID adds a later element with no usable update_id (B-135), so a nested decode error still fails the
// whole batch instead of being dropped.
func b71ThenNoID(body string) string {
	return strings.TrimSuffix(body, "]}") + `,{"message":{"text":7}}]}`
}

// b71RT answers every request in memory with one canned response, or with rtErr; it never dials.
type b71RT struct {
	status  int
	body    string
	readErr error
	rtErr   error

	mu   sync.Mutex
	seen []string
}

func (rt *b71RT) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.seen = append(rt.seen, req.URL.Path)
	rt.mu.Unlock()
	if req.Body != nil { // a RoundTripper must close the request body
		_, _ = io.Copy(io.Discard, req.Body)
		_ = req.Body.Close()
	}
	if rt.rtErr != nil {
		return nil, rt.rtErr
	}
	var body io.Reader = strings.NewReader(rt.body)
	if rt.readErr != nil {
		body = io.MultiReader(body, iotest.ErrReader(rt.readErr))
	}
	return &http.Response{
		StatusCode: rt.status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(body),
		Request:    req,
	}, nil
}

func (rt *b71RT) paths() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]string(nil), rt.seen...)
}

func TestFetchUpdatesServerTextScrubbed(t *testing.T) {
	rows := []struct {
		name    string
		token   string
		rt      *b71RT
		want    string
		readErr error
		code    int
		desc    string   // the exact APIError.Description
		cause   string   // the exact text of APIError.Err; "" means none
		typed   bool     // a number decode: errors.As still finds the original *json.UnmarshalTypeError
		alt     string   // the other Go version's whole text (go.mod minimum and toolchain); the cause follows it
		chain   []string // the types from APIError.Err down, every link
	}{
		{name: "read whole", token: b71Token, rt: &b71RT{status: 200, body: `{"ok":`, readErr: errB71ReadWhole},
			want: "telegram API error 0: request failed: malformed trailer line: /bot[REDACTED]", readErr: errB71ReadWhole,
			desc: "request failed", cause: "malformed trailer line: /bot[REDACTED]", chain: []string{"*scrub.scrubbedError", "*errors.errorString"}},
		{name: "read piece", token: b71Token, rt: &b71RT{status: 200, body: `{"ok":`, readErr: errB71ReadPiece},
			want: "telegram API error 0: request failed: malformed trailer line: key [REDACTED]", readErr: errB71ReadPiece,
			desc: "request failed", cause: "malformed trailer line: key [REDACTED]", chain: []string{"*scrub.scrubbedError", "*errors.errorString"}},
		{name: "decode piece", token: b71Token, rt: &b71RT{status: 200, body: b71Decode},
			want: "telegram API error 0: failed to parse response: json: cannot unmarshal number [REDACTED]123 into Go struct field getUpdatesResponse.error_code of type int",
			desc: "failed to parse response", cause: "json: cannot unmarshal number [REDACTED]123 into Go struct field getUpdatesResponse.error_code of type int", typed: true, chain: []string{"*scrub.scrubbedError", "*json.UnmarshalTypeError"}},
		{name: "decode whole (nested chat member)", token: b71Token, rt: &b71RT{status: 200, body: b71ThenNoID(b71Nested)},
			want: "telegram API error 0: failed to parse response: old_chat_member: unknown chat member status: \"/bot[REDACTED]/getUpdates\"",
			desc: "failed to parse response", cause: "old_chat_member: unknown chat member status: \"/bot[REDACTED]/getUpdates\"", chain: []string{"*scrub.scrubbedError", "*fmt.wrapError", "*errors.errorString"}},
		{name: "decode piece (nested chat member, valid status)", token: b71Token, rt: &b71RT{status: 200, body: b71ThenNoID(b71NestedNum)},
			want:  "telegram API error 0: failed to parse response: old_chat_member: failed to unmarshal chat member (member): json: cannot unmarshal number [REDACTED]123 into Go struct field ChatMemberMember.user.id of type int64",
			alt:   "telegram API error 0: failed to parse response: old_chat_member: failed to unmarshal chat member (member): json: cannot unmarshal number [REDACTED]123 into Go struct field User.chatMemberBase.user.id of type int64",
			desc:  "failed to parse response",
			cause: "old_chat_member: failed to unmarshal chat member (member): json: cannot unmarshal number [REDACTED]123 into Go struct field ChatMemberMember.user.id of type int64", typed: true, chain: []string{"*scrub.scrubbedError", "*fmt.wrapError", "*fmt.wrapError", "*json.UnmarshalTypeError"}},
		{name: "decode piece, one pass", token: over71Token, rt: &b71RT{status: 200, body: b71Decode},
			want: "telegram API error 0: failed to parse response: json: cannot unmarshal number [REDACTED]123 into Go struct field getUpdatesResponse.error_code of type int",
			desc: "failed to parse response", cause: "json: cannot unmarshal number [REDACTED]123 into Go struct field getUpdatesResponse.error_code of type int", typed: true, chain: []string{"*scrub.scrubbedError", "*json.UnmarshalTypeError"}},
		// the read error comes before the status check: it is scrubbed at any status
		{name: "read whole, error status", token: b71Token, rt: &b71RT{status: 502, body: `{"ok":`, readErr: errB71ReadWhole},
			want: "telegram API error 0: request failed: malformed trailer line: /bot[REDACTED]", readErr: errB71ReadWhole,
			desc: "request failed", cause: "malformed trailer line: /bot[REDACTED]", chain: []string{"*scrub.scrubbedError", "*errors.errorString"}},
		// over71Token holds "telegram" and "REDACTED": only the transport error is scrubbed, and only once
		{name: "transport error, one pass", token: over71Token, rt: &b71RT{rtErr: errB71Dial},
			want: "telegram API error 0: request failed: Get \"http://b71.example.test/bot[REDACTED]/getUpdates?limit=100&offset=0&timeout=30\": dial refused: /bot[REDACTED]", readErr: errB71Dial,
			desc: "request failed", cause: "Get \"http://b71.example.test/bot[REDACTED]/getUpdates?limit=100&offset=0&timeout=30\": dial refused: /bot[REDACTED]", chain: []string{"*scrub.scrubbedError", "*url.Error", "*errors.errorString"}},
		// galigo's own status text is not scrubbed, even when it shares a window with the token
		{name: "error status", token: status71Token, rt: &b71RT{status: 502, body: `{"ok":true,"result":[]}`},
			want: "telegram API error 0: request failed: unexpected status code: 502",
			desc: "request failed", cause: "unexpected status code: 502", chain: []string{"*errors.errorString"}},
		{name: "description whole", token: b71Token, rt: &b71RT{status: 200, body: b71DescWhole},
			want: "telegram API error 400: Bad Request: echo /bot[REDACTED]/getMe", code: 400, desc: "Bad Request: echo /bot[REDACTED]/getMe"},
		{name: "description piece", token: b71Token, rt: &b71RT{status: 200, body: b71DescPiece},
			want: "telegram API error 400: Bad Request: echo key [REDACTED]", code: 400, desc: "Bad Request: echo key [REDACTED]"},
		{name: "description, one pass", token: over71Token, rt: &b71RT{status: 200, body: b71DescOver},
			want: "telegram API error 400: Bad Request: echo [REDACTED]", code: 400, desc: "Bad Request: echo [REDACTED]"},
		{name: "clean", token: clean71Token, rt: &b71RT{status: 200, body: b71Clean},
			want: "telegram API error 400: Bad Request: chat not found", code: 400, desc: "Bad Request: chat not found"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Mode = ModeLongPolling
			cfg.BaseURL = "http://b71.example.test/bot"
			c := NewPollingClient(tg.SecretToken(r.token), make(chan tg.Update, 1), nil, cfg,
				WithPollingHTTPClient(&http.Client{Transport: r.rt}))

			_, err := c.fetchUpdates(context.Background())
			require.Error(t, err)
			cause := r.cause
			if r.alt != "" && err.Error() == r.alt { // the same label, the other Go wording
				cause = strings.TrimPrefix(r.alt, strings.TrimSuffix(r.want, r.cause))
			} else {
				require.EqualError(t, err, r.want)
			}
			require.Equal(t, []string{"/bot" + r.token + "/getUpdates"}, r.rt.paths())
			if r.readErr != nil {
				assert.ErrorIs(t, err, r.readErr)
			}
			apiErr, ok := err.(*APIError) //nolint:errorlint // the exact type is the point: returned as it is, with no wrapper
			require.True(t, ok, "want *APIError, got %T", err)
			assert.Equal(t, r.code, apiErr.Code)
			assert.Equal(t, r.desc, apiErr.Description)
			if cause == "" {
				assert.Nil(t, apiErr.Err)
			} else { // the cause one level down is the scrubbed one
				require.NotNil(t, apiErr.Err)
				assert.Equal(t, cause, apiErr.Err.Error())
				var chain []string
				for e := apiErr.Err; e != nil; e = errors.Unwrap(e) {
					chain = append(chain, fmt.Sprintf("%T", e))
				}
				assert.Equal(t, r.chain, chain)
				// and below it is the original itself, with no layer in between
				orig := errors.Unwrap(apiErr.Err)
				switch {
				case len(r.chain) == 1: // galigo's own text: no scrubber wrapper, nothing below
				case r.rt.rtErr != nil:
					var ue *url.Error
					require.ErrorAs(t, apiErr.Err, &ue)
					assert.True(t, orig == ue, "want the transport error, got %T", orig) //nolint:errorlint // identity is the point: the client's error itself, no wrapper
				case r.readErr != nil:
					assert.True(t, orig == r.readErr, "want the read error, got %T", orig) //nolint:errorlint // identity is the point: the read error itself, no wrapper
				case r.typed:
					assert.True(t, orig != nil && strings.Contains(orig.Error(), "123456789123456789123"), "want the original, got %T", orig)
				default:
					assert.True(t, orig != nil && strings.Contains(orig.Error(), "FakeB71Token"), "want the original, got %T", orig)
				}
			}
			if r.typed {
				var te *json.UnmarshalTypeError
				assert.ErrorAs(t, err, &te)
			}
		})
	}
}

// A successfully decoded update is returned as sent, even when a value holds the token (AC6).
func TestFetchUpdatesSuccessfulValueReturnedAsSent(t *testing.T) {
	rt := &b71RT{status: 200, body: `{"ok":true,"result":[{"update_id":1,"message":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"text":"` + b71Token + `"}}]}`}
	cfg := DefaultConfig()
	cfg.Mode = ModeLongPolling
	cfg.BaseURL = "http://b71.example.test/bot"
	c := NewPollingClient(tg.SecretToken(b71Token), make(chan tg.Update, 1), nil, cfg,
		WithPollingHTTPClient(&http.Client{Transport: rt}))

	items, err := c.fetchUpdates(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NoError(t, items[0].err)
	require.NotNil(t, items[0].update.Message)
	assert.Equal(t, b71Token, items[0].update.Message.Text)
	require.Equal(t, []string{"/bot" + b71Token + "/getUpdates"}, rt.paths())
}
