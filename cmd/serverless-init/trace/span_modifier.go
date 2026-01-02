// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"sync"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// metaTraceIDHigh is the meta tag key for the high 64 bits of a 128-bit trace ID.
	// This is used by Datadog tracers to propagate the upper bits of the trace ID.
	metaTraceIDHigh = "_dd.p.tid"
)

// copyTraceID copies the trace ID (both low and high 64 bits) from src to dst.
// This handles both 64-bit and 128-bit trace IDs by copying the TraceID field
// and the _dd.p.tid meta tag (if present).
func copyTraceID(dst, src *pb.Span) {
	dst.TraceID = src.TraceID
	if tidHigh, ok := src.Meta[metaTraceIDHigh]; ok {
		if dst.Meta == nil {
			dst.Meta = make(map[string]string)
		}
		dst.Meta[metaTraceIDHigh] = tidHigh
	}
}

// SpanModifierSetter is an interface for setting span modifiers
type SpanModifierSetter interface {
	SetSpanModifier(agent.SpanModifier)
}

// CloudRunJobsSpanModifier reparents user spans under the Cloud Run Job span.
//
// This modifier preserves span hierarchies by only reparenting root spans (ParentID=0)
// and leaving child spans unmodified. The job span adopts the first user trace's TraceID
// to maintain log-trace correlation.
//
// IMPORTANT: This implementation assumes no trace propagation into the Cloud Run Job.
// If trace context was propagated into the job (e.g., via HTTP headers), the spans generated
// by the job code might not have root spans (ParentID=0) since they would already be part of
// a parent trace. This is currently not an issue because there is no standard approach to
// trace propagation for Cloud Run Jobs - it is a limitation of the Cloud Run Jobs architecture
// and APIs.
type CloudRunJobsSpanModifier struct {
	mu             sync.Mutex // Protects adoptedTraceID
	adoptedTraceID uint64     // First user TraceID seen (0 = none)
	jobSpan        *pb.Span   // Reference to job span for updating TraceID
}

// NewCloudRunJobsSpanModifier creates a new span modifier for Cloud Run Jobs
func NewCloudRunJobsSpanModifier(jobSpan *pb.Span) *CloudRunJobsSpanModifier {
	return &CloudRunJobsSpanModifier{
		jobSpan: jobSpan,
	}
}

// ModifySpan reparents user spans under the Cloud Run Job span
func (m *CloudRunJobsSpanModifier) ModifySpan(_ *pb.TraceChunk, span *pb.Span) {
	// 1. Skip job span itself
	if span.Name == "gcp.run.job.task" {
		return
	}

	// 2. Skip child spans (preserve hierarchy)
	// NOTE: This assumes no trace propagation into the job. If trace context was propagated,
	// all spans might have non-zero ParentIDs and we wouldn't adopt any TraceID. This is
	// acceptable since there's no standard trace propagation mechanism for Cloud Run Jobs.
	if span.ParentID != 0 {
		return // Never modify child spans
	}

	// 3. Handle root spans (ParentID == 0) with thread-safe adoption
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.adoptedTraceID == 0 {
		// First root span - adopt its TraceID
		m.adoptedTraceID = span.TraceID

		// Update job span to match (for log-trace correlation)
		// This copies both low and high 64 bits for 128-bit trace ID support
		if m.jobSpan != nil {
			copyTraceID(m.jobSpan, span)
		}
	}

	// Reparent root spans that match adopted trace
	if span.TraceID == m.adoptedTraceID {
		span.ParentID = m.jobSpan.SpanID
	} else {
		// Different trace - log for observability
		log.Debugf("Cloud Run Job: Ignoring root span with unexpected TraceID=%016x (adopted=%016x, span=%s)",
			span.TraceID, m.adoptedTraceID, span.Name)
	}
}
