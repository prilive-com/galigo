package receiver_test

// B-163: DeliveryPolicyDropOldest must not stall when the consumer empties the buffer while the deliverer waits,
// and at capacity 0 it must not take another sender's update as a "drop". Rows T1-T5 and T7 run in a synctest
// bubble (fake time; everything a row blocks on is made inside the bubble). Polling rows use an in-memory
// RoundTripper, so no socket is used.

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"strconv"
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

// The functions whose ctx.Done() calls b163Ctx counts (the direct caller of Done).
const (
	b163PollFn = "github.com/prilive-com/galigo/receiver.(*PollingClient).deliverDropOldest"
	b163HookFn = "github.com/prilive-com/galigo/receiver.(*WebhookHandler).webhookDeliverDropOldest"
)

// b163Ctx counts the Done() calls made directly by the two DropOldest functions. At 100 calls it cancels itself
// (spin guard: a deliverer that loops without blocking fails the row instead of hanging it). With drain set, the
// first such call takes one update from drain: this empties the buffer after the outer non-blocking send missed and
// before the inner select chooses (Done() is evaluated with the select's operands, before a case is chosen).
type b163Ctx struct {
	context.Context
	cancel  context.CancelFunc
	drain   chan tg.Update
	mu      sync.Mutex
	count   int
	spin    bool
	drained bool
	taken   []int
}

func (c *b163Ctx) Done() <-chan struct{} {
	pcs := make([]uintptr, 1)
	if runtime.Callers(2, pcs) == 1 {
		fr, _ := runtime.CallersFrames(pcs).Next()
		if fr.Function == b163PollFn || fr.Function == b163HookFn {
			c.mu.Lock()
			c.count++
			if c.count >= 100 && !c.spin {
				c.spin = true
				c.cancel()
			}
			if c.drain != nil && !c.drained {
				c.drained = true
				select {
				case u := <-c.drain:
					c.taken = append(c.taken, u.UpdateID)
				default:
				}
			}
			c.mu.Unlock()
		}
	}
	return c.Context.Done()
}

func (c *b163Ctx) stats() (count int, spin bool, taken []int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count, c.spin, slices.Clone(c.taken)
}

// b163RT answers getUpdates: body i to request i; every later request waits for its context to end. It records
// the offset of each request, so progress is read from the offset of the next request.
type b163RT struct {
	bodies []string
	mu     sync.Mutex
	offs   []string
}

func (rt *b163RT) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	i := len(rt.offs)
	rt.offs = append(rt.offs, req.URL.Query().Get("offset"))
	rt.mu.Unlock()
	if i >= len(rt.bodies) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Request: req,
		Body: io.NopCloser(strings.NewReader(rt.bodies[i]))}, nil
}

func (rt *b163RT) offsets() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return slices.Clone(rt.offs)
}

type b163Drops struct {
	mu sync.Mutex
	d  []string
}

func (d *b163Drops) add(id int, reason string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.d = append(d.d, strconv.Itoa(id)+":"+reason)
}

func (d *b163Drops) get() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.d)
}

// b163Row holds one row's state. Its cleanup runs inside the bubble on success and on failure: cancel every
// context, drain the channel, then Stop every poller and wait for every ServeHTTP, so no bubble goroutine is
// left blocked when the row returns.
type b163Row struct {
	t       *testing.T
	ch      chan tg.Update
	start   time.Time
	cancels []context.CancelFunc
	pollers []*receiver.PollingClient
	serving sync.WaitGroup
	drops   b163Drops
}

func b163NewRow(t *testing.T, ch chan tg.Update) *b163Row {
	r := &b163Row{t: t, ch: ch, start: time.Now()}
	t.Cleanup(func() {
		for _, cancel := range r.cancels {
			cancel()
		}
		stop := make(chan struct{})
		drained := make(chan struct{})
		go func() {
			defer close(drained)
			for {
				select {
				case <-r.ch:
				case <-stop:
					return
				}
			}
		}()
		for _, p := range r.pollers {
			p.Stop()
		}
		r.serving.Wait()
		close(stop)
		<-drained
	})
	return r
}

// ctx returns a counting context; timeout 0 means no deadline.
func (r *b163Row) ctx(timeout time.Duration, drain chan tg.Update) *b163Ctx {
	base, cancel := context.WithCancel(context.Background())
	if timeout > 0 {
		base, cancel = context.WithTimeout(context.Background(), timeout)
	}
	r.cancels = append(r.cancels, cancel)
	return &b163Ctx{Context: base, cancel: cancel, drain: drain}
}

func (r *b163Row) cfg(timeout time.Duration) receiver.Config {
	cfg := receiver.DefaultConfig()
	cfg.UpdateDeliveryPolicy = receiver.DeliveryPolicyDropOldest
	cfg.UpdateDeliveryTimeout = timeout
	cfg.PollingTimeout = 1
	cfg.BaseURL = "http://b163.invalid/bot"
	cfg.OnUpdateDropped = r.drops.add
	return cfg
}

// poll starts a poller on ctx whose first getUpdates returns the given update ids.
func (r *b163Row) poll(ctx context.Context, timeout time.Duration, ids ...int) (*receiver.PollingClient, *b163RT) {
	r.t.Helper()
	rt := &b163RT{bodies: []string{b163Body(ids...)}}
	p := receiver.NewPollingClient(tg.SecretToken("123:fake"), r.ch, slog.New(slog.DiscardHandler), r.cfg(timeout),
		receiver.WithPollingHTTPClient(&http.Client{Transport: rt}))
	r.pollers = append(r.pollers, p)
	require.NoError(r.t, p.Start(ctx))
	return p, rt
}

type b163Served struct {
	done chan struct{}
	rec  *httptest.ResponseRecorder
	at   time.Duration // bubble time when ServeHTTP returned
}

// serve runs ServeHTTP for one update in its own goroutine.
func (r *b163Row) serve(h *receiver.WebhookHandler, ctx context.Context, id int) *b163Served {
	s := &b163Served{done: make(chan struct{}), rec: httptest.NewRecorder()}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"update_id":`+strconv.Itoa(id)+`}`)).
		WithContext(ctx)
	r.serving.Add(1)
	go func() {
		defer r.serving.Done()
		h.ServeHTTP(s.rec, req)
		s.at = time.Since(r.start)
		close(s.done)
	}()
	return s
}

func (r *b163Row) handler(timeout time.Duration) *receiver.WebhookHandler {
	return receiver.NewWebhookHandler(slog.New(slog.DiscardHandler), r.ch, r.cfg(timeout))
}

// returned waits a bounded time for done, so a mutant that never returns fails the row with a named value.
func (r *b163Row) returned(done <-chan struct{}, what string) bool {
	select {
	case <-done:
		return true
	case <-time.After(20 * time.Second):
		r.t.Errorf("%s did not return within 20 s", what)
		return false
	}
}

func (r *b163Row) waiting(s *b163Served) bool {
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

// b163Read is the late reader: it waits 10 s of bubble time, then gives -1.
func b163Read(ch chan tg.Update) int {
	select {
	case u := <-ch:
		return u.UpdateID
	case <-time.After(10 * time.Second):
		return -1
	}
}

func b163Body(ids ...int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = `{"update_id":` + strconv.Itoa(id) + `}`
	}
	return `{"ok":true,"result":[` + strings.Join(parts, ",") + `]}`
}

// b163NoSpin: with the fix the inner select evaluates Done() once and blocks.
func b163NoSpin(t *testing.T, c *b163Ctx) {
	t.Helper()
	count, spin, _ := c.stats()
	assert.False(t, spin, "spin guard fired")
	assert.True(t, 1 <= count && count <= 2, "Done() calls from the deliverer = %d, want 1 or 2", count)
}

// T1: capacity 0, the reader comes after UpdateDeliveryTimeout. DropOldest waits for it (no drop) and delivers as
// soon as it reads.
func TestB163T1LateReaderPolling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := b163NewRow(t, make(chan tg.Update))
		ctx := r.ctx(0, nil)
		_, rt := r.poll(ctx, 10*time.Millisecond, 1)
		synctest.Wait()
		time.Sleep(20 * time.Millisecond)
		synctest.Wait()
		assert.Equal(t, []string{"0"}, rt.offsets(), "requests before the reader")
		assert.Empty(t, r.drops.get(), "drops before the reader")

		assert.Equal(t, 1, b163Read(r.ch), "first read")
		assert.Equal(t, 20*time.Millisecond, time.Since(r.start), "bubble time of the first read")
		assert.Equal(t, -1, b163Read(r.ch), "second read (no duplicate)")
		assert.Equal(t, []string{"0", "2"}, rt.offsets(), "next request offset")
		assert.Empty(t, r.drops.get(), "drops")
		b163NoSpin(t, ctx)
	})
}

func TestB163T1LateReaderWebhook(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := b163NewRow(t, make(chan tg.Update))
		ctx := r.ctx(10*time.Second, nil)
		s := r.serve(r.handler(10*time.Millisecond), ctx, 9)
		synctest.Wait()
		time.Sleep(20 * time.Millisecond)
		synctest.Wait()
		assert.True(t, r.waiting(s), "handler returned before the reader")
		assert.Empty(t, r.drops.get(), "drops before the reader")

		assert.Equal(t, 9, b163Read(r.ch), "read")
		if r.returned(s.done, "ServeHTTP") {
			assert.Equal(t, 20*time.Millisecond, s.at, "bubble time of the handler return")
			assert.Equal(t, http.StatusOK, s.rec.Code)
		}
		assert.Empty(t, r.drops.get(), "drops")
		b163NoSpin(t, ctx)
	})
}

// T2: capacity 1, the buffer is full, and the consumer empties it after the outer send missed and before the inner
// select chooses (b163Ctx drain). The new update is delivered at once, nothing is dropped.
func TestB163T2BufferEmptiedInWindowPolling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ch := make(chan tg.Update, 1)
		ch <- tg.Update{UpdateID: 100}
		r := b163NewRow(t, ch)
		ctx := r.ctx(10*time.Second, ch)
		_, rt := r.poll(ctx, 0, 1)
		synctest.Wait()
		require.Equal(t, time.Duration(0), time.Since(r.start), "bubble time")
		assert.Equal(t, 1, len(ch), "updates in the channel at bubble time 0")
		_, _, taken := ctx.stats()
		assert.Equal(t, []int{100}, taken, "taken by the consumer in the window")
		assert.Equal(t, []string{"0", "2"}, rt.offsets(), "next request offset")
		assert.Empty(t, r.drops.get(), "drops")
		if len(ch) == 1 {
			assert.Equal(t, 1, (<-ch).UpdateID, "update in the channel")
		}
	})
}

func TestB163T2BufferEmptiedInWindowWebhook(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ch := make(chan tg.Update, 1)
		ch <- tg.Update{UpdateID: 100}
		r := b163NewRow(t, ch)
		ctx := r.ctx(10*time.Second, ch)
		s := r.serve(r.handler(0), ctx, 9)
		if r.returned(s.done, "ServeHTTP") {
			assert.Equal(t, time.Duration(0), s.at, "bubble time of the handler return")
			assert.Equal(t, http.StatusOK, s.rec.Code)
		}
		_, _, taken := ctx.stats()
		assert.Equal(t, []int{100}, taken, "taken by the consumer in the window")
		assert.Empty(t, r.drops.get(), "drops")
		if assert.Equal(t, 1, len(ch), "updates in the channel") {
			assert.Equal(t, 9, (<-ch).UpdateID, "update in the channel")
		}
	})
}

// T3: capacity 0, a second sender on the same channel. The waiting deliverer must not receive the other sender's
// update and report it as a drop; both updates reach the reader.
func TestB163T3SecondSenderPolling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := b163NewRow(t, make(chan tg.Update))
		ctx := r.ctx(0, nil)
		_, rt := r.poll(ctx, 0, 1)
		synctest.Wait()
		go func() { r.ch <- tg.Update{UpdateID: 500} }() // the user owns the channel and may send to it
		synctest.Wait()
		assert.Empty(t, r.drops.get(), "drops before the reader")

		got := []int{b163Read(r.ch), b163Read(r.ch)}
		slices.Sort(got)
		assert.Equal(t, []int{1, 500}, got, "read")
		synctest.Wait()
		assert.Equal(t, []string{"0", "2"}, rt.offsets(), "next request offset")
		assert.Empty(t, r.drops.get(), "drops")
		b163NoSpin(t, ctx)
	})
}

func TestB163T3SecondSenderWebhook(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := b163NewRow(t, make(chan tg.Update))
		h := r.handler(0)
		c1, c2 := r.ctx(10*time.Second, nil), r.ctx(10*time.Second, nil)
		s1 := r.serve(h, c1, 1)
		synctest.Wait()
		s2 := r.serve(h, c2, 2)
		synctest.Wait()
		assert.Empty(t, r.drops.get(), "drops before the reader")

		got := []int{b163Read(r.ch), b163Read(r.ch)}
		slices.Sort(got)
		assert.Equal(t, []int{1, 2}, got, "read")
		for i, s := range []*b163Served{s1, s2} {
			if r.returned(s.done, "ServeHTTP "+strconv.Itoa(i+1)) {
				assert.Equal(t, http.StatusOK, s.rec.Code)
			}
		}
		assert.Empty(t, r.drops.get(), "drops")
		b163NoSpin(t, c1)
		b163NoSpin(t, c2)
	})
}

// T4: capacity 1, the buffer stays full (no reader). DropOldest drops exactly the oldest update, as before.
func TestB163T4BufferStaysFullPolling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ch := make(chan tg.Update, 1)
		ch <- tg.Update{UpdateID: 100}
		r := b163NewRow(t, ch)
		_, rt := r.poll(r.ctx(0, nil), 0, 1)
		synctest.Wait()
		assert.Equal(t, []string{"100:channel_full_drop_oldest"}, r.drops.get(), "drops")
		assert.Equal(t, []string{"0", "2"}, rt.offsets(), "next request offset")
		if assert.Equal(t, 1, len(ch), "updates in the channel") {
			assert.Equal(t, 1, (<-ch).UpdateID, "update in the channel")
		}
	})
}

func TestB163T4BufferStaysFullWebhook(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ch := make(chan tg.Update, 1)
		ch <- tg.Update{UpdateID: 100}
		r := b163NewRow(t, ch)
		s := r.serve(r.handler(0), r.ctx(0, nil), 9)
		if r.returned(s.done, "ServeHTTP") {
			assert.Equal(t, http.StatusOK, s.rec.Code)
		}
		assert.Equal(t, []string{"100:webhook_channel_full_drop_oldest"}, r.drops.get(), "drops")
		if assert.Equal(t, 1, len(ch), "updates in the channel") {
			assert.Equal(t, 9, (<-ch).UpdateID, "update in the channel")
		}
	})
}

// T5: capacity 0, no reader: the deliverer leaves on ctx end or Stop, drops nothing and delivers nothing.
func TestB163T5ContextEndPolling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := b163NewRow(t, make(chan tg.Update))
		ctx := r.ctx(0, nil)
		p, rt := r.poll(ctx, 0, 1)
		synctest.Wait()
		ctx.cancel()
		synctest.Wait()
		assert.False(t, p.Running(), "Running after the Start context ended")
		assert.Empty(t, r.drops.get(), "drops")
		assert.Equal(t, []string{"0"}, rt.offsets(), "requests")
		assert.EqualValues(t, 0, p.Offset(), "Offset")
		b163NoSpin(t, ctx)
	})
}

func TestB163T5StopPolling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := b163NewRow(t, make(chan tg.Update))
		ctx := r.ctx(0, nil)
		p, rt := r.poll(ctx, 0, 1)
		synctest.Wait()
		done := make(chan struct{})
		var at time.Duration
		go func() { p.Stop(); at = time.Since(r.start); close(done) }()
		if r.returned(done, "Stop") {
			assert.Equal(t, time.Duration(0), at, "bubble time of the Stop return")
		}
		assert.Empty(t, r.drops.get(), "drops")
		assert.Equal(t, []string{"0"}, rt.offsets(), "requests")
		assert.EqualValues(t, 0, p.Offset(), "Offset")
		b163NoSpin(t, ctx)
	})
}

func TestB163T5ContextEndWebhook(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := b163NewRow(t, make(chan tg.Update))
		ctx := r.ctx(10*time.Second, nil)
		s := r.serve(r.handler(0), ctx, 9)
		if r.returned(s.done, "ServeHTTP") {
			assert.Equal(t, 10*time.Second, s.at, "bubble time of the handler return")
			assert.Equal(t, http.StatusOK, s.rec.Code)
		}
		assert.Empty(t, r.drops.get(), "drops")
		b163NoSpin(t, ctx)
	})
}

// T7: capacity 2 with one item: the outer non-blocking send must deliver without a drop. A single blocking select
// with send and drop cases would drop with probability 1/2 in each trial.
func TestB163T7FreeSlotPolling(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for trial := range 20 {
			ch := make(chan tg.Update, 2)
			ch <- tg.Update{UpdateID: 100}
			r := &b163Row{t: t, ch: ch, start: time.Now()}
			ctx, cancel := context.WithCancel(context.Background())
			p, rt := r.poll(ctx, 0, 1)
			synctest.Wait()
			assert.Empty(t, r.drops.get(), "trial %d: drops", trial)
			assert.Equal(t, 2, len(ch), "trial %d: updates in the channel", trial)
			assert.Equal(t, []string{"0", "2"}, rt.offsets(), "trial %d: next request offset", trial)
			cancel()
			p.Stop()
		}
	})
}

func TestB163T7FreeSlotWebhook(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for trial := range 20 {
			ch := make(chan tg.Update, 2)
			ch <- tg.Update{UpdateID: 100}
			r := &b163Row{t: t, ch: ch, start: time.Now()}
			ctx, cancel := context.WithCancel(context.Background())
			s := r.serve(r.handler(0), ctx, 9)
			if r.returned(s.done, "ServeHTTP") {
				assert.Equal(t, time.Duration(0), s.at, "trial %d: bubble time of the handler return", trial)
				assert.Equal(t, http.StatusOK, s.rec.Code, "trial %d", trial)
			}
			assert.Empty(t, r.drops.get(), "trial %d: drops", trial)
			assert.Equal(t, 2, len(ch), "trial %d: updates in the channel", trial)
			cancel()
			r.serving.Wait()
		}
	})
}
