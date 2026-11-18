package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// Fake tokens. b71Token has the piece "FakeB71Token"; clean71Token has no 4-byte window in the texts the rows use.
// over71Token shares 4-byte windows with galigo's label "getUpdates" and the marker "[REDACTED]": only the server's
// text may be scrubbed, and only once.
const (
	b71Token     = "123456789:AAFakeB71TokenForUnitUse00000000"
	clean71Token = "987654321:AAxyzTokenNoWindows0000000000000"
	over71Token  = "123456789:AAgetUpdatesREDACTED0000000000"

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
)

// b71RT answers every request in memory with one canned response; it never dials.
type b71RT struct {
	status  int
	body    string
	readErr error

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

func TestAdapterGetUpdatesServerTextScrubbed(t *testing.T) {
	rows := []struct {
		name    string
		token   string
		status  int
		body    string
		readErr error
		want    string
		cause   string   // the exact text of the wrapped error; "" means none
		typed   bool     // a number decode: errors.As still finds the original *json.UnmarshalTypeError
		alt     string   // the other Go version's whole text (go.mod minimum and toolchain); the cause follows it
		chain   []string // the types from the original error down, below the scrubbed one
	}{
		{name: "read whole", token: b71Token, status: 200, body: `{"ok":`, readErr: errB71ReadWhole,
			want: "failed to decode getUpdates response: malformed trailer line: /bot[REDACTED]", cause: "malformed trailer line: /bot[REDACTED]", chain: []string{"*errors.errorString"}},
		{name: "read piece", token: b71Token, status: 200, body: `{"ok":`, readErr: errB71ReadPiece,
			want: "failed to decode getUpdates response: malformed trailer line: key [REDACTED]", cause: "malformed trailer line: key [REDACTED]", chain: []string{"*errors.errorString"}},
		{name: "decode piece", token: b71Token, status: 200, body: b71Decode,
			want:  "failed to decode getUpdates response: json: cannot unmarshal number [REDACTED]123 into Go struct field .error_code of type int",
			cause: "json: cannot unmarshal number [REDACTED]123 into Go struct field .error_code of type int", typed: true, chain: []string{"*json.UnmarshalTypeError"}},
		// the status does not choose the path: an undecodable reply at an error status is scrubbed the same way
		{name: "decode piece, status 400", token: b71Token, status: 400, body: b71Decode,
			want:  "failed to decode getUpdates response: json: cannot unmarshal number [REDACTED]123 into Go struct field .error_code of type int",
			cause: "json: cannot unmarshal number [REDACTED]123 into Go struct field .error_code of type int", typed: true, chain: []string{"*json.UnmarshalTypeError"}},
		{name: "decode whole (nested chat member)", token: b71Token, status: 200, body: b71Nested,
			want:  "failed to decode getUpdates response: old_chat_member: unknown chat member status: \"/bot[REDACTED]/getUpdates\"",
			cause: "old_chat_member: unknown chat member status: \"/bot[REDACTED]/getUpdates\"", chain: []string{"*fmt.wrapError", "*errors.errorString"}},
		{name: "decode piece (nested chat member, valid status)", token: b71Token, status: 200, body: b71NestedNum,
			want:  "failed to decode getUpdates response: old_chat_member: failed to unmarshal chat member (member): json: cannot unmarshal number [REDACTED]123 into Go struct field ChatMemberMember.user.id of type int64",
			alt:   "failed to decode getUpdates response: old_chat_member: failed to unmarshal chat member (member): json: cannot unmarshal number [REDACTED]123 into Go struct field User.chatMemberBase.user.id of type int64",
			cause: "old_chat_member: failed to unmarshal chat member (member): json: cannot unmarshal number [REDACTED]123 into Go struct field ChatMemberMember.user.id of type int64", typed: true, chain: []string{"*fmt.wrapError", "*fmt.wrapError", "*json.UnmarshalTypeError"}},
		{name: "decode piece, one pass", token: over71Token, status: 200, body: b71Decode,
			want:  "failed to decode getUpdates response: json: cannot unmarshal number [REDACTED]123 into Go struct field .error_code of type int",
			cause: "json: cannot unmarshal number [REDACTED]123 into Go struct field .error_code of type int", typed: true, chain: []string{"*json.UnmarshalTypeError"}},
		{name: "description whole", token: b71Token, status: 400, body: b71DescWhole,
			want: "getUpdates failed: 400 Bad Request: echo /bot[REDACTED]/getMe"},
		// and a description sent with status 200 is scrubbed the same way
		{name: "description whole, status 200", token: b71Token, status: 200, body: b71DescWhole,
			want: "getUpdates failed: 400 Bad Request: echo /bot[REDACTED]/getMe"},
		{name: "description piece", token: b71Token, status: 400, body: b71DescPiece,
			want: "getUpdates failed: 400 Bad Request: echo key [REDACTED]"},
		{name: "description, one pass", token: over71Token, status: 400, body: b71DescOver,
			want: "getUpdates failed: 400 Bad Request: echo [REDACTED]"},
		{name: "clean", token: clean71Token, status: 400, body: b71Clean,
			want: "getUpdates failed: 400 Bad Request: chat not found"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			rt := &b71RT{status: r.status, body: r.body, readErr: r.readErr}
			a := NewSenderAdapter(nil).WithToken(tg.SecretToken(r.token)).WithHTTPClient(&http.Client{Transport: rt})
			_, err := a.GetUpdates(context.Background(), 0, 1, 0)
			require.Error(t, err)
			cause := r.cause
			if r.alt != "" && err.Error() == r.alt { // the same label, the other Go wording
				cause = strings.TrimPrefix(r.alt, strings.TrimSuffix(r.want, r.cause))
			} else {
				require.EqualError(t, err, r.want)
			}
			require.Equal(t, []string{"/bot" + r.token + "/getUpdates"}, rt.paths())
			if r.readErr != nil {
				assert.ErrorIs(t, err, r.readErr)
			}
			assertCause(t, err, cause) // the cause one level down is the scrubbed one
			if r.typed {
				var te *json.UnmarshalTypeError
				assert.ErrorAs(t, err, &te)
			}
			if cause != "" { // below it is the original itself, with no layer in between
				orig := errors.Unwrap(errors.Unwrap(err))
				var chain []string
				for e := orig; e != nil; e = errors.Unwrap(e) {
					chain = append(chain, fmt.Sprintf("%T", e))
				}
				assert.Equal(t, r.chain, chain)
				switch {
				case r.readErr != nil:
					assert.True(t, orig == r.readErr, "want the read error, got %T", orig) //nolint:errorlint // identity is the point: the read error itself, no wrapper
				case r.typed:
					assert.True(t, orig != nil && strings.Contains(orig.Error(), "123456789123456789123"), "want the original, got %T", orig)
				default:
					assert.True(t, orig != nil && strings.Contains(orig.Error(), "FakeB71Token"), "want the original, got %T", orig)
				}
			}
		})
	}
}

// A successfully decoded update is returned as sent, even when a value holds the token (AC6).
func TestAdapterGetUpdatesSuccessfulValueReturnedAsSent(t *testing.T) {
	rt := &b71RT{status: 200, body: `{"ok":true,"result":[{"update_id":1,"message":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"text":"` + b71Token + `"}}]}`}
	a := NewSenderAdapter(nil).WithToken(tg.SecretToken(b71Token)).WithHTTPClient(&http.Client{Transport: rt})
	updates, err := a.GetUpdates(context.Background(), 0, 1, 0)
	require.NoError(t, err)
	require.Len(t, updates, 1)
	require.NotNil(t, updates[0].Message)
	assert.Equal(t, b71Token, updates[0].Message.Text)
	require.Equal(t, []string{"/bot" + b71Token + "/getUpdates"}, rt.paths())
}

// assertCause checks the exact text of the error that err directly wraps; cause "" means that there is none.
func assertCause(t *testing.T, err error, cause string) {
	t.Helper()
	u := errors.Unwrap(err)
	if cause == "" {
		assert.Nil(t, u)
		return
	}
	require.NotNil(t, u)
	assert.Equal(t, cause, u.Error())
}
