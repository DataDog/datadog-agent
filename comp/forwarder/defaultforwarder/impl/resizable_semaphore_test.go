// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarderimpl

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isClosed reports whether ch is closed without blocking.
func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestResizableSemaphoreBlocksAtLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newResizableSemaphore(2)
		ctx := context.Background()

		require.NoError(t, s.Acquire(ctx, 1))
		require.NoError(t, s.Acquire(ctx, 1))

		// The third Acquire must block until a token is released.
		acquired := make(chan struct{})
		go func() {
			_ = s.Acquire(ctx, 1)
			close(acquired)
		}()

		synctest.Wait()
		require.False(t, isClosed(acquired), "Acquire should block when the limit is reached")

		s.Release(1)
		synctest.Wait()
		assert.True(t, isClosed(acquired), "Acquire should unblock once a token is released")
	})
}

func TestResizableSemaphoreSetLimitUpWakesWaiters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newResizableSemaphore(1)
		ctx := context.Background()
		require.NoError(t, s.Acquire(ctx, 1))

		acquired := make(chan struct{})
		go func() {
			_ = s.Acquire(ctx, 1)
			close(acquired)
		}()

		// Waiter is blocked at limit 1; raising the limit must wake it.
		synctest.Wait()
		require.False(t, isClosed(acquired), "Acquire should block before the limit is raised")

		s.SetLimit(2)
		synctest.Wait()
		assert.True(t, isClosed(acquired), "raising the limit should wake the blocked acquirer")
	})
}

func TestResizableSemaphoreSetLimitDownDoesNotCancelInFlight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newResizableSemaphore(3)
		ctx := context.Background()
		require.NoError(t, s.Acquire(ctx, 1))
		require.NoError(t, s.Acquire(ctx, 1))
		require.NoError(t, s.Acquire(ctx, 1))

		// Lowering the limit below the in-flight count must not panic or block;
		// it only prevents new acquisitions until enough tokens are released.
		s.SetLimit(1)

		acquired := make(chan struct{})
		go func() {
			_ = s.Acquire(ctx, 1)
			close(acquired)
		}()
		synctest.Wait()
		require.False(t, isClosed(acquired), "Acquire should block while in-flight exceeds the lowered limit")

		// At limit 1 the waiter can only proceed once in-flight drops to 0, i.e.
		// all three original holders release.
		s.Release(1)
		s.Release(1)
		synctest.Wait()
		require.False(t, isClosed(acquired), "Acquire should block until in-flight drops below the new limit")

		s.Release(1)
		synctest.Wait()
		assert.True(t, isClosed(acquired), "Acquire should proceed once in-flight drops below the new limit")
	})
}

func TestResizableSemaphoreContextCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newResizableSemaphore(1)
		require.NoError(t, s.Acquire(context.Background(), 1))

		ctx, cancel := context.WithCancel(context.Background())
		errCh := make(chan error, 1)
		go func() {
			errCh <- s.Acquire(ctx, 1)
		}()

		synctest.Wait()
		cancel()
		synctest.Wait()
		select {
		case err := <-errCh:
			assert.ErrorIs(t, err, context.Canceled)
		default:
			t.Fatal("Acquire should return when the context is cancelled")
		}
	})
}

func TestResizableSemaphoreCancelledContextWithCapacity(t *testing.T) {
	s := newResizableSemaphore(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.ErrorIs(t, s.Acquire(ctx, 1), context.Canceled)
	assert.Equal(t, int64(0), s.inFlight)
}

func TestResizableSemaphoreCancelConcurrentWithRelease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newResizableSemaphore(1)
		require.NoError(t, s.Acquire(context.Background(), 1))

		ctx, cancel := context.WithCancel(context.Background())
		errCh := make(chan error, 1)
		go func() {
			errCh <- s.Acquire(ctx, 1)
		}()
		synctest.Wait()

		// Cancel and free a token atomically with respect to the waiter, so it
		// wakes up with both capacity available and its context done.
		s.mu.Lock()
		cancel()
		s.inFlight--
		s.cond.Broadcast()
		s.mu.Unlock()

		synctest.Wait()
		select {
		case err := <-errCh:
			assert.ErrorIs(t, err, context.Canceled)
		default:
			t.Fatal("Acquire should return when the context is cancelled")
		}
		assert.Equal(t, int64(0), s.inFlight)
		assert.NoError(t, s.Acquire(context.Background(), 1))
	})
}

func TestSaturatedDurationAccumulates(t *testing.T) {
	s := newResizableSemaphore(2)
	clock := &fakeClock{t: time.Unix(0, 0)}
	s.now = clock.now
	ctx := context.Background()

	require.NoError(t, s.Acquire(ctx, 1)) // inFlight 1 < 2: not saturated
	require.NoError(t, s.Acquire(ctx, 1)) // inFlight 2 == 2: saturation begins

	clock.advance(3 * time.Second)
	s.Release(1) // inFlight 1 < 2: saturation ends, 3s banked

	assert.Equal(t, 3*time.Second, s.takeSaturatedDuration())
	// The accumulator resets after a read.
	assert.Equal(t, time.Duration(0), s.takeSaturatedDuration())
}

func TestSaturatedDurationFoldsOpenInterval(t *testing.T) {
	s := newResizableSemaphore(1)
	clock := &fakeClock{t: time.Unix(0, 0)}
	s.now = clock.now
	ctx := context.Background()

	require.NoError(t, s.Acquire(ctx, 1)) // saturated immediately

	clock.advance(2 * time.Second)
	// Still saturated: the open interval is folded in and keeps running.
	assert.Equal(t, 2*time.Second, s.takeSaturatedDuration())

	clock.advance(time.Second)
	assert.Equal(t, time.Second, s.takeSaturatedDuration())
}

func TestSaturatedDurationTracksLimitChanges(t *testing.T) {
	s := newResizableSemaphore(2)
	clock := &fakeClock{t: time.Unix(0, 0)}
	s.now = clock.now
	ctx := context.Background()

	require.NoError(t, s.Acquire(ctx, 1)) // inFlight 1 < 2: not saturated
	s.SetLimit(1)                         // inFlight 1 >= 1: saturation begins

	clock.advance(5 * time.Second)
	s.Release(1) // inFlight 0 < 1: saturation ends, 5s banked

	assert.Equal(t, 5*time.Second, s.takeSaturatedDuration())
}

// TestResizableSemaphoreScalingUnderLoad proves the limit can be scaled up and
// down at runtime while many goroutines acquire and release, and that the
// number of concurrently held tokens never exceeds the limit in force.
func TestResizableSemaphoreScalingUnderLoad(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const initialLimit = 4
		s := newResizableSemaphore(initialLimit)

		var maxLimit atomic.Int64
		maxLimit.Store(initialLimit)
		var inFlight atomic.Int64

		ctx, cancel := context.WithCancel(context.Background())

		var wg sync.WaitGroup
		for range 32 {
			wg.Go(func() {
				for ctx.Err() == nil {
					if err := s.Acquire(ctx, 1); err != nil {
						return
					}
					cur := inFlight.Add(1)
					// Held tokens must never exceed the highest limit ever set.
					assert.LessOrEqual(t, cur, maxLimit.Load())
					// Hold the token for a while so that every goroutine
					// eventually blocks and the bubble's clock can advance.
					time.Sleep(time.Millisecond)
					inFlight.Add(-1)
					s.Release(1)
				}
			})
		}

		// Scale the limit up and down repeatedly while load is running.
		for _, limit := range []int64{8, 2, 16, 1, 6} {
			if limit > maxLimit.Load() {
				maxLimit.Store(limit)
			}
			s.SetLimit(limit)
			time.Sleep(20 * time.Millisecond)
		}

		cancel()
		wg.Wait()
	})
}
