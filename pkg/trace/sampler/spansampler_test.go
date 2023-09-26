// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

// TestNoTagNoTouch verifies that if none of the spans passed to
// ApplySpanSampling have the span sampling tag, then ApplySpanSampling does not
// modify its argument at all.
func TestNoTagNoTouch(t *testing.T) {
	original := &pb.TraceChunk{
		Spans: []*pb.Span{
			{
				Service:  "testsvc",
				Name:     "parent",
				TraceID:  1,
				SpanID:   1,
				Start:    time.Now().Add(-time.Second).UnixNano(),
				Duration: time.Millisecond.Nanoseconds(),
			},
			{
				Service:  "testsvc",
				Name:     "child",
				TraceID:  1,
				SpanID:   2,
				Start:    time.Now().Add(-time.Second).UnixNano(),
				Duration: time.Millisecond.Nanoseconds(),
			},
		},
	}

	pt := &traceutil.ProcessedTrace{TraceChunk: original}
	modified := SingleSpanSampling(pt)
	assert.False(t, modified)
	assert.True(t, proto.Equal(pt.TraceChunk, original))
}

// TestTagCausesInPlaceFilterAndKeep verifies that the presence of a span
// sampling tag in any of the spans passed to GetSingleSpanSampledSpans causes the
// argument of GetSingleSpanSampledSpans to be modified in the following ways:
//   - The chunk is filtered to contain only those spans that have the span
//     sampling tag.
//   - The chunk's sampling priority is PriorityUserKeep.
//   - The chunk is not marked as dropped.
func TestTagCausesInPlaceFilterAndKeep(t *testing.T) {
	// spanSamplingMetrics returns a map of numeric tags that contains the span
	// sampling metric (numeric tag) that tracers use to indicate that the span
	// should be kept by the span sampler.
	spanSamplingMetrics := func() map[string]float64 {
		metrics := make(map[string]float64, 1)
		// The value of this metric does not matter to the trace agent, but per
		// the single span ingestion control RFC it will be 8.
		metrics[KeySpanSamplingMechanism] = 8
		return metrics
	}

	original := &pb.TraceChunk{
		Spans: []*pb.Span{
			{
				Service:  "testsvc",
				Name:     "parent",
				TraceID:  1,
				SpanID:   1,
				Start:    time.Now().Add(-time.Second).UnixNano(),
				Duration: time.Millisecond.Nanoseconds(),
			},
			{
				Service:  "testsvc",
				Name:     "child",
				TraceID:  1,
				SpanID:   2,
				ParentID: 1,
				// Keep this one.
				Metrics:  spanSamplingMetrics(),
				Start:    time.Now().Add(-time.Second).UnixNano(),
				Duration: time.Millisecond.Nanoseconds(),
			},
			{
				Service:  "testsvc",
				Name:     "grandchild",
				TraceID:  1,
				SpanID:   3,
				ParentID: 2,
				Start:    time.Now().Add(-time.Second).UnixNano(),
				Duration: time.Millisecond.Nanoseconds(),
			},
			{
				Service:  "testsvc",
				Name:     "great-grandchild",
				TraceID:  1,
				SpanID:   4,
				ParentID: 3,
				// Keep this one.
				Metrics:  spanSamplingMetrics(),
				Start:    time.Now().Add(-time.Second).UnixNano(),
				Duration: time.Millisecond.Nanoseconds(),
			},
		},
	}

	ptChunk := proto.Clone(original).(*pb.TraceChunk)
	pt := &traceutil.ProcessedTrace{TraceChunk: ptChunk}
	modified := SingleSpanSampling(pt)
	assert.True(t, modified)
	assert.False(t, pt.TraceChunk.DroppedTrace)
	assert.Equal(t, int32(PriorityUserKeep), pt.TraceChunk.Priority)
	assert.Len(t, pt.TraceChunk.Spans, 2)
	// child
	assert.Equal(t, original.Spans[1], pt.TraceChunk.Spans[0])
	// great-grandchild
	assert.Equal(t, original.Spans[3], pt.TraceChunk.Spans[1])
}
