package time

import (
	"testing"
	"time"
)

func withRealAndFake(t *testing.T, f func(*testing.T)) {
	t.Run("real", func(t *testing.T) {
		f(t)
	})
	t.Run("fake", func(t *testing.T) {
		fkr := StartAcceleratedFake(1 * time.Millisecond)
		defer fkr.Stop()
		f(t)
	})
}
