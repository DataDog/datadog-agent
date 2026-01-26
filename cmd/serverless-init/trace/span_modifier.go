// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"sync"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
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
	mu      sync.Mutex // Protects adopted
	adopted bool       // Whether we've adopted a TraceID (stored in jobSpan)
	jobSpan *pb.Span   // Reference to job span (holds adopted TraceID)
}

// NewCloudRunJobsSpanModifier creates a new span modifier for Cloud Run Jobs
func NewCloudRunJobsSpanModifier(jobSpan *pb.Span) *CloudRunJobsSpanModifier {
	return &CloudRunJobsSpanModifier{
		jobSpan: jobSpan,
	}
}

// ModifySpan reparents user spans under the Cloud Run Job span
func (m *CloudRunJobsSpanModifier) ModifySpan(_ *pb.TraceChunk, span *pb.Span) {
	// Skip job span itself
	if span.Name == "gcp.run.job.task" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Adopt TraceID from first span seen (regardless of whether it's a root span)
	if !m.adopted {
		m.adopted = true
		if m.jobSpan != nil {
			traceutil.CopyTraceID(m.jobSpan, span)
		}
	}

	// Check full 128-bit trace ID match (low 64 bits + high 64 bits in _dd.p.tid)
	if !traceutil.SameTraceID(m.jobSpan, span) {
		log.Debugf("Cloud Run Job: span has different TraceID (span=%s)", span.Name)
		return
	}

	// Upgrade job span to 128-bit trace ID if we see high bits we didn't have before.
	// This handles the case where a 64-bit span arrives first, then a 128-bit span
	// from the same trace arrives later with _dd.p.tid.
	traceutil.UpgradeTraceID(m.jobSpan, span)

	// Only reparent root spans (ParentID == 0) that match adopted trace
	if span.ParentID == 0 {
		span.ParentID = m.jobSpan.SpanID
	}
}
