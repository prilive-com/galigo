package sender_test

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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Fake tokens. b71Token has the piece "FakeB71Token"; chat71Token has the piece "chat", which is also in the
// description "Bad Request: chat not found"; clean71Token has no 4-byte window in the texts the rows use.
const (
	b71Token     = "123456789:AAFakeB71TokenForUnitUse00000000"
	chat71Token  = "123456789:AAchatQwz000000000000000000000"
	clean71Token = "987654321:AAxyzTokenNoWindows0000000000000"
	// over71Token shares 4-byte windows with galigo's label "getMe", the printed duration "1h0m0s" and the
	// marker "[REDACTED]": only the server's text may be scrubbed, and only once.
	over71Token = "123456789:AAgetMe1h0m0sREDACTED0000000000"
	// label71Token shares 4-byte windows with galigo's labels "failed to read response" and "failed to parse ..."
	// and with the marker "[REDACTED]": a scrub of a label, or a second pass, changes the text.
	label71Token = "123456789:AAparsereadREDACTED000000000000"
)

var (
	errB71ReadWhole = errors.New("malformed trailer line: /bot" + b71Token)
	errB71ReadPiece = errors.New("malformed trailer line: key FakeB71Token")
	errB71ReadLabel = errors.New("malformed trailer line: /bot" + label71Token)
)

// b71RT answers every request in memory with one canned response; it never dials.
type b71RT struct {
	status  int
	body    string
	readErr error
	header  http.Header // extra response headers

	mu   sync.Mutex
	seen []string
}

func (rt *b71RT) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.seen = append(rt.seen, req.URL.Path)
	rt.mu.Unlock()
	if req.Body != nil { // a RoundTripper must close the request body; draining it ends a multipart writer
		_, _ = io.Copy(io.Discard, req.Body)
		_ = req.Body.Close()
	}
	var body io.Reader = strings.NewReader(rt.body)
	if rt.readErr != nil {
		body = io.MultiReader(body, iotest.ErrReader(rt.readErr))
	}
	h := http.Header{"Content-Type": {"application/json"}}
	for k, v := range rt.header {
		h[k] = v
	}
	return &http.Response{
		StatusCode: rt.status,
		Header:     h,
		Body:       io.NopCloser(body),
		Request:    req,
	}, nil
}

func TestServerTextScrubbed(t *testing.T) {
	const descWhole = `{"ok":false,"error_code":400,"description":"Bad Request: echo /bot` + b71Token + `/getMe"}`
	const descPiece = `{"ok":false,"error_code":400,"description":"Bad Request: echo key FakeB71Token"}`
	rows := []struct {
		name     string
		token    string
		rt       *b71RT
		want     string
		readErr  error
		decode   bool
		code     int
		retry    time.Duration
		sentinel error
		desc     string
		cause    string // the exact text of the error the site wraps (read and decode rows)
	}{
		{name: "read whole", token: b71Token, rt: &b71RT{status: 200, body: `{"ok":`, readErr: errB71ReadWhole},
			want: "failed to read response: malformed trailer line: /bot[REDACTED]", readErr: errB71ReadWhole,
			cause: "malformed trailer line: /bot[REDACTED]"},
		{name: "read piece", token: b71Token, rt: &b71RT{status: 200, body: `{"ok":`, readErr: errB71ReadPiece},
			want: "failed to read response: malformed trailer line: key [REDACTED]", readErr: errB71ReadPiece,
			cause: "malformed trailer line: key [REDACTED]"},
		// the body is read before the status is looked at
		{name: "read whole, error status", token: b71Token, rt: &b71RT{status: 502, body: `{"ok":`, readErr: errB71ReadWhole},
			want: "failed to read response: malformed trailer line: /bot[REDACTED]", readErr: errB71ReadWhole,
			cause: "malformed trailer line: /bot[REDACTED]"},
		// label71Token holds "read" and "REDACTED": a scrub of the label or of the wrapped text would change it
		{name: "read whole, one pass", token: label71Token, rt: &b71RT{status: 200, body: `{"ok":`, readErr: errB71ReadLabel},
			want: "failed to read response: malformed trailer line: /bot[REDACTED]", readErr: errB71ReadLabel,
			cause: "malformed trailer line: /bot[REDACTED]"},
		{name: "decode piece", token: b71Token, rt: &b71RT{status: 400, body: `{"ok":false,"error_code":123456789123456789123,"description":"x"}`},
			want: "failed to parse response: json: cannot unmarshal number [REDACTED]123 into Go struct field apiResponse.error_code of type int", decode: true,
			cause: "json: cannot unmarshal number [REDACTED]123 into Go struct field apiResponse.error_code of type int"},
		// the envelope is decoded before the status is looked at: an undecodable reply at 200 is scrubbed the same way
		{name: "decode piece, status 200", token: b71Token, rt: &b71RT{status: 200, body: `{"ok":false,"error_code":123456789123456789123,"description":"x"}`},
			want: "failed to parse response: json: cannot unmarshal number [REDACTED]123 into Go struct field apiResponse.error_code of type int", decode: true,
			cause: "json: cannot unmarshal number [REDACTED]123 into Go struct field apiResponse.error_code of type int"},
		{name: "description whole", token: b71Token, rt: &b71RT{status: 400, body: descWhole},
			want: "galigo: getMe failed: Bad Request: echo /bot[REDACTED]/getMe (code=400)", code: 400, desc: "Bad Request: echo /bot[REDACTED]/getMe"},
		{name: "description piece", token: b71Token, rt: &b71RT{status: 400, body: descPiece},
			want: "galigo: getMe failed: Bad Request: echo key [REDACTED] (code=400)", code: 400, desc: "Bad Request: echo key [REDACTED]"},
		// the code does not look at the HTTP status before "ok": an ok:false reply at 200 is scrubbed the same way
		{name: "description whole, status 200", token: b71Token, rt: &b71RT{status: 200, body: descWhole},
			want: "galigo: getMe failed: Bad Request: echo /bot[REDACTED]/getMe (code=400)", code: 400, desc: "Bad Request: echo /bot[REDACTED]/getMe"},
		{name: "description whole, retry branch", token: b71Token,
			rt:   &b71RT{status: 429, body: `{"ok":false,"error_code":429,"description":"Too Many Requests: echo /bot` + b71Token + `/getMe","parameters":{"retry_after":7}}`},
			want: "galigo: getMe failed: Too Many Requests: echo /bot[REDACTED]/getMe (code=429, retry_after=7s)", code: 429, retry: 7 * time.Second, sentinel: tg.ErrTooManyRequests, desc: "Too Many Requests: echo /bot[REDACTED]/getMe"},
		// retry_after only in the HTTP header (the fallback source): the description is scrubbed the same way
		{name: "description whole, retry header only", token: b71Token,
			rt:   &b71RT{status: 429, body: `{"ok":false,"error_code":429,"description":"Too Many Requests: echo /bot` + b71Token + `/getMe"}`, header: http.Header{"Retry-After": {"7"}}},
			want: "galigo: getMe failed: Too Many Requests: echo /bot[REDACTED]/getMe (code=429, retry_after=7s)", code: 429, retry: 7 * time.Second, sentinel: tg.ErrTooManyRequests, desc: "Too Many Requests: echo /bot[REDACTED]/getMe"},
		{name: "sentinel from the original text", token: chat71Token, rt: &b71RT{status: 400, body: `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`},
			want: "galigo: getMe failed: Bad Request: [REDACTED] not found (code=400)", code: 400, sentinel: tg.ErrChatNotFound, desc: "Bad Request: [REDACTED] not found"},
		{name: "sentinel from the original text, retry branch", token: chat71Token,
			rt:   &b71RT{status: 400, body: `{"ok":false,"error_code":400,"description":"Bad Request: chat not found","parameters":{"retry_after":3}}`},
			want: "galigo: getMe failed: Bad Request: [REDACTED] not found (code=400, retry_after=3s)", code: 400, retry: 3 * time.Second, sentinel: tg.ErrChatNotFound, desc: "Bad Request: [REDACTED] not found"},
		{name: "clean", token: clean71Token, rt: &b71RT{status: 400, body: `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`},
			want: "galigo: getMe failed: Bad Request: chat not found (code=400)", code: 400, sentinel: tg.ErrChatNotFound, desc: "Bad Request: chat not found"},
		{name: "labels and numbers kept, one pass", token: over71Token,
			rt:   &b71RT{status: 429, body: `{"ok":false,"error_code":429,"description":"Too Many Requests: echo REDACTED0000","parameters":{"retry_after":3600}}`},
			want: "galigo: getMe failed: Too Many Requests: echo [REDACTED] (code=429, retry_after=1h0m0s)", code: 429, retry: time.Hour, sentinel: tg.ErrTooManyRequests, desc: "Too Many Requests: echo [REDACTED]"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			c, err := sender.New(r.token,
				sender.WithHTTPClient(&http.Client{Transport: r.rt}),
				sender.WithBaseURL("http://b71.example.test"),
				sender.WithRetries(0))
			require.NoError(t, err)
			t.Cleanup(func() { _ = c.Close() })

			_, err = c.GetMe(context.Background())
			require.EqualError(t, err, r.want)
			require.Equal(t, []string{"/bot" + r.token + "/getMe"}, r.rt.paths())
			if r.cause != "" { // the cause one level down is the scrubbed one
				assertCause(t, err, r.cause)
			}
			if r.readErr != nil {
				assert.ErrorIs(t, err, r.readErr)
				assertOrigin(t, errors.Unwrap(err), func(e error) bool { return e == r.readErr }) //nolint:errorlint // identity is the point
			}
			if r.decode {
				var te *json.UnmarshalTypeError
				assert.ErrorAs(t, err, &te)
				assertOrigin(t, errors.Unwrap(err), isTypeErr)
			}
			if r.code != 0 {
				apiErr, ok := err.(*tg.APIError) //nolint:errorlint // the exact type is the point: returned as it is, with no wrapper
				require.True(t, ok, "want *tg.APIError, got %T", err)
				// its cause is the sentinel detected on the original text, or nil
				assert.True(t, errors.Unwrap(err) == r.sentinel, "cause %v, want %v", errors.Unwrap(err), r.sentinel) //nolint:errorlint // the exact sentinel one link down is the point
				assert.Equal(t, r.code, apiErr.Code)
				assert.Equal(t, "getMe", apiErr.Method)
				assert.Equal(t, r.retry, apiErr.RetryAfter)
				assert.Equal(t, r.desc, apiErr.Description)
			}
			if r.sentinel != nil {
				assert.ErrorIs(t, err, r.sentinel)
			}
		})
	}
}

func (rt *b71RT) paths() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]string(nil), rt.seen...)
}

// b71Big holds the bot-id digits twice and overflows every Go integer type, so encoding/json quotes it.
const b71Big = "123456789123456789123"

// assertCause checks the exact text of the error that err directly wraps. Where a method returns the scrubbed
// error itself (bare), there is no wrapper: one unwrap already reaches the original error by design.
func assertCause(t *testing.T, err error, cause string) {
	t.Helper()
	u := errors.Unwrap(err)
	require.NotNil(t, u)
	assert.Equal(t, cause, u.Error())
}

// assertOrigin checks the link below a scrubbed error: the scrubber's wrapper unwraps to the original error
// itself, with no layer added in between. scrubbed is what a bare site returns, or what a wrapping
// site wraps (one unwrap down).
func assertOrigin(t *testing.T, scrubbed error, original func(error) bool) {
	t.Helper()
	require.NotNil(t, scrubbed)
	assert.True(t, original(errors.Unwrap(scrubbed)), "the scrubbed error must unwrap to the original, got %T", errors.Unwrap(scrubbed))
}

func isTypeErr(e error) bool { _, ok := e.(*json.UnmarshalTypeError); return ok } //nolint:errorlint // one link, not the chain: assertOrigin walks it

// The error chains below a scrubbed decode error, by type, from the original down (the same on both Go versions).
var (
	chainNum       = []string{"*json.UnmarshalTypeError"}                   // a number in a struct field
	chainStatus    = []string{"*errors.errorString"}                        // tg.UnmarshalChatMember: unknown status
	chainMemberNum = []string{"*fmt.wrapError", "*json.UnmarshalTypeError"} // tg.UnmarshalChatMember: a member field
)

// assertOriginChain checks the chain below a scrubbed error: the original holds the removed text raw, and it and
// every link under it have exactly the types in want, so no layer was added anywhere.
func assertOriginChain(t *testing.T, scrubbed error, raw string, want []string) {
	t.Helper()
	require.NotNil(t, scrubbed)
	orig := errors.Unwrap(scrubbed)
	require.NotNil(t, orig)
	assert.Contains(t, orig.Error(), raw)
	var got []string
	for e := orig; e != nil; e = errors.Unwrap(e) {
		got = append(got, fmt.Sprintf("%T", e))
	}
	assert.Equal(t, want, got)
}

// b71GoAlt holds the second whole text a row accepts: encoding/json names the field of a slice element
// differently in the Go versions this module supports (go.mod minimum and toolchain).
var b71GoAlt = map[string]string{
	"sendMediaGroup":  "json: cannot unmarshal number [REDACTED]123 into Go struct field Message.message_id of type int",
	"forwardMessages": "json: cannot unmarshal number [REDACTED]123 into Go struct field MessageID.message_id of type int",
	"copyMessages":    "json: cannot unmarshal number [REDACTED]123 into Go struct field MessageID.message_id of type int",
	"getChatAdministrators valid status, number": "galigo: getChatAdministrators: failed to unmarshal chat member (member): json: cannot unmarshal number [REDACTED]123 into Go struct field User.chatMemberBase.user.id of type int64",
	"getChatMember valid status, number":         "failed to unmarshal chat member (member): json: cannot unmarshal number [REDACTED]123 into Go struct field User.chatMemberBase.user.id of type int64",
	"getChatAdministrators number, one pass":     "galigo: getChatAdministrators: failed to unmarshal chat member (member): json: cannot unmarshal number [REDACTED]123 into Go struct field User.chatMemberBase.user.id of type int64",
	"getChatAdministrators second member":        "galigo: getChatAdministrators: failed to unmarshal chat member (member): json: cannot unmarshal number [REDACTED]123 into Go struct field User.chatMemberBase.user.id of type int64",
}

func TestResultDecodeScrubbed(t *testing.T) {
	const member = `{"status":"/bot` + b71Token + `/getChatMember"}`
	const memberPiece = `{"status":"key FakeB71Token"}`
	// a valid status with a bad number: the decode error comes from the member's own fields
	const memberNum = `{"status":"member","user":{"id":` + b71Big + `,"is_bot":false,"first_name":"a"}}`
	const goodMember = `{"status":"member","user":{"id":1,"is_bot":false,"first_name":"a"}}`
	const numUser = "failed to unmarshal chat member (member): json: cannot unmarshal number [REDACTED]123 into Go struct field ChatMemberMember.user.id of type int64"
	ctx := context.Background()
	chat := tg.ChatID(int64(1))
	// GetChatMember with a nil member check: on an error it must return no member at all
	getMember := func(c *sender.Client) error {
		m, err := c.GetChatMember(ctx, chat, 1)
		if m != nil {
			return fmt.Errorf("member %T returned with the error %v", m, err)
		}
		return err
	}
	num := func(field string) string {
		return "json: cannot unmarshal number [REDACTED]123 into Go struct field " + field
	}
	rows := []struct {
		name   string
		result string
		call   func(*sender.Client) error
		method string
		want   string
		cause  string   // the exact text of the wrapped error; "" where the method returns the scrubbed error itself
		typed  bool     // a number decode: errors.As still finds the original *json.UnmarshalTypeError
		chain  []string // the types from the original error down
		token  string   // b71Token when empty
	}{
		{"callJSON getChat", `{"id":` + b71Big + `}`, func(c *sender.Client) error { _, err := c.GetChat(ctx, chat); return err }, "getChat",
			"galigo: getChat: failed to parse response: " + num("ChatFullInfo.id of type int64"), num("ChatFullInfo.id of type int64"), true, chainNum, ""},
		{"copyMessage", `{"message_id":` + b71Big + `}`, func(c *sender.Client) error {
			_, err := c.CopyMessage(ctx, sender.CopyMessageRequest{ChatID: chat, FromChatID: chat, MessageID: 1})
			return err
		}, "copyMessage", "failed to parse result: " + num("MessageID.message_id of type int"), num("MessageID.message_id of type int"), true, chainNum, ""},
		{"parseMessage sendMessage", `{"message_id":` + b71Big + `}`, func(c *sender.Client) error {
			_, err := c.SendMessage(ctx, sender.SendMessageRequest{ChatID: chat, Text: "x"})
			return err
		}, "sendMessage", "failed to parse message: " + num("Message.message_id of type int"), num("Message.message_id of type int"), true, chainNum, ""},
		{"parseEditResult editMessageText", `{"message_id":` + b71Big + `}`, func(c *sender.Client) error {
			_, err := c.EditMessageText(ctx, sender.EditMessageTextRequest{ChatID: chat, MessageID: 1, Text: "x"})
			return err
		}, "editMessageText", "failed to parse message: " + num("Message.message_id of type int"), num("Message.message_id of type int"), true, chainNum, ""},
		{"getMe", `{"id":` + b71Big + `}`, func(c *sender.Client) error { _, err := c.GetMe(ctx); return err }, "getMe", num("User.id of type int64"), "", true, chainNum, ""},
		{"sendMediaGroup", `[{"message_id":` + b71Big + `}]`, func(c *sender.Client) error {
			_, err := c.SendMediaGroup(ctx, sender.SendMediaGroupRequest{ChatID: chat, Media: []sender.InputFile{sender.FromFileID("a").WithMediaType("photo"), sender.FromFileID("b").WithMediaType("photo")}})
			return err
		}, "sendMediaGroup", num(".0.message_id of type int"), "", true, chainNum, ""},
		{"getFile", `{"file_id":"f","file_size":` + b71Big + `}`, func(c *sender.Client) error { _, err := c.GetFile(ctx, "f"); return err }, "getFile", num("File.file_size of type int64"), "", true, chainNum, ""},
		{"getUserProfilePhotos", `{"total_count":` + b71Big + `}`, func(c *sender.Client) error { _, err := c.GetUserProfilePhotos(ctx, 1); return err }, "getUserProfilePhotos", num("UserProfilePhotos.total_count of type int"), "", true, chainNum, ""},
		{"getUserProfileAudios", `{"total_count":` + b71Big + `}`, func(c *sender.Client) error { _, err := c.GetUserProfileAudios(ctx, 1); return err }, "getUserProfileAudios", num("UserProfileAudios.total_count of type int"), "", true, chainNum, ""},
		{"forwardMessages", `[{"message_id":` + b71Big + `}]`, func(c *sender.Client) error {
			_, err := c.ForwardMessages(ctx, sender.ForwardMessagesRequest{ChatID: chat, FromChatID: chat, MessageIDs: []int{1}})
			return err
		}, "forwardMessages", num(".0.message_id of type int"), "", true, chainNum, ""},
		{"copyMessages", `[{"message_id":` + b71Big + `}]`, func(c *sender.Client) error {
			_, err := c.CopyMessages(ctx, sender.CopyMessagesRequest{ChatID: chat, FromChatID: chat, MessageIDs: []int{1}})
			return err
		}, "copyMessages", num(".0.message_id of type int"), "", true, chainNum, ""},
		{"getChatAdministrators whole", `[` + member + `]`, func(c *sender.Client) error { _, err := c.GetChatAdministrators(ctx, chat); return err }, "getChatAdministrators",
			"galigo: getChatAdministrators: unknown chat member status: \"/bot[REDACTED]/getChatMember\"", "unknown chat member status: \"/bot[REDACTED]/getChatMember\"", false, chainStatus, ""},
		{"getChatAdministrators piece", `[` + memberPiece + `]`, func(c *sender.Client) error { _, err := c.GetChatAdministrators(ctx, chat); return err }, "getChatAdministrators",
			"galigo: getChatAdministrators: unknown chat member status: \"key [REDACTED]\"", "unknown chat member status: \"key [REDACTED]\"", false, chainStatus, ""},
		{"getChatAdministrators valid status, number", `[` + memberNum + `]`, func(c *sender.Client) error { _, err := c.GetChatAdministrators(ctx, chat); return err }, "getChatAdministrators",
			"galigo: getChatAdministrators: " + numUser, numUser, true, chainMemberNum, ""},
		{"getChatMember whole", member, getMember, "getChatMember", "unknown chat member status: \"/bot[REDACTED]/getChatMember\"", "", false, chainStatus, ""},
		{"getChatMember piece", memberPiece, getMember, "getChatMember", "unknown chat member status: \"key [REDACTED]\"", "", false, chainStatus, ""},
		{"getChatMember valid status, number", memberNum, getMember, "getChatMember", numUser, "", true, chainMemberNum, ""},
		// one pass: label71Token holds "pars" and "REDACTED", so a scrub of the label or a second pass changes the text
		{"callJSON getChat, one pass", `{"id":` + b71Big + `}`, func(c *sender.Client) error { _, err := c.GetChat(ctx, chat); return err }, "getChat",
			"galigo: getChat: failed to parse response: " + num("ChatFullInfo.id of type int64"), num("ChatFullInfo.id of type int64"), true, chainNum, label71Token},
		{"copyMessage, one pass", `{"message_id":` + b71Big + `}`, func(c *sender.Client) error {
			_, err := c.CopyMessage(ctx, sender.CopyMessageRequest{ChatID: chat, FromChatID: chat, MessageID: 1})
			return err
		}, "copyMessage", "failed to parse result: " + num("MessageID.message_id of type int"), num("MessageID.message_id of type int"), true, chainNum, label71Token},
		{"parseMessage sendMessage, one pass", `{"message_id":` + b71Big + `}`, func(c *sender.Client) error {
			_, err := c.SendMessage(ctx, sender.SendMessageRequest{ChatID: chat, Text: "x"})
			return err
		}, "sendMessage", "failed to parse message: " + num("Message.message_id of type int"), num("Message.message_id of type int"), true, chainNum, label71Token},
		{"getChatAdministrators number, one pass", `[` + memberNum + `]`, func(c *sender.Client) error { _, err := c.GetChatAdministrators(ctx, chat); return err }, "getChatAdministrators",
			"galigo: getChatAdministrators: " + numUser, numUser, true, chainMemberNum, label71Token},
		// the bad member is not the first: every member's decode error is scrubbed
		{"getChatAdministrators second member", `[` + goodMember + `,` + memberNum + `]`, func(c *sender.Client) error { _, err := c.GetChatAdministrators(ctx, chat); return err }, "getChatAdministrators",
			"galigo: getChatAdministrators: " + numUser, numUser, true, chainMemberNum, ""},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			token := r.token
			if token == "" {
				token = b71Token
			}
			rt := &b71RT{status: 200, body: `{"ok":true,"result":` + r.result + `}`}
			c, err := sender.New(token,
				sender.WithHTTPClient(&http.Client{Transport: rt}),
				sender.WithBaseURL("http://b71.example.test"),
				sender.WithRetries(0))
			require.NoError(t, err)
			t.Cleanup(func() { _ = c.Close() })

			err = r.call(c)
			require.Error(t, err)
			cause := r.cause
			if alt, ok := b71GoAlt[r.name]; ok {
				require.Contains(t, []string{r.want, alt}, err.Error())
				if err.Error() == alt && cause != "" { // the same label, the other Go wording
					cause = strings.TrimPrefix(alt, strings.TrimSuffix(r.want, r.cause))
				}
			} else {
				require.EqualError(t, err, r.want)
			}
			require.Equal(t, []string{"/bot" + token + "/" + r.method}, rt.paths())
			if cause != "" {
				assertCause(t, err, cause)
			}
			if r.typed {
				var te *json.UnmarshalTypeError
				assert.ErrorAs(t, err, &te)
			}
			// below the scrubbed error is the original, whose text still holds what was removed
			scrubbed, raw := err, "FakeB71Token"
			if cause != "" {
				scrubbed = errors.Unwrap(err)
			}
			if r.typed {
				raw = b71Big
			}
			assertOriginChain(t, scrubbed, raw, r.chain)
		})
	}
}

// The list decode in getChatAdministrators cannot quote a value, but its fixed wording can hold a token piece:
// this token holds "string", the word the decoder uses for a JSON string result. It also holds "pars" (in the fixed
// label) and "REDACTED" (in the marker), so a scrub of the label or a second pass would change the text.
func TestChatAdministratorsListDecodeScrubbed(t *testing.T) {
	const string71Token = "123456789:AAstringparsREDACTED00000000"
	rt := &b71RT{status: 200, body: `{"ok":true,"result":"x"}`}
	c, err := sender.New(string71Token,
		sender.WithHTTPClient(&http.Client{Transport: rt}),
		sender.WithBaseURL("http://b71.example.test"),
		sender.WithRetries(0))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	_, err = c.GetChatAdministrators(context.Background(), tg.ChatID(int64(1)))
	require.Error(t, err)
	require.Equal(t, []string{"/bot" + string71Token + "/getChatAdministrators"}, rt.paths())
	u := errors.Unwrap(err)
	require.NotNil(t, u)
	// The type name differs by Go version (go.mod minimum and toolchain); the text and its cause must be one pair.
	const label = "galigo: getChatAdministrators: failed to parse response: "
	pairs := [][2]string{
		{label + "json: cannot unmarshal [REDACTED] into Go value of type []json.RawMessage", "json: cannot unmarshal [REDACTED] into Go value of type []json.RawMessage"},
		{label + "json: cannot unmarshal [REDACTED] into Go value of type []jsontext.Value", "json: cannot unmarshal [REDACTED] into Go value of type []jsontext.Value"},
	}
	require.Contains(t, pairs, [2]string{err.Error(), u.Error()})
	var typeErr *json.UnmarshalTypeError
	assert.ErrorAs(t, err, &typeErr)
	assertOrigin(t, u, isTypeErr)
}

// A successfully decoded result is returned as sent, even when a value holds the token: only errors are scrubbed.
func TestSuccessfulValueReturnedAsSent(t *testing.T) {
	ctx := context.Background()
	chat := tg.ChatID(int64(1))
	rows := []struct {
		name   string
		method string
		result string
		check  func(*testing.T, *sender.Client)
	}{
		{"getMe", "getMe", `{"id":1,"is_bot":true,"first_name":"` + b71Token + `"}`, func(t *testing.T, c *sender.Client) {
			user, err := c.GetMe(ctx)
			require.NoError(t, err)
			assert.Equal(t, b71Token, user.FirstName)
		}},
		{"callJSON getChat", "getChat", `{"id":1,"type":"group","title":"` + b71Token + `"}`, func(t *testing.T, c *sender.Client) {
			info, err := c.GetChat(ctx, chat)
			require.NoError(t, err)
			assert.Equal(t, b71Token, info.Title)
		}},
		{"parseMessage sendMessage", "sendMessage", `{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"text":"` + b71Token + `"}`, func(t *testing.T, c *sender.Client) {
			msg, err := c.SendMessage(ctx, sender.SendMessageRequest{ChatID: chat, Text: "x"})
			require.NoError(t, err)
			assert.Equal(t, b71Token, msg.Text)
		}},
		{"getChatMember", "getChatMember", `{"status":"member","user":{"id":1,"is_bot":false,"first_name":"` + b71Token + `"}}`, func(t *testing.T, c *sender.Client) {
			m, err := c.GetChatMember(ctx, chat, 1)
			require.NoError(t, err)
			require.NotNil(t, m)
			assert.Equal(t, "member", m.Status())
			assert.Equal(t, b71Token, m.GetUser().FirstName)
		}},
		{"getFile", "getFile", `{"file_id":"f","file_unique_id":"u","file_path":"` + b71Token + `"}`, func(t *testing.T, c *sender.Client) {
			f, err := c.GetFile(ctx, "f")
			require.NoError(t, err)
			assert.Equal(t, b71Token, f.FilePath)
		}},
		{"getChatAdministrators", "getChatAdministrators", `[{"status":"member","user":{"id":1,"is_bot":false,"first_name":"` + b71Token + `"}}]`, func(t *testing.T, c *sender.Client) {
			ms, err := c.GetChatAdministrators(ctx, chat)
			require.NoError(t, err)
			require.Len(t, ms, 1)
			assert.Equal(t, b71Token, ms[0].GetUser().FirstName)
		}},
		{"sendMediaGroup", "sendMediaGroup", `[{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"text":"` + b71Token + `"}]`, func(t *testing.T, c *sender.Client) {
			msgs, err := c.SendMediaGroup(ctx, sender.SendMediaGroupRequest{ChatID: chat, Media: []sender.InputFile{sender.FromFileID("a").WithMediaType("photo"), sender.FromFileID("b").WithMediaType("photo")}})
			require.NoError(t, err)
			require.Len(t, msgs, 1)
			assert.Equal(t, b71Token, msgs[0].Text)
		}},
		{"getUserProfilePhotos", "getUserProfilePhotos", `{"total_count":1,"photos":[[{"file_id":"` + b71Token + `","file_unique_id":"u","width":1,"height":1}]]}`, func(t *testing.T, c *sender.Client) {
			p, err := c.GetUserProfilePhotos(ctx, 1)
			require.NoError(t, err)
			require.Len(t, p.Photos, 1)
			require.Len(t, p.Photos[0], 1)
			assert.Equal(t, b71Token, p.Photos[0][0].FileID)
		}},
		{"getUserProfileAudios", "getUserProfileAudios", `{"total_count":1,"audios":[{"file_id":"f","file_unique_id":"u","duration":1,"title":"` + b71Token + `"}]}`, func(t *testing.T, c *sender.Client) {
			a, err := c.GetUserProfileAudios(ctx, 1)
			require.NoError(t, err)
			require.Len(t, a.Audios, 1)
			assert.Equal(t, b71Token, a.Audios[0].Title)
		}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			rt := &b71RT{status: 200, body: `{"ok":true,"result":` + r.result + `}`}
			c, err := sender.New(b71Token,
				sender.WithHTTPClient(&http.Client{Transport: rt}),
				sender.WithBaseURL("http://b71.example.test"),
				sender.WithRetries(0))
			require.NoError(t, err)
			t.Cleanup(func() { _ = c.Close() })
			r.check(t, c)
			require.Equal(t, []string{"/bot" + b71Token + "/" + r.method}, rt.paths())
		})
	}
}
