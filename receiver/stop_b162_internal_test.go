package receiver

// B-162 white-box rows. R3 stages the state a second Stop sees between the first Stop's Swap and its lock
// (running already false, the loop still alive) by writing running directly. R5a and R5b check that Stop closes
// the stop channel and sets stopped also when the client was not running. The black-box rows are in
// stop_b162_test.go.

import (
	"context"
	"errors"
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

	"github.com/prilive-com/galigo/tg"
)

// b162iRT: call 1 closes entered and waits for a result array on release or for the request context; later calls
// wait for the request context. With fail, every call fails at once.
type b162iRT struct {
	mu      sync.Mutex
	calls   int
	entered chan struct{}
	release chan string
	fail    bool
}

func (rt *b162iRT) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.calls++
	n := rt.calls
	rt.mu.Unlock()
	if rt.fail {
		return nil, errors.New("b162: request failed")
	}
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

type b162iLog struct {
	mu   sync.Mutex
	msgs []string
}

func (l *b162iLog) Enabled(context.Context, slog.Level) bool { return true }
func (l *b162iLog) WithAttrs([]slog.Attr) slog.Handler       { return l }
func (l *b162iLog) WithGroup(string) slog.Handler            { return l }
func (l *b162iLog) Handle(_ context.Context, r slog.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.msgs = append(l.msgs, r.Message)
	return nil
}

func (l *b162iLog) count(msg string) int {
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

func b162iClient(rt *b162iRT, l *b162iLog, maxErrors int) *PollingClient {
	cfg := DefaultConfig()
	cfg.Mode = ModeLongPolling
	cfg.PollingTimeout = 30
	cfg.PollingMaxErrors = maxErrors
	cfg.UpdateDeliveryPolicy = DeliveryPolicyBlock
	cfg.BaseURL = "http://b162.invalid/bot"
	return NewPollingClient(tg.SecretToken("123:fake"), make(chan tg.Update, 10), slog.New(l), cfg,
		WithPollingHTTPClient(&http.Client{Transport: rt}))
}

func b162iDone(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func b162iGo(f func()) chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	return done
}

// b162iStopSignal reports whether stopCh is closed and stopped is set, read under c.mu.
func b162iStopSignal(c *PollingClient) (closed, stopped bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.stopCh:
		closed = true
	default:
	}
	return closed, c.stopped.Load()
}

// R3: running is already false while the loop is still in its request. Stop must still close the stop channel
// and wait for the loop.
func TestB162R3StopWaitsWhenRunningAlreadyFalse(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		rt := &b162iRT{entered: make(chan struct{}), release: make(chan string, 1)}
		c := b162iClient(rt, &b162iLog{}, 0)
		require.NoError(t, c.Start(ctx))
		synctest.Wait()
		require.True(t, b162iDone(rt.entered), "getUpdates call 1 did not begin")
		c.running.Store(false)
		s := b162iGo(c.Stop)
		synctest.Wait()
		time.Sleep(time.Hour)
		synctest.Wait()
		assert.False(t, b162iDone(s), "Stop returned while the poll loop was in its request")
		rt.release <- "[]"
		synctest.Wait()
		assert.True(t, b162iDone(s), "Stop after the request ended: the poll loop got no stop signal")
	})
}

// R5a: Stop on a client that was never started closes the stop channel and sets stopped.
func TestB162R5StopClosesStopChNeverStarted(t *testing.T) {
	c := b162iClient(&b162iRT{}, &b162iLog{}, 0)
	c.Stop()
	closed, stopped := b162iStopSignal(c)
	assert.True(t, closed, "stopCh still open")
	assert.True(t, stopped, "stopped still false")
}

// R5b: the loop ended by itself (PollingMaxErrors 1, the request fails); a later Stop returns, closes the stop
// channel, sets stopped and writes no "long polling stopped" line (no running loop was stopped).
func TestB162R5StopAfterSelfEnd(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		lg := &b162iLog{}
		c := b162iClient(&b162iRT{fail: true}, lg, 1)
		require.NoError(t, c.Start(ctx))
		synctest.Wait()
		require.False(t, c.Running(), "the loop did not end by itself")
		s := b162iGo(c.Stop)
		synctest.Wait()
		assert.True(t, b162iDone(s), "Stop after the loop ended by itself")
		closed, stopped := b162iStopSignal(c)
		assert.True(t, closed, "stopCh still open")
		assert.True(t, stopped, "stopped still false")
		assert.Equal(t, 0, lg.count("long polling stopped"), "stop log lines")
	})
}
