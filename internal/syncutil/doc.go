// Package syncutil provides synchronization utilities for galigo.
//
// This package provides helper functions that complement the standard sync package,
// offering cleaner APIs for common concurrency patterns.
//
// # WaitGroup Helper
//
// The Go function provides a cleaner way to spawn goroutines tracked by a WaitGroup:
//
//	var wg sync.WaitGroup
//	syncutil.Go(&wg, func() {
//	    // work
//	})
//	wg.Wait()
//
// This is equivalent to:
//
//	var wg sync.WaitGroup
//	wg.Add(1)
//	go func() {
//	    defer wg.Done()
//	    // work
//	}()
//	wg.Wait()
package syncutil
