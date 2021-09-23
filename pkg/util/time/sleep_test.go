package time

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSleep(t *testing.T) {
	withRealAndFake(t, func(t *testing.T) {
		start := Now()
		Sleep(10 * Millisecond)
		end := Now()
		require.LessOrEqual(t, 5*Millisecond, end.Sub(start))
	})
}

func TestTimer(t *testing.T) {
	withRealAndFake(t, func(t *testing.T) {
		_, c := NewTimer(30 * Millisecond)

		select {
		case <-c:
			require.Fail(t, "should not have triggered yet")
		default:
		}

		Sleep(40 * Millisecond)

		select {
		case <-c:
		default:
			require.Fail(t, "should have triggered already")
		}
	})
}

func TestTimerStop(t *testing.T) {
	withRealAndFake(t, func(t *testing.T) {
		tmr, c := NewTimer(30 * Millisecond)

		select {
		case <-c:
			require.Fail(t, "should not have triggered yet")
		default:
		}

		stopped := tmr.Stop()
		require.True(t, stopped)
		Sleep(40 * Millisecond)

		select {
		case <-c:
			require.Fail(t, "should not have triggered")
		default:
		}

		stopped = tmr.Stop()
		require.False(t, stopped)
	})
}

func TestTimerReset(t *testing.T) {
	withRealAndFake(t, func(t *testing.T) {
		tmr, c := NewTimer(30 * Millisecond)

		select {
		case <-c:
			require.Fail(t, "should not have triggered yet")
		default:
		}

		tmr.Reset(60 * Millisecond)
		Sleep(40 * Millisecond)

		select {
		case <-c:
			require.Fail(t, "still should not have triggered")
		default:
		}

		Sleep(40 * Millisecond)

		select {
		case <-c:
		default:
			require.Fail(t, "should have triggered")
		}
	})
}

func TestAfter(t *testing.T) {
	withRealAndFake(t, func(t *testing.T) {
		c := After(30 * Millisecond)

		select {
		case <-c:
			require.Fail(t, "should not have triggered yet")
		default:
		}

		Sleep(40 * Millisecond)

		select {
		case <-c:
		default:
			require.Fail(t, "should have triggered already")
		}
	})
}

func TestAfterFunc(t *testing.T) {
	withRealAndFake(t, func(t *testing.T) {
		var triggered int32

		AfterFunc(
			30*Millisecond,
			func() { atomic.StoreInt32(&triggered, 1) },
		)

		Sleep(1 * Millisecond)

		require.Equal(t, int32(0), atomic.LoadInt32(&triggered))

		Sleep(40 * Millisecond)

		require.Equal(t, int32(1), atomic.LoadInt32(&triggered))
	})
}

func TestAfterFuncReset(t *testing.T) {
	withRealAndFake(t, func(t *testing.T) {
		var triggered int32

		tmr, _ := AfterFunc(
			30*Millisecond,
			func() {
				atomic.StoreInt32(&triggered, 1)
			},
		)

		Sleep(1 * Millisecond)

		require.Equal(t, int32(0), atomic.LoadInt32(&triggered))
		tmr.Reset(80 * Millisecond)

		Sleep(40 * Millisecond)

		require.Equal(t, int32(0), atomic.LoadInt32(&triggered))

		Sleep(60 * Millisecond)

		require.Equal(t, int32(1), atomic.LoadInt32(&triggered))
	})
}

func TestAfterFuncStop(t *testing.T) {
	withRealAndFake(t, func(t *testing.T) {
		var triggered int32

		timer, _ := AfterFunc(
			30*Millisecond,
			func() { atomic.StoreInt32(&triggered, 1) },
		)

		Sleep(1 * Millisecond)

		require.Equal(t, int32(0), atomic.LoadInt32(&triggered))

		stopped := timer.Stop()
		require.True(t, stopped)
		Sleep(40 * Millisecond)

		require.Equal(t, int32(0), atomic.LoadInt32(&triggered))

		stopped = timer.Stop()
		require.False(t, stopped)
	})
}
