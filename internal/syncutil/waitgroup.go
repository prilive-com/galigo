// Package syncutil provides synchronization utilities.
package syncutil

import "sync"

// Go spawns a goroutine tracked by wg.
// Provides WaitGroup.Go() ergonomics without stdlib dependency.
//
// Usage:
//
//	var wg sync.WaitGroup
//	syncutil.Go(&wg, func() {
//	    // work
//	})
//	wg.Wait()
func Go(wg *sync.WaitGroup, fn func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		fn()
	}()
}
