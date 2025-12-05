// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	idx "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
)

// SpanModifierSetter is an interface for setting span modifiers
type SpanModifierSetter interface {
	SetSpanModifier(agent.SpanModifier)
}

// CloudRunJobsSpanModifier reparents user spans under the Cloud Run Job span
type CloudRunJobsSpanModifier struct {
	jobTraceID uint64
	jobSpanID  uint64
}

// NewCloudRunJobsSpanModifier creates a new span modifier for Cloud Run Jobs
func NewCloudRunJobsSpanModifier(traceID, spanID uint64) *CloudRunJobsSpanModifier {
	return &CloudRunJobsSpanModifier{
		jobTraceID: traceID,
		jobSpanID:  spanID,
	}
}

// ModifySpan reparents user spans under the Cloud Run Job span
func (m *CloudRunJobsSpanModifier) ModifySpan(chunk *idx.InternalTraceChunk, span *idx.InternalSpan) {
	// Only modify tracer-generated spans, not our own job span
	if span.Name() == "gcp.run.job.task" {
		return
	}

	// Reparent the span under the job span
	chunk.SetLegacyTraceID(m.jobTraceID)
	span.SetParentID(m.jobSpanID)
}
