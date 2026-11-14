// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sync"
	"time"
	"unicode/utf8"

	"github.com/benbjohnson/clock"
)

// Shared bounds for the loss trackers that feed Agent Health issues. Every tracker
// records even when Agent Health is disabled and nothing ever reads it, so each one
// must stay bounded on its own.
const (
	// How long a loss stays in a snapshot. Dropping out resolves the issue.
	lossWindow = 24 * time.Hour

	lossBucketSize = time.Hour

	// +1: a bucket counts while any part of it still overlaps the window.
	lossMaxBuckets = int(lossWindow/lossBucketSize) + 1

	// Caps keys held between two reads; source and service are unbounded user text.
	lossMaxKeys = 200

	// Source and service every key past lossMaxKeys folds into.
	lossOverflowLabel = "other"

	// Caps the length of a name. Names are raw config strings (YAML, pod
	// annotations), so bounding the entry count alone does not bound memory.
	// Two names sharing a prefix this long fold into one key.
	lossMaxNameLen = 64
)

// lossCounts is what one key lost: count is events for rotation loss and log lines
// for dropped logs; bytes is the data lost.
type lossCounts struct {
	count int64
	bytes int64
}

func (c lossCounts) empty() bool {
	return c.count <= 0 && c.bytes <= 0
}

type lossEntry struct {
	// Keyed by bucket start, as Unix nanoseconds.
	buckets      map[int64]*lossCounts
	lastLossNano int64
}

// pruneAndSum totals the buckets still overlapping the window ending at cutoffNano.
// The only place buckets are removed.
func (e *lossEntry) pruneAndSum(cutoffNano int64) lossCounts {
	var total lossCounts
	for start, bucket := range e.buckets {
		if start+int64(lossBucketSize) <= cutoffNano {
			delete(e.buckets, start)
			continue
		}
		total.count += bucket.count
		total.bytes += bucket.bytes
	}
	return total
}

// lossSummary is one key's loss over the trailing window.
type lossSummary[K comparable] struct {
	key        K
	counts     lossCounts
	lastLossAt time.Time
}

// lossTracker holds the trailing window of loss per key. Bounded without a reader:
// at most lossMaxKeys entries plus the overflow keys, each of lossMaxBuckets buckets.
type lossTracker[K comparable] struct {
	mu  sync.Mutex
	clk clock.Clock
	// overflow maps a key that does not fit under lossMaxKeys to the key it folds
	// into. It must return a key from a small, fixed set.
	overflow func(K) K
	entries  map[K]*lossEntry
}

func newLossTracker[K comparable](clk clock.Clock, overflow func(K) K) *lossTracker[K] {
	return &lossTracker[K]{
		clk:      clk,
		overflow: overflow,
		entries:  make(map[K]*lossEntry),
	}
}

// add records one loss. Callers bound the names in key first.
func (t *lossTracker[K]) add(key K, counts lossCounts) {
	if counts.empty() {
		return
	}
	nowNano := t.clk.Now().UnixNano()

	t.mu.Lock()
	defer t.mu.Unlock()
	t.addLocked(key, counts, nowNano)
}

// addAll records several losses that happened at once (one dropped payload) under
// a single lock acquisition.
func (t *lossTracker[K]) addAll(losses map[K]lossCounts) {
	if len(losses) == 0 {
		return
	}
	nowNano := t.clk.Now().UnixNano()

	t.mu.Lock()
	defer t.mu.Unlock()
	for key, counts := range losses {
		if !counts.empty() {
			t.addLocked(key, counts, nowNano)
		}
	}
}

func (t *lossTracker[K]) addLocked(key K, counts lossCounts, nowNano int64) {
	entry, ok := t.entries[key]
	if !ok {
		if len(t.entries) >= lossMaxKeys {
			key = t.overflow(key)
			entry, ok = t.entries[key]
		}
		if !ok {
			entry = &lossEntry{buckets: make(map[int64]*lossCounts)}
			t.entries[key] = entry
		}
	}

	// Pruned here too: nothing reads the tracker when health_platform.enabled is false.
	entry.pruneAndSum(nowNano - int64(lossWindow))

	start := alignLossBucket(nowNano)
	bucket, ok := entry.buckets[start]
	if !ok {
		bucket = &lossCounts{}
		entry.buckets[start] = bucket
	}
	bucket.count += counts.count
	bucket.bytes += counts.bytes
	entry.lastLossNano = nowNano
}

// collectAndPrune returns one summary per key still inside the window, in no
// particular order, and drops keys that aged out so a quiet agent returns to an
// empty tracker.
func (t *lossTracker[K]) collectAndPrune() []lossSummary[K] {
	cutoff := t.clk.Now().UnixNano() - int64(lossWindow)

	t.mu.Lock()
	defer t.mu.Unlock()

	summaries := make([]lossSummary[K], 0, len(t.entries))
	for key, entry := range t.entries {
		counts := entry.pruneAndSum(cutoff)
		if counts.empty() {
			delete(t.entries, key)
			continue
		}
		summaries = append(summaries, lossSummary[K]{
			key:        key,
			counts:     counts,
			lastLossAt: time.Unix(0, entry.lastLossNano),
		})
	}
	return summaries
}

func (t *lossTracker[K]) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = make(map[K]*lossEntry)
}

func alignLossBucket(tsNano int64) int64 {
	return (tsNano / int64(lossBucketSize)) * int64(lossBucketSize)
}

// boundName caps a key's length, cutting on a rune boundary. Rendering the cut for
// a reader is the issue template's job.
func boundName(name string) string {
	return boundNameTo(name, lossMaxNameLen)
}

func boundNameTo(name string, maxLen int) string {
	if utf8.RuneCountInString(name) <= maxLen {
		return name
	}
	return string([]rune(name)[:maxLen])
}
