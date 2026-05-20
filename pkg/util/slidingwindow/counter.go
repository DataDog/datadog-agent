// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package slidingwindow provides a thread-safe, second-resolution sliding
// window event counter backed by a ring buffer.
package slidingwindow

import (
	"sync"
	"time"
)

// Counter tracks how many events have occurred within a sliding time window.
//
// Count returns events from the last windowSize seconds,.
// including the current second and excluding events exactly
// windowSize seconds old.
//
// Internally the counter uses a ring buffer of windowSize int64 slots
// (one per second).  Add is O(1); Count is O(windowSize).
//
// Safe for concurrent use by multiple goroutines.
type Counter struct {
	mu         sync.Mutex
	slots      []int64 // ring: slots[i] holds the count for the second in slotTimes[i]
	slotTimes  []int64 // Unix second each slot represents; sentinel -windowSize means "empty"
	windowSize int64
	nowFn      func() int64
}

// New creates a Counter with the given window size in seconds.
// windowSize must be > 0.
func New(windowSize int) *Counter {
	return newCounter(windowSize, func() int64 { return time.Now().Unix() })
}

// newCounter is the internal constructor that accepts an injectable clock.
func newCounter(windowSize int, nowFn func() int64) *Counter {
	if windowSize <= 0 {
		panic("slidingwindow: windowSize must be > 0")
	}
	c := &Counter{
		slots:      make([]int64, windowSize),
		slotTimes:  make([]int64, windowSize),
		windowSize: int64(windowSize),
		nowFn:      nowFn,
	}
	// Initialise every slot to a time so far in the past that it is always
	// considered stale, even when the fake clock starts at t=0.
	for i := range c.slotTimes {
		c.slotTimes[i] = -int64(windowSize)
	}
	return c
}

// Add records n events at the current second.
// n must be ≥ 0; a zero or negative n is a no-op.
func (c *Counter) Add(n int64) {
	if n <= 0 {
		return
	}
	now := c.nowFn()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.addLocked(n, now)
}

func (c *Counter) addLocked(n int64, now int64) {
	// Compute slot index with a positive modulo so negative mock-clock
	// values do not produce a negative index.
	idx := now % c.windowSize
	if idx < 0 {
		idx += c.windowSize
	}
	// If the slot belongs to a different second, clear any stale data first.
	if c.slotTimes[idx] != now {
		c.slots[idx] = 0
		c.slotTimes[idx] = now
	}
	c.slots[idx] += n
}

// Count returns the total number of events recorded in [now-windowSize, now).
// Events from exactly windowSize seconds ago are NOT included.
func (c *Counter) Count() int64 {
	now := c.nowFn()
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.countLocked(now)
}

func (c *Counter) countLocked(now int64) int64 {
	var total int64
	for i := int64(0); i < c.windowSize; i++ {
		age := now - c.slotTimes[i]
		// Include only slots whose age is in [0, windowSize).
		// age < 0 means the slot is from the future (clock went backward);
		// age >= windowSize means the slot is outside the window.
		if age >= 0 && age < c.windowSize {
			total += c.slots[i]
		}
	}
	return total
}

// Reset zeros the counter, discarding all recorded events.
func (c *Counter) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.slots {
		c.slots[i] = 0
		c.slotTimes[i] = -c.windowSize
	}
}
