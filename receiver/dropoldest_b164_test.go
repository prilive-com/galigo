package receiver_test

// B-164: with DeliveryPolicyDropOldest, deliverDropOldest moves the polling offset past an update only when it sends
// that update (deliverUpdates also moves it past an update that does not decode, B-135; not staged here).
// After a drop the loop may leave on ctx end or Stop (the inner select chooses at random among ready cases, Go
// spec "Select statements"); then the offset must not move for the update that was not sent. The rows stage this
// through a drop hook, so the expected values do not depend on how many drops happen: capacity 1; after every drop
// the hook puts a new update (200, 201, ...) into the freed slot, so the send case is never ready while the exit is.
// Each exit runs from offset 0 (100 in the buffer, the reply [1]: offset must stay 0) and from offset 101 (the reply
// 100, which is sent, then a pending id far above the offset (10000) or equal to it (101); the pending update finds
// the buffer full and the offset must stay 101). The cancel rows cancel the Start context at the first drop; the
// deadline rows sleep in the hook past the context deadline at the first drop; the Stop rows hold the hook after the
// third drop until Stop has closed its channel. The deadline and Stop rows also read the offset while the hook holds
// (the sleep, the wait at drop 3): a move made before the hold and undone at the exit is caught there. Every case runs
// twice: with OnUpdateDropped as the hook (subtest "callback") and with no OnUpdateDropped (the default config has
// none) and the drop's Warn record as the hook (subtest "log").

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/receiver"
	"github.com/prilive-com/galigo/tg"
)

// b164Limit bounds the refills, as a guard for a wrong version that keeps calling the drop hook without leaving (it
// then sends the pending update and fails, instead of spinning forever: bubble time does not move while a goroutine
// runs). With the correct code a drop without a refill (drop 1001) needs 1000 drop picks in a row once the exit is
// ready from drop 2 (cancel and deadline rows) or 998 from drop 4 (Stop row): estimated chances 2^-1000 and 2^-998 under
// a model of independent 50/50 picks, not a Go guarantee.
const b164Limit = 1000

// b164Refill is the drop hook: OnUpdateDropped, or (viaLog) a log handler on the drop's Warn record; both run on the
// poll goroutine after the drop freed the slot (receiver/polling.go). Each call records the drop and refills the freed
// slot (non-blocking: only the poll goroutine uses the channel during a row). At drop holdAt it waits on release; it
// never waits for Stop (OnUpdateDropped must not, receiver/config.go).
type b164Refill struct {
	r       *b163Row
	cancel  context.CancelFunc // cancel row: called at the first drop
	sleep   time.Duration      // deadline row: sleep this long at the first drop (bubble time)
	holdAt  int                // Stop row: wait on release at this drop (0 = never)
	viaLog  bool               // no OnUpdateDropped; the hook is b164LogHook
	release chan struct{}
	mu      sync.Mutex
	n       int // drops seen
	next    int // refills made
}

func (f *b164Refill) dropped(id int, reason string) {
	f.r.drops.add(id, reason)
	f.mu.Lock()
	f.n++
	n := f.n
	if f.next < b164Limit {
		select {
		case f.r.ch <- tg.Update{UpdateID: 200 + f.next}:
			f.next++
		default:
		}
	}
	f.mu.Unlock()
	if n == 1 && f.cancel != nil {
		f.cancel()
	}
	if n == 1 && f.sleep > 0 {
		time.Sleep(f.sleep)
	}
	if n == f.holdAt {
		<-f.release
	}
}

func (f *b164Refill) counts() (drops, refills int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n, f.next
}

// b164LogHook calls f.dropped for each Warn record with a dropped_id attribute (the drop record in deliverDropOldest,
// receiver/polling.go), with that id and the reason OnUpdateDropped would get. It matches the level and the key, not
// the message text; if the key is renamed the hook never runs and every "log" row fails (the deadline rows first with
// b164Check's "drops seen by the hook: none"; the cancel and Stop rows first with their own checks). Nearest existing:
// b162Log (stop_b162_test.go), which records and holds on a message; it has no refill, so it is not reused.
type b164LogHook struct{ f *b164Refill }

func (h b164LogHook) Enabled(context.Context, slog.Level) bool { return true }
func (h b164LogHook) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h b164LogHook) WithGroup(string) slog.Handler            { return h }
func (h b164LogHook) Handle(_ context.Context, rec slog.Record) error {
	if rec.Level != slog.LevelWarn {
		return nil
	}
	rec.Attrs(func(a slog.Attr) bool {
		if a.Key == "dropped_id" {
			h.f.dropped(int(a.Value.Int64()), "channel_full_drop_oldest")
			return false
		}
		return true
	})
	return nil
}

// b164Start starts a poller whose first getUpdates returns ids, with f as the drop hook.
func b164Start(t *testing.T, r *b163Row, ctx context.Context, f *b164Refill, ids ...int) (*receiver.PollingClient, *b163RT) {
	t.Helper()
	cfg := r.cfg(0)
	logger := slog.New(slog.DiscardHandler)
	if f.viaLog {
		cfg.OnUpdateDropped = nil
		logger = slog.New(b164LogHook{f})
	} else {
		cfg.OnUpdateDropped = f.dropped
	}
	rt := &b163RT{bodies: []string{b163Body(ids...)}}
	p := receiver.NewPollingClient(tg.SecretToken("123:fake"), r.ch, logger, cfg,
		receiver.WithPollingHTTPClient(&http.Client{Transport: rt}))
	r.pollers = append(r.pollers, p)
	require.NoError(t, p.Start(ctx))
	return p, rt
}

// b164Check: the pending update was never sent. At least one drop was seen (the hook ran), every drop was refilled,
// the offset is still want, there was one getUpdates request with offset 0, the drops are 100 and then every refill
// but the last, and the channel holds the last refill.
func b164Check(t *testing.T, r *b163Row, f *b164Refill, p *receiver.PollingClient, rt *b163RT, want int64) {
	t.Helper()
	drops, n := f.counts()
	require.Positive(t, drops, "drops seen by the hook: none (the staging did not run)")
	assert.Equal(t, drops, n, "drops != refills: not every drop was refilled")
	assert.Equal(t, want, p.Offset(), "polling offset after the staged exit (it may move only for a sent update; see the channel check)")
	assert.Equal(t, []string{"0"}, rt.offsets(), "offsets of the getUpdates requests: one request, offset 0")
	wantDrops := []string{"100:channel_full_drop_oldest"}
	for i := range n - 1 {
		wantDrops = append(wantDrops, strconv.Itoa(200+i)+":channel_full_drop_oldest")
	}
	assert.Equal(t, wantDrops, r.drops.get(), "drops: the first buffered update, then every refill but the last")
	if assert.Equal(t, 1, len(r.ch), "updates in the channel") {
		assert.Equal(t, 200+n-1, (<-r.ch).UpdateID, "update in the channel: the last refill, not the pending update")
	}
}

// b164Case: a start state, the getUpdates reply and the offset that must hold after the exit.
type b164Case struct {
	name     string
	buffered bool // the channel holds update 100 at the start
	ids      []int
	want     int64
}

// b164Cases: the cases of the cancel and Stop rows (the deadline rows add offset101eq).
var b164Cases = []b164Case{
	{"pending101", false, []int{100, 101}, 101}, // pending id equal to the offset after 100 is sent
	{"pending10000", false, []int{100, 10000}, 101},
	{"offset0", true, []int{1}, 0},
}

// b164Chan: the capacity-1 channel of a row, holding update 100 when the case says so.
func b164Chan(tc b164Case) chan tg.Update {
	ch := make(chan tg.Update, 1)
	if tc.buffered {
		ch <- tg.Update{UpdateID: 100}
	}
	return ch
}

// b164Hooks: the two drop hooks every case runs with (b164Refill.viaLog).
var b164Hooks = []struct {
	name   string
	viaLog bool
}{{"callback", false}, {"log", true}}

// The Start context is cancelled at the first drop (the hook cancels it).
func TestB164OffsetAfterDropCancel(t *testing.T) {
	for _, tc := range b164Cases {
		for _, h := range b164Hooks {
			t.Run(tc.name+"/"+h.name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					r := b163NewRow(t, b164Chan(tc))
					ctx, cancel := context.WithCancel(context.Background())
					r.cancels = append(r.cancels, cancel)
					f := &b164Refill{r: r, cancel: cancel, viaLog: h.viaLog}
					p, rt := b164Start(t, r, ctx, f, tc.ids...)
					synctest.Wait()
					assert.False(t, p.Running(), "Running after the Start context ended")
					b164Check(t, r, f, p, rt, tc.want)
				})
			})
		}
	}
}

// The Start context passes its deadline at the first drop (the hook sleeps past it). From offset 0 (100 buffered, a
// pending id below every drop) and from offset 101 (a pending id far above it, and one equal to it). The offset is read
// also while the hook sleeps.
func TestB164OffsetAfterDropDeadline(t *testing.T) {
	for _, tc := range []b164Case{
		{"offset0", true, []int{1}, 0},
		{"offset101", false, []int{100, 10000}, 101},
		{"offset101eq", false, []int{100, 101}, 101}, // pending id equal to the offset
	} {
		for _, h := range b164Hooks {
			t.Run(tc.name+"/"+h.name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					r := b163NewRow(t, b164Chan(tc))
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					r.cancels = append(r.cancels, cancel)
					f := &b164Refill{r: r, sleep: 2 * time.Second, viaLog: h.viaLog}
					p, rt := b164Start(t, r, ctx, f, tc.ids...)
					synctest.Wait() // the hook sleeps at the first drop
					assert.Equal(t, tc.want, p.Offset(), "polling offset while the hook sleeps at the first drop (before the deadline)")
					time.Sleep(3 * time.Second) // bubble time: the hook's sleep ends after the deadline
					synctest.Wait()
					assert.False(t, p.Running(), "Running after the Start context ended")
					b164Check(t, r, f, p, rt, tc.want)
				})
			})
		}
	}
}

// Stop is called while the hook waits at the third drop; the hook goes on only after Stop has closed its channel and
// waits for the poll loop. Stop runs on the test side, not on the poll goroutine.
func TestB164OffsetAfterDropStop(t *testing.T) {
	for _, tc := range b164Cases {
		for _, h := range b164Hooks {
			t.Run(tc.name+"/"+h.name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					r := b163NewRow(t, b164Chan(tc))
					ctx, cancel := context.WithCancel(context.Background())
					r.cancels = append(r.cancels, cancel)
					f := &b164Refill{r: r, holdAt: 3, release: make(chan struct{}), viaLog: h.viaLog}
					var once sync.Once
					free := func() { once.Do(func() { close(f.release) }) }
					r.cancels = append(r.cancels, free) // the B-163 cleanup calls it before Stop, so a failed row cannot leave the hook waiting
					p, rt := b164Start(t, r, ctx, f, tc.ids...)
					synctest.Wait() // the hook waits at the third drop
					_, n := f.counts()
					require.Equal(t, 3, n, "refills before Stop")
					assert.Equal(t, tc.want, p.Offset(), "polling offset while the hook holds the third drop (before Stop)")
					done := make(chan struct{})
					go func() { p.Stop(); close(done) }()
					synctest.Wait() // Stop has closed its channel and waits for the poll loop
					free()
					if r.returned(done, "Stop") {
						b164Check(t, r, f, p, rt, tc.want)
					}
				})
			})
		}
	}
}
