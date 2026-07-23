// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package errortracking

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBouncer_FirstSightingNotSuppressed(t *testing.T) {
	b := NewBouncer(15*time.Minute, 0)
	now := time.Now()
	suppressed, count, firstSeen := b.Observe(0xCAFE, now)
	assert.False(t, suppressed, "first sighting must NOT be suppressed")
	assert.Equal(t, uint32(1), count, "first sighting count")
	assert.True(t, firstSeen.Equal(now), "first sighting firstSeen = %v, want %v", firstSeen, now)
}

func TestBouncer_SecondSightingSuppressedInsideWindow(t *testing.T) {
	b := NewBouncer(15*time.Minute, 0)
	t0 := time.Now()
	b.Observe(0xCAFE, t0)

	t1 := t0.Add(time.Minute)
	suppressed, count, firstSeen := b.Observe(0xCAFE, t1)
	assert.True(t, suppressed, "second sighting inside window MUST be suppressed")
	assert.Equal(t, uint32(2), count, "second sighting count")
	assert.True(t, firstSeen.Equal(t0), "firstSeen drifted: got %v, want original %v", firstSeen, t0)
}

func TestBouncer_ResetsAfterWindow(t *testing.T) {
	b := NewBouncer(15*time.Minute, 0)
	t0 := time.Now()
	b.Observe(0xCAFE, t0)                  // count=1
	b.Observe(0xCAFE, t0.Add(time.Minute)) // count=2 (suppressed)

	// Step past the window — next sighting MUST NOT be suppressed and
	// MUST carry the suppressed count of the prior window (1 = total-1)
	// rather than the full total, to avoid double-counting the first
	// sighting already delivered. The entry is then reset to a fresh
	// window internally.
	t2 := t0.Add(16 * time.Minute)
	suppressed, count, firstSeen := b.Observe(0xCAFE, t2)
	assert.False(t, suppressed, "sighting after window MUST NOT be suppressed")
	assert.Equal(t, uint32(1), count, "post-window count (want suppressed-only: total-1)")
	assert.True(t, firstSeen.Equal(t2), "post-window firstSeen = %v, want %v", firstSeen, t2)
}

// TestBouncer_WindowElapseCarriesPriorCount exercises the
// suppressed-count carry-forward contract end-to-end: a hot bug path
// with N sightings per window must deliver Count=1 at first sighting
// and Count=N-1 on rollover (the suppressed sightings), so consumers
// summing both get N without double-counting the first sighting.
func TestBouncer_WindowElapseCarriesPriorCount(t *testing.T) {
	b := NewBouncer(15*time.Minute, 0)
	var key uint64 = 0xABCDEF

	t0 := time.Now()

	// First sighting — delivered, count=1.
	suppressed, count, _ := b.Observe(key, t0)
	assert.False(t, suppressed, "first sighting must NOT be suppressed")
	assert.Equal(t, uint32(1), count, "first sighting count")

	// Two more sightings within the window — both suppressed.
	suppressed, count, _ = b.Observe(key, t0.Add(5*time.Minute))
	assert.True(t, suppressed, "sighting inside window MUST be suppressed")
	assert.Equal(t, uint32(2), count, "second sighting count")

	suppressed, count, _ = b.Observe(key, t0.Add(10*time.Minute))
	assert.True(t, suppressed, "sighting inside window MUST be suppressed")
	assert.Equal(t, uint32(3), count, "third sighting count")

	// Window elapses; next sighting delivers the suppressed count of the
	// prior window (2 = total(3) - first-already-delivered(1)).
	suppressed, count, _ = b.Observe(key, t0.Add(20*time.Minute))
	assert.False(t, suppressed, "window elapsed; sighting must be delivered")
	assert.Equal(t, uint32(2), count, "rollover count (want suppressed-only: total-1)")

	// Subsequent sighting in the new window — suppressed, count starts
	// from the post-reset 1 and increments to 2.
	suppressed, count, _ = b.Observe(key, t0.Add(21*time.Minute))
	assert.True(t, suppressed, "sighting inside fresh window MUST be suppressed")
	assert.Equal(t, uint32(2), count, "post-reset sighting count")
}

func TestBouncer_PerPCIndependent(t *testing.T) {
	b := NewBouncer(15*time.Minute, 0)
	now := time.Now()
	b.Observe(0xCAFE, now)
	// A different PC must NOT see the suppression state of 0xCAFE.
	suppressed, count, _ := b.Observe(0xBEEF, now)
	assert.False(t, suppressed, "different PC must NOT be suppressed by another PC's sighting")
	assert.Equal(t, uint32(1), count, "different PC count")
}

func TestBouncer_DisabledWindowPassesThrough(t *testing.T) {
	b := NewBouncer(0, 0)
	now := time.Now()
	for i := 0; i < 5; i++ {
		suppressed, count, _ := b.Observe(0xCAFE, now.Add(time.Duration(i)*time.Second))
		assert.False(t, suppressed, "disabled window must never suppress (i=%d)", i)
		assert.Equal(t, uint32(1), count, "disabled window count (i=%d)", i)
	}
}

func TestBouncer_RaceFree_ConcurrentObserve(t *testing.T) {
	b := NewBouncer(15*time.Minute, 1024)
	const goroutines = 32
	const perGoroutine = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		g := g
		go func() {
			defer wg.Done()
			now := time.Now()
			for i := 0; i < perGoroutine; i++ {
				pc := uint64(g*1000 + i%50) // some overlap to exercise the suppress path
				b.Observe(pc, now)
			}
		}()
	}
	wg.Wait()
	// Survived without -race detecting anything; this is the
	// observable for the test (the race detector flags concurrent
	// map access or counter races at the failure site, not here).
	t.Log("concurrent observe completed without race violation")
}

func TestBouncer_PrunesNearCap(t *testing.T) {
	b := NewBouncer(time.Minute, 4)
	t0 := time.Now()
	// Fill the cap with entries that are about to expire.
	for pc := uint64(1); pc <= 4; pc++ {
		b.Observe(pc, t0)
	}
	// All entries are well past their window when we insert one more.
	t1 := t0.Add(2 * time.Minute)
	b.Observe(uint64(5), t1)

	b.mu.Lock()
	defer b.mu.Unlock()
	assert.LessOrEqual(t, len(b.entries), b.maxEntries, "entries exceeded cap after prune")
}

// TestBouncer_CapEnforced_WhenAllWithinWindow verifies that when the cap
// is reached and pruneLocked removes nothing (all entries still within the
// window), new keys are dropped rather than growing the map past maxEntries.
// The dropped observation is returned as a pass-through (suppressed=false,
// count=1) so the record still reaches the consumer without dedup tracking.
func TestBouncer_CapEnforced_WhenAllWithinWindow(t *testing.T) {
	b := NewBouncer(time.Hour, 4) // large window: no entries expire
	t0 := time.Now()
	for pc := uint64(1); pc <= 4; pc++ {
		b.Observe(pc, t0)
	}

	// New unique key inserted while all existing entries are still within window.
	t1 := t0.Add(time.Minute)
	suppressed, count, _ := b.Observe(uint64(5), t1)
	assert.False(t, suppressed, "dropped key must not be reported as suppressed")
	assert.Equal(t, uint32(1), count, "dropped key must return count=1 (pass-through)")

	b.mu.Lock()
	defer b.mu.Unlock()
	assert.LessOrEqual(t, len(b.entries), b.maxEntries, "map must not exceed maxEntries")
}
