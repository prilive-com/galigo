package galigo

// B-162: Bot.Stop and Bot.Close at the same time. Close must close the updates channel only after the poll loop
// has exited. Internal test: Bot has no option for the receiver's HTTP client, so the rows replace b.receiver with
// a PollingClient on an in-memory RoundTripper (synctest bubble, fake time, no socket).

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/receiver"
	"github.com/prilive-com/galigo/tg"
)

const b162FetchFailed = "fetch updates failed"

// b162BotRT: call 1 closes entered and waits for a result array on release or for the request context; later
// calls wait for the request context.
type b162BotRT struct {
	mu      sync.Mutex
	calls   int
	entered chan struct{}
	release chan string
}

func (rt *b162BotRT) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.calls++
	n := rt.calls
	rt.mu.Unlock()
	if n > 1 {
		<-req.Context().Done()
		return nil, req.Context().Err()
	}
	close(rt.entered)
	select {
	case result := <-rt.release:
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Request: req,
			Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":` + result + `}`))}, nil
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
}

// b162BotLog records log messages; with holdOn set, the first record with that message closes atHold and blocks
// until releaseHold is closed.
type b162BotLog struct {
	mu          sync.Mutex
	msgs        []string
	holdOn      string
	held        bool
	atHold      chan struct{}
	releaseHold chan struct{}
	freeOnce    sync.Once
}

func (l *b162BotLog) Enabled(context.Context, slog.Level) bool { return true }
func (l *b162BotLog) WithAttrs([]slog.Attr) slog.Handler       { return l }
func (l *b162BotLog) WithGroup(string) slog.Handler            { return l }

func (l *b162BotLog) Handle(_ context.Context, r slog.Record) error {
	l.mu.Lock()
	l.msgs = append(l.msgs, r.Message)
	hold := l.holdOn != "" && !l.held && r.Message == l.holdOn
	if hold {
		l.held = true
	}
	l.mu.Unlock()
	if hold {
		close(l.atHold)
		<-l.releaseHold
	}
	return nil
}

func (l *b162BotLog) count(msg string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, m := range l.msgs {
		if m == msg {
			n++
		}
	}
	return n
}

func (l *b162BotLog) free() {
	if l.releaseHold != nil {
		l.freeOnce.Do(func() { close(l.releaseHold) })
	}
}

// b162Bot makes a polling Bot whose receiver uses rt. The caller defers b.sender.Close (with its cancel), so the
// sender's cleanup goroutine ends even when a row fails before the Close under test.
func b162Bot(t *testing.T, rt http.RoundTripper, l *b162BotLog) *Bot {
	t.Helper()
	logger := slog.New(l)
	b, err := New("123456:b162-token-not-real", WithPolling(30, 100), WithLogger(logger))
	require.NoError(t, err)
	cfg := receiver.DefaultConfig()
	cfg.Mode = receiver.ModeLongPolling
	cfg.PollingTimeout = 30
	cfg.UpdateDeliveryPolicy = receiver.DeliveryPolicyBlock
	cfg.BaseURL = "http://b162.invalid/bot"
	b.receiver = receiver.NewPollingClient(b.token, b.updates, logger, cfg,
		receiver.WithPollingHTTPClient(&http.Client{Transport: rt}))
	return b
}

func b162BotGo(f func()) chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	return done
}

func b162BotDone(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

// b162Open: the channel is not closed (no update is sent in these rows, so a receive that proceeds means closed).
func b162Open(ch chan tg.Update) bool {
	select {
	case _, ok := <-ch:
		return ok
	default:
		return true
	}
}

func b162Closed(ch chan tg.Update) bool {
	select {
	case _, ok := <-ch:
		return !ok
	default:
		return false
	}
}

func b162Entered(t *testing.T, rt *b162BotRT) {
	t.Helper()
	synctest.Wait()
	select {
	case <-rt.entered:
	default:
		t.Fatalf("getUpdates call 1 did not begin")
	}
}

// R2: Stop, then Close, while the poller is in its request. Close must not return and the channel must stay open
// while the request is held (1 h of fake time); after the release both return and the channel is closed.
func TestB162R2StopThenClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		rt := &b162BotRT{entered: make(chan struct{}), release: make(chan string, 1)}
		lg := &b162BotLog{}
		b := b162Bot(t, rt, lg)
		defer func() { cancel(); _ = b.sender.Close() }()
		require.NoError(t, b.Start(ctx))
		b162Entered(t, rt)
		stop := b162BotGo(b.Stop)
		synctest.Wait()
		var closeErr error
		closed := b162BotGo(func() { closeErr = b.Close() })
		synctest.Wait()
		time.Sleep(time.Hour)
		synctest.Wait()
		assert.False(t, b162BotDone(stop), "Stop returned while the poller was in its request")
		assert.False(t, b162BotDone(closed), "Close returned while the poller was in its request")
		assert.True(t, b162Open(b.updates), "updates channel closed while the poller was in its request")
		assert.Equal(t, 0, lg.count("long polling stopped"), "stop log while the poller was in its request")
		rt.release <- "[]"
		synctest.Wait()
		assert.True(t, b162BotDone(stop), "Stop after the poll loop exited")
		require.True(t, b162BotDone(closed), "Close after the poll loop exited")
		assert.NoError(t, closeErr)
		assert.True(t, b162Closed(b.updates), "updates channel after Close")
		assert.Equal(t, 1, lg.count("long polling stopped"), "stop log lines")
	})
}

// R2b: Close, then Stop. Neither returns and the channel stays open while the request is held.
func TestB162R2CloseThenStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		rt := &b162BotRT{entered: make(chan struct{}), release: make(chan string, 1)}
		b := b162Bot(t, rt, &b162BotLog{})
		defer func() { cancel(); _ = b.sender.Close() }()
		require.NoError(t, b.Start(ctx))
		b162Entered(t, rt)
		closed := b162BotGo(func() { _ = b.Close() })
		synctest.Wait()
		stop := b162BotGo(b.Stop)
		synctest.Wait()
		time.Sleep(time.Hour)
		synctest.Wait()
		assert.False(t, b162BotDone(closed), "Close returned while the poller was in its request")
		assert.False(t, b162BotDone(stop), "Stop returned while the poller was in its request")
		assert.True(t, b162Open(b.updates), "updates channel closed while the poller was in its request")
		rt.release <- "[]"
		synctest.Wait()
		assert.True(t, b162BotDone(closed), "Close after the poll loop exited")
		assert.True(t, b162BotDone(stop), "Stop after the poll loop exited")
		assert.True(t, b162Closed(b.updates), "updates channel after Close")
	})
}

// R7: the Start context is cancelled while the poller is in its request; the loop logs "fetch updates failed" (held
// by the log handler) and would then return. A Close at that point must still wait and keep the channel open.
func TestB162R7CloseAfterCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		rt := &b162BotRT{entered: make(chan struct{}), release: make(chan string, 1)}
		lg := &b162BotLog{holdOn: b162FetchFailed, atHold: make(chan struct{}), releaseHold: make(chan struct{})}
		b := b162Bot(t, rt, lg)
		defer func() { lg.free(); cancel(); _ = b.sender.Close() }()
		require.NoError(t, b.Start(ctx))
		b162Entered(t, rt)
		cancel()
		synctest.Wait()
		require.True(t, b162BotDone(lg.atHold), "the poll loop did not log the failed request")
		closed := b162BotGo(func() { _ = b.Close() })
		synctest.Wait()
		assert.False(t, b162BotDone(closed), "Close returned while the cancelled poller was still running")
		assert.True(t, b162Open(b.updates), "updates channel closed while the cancelled poller was still running")
		lg.free()
		synctest.Wait()
		assert.True(t, b162BotDone(closed), "Close after the poll loop exited")
		assert.True(t, b162Closed(b.updates), "updates channel after Close")
	})
}

// R6b: Stop, then Close; the poll loop has seen the stop signal and is held in its last log call. Close must still
// wait and the channel must stay open.
func TestB162R6bCloseWaitsForLastLog(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		rt := &b162BotRT{entered: make(chan struct{}), release: make(chan string, 1)}
		lg := &b162BotLog{holdOn: "polling stopped: stop signal", atHold: make(chan struct{}),
			releaseHold: make(chan struct{})}
		b := b162Bot(t, rt, lg)
		defer func() { lg.free(); cancel(); _ = b.sender.Close() }()
		require.NoError(t, b.Start(ctx))
		b162Entered(t, rt)
		stop := b162BotGo(b.Stop)
		synctest.Wait()
		closed := b162BotGo(func() { _ = b.Close() })
		synctest.Wait()
		rt.release <- "[]"
		synctest.Wait()
		require.True(t, b162BotDone(lg.atHold), "the poll loop did not reach its last log line")
		assert.False(t, b162BotDone(stop), "Stop returned before the poll loop exited")
		assert.False(t, b162BotDone(closed), "Close returned before the poll loop exited")
		assert.True(t, b162Open(b.updates), "updates channel closed before the poll loop exited")
		assert.Equal(t, 0, lg.count("long polling stopped"), "stop log before the poll loop exited")
		lg.free()
		synctest.Wait()
		assert.True(t, b162BotDone(stop), "Stop after the poll loop exited")
		assert.True(t, b162BotDone(closed), "Close after the poll loop exited")
		assert.True(t, b162Closed(b.updates), "updates channel after Close")
		assert.Equal(t, 1, lg.count("long polling stopped"), "stop log lines")
	})
}
