// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import "container/heap"

// dedupEntry holds per-pattern deduplication state for the reporter delta.
// Entries live in correlationDedupTracker.entries and may additionally sit in
// the inactive min-heap when the pattern is not currently in any correlator's
// active window.
type dedupEntry struct {
	pattern       string
	lastUpdated   int64 // LastUpdated timestamp at the time of the last emission
	inactiveSince int64 // data timestamp when pattern first went inactive; 0 = still active
	heapIdx       int   // index in the inactive min-heap; -1 = not in heap (pattern is active)
}

// dedupHeap is a min-heap of *dedupEntry ordered by inactiveSince (oldest
// first). It contains only inactive entries (inactiveSince > 0).
// Implements container/heap.Interface.
type dedupHeap []*dedupEntry

func (h dedupHeap) Len() int           { return len(h) }
func (h dedupHeap) Less(i, j int) bool { return h[i].inactiveSince < h[j].inactiveSince }
func (h dedupHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].heapIdx = i
	h[j].heapIdx = j
}

func (h *dedupHeap) Push(x any) {
	entry := x.(*dedupEntry)
	entry.heapIdx = len(*h)
	*h = append(*h, entry)
}

func (h *dedupHeap) Pop() any {
	old := *h
	n := len(old)
	entry := old[n-1]
	old[n-1] = nil // prevent memory leak
	*h = old[:n-1]
	entry.heapIdx = -1
	return entry
}

// correlationDedupTracker tracks which correlation patterns have been emitted
// to reporters, with TTL-based re-arming for inactive entries.
//
// The structure combines:
//   - entries: O(1) map for existence/lastUpdated lookups.
//   - inactive: min-heap (ordered by inactiveSince) covering only entries whose
//     pattern has left all correlators' active windows. Entries are removed from
//     the heap as soon as a pattern becomes active again.
//
// When an entry has been continuously inactive for ttlSec data-time seconds it
// is evicted from entries entirely. The next time the pattern appears in
// accumulatedCorrelations newCorrelations will treat it as new and fire again,
// correctly re-arming for same-timestamp recurrences without spurious fires.
//
// maxItems caps the number of tracked entries by evicting the oldest inactive
// entry when the map exceeds the limit.
type correlationDedupTracker struct {
	ttlSec   int64
	maxItems int
	entries  map[string]*dedupEntry
	inactive dedupHeap
}

// newCorrelationDedupTracker creates a tracker with the given TTL and capacity.
func newCorrelationDedupTracker(ttlSec int64, maxItems int) *correlationDedupTracker {
	return &correlationDedupTracker{
		ttlSec:   ttlSec,
		maxItems: maxItems,
		entries:  make(map[string]*dedupEntry),
	}
}

// seen returns the LastUpdated timestamp stored for pattern and whether the
// pattern is currently tracked.
func (t *correlationDedupTracker) seen(pattern string) (int64, bool) {
	e, ok := t.entries[pattern]
	if !ok {
		return 0, false
	}
	return e.lastUpdated, true
}

// markSeen records that pattern was emitted with the given lastUpdated. Creates
// a new active entry if the pattern is not yet tracked.
func (t *correlationDedupTracker) markSeen(pattern string, lastUpdated int64) {
	if e, ok := t.entries[pattern]; ok {
		e.lastUpdated = lastUpdated
	} else {
		t.entries[pattern] = &dedupEntry{
			pattern:     pattern,
			lastUpdated: lastUpdated,
			heapIdx:     -1, // active, not in heap
		}
	}
}

// markActive moves entry from the inactive heap back to active state.
// No-op if the entry is already active.
func (t *correlationDedupTracker) markActive(e *dedupEntry) {
	if e.heapIdx < 0 {
		return
	}
	heap.Remove(&t.inactive, e.heapIdx)
	e.inactiveSince = 0
}

// markInactive records that entry became inactive at nowSec and pushes it onto
// the TTL heap. No-op if the entry is already inactive.
func (t *correlationDedupTracker) markInactive(e *dedupEntry, nowSec int64) {
	if e.inactiveSince > 0 {
		return
	}
	e.inactiveSince = nowSec
	heap.Push(&t.inactive, e)
}

// evictExpired removes all entries that have been continuously inactive for at
// least ttlSec data-time seconds. Evicted entries will be treated as new on
// their next appearance in accumulatedCorrelations, re-arming recurrence
// detection without a spurious fire.
func (t *correlationDedupTracker) evictExpired(nowSec int64) {
	for t.inactive.Len() > 0 {
		oldest := t.inactive[0]
		if oldest.inactiveSince+t.ttlSec > nowSec {
			break
		}
		heap.Pop(&t.inactive)
		delete(t.entries, oldest.pattern)
	}
}

// evictOverLimit removes the oldest inactive entries until len(entries) is at
// or below maxItems. Active entries are never evicted.
func (t *correlationDedupTracker) evictOverLimit() {
	for len(t.entries) > t.maxItems && t.inactive.Len() > 0 {
		oldest := heap.Pop(&t.inactive).(*dedupEntry)
		delete(t.entries, oldest.pattern)
	}
}

// reset clears all tracking state.
func (t *correlationDedupTracker) reset() {
	t.entries = make(map[string]*dedupEntry)
	t.inactive = t.inactive[:0]
}
