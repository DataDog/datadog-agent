package time

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTicker(t *testing.T) {
	withRealAndFake(t, func(t *testing.T) {
		ticker, tickerC := NewTicker(10 * Millisecond)
		defer ticker.Stop()
		done := make(chan bool)
		go func() {
			Sleep(500 * Millisecond)
			done <- true
		}()
		ticks := 0
		for {
			select {
			case <-tickerC:
				ticks++
			case <-done:
				// ticks should be ~50, but at least once..
				require.NotEqual(t, 0, ticks)
				return
			}
		}
	})
}

func TestTickerReset(t *testing.T) {
	withRealAndFake(t, func(t *testing.T) {
		dur := 1 * Millisecond

		ticker, tickerC := NewTicker(dur)
		defer ticker.Stop()

		start := Now()

		ticks := 0
		for range tickerC {
			ticks++
			dur = dur * 2
			if dur > 32*Millisecond {
				break
			}
			ticker.Reset(dur)
		}

		elapsed := Since(start)
		// if Reset did nothing, then we spent about ticks ms
		// in the loop above; otherwise we spent about 63ms
		require.Less(t, Duration(ticks)*Millisecond, elapsed)
	})
}
