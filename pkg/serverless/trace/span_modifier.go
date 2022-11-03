// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"github.com/DataDog/datadog-agent/pkg/serverless/trace/inferredspan"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type SpanModifier struct {
	tags   map[string]string
	Origin string
}

// Process applies extra logic to the given span
func (s *SpanModifier) ModifySpan(span *pb.Span) {
	if span.Service == "aws.lambda" && s.tags["service"] != "" {
		// service name could be incorrectly set to 'aws.lambda' in datadog lambda libraries
		span.Service = s.tags["service"]
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
	// For agent-in-container, add root spans
	if s.Origin == "cloudrun" || s.Origin == "containerapp" {
		if span.ParentID != 0 {
			return
		}
		span.Meta["root_span"] = "true"
	}
}
