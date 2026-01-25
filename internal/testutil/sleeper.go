package testutil

import (
	"context"
	"sync"
	"time"
)

// Sleeper abstracts time-based waiting for deterministic testing.
type Sleeper interface {
	Sleep(ctx context.Context, d time.Duration) error
}

// RealSleeper uses actual time (production).
type RealSleeper struct{}

// Sleep waits for the specified duration or until context is cancelled.
func (RealSleeper) Sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// FakeSleeper records sleep calls without actually sleeping.
// Use this in tests to verify retry timing without real delays.
type FakeSleeper struct {
	mu    sync.Mutex
	calls []time.Duration
}

// Sleep records the duration without actually sleeping.
// Returns ctx.Err() if the context is already cancelled.
func (f *FakeSleeper) Sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		f.mu.Lock()
		f.calls = append(f.calls, d)
		f.mu.Unlock()
		return nil
	}
}

// Calls returns all recorded sleep durations.
func (f *FakeSleeper) Calls() []time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Duration{}, f.calls...)
}

// TotalDuration returns the sum of all sleep durations.
func (f *FakeSleeper) TotalDuration() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	var total time.Duration
	for _, d := range f.calls {
		total += d
	}
	return total
}

// CallCount returns the number of sleep calls.
func (f *FakeSleeper) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// LastCall returns the most recent sleep duration.
// Returns 0 if no calls have been made.
func (f *FakeSleeper) LastCall() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return 0
	}
	return f.calls[len(f.calls)-1]
}

// CallAt returns the sleep duration at the given index.
// Returns 0 if index is out of bounds.
func (f *FakeSleeper) CallAt(index int) time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	if index < 0 || index >= len(f.calls) {
		return 0
	}
	return f.calls[index]
}

// Reset clears all recorded calls.
func (f *FakeSleeper) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = f.calls[:0]
}

// Verify interface compliance.
var (
	_ Sleeper = RealSleeper{}
	_ Sleeper = (*FakeSleeper)(nil)
)
