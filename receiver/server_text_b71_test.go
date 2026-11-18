package receiver_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/receiver"
	"github.com/prilive-com/galigo/tg"
)

// Fake tokens. b71Token has the piece "FakeB71Token"; clean71Token has no 4-byte window in the texts the rows use.
// over71Token shares 4-byte windows with galigo's label "telegram" and the marker "[REDACTED]"; label71Token with
// the labels "failed to decode response" and "failed to parse webhook info" and the marker.
const (
	b71Token     = "123456789:AAFakeB71TokenForUnitUse00000000"
	clean71Token = "987654321:AAxyzTokenNoWindows0000000000000"
	over71Token  = "123456789:AAtelegramREDACTED000000000000"
	label71Token = "123456789:AAparsedecoREDACTED00000000000"

	b71DescWhole = `{"ok":false,"error_code":400,"description":"Bad Request: echo /bot` + b71Token + `/getMe"}`
	b71DescPiece = `{"ok":false,"error_code":400,"description":"Bad Request: echo key FakeB71Token"}`
	b71Decode    = `{"ok":false,"error_code":123456789123456789123,"description":"x"}`
	b71Clean     = `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`
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

func TestWebhookAPIServerTextScrubbed(t *testing.T) {
	calls := []struct {
		name string
		path string
		call func(*http.Client, tg.SecretToken) error
	}{
		{"setWebhook", "setWebhook", func(c *http.Client, tok tg.SecretToken) error {
			return receiver.SetWebhook(context.Background(), c, tok, "https://example.test/hook", "")
		}},
		{"getWebhookInfo", "getWebhookInfo", func(c *http.Client, tok tg.SecretToken) error {
			_, err := receiver.GetWebhookInfo(context.Background(), c, tok)
			return err
		}},
		{"deleteWebhook", "deleteWebhook", func(c *http.Client, tok tg.SecretToken) error {
			return receiver.DeleteWebhook(context.Background(), c, tok, false)
		}},
	}
	rows := []struct {
		name    string
		token   string
		status  int
		body    string
		readErr error
		want    string
		code    int
		cause   string // the exact text of the wrapped error; "" means none
		typed   bool   // a number decode: errors.As still finds the original *json.UnmarshalTypeError
	}{
		{name: "read whole", token: b71Token, status: 200, body: `{"ok":`, readErr: errB71ReadWhole,
			want: "failed to decode response: malformed trailer line: /bot[REDACTED]", cause: "malformed trailer line: /bot[REDACTED]"},
		{name: "read piece", token: b71Token, status: 200, body: `{"ok":`, readErr: errB71ReadPiece,
			want: "failed to decode response: malformed trailer line: key [REDACTED]", cause: "malformed trailer line: key [REDACTED]"},
		{name: "decode piece", token: b71Token, status: 200, body: b71Decode,
			want:  "failed to decode response: json: cannot unmarshal number [REDACTED]123 into Go struct field apiResponse.error_code of type int",
			cause: "json: cannot unmarshal number [REDACTED]123 into Go struct field apiResponse.error_code of type int", typed: true},
		// a scrub of the label, or a second pass over the scrubbed text, changes it
		{name: "decode piece, one pass", token: label71Token, status: 200, body: b71Decode,
			want:  "failed to decode response: json: cannot unmarshal number [REDACTED]123 into Go struct field apiResponse.error_code of type int",
			cause: "json: cannot unmarshal number [REDACTED]123 into Go struct field apiResponse.error_code of type int", typed: true},
		// the status does not choose the path: an undecodable reply at an error status is scrubbed the same way
		{name: "decode piece, status 400", token: b71Token, status: 400, body: b71Decode,
			want:  "failed to decode response: json: cannot unmarshal number [REDACTED]123 into Go struct field apiResponse.error_code of type int",
			cause: "json: cannot unmarshal number [REDACTED]123 into Go struct field apiResponse.error_code of type int", typed: true},
		{name: "description whole", token: b71Token, status: 400, body: b71DescWhole,
			want: "telegram API error 400: Bad Request: echo /bot[REDACTED]/getMe", code: 400},
		// and a description sent with status 200 is scrubbed the same way
		{name: "description whole, status 200", token: b71Token, status: 200, body: b71DescWhole,
			want: "telegram API error 400: Bad Request: echo /bot[REDACTED]/getMe", code: 400},
		{name: "description piece", token: b71Token, status: 400, body: b71DescPiece,
			want: "telegram API error 400: Bad Request: echo key [REDACTED]", code: 400},
		{name: "description, one pass", token: over71Token, status: 400, body: b71DescOver,
			want: "telegram API error 400: Bad Request: echo [REDACTED]", code: 400},
		{name: "clean", token: clean71Token, status: 400, body: b71Clean,
			want: "telegram API error 400: Bad Request: chat not found", code: 400},
	}
	for _, call := range calls {
		for _, r := range rows {
			t.Run(call.name+"/"+r.name, func(t *testing.T) {
				rt := &b71RT{status: r.status, body: r.body, readErr: r.readErr}
				err := call.call(&http.Client{Transport: rt}, tg.SecretToken(r.token))
				require.EqualError(t, err, r.want)
				require.Equal(t, []string{"/bot" + r.token + "/" + call.path}, rt.paths())
				if r.readErr != nil {
					assert.ErrorIs(t, err, r.readErr)
					assertOrigin(t, errors.Unwrap(err), func(e error) bool { return e == r.readErr }) //nolint:errorlint // identity is the point
				}
				assertCause(t, err, r.cause) // the cause one level down is the scrubbed one
				if r.typed {
					var te *json.UnmarshalTypeError
					assert.ErrorAs(t, err, &te)
					assertOrigin(t, errors.Unwrap(err), isTypeErr)
				}
				if r.code != 0 {
					apiErr, ok := err.(*receiver.APIError) //nolint:errorlint // the exact type is the point: returned as it is, with no wrapper
					require.True(t, ok, "want *receiver.APIError, got %T", err)
					assert.Equal(t, r.code, apiErr.Code)
					assert.Equal(t, strings.TrimPrefix(r.want, "telegram API error 400: "), apiErr.Description)
				}
			})
		}
	}
}

// The poll loop logs fetch errors; the logged error is the scrubbed one, and the log holds nothing else. Every record
// is compared whole (fields and records in any order), so a token piece in another field fails too. The JSON handler
// prints an error value with Error(), as the text handler does for an error without its own Format or marshaler.
func TestPollingLogServerTextScrubbed(t *testing.T) {
	rows := []struct {
		name    string
		body    string
		readErr error
		want    string // the exact logged error
	}{
		{name: "description whole", body: b71DescWhole, want: "telegram API error 400: Bad Request: echo /bot[REDACTED]/getMe"},
		{name: "read whole", body: `{"ok":`, readErr: errB71ReadWhole,
			want: "telegram API error 0: request failed: malformed trailer line: /bot[REDACTED]"},
		{name: "decode piece", body: b71Decode,
			want: "telegram API error 0: failed to parse response: json: cannot unmarshal number [REDACTED]123 into Go struct field getUpdatesResponse.error_code of type int"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			logs := &lockedBuffer{}
			cfg := pollingTestConfig()
			cfg.BaseURL = "http://b71.example.test/bot"
			cfg.PollingMaxErrors = 1
			rt := &b71RT{status: 200, body: r.body, readErr: r.readErr}
			noTime := func(groups []string, a slog.Attr) slog.Attr {
				if len(groups) == 0 && a.Key == slog.TimeKey {
					return slog.Attr{}
				}
				return a
			}
			c := receiver.NewPollingClient(tg.SecretToken(b71Token), make(chan tg.Update, 1),
				slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{ReplaceAttr: noTime})), cfg,
				receiver.WithPollingHTTPClient(&http.Client{Transport: rt}))

			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			require.NoError(t, c.Start(ctx))
			require.Eventually(t, func() bool { return !c.Running() }, 5*time.Second, 10*time.Millisecond)

			out := logs.String()
			var got []map[string]any
			for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
				var rec map[string]any
				require.NoError(t, json.Unmarshal([]byte(line), &rec), line)
				if rec["msg"] == "fetch updates failed" {
					_, ok := rec["retry_delay"].(float64) // nanoseconds; the value comes from the backoff
					assert.True(t, ok, "retry_delay: %v", rec["retry_delay"])
					delete(rec, "retry_delay")
				}
				got = append(got, rec)
			}
			assert.ElementsMatch(t, []map[string]any{
				{"level": "INFO", "msg": "long polling started", "timeout": float64(1), "limit": float64(100), "max_errors": float64(1)},
				{"level": "ERROR", "msg": "fetch updates failed", "error": r.want, "consecutive_errors": float64(1)},
				{"level": "ERROR", "msg": "max consecutive errors exceeded", "max_errors": float64(1)},
			}, got)
			assert.NotContains(t, out, b71Token)
			assert.NotContains(t, out, "FakeB71Token")
			assert.NotContains(t, out, "123456789123456789123") // the raw number in b71Decode
			assert.Equal(t, []string{"/bot" + b71Token + "/getUpdates"}, rt.paths())
		})
	}
}

// GetWebhookInfo decodes its result after the envelope; that decode error is scrubbed too.
func TestGetWebhookInfoResultScrubbed(t *testing.T) {
	rt := &b71RT{status: 200, body: `{"ok":true,"result":{"url":"","has_custom_certificate":false,"pending_update_count":123456789123456789123}}`}
	_, err := receiver.GetWebhookInfo(context.Background(), &http.Client{Transport: rt}, tg.SecretToken(b71Token))
	require.EqualError(t, err, "failed to parse webhook info: json: cannot unmarshal number [REDACTED]123 into Go struct field WebhookInfo.pending_update_count of type int")
	require.Equal(t, []string{"/bot" + b71Token + "/getWebhookInfo"}, rt.paths())
	assertCause(t, err, "json: cannot unmarshal number [REDACTED]123 into Go struct field WebhookInfo.pending_update_count of type int")
	var te *json.UnmarshalTypeError
	assert.ErrorAs(t, err, &te)
	assertOrigin(t, errors.Unwrap(err), isTypeErr)
}

// The same with label71Token: the label "failed to parse webhook info" stays as it is, and the cause is scrubbed once.
func TestGetWebhookInfoResultScrubbedOnePass(t *testing.T) {
	rt := &b71RT{status: 200, body: `{"ok":true,"result":{"url":"","has_custom_certificate":false,"pending_update_count":123456789123456789123}}`}
	_, err := receiver.GetWebhookInfo(context.Background(), &http.Client{Transport: rt}, tg.SecretToken(label71Token))
	require.EqualError(t, err, "failed to parse webhook info: json: cannot unmarshal number [REDACTED]123 into Go struct field WebhookInfo.pending_update_count of type int")
	require.Equal(t, []string{"/bot" + label71Token + "/getWebhookInfo"}, rt.paths())
	assertCause(t, err, "json: cannot unmarshal number [REDACTED]123 into Go struct field WebhookInfo.pending_update_count of type int")
	assertOrigin(t, errors.Unwrap(err), isTypeErr)
}

// A successfully decoded webhook info is returned as sent, even when a value holds the token (AC6).
func TestGetWebhookInfoSuccessfulValueReturnedAsSent(t *testing.T) {
	rt := &b71RT{status: 200, body: `{"ok":true,"result":{"url":"https://example.test/` + b71Token + `","has_custom_certificate":false,"pending_update_count":0}}`}
	info, err := receiver.GetWebhookInfo(context.Background(), &http.Client{Transport: rt}, tg.SecretToken(b71Token))
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "https://example.test/"+b71Token, info.URL)
	require.Equal(t, []string{"/bot" + b71Token + "/getWebhookInfo"}, rt.paths())
}

// assertOrigin checks the link below a scrubbed error: the scrubber's wrapper unwraps to the original error
// itself, with no layer added in between.
func assertOrigin(t *testing.T, scrubbed error, original func(error) bool) {
	t.Helper()
	require.NotNil(t, scrubbed)
	assert.True(t, original(errors.Unwrap(scrubbed)), "the scrubbed error must unwrap to the original, got %T", errors.Unwrap(scrubbed))
}

func isTypeErr(e error) bool { _, ok := e.(*json.UnmarshalTypeError); return ok } //nolint:errorlint // one link, not the chain: assertOrigin walks it

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
