// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/benbjohnson/clock"
)

const (
	// How long a loss stays in MissedBytesSnapshot. Dropping out resolves the issue.
	missedBytesWindow = 24 * time.Hour

	missedBytesBucketSize = time.Hour

	// +1: a bucket counts while any part of it still overlaps the window.
	missedBytesMaxBuckets = int(missedBytesWindow/missedBytesBucketSize) + 1

	// Caps tuples held between two reads; service is unbounded user text.
	missedBytesMaxKeys = 200

	// Source and service every tuple past missedBytesMaxKeys folds into.
	missedBytesOverflowLabel = "other"

	// Caps the length of a key. Names are raw config strings (YAML, pod
	// annotations), so bounding the entry count alone does not bound memory.
	// Two names sharing a prefix this long fold into one tuple.
	missedBytesMaxNameLen = 64
)

// MissedBytesSummary is one (source, service) tuple's loss over the trailing window.
type MissedBytesSummary struct {
	Source     string
	Service    string
	Bytes      int64
	Rotations  int64
	LastLossAt time.Time
}

type missedBytesKey struct {
	source  string
	service string
}

type missedBytesBucket struct {
	bytes     int64
	rotations int64
}

type missedBytesEntry struct {
	// Keyed by bucket start, as Unix nanoseconds.
	buckets      map[int64]*missedBytesBucket
	lastLossNano int64
}

// pruneAndSum totals the buckets still overlapping the window ending at cutoffNano.
// The only place buckets are removed.
func (e *missedBytesEntry) pruneAndSum(cutoffNano int64) (bytes, rotations int64) {
	for start, bucket := range e.buckets {
		if start+int64(missedBytesBucketSize) <= cutoffNano {
			delete(e.buckets, start)
			continue
		}
		bytes += bucket.bytes
		rotations += bucket.rotations
	}
	return bytes, rotations
}

// missedBytesTracker holds the trailing window of loss per tuple. Bounded without a
// reader: at most missedBytesMaxKeys+1 entries of missedBytesMaxBuckets buckets.
type missedBytesTracker struct {
	mu      sync.Mutex
	clk     clock.Clock
	entries map[missedBytesKey]*missedBytesEntry
}

func newMissedBytesTracker(clk clock.Clock) *missedBytesTracker {
	return &missedBytesTracker{
		clk:     clk,
		entries: make(map[missedBytesKey]*missedBytesEntry),
	}
}

func (t *missedBytesTracker) record(source, service string, bytes int64) {
	if bytes <= 0 {
		return
	}

	nowNano := t.clk.Now().UnixNano()

	t.mu.Lock()
	defer t.mu.Unlock()

	key := missedBytesKey{source: boundName(source), service: boundName(service)}
	entry, ok := t.entries[key]
	if !ok {
		if len(t.entries) >= missedBytesMaxKeys {
			key = missedBytesKey{source: missedBytesOverflowLabel, service: missedBytesOverflowLabel}
			entry, ok = t.entries[key]
		}
		if !ok {
			entry = &missedBytesEntry{buckets: make(map[int64]*missedBytesBucket)}
			t.entries[key] = entry
		}
	}

	// Pruned here too: nothing reads the tracker when health_platform.enabled is false.
	entry.pruneAndSum(nowNano - int64(missedBytesWindow))

	start := alignMissedBytesBucket(nowNano)
	bucket, ok := entry.buckets[start]
	if !ok {
		bucket = &missedBytesBucket{}
		entry.buckets[start] = bucket
	}
	bucket.bytes += bytes
	bucket.rotations++
	entry.lastLossNano = nowNano
}

// collectAndPrune returns one sorted summary per tuple still inside the window and
// drops those that aged out, so a quiet agent returns to an empty tracker.
func (t *missedBytesTracker) collectAndPrune() []MissedBytesSummary {
	cutoff := t.clk.Now().UnixNano() - int64(missedBytesWindow)

	t.mu.Lock()
	defer t.mu.Unlock()

	summaries := make([]MissedBytesSummary, 0, len(t.entries))
	for key, entry := range t.entries {
		bytes, rotations := entry.pruneAndSum(cutoff)

		if bytes <= 0 {
			delete(t.entries, key)
			continue
		}

		summaries = append(summaries, MissedBytesSummary{
			Source:     key.source,
			Service:    key.service,
			Bytes:      bytes,
			Rotations:  rotations,
			LastLossAt: time.Unix(0, entry.lastLossNano),
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Source != summaries[j].Source {
			return summaries[i].Source < summaries[j].Source
		}
		return summaries[i].Service < summaries[j].Service
	})

	return summaries
}

func (t *missedBytesTracker) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = make(map[missedBytesKey]*missedBytesEntry)
}

func alignMissedBytesBucket(tsNano int64) int64 {
	return (tsNano / int64(missedBytesBucketSize)) * int64(missedBytesBucketSize)
}

// boundName caps a key's length, cutting on a rune boundary. Rendering the cut for
// a reader is the issue template's job.
func boundName(name string) string {
	if utf8.RuneCountInString(name) <= missedBytesMaxNameLen {
		return name
	}
	return string([]rune(name)[:missedBytesMaxNameLen])
}

// Process-wide because runner.HealthCheckFunc takes no arguments. Like BytesMissed.
var missedBytes = newMissedBytesTracker(clock.New())

// Distinguishes "no loss" from "no logs agent here". One-shot commands wire the
// health platform and share the agent's issue store, so their empty tracker must
// not read as a resolution.
var logsAgentRunning atomic.Bool

// RecordMissedBytes records bytes lost when a rotation closed a file early. Never
// reached on Windows, where the tailer holds no os.File to size the loss with.
func RecordMissedBytes(source, service string, bytes int64) {
	missedBytes.record(source, service, bytes)
}

// MissedBytesSnapshot returns in-window losses, sorted by source then service.
func MissedBytesSnapshot() []MissedBytesSummary {
	return missedBytes.collectAndPrune()
}

// MarkLogsAgentRunning records that the logs agent started here. Call only from the
// logs agent's start path: analyze-logs starts a file launcher without one.
func MarkLogsAgentRunning() {
	logsAgentRunning.Store(true)
}

// LogsAgentRunning reports whether the logs agent started in this process.
func LogsAgentRunning() bool {
	return logsAgentRunning.Load()
}
