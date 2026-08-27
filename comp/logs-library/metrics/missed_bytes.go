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

	"github.com/benbjohnson/clock"
)

const (
	// missedBytesWindow is how long a loss stays visible to MissedBytesSnapshot,
	// and so how long the Agent Health issue stays alive: the check reports for as
	// long as a tuple appears in the snapshot, and ceasing to report resolves it.
	missedBytesWindow = 24 * time.Hour

	// missedBytesBucketSize is the granularity at which loss ages out of the
	// window.
	missedBytesBucketSize = time.Hour

	// missedBytesMaxKeys caps distinct tracked tuples. Service is user-controlled
	// free text, so the key space is not bounded by config size alone.
	missedBytesMaxKeys = 200

	// missedBytesOverflowLabel is the source and service every tuple past
	// missedBytesMaxKeys folds into.
	missedBytesOverflowLabel = "other"
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
	// buckets is keyed by the bucket's start, as Unix nanoseconds.
	buckets      map[int64]*missedBytesBucket
	lastLossNano int64
}

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

// record adds one rotation's worth of lost bytes for a tuple.
func (t *missedBytesTracker) record(source, service string, bytes int64) {
	if bytes <= 0 {
		return
	}

	nowNano := t.clk.Now().UnixNano()

	t.mu.Lock()
	defer t.mu.Unlock()

	key := missedBytesKey{source: source, service: service}
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

// snapshot returns one summary per tuple with loss inside the trailing window,
// sorted for deterministic output. Buckets and tuples that have aged out are
// dropped here, so a quiet agent returns to an empty tracker.
func (t *missedBytesTracker) snapshot() []MissedBytesSummary {
	cutoff := t.clk.Now().UnixNano() - int64(missedBytesWindow)

	t.mu.Lock()
	defer t.mu.Unlock()

	summaries := make([]MissedBytesSummary, 0, len(t.entries))
	for key, entry := range t.entries {
		var bytes, rotations int64
		for start, bucket := range entry.buckets {
			if start+int64(missedBytesBucketSize) <= cutoff {
				delete(entry.buckets, start)
				continue
			}
			bytes += bucket.bytes
			rotations += bucket.rotations
		}

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

// reset clears all tracked state, leaving the tracker usable.
func (t *missedBytesTracker) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = make(map[missedBytesKey]*missedBytesEntry)
}

func alignMissedBytesBucket(tsNano int64) int64 {
	return (tsNano / int64(missedBytesBucketSize)) * int64(missedBytesBucketSize)
}

// missedBytes is the process-wide tracker. It is a singleton rather than an
// injected component because runner.HealthCheckFunc takes no arguments and
// issues.ModuleDeps carries no logs dependency. This mirrors BytesMissed.
var missedBytes = newMissedBytesTracker(clock.New())

// fileTailingActive reports whether the file launcher started in this process,
// letting the health check distinguish "no loss" from "no logs agent here".
var fileTailingActive atomic.Bool

// RecordMissedBytes records bytes lost for a (source, service) tuple after a log
// rotation closed a file before the tailer finished reading it.
func RecordMissedBytes(source, service string, bytes int64) {
	missedBytes.record(source, service, bytes)
}

// MissedBytesSnapshot returns the tuples that lost bytes within the trailing
// window, sorted by source then service.
func MissedBytesSnapshot() []MissedBytesSummary {
	return missedBytes.snapshot()
}

// MarkFileTailingActive records that the file launcher started in this process.
func MarkFileTailingActive() {
	fileTailingActive.Store(true)
}

// FileTailingActive reports whether the file launcher started in this process.
func FileTailingActive() bool {
	return fileTailingActive.Load()
}

// ResetMissedBytesForTest clears the process-wide tracker state. Test-only.
// It mutates the existing tracker rather than replacing it, so it stays safe to
// call while tailer goroutines hold a reference.
func ResetMissedBytesForTest() {
	missedBytes.reset()
	fileTailingActive.Store(false)
}
