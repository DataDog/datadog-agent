// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package trace provides trace collection and processing for serverless environments.
package trace

import (
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
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
func (s *spanModifier) ModifySpan(_ *pb.TraceChunk, span *pb.Span) {
	// ensure all spans have tag _dd.origin in addition to span.Origin
	if origin := span.Meta[ddOriginTagName]; origin == "" {
		traceutil.SetMeta(span, ddOriginTagName, s.ddOrigin)
	}
}

// SetTags sets the tags to be used by the span modifier.
func (s *spanModifier) SetTags(tags map[string]string) {
	s.tags = tags
}
