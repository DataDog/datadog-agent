package daemon

import (
	"sync"
	"time"
)

// waitWithTimeout waits for a WaitGroup with a specified max timeout.
// Returns true if waiting timed out.
func waitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return false // completed normally
	case <-time.After(timeout):
		return true // timed out
	}
}
