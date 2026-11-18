package receiver

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// Every check message starts with a marker, so a failure can be told apart
// by kind: "b113 requests", "b113 client" and "b113 start" check what the
// client does; "b113 delivery" and "b113 wait" check that the run went far
// enough; "b113 setup" checks the test's own inputs. When the request check
// fails, a "b113 capture:" log line holds the recorded requests as JSON.

// b113Token is a fake token in the Bot API token shape.
const b113Token = "123456:b113-FAKE-token"

// b113Target is a Config.BaseURL and where its requests must go.
type b113Target struct {
	base, scheme, host, prefix string
}

// b113Base is a Config.BaseURL that no default has: another scheme than
// the cloud API, a port and a path prefix.
var b113Base = b113Target{base: "http://b113.example.test:8081/custom/bot",
	scheme: "http", host: "b113.example.test:8081", prefix: "/custom/bot"}

// b113Default is the empty Config.BaseURL: the cloud Bot API. The stub
// serves it in memory, so nothing is dialled.
var b113Default = b113Target{base: "", scheme: "https", host: "api.telegram.org", prefix: "/bot"}

// b113Request is one request as the stub saw it.
type b113Request struct {
	Scheme   string
	Host     string
	Method   string
	Path     string // escaped
	RawQuery string
}

// b113Server is an in-memory Bot API stand-in used as an http.RoundTripper:
// no socket is opened. It records every request and answers deleteWebhook
// with true, the first getUpdates with one update (id 41, so the client's
// next offset is 42) and the second with none. Every later getUpdates is
// recorded and gets no answer until its request context ends. When the
// third is recorded, the server closes third and calls onThird (b113Run
// sets it to cancel the run's context), so the third poll ends at once and
// no later request can be sent, however late the test goroutine runs. Any
// other path gets a 404 API error. afterFirst, when set, runs while the
// first getUpdates is served, before its answer is returned.
type b113Server struct {
	mu         sync.Mutex
	reqs       []b113Request
	polls      int
	third      chan struct{}
	onThird    func()
	afterFirst func()
}

func newB113Server() *b113Server {
	return &b113Server{third: make(chan struct{})}
}

// RoundTrip records r and answers it.
func (s *b113Server) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Body != nil {
		_ = r.Body.Close()
	}
	s.mu.Lock()
	s.reqs = append(s.reqs, b113Request{Scheme: r.URL.Scheme, Host: r.URL.Host, Method: r.Method,
		Path: r.URL.EscapedPath(), RawQuery: r.URL.RawQuery})
	poll := 0
	if strings.HasSuffix(r.URL.Path, "/getUpdates") {
		s.polls++
		poll = s.polls
		if poll == 3 {
			close(s.third)
		}
	}
	onThird := s.onThird
	s.mu.Unlock()
	if poll == 3 && onThird != nil {
		onThird()
	}
	var body string
	switch {
	case strings.HasSuffix(r.URL.Path, "/deleteWebhook"):
		body = `{"ok":true,"result":true}`
	case poll == 1:
		if s.afterFirst != nil {
			s.afterFirst()
		}
		body = `{"ok":true,"result":[{"update_id":41}]}`
	case poll == 2:
		body = `{"ok":true,"result":[]}`
	case poll >= 3:
		<-r.Context().Done()
		return nil, r.Context().Err()
	default:
		body = `{"ok":false,"error_code":404,"description":"Not Found"}`
	}
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)), Request: r}, nil
}

func (s *b113Server) requests() []b113Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]b113Request(nil), s.reqs...)
}

// b113Expect is what a row's client must send: where, whether it deletes
// the webhook first, the allowed_updates list, and the timeout and limit
// query values (the row's own fixture values).
type b113Expect struct {
	tgt            b113Target
	del            bool
	list           []string
	timeout, limit int
}

// b113Want is the request list a client must send: an optional
// deleteWebhook, then getUpdates at offset 0, 42 and 42 again (the third
// one), each with allowed_updates when list is non-empty.
func b113Want(t *testing.T, w b113Expect) []b113Request {
	t.Helper()
	var want []b113Request
	if w.del {
		want = append(want, b113Request{Scheme: w.tgt.scheme, Host: w.tgt.host, Method: http.MethodGet,
			Path: w.tgt.prefix + b113Token + "/deleteWebhook", RawQuery: "drop_pending_updates=false"})
	}
	var filter []byte
	if len(w.list) > 0 {
		b, err := json.Marshal(w.list)
		require.NoError(t, err, "b113 setup: marshal the list")
		filter = b
	}
	for _, offset := range []int{0, 42, 42} {
		q := url.Values{"timeout": {strconv.Itoa(w.timeout)}, "limit": {strconv.Itoa(w.limit)}, "offset": {strconv.Itoa(offset)}}
		if filter != nil {
			q.Set("allowed_updates", string(filter))
		}
		want = append(want, b113Request{Scheme: w.tgt.scheme, Host: w.tgt.host, Method: http.MethodGet,
			Path: w.tgt.prefix + b113Token + "/getUpdates", RawQuery: q.Encode()})
	}
	return want
}

// b113Config is DefaultConfig with a fast, known polling setup.
func b113Config(mode Mode, base string) Config {
	cfg := DefaultConfig()
	cfg.Mode = mode
	cfg.BaseURL = base
	cfg.PollingTimeout = 1
	cfg.PollingLimit = 100
	cfg.PollingMaxErrors = 3
	cfg.RetryInitialDelay = 10 * time.Millisecond
	cfg.RetryMaxDelay = 50 * time.Millisecond
	cfg.UpdateDeliveryTimeout = 100 * time.Millisecond
	cfg.WebhookSecret = "b113-secret"
	cfg.AllowedDomain = "b113.example.test"
	return cfg
}

func b113Logger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// b113Run checks that NewPollingClient sent nothing. With optClient nil it
// checks the HTTP client NewPollingClient built (clientTimeout, and a
// default *http.Transport with TLS 1.2 at least), keeps it and sets only its
// Transport to the stub, so a row needs no option; otherwise c must still
// hold optClient. A failed client check falls back to a working client, so
// the run goes on. Then it runs between, starts c with a context it owns
// (the server cancels it at the third getUpdates), waits for update 41 and
// the third getUpdates, waits for the poll goroutine to return and checks
// every request the client sent against want.
func b113Run(t *testing.T, srv *b113Server, c *PollingClient, updates chan tg.Update, between func(),
	optClient *http.Client, clientTimeout time.Duration, want b113Expect) {
	t.Helper()
	assert.Empty(t, srv.requests(), "b113 requests: NewPollingClient must not send a request")
	switch {
	case optClient != nil:
		if !assert.Same(t, optClient, c.client, "b113 client: the option's client is kept") {
			c.client = optClient
		}
	case c.client == nil:
		assert.Fail(t, "b113 client: NewPollingClient built no HTTP client")
		c.client = &http.Client{Transport: srv}
	default:
		assert.Equal(t, clientTimeout, c.client.Timeout, "b113 client: timeout of the default client")
		tr, ok := c.client.Transport.(*http.Transport)
		if assert.True(t, ok && tr != nil, "b113 client: the default client has an *http.Transport") &&
			assert.NotNil(t, tr.TLSClientConfig, "b113 client: the default transport has a TLS config") {
			assert.Equal(t, uint16(tls.VersionTLS12), tr.TLSClientConfig.MinVersion, "b113 client: TLS 1.2 at least")
		}
		c.client.Transport = srv
	}
	if between != nil {
		between()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv.mu.Lock()
	srv.onThird = cancel
	srv.mu.Unlock()
	if assert.NoError(t, c.Start(ctx), "b113 start: Start") {
		select {
		case u := <-updates:
			assert.Equal(t, 41, u.UpdateID, "b113 delivery: update 41")
		case <-time.After(10 * time.Second):
			t.Error("b113 wait: update 41 not delivered within 10s")
		}
		select {
		case <-srv.third:
		case <-time.After(10 * time.Second):
			t.Error("b113 wait: no third getUpdates within 10s")
		}
		cancel()
		c.wg.Wait()
		c.Stop()
	}
	got := srv.requests()
	if !assert.Equal(t, b113Want(t, want), got, "b113 requests: every request the client sent") {
		b, err := json.Marshal(got)
		require.NoError(t, err, "b113 setup: marshal the capture")
		t.Logf("b113 capture: %s", b)
	}
}

type b113Opt int

const (
	b113Absent b113Opt = iota
	b113False
	b113True
)

func (o b113Opt) String() string { return [...]string{"absent", "false", "true"}[o] }

type b113List struct {
	name    string
	list    []string
	present bool // for an option: whether it is passed at all
}

// TestNewPollingClient_ConfigFields checks that NewPollingClient reads
// Config.DeleteWebhookFirst and Config.AllowedUpdates in either Mode, and
// that WithPollingDeleteWebhook and WithPollingAllowedUpdates override them
// in both directions (an option given as false, nil or empty included). A
// row with both options absent passes no option at all. Every combination
// is one row. The client gets copies of the fixture lists, so a constructor
// that changed its input could not change the expected requests.
func TestNewPollingClient_ConfigFields(t *testing.T) {
	modes := []Mode{ModeLongPolling, ModeWebhook}
	delFields := []bool{false, true}
	delOpts := []b113Opt{b113Absent, b113False, b113True}
	listFields := []b113List{
		{name: "nil"},
		{name: "empty", list: []string{}},
		{name: "one", list: []string{"message"}},
		{name: "two", list: []string{"message", "callback_query"}},
	}
	listOpts := []b113List{
		{name: "absent"},
		{name: "nil", present: true},
		{name: "empty", list: []string{}, present: true},
		{name: "value", list: []string{"edited_message"}, present: true},
	}
	rows := 0
	for _, mode := range modes {
		for _, df := range delFields {
			for _, do := range delOpts {
				for _, lf := range listFields {
					for _, lo := range listOpts {
						rows++
						name := fmt.Sprintf("mode %s delete field %t option %s list field %s option %s", mode, df, do, lf.name, lo.name)
						t.Run(name, func(t *testing.T) {
							srv := newB113Server()
							cfg := b113Config(mode, b113Base.base)
							cfg.DeleteWebhookFirst = df
							cfg.AllowedUpdates = slices.Clone(lf.list)
							var opts []PollingOption
							want := b113Expect{tgt: b113Base, del: df, list: lf.list, timeout: 1, limit: 100}
							if do != b113Absent {
								opts = append(opts, WithPollingDeleteWebhook(do == b113True))
								want.del = do == b113True
							}
							if lo.present {
								opts = append(opts, WithPollingAllowedUpdates(slices.Clone(lo.list)))
								want.list = lo.list
							}
							updates := make(chan tg.Update, 10)
							c := NewPollingClient(tg.SecretToken(b113Token), updates, b113Logger(), cfg, opts...)
							b113Run(t, srv, c, updates, nil, nil, 11*time.Second, want)
						})
					}
				}
			}
		}
	}
	require.Equal(t, 192, rows, "b113 setup: row count")
}

// TestNewPollingClient_ConfigFieldsModeBase checks both fields together,
// with no option, for every Mode (the zero Mode and a Mode value the
// package does not define included) with a custom and with an empty
// Config.BaseURL (the cloud Bot API, served in memory).
func TestNewPollingClient_ConfigFieldsModeBase(t *testing.T) {
	modes := []struct {
		name string
		mode Mode
	}{{"longpolling", ModeLongPolling}, {"webhook", ModeWebhook}, {"zero", ""}, {"other", Mode("b113-other")}}
	bases := []struct {
		name string
		tgt  b113Target
	}{{"custom", b113Base}, {"default", b113Default}}
	for _, m := range modes {
		for _, b := range bases {
			t.Run(fmt.Sprintf("mode %s base %s", m.name, b.name), func(t *testing.T) {
				srv := newB113Server()
				cfg := b113Config(m.mode, b.tgt.base)
				cfg.DeleteWebhookFirst = true
				cfg.AllowedUpdates = []string{"message", "callback_query"}
				updates := make(chan tg.Update, 10)
				c := NewPollingClient(tg.SecretToken(b113Token), updates, b113Logger(), cfg)
				b113Run(t, srv, c, updates, nil, nil, 11*time.Second,
					b113Expect{tgt: b.tgt, del: true, list: []string{"message", "callback_query"}, timeout: 1, limit: 100})
			})
		}
	}
}

// TestNewPollingClient_ConfigFieldsOptionsDefaultBase checks the options
// against the fields with an empty Config.BaseURL; the fields are delete
// true and two types. In O1 both options differ from the fields and win. O2
// is the shape galigo.New uses: the same values in the fields and the
// options, so one delete and one filter. In O3 and O4 only the list option
// is passed, nil or empty: it wins, so no filter is sent, and the delete
// comes from the field.
func TestNewPollingClient_ConfigFieldsOptionsDefaultBase(t *testing.T) {
	tests := []struct {
		name    string
		delOpt  bool // pass WithPollingDeleteWebhook(del)
		del     bool
		list    []string
		wantDel bool
	}{
		{name: "O1 options differ", delOpt: true, del: false, list: []string{"edited_message"}},
		{name: "O2 bot shape", delOpt: true, del: true, list: []string{"message", "callback_query"}, wantDel: true},
		{name: "O3 nil list option", list: nil, wantDel: true},
		{name: "O4 empty list option", list: []string{}, wantDel: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newB113Server()
			cfg := b113Config(ModeLongPolling, "")
			cfg.DeleteWebhookFirst = true
			cfg.AllowedUpdates = []string{"message", "callback_query"}
			opts := []PollingOption{WithPollingAllowedUpdates(slices.Clone(tt.list))}
			if tt.delOpt {
				opts = append(opts, WithPollingDeleteWebhook(tt.del))
			}
			updates := make(chan tg.Update, 10)
			c := NewPollingClient(tg.SecretToken(b113Token), updates, b113Logger(), cfg, opts...)
			b113Run(t, srv, c, updates, nil, nil, 11*time.Second,
				b113Expect{tgt: b113Default, del: tt.wantDel, list: tt.list, timeout: 1, limit: 100})
		})
	}
}

// TestNewPollingClient_ConfigFieldsOptionClient passes the stub with
// WithPollingHTTPClient, the only option, and keeps that client through
// Start, so a request the constructor sent through it would be recorded:
// there must be none before Start.
func TestNewPollingClient_ConfigFieldsOptionClient(t *testing.T) {
	srv := newB113Server()
	cfg := b113Config(ModeWebhook, b113Base.base)
	cfg.DeleteWebhookFirst = true
	cfg.AllowedUpdates = []string{"message", "callback_query"}
	updates := make(chan tg.Update, 10)
	hc := &http.Client{Transport: srv}
	c := NewPollingClient(tg.SecretToken(b113Token), updates, b113Logger(), cfg, WithPollingHTTPClient(hc))
	b113Run(t, srv, c, updates, nil, hc, 0,
		b113Expect{tgt: b113Base, del: true, list: []string{"message", "callback_query"}, timeout: 1, limit: 100})
}

// TestNewPollingClient_ConfigValues checks values the other rows do not
// use; the client must send each list as given. "zero config" is a zero
// Config (Mode, timeout, limit and every other field zero, so the cloud Bot
// API) with only the two fields set: three update types, not in sorted
// order, unknown to galigo, with spaces at both ends, upper case, a final
// TAB and a final CR. "repeated and empty type" has a type twice in a row,
// an empty one and one with a final LF.
func TestNewPollingClient_ConfigValues(t *testing.T) {
	t.Run("zero config", func(t *testing.T) {
		srv := newB113Server()
		updates := make(chan tg.Update, 10)
		c := NewPollingClient(tg.SecretToken(b113Token), updates, b113Logger(),
			Config{DeleteWebhookFirst: true, AllowedUpdates: []string{"edited_message\t", " B113_custom ", "message\r"}})
		b113Run(t, srv, c, updates, nil, nil, 10*time.Second,
			b113Expect{tgt: b113Default, del: true, list: []string{"edited_message\t", " B113_custom ", "message\r"}, timeout: 0, limit: 0})
	})
	t.Run("repeated and empty type", func(t *testing.T) {
		srv := newB113Server()
		cfg := b113Config(ModeLongPolling, b113Base.base)
		cfg.AllowedUpdates = []string{"message", "message", "", "callback_query\n"}
		updates := make(chan tg.Update, 10)
		c := NewPollingClient(tg.SecretToken(b113Token), updates, b113Logger(), cfg)
		b113Run(t, srv, c, updates, nil, nil, 11*time.Second,
			b113Expect{tgt: b113Base, list: []string{"message", "message", "", "callback_query\n"}, timeout: 1, limit: 100})
	})
}

// TestNewPollingClient_ConfigAllowedUpdatesCopied checks that the client
// keeps its own copy of Config.AllowedUpdates, for a one-element list with
// no spare capacity (custom base) and a two-element list with one spare slot
// (empty Config.BaseURL), with no options and with an option that does not
// set the list (WithPollingDeleteWebhook(false), the field's own value):
// NewPollingClient leaves the caller's slice as it was, spare capacity
// included, and writing an element of it after NewPollingClient (before
// Start, or while the first getUpdates is served) does not change the
// requests.
func TestNewPollingClient_ConfigAllowedUpdatesCopied(t *testing.T) {
	for _, size := range []string{"one", "two"} {
		for _, when := range []string{"before start", "between polls"} {
			for _, withOpt := range []bool{false, true} {
				name := size + " " + when
				var opts []PollingOption
				if withOpt {
					name += " with option"
					opts = []PollingOption{WithPollingDeleteWebhook(false)}
				}
				t.Run(name, func(t *testing.T) {
					orig := []string{"message", "callback_query"}
					if size == "one" {
						orig = orig[:1]
					}
					// one spare slot for the two-element list, none for the one-element list
					list := make([]string, len(orig), 2*len(orig)-1)
					copy(list, orig)
					whole := append(slices.Clone(orig), make([]string, cap(list)-len(list))...)
					tgt := b113Base
					if size == "two" {
						tgt = b113Default
					}
					srv := newB113Server()
					cfg := b113Config(ModeLongPolling, tgt.base)
					cfg.AllowedUpdates = list
					updates := make(chan tg.Update, 10)
					c := NewPollingClient(tg.SecretToken(b113Token), updates, b113Logger(), cfg, opts...)
					assert.Equal(t, whole, list[:cap(list)], "b113 client: NewPollingClient must not change the caller's list or its spare capacity")
					write := func() { list[0] = "poll" }
					var between func()
					if when == "before start" {
						between = write
					} else {
						srv.afterFirst = write
					}
					b113Run(t, srv, c, updates, between, nil, 11*time.Second,
						b113Expect{tgt: tgt, list: orig, timeout: 1, limit: 100})
				})
			}
		}
	}
}

// b113Env is every variable LoadConfig reads, set to a valid value (empty
// only where LoadConfig accepts empty), so LoadConfig cannot fail on one a
// row does not name. The webhook secret and domain are set, so a copy that
// depended on them being empty would show.
var b113Env = map[string]string{
	"RECEIVER_MODE": "longpolling", "TELEGRAM_BOT_TOKEN": b113Token,
	"WEBHOOK_PORT": "8443", "TLS_CERT_PATH": "", "TLS_KEY_PATH": "", "WEBHOOK_SECRET": "b113-secret",
	"ALLOWED_DOMAIN": "b113.example.test", "WEBHOOK_URL": "https://b113.example.test/hook",
	"POLLING_TIMEOUT": "1", "POLLING_LIMIT": "100", "POLLING_MAX_ERRORS": "3",
	"POLLING_DELETE_WEBHOOK": "false", "ALLOWED_UPDATES": "",
	"POLLING_RETRY_INITIAL_DELAY": "10ms", "POLLING_RETRY_MAX_DELAY": "50ms", "POLLING_RETRY_BACKOFF_FACTOR": "2.0",
	"RATE_LIMIT_REQUESTS": "10", "RATE_LIMIT_BURST": "20", "MAX_BODY_SIZE": "1048576",
	"UPDATE_DELIVERY_POLICY": "block", "UPDATE_DELIVERY_TIMEOUT": "100ms",
	"BREAKER_MAX_REQUESTS": "5", "BREAKER_INTERVAL": "60s", "BREAKER_TIMEOUT": "30s",
	"READ_TIMEOUT": "10s", "READ_HEADER_TIMEOUT": "5s", "WRITE_TIMEOUT": "10s", "IDLE_TIMEOUT": "120s",
	"DRAIN_DELAY": "0s", "SHUTDOWN_TIMEOUT": "5s",
}

// TestNewPollingClient_LoadConfigFields checks the case end to end: a
// client built from LoadConfig, with no option and LoadConfig's empty
// BaseURL (the cloud Bot API, served in memory), sends the delete and the
// filter that POLLING_DELETE_WEBHOOK and ALLOWED_UPDATES set, in either
// receiver mode. POLLING_DELETE_WEBHOOK is true only for "true" in any
// case; "1" is false (today's parse). WEBHOOK_URL is set in every row, so
// a delete taken from it instead would show. L2 sets POLLING_TIMEOUT to 30,
// so the timeout values are not only the other rows' 1.
func TestNewPollingClient_LoadConfigFields(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		del      string
		allowed  string
		timeout  int
		wantDel  bool
		wantList []string
	}{
		{name: "L1 true and two types with spaces", mode: "longpolling", del: "true", allowed: " message, callback_query ",
			timeout: 1, wantDel: true, wantList: []string{"message", "callback_query"}},
		{name: "L2 TRUE and no types", mode: "longpolling", del: "TRUE", timeout: 30, wantDel: true},
		{name: "L3 false and no types", mode: "longpolling", del: "false", timeout: 1},
		{name: "L4 1 is not true", mode: "longpolling", del: "1", allowed: "message", timeout: 1, wantList: []string{"message"}},
		{name: "L5 webhook mode", mode: "webhook", del: "true", allowed: "edited_message,message", timeout: 1,
			wantDel: true, wantList: []string{"edited_message", "message"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range b113Env {
				t.Setenv(k, v)
			}
			t.Setenv("RECEIVER_MODE", tt.mode)
			t.Setenv("POLLING_DELETE_WEBHOOK", tt.del)
			t.Setenv("ALLOWED_UPDATES", tt.allowed)
			t.Setenv("POLLING_TIMEOUT", strconv.Itoa(tt.timeout))
			cfg, err := LoadConfig()
			require.NoError(t, err, "b113 setup: LoadConfig")
			require.NotNil(t, cfg, "b113 setup: LoadConfig")
			require.Empty(t, cfg.BaseURL, "b113 setup: LoadConfig sets no BaseURL")
			srv := newB113Server()
			updates := make(chan tg.Update, 10)
			c := NewPollingClient(cfg.Token, updates, b113Logger(), *cfg)
			b113Run(t, srv, c, updates, nil, nil, time.Duration(tt.timeout+10)*time.Second,
				b113Expect{tgt: b113Default, del: tt.wantDel, list: tt.wantList, timeout: tt.timeout, limit: 100})
		})
	}
}
