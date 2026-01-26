// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"bytes"
	"sync"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
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
	mu      sync.Mutex // Protects adopted and adoptedTraceID
	adopted bool       // Whether we've adopted a TraceID
	jobSpan *pb.Span   // Reference to job span (used to update its TraceID)

	// Cached values from jobSpan for use in ModifySpan
	jobSpanID      uint64
	adoptedTraceID []byte // The 128-bit trace ID we've adopted (nil until adopted)
}

// NewCloudRunJobsSpanModifier creates a new span modifier for Cloud Run Jobs
func NewCloudRunJobsSpanModifier(jobSpan *pb.Span) *CloudRunJobsSpanModifier {
	return &CloudRunJobsSpanModifier{
		jobSpan:   jobSpan,
		jobSpanID: jobSpan.SpanID,
	}
}

// ModifySpan reparents user spans under the Cloud Run Job span
func (m *CloudRunJobsSpanModifier) ModifySpan(chunk *idx.InternalTraceChunk, span *idx.InternalSpan) {
	// Skip job span itself
	if span.Name() == "gcp.run.job.task" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Adopt TraceID from first span seen (regardless of whether it's a root span)
	if !m.adopted {
		m.adopted = true
		// Store the 128-bit trace ID from the chunk
		m.adoptedTraceID = make([]byte, len(chunk.TraceID))
		copy(m.adoptedTraceID, chunk.TraceID)

		// Also update the jobSpan's TraceID so it matches when submitted
		if m.jobSpan != nil {
			m.jobSpan.TraceID = chunk.LegacyTraceID()
			// Handle 128-bit trace ID high bits if present
			if highBits, ok := chunk.GetAttributeAsString("_dd.p.tid"); ok {
				if m.jobSpan.Meta == nil {
					m.jobSpan.Meta = make(map[string]string)
				}
				m.jobSpan.Meta["_dd.p.tid"] = highBits
			}
		}
	}

	// Check full 128-bit trace ID match
	if !bytes.Equal(m.adoptedTraceID, chunk.TraceID) {
		log.Debugf("Cloud Run Job: span has different TraceID (span=%s)", span.Name())
		return
	}

	// Upgrade job span to 128-bit trace ID if we see high bits we didn't have before.
	// This handles the case where a 64-bit span arrives first, then a 128-bit span
	// from the same trace arrives later with _dd.p.tid.
	traceutil.UpgradeTraceID(m.jobSpan, span)

	// Only reparent root spans (ParentID == 0) that match adopted trace
	if span.ParentID() == 0 {
		span.SetParentID(m.jobSpanID)
	}
}
