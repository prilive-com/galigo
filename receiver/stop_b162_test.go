package receiver_test

// B-162: PollingClient.Stop must return only after the poll loop has exited, also when another Stop is already
// stopping it. Rows R1, R1b, R4a-R4e and R6 run in a synctest bubble (fake time; everything a row blocks on is
// made inside the bubble; the poller uses an in-memory RoundTripper, so no socket is used). S2a and S2b are
// bounded real-time fan-in rows. The internal rows R3, R5a, R5b are in stop_b162_internal_test.go.

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/receiver"
	"github.com/prilive-com/galigo/tg"
)

const (
	b162StopMsg     = "long polling stopped"
	b162LastLoopLog = "polling stopped: stop signal"
)

// b162RT answers getUpdates. Call n (1-based, up to len(entered)) closes entered[n-1] and waits for a result array
// on release[n-1] or for the request context to end. Later calls wait for the request context. With fast, every
// call after call 1 answers [] at once.
type b162RT struct {
	mu      sync.Mutex
	calls   int
	entered []chan struct{}
	release []chan string
	fast    bool
}

func newB162RT(n int) *b162RT {
	rt := &b162RT{}
	for range n {
		rt.entered = append(rt.entered, make(chan struct{}))
		rt.release = append(rt.release, make(chan string, 1))
	}
	return rt
}

func (rt *b162RT) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.calls++
	n := rt.calls
	rt.mu.Unlock()
	if rt.fast && n > 1 {
		return b162Resp(req, "[]"), nil
	}
	if n > len(rt.entered) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	}
	close(rt.entered[n-1])
	select {
	case result := <-rt.release[n-1]:
		return b162Resp(req, result), nil
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
}

func (rt *b162RT) send(n int, result string) { rt.release[n-1] <- result }

func b162Resp(req *http.Request, result string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Request: req,
		Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":` + result + `}`))}
}

// b162Log records log messages. With holdOn set, the first record with that message closes atHold and blocks
// until releaseHold is closed (the log call the poll loop already makes is the hook; no product hook).
type b162Log struct {
	mu          sync.Mutex
	msgs        []string
	holdOn      string
	held        bool
	atHold      chan struct{}
	releaseHold chan struct{}
	freeOnce    sync.Once
}

func (l *b162Log) Enabled(context.Context, slog.Level) bool { return true }
func (l *b162Log) WithAttrs([]slog.Attr) slog.Handler       { return l }
func (l *b162Log) WithGroup(string) slog.Handler            { return l }

func (l *b162Log) Handle(_ context.Context, r slog.Record) error {
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

func (l *b162Log) count(msg string) int {
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

func (l *b162Log) hold(msg string) {
	l.holdOn, l.atHold, l.releaseHold = msg, make(chan struct{}), make(chan struct{})
}

func (l *b162Log) free() {
	if l.releaseHold != nil {
		l.freeOnce.Do(func() { close(l.releaseHold) })
	}
}

func b162Client(rt *b162RT, l *b162Log) (*receiver.PollingClient, chan tg.Update) {
	cfg := receiver.DefaultConfig()
	cfg.Mode = receiver.ModeLongPolling
	cfg.PollingTimeout = 30
	cfg.PollingMaxErrors = 0
	cfg.UpdateDeliveryPolicy = receiver.DeliveryPolicyBlock
	cfg.BaseURL = "http://b162.invalid/bot"
	ch := make(chan tg.Update, 10)
	return receiver.NewPollingClient(tg.SecretToken("123:fake"), ch, slog.New(l), cfg,
		receiver.WithPollingHTTPClient(&http.Client{Transport: rt})), ch
}

// b162Go runs f in a goroutine; the channel closes when f returns.
func b162Go(f func()) chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	return done
}

func b162Done(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

// b162Entered: after synctest.Wait, call n must have begun (a non-blocking check, so a loop that never gets there
// fails the row instead of deadlocking the bubble).
func b162Entered(t *testing.T, rt *b162RT, n int) {
	t.Helper()
	synctest.Wait()
	select {
	case <-rt.entered[n-1]:
	default:
		t.Fatalf("getUpdates call %d did not begin", n)
	}
}

func b162Got(ch <-chan tg.Update) (int, bool) {
	select {
	case u := <-ch:
		return u.UpdateID, true
	default:
		return 0, false
	}
}

// R1: two Stops while the poller is in its request. Neither may return while the request is held (1 h of fake
// time) and nothing logs "long polling stopped" yet; after the release both return and the line is written once.
func TestB162R1TwoStopsBothWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		rt, lg := newB162RT(1), &b162Log{}
		c, _ := b162Client(rt, lg)
		require.NoError(t, c.Start(ctx))
		b162Entered(t, rt, 1)
		a := b162Go(c.Stop)
		synctest.Wait()
		b := b162Go(c.Stop)
		synctest.Wait()
		time.Sleep(time.Hour)
		synctest.Wait()
		assert.False(t, b162Done(a), "Stop A returned while the poller was in its request")
		assert.False(t, b162Done(b), "Stop B returned while the poller was in its request")
		assert.False(t, c.Running(), "Running() while stopping")
		assert.Equal(t, 0, lg.count(b162StopMsg), "stop log while the poller was in its request")
		rt.send(1, "[]")
		synctest.Wait()
		assert.True(t, b162Done(a), "Stop A after the poll loop exited")
		assert.True(t, b162Done(b), "Stop B after the poll loop exited")
		assert.Equal(t, 1, lg.count(b162StopMsg), "stop log lines")
	})
}

// R1b: three Stops; none returns while the request is held; one stop log line.
func TestB162R1ThreeStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		rt, lg := newB162RT(1), &b162Log{}
		c, _ := b162Client(rt, lg)
		require.NoError(t, c.Start(ctx))
		b162Entered(t, rt, 1)
		var stops []chan struct{}
		for range 3 {
			stops = append(stops, b162Go(c.Stop))
			synctest.Wait()
		}
		time.Sleep(time.Hour)
		synctest.Wait()
		for i, s := range stops {
			assert.False(t, b162Done(s), "Stop %d returned while the poller was in its request", i+1)
		}
		assert.False(t, c.Running(), "Running() while stopping")
		assert.Equal(t, 0, lg.count(b162StopMsg), "stop log while the poller was in its request")
		rt.send(1, "[]")
		synctest.Wait()
		for i, s := range stops {
			assert.True(t, b162Done(s), "Stop %d after the poll loop exited", i+1)
		}
		assert.Equal(t, 1, lg.count(b162StopMsg), "stop log lines")
	})
}

// R4a: one Stop stops a running poller and logs once.
func TestB162R4OneStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		rt, lg := newB162RT(1), &b162Log{}
		c, _ := b162Client(rt, lg)
		require.NoError(t, c.Start(ctx))
		b162Entered(t, rt, 1)
		s := b162Go(c.Stop)
		synctest.Wait()
		rt.send(1, "[]")
		synctest.Wait()
		assert.True(t, b162Done(s), "Stop after the poll loop exited")
		assert.False(t, c.Running(), "Running() after Stop")
		assert.Equal(t, 1, lg.count(b162StopMsg), "stop log lines")
	})
}

// R4b: Stop on a client that was never started returns at once and logs nothing; a later Start still works.
func TestB162R4StopNeverStarted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		rt, lg := newB162RT(2), &b162Log{}
		c, ch := b162Client(rt, lg)
		first := b162Go(c.Stop)
		synctest.Wait()
		assert.True(t, b162Done(first), "Stop on a client that was never started")
		assert.Equal(t, 0, lg.count(b162StopMsg), "stop log lines")
		require.NoError(t, c.Start(ctx))
		b162Entered(t, rt, 1)
		rt.send(1, `[{"update_id":1}]`)
		synctest.Wait()
		id, ok := b162Got(ch)
		assert.True(t, ok && id == 1, "update 1 after Start: got %d, %v", id, ok)
		b162Entered(t, rt, 2)
		s := b162Go(c.Stop)
		synctest.Wait()
		rt.send(2, "[]")
		synctest.Wait()
		assert.True(t, b162Done(s), "Stop after the poll loop exited")
		assert.Equal(t, 1, lg.count(b162StopMsg), "stop log lines")
	})
}

// R4c: a second Stop on a stopped client returns at once, does not panic and logs nothing more. Only this Stop
// recovers: a panic under c.mu leaves the mutex locked, so a recover in other rows would turn it into a hang.
func TestB162R4StopTwice(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		rt, lg := newB162RT(1), &b162Log{}
		c, _ := b162Client(rt, lg)
		require.NoError(t, c.Start(ctx))
		b162Entered(t, rt, 1)
		s := b162Go(c.Stop)
		synctest.Wait()
		rt.send(1, "[]")
		synctest.Wait()
		require.True(t, b162Done(s), "first Stop")
		again := b162Go(func() {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("second Stop panicked: %v", p)
				}
			}()
			c.Stop()
		})
		synctest.Wait()
		assert.True(t, b162Done(again), "second Stop on a stopped client")
		assert.Equal(t, 1, lg.count(b162StopMsg), "stop log lines")
	})
}

// R4d: after two Stops have returned, Start works again and delivers.
func TestB162R4RestartAfterStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		rt, lg := newB162RT(3), &b162Log{}
		c, ch := b162Client(rt, lg)
		require.NoError(t, c.Start(ctx))
		b162Entered(t, rt, 1)
		a := b162Go(c.Stop)
		b := b162Go(c.Stop)
		synctest.Wait()
		rt.send(1, "[]")
		synctest.Wait()
		require.True(t, b162Done(a) && b162Done(b), "both Stops after the poll loop exited")
		require.NoError(t, c.Start(ctx))
		b162Entered(t, rt, 2)
		rt.send(2, `[{"update_id":1}]`)
		synctest.Wait()
		id, ok := b162Got(ch)
		assert.True(t, ok && id == 1, "update 1 after the restart: got %d, %v", id, ok)
		b162Entered(t, rt, 3)
		s := b162Go(c.Stop)
		synctest.Wait()
		rt.send(3, "[]")
		synctest.Wait()
		assert.True(t, b162Done(s), "Stop after the restart")
		assert.Equal(t, 2, lg.count(b162StopMsg), "stop log lines")
	})
}

// R4e: Stop on a zero-value PollingClient (no stop channel) returns without a panic.
func TestB162R4StopZeroValue(t *testing.T) {
	var c receiver.PollingClient
	c.Stop()
	assert.False(t, c.Running())
}

// R6: the poll loop has seen the stop signal and is in its last log call (held by the log handler); Stop must
// still wait, and "long polling stopped" is not written yet.
func TestB162R6StopWaitsForLastLog(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		rt, lg := newB162RT(1), &b162Log{}
		lg.hold(b162LastLoopLog)
		defer lg.free()
		c, _ := b162Client(rt, lg)
		require.NoError(t, c.Start(ctx))
		b162Entered(t, rt, 1)
		s := b162Go(c.Stop)
		synctest.Wait()
		rt.send(1, "[]")
		synctest.Wait()
		require.True(t, b162Done(lg.atHold), "the poll loop did not reach its last log line")
		assert.False(t, b162Done(s), "Stop returned before the poll loop exited")
		assert.Equal(t, 0, lg.count(b162StopMsg), "stop log before the poll loop exited")
		lg.free()
		synctest.Wait()
		assert.True(t, b162Done(s), "Stop after the poll loop exited")
		assert.Equal(t, 1, lg.count(b162StopMsg), "stop log lines")
	})
}

// b162WaitReal waits up to d of real time for wg.
func b162WaitReal(wg *sync.WaitGroup, d time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

// S2a: 16 Stops at once on a client that was never started: no panic (close of a closed stop channel), all return.
func TestB162S2FanInNeverStarted(t *testing.T) {
	for trial := range 200 {
		c, _ := b162Client(newB162RT(0), &b162Log{})
		var wg sync.WaitGroup
		var panics atomic.Int32
		gate := make(chan struct{})
		for range 16 {
			wg.Go(func() {
				defer func() {
					if recover() != nil {
						panics.Add(1)
					}
				}()
				<-gate
				c.Stop()
			})
		}
		close(gate)
		if !b162WaitReal(&wg, 10*time.Second) {
			t.Fatalf("trial %d: the Stop calls did not return", trial)
		}
		if n := panics.Load(); n != 0 {
			t.Errorf("trial %d: %d Stop calls panicked", trial, n)
			return
		}
	}
}

// S2b: 16 Stops at once on a running client: all 16 return and exactly one logs "long polling stopped". It does not
// prove that each one waited for the loop (the request is released before the check): R1 and R2 prove that.
func TestB162S2FanInRunning(t *testing.T) {
	for trial := range 500 {
		ok := func() bool {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			rt, lg := newB162RT(1), &b162Log{}
			rt.fast = true
			c, _ := b162Client(rt, lg)
			require.NoError(t, c.Start(ctx))
			select {
			case <-rt.entered[0]:
			case <-time.After(10 * time.Second):
				t.Errorf("trial %d: getUpdates call 1 did not begin", trial)
				return false
			}
			var wg sync.WaitGroup
			gate := make(chan struct{})
			for range 16 {
				wg.Go(func() {
					<-gate
					c.Stop()
				})
			}
			close(gate)
			deadline := time.Now().Add(10 * time.Second)
			for c.Running() {
				if time.Now().After(deadline) {
					t.Errorf("trial %d: Running() stayed true", trial)
					return false
				}
				time.Sleep(time.Millisecond)
			}
			rt.send(1, "[]")
			if !b162WaitReal(&wg, 10*time.Second) {
				t.Errorf("trial %d: the Stop calls did not return", trial)
				return false
			}
			if n := lg.count(b162StopMsg); n != 1 {
				t.Errorf("trial %d: stop log lines: got %d, want 1", trial, n)
				return false
			}
			return true
		}()
		if !ok {
			return
		}
	}
}
