// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sort"
	"time"

	"github.com/benbjohnson/clock"
)

// DropReason says why the Agent gave up on logs it had already read. It is a
// closed set, one value per remediation, so the dropped-logs issue can tell the
// customer what to fix.
type DropReason string

const (
	// DropReasonAuth is an HTTP 401 or 403, or no API key to send with.
	DropReasonAuth DropReason = "auth"
	// DropReasonTooLarge is an HTTP 413, or one message too large for a batch.
	DropReasonTooLarge DropReason = "too_large"
	// DropReasonMalformed is an HTTP 400, or a batch or frame that failed to encode.
	DropReasonMalformed DropReason = "malformed"
	// DropReasonBufferFull means a destination that was falling behind had no room left.
	DropReasonBufferFull DropReason = "buffer_full"
	// DropReasonSendFailed is any other send error on a destination that does not retry.
	DropReasonSendFailed DropReason = "send_failed"
)

// DropTarget names the destination that lost the logs. Main is true for the main
// logs endpoint, which is what the customer's own org reads; Host is empty for
// logs dropped before any destination got them, which also count as Main.
type DropTarget struct {
	Host string
	Main bool
}

// DropOrigin is the log source and service that dropped log lines came from.
type DropOrigin struct {
	Source  string
	Service string
}

// DropCount is how many log lines, and how many bytes of them, were dropped.
type DropCount struct {
	Logs  int64
	Bytes int64
}

// DroppedLogsSummary is one (source, service, reason, destination) key's ongoing
// incident: what it lost since StartedAt.
type DroppedLogsSummary struct {
	Source     string
	Service    string
	Reason     DropReason
	Target     DropTarget
	Logs       int64
	Bytes      int64
	StartedAt  time.Time
	LastDropAt time.Time
}

// A hostname is at most 253 characters; anything longer was not one.
const droppedLogsMaxHostLen = 253

// droppedLogsOverflow folds the source and service of a key past the cap but keeps
// why and where it was dropped: remediation depends on those. Reasons and
// configured endpoints are small fixed sets, so the overflow keys are too.
func droppedLogsOverflow(k lossKey) lossKey {
	k.source, k.service = lossOverflowLabel, lossOverflowLabel
	return k
}

func newDroppedLogsTracker(clk clock.Clock) *lossTracker {
	return newLossTracker(clk, droppedLogsOverflow)
}

// Process-wide because runner.HealthCheckFunc takes no arguments. Like TlmLogsDropped.
var droppedLogs = newDroppedLogsTracker(clock.New())

// RecordDroppedLogs records log lines the Agent read but will never deliver to
// target. Callers aggregate a payload per origin first so the tracker is locked once.
func RecordDroppedLogs(reason DropReason, target DropTarget, drops map[DropOrigin]DropCount) {
	recordDroppedLogs(droppedLogs, reason, target, drops)
}

func recordDroppedLogs(t *lossTracker, reason DropReason, target DropTarget, drops map[DropOrigin]DropCount) {
	if len(drops) == 0 {
		return
	}
	host := boundNameTo(target.Host, droppedLogsMaxHostLen)
	losses := make(map[lossKey]lossCounts, len(drops))
	for origin, count := range drops {
		key := lossKey{
			source:  boundNameTo(origin.Source, lossMaxNameLen),
			service: boundNameTo(origin.Service, lossMaxNameLen),
			reason:  reason,
			host:    host,
			main:    target.Main,
		}
		// Two origins can share a bounded prefix; add rather than overwrite.
		c := losses[key]
		c.count += count.Logs
		c.bytes += count.Bytes
		losses[key] = c
	}
	t.addAll(losses)
}

// DroppedLogsSnapshot returns ongoing incidents, largest first, ties broken on the
// key so repeated checks render identically.
func DroppedLogsSnapshot() []DroppedLogsSummary {
	return droppedLogsSnapshot(droppedLogs)
}

func droppedLogsSnapshot(t *lossTracker) []DroppedLogsSummary {
	raw := t.collectAndPrune()
	summaries := make([]DroppedLogsSummary, 0, len(raw))
	for _, s := range raw {
		summaries = append(summaries, DroppedLogsSummary{
			Source:     s.key.source,
			Service:    s.key.service,
			Reason:     s.key.reason,
			Target:     DropTarget{Host: s.key.host, Main: s.key.main},
			Logs:       s.counts.count,
			Bytes:      s.counts.bytes,
			StartedAt:  s.startedAt,
			LastDropAt: s.lastLossAt,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		a, b := summaries[i], summaries[j]
		if a.Logs != b.Logs {
			return a.Logs > b.Logs
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Service != b.Service {
			return a.Service < b.Service
		}
		if a.Reason != b.Reason {
			return a.Reason < b.Reason
		}
		return a.Target.Host < b.Target.Host
	})
	return summaries
}
