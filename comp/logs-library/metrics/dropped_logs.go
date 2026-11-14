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

// DropReason says why the Agent gave up on logs it had already read. It is a closed
// set: each value maps to one remediation branch of the dropped-logs health issue,
// and the backend may key a remediation catalog on it, so values never change.
type DropReason string

const (
	// DropReasonAuthRejected is an HTTP 401 or 403 from the intake.
	DropReasonAuthRejected DropReason = "auth_rejected"
	// DropReasonPayloadTooLarge is an HTTP 413 from the intake.
	DropReasonPayloadTooLarge DropReason = "payload_too_large"
	// DropReasonBadRequest is an HTTP 400 from the intake.
	DropReasonBadRequest DropReason = "bad_request"
	// DropReasonCredentialUnavailable means no API key was available and none was coming.
	DropReasonCredentialUnavailable DropReason = "credential_unavailable"
	// DropReasonMessageTooLarge means one message did not fit in an empty batch.
	DropReasonMessageTooLarge DropReason = "message_too_large"
	// DropReasonEncodeFailed means a batch or frame could not be serialized or compressed.
	DropReasonEncodeFailed DropReason = "encode_failed"
	// DropReasonBufferFull means a destination that was falling behind had no room left.
	DropReasonBufferFull DropReason = "buffer_full"
	// DropReasonSendFailed is any other send error on a destination that does not retry.
	DropReasonSendFailed DropReason = "send_failed"
)

// DropTargetKind says which destinations lost the logs, which decides how bad the
// loss is: the primary endpoint is what the customer's own org reads.
type DropTargetKind string

const (
	// DropTargetPrimary is the main logs endpoint.
	DropTargetPrimary DropTargetKind = "primary"
	// DropTargetAdditional is a reliable additional endpoint (dual shipping).
	DropTargetAdditional DropTargetKind = "additional"
	// DropTargetBestEffort is an additional endpoint configured with is_reliable: false.
	DropTargetBestEffort DropTargetKind = "best_effort"
	// DropTargetMRF is a Multi-Region Failover endpoint.
	DropTargetMRF DropTargetKind = "mrf"
	// DropTargetAll means the logs were dropped before any destination got them.
	DropTargetAll DropTargetKind = "all"
)

// DropTarget names the destination that lost the logs. Host is empty for DropTargetAll.
type DropTarget struct {
	Host string
	Kind DropTargetKind
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

// DroppedLogsSummary is one (source, service, reason, destination) key's loss over
// the trailing window.
type DroppedLogsSummary struct {
	Source      string
	Service     string
	Reason      DropReason
	Destination string
	Kind        DropTargetKind
	Logs        int64
	Bytes       int64
	LastDropAt  time.Time
}

type droppedLogsKey struct {
	source      string
	service     string
	reason      DropReason
	destination string
	kind        DropTargetKind
}

// A hostname is at most 253 characters; anything longer was not one.
const droppedLogsMaxHostLen = 253

// droppedLogsOverflow folds the source and service of a key past the cap but keeps
// why and where it was dropped: remediation branches on those. Reasons, kinds and
// configured endpoints are all small fixed sets, so the overflow keys are too.
func droppedLogsOverflow(k droppedLogsKey) droppedLogsKey {
	k.source, k.service = lossOverflowLabel, lossOverflowLabel
	return k
}

func newDroppedLogsTracker(clk clock.Clock) *lossTracker[droppedLogsKey] {
	return newLossTracker(clk, droppedLogsOverflow)
}

// Process-wide because runner.HealthCheckFunc takes no arguments. Like TlmLogsDropped.
var droppedLogs = newDroppedLogsTracker(clock.New())

// RecordDroppedLogs records log lines the Agent read but will never deliver to
// target. Callers aggregate a payload per origin first so the tracker is locked once.
func RecordDroppedLogs(reason DropReason, target DropTarget, drops map[DropOrigin]DropCount) {
	recordDroppedLogs(droppedLogs, reason, target, drops)
}

func recordDroppedLogs(t *lossTracker[droppedLogsKey], reason DropReason, target DropTarget, drops map[DropOrigin]DropCount) {
	if len(drops) == 0 {
		return
	}
	host := boundNameTo(target.Host, droppedLogsMaxHostLen)
	losses := make(map[droppedLogsKey]lossCounts, len(drops))
	for origin, count := range drops {
		key := droppedLogsKey{
			source:      boundName(origin.Source),
			service:     boundName(origin.Service),
			reason:      reason,
			destination: host,
			kind:        target.Kind,
		}
		// Two origins can share a bounded prefix; add rather than overwrite.
		c := losses[key]
		c.count += count.Logs
		c.bytes += count.Bytes
		losses[key] = c
	}
	t.addAll(losses)
}

// DroppedLogsSnapshot returns in-window drops sorted by source, service, reason,
// then destination, so repeated checks render identically.
func DroppedLogsSnapshot() []DroppedLogsSummary {
	return droppedLogsSnapshot(droppedLogs)
}

func droppedLogsSnapshot(t *lossTracker[droppedLogsKey]) []DroppedLogsSummary {
	raw := t.collectAndPrune()
	summaries := make([]DroppedLogsSummary, 0, len(raw))
	for _, s := range raw {
		summaries = append(summaries, DroppedLogsSummary{
			Source:      s.key.source,
			Service:     s.key.service,
			Reason:      s.key.reason,
			Destination: s.key.destination,
			Kind:        s.key.kind,
			Logs:        s.counts.count,
			Bytes:       s.counts.bytes,
			LastDropAt:  s.lastLossAt,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		a, b := summaries[i], summaries[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Service != b.Service {
			return a.Service < b.Service
		}
		if a.Reason != b.Reason {
			return a.Reason < b.Reason
		}
		if a.Destination != b.Destination {
			return a.Destination < b.Destination
		}
		return a.Kind < b.Kind
	})
	return summaries
}
