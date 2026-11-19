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

// Rotation loss keeps the loss tracker's bounds; these names say so where the tests
// read them.
const (
	missedBytesMaxKeys       = lossMaxKeys
	missedBytesOverflowLabel = lossOverflowLabel
	missedBytesMaxNameLen    = lossMaxNameLen
)

// MissedBytesSummary is one (source, service) tuple's ongoing incident: what log
// rotations closed early lost since StartedAt.
type MissedBytesSummary struct {
	Source     string
	Service    string
	Bytes      int64
	Rotations  int64
	StartedAt  time.Time
	LastLossAt time.Time
}

// missedBytesTracker keys losses on source and service only; reason and
// destination stay empty.
type missedBytesTracker struct {
	*lossTracker
}

func newMissedBytesTracker(clk clock.Clock) *missedBytesTracker {
	return &missedBytesTracker{newLossTracker(clk, func(lossKey) lossKey {
		return lossKey{source: missedBytesOverflowLabel, service: missedBytesOverflowLabel}
	})}
}

// record counts one rotation that lost bytes. A rotation that lost nothing is not a
// loss and is not counted.
func (t *missedBytesTracker) record(source, service string, bytes int64) {
	if bytes <= 0 {
		return
	}
	t.add(lossKey{
		source:  boundNameTo(source, missedBytesMaxNameLen),
		service: boundNameTo(service, missedBytesMaxNameLen),
	}, lossCounts{count: 1, bytes: bytes})
}

// collectAndPrune returns ongoing incidents sorted by source then service.
func (t *missedBytesTracker) collectAndPrune() []MissedBytesSummary {
	raw := t.lossTracker.collectAndPrune()
	summaries := make([]MissedBytesSummary, 0, len(raw))
	for _, s := range raw {
		summaries = append(summaries, MissedBytesSummary{
			Source:     s.key.source,
			Service:    s.key.service,
			Bytes:      s.counts.bytes,
			Rotations:  s.counts.count,
			StartedAt:  s.startedAt,
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

// MissedBytesSnapshot returns ongoing incidents, sorted by source then service.
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
