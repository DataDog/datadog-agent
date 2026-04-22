// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"bytes"
	"sync"

	idx "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SpanModifierSetter is an interface for setting span modifiers
type SpanModifierSetter interface {
	SetSpanModifier(agent.SpanModifier)
}

// CloudRunJobsSpanModifier reparents user spans under the Cloud Run Job span.
//
// This modifier adopts the TraceID from the first span seen (regardless of ParentID)
// and only reparents root spans (ParentID=0). Child spans are left unmodified to
// preserve the original span hierarchy.
//
// In trace propagation scenarios where all spans have non-zero ParentIDs, the job span
// will be "floating" (unparented) within the trace. This is an acceptable trade-off
// as all spans will still share the same TraceID for log-trace correlation.
type CloudRunJobsSpanModifier struct {
	mu       sync.Mutex              // Protects adopted, stopped, and jobChunk mutations
	adopted  bool                    // Whether we've adopted a TraceID
	stopped  bool                    // Whether further jobChunk mutation is forbidden (set at shutdown)
	jobChunk *idx.InternalTraceChunk // Reference to job span (used to update its TraceID)

	// Cached values from jobSpan for use in ModifySpan
	jobSpanID uint64
}

// NewCloudRunJobsSpanModifier creates a new span modifier for Cloud Run Jobs
// The provided jobChunk must be a chunk with a single span, the job span.
func NewCloudRunJobsSpanModifier(jobChunk *idx.InternalTraceChunk) *CloudRunJobsSpanModifier {
	return &CloudRunJobsSpanModifier{
		jobChunk:  jobChunk,
		jobSpanID: jobChunk.Spans[0].SpanID(),
	}
}

// Shutdown blocks until any in-flight ModifySpan call completes and then marks the modifier
// so that subsequent ModifySpan calls will not mutate the underlying jobChunk. This is used
// at shutdown to ensure jobChunk is safe to hand off to the trace pipeline without racing
// with user-span processing.
func (m *CloudRunJobsSpanModifier) Shutdown() {
	m.mu.Lock()
	m.stopped = true
	m.mu.Unlock()
}

// ModifySpan reparents user spans under the Cloud Run Job span
func (m *CloudRunJobsSpanModifier) ModifySpan(chunk *idx.InternalTraceChunk, span *idx.InternalSpan) {
	// Skip job span itself
	if span.Name() == "gcp.run.job.task" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// After Shutdown, the jobChunk has been handed to the trace pipeline; do not mutate it.
	if m.stopped {
		return
	}

	// Adopt TraceID from first span seen (regardless of whether it's a root span)
	if !m.adopted {
		m.adopted = true

		// Also update the jobSpan's TraceID so it matches when submitted
		if m.jobChunk != nil {
			m.jobChunk.TraceID = make([]byte, len(chunk.TraceID))
			copy(m.jobChunk.TraceID, chunk.TraceID)
		}
	} else if m.jobChunk != nil && len(chunk.TraceID) > len(m.jobChunk.TraceID) && sameTraceID(m.jobChunk.TraceID, chunk.TraceID) {
		// Upgrade the adopted trace ID in place: a 64-bit span arrived first and a 128-bit span
		// from the same logical trace arrived later (carrying _dd.p.tid). Extend to the full width
		// so the job span keeps matching subsequent 128-bit arrivals.
		upgraded := make([]byte, len(chunk.TraceID))
		copy(upgraded, chunk.TraceID)
		m.jobChunk.TraceID = upgraded
	}

	if m.jobChunk != nil && !sameTraceID(m.jobChunk.TraceID, chunk.TraceID) {
		log.Debugf("Cloud Run Job: span has different TraceID (span=%s)", span.Name())
		return
	}

	// Only reparent root spans (ParentID == 0) that match adopted trace
	if span.ParentID() == 0 {
		span.SetParentID(m.jobSpanID)
	}
}

// sameTraceID reports whether a and b share the same low 64 bits of a trace ID. High bits
// (the upper 8 bytes of a 128-bit ID) are informational — missing high bits are treated as
// zero so a 64-bit span is considered the same trace as a later 128-bit span whose low bits
// match. Unexpected lengths return false to stay safe.
func sameTraceID(a, b []byte) bool {
	lo := func(x []byte) ([]byte, bool) {
		switch len(x) {
		case 8:
			return x, true
		case 16:
			return x[8:], true
		default:
			return nil, false
		}
	}
	aLow, ok := lo(a)
	if !ok {
		return false
	}
	bLow, ok := lo(b)
	if !ok {
		return false
	}
	return bytes.Equal(aLow, bLow)
}
