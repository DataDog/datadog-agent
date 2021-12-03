package tagset

import (
	"testing"
	"time"
)

// fuzz implements poor-soul's attempt at fuzzing. The idea is to catch edge
// cases by running a bunch of random, but deterministic (the same on every
// run), scenarios. In "-short" mode it runs for about 100ms; otherwise about
// 1s.
func fuzz(t *testing.T, test func(int64)) {
	finish := time.Now().Add(1 * time.Second)
	if testing.Short() {
		finish = time.Now().Add(100 * time.Millisecond)
	}
	var i int64
	for time.Now().Before(finish) {
		defer func() {
			if t.Failed() {
				t.Errorf("Fuzzing failed with random seed: %d\n", i)
			}
		}()
		test(i)
		i++
	}
}
