package receiver_test

// B-164: with DeliveryPolicyDropOldest the polling offset moves past an update only when that update is sent.
// After a drop the loop may leave on ctx end or Stop (the inner select chooses at random among ready cases, Go
// spec "Select statements"); then the offset must not move for the update that was not sent. The rows stage this
// through a drop hook, so the expected values do not depend on how many drops happen: capacity 1; after every drop
// the hook puts a new update (200, 201, ...) into the freed slot, so the send case is never ready while the exit is.
// The cancel and Stop rows run twice, with the reply 100 and a pending id equal to the offset after 100 is sent (101)
// or far above it (10000): 100 is sent (offset 101) and the pending update finds the buffer full. The cancel row
// cancels the Start context at the first drop; the Stop row holds the hook after the third drop until Stop has
// closed its channel; either way the pending update is never sent and the offset must stay 101. The deadline row
// runs from offset 0 (100 in the buffer, the reply [1]: offset must stay 0) and from offset 101 (the reply 100 and
// 10000); at the first drop the hook sleeps past the context deadline. The pending10000 and deadline rows set no
// OnUpdateDropped (the default config has none) and use the drop's Warn record as their hook; the pending101 rows use
// OnUpdateDropped. So every exit runs with and without a callback.

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
// the message text; if the key is renamed the hook never runs and b164Check says so first. Nearest existing:
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

// b164Pending: the two pending ids of the cancel and Stop rows: equal to the offset after 100 is sent, and far above it.
var b164Pending = []int{101, 10000}

// The Start context is cancelled at the first drop (the hook cancels it).
func TestB164OffsetAfterDropCancel(t *testing.T) {
	for _, pending := range b164Pending {
		t.Run("pending"+strconv.Itoa(pending), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r := b163NewRow(t, make(chan tg.Update, 1))
				ctx, cancel := context.WithCancel(context.Background())
				r.cancels = append(r.cancels, cancel)
				f := &b164Refill{r: r, cancel: cancel, viaLog: pending == 10000}
				p, rt := b164Start(t, r, ctx, f, 100, pending)
				synctest.Wait()
				assert.False(t, p.Running(), "Running after the Start context ended")
				b164Check(t, r, f, p, rt, 101)
			})
		})
	}
}

// The Start context passes its deadline at the first drop (the hook sleeps past it). From offset 0 (100 buffered, a
// pending id below every drop) and from offset 101. No OnUpdateDropped: the hook is the drop's Warn record.
func TestB164OffsetAfterDropDeadline(t *testing.T) {
	for _, tc := range []struct {
		name     string
		buffered bool
		ids      []int
		want     int64
	}{
		{"offset0", true, []int{1}, 0},
		{"offset101", false, []int{100, 10000}, 101},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ch := make(chan tg.Update, 1)
				if tc.buffered {
					ch <- tg.Update{UpdateID: 100}
				}
				r := b163NewRow(t, ch)
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				r.cancels = append(r.cancels, cancel)
				f := &b164Refill{r: r, sleep: 2 * time.Second, viaLog: true}
				p, rt := b164Start(t, r, ctx, f, tc.ids...)
				time.Sleep(3 * time.Second) // bubble time: the hook's sleep ends after the deadline
				synctest.Wait()
				assert.False(t, p.Running(), "Running after the Start context ended")
				b164Check(t, r, f, p, rt, tc.want)
			})
		})
	}
}

// Stop is called while the hook waits at the third drop; the hook goes on only after Stop has closed its channel and
// waits for the poll loop. Stop runs on the test side, not on the poll goroutine.
func TestB164OffsetAfterDropStop(t *testing.T) {
	for _, pending := range b164Pending {
		t.Run("pending"+strconv.Itoa(pending), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				r := b163NewRow(t, make(chan tg.Update, 1))
				ctx, cancel := context.WithCancel(context.Background())
				r.cancels = append(r.cancels, cancel)
				f := &b164Refill{r: r, holdAt: 3, release: make(chan struct{}), viaLog: pending == 10000}
				var once sync.Once
				free := func() { once.Do(func() { close(f.release) }) }
				r.cancels = append(r.cancels, free) // the B-163 cleanup calls it before Stop, so a failed row cannot leave the callback waiting
				p, rt := b164Start(t, r, ctx, f, 100, pending)
				synctest.Wait() // the hook waits at the third drop
				_, n := f.counts()
				require.Equal(t, 3, n, "refills before Stop")
				done := make(chan struct{})
				go func() { p.Stop(); close(done) }()
				synctest.Wait() // Stop has closed its channel and waits for the poll loop
				free()
				if r.returned(done, "Stop") {
					b164Check(t, r, f, p, rt, 101)
				}
			})
		})
	}
}
