// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sort"
	"sync/atomic"
	"time"

	"github.com/benbjohnson/clock"
)

// The rotation-loss tracker uses the shared loss bounds; the aliases keep its
// tests readable.
const (
	missedBytesWindow        = lossWindow
	missedBytesBucketSize    = lossBucketSize
	missedBytesMaxBuckets    = lossMaxBuckets
	missedBytesMaxKeys       = lossMaxKeys
	missedBytesOverflowLabel = lossOverflowLabel
	missedBytesMaxNameLen    = lossMaxNameLen
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

// missedBytesTracker counts, per tuple, the bytes lost and the rotations that lost
// them. The shared tracker's count is the rotation count.
type missedBytesTracker struct {
	*lossTracker[missedBytesKey]
}

func newMissedBytesTracker(clk clock.Clock) *missedBytesTracker {
	return &missedBytesTracker{newLossTracker(clk, func(missedBytesKey) missedBytesKey {
		return missedBytesKey{source: missedBytesOverflowLabel, service: missedBytesOverflowLabel}
	})}
}

func (t *missedBytesTracker) record(source, service string, bytes int64) {
	// A rotation that lost nothing is not a rotation that lost data.
	if bytes <= 0 {
		return
	}
	t.add(missedBytesKey{source: boundName(source), service: boundName(service)},
		lossCounts{count: 1, bytes: bytes})
}

// collectAndPrune returns one summary per tuple still inside the window, sorted by
// source then service, and drops those that aged out.
func (t *missedBytesTracker) collectAndPrune() []MissedBytesSummary {
	raw := t.lossTracker.collectAndPrune()
	summaries := make([]MissedBytesSummary, 0, len(raw))
	for _, s := range raw {
		summaries = append(summaries, MissedBytesSummary{
			Source:     s.key.source,
			Service:    s.key.service,
			Bytes:      s.counts.bytes,
			Rotations:  s.counts.count,
			LastLossAt: s.lastLossAt,
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
