// +build !race

package daemon

import (
	"sync"
	"testing"
	"time"

	"gotest.tools/assert"
)

func TestWaitWithTimeoutTimesOut(t *testing.T) {
	wg := sync.WaitGroup{}
	wg.Add(1)
	// this will time out as wg.Done() is never called
	result := waitWithTimeout(&wg, 1*time.Millisecond)
	assert.Equal(t, result, true)
}

func TestWaitWithTimeoutCompletesNormally(t *testing.T) {
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		wg.Done()
	}()
	result := waitWithTimeout(&wg, 250*time.Millisecond)
	assert.Equal(t, result, false)
}
