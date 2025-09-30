// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"github.com/DataDog/datadog-agent/pkg/agentless/trace/inferredspan"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	ddOriginTagName  = "_dd.origin"
	ddOriginTagValue = "agentless"
)

type spanModifier struct {
	tags     map[string]string
	ddOrigin string
}

// ModifySpan applies extra logic to the given span
func (s *spanModifier) ModifySpan(_ *pb.TraceChunk, span *pb.Span) {
	// Apply service name override if configured
	if s.tags["service"] != "" && span.Service == "" {
		span.Service = s.tags["service"]
	}

	// ensure all spans have tag _dd.origin in addition to span.Origin
	if origin := span.Meta[ddOriginTagName]; origin == "" {
		traceutil.SetMeta(span, ddOriginTagName, s.ddOrigin)
	}

	if inferredspan.CheckIsInferredSpan(span) {
		log.Debug("Detected a managed service span, filtering out function tags")

		// filter out existing function tags inside span metadata
		spanMetadataTags := span.Meta
		if spanMetadataTags != nil {
			spanMetadataTags = inferredspan.FilterFunctionTags(spanMetadataTags)
			span.Meta = spanMetadataTags
		}
	}
}

// SetTags sets the tags to be used by the span modifier.
func (s *spanModifier) SetTags(tags map[string]string) {
	s.tags = tags
}
