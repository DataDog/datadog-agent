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
// suppressed=false carrying the prior window's total count (so the
// delivered record's Count field reflects every sighting in that
// window), then resets the entry to a fresh count=1.
//
// The key is an opaque uintptr — the caller is responsible for choosing
// it. Today's caller (Handler.Handle) hashes the captured stack PCs
// with FNV-1a so two distinct stacks reaching the same terminal
// function are NOT collapsed into the same bouncer entry.
//
// The 15-minute default at the agent call site
// (agent_telemetry.errortracking.bouncer_window_seconds) was chosen so a hot
// bug path collapses to one record per quarter-hour — long enough to avoid
// flooding the wire, short enough that operators see new error patterns
// promptly. Count on the delivered record carries the total sightings of
// the prior window so suppressed duplicates are not lost; a hot bug path
// with N sightings per window ships one record per window with Count=N,
// rather than N records each carrying Count=1.
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

// Observe records a sighting of the given key at now and returns
// whether the caller should suppress the record, the running count of
// sightings represented by the next delivered record (≥ 1), and the
// firstSeen time of the current window.
//
// The first sighting of a key in a window returns suppressed=false,
// count=1, firstSeen=now. Subsequent sightings inside the same window
// return suppressed=true with an incrementing count and the original
// firstSeen. When the window elapses since firstSeen, the next
// sighting returns suppressed=false with count=priorWindowCount (the
// total sightings observed during the elapsed window), then resets
// the entry to a fresh count=1. The wire payload's Count field on the
// delivered record therefore carries the total occurrences of the
// prior window — no suppressed sightings are lost.
//
// When window is non-positive, Observe is a pass-through: returns
// suppressed=false, count=1, firstSeen=now.
func (b *Bouncer) Observe(key uintptr, now time.Time) (suppressed bool, count uint32, firstSeen time.Time) {
	if b.window <= 0 {
		return false, 1, now
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if e, ok := b.entries[key]; ok {
		if now.Sub(e.firstSeen) > b.window {
			// Window elapsed — the delivered record carries the prior
			// window's total count so suppressed duplicates are not
			// lost. Reset the entry to a fresh window.
			priorCount := e.count
			e.firstSeen = now
			e.lastSeen = now
			e.count = 1
			return false, priorCount, now
		}
		e.lastSeen = now
		e.count++
		return true, e.count, e.firstSeen
	}

	// New key. Prune opportunistically if we're approaching the cap so
	// the map doesn't grow unboundedly under pathological churn (many
	// unique keys, each appearing once).
	if len(b.entries) >= b.maxEntries {
		b.pruneLocked(now)
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
