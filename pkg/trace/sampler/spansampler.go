// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

// ApplySpanSampling searches chunk for spans that have a span sampling tag set.
// If it finds such spans, then it replaces chunk's spans with only those spans,
// and sets the chunk's sampling priority to "user keep." Tracers that wish to
// keep certain spans even when the trace is dropped will set the appropriate
// tags on the spans to be kept.
// Do not call ApplySpanSampling on a chunk that the other samplers have
// decided to keep. Doing so might wrongfully remove spans from a kept trace.
func ApplySpanSampling(chunk *pb.TraceChunk) (applied bool) {
	// Find the first span in chunk.Spans that has the span sampling tag set, if any.
	first := -1
	for i, span := range chunk.Spans {
		if _, ok := traceutil.GetMetric(span, KeySpanSamplingMechanism); ok {
			first = i
			break
		}
	}
	if first == -1 {
		// No span sampling tags → no span sampling.
		return false
	}

	// Keep only those spans that have a span sampling tag.
	sampledSpans := []*pb.Span{chunk.Spans[first]}
	for _, span := range chunk.Spans[first+1:] {
		if _, ok := traceutil.GetMetric(span, KeySpanSamplingMechanism); ok {
			sampledSpans = append(sampledSpans, span)
		}
	}

	// Keep what we selected above.
	chunk.Spans = sampledSpans
	chunk.Priority = int32(PriorityUserKeep)
	chunk.DroppedTrace = false

	return true
}
