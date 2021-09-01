package daemon

import (
	"math/rand"
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

// generateCorrelationID generates a random 4-digit integer for use as a correlation ID
func generateCorrelationID() int {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return r.Intn(10000)
}
