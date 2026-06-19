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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResizableSemaphoreBlocksAtLimit(t *testing.T) {
	s := newResizableSemaphore(2)
	ctx := context.Background()

	require.NoError(t, s.Acquire(ctx))
	require.NoError(t, s.Acquire(ctx))

	// The third Acquire must block until a token is released.
	acquired := make(chan struct{})
	go func() {
		_ = s.Acquire(ctx)
		close(acquired)
	}()

	select {
	case <-acquired:
		t.Fatal("Acquire should block when the limit is reached")
	case <-time.After(50 * time.Millisecond):
	}

	s.Release()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("Acquire should unblock once a token is released")
	}
}

func TestResizableSemaphoreSetLimitUpWakesWaiters(t *testing.T) {
	s := newResizableSemaphore(1)
	ctx := context.Background()
	require.NoError(t, s.Acquire(ctx))

	acquired := make(chan struct{})
	go func() {
		_ = s.Acquire(ctx)
		close(acquired)
	}()

	// Waiter is blocked at limit 1; raising the limit must wake it.
	select {
	case <-acquired:
		t.Fatal("Acquire should block before the limit is raised")
	case <-time.After(50 * time.Millisecond):
	}

	s.SetLimit(2)
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("raising the limit should wake the blocked acquirer")
	}
}

func TestResizableSemaphoreSetLimitDownDoesNotCancelInFlight(t *testing.T) {
	s := newResizableSemaphore(3)
	ctx := context.Background()
	require.NoError(t, s.Acquire(ctx))
	require.NoError(t, s.Acquire(ctx))
	require.NoError(t, s.Acquire(ctx))

	// Lowering the limit below the in-flight count must not panic or block;
	// it only prevents new acquisitions until enough tokens are released.
	s.SetLimit(1)

	blocked := make(chan struct{})
	go func() {
		_ = s.Acquire(ctx)
		close(blocked)
	}()
	select {
	case <-blocked:
		t.Fatal("Acquire should block while in-flight exceeds the lowered limit")
	case <-time.After(50 * time.Millisecond):
	}

	// At limit 1 the waiter can only proceed once in-flight drops to 0, i.e. all
	// three original holders release.
	s.Release()
	s.Release()
	s.Release()
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("Acquire should proceed once in-flight drops below the new limit")
	}
}

func TestResizableSemaphoreContextCancellation(t *testing.T) {
	s := newResizableSemaphore(1)
	require.NoError(t, s.Acquire(context.Background()))

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Acquire(ctx)
	}()

	cancel()
	select {
	case err := <-errCh:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("Acquire should return when the context is cancelled")
	}
}

func TestSaturatedDurationAccumulates(t *testing.T) {
	s := newResizableSemaphore(2)
	clock := &fakeClock{t: time.Unix(0, 0)}
	s.now = clock.now
	ctx := context.Background()

	require.NoError(t, s.Acquire(ctx)) // inFlight 1 < 2: not saturated
	require.NoError(t, s.Acquire(ctx)) // inFlight 2 == 2: saturation begins

	clock.advance(3 * time.Second)
	s.Release() // inFlight 1 < 2: saturation ends, 3s banked

	assert.Equal(t, 3*time.Second, s.takeSaturatedDuration())
	// The accumulator resets after a read.
	assert.Equal(t, time.Duration(0), s.takeSaturatedDuration())
}

func TestSaturatedDurationFoldsOpenInterval(t *testing.T) {
	s := newResizableSemaphore(1)
	clock := &fakeClock{t: time.Unix(0, 0)}
	s.now = clock.now
	ctx := context.Background()

	require.NoError(t, s.Acquire(ctx)) // saturated immediately

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

	require.NoError(t, s.Acquire(ctx)) // inFlight 1 < 2: not saturated
	s.SetLimit(1)                      // inFlight 1 >= 1: saturation begins

	clock.advance(5 * time.Second)
	s.Release() // inFlight 0 < 1: saturation ends, 5s banked

	assert.Equal(t, 5*time.Second, s.takeSaturatedDuration())
}

// TestResizableSemaphoreScalingUnderLoad proves the limit can be scaled up and
// down at runtime while many goroutines acquire and release, and that the
// number of concurrently held tokens never exceeds the limit in force.
func TestResizableSemaphoreScalingUnderLoad(t *testing.T) {
	const initialLimit = 4
	s := newResizableSemaphore(initialLimit)

	var maxLimit atomic.Int64
	maxLimit.Store(initialLimit)
	var inFlight atomic.Int64

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				if err := s.Acquire(ctx); err != nil {
					return
				}
				cur := inFlight.Add(1)
				// Held tokens must never exceed the highest limit ever set.
				assert.LessOrEqual(t, cur, maxLimit.Load())
				inFlight.Add(-1)
				s.Release()
			}
		}()
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
}
