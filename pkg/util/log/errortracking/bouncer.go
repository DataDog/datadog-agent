// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package errortracking

import (
	"sync"
	"time"
)

// Bouncer is a per-key first-sighting deduplicator with a sliding time
// window. The first time a given key is observed inside a window,
// Observe returns suppressed=false with count=1 and the observation
// time as firstSeen. Subsequent observations of the same key inside the
// same window are suppressed (return suppressed=true) and the count is
// incremented. After the window elapses, the next observation returns
// suppressed=false carrying the suppressed count of the prior window
// (priorTotal-1, since the first sighting was already delivered), then
// resets the entry to a fresh count=1. If no sightings were suppressed
// (priorTotal==1), the rollover is silent and the next observation is
// treated as a fresh first sighting (count=1).
//
// The key is an opaque uint64 — the caller is responsible for choosing
// it. Today's caller (Handler.Handle) hashes the captured stack PCs
// with FNV-1a so two distinct stacks reaching the same terminal
// function are NOT collapsed into the same bouncer entry.
//
// The 15-minute default at the agent call site
// (agent_telemetry.errortracking.bouncer_window_seconds) was chosen so a hot
// bug path collapses to one record per quarter-hour — long enough to avoid
// flooding the wire, short enough that operators see new error patterns
// promptly. A hot bug path with N sightings per window ships Count=1 at
// first sighting, then Count=N-1 on rollover (the suppressed portion).
// Summing both gives N without double-counting the first sighting.
//
// Bouncer is purpose-built rather than reusing the global
// rate.Sometimes wrapper in pkg/util/log/log_limit.go — that primitive
// is keyless (one Limit per call site), whereas we need per-key state
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
	entries map[uint64]*bouncerEntry
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
		entries:    make(map[uint64]*bouncerEntry),
	}
}

// Observe records a sighting of the given key at now and returns
// whether the caller should suppress the record, the count to attach to
// the next delivered record (≥ 1), and the firstSeen time of the
// current window.
//
// The first sighting of a key in a window returns suppressed=false,
// count=1, firstSeen=now. Subsequent sightings inside the same window
// return suppressed=true with an incrementing count and the original
// firstSeen. When the window elapses since firstSeen, the next
// sighting returns suppressed=false with count=priorSuppressed (the
// number of sightings that were suppressed in the elapsed window, i.e.
// priorTotal-1), then resets the entry to a fresh count=1. If no
// sightings were suppressed (priorTotal==1), the rollover is silent:
// Observe returns suppressed=false, count=1, as though it were a fresh
// first sighting, avoiding a Count=0 delivery.
//
// This design ensures consumers can sum Count fields without
// double-counting: the first delivery carries Count=1, and the rollover
// carries the remainder.
//
// When window is non-positive, Observe is a pass-through: returns
// suppressed=false, count=1, firstSeen=now.
func (b *Bouncer) Observe(key uint64, now time.Time) (suppressed bool, count uint32, firstSeen time.Time) {
	if b.window <= 0 {
		return false, 1, now
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if e, ok := b.entries[key]; ok {
		if now.Sub(e.firstSeen) > b.window {
			// Window elapsed. Deliver the suppressed portion of the prior
			// window (priorTotal-1) so callers can sum without
			// double-counting the first sighting already delivered.
			// When nothing was suppressed (priorTotal==1), skip the
			// rollover and treat this as a fresh first sighting.
			priorCount := e.count
			e.firstSeen = now
			e.lastSeen = now
			e.count = 1
			if priorCount <= 1 {
				return false, 1, now
			}
			return false, priorCount - 1, now
		}
		e.lastSeen = now
		e.count++
		return true, e.count, e.firstSeen
	}

	// New key. Prune opportunistically if at the cap so the map doesn't
	// grow unboundedly under pathological churn (many unique keys, each
	// appearing once). Re-check after pruning: if all entries are still
	// within the window, pruneLocked removes nothing and the cap must be
	// enforced by dropping the new key (pass-through, not suppressed).
	if len(b.entries) >= b.maxEntries {
		b.pruneLocked(now)
		if len(b.entries) >= b.maxEntries {
			return false, 1, now
		}
	}
	b.entries[key] = &bouncerEntry{
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
	for key, e := range b.entries {
		if now.Sub(e.firstSeen) > b.window {
			delete(b.entries, key)
		}
	}
}
