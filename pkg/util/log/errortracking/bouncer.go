// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package errortracking

import (
	"sync"
	"time"
)

// Bouncer is a per-PC first-sighting deduplicator with a sliding time
// window. The first time a given program counter is observed inside a
// window, Observe returns suppressed=false with count=1 and the
// observation time as firstSeen. Subsequent observations of the same PC
// inside the same window are suppressed (return suppressed=true) and
// the count is incremented. After the window elapses, the next
// observation resets the entry and is treated as a fresh first
// sighting.
//
// Design rationale (PR #50607 iglendd thread "we need to implement
// Bouncer? PC could be the key … APM has 1m I think … start from
// 15m"): the wire payload's Count field becomes the number of
// suppressed duplicates, giving operators a sense of error volume
// without paying for one record per occurrence on a hot bug path.
//
// Bouncer is purpose-built rather than reusing the global
// rate.Sometimes wrapper in pkg/util/log/log_limit.go — that primitive
// is keyless (one Limit per call site), whereas we need per-PC state
// and need to expose the per-key count. The implementation is a small
// mutex-protected map with a periodic prune to bound memory; total
// entries are capped so a pathological input cannot blow up memory.
//
// The zero value is NOT usable; construct via NewBouncer. Observe is
// safe for concurrent use.
type Bouncer struct {
	window     time.Duration
	maxEntries int

	mu      sync.Mutex
	entries map[uintptr]*bouncerEntry
}

type bouncerEntry struct {
	firstSeen time.Time
	lastSeen  time.Time
	count     uint32
}

// NewBouncer returns a Bouncer with the given sliding-window duration
// and a soft cap on tracked entries. A non-positive window disables
// dedup (Observe always returns suppressed=false with count=1); a
// non-positive maxEntries falls back to a sane default (4096).
func NewBouncer(window time.Duration, maxEntries int) *Bouncer {
	if maxEntries <= 0 {
		maxEntries = 4096
	}
	return &Bouncer{
		window:     window,
		maxEntries: maxEntries,
		entries:    make(map[uintptr]*bouncerEntry),
	}
}

// Observe records a sighting of pc at now and returns whether the
// caller should suppress the record (drop it from the wire), the
// running count of sightings inside the current window (≥ 1), and the
// firstSeen time of the current window (useful for diagnostics).
//
// The first sighting of a pc in a window returns suppressed=false,
// count=1, firstSeen=now. Subsequent sightings inside the same window
// return suppressed=true with an incrementing count and the original
// firstSeen. After window elapses since firstSeen, the next sighting
// resets the entry and is treated as fresh (suppressed=false, count=1).
//
// When window is non-positive, Observe is a pass-through: returns
// suppressed=false, count=1, firstSeen=now.
func (b *Bouncer) Observe(pc uintptr, now time.Time) (suppressed bool, count uint32, firstSeen time.Time) {
	if b.window <= 0 {
		return false, 1, now
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if e, ok := b.entries[pc]; ok {
		if now.Sub(e.firstSeen) > b.window {
			// Window elapsed — reset and treat as fresh sighting.
			e.firstSeen = now
			e.lastSeen = now
			e.count = 1
			return false, 1, now
		}
		e.lastSeen = now
		e.count++
		return true, e.count, e.firstSeen
	}

	// New PC. Prune opportunistically if we're approaching the cap so
	// the map doesn't grow unboundedly under pathological churn (many
	// unique PCs, each appearing once).
	if len(b.entries) >= b.maxEntries {
		b.pruneLocked(now)
	}
	b.entries[pc] = &bouncerEntry{
		firstSeen: now,
		lastSeen:  now,
		count:     1,
	}
	return false, 1, now
}

// pruneLocked drops entries whose firstSeen is older than the window.
// Caller MUST hold b.mu. Linear in entry count; runs only on
// near-capacity insert, so amortized cost is bounded.
func (b *Bouncer) pruneLocked(now time.Time) {
	for pc, e := range b.entries {
		if now.Sub(e.firstSeen) > b.window {
			delete(b.entries, pc)
		}
	}
}
