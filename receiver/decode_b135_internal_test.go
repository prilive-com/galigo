package receiver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// B-135: one update that does not decode is dropped (log + callback + offset); it does not stop the receiver.

// ---------- bodies ----------

func b135Good(id string) string {
	return `{"update_id":` + id + `,"message":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"text":"kept-` + id + `"}}`
}

// b135Bad has a usable id and a wrong type in message.text.
func b135Bad(id string) string {
	return `{"update_id":` + id + `,"message":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"text":7}}`
}

// b135BadIDRaw puts raw text as the update_id value of a body with a bad field.
func b135BadIDRaw(raw string) string {
	return `{"message":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"text":7},"update_id":` + raw + `}`
}

const (
	b135NoID    = `{"message":{"text":7}}`
	b135Secret  = "test-secret-token"
	b135Mark    = "PRIVATE-MARK-135"
	b135Token   = "135791:QzXjWvKqYpZbNmFgHdJcLrTsBwVx0011" // #nosec G101 -- a fake token for the test, not a credential
	b135DropMsg = "update decode failed, dropping"
)

func b135Batch(elems ...string) string {
	return `{"ok":true,"result":[` + strings.Join(elems, ",") + `]}`
}

// b135Max returns n as decimal text, built with strconv (no 64-bit literal, B-140).
func b135Max(n int64) string { return strconv.FormatInt(n, 10) }

// b135ChatMember puts status as the old chat member status of a chat_member update with id 2.
func b135ChatMember(status string) string {
	return `{"update_id":2,"chat_member":{"chat":{"id":1,"type":"private"},"from":{"id":1,"is_bot":false,"first_name":"a"},"date":1,"old_chat_member":{"status":` + strconv.Quote(status) + `},"new_chat_member":{"status":"member"}}}`
}

// b135L2 has a wrong type under MESSAGE.TeXt: the decode error's field path holds that input text.
const b135L2 = `{"update_id":2,"MESSAGE":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"TeXt":7}}`

// ---------- recording handler and trace ----------

type b135Entry struct {
	level slog.Level
	msg   string
	attrs []slog.Attr
}

func (e b135Entry) text() string {
	var b strings.Builder
	b.WriteString(e.msg)
	for _, a := range e.attrs {
		b.WriteString(" " + a.String())
	}
	return b.String()
}

// b135Lifecycle are today's lifecycle records (receiver/polling.go at 9d1db97); they are kept out of the trace.
var b135Lifecycle = map[string]bool{
	"long polling started":                        true,
	"long polling stopped":                        true,
	"polling stopped: context cancelled":          true,
	"polling stopped: stop signal":                true,
	"stopping update delivery: context cancelled": true,
	"stopping update delivery: stop signal":       true,
	"circuit breaker state changed":               true,
	"deleting existing webhook":                   true,
}

type b135Rec struct {
	mu    sync.Mutex
	recs  []b135Entry
	trace []string
	args  []string // callback arguments, as text
	dropC []int32  // ConsecutiveErrors() at each polling callback
}

func (r *b135Rec) add(s string) {
	r.mu.Lock()
	r.trace = append(r.trace, s)
	r.mu.Unlock()
}

func b135ID(attrs []slog.Attr) string {
	for _, a := range attrs {
		if a.Key == "update_id" {
			return a.Value.String()
		}
	}
	return "?"
}

func (r *b135Rec) record(level slog.Level, msg string, attrs []slog.Attr) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recs = append(r.recs, b135Entry{level: level, msg: msg, attrs: attrs})
	switch {
	case level == slog.LevelDebug && msg == "update sent":
		r.trace = append(r.trace, "sent:"+b135ID(attrs))
	case level == slog.LevelDebug && msg == "update forwarded":
		r.trace = append(r.trace, "fwd:"+b135ID(attrs))
	case level == slog.LevelWarn && msg == b135DropMsg:
		r.trace = append(r.trace, "warn:"+b135ID(attrs))
	case b135Lifecycle[msg]:
	default:
		r.trace = append(r.trace, "rec:"+level.String()+":"+msg)
	}
}

func (r *b135Rec) snapshot() ([]string, []b135Entry, []string, []int32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.trace...), append([]b135Entry(nil), r.recs...),
		append([]string(nil), r.args...), append([]int32(nil), r.dropC...)
}

type b135Handler struct {
	rec   *b135Rec
	attrs []slog.Attr
	group string
}

func (h *b135Handler) Enabled(context.Context, slog.Level) bool { return true }

func (h *b135Handler) Handle(_ context.Context, rec slog.Record) error {
	var own []slog.Attr
	rec.Attrs(func(a slog.Attr) bool { own = append(own, a); return true })
	attrs := append([]slog.Attr(nil), h.attrs...)
	if h.group != "" && len(own) > 0 {
		args := make([]any, len(own))
		for i, a := range own {
			args[i] = a
		}
		attrs = append(attrs, slog.Group(h.group, args...))
	} else {
		attrs = append(attrs, own...)
	}
	h.rec.record(rec.Level, rec.Message, attrs)
	return nil
}

func (h *b135Handler) WithAttrs(as []slog.Attr) slog.Handler {
	return &b135Handler{rec: h.rec, attrs: append(append([]slog.Attr(nil), h.attrs...), as...), group: h.group}
}

func (h *b135Handler) WithGroup(name string) slog.Handler {
	g := name
	if h.group != "" {
		g = h.group + "." + name
	}
	return &b135Handler{rec: h.rec, attrs: h.attrs, group: g}
}

// b135Desc names an update by id and text, so payloads are compared, not only ids.
func b135Desc(us []tg.Update) []string {
	out := []string{}
	for _, u := range us {
		text := "-"
		if u.Message != nil {
			text = u.Message.Text
		}
		out = append(out, fmt.Sprintf("%d:%s", u.UpdateID, text))
	}
	return out
}

func b135Drain(ch chan tg.Update) []tg.Update {
	var out []tg.Update
	for {
		select {
		case u := <-ch:
			out = append(out, u)
		default:
			return out
		}
	}
}

// b135Warns returns the decode Warn records.
func b135Warns(recs []b135Entry) []b135Entry {
	var out []b135Entry
	for _, e := range recs {
		if e.level == slog.LevelWarn && e.msg == b135DropMsg {
			out = append(out, e)
		}
	}
	return out
}

// b135OneWarn checks one decode Warn with exactly one attribute, update_id, of kind Int64.
func b135OneWarn(t *testing.T, recs []b135Entry, id int64) {
	t.Helper()
	b135WarnIDs(t, recs, []int64{id})
}

// b135WarnIDs checks that the decode Warn records are exactly one per id, in order, each with only update_id.
func b135WarnIDs(t *testing.T, recs []b135Entry, ids []int64) {
	t.Helper()
	var got []int64
	for i, w := range b135Warns(recs) {
		require.Len(t, w.attrs, 1, "Warn %d: one attribute", i)
		assert.Equal(t, "update_id", w.attrs[0].Key)
		require.Equal(t, slog.KindInt64, w.attrs[0].Value.Kind())
		got = append(got, w.attrs[0].Value.Int64())
	}
	assert.Equal(t, ids, got, "one decode Warn per drop, in order")
}

// b135NoText checks that no record and no callback argument holds any of needles.
func b135NoText(t *testing.T, recs []b135Entry, args []string, needles []string) {
	t.Helper()
	for _, n := range needles {
		for _, e := range recs {
			assert.NotContains(t, e.text(), n, "record %q", e.msg)
		}
		for _, a := range args {
			assert.NotContains(t, a, n, "callback argument")
		}
	}
}

// b135Windows returns every 4-byte piece of s.
func b135Windows(s string) []string {
	var out []string
	for i := 0; i+4 <= len(s); i++ {
		out = append(out, s[i:i+4])
	}
	return out
}

// ---------- polling fixture ----------

type b135PollOpts struct {
	policy    UpdateDeliveryPolicy
	chanCap   int
	prefill   []tg.Update
	bodies    []string
	noCB      bool
	maxErrors int
	timeout   time.Duration // UpdateDeliveryTimeout
	token     string
	viaOption bool // X1: the callback only through WithUpdateDroppedCallback
	selfStop  bool // P7: the loop stops by itself; no held request
}

type b135Snap struct {
	trace   []string
	recs    []b135Entry
	args    []string
	dropCE  []int32
	ce      int32
	healthy bool
	offset  int64
	queue   []tg.Update
	held    string
}

func b135Poll(t *testing.T, o b135PollOpts) b135Snap {
	t.Helper()
	if o.chanCap == 0 {
		o.chanCap = 10
	}
	if o.token == "" {
		o.token = "123:abc"
	}
	rec := &b135Rec{}
	held := make(chan string, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	var k atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(k.Add(1))
		w.Header().Set("Content-Type", "application/json")
		off := r.URL.Query().Get("offset")
		switch {
		case n <= len(o.bodies):
			rec.add("offset:" + off)
			_, _ = io.WriteString(w, o.bodies[n-1])
			return
		case n == len(o.bodies)+1:
			rec.add("offset:" + off)
			held <- off
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		_, _ = io.WriteString(w, `{"ok":true,"result":[]}`)
	}))

	ch := make(chan tg.Update, o.chanCap)
	for _, u := range o.prefill {
		ch <- u
	}
	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL + "/bot"
	cfg.PollingTimeout = 0
	cfg.RetryInitialDelay = time.Millisecond
	cfg.RetryMaxDelay = 5 * time.Millisecond
	cfg.UpdateDeliveryTimeout = o.timeout
	cfg.UpdateDeliveryPolicy = o.policy
	cfg.PollingMaxErrors = o.maxErrors

	var c *PollingClient
	cb := func(id int, reason string) {
		s := fmt.Sprintf("drop:%d:%s:off=%d:q=%d", id, reason, c.Offset(), len(ch))
		rec.mu.Lock()
		rec.trace = append(rec.trace, s)
		rec.args = append(rec.args, fmt.Sprintf("%d %s", id, reason))
		rec.dropC = append(rec.dropC, c.ConsecutiveErrors())
		rec.mu.Unlock()
	}
	var opts []PollingOption
	switch {
	case o.noCB:
	case o.viaOption:
		opts = append(opts, WithUpdateDroppedCallback(cb))
	default:
		cfg.OnUpdateDropped = cb
	}
	c = NewPollingClient(tg.SecretToken(o.token), ch, slog.New(&b135Handler{rec: rec}), cfg, opts...)
	ctx, cancel := context.WithCancel(context.Background())

	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		cancel()
		joined := make(chan struct{})
		go func() { c.wg.Wait(); close(joined) }()
		select {
		case <-joined:
			c.Stop()
			srv.Close()
		case <-time.After(5 * time.Second):
			t.Errorf("%s: the polling loop did not exit within 5s", t.Name())
			// No Stop: the loop is still running. Every handler returns (release is closed, and a closed
			// connection ends its request context), so Close does not wait for long; it also closes the listener.
			srv.CloseClientConnections()
			srv.Close()
		}
	})

	require.NoError(t, c.Start(ctx))
	var snap b135Snap
	if o.selfStop {
		done := make(chan struct{})
		go func() { c.wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: the loop did not stop by itself within 5s", t.Name())
		}
		select {
		case off := <-held:
			t.Fatalf("held request reached (offset %s); the loop should have stopped", off)
		default:
		}
	} else {
		select {
		case snap.held = <-held:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: no held request within 5s", t.Name())
		}
	}
	snap.trace, snap.recs, snap.args, snap.dropCE = rec.snapshot()
	snap.ce = c.ConsecutiveErrors()
	snap.healthy = c.IsHealthy()
	snap.offset = c.Offset()
	snap.queue = b135Drain(ch)
	return snap
}

var b135Policies = []struct {
	name string
	p    UpdateDeliveryPolicy
}{
	{"Block", DeliveryPolicyBlock},
	{"DropNewest", DeliveryPolicyDropNewest},
	{"DropOldest", DeliveryPolicyDropOldest},
}

// ---------- O: oneMember ----------

func TestB135OneMember(t *testing.T) {
	o6 := `{"update` + "\x5c" + `u005fid":5}` // a JSON escape for "_" inside the name
	require.Contains(t, o6, `\`, "O6 setup: a backslash")
	require.NotContains(t, o6, "_", "O6 setup: no underscore")
	rows := []struct {
		name, in string
		want     string
		ok       bool
	}{
		{"O1 nested not counted", `{"update_id":5,"message":{"update_id":9}}`, "5", true},
		{"O2 nested in array", `{"a":[1,{"update_id":1}],"update_id":5}`, "5", true},
		{"O3 repeated", `{"update_id":1,"update_id":5}`, "", false},
		{"O4 other case only", `{"UPDATE_ID":5}`, "", false},
		{"O5 two case matches", `{"update_id":5,"Update_Id":6}`, "", false},
		{"O6 escaped name", o6, "5", true},
		{"O7 empty object", `{}`, "", false},
		{"O7 array", `[]`, "", false},
		{"O7 string", `"x"`, "", false},
		{"O7 null", `null`, "", false},
		{"O7 number", `7`, "", false},
		{"O14 array with the name", `["update_id",5]`, "", false},
		{"O8 trailing data", `{"update_id":5} x`, "", false},
		{"O9 cut", `{"update_id":5`, "", false},
		{"O10 name as a value", `{"x":"update_id","update_id":5}`, "5", true},
		{"O11 extra brace", `{"update_id":5}}`, "", false},
		{"O12 second value", `{"update_id":5}{"update_id":6}`, "", false},
		{"O13 white space after", "{\"update_id\":5} \n\t", "5", true},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			v, ok := oneMember([]byte(r.in), "update_id")
			require.Equal(t, r.ok, ok)
			if r.ok {
				assert.Equal(t, r.want, string(v))
			} else {
				assert.Nil(t, v, "a false result has a nil value")
			}
		})
	}
}

// ---------- D: decodeUpdate ----------

func TestB135DecodeUpdate(t *testing.T) {
	type want struct {
		id    int
		ok    bool
		isErr bool
		text  string // "" = zero update expected
	}
	maxS := b135Max(math.MaxInt64)
	b4 := b135Bad("2")
	rows := []struct {
		name string
		in   string
		w    want
	}{
		{"D1 good", b135Good("7"), want{7, true, false, "kept-7"}},
		{"D2 empty object", `{}`, want{0, true, false, ""}},
		{"D3 null", `null`, want{0, true, false, ""}},
		{"D4 bad text", b4, want{2, true, true, ""}},
		{"D5 chat member status", b135ChatMember("zzz"), want{2, true, true, ""}},
		{"D6 quote rich texts", `{"update_id":2,"message":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"quote":{"rich_texts":"x"}}}`, want{2, true, true, ""}},
		{"D7 id after the bad field", b135BadIDRaw("2"), want{2, true, true, ""}},
		{"D8 id 1", b135Bad("1"), want{1, true, true, ""}},
		{"D9 id MaxInt64", b135Bad(maxS), want{0, false, true, ""}},
		{"D11 missing", b135NoID, want{0, false, true, ""}},
		{"D12 null id", b135BadIDRaw("null"), want{0, false, true, ""}},
		{"D13 zero", b135BadIDRaw("0"), want{0, false, true, ""}},
		{"D14 minus one", b135BadIDRaw("-1"), want{0, false, true, ""}},
		{"D15 2.0", b135BadIDRaw("2.0"), want{0, false, true, ""}},
		{"D16 1e0", b135BadIDRaw("1e0"), want{0, false, true, ""}},
		{"D17 string", b135BadIDRaw(`"2"`), want{0, false, true, ""}},
		{"D18 too large", b135BadIDRaw("9223372036854775808"), want{0, false, true, ""}},
		{"D19 true", b135BadIDRaw("true"), want{0, false, true, ""}},
		{"D20 repeated", `{"update_id":1,"message":{"text":7},"update_id":3}`, want{0, false, true, ""}},
		{"D21 other case only", `{"UPDATE_ID":3,"message":{"text":7}}`, want{0, false, true, ""}},
		{"D22 array", `[1]`, want{0, false, true, ""}},
		{"D22 string", `"x"`, want{0, false, true, ""}},
		{"D22 number", `7`, want{0, false, true, ""}},
		{"D22 array with the name", `["update_id",2]`, want{0, false, true, ""}},
		{"D23 good id -1", `{"update_id":-1,"message":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"text":"kept"}}`, want{-1, true, false, "kept"}},
		{"D23 good no id", `{"message":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"text":"kept"}}`, want{0, true, false, "kept"}},
		{"D24 good repeated id", `{"update_id":1,"message":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},"text":"kept"},"update_id":3}`, want{3, true, false, "kept"}},
		{"D25 trailing data", b4 + " xyz", want{0, false, true, ""}},
		{"D25 extra brace", b4 + "}", want{0, false, true, ""}},
		{"D26 two case matches", `{"update_id":2,"Update_Id":2,"message":{"text":7}}`, want{0, false, true, ""}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			u, id, ok, err := decodeUpdate([]byte(r.in))
			assert.Equal(t, r.w.ok, ok, "ok")
			assert.Equal(t, r.w.isErr, err != nil, "err: %v", err)
			if ok {
				assert.Equal(t, r.w.id, id, "id")
			}
			if err != nil {
				assert.True(t, u == tg.Update{}, "a zero update with an error")
				return
			}
			if r.w.text == "" {
				assert.True(t, u == tg.Update{}, "zero update")
			} else {
				require.NotNil(t, u.Message)
				assert.Equal(t, r.w.text, u.Message.Text)
				assert.Equal(t, r.w.id, u.UpdateID)
			}
		})
	}
	t.Run("D10 id MaxInt64-1", func(t *testing.T) {
		if strconv.IntSize == 32 {
			t.Skip("needs a 64-bit int")
		}
		_, id, ok, err := decodeUpdate([]byte(b135Bad(b135Max(math.MaxInt64 - 1))))
		require.True(t, ok)
		require.Error(t, err)
		assert.Equal(t, int64(math.MaxInt64-1), int64(id))
	})
	// D27 (c1, C1-02): the math.MaxInt guard matters only where int has 32 bits; the gate row test-386-d27 runs
	// these two leaves with GOARCH=386.
	t.Run("D27a id 2147483647", func(t *testing.T) {
		_, id, ok, err := decodeUpdate([]byte(b135Bad("2147483647")))
		require.Error(t, err)
		require.True(t, ok, "math.MaxInt32 is usable on every arch")
		assert.Equal(t, int64(2147483647), int64(id))
	})
	t.Run("D27b id 2147483648", func(t *testing.T) {
		t.Logf("intsize=%d", strconv.IntSize) // the test-386-d27 gate row needs intsize=32
		_, id, ok, err := decodeUpdate([]byte(b135Bad("2147483648")))
		require.Error(t, err)
		if strconv.IntSize == 32 {
			assert.False(t, ok, "above math.MaxInt on a 32-bit int")
			assert.Equal(t, 0, id)
			return
		}
		require.True(t, ok)
		assert.Equal(t, int64(2147483648), int64(id))
	})
}

// ---------- F: fetchUpdates ----------

func b135Fetch(t *testing.T, body string) (pollBatch, error) {
	t.Helper()
	cfg := DefaultConfig()
	cfg.BaseURL = "http://b135.example.test/bot"
	c := NewPollingClient(tg.SecretToken("123:abc"), make(chan tg.Update, 1), nil, cfg,
		WithPollingHTTPClient(&http.Client{Transport: &b71RT{status: 200, body: body}}))
	return c.fetchUpdates(context.Background())
}

// b135Baseline decodes body as today (getUpdatesResponse, unchanged).
func b135Baseline(t *testing.T, body string) []tg.Update {
	t.Helper()
	var r getUpdatesResponse
	require.NoError(t, json.Unmarshal([]byte(body), &r))
	return r.Result
}

func TestB135FetchUpdates(t *testing.T) {
	const prefix = "telegram API error 0: failed to parse response: "
	typeErr := func(what, field, typ string) string {
		return "json: cannot unmarshal " + what + " into Go struct field getUpdatesResponse." + field + " of type " + typ
	}
	maxS := b135Max(math.MaxInt64)
	refused := []struct {
		name, body, cause, field string
		skip32                   bool
	}{
		{"F3 good then no id", b135Batch(b135Good("1"), b135NoID), typeErr("number", "result.1.message.text", "string"), "result.1.message.text", false},
		{"F4 droppable then no id", b135Batch(b135Bad("1"), b135NoID), typeErr("number", "result.0.message.text", "string"), "result.0.message.text", false},
		{"F5 ok false", `{"ok":false,"error_code":400,"description":"x","result":[` + b135Bad("2") + `]}`, typeErr("number", "result.0.message.text", "string"), "result.0.message.text", false},
		{"F6 error_code string", `{"ok":true,"error_code":"x","result":[` + b135Bad("2") + `]}`, typeErr("string", "error_code", "int"), "error_code", false},
		{"F7a description number, good", `{"ok":true,"description":5,"result":[` + b135Good("1") + `]}`, typeErr("number", "description", "string"), "description", false},
		{"F7b description number, droppable", `{"ok":true,"description":5,"result":[` + b135Bad("2") + `]}`, typeErr("number", "description", "string"), "description", false},
		{"F8a two results, no id first", `{"ok":true,"result":[` + b135NoID + `],"result":[` + b135Good("9") + `]}`, typeErr("number", "result.0.message.text", "string"), "result.0.message.text", false},
		{"F8b two results, droppable", `{"ok":true,"result":[` + b135Good("1") + `,` + b135Bad("2") + `],"result":[` + b135Good("1") + `,` + b135Bad("2") + `]}`, typeErr("number", "result.1.message.text", "string"), "result.1.message.text", false},
		{"F9 RESULT only", `{"ok":true,"RESULT":[` + b135Bad("2") + `]}`, typeErr("number", "RESULT.0.message.text", "string"), "RESULT.0.message.text", false},
		{"F15 result and RESULT", `{"ok":true,"result":[],"RESULT":[` + b135Bad("2") + `]}`, typeErr("number", "RESULT.0.message.text", "string"), "RESULT.0.message.text", false},
		{"F10 good MaxInt64 and droppable", b135Batch(b135Good(maxS), b135Bad("2")), typeErr("number", "result.1.message.text", "string"), "result.1.message.text", true},
		{"F12 result string", `{"ok":true,"result":"x"}`, typeErr("string", "result", "[]tg.Update"), "result", false},
	}
	for _, r := range refused {
		t.Run(r.name, func(t *testing.T) {
			if r.skip32 && strconv.IntSize == 32 {
				t.Skip("needs a 64-bit int")
			}
			b, err := b135Fetch(t, r.body)
			assert.Nil(t, b.updates, "no updates with the error")
			assert.Nil(t, b.errs, "no errs with the error")
			require.EqualError(t, err, prefix+r.cause)
			apiErr, ok := err.(*APIError) //nolint:errorlint // the exact type is the point
			require.True(t, ok, "want *APIError, got %T", err)
			assert.Equal(t, 0, apiErr.Code)
			assert.Equal(t, "failed to parse response", apiErr.Description)
			require.NotNil(t, apiErr.Err)
			assert.Equal(t, r.cause, apiErr.Err.Error())
			var te *json.UnmarshalTypeError
			require.ErrorAs(t, err, &te, "the original cause is reachable")
			assert.Equal(t, r.field, te.Field)
		})
	}

	t.Run("F1 three good", func(t *testing.T) {
		body := b135Batch(b135Good("1"), b135Good("2"), b135Good("3"))
		b, err := b135Fetch(t, body)
		require.NoError(t, err)
		base := b135Baseline(t, body)
		assert.Equal(t, base, b.updates)
		assert.Equal(t, cap(base), cap(b.updates), "the decoded slice, not a copy")
		assert.Nil(t, b.errs, "a clean batch has no errs")
	})
	t.Run("F2 one droppable", func(t *testing.T) {
		b, err := b135Fetch(t, b135Batch(b135Good("1"), b135Bad("2"), b135Good("3")))
		require.NoError(t, err)
		base := b135Baseline(t, b135Batch(b135Good("1"), b135Good("3")))
		require.Len(t, b.updates, 3)
		require.Len(t, b.errs, 3)
		assert.NoError(t, b.errs[0])
		assert.Equal(t, base[0], b.updates[0])
		assert.Error(t, b.errs[1])
		assert.True(t, b.updates[1] == tg.Update{UpdateID: 2}, "only the id")
		assert.NoError(t, b.errs[2])
		assert.Equal(t, base[1], b.updates[2])
	})
	for _, body := range []string{`{"ok":true,"result":null}`, `{"ok":true}`} {
		t.Run("F11 "+body, func(t *testing.T) {
			b, err := b135Fetch(t, body)
			require.NoError(t, err)
			assert.Empty(t, b.updates)
			assert.Nil(t, b.errs)
		})
	}
	t.Run("F13 two results that decode", func(t *testing.T) {
		body := `{"ok":true,"result":[` + b135Good("1") + `],"result":[{"update_id":1}]}`
		b, err := b135Fetch(t, body)
		require.NoError(t, err)
		assert.Equal(t, b135Baseline(t, body), b.updates)
		assert.Nil(t, b.errs)
		require.Len(t, b.updates, 1)
		require.NotNil(t, b.updates[0].Message)
		assert.Equal(t, "kept-1", b.updates[0].Message.Text, "today's merge keeps the text")
	})
	t.Run("F14 a clean batch is not copied", func(t *testing.T) {
		// c2 (C2-01): per clean update, fetchUpdates allocates only what decoding allocates. A copy of the result
		// costs one more allocation for one update and none for an empty batch. Each count is the smallest over
		// many single calls: noise only adds allocations (with -race, sync.Pool drops items at random).
		empty, one := b135Batch(), b135Batch(b135Good("1"))
		require.Less(t, len(one), 512, "both bodies fit the first io.ReadAll buffer")
		fetch := func(body string) uint64 {
			cfg := DefaultConfig()
			cfg.BaseURL = "http://b135.example.test/bot"
			c := NewPollingClient(tg.SecretToken("123:abc"), make(chan tg.Update, 1), nil, cfg,
				WithPollingHTTPClient(&http.Client{Transport: b135RT{body: body}}))
			return b135MinAllocs(func() { _, _ = c.fetchUpdates(context.Background()) })
		}
		decode := func(body string) uint64 {
			raw := []byte(body)
			return b135MinAllocs(func() {
				var r getUpdatesResponse
				_ = json.Unmarshal(raw, &r)
			})
		}
		assert.Equal(t, decode(one)-decode(empty), fetch(one)-fetch(empty), "allocations for one clean update")
		for _, body := range []string{empty, one} { // a copy with another capacity, even one that allocates for both
			b, err := b135Fetch(t, body)
			require.NoError(t, err)
			assert.Equal(t, cap(b135Baseline(t, body)), cap(b.updates), "the decoded slice, not a copy")
		}
	})
}

// b135MinAllocs returns the smallest number of heap allocations of one call of f, over 200 calls.
func b135MinAllocs(f func()) uint64 {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	f() // warm up
	var m runtime.MemStats
	best := uint64(math.MaxUint64)
	for range 200 {
		runtime.ReadMemStats(&m)
		before := m.Mallocs
		f()
		runtime.ReadMemStats(&m)
		best = min(best, m.Mallocs-before)
	}
	return best
}

// b135RT serves body on every request and keeps no state, so F14 counts only fetchUpdates' own allocations.
type b135RT struct{ body string }

func (rt b135RT) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(rt.body)), Request: req}, nil
}

// ---------- P: polling trace ----------

func TestB135PollingTrace(t *testing.T) {
	type row struct {
		name   string
		o      b135PollOpts
		trace  []string // the same for every policy unless per is set
		per    map[UpdateDeliveryPolicy][]string
		queue  []string
		perQ   map[UpdateDeliveryPolicy][]string
		dropCE []int32
	}
	rows := []row{
		{name: "P1 bad in the middle", o: b135PollOpts{bodies: []string{b135Batch(b135Good("1"), b135Bad("2"), b135Good("3"))}},
			trace: []string{"offset:0", "sent:1", "warn:2", "drop:2:decode_error:off=2:q=1", "sent:3", "offset:4"}, queue: []string{"1:kept-1", "3:kept-3"}},
		{name: "P2 bad first", o: b135PollOpts{bodies: []string{b135Batch(b135Bad("1"), b135Good("2"), b135Good("3"))}},
			trace: []string{"offset:0", "warn:1", "drop:1:decode_error:off=0:q=0", "sent:2", "sent:3", "offset:4"}, queue: []string{"2:kept-2", "3:kept-3"}},
		{name: "P3 bad last", o: b135PollOpts{bodies: []string{b135Batch(b135Good("1"), b135Good("2"), b135Bad("3"))}},
			trace: []string{"offset:0", "sent:1", "sent:2", "warn:3", "drop:3:decode_error:off=3:q=2", "offset:4"}, queue: []string{"1:kept-1", "2:kept-2"}},
		{name: "P4 two bad", o: b135PollOpts{bodies: []string{b135Batch(b135Bad("1"), b135Bad("2"))}},
			trace: []string{"offset:0", "warn:1", "drop:1:decode_error:off=0:q=0", "warn:2", "drop:2:decode_error:off=2:q=0", "offset:3"}, queue: []string{}},
		{name: "P5 null then bad", o: b135PollOpts{bodies: []string{b135Batch(b135Good("1"), "null", b135Bad("3"))}},
			trace: []string{"offset:0", "sent:1", "sent:0", "warn:3", "drop:3:decode_error:off=2:q=2", "offset:4"}, queue: []string{"1:kept-1", "0:-"}},
		{name: "P6 no callback", o: b135PollOpts{noCB: true, bodies: []string{b135Batch(b135Good("1"), b135Bad("2"))}},
			trace: []string{"offset:0", "sent:1", "warn:2", "offset:3"}, queue: []string{"1:kept-1"}},
		{name: "P8 lower bad id", o: b135PollOpts{bodies: []string{b135Batch(b135Good("9"), b135Bad("2"))}},
			trace: []string{"offset:0", "sent:9", "warn:2", "drop:2:decode_error:off=10:q=1", "offset:10"}, queue: []string{"9:kept-9"}},
		{name: "P9 full channel", o: b135PollOpts{chanCap: 1, prefill: []tg.Update{{UpdateID: 99}}, bodies: []string{b135Batch(b135Bad("2"))}},
			trace: []string{"offset:0", "warn:2", "drop:2:decode_error:off=0:q=1", "offset:3"}, queue: []string{"99:-"}},
		{name: "P10 policy next to a drop", o: b135PollOpts{chanCap: 1, bodies: []string{b135Batch(b135Good("1"), b135Bad("2"), b135Good("3"))}},
			per: map[UpdateDeliveryPolicy][]string{
				DeliveryPolicyBlock:      {"offset:0", "sent:1", "warn:2", "drop:2:decode_error:off=2:q=1", "rec:WARN:update delivery timeout, dropping", "drop:3:delivery_timeout:off=3:q=1", "offset:4"},
				DeliveryPolicyDropNewest: {"offset:0", "sent:1", "warn:2", "drop:2:decode_error:off=2:q=1", "rec:WARN:channel full, dropping newest update", "drop:3:channel_full_drop_newest:off=3:q=1", "offset:4"},
				DeliveryPolicyDropOldest: {"offset:0", "sent:1", "warn:2", "drop:2:decode_error:off=2:q=1", "rec:WARN:channel full, dropping oldest update", "drop:1:channel_full_drop_oldest:off=3:q=0", "sent:3", "offset:4"},
			},
			perQ: map[UpdateDeliveryPolicy][]string{
				DeliveryPolicyBlock: {"1:kept-1"}, DeliveryPolicyDropNewest: {"1:kept-1"}, DeliveryPolicyDropOldest: {"3:kept-3"},
			}},
		{name: "P11 recovery after a refused batch", o: b135PollOpts{bodies: []string{b135Batch(b135NoID), b135Batch(b135Good("1"), b135Bad("2"), b135Good("3"))}},
			trace: []string{"offset:0", "rec:ERROR:fetch updates failed", "offset:0", "sent:1", "warn:2", "drop:2:decode_error:off=2:q=1", "sent:3", "offset:4"},
			queue: []string{"1:kept-1", "3:kept-3"}, dropCE: []int32{0}},
	}
	for _, r := range rows {
		for _, pol := range b135Policies {
			t.Run(r.name+"/"+pol.name, func(t *testing.T) {
				o := r.o
				o.policy = pol.p
				if r.per != nil && pol.p == DeliveryPolicyBlock {
					o.timeout = 50 * time.Millisecond
				}
				s := b135Poll(t, o)
				want, wantQ := r.trace, r.queue
				if r.per != nil {
					want, wantQ = r.per[pol.p], r.perQ[pol.p]
				}
				assert.Equal(t, want, s.trace, "trace")
				var ids []int64 // C1-03: every decode Warn in full, not only its trace entry
				for _, e := range want {
					if v, ok := strings.CutPrefix(e, "warn:"); ok {
						n, err := strconv.ParseInt(v, 10, 64)
						require.NoError(t, err)
						ids = append(ids, n)
					}
				}
				b135WarnIDs(t, s.recs, ids)
				assert.Equal(t, wantQ, b135Desc(s.queue), "queue")
				assert.Equal(t, int32(0), s.ce, "ConsecutiveErrors")
				assert.True(t, s.healthy, "IsHealthy")
				assert.Equal(t, s.held, strconv.FormatInt(s.offset, 10), "Offset() equals the held request's offset")
				if r.dropCE != nil {
					assert.Equal(t, r.dropCE, s.dropCE, "ConsecutiveErrors at each drop")
				}
			})
		}
	}
}

func TestB135PollingRefusedBatchStops(t *testing.T) { // P7
	body := b135Batch(b135Good("1"), b135Bad("2"), b135NoID)
	for _, pol := range b135Policies {
		t.Run("P7/"+pol.name, func(t *testing.T) {
			s := b135Poll(t, b135PollOpts{policy: pol.p, bodies: []string{body, body}, maxErrors: 2, selfStop: true})
			assert.Equal(t, []string{"offset:0", "rec:ERROR:fetch updates failed", "offset:0", "rec:ERROR:fetch updates failed", "rec:ERROR:max consecutive errors exceeded"}, s.trace)
			assert.Empty(t, s.queue)
			assert.Equal(t, int64(0), s.offset)
			assert.Equal(t, int32(2), s.ce)
		})
	}
}

// ---------- S: the stop guard; N1 ----------

type b135DirectFx struct {
	c   *PollingClient
	ch  chan tg.Update
	rec *b135Rec
}

func b135Direct(t *testing.T, policy UpdateDeliveryPolicy, chanCap int, withCB bool, onDrop func(int)) *b135DirectFx {
	t.Helper()
	f := &b135DirectFx{ch: make(chan tg.Update, chanCap), rec: &b135Rec{}}
	cfg := DefaultConfig()
	cfg.BaseURL = "http://b135.example.test/bot"
	cfg.PollingTimeout = 0
	cfg.UpdateDeliveryTimeout = 0
	cfg.UpdateDeliveryPolicy = policy
	if withCB {
		cfg.OnUpdateDropped = func(id int, reason string) {
			f.rec.add(fmt.Sprintf("drop:%d:%s:off=%d", id, reason, f.c.Offset()))
			if onDrop != nil {
				onDrop(id)
			}
		}
	}
	f.c = NewPollingClient(tg.SecretToken("123:abc"), f.ch, slog.New(&b135Handler{rec: f.rec}), cfg)
	return f
}

var errB135 = errors.New("b135 decode error")

func b135Ctx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return ctx, cancel
}

// b135Drops is a batch where every update failed to decode.
func b135Drops(ids ...int) pollBatch {
	var b pollBatch
	for _, id := range ids {
		b.updates = append(b.updates, tg.Update{UpdateID: id})
		b.errs = append(b.errs, errB135)
	}
	return b
}

func TestB135StopGuard(t *testing.T) {
	for _, pol := range b135Policies {
		t.Run("S1 ctx cancelled/"+pol.name, func(t *testing.T) {
			f := b135Direct(t, pol.p, 0, true, nil)
			ctx, cancel := b135Ctx(t)
			cancel()
			err := f.c.deliverUpdates(ctx, b135Drops(2))
			assert.ErrorIs(t, err, context.Canceled)
			tr, _, _, _ := f.rec.snapshot()
			assert.Empty(t, tr, "no Warn, no callback")
			assert.Equal(t, int64(0), f.c.Offset())
		})
		t.Run("S2 stop closed/"+pol.name, func(t *testing.T) {
			f := b135Direct(t, pol.p, 0, true, nil)
			ctx, _ := b135Ctx(t)
			close(f.c.stopCh)
			err := f.c.deliverUpdates(ctx, b135Drops(2))
			require.EqualError(t, err, "stop signal received")
			tr, _, _, _ := f.rec.snapshot()
			assert.Empty(t, tr, "no Warn, no callback")
			assert.Equal(t, int64(0), f.c.Offset())
			assert.NotErrorIs(t, ctx.Err(), context.DeadlineExceeded)
		})
		t.Run("S3 ctx cancelled by the callback/"+pol.name, func(t *testing.T) {
			ctx, cancel := b135Ctx(t)
			f := b135Direct(t, pol.p, 0, true, func(id int) {
				if id == 1 {
					cancel()
				}
			})
			err := f.c.deliverUpdates(ctx, b135Drops(1, 2))
			assert.ErrorIs(t, err, context.Canceled)
			tr, _, _, _ := f.rec.snapshot()
			assert.Equal(t, []string{"warn:1", "drop:1:decode_error:off=0"}, tr)
			assert.Equal(t, int64(2), f.c.Offset())
		})
		t.Run("S4 both/"+pol.name, func(t *testing.T) {
			f := b135Direct(t, pol.p, 0, true, nil)
			ctx, cancel := b135Ctx(t)
			cancel()
			close(f.c.stopCh)
			err := f.c.deliverUpdates(ctx, b135Drops(2))
			assert.ErrorIs(t, err, context.Canceled, "ctx is checked first")
		})
		t.Run("S6 stop closed by the callback/"+pol.name, func(t *testing.T) {
			ctx, _ := b135Ctx(t)
			var f *b135DirectFx
			f = b135Direct(t, pol.p, 0, true, func(id int) {
				if id == 1 {
					close(f.c.stopCh)
				}
			})
			err := f.c.deliverUpdates(ctx, b135Drops(1, 2))
			require.EqualError(t, err, "stop signal received")
			tr, _, _, _ := f.rec.snapshot()
			assert.Equal(t, []string{"warn:1", "drop:1:decode_error:off=0"}, tr)
			assert.Equal(t, int64(2), f.c.Offset())
			assert.NotErrorIs(t, ctx.Err(), context.DeadlineExceeded)
		})
	}
	for _, pol := range b135Policies[1:] {
		t.Run("S5 a good item is sent/"+pol.name, func(t *testing.T) {
			f := b135Direct(t, pol.p, 1, true, nil)
			ctx, _ := b135Ctx(t)
			close(f.c.stopCh)
			err := f.c.deliverUpdates(ctx, pollBatch{updates: []tg.Update{{UpdateID: 5}}})
			require.NoError(t, err)
			assert.Equal(t, int64(6), f.c.Offset())
			assert.Equal(t, []string{"5:-"}, b135Desc(b135Drain(f.ch)))
		})
	}
}

func TestB135NilCallbackPolling(t *testing.T) { // N1
	f := b135Direct(t, DeliveryPolicyBlock, 0, false, nil)
	ctx, _ := b135Ctx(t)
	var err error
	require.NotPanics(t, func() { err = f.c.deliverUpdates(ctx, b135Drops(2)) })
	require.NoError(t, err)
	require.NoError(t, ctx.Err(), "no wait")
	assert.Equal(t, int64(3), f.c.Offset())
	_, recs, _, _ := f.rec.snapshot()
	b135OneWarn(t, recs, 2)
}

// ---------- W: webhook ----------

type b135HookFx struct {
	h   *WebhookHandler
	ch  chan tg.Update
	rec *b135Rec
}

func b135Hook(t *testing.T, policy UpdateDeliveryPolicy, chanCap int, prefill []tg.Update, withCB bool) *b135HookFx {
	t.Helper()
	if chanCap == 0 {
		chanCap = 10
	}
	f := &b135HookFx{ch: make(chan tg.Update, chanCap), rec: &b135Rec{}}
	for _, u := range prefill {
		f.ch <- u
	}
	cfg := DefaultConfig()
	cfg.WebhookSecret = b135Secret
	cfg.RateLimitRequests = 1000
	cfg.RateLimitBurst = 100
	cfg.MaxBodySize = 1 << 20
	cfg.UpdateDeliveryPolicy = policy
	cfg.UpdateDeliveryTimeout = 0
	if withCB {
		cfg.OnUpdateDropped = func(id int, reason string) {
			f.rec.mu.Lock()
			f.rec.trace = append(f.rec.trace, fmt.Sprintf("drop:%d:%s:q=%d", id, reason, len(f.ch)))
			f.rec.args = append(f.rec.args, fmt.Sprintf("%d %s", id, reason))
			f.rec.mu.Unlock()
		}
	}
	f.h = NewWebhookHandler(slog.New(&b135Handler{rec: f.rec}), f.ch, cfg)
	return f
}

func (f *b135HookFx) post(t *testing.T, body string) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", b135Secret)
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, req)
	if ctx.Err() != nil {
		t.Errorf("the request context expired before ServeHTTP returned: %v", ctx.Err())
	}
	return w.Code, w.Body.String()
}

func TestB135Webhook(t *testing.T) {
	refusal := []string{"rec:ERROR:invalid JSON"}
	w1 := b135Bad("2")
	rows := []struct {
		name    string
		chanCap int
		prefill []tg.Update
		bodies  []string
		code    int
		trace   []string
		queue   []string
		warns   int
	}{
		{name: "W1 bad type", bodies: []string{w1}, code: 200, trace: []string{"warn:2", "drop:2:webhook_decode_error:q=0"}, queue: []string{}, warns: 1},
		{name: "W2 not JSON", bodies: []string{`{not json`}, code: 400, trace: refusal, queue: []string{}},
		{name: "W3 no update_id", bodies: []string{b135NoID}, code: 400, trace: refusal, queue: []string{}},
		{name: "W4 UPDATE_ID only", bodies: []string{`{"UPDATE_ID":2,"message":{"text":7}}`}, code: 400, trace: refusal, queue: []string{}},
		{name: "W5 good", bodies: []string{b135Good("5")}, code: 200, trace: []string{"fwd:5"}, queue: []string{"5:kept-5"}},
		{name: "W8 trailing data", bodies: []string{w1 + " xyz"}, code: 400, trace: refusal, queue: []string{}},
		{name: "W8 extra brace", bodies: []string{w1 + "}"}, code: 400, trace: refusal, queue: []string{}},
		{name: "W9 null", bodies: []string{`null`}, code: 200, trace: []string{"fwd:0"}, queue: []string{"0:-"}},
		{name: "W9 empty object", bodies: []string{`{}`}, code: 200, trace: []string{"fwd:0"}, queue: []string{"0:-"}},
		{name: "W10 two case matches", bodies: []string{`{"update_id":2,"Update_Id":2,"message":{"text":7}}`}, code: 400, trace: refusal, queue: []string{}},
		{name: "W11 full channel", chanCap: 1, prefill: []tg.Update{{UpdateID: 99}}, bodies: []string{w1}, code: 200,
			trace: []string{"warn:2", "drop:2:webhook_decode_error:q=1"}, queue: []string{"99:-"}, warns: 1},
	}
	for _, r := range rows {
		for _, pol := range b135Policies {
			t.Run(r.name+"/"+pol.name, func(t *testing.T) {
				f := b135Hook(t, pol.p, r.chanCap, r.prefill, true)
				for _, b := range r.bodies {
					code, resp := f.post(t, b)
					assert.Equal(t, r.code, code)
					if r.code == 400 {
						assert.Equal(t, "invalid JSON\n", resp)
					}
				}
				tr, recs, _, _ := f.rec.snapshot()
				assert.Equal(t, r.trace, tr, "trace")
				assert.Equal(t, r.queue, b135Desc(b135Drain(f.ch)), "queue")
				if r.warns == 1 {
					b135OneWarn(t, recs, 2)
				} else {
					assert.Empty(t, b135Warns(recs))
				}
			})
		}
	}
	for _, pol := range b135Policies {
		t.Run("W7 ten drops then a good update/"+pol.name, func(t *testing.T) {
			f := b135Hook(t, pol.p, 0, nil, true)
			for i := range 10 {
				code, _ := f.post(t, w1)
				assert.Equal(t, 200, code, "request %d", i+1)
			}
			code, _ := f.post(t, b135Good("5"))
			assert.Equal(t, 200, code)
			var want []string
			for range 10 {
				want = append(want, "warn:2", "drop:2:webhook_decode_error:q=0")
			}
			want = append(want, "fwd:5")
			tr, recs, _, _ := f.rec.snapshot()
			assert.Equal(t, want, tr)
			b135WarnIDs(t, recs, []int64{2, 2, 2, 2, 2, 2, 2, 2, 2, 2}) // c2 (C2-02): every Warn in full
			assert.Equal(t, []string{"5:kept-5"}, b135Desc(b135Drain(f.ch)))
		})
	}
}

func TestB135NilCallbackWebhook(t *testing.T) { // N2, W6
	f := b135Hook(t, DeliveryPolicyBlock, 0, nil, false)
	var code int
	require.NotPanics(t, func() { code, _ = f.post(t, b135Bad("2")) })
	assert.Equal(t, 200, code)
	_, recs, _, _ := f.rec.snapshot()
	b135OneWarn(t, recs, 2)
	assert.Empty(t, b135Drain(f.ch))
}

// ---------- L: no value in the log or the callback ----------

func TestB135NoValueInLog(t *testing.T) {
	fixed := []string{b135Mark, b135DropMsg, "update sent", "update forwarded", "decode_error", "webhook_decode_error",
		"fetch updates failed", "update_id", "long polling started", "long polling stopped"}
	for _, w := range b135Windows(b135Token) {
		for _, f := range fixed {
			require.NotContains(t, f, w, "setup: the token shares no 4-byte piece with %q", f)
		}
	}
	l1 := b135ChatMember(b135Mark + b135Token)
	needles1 := append([]string{b135Mark}, b135Windows(b135Token)...)

	var baseErr error
	var u tg.Update
	baseErr = json.Unmarshal([]byte(b135L2), &u)
	require.Error(t, baseErr)
	require.Contains(t, baseErr.Error(), "MESSAGE.TeXt", "setup: the baseline decode error holds the input path")
	needles2 := []string{"MESSAGE", "TeXt", "TEXT"}

	t.Run("L1 polling", func(t *testing.T) {
		s := b135Poll(t, b135PollOpts{token: b135Token, bodies: []string{b135Batch(l1)}})
		b135OneWarn(t, s.recs, 2)
		assert.Equal(t, []string{"2 decode_error"}, s.args)
		b135NoText(t, s.recs, s.args, needles1)
	})
	t.Run("L2 polling", func(t *testing.T) {
		s := b135Poll(t, b135PollOpts{bodies: []string{b135Batch(b135L2)}})
		b135OneWarn(t, s.recs, 2)
		assert.Equal(t, []string{"2 decode_error"}, s.args)
		b135NoText(t, s.recs, s.args, needles2)
	})
	t.Run("L3 webhook", func(t *testing.T) {
		f := b135Hook(t, DeliveryPolicyBlock, 0, nil, true)
		code, _ := f.post(t, l1)
		assert.Equal(t, 200, code)
		_, recs, args, _ := f.rec.snapshot()
		b135OneWarn(t, recs, 2)
		assert.Equal(t, []string{"2 webhook_decode_error"}, args)
		b135NoText(t, recs, args, needles1)
	})
	t.Run("L4 webhook", func(t *testing.T) {
		f := b135Hook(t, DeliveryPolicyBlock, 0, nil, true)
		code, _ := f.post(t, b135L2)
		assert.Equal(t, 200, code)
		_, recs, args, _ := f.rec.snapshot()
		b135OneWarn(t, recs, 2)
		assert.Equal(t, []string{"2 webhook_decode_error"}, args)
		b135NoText(t, recs, args, needles2)
	})
}

// ---------- X1: the public option ----------

func TestB135PublicCallbackOption(t *testing.T) {
	s := b135Poll(t, b135PollOpts{viaOption: true, bodies: []string{b135Batch(b135Good("1"), b135Bad("2"), b135Good("3"))}})
	assert.Equal(t, []string{"1:kept-1", "3:kept-3"}, b135Desc(s.queue))
	assert.Equal(t, []string{"2 decode_error"}, s.args)
	assert.Equal(t, "4", s.held)
}

// ---------- B-71 bodies: a drop prints no piece of the token ----------

func TestB135B71Drop(t *testing.T) {
	rows := []struct{ name, body, token string }{
		{"nested status, b71Token", b71Nested, b71Token},
		{"nested number, b71Token", b71NestedNum, b71Token},
		{"nested status, over71Token", strings.ReplaceAll(b71Nested, b71Token, over71Token), over71Token},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			s := b135Poll(t, b135PollOpts{token: r.token, bodies: []string{b135Batch(strings.TrimSuffix(strings.TrimPrefix(r.body, `{"ok":true,"result":[`), `]}`))}})
			assert.Equal(t, []string{"1 decode_error"}, s.args, "one drop")
			b135OneWarn(t, s.recs, 1)
			b135NoText(t, s.recs, s.args, b135Windows(r.token))
		})
	}
}
