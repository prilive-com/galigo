package syncutil_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prilive-com/galigo/internal/syncutil"
	"github.com/stretchr/testify/assert"
)

func TestGo_ExecutesFunction(t *testing.T) {
	var wg sync.WaitGroup
	var executed atomic.Bool

	syncutil.Go(&wg, func() {
		executed.Store(true)
	})

	wg.Wait()
	assert.True(t, executed.Load(), "function should have been executed")
}

func TestGo_TracksWaitGroup(t *testing.T) {
	var wg sync.WaitGroup
	counter := atomic.Int32{}

	// Launch multiple goroutines
	for i := 0; i < 10; i++ {
		syncutil.Go(&wg, func() {
			counter.Add(1)
			time.Sleep(10 * time.Millisecond)
		})
	}

	// Wait should block until all complete
	wg.Wait()
	assert.Equal(t, int32(10), counter.Load(), "all goroutines should have completed")
}

func TestGo_DoneCalledOnCompletion(t *testing.T) {
	var wg sync.WaitGroup
	done := make(chan struct{})

	syncutil.Go(&wg, func() {
		close(done)
	})

	// Wait should return after goroutine completes
	wg.Wait()

	// Verify goroutine actually ran
	select {
	case <-done:
		// Success
	default:
		t.Fatal("goroutine did not complete")
	}
}

func TestGo_ConcurrentUsage(t *testing.T) {
	var wg sync.WaitGroup
	results := make([]int, 100)

	for i := 0; i < 100; i++ {
		i := i // capture
		syncutil.Go(&wg, func() {
			results[i] = i * 2
		})
	}

	wg.Wait()

	for i := 0; i < 100; i++ {
		assert.Equal(t, i*2, results[i], "result mismatch at index %d", i)
	}
}
