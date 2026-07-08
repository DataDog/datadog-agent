// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package trace provides trace collection and processing for serverless environments.
package trace

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

const (
	ddOriginTagName = "_dd.origin"
)

type spanModifier struct {
	tags     map[string]string
	ddOrigin string
}

// ModifySpan applies extra logic to the given span
func (s *spanModifier) ModifySpan(chunk *pb.TraceChunk, span *pb.Span) {
	// ensure all spans have tag _dd.origin in addition to span.Origin
	if origin := span.Meta[ddOriginTagName]; origin == "" {
		traceutil.SetMeta(span, ddOriginTagName, s.ddOrigin)
	}
	// Origin is canonically a chunk-level attribute (and stats aggregation reads
	// it from the chunk). The serverless cloud origin is only known to the agent,
	// not the tracer, so populate the chunk origin here when it is not already
	// set. Guarded so a tracer-provided origin is never overwritten.
	if chunk != nil && chunk.Origin == "" {
		chunk.Origin = s.ddOrigin
	}
}

// ModifySpanV1 is the V1 (idx) equivalent of ModifySpan.
func (s *spanModifier) ModifySpanV1(chunk *idx.InternalTraceChunk, span *idx.InternalSpan) {
	// ensure all spans have tag _dd.origin in addition to span.Origin
	if origin, ok := span.GetAttributeAsString(ddOriginTagName); !ok || origin == "" {
		span.SetStringAttribute(ddOriginTagName, s.ddOrigin)
	}
	// Origin is canonically a chunk-level attribute in the v1 representation (and
	// stats aggregation reads it from the chunk). The serverless cloud origin is
	// only known to the agent, not the tracer, so populate the chunk origin here
	// when it is not already set. Guarded so a tracer-provided origin is never
	// overwritten.
	if chunk != nil && chunk.Origin() == "" {
		chunk.SetOrigin(s.ddOrigin)
	}
}

// SetTags sets the tags to be used by the span modifier.
func (s *spanModifier) SetTags(tags map[string]string) {
	s.tags = tags
}
