package testutil

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/tg"
)

// WebhookSecretCase is a webhook secret and whether Telegram's rule for setWebhook's secret_token accepts it
// (B-103): 1-256 characters, only A-Z, a-z, 0-9, _ and -.
type WebhookSecretCase struct {
	Name   string
	Secret string
	OK     bool
}

// webhookSecretAllowed is Telegram's alphabet for secret_token, written out byte by byte. It is the oracle of
// WebhookSecretMatrix and does not call the check under test.
const webhookSecretAllowed = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"

// webhookSecretOK is the oracle: every byte in webhookSecretAllowed and 1-256 bytes long.
func webhookSecretOK(s string) bool {
	if len(s) < 1 || len(s) > 256 {
		return false
	}
	for i := range len(s) {
		if strings.IndexByte(webhookSecretAllowed, s[i]) < 0 {
			return false
		}
	}
	return true
}

// WebhookSecretMatrix returns every byte 0x00-0xFF alone, first, in the middle and last of a valid secret, a "." at
// every offset of a 256-byte valid secret (so a check that skips one index is caught), the length edges, non-ASCII
// digits, and named cases. The empty secret is not in the list: it means "no secret" (the
// check is off), and each test has its own empty row.
func WebhookSecretMatrix() []WebhookSecretCase {
	var m []WebhookSecretCase
	add := func(name, s string) { m = append(m, WebhookSecretCase{name, s, webhookSecretOK(s)}) }
	for b := range 256 {
		c := string([]byte{byte(b)})
		add(fmt.Sprintf("x%02x_alone", b), c)
		add(fmt.Sprintf("x%02x_first", b), c+"Az9")
		add(fmt.Sprintf("x%02x_middle", b), "Az"+c+"9_")
		add(fmt.Sprintf("x%02x_last", b), "Az9"+c)
	}
	full := strings.Repeat("Az9_", 64)
	add("len1", "Q")
	add("len255", full[:255])
	add("len256", full)
	add("len257", full+"Z")
	add("len300", strings.Repeat("a", 300))
	add("len256_last_bad", full[:255]+".")
	for i := range 255 {
		add(fmt.Sprintf("len256_dot_at_%03d", i), full[:i]+"."+full[i+1:])
	}
	add("all_ends", "09AZaz_-")
	add("redacted", "[REDACTED]")
	add("spaced", " B103 spaced ")
	add("blank", " ")
	add("base64", "abc+/=")
	e := strings.Repeat("\u00e9", 128)
	if len(e) != 256 || utf8.RuneCountInString(e) != 128 {
		panic("e_acute_x128 is not 256 bytes and 128 runes")
	}
	add("e_acute_x128", e)
	// Valid UTF-8 decimal digits outside ASCII: unicode.IsDigit reports true for them, Telegram's 0-9 does not.
	add("u0661", "\u0661")
	add("fullwidth_mixed", "Az\uff11_")
	add("u0967", "\u0967")
	return m
}

// LogRecord is one slog record a LogRecorder received: its level, message and every attribute, including those added
// with Logger.With and a group name added with Logger.WithGroup (as an attribute "group:<name>").
type LogRecord struct {
	Level   slog.Level
	Message string
	Attrs   []slog.Attr
}

// LogRecorder is a slog.Handler that keeps every record at every level (B-103). Loggers derived from it with With or
// WithGroup write to the same list.
type LogRecorder struct {
	mu      *sync.Mutex
	records *[]LogRecord
	pre     []slog.Attr
}

// NewLogRecorder returns an empty LogRecorder.
func NewLogRecorder() *LogRecorder {
	return &LogRecorder{mu: &sync.Mutex{}, records: &[]LogRecord{}}
}

// Enabled reports true for every level, so no record is filtered out.
func (h *LogRecorder) Enabled(context.Context, slog.Level) bool { return true }

// Handle keeps the record.
func (h *LogRecorder) Handle(_ context.Context, r slog.Record) error {
	attrs := slices.Clone(h.pre)
	r.Attrs(func(a slog.Attr) bool { attrs = append(attrs, a); return true })
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.records = append(*h.records, LogRecord{r.Level, r.Message, attrs})
	return nil
}

// WithAttrs returns a handler that adds attrs to every record.
func (h *LogRecorder) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LogRecorder{mu: h.mu, records: h.records, pre: append(slices.Clone(h.pre), attrs...)}
}

// WithGroup returns a handler that adds the attribute "group:<name>" to every record.
func (h *LogRecorder) WithGroup(name string) slog.Handler {
	return h.WithAttrs([]slog.Attr{slog.String("group:"+name, "")})
}

// Records returns a copy of the records kept so far, or nil when there is none.
func (h *LogRecorder) Records() []LogRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(*h.records) == 0 {
		return nil
	}
	return slices.Clone(*h.records)
}

// WebhookAuthLog returns the records a webhook handler logs, in order, while AssertWebhookAuth sends the requests
// of WebhookAuthCases(secret): a refused request logs {ERROR, "unauthorized", code=401} (WebhookHandler.fail), an
// accepted one {DEBUG, "update forwarded", update_id=1} (the default block policy).
func WebhookAuthLog(secret string) []LogRecord {
	var out []LogRecord
	for _, c := range WebhookAuthCases(secret) {
		if c.Want == http.StatusOK {
			out = append(out, LogRecord{slog.LevelDebug, "update forwarded", []slog.Attr{slog.Int("update_id", 1)}})
		} else {
			out = append(out, LogRecord{slog.LevelError, "unauthorized", []slog.Attr{slog.Int("code", http.StatusUnauthorized)}})
		}
	}
	return out
}

// AssertWebhookAuth sends every request of WebhookAuthCases(secret) to h and checks its status, its whole body (a
// refused request gets http.Error's "unauthorized\n", an accepted one nothing), and that an accepted request delivers
// one update and a refused one none (delivered returns the number of updates delivered so far).
func AssertWebhookAuth(t *testing.T, h http.Handler, delivered func() int, secret string) {
	t.Helper()
	for _, c := range WebhookAuthCases(secret) {
		before := delivered()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"update_id":1}`))
		if c.Header != nil {
			req.Header.Set("X-Telegram-Bot-Api-Secret-Token", *c.Header)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, c.Want, rec.Code, c.Name)
		want, body := 0, "unauthorized\n"
		if c.Want == http.StatusOK {
			want, body = 1, ""
		}
		assert.Equal(t, body, rec.Body.String(), c.Name+": body")
		assert.Equal(t, want, delivered()-before, c.Name+": updates delivered")
	}
}

// The settings of a webhook handler for AssertWebhookOther: requests to another host are refused, a body over
// WebhookOtherMaxBody bytes is too large, and the rate limiter allows WebhookOtherBurst requests and then none (rate 0).
const (
	WebhookOtherHost    = "b103.example"
	WebhookOtherMaxBody = 64
	WebhookOtherBurst   = 4
)

// webhookOtherCases are the requests of AssertWebhookOther, in order: each fails on a handler path other than the
// secret check (receiver/webhook.go ServeHTTP and processUpdate), with the status and message that fail logs and writes.
var webhookOtherCases = []struct {
	name, method, host, body, msg string
	want                          int
}{
	{"other_host", http.MethodPost, "other.example", `{"update_id":1}`, "forbidden", http.StatusForbidden},
	{"get", http.MethodGet, WebhookOtherHost, "", "method not allowed", http.StatusMethodNotAllowed},
	{"bad_json", http.MethodPost, WebhookOtherHost, "not json", "invalid JSON", http.StatusBadRequest},
	{"too_large", http.MethodPost, WebhookOtherHost, `{"update_id":1,"x":"` + strings.Repeat("a", WebhookOtherMaxBody) + `"}`, "payload too large", http.StatusRequestEntityTooLarge},
	{"rate_limited", http.MethodPost, WebhookOtherHost, `{"update_id":1}`, "rate limit exceeded", http.StatusTooManyRequests},
}

// AssertWebhookOther sends five requests with the right secret to h, a new handler made with the WebhookOther*
// settings: another host (403), GET (405), a body that is not JSON (400), a body over the limit (413), and a fifth
// request after the burst (429). It checks each status and whole body (http.Error's message and "\n").
func AssertWebhookOther(t *testing.T, h http.Handler, secret string) {
	t.Helper()
	for _, c := range webhookOtherCases {
		req := httptest.NewRequest(c.method, "http://"+c.host+"/", strings.NewReader(c.body))
		if secret != "" {
			req.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, c.want, rec.Code, c.name)
		assert.Equal(t, c.msg+"\n", rec.Body.String(), c.name+": body")
	}
}

// WebhookSecretWarning is the message of the record NewWebhookHandler logs for a non-empty secret outside the rule
// (B-103).
const WebhookSecretWarning = "webhook secret must be 1-256 characters, only A-Z, a-z, 0-9, _ and -; Telegram refuses it in setWebhook"

// WebhookSecretWarnLog returns the records NewWebhookHandler logs when it is made: none for an accepted secret, one
// WARN with WebhookSecretWarning and no attribute otherwise.
func WebhookSecretWarnLog(ok bool) []LogRecord {
	if ok {
		return nil
	}
	return []LogRecord{{Level: slog.LevelWarn, Message: WebhookSecretWarning}}
}

// AssertWebhookHandlerLog checks the handlers newHandler makes for c.Secret and the records rec receives from them
// (B-103, AC3, AC4): the records right after newHandler(false, …) returns, then the AssertWebhookAuth sweep, then the
// whole record list again (the construction records followed by exactly the records of the requests); then a handler
// newHandler(true, …), which must use the WebhookOther* settings, its AssertWebhookOther requests, and the whole list
// once more. So no record and no response body carries the secret on any of these paths.
func AssertWebhookHandlerLog(t *testing.T, rec *LogRecorder, newHandler func(other bool, updates chan tg.Update) http.Handler, c WebhookSecretCase) {
	t.Helper()
	updates := make(chan tg.Update, 1)
	h := newHandler(false, updates)
	if !assert.NotNil(t, h) {
		return
	}
	want := WebhookSecretWarnLog(c.OK)
	assert.Equal(t, want, rec.Records(), "records after the handler is made")
	AssertWebhookAuth(t, h, func() int {
		n := len(updates)
		for range n {
			<-updates
		}
		return n
	}, c.Secret)
	want = append(want, WebhookAuthLog(c.Secret)...)
	assert.Equal(t, want, rec.Records(), "records after the requests")
	AssertWebhookOther(t, newHandler(true, make(chan tg.Update, 1)), c.Secret)
	want = append(append(want, WebhookSecretWarnLog(c.OK)...), WebhookOtherLog()...)
	assert.Equal(t, want, rec.Records(), "records after the other requests")
}

// WebhookOtherLog returns the records a handler logs while AssertWebhookOther runs: one {ERROR, message, code=status}
// per request (WebhookHandler.fail).
func WebhookOtherLog() []LogRecord {
	var out []LogRecord
	for _, c := range webhookOtherCases {
		out = append(out, LogRecord{slog.LevelError, c.msg, []slog.Attr{slog.Int("code", c.want)}})
	}
	return out
}
