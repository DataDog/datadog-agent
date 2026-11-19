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

// Bounds for the loss trackers that feed Agent Health issues (dropped logs and
// rotation loss). They record even when Agent Health is disabled and nothing ever
// reads them, so each must stay bounded on its own.
const (
	// lossQuietPeriod is how long a key must go without a new loss for its incident
	// to end. An issue resolves once every key behind it has gone quiet.
	lossQuietPeriod = time.Hour

	// lossMaxIncident caps an incident that never goes quiet: the first loss after
	// this long starts a new incident, so totals stay on a one-day scale.
	lossMaxIncident = 24 * time.Hour

	// Caps keys held between two reads; source and service are unbounded user text.
	lossMaxKeys = 200

	// Source and service every key past lossMaxKeys folds into.
	lossOverflowLabel = "other"

	// Caps the length of a name. Names are raw config strings (YAML, pod
	// annotations), so bounding the entry count alone does not bound memory.
	// Two names sharing a prefix this long fold into one key.
	lossMaxNameLen = 64
)

// lossKey identifies what lost data: which source and service, and for dropped logs
// also why and at which destination. Rotation loss leaves those empty.
type lossKey struct {
	source  string
	service string
	reason  DropReason
	host    string
	main    bool
}

// lossCounts is what one key lost: count is log lines for dropped logs and rotations
// for rotation loss; bytes is the data lost.
type lossCounts struct {
	count int64
	bytes int64
}

func (c lossCounts) empty() bool {
	return c.count <= 0 && c.bytes <= 0
}

// lossEntry is one key's current incident: what it lost since startNano.
type lossEntry struct {
	counts       lossCounts
	startNano    int64
	lastLossNano int64
}

func (e *lossEntry) quiet(nowNano int64) bool {
	return nowNano-e.lastLossNano >= int64(lossQuietPeriod)
}

// lossSummary is one key's ongoing incident.
type lossSummary struct {
	key        lossKey
	counts     lossCounts
	startedAt  time.Time
	lastLossAt time.Time
}

// lossTracker holds one incident per key. Bounded without a reader: at most
// lossMaxKeys entries plus the overflow keys, each a fixed size.
type lossTracker struct {
	mu  sync.Mutex
	clk clock.Clock
	// overflow maps a key that does not fit under lossMaxKeys to the key it folds
	// into. It must return a key from a small, fixed set.
	overflow func(lossKey) lossKey
	entries  map[lossKey]*lossEntry
}

func newLossTracker(clk clock.Clock, overflow func(lossKey) lossKey) *lossTracker {
	return &lossTracker{
		clk:      clk,
		overflow: overflow,
		entries:  make(map[lossKey]*lossEntry),
	}
}

// add records one loss. Callers bound the names in key first.
func (t *lossTracker) add(key lossKey, counts lossCounts) {
	if counts.empty() {
		return
	}
	nowNano := t.clk.Now().UnixNano()

	t.mu.Lock()
	defer t.mu.Unlock()
	t.addLocked(key, counts, nowNano)
}

// addAll records several losses that happened at once (one dropped payload) under
// a single lock acquisition. Callers bound the names in each key first.
func (t *lossTracker) addAll(losses map[lossKey]lossCounts) {
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

func (t *lossTracker) addLocked(key lossKey, counts lossCounts, nowNano int64) {
	entry, ok := t.entries[key]
	if !ok {
		if len(t.entries) >= lossMaxKeys {
			// Keys that went quiet hold no incident; free them before folding a new
			// key into the overflow. Nothing else frees them without a reader.
			t.evictQuietLocked(nowNano)
		}
		if len(t.entries) >= lossMaxKeys {
			key = t.overflow(key)
			entry, ok = t.entries[key]
		}
		if !ok {
			entry = &lossEntry{startNano: nowNano}
			t.entries[key] = entry
		}
	}

	// A key that went quiet, or whose incident reached the cap, starts a new one.
	if entry.lastLossNano != 0 && (entry.quiet(nowNano) || nowNano-entry.startNano >= int64(lossMaxIncident)) {
		*entry = lossEntry{startNano: nowNano}
	}
	entry.counts.count += counts.count
	entry.counts.bytes += counts.bytes
	entry.lastLossNano = nowNano
}

func (t *lossTracker) evictQuietLocked(nowNano int64) {
	for key, entry := range t.entries {
		if entry.quiet(nowNano) {
			delete(t.entries, key)
		}
	}
}

// collectAndPrune returns one summary per ongoing incident, in no particular order,
// and drops the keys that went quiet, so a quiet agent returns to an empty tracker.
func (t *lossTracker) collectAndPrune() []lossSummary {
	nowNano := t.clk.Now().UnixNano()

	t.mu.Lock()
	defer t.mu.Unlock()

	summaries := make([]lossSummary, 0, len(t.entries))
	for key, entry := range t.entries {
		if entry.quiet(nowNano) || entry.counts.empty() {
			delete(t.entries, key)
			continue
		}
		summaries = append(summaries, lossSummary{
			key:        key,
			counts:     entry.counts,
			startedAt:  time.Unix(0, entry.startNano),
			lastLossAt: time.Unix(0, entry.lastLossNano),
		})
	}
	return summaries
}

func (t *lossTracker) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = make(map[lossKey]*lossEntry)
}

// boundNameTo caps a name at maxLen runes, cutting on a rune boundary. Rendering
// the cut for a reader is the issue template's job.
func boundNameTo(name string, maxLen int) string {
	if utf8.RuneCountInString(name) <= maxLen {
		return name
	}
	return string([]rune(name)[:maxLen])
}

// ClearLogsHealth forgets every ongoing loss incident, dropped logs and rotation
// loss alike, so their Agent Health issues resolve at the next check. A customer
// runs it (`agent logs clear-health`) once the cause is fixed; if logs are still
// being lost, a new incident starts and the issue comes back.
func ClearLogsHealth() {
	droppedLogs.reset()
	missedBytes.reset()
}
