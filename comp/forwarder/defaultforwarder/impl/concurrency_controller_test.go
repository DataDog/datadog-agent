// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultforwarderimpl

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

// fakeClock is a manually-advanced clock for deterministic time-based tests.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestController(t *testing.T, maxLimit int64) (*concurrencyController, *resizableSemaphore, *fakeClock) {
	clock := &fakeClock{t: time.Unix(0, 0)}
	sem := newResizableSemaphore(initialLimit)
	c := newConcurrencyController(logmock.New(t), sem, "test", maxLimit, make(chan struct{}, 1))
	c.now = clock.now
	c.limit = initialLimit
	c.lastEval = clock.now()
	return c, sem, clock
}

func TestControllerIncreasesWhenSaturated(t *testing.T) {
	c, sem, clock := newTestController(t, 10)

	// A full window spent saturated grows the limit by one step.
	clock.advance(evalInterval)
	sem.saturatedAccum = evalInterval
	c.onEval()

	assert.Equal(t, int64(initialLimit+increaseStep), c.limit)
	assert.Equal(t, c.limit, sem.limit)
}

func TestControllerCapsAtMaxLimit(t *testing.T) {
	c, sem, clock := newTestController(t, 3)
	c.limit = 3

	clock.advance(evalInterval)
	sem.saturatedAccum = evalInterval
	c.onEval()

	assert.Equal(t, int64(3), c.limit)
}

func TestControllerHoldsWhenNotSaturated(t *testing.T) {
	c, sem, clock := newTestController(t, 10)
	c.limit = 4

	// Saturated for only a fraction of the window (below the threshold).
	clock.advance(evalInterval)
	sem.saturatedAccum = evalInterval / 2
	c.onEval()

	assert.Equal(t, int64(4), c.limit)
}

func TestControllerDecreasesImmediatelyAndDebounces(t *testing.T) {
	c, sem, clock := newTestController(t, 16)
	c.limit = 8

	// First backoff halves immediately.
	c.onBackoff()
	assert.Equal(t, int64(4), c.limit)
	assert.Equal(t, int64(4), sem.limit)

	// A burst within minDecreaseInterval is debounced into the same step.
	c.onBackoff()
	assert.Equal(t, int64(4), c.limit)

	// After the debounce window, a further backoff steps down again.
	clock.advance(minDecreaseInterval + time.Millisecond)
	c.onBackoff()
	assert.Equal(t, int64(2), c.limit)
}

func TestControllerDecreaseFloorsAtOne(t *testing.T) {
	c, _, clock := newTestController(t, 16)
	c.limit = 1

	clock.advance(minDecreaseInterval + time.Millisecond)
	c.onBackoff()
	assert.Equal(t, int64(1), c.limit)
}

func TestControllerSuppressesIncreaseAfterDecrease(t *testing.T) {
	c, sem, clock := newTestController(t, 10)
	c.limit = 4

	c.onBackoff()
	assert.Equal(t, int64(2), c.limit)

	// Within the cooldown, a fully-saturated window must not grow the limit.
	clock.advance(increaseCooldown - time.Second)
	c.lastEval = clock.now().Add(-evalInterval)
	sem.saturatedAccum = evalInterval
	c.onEval()
	assert.Equal(t, int64(2), c.limit)

	// Once the cooldown elapses, growth resumes.
	clock.advance(2 * time.Second)
	c.lastEval = clock.now().Add(-evalInterval)
	sem.saturatedAccum = evalInterval
	c.onEval()
	assert.Equal(t, int64(3), c.limit)
}
