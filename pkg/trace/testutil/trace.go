// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"math/rand"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

// genNextLevel generates a new level for the trace tree structure,
// having maxSpans as the max number of spans for this level
func genNextLevel(prevLevel []*pb.Span, maxSpans int) []*pb.Span {
	var spans []*pb.Span
	numSpans := rand.Intn(maxSpans) + 1

	// the spans have to be "nested" in the previous level
	// choose randomly spans from prev level
	chosenSpans := rand.Perm(len(prevLevel))
	// cap to a random number > 1
	maxParentSpans := rand.Intn(len(prevLevel))
	if maxParentSpans == 0 {
		maxParentSpans = 1
	}
	chosenSpans = chosenSpans[:maxParentSpans]

	// now choose a random amount of spans per chosen span
	// total needs to be numSpans
	for i, prevIdx := range chosenSpans {
		prev := prevLevel[prevIdx]

		var childSpans int
		value := numSpans - (len(chosenSpans) - i)
		if i == len(chosenSpans)-1 || value < 1 {
			childSpans = numSpans
		} else {
			childSpans = rand.Intn(value)
		}
		numSpans -= childSpans

		timeLeft := prev.Duration

		// create the spans
		curSpans := make([]*pb.Span, 0, childSpans)
		for j := 0; j < childSpans && timeLeft > 0; j++ {
			news := RandomSpan()
			traceutil.CopyTraceID(news, prev)
			news.ParentID = prev.SpanID

			// distribute durations in prev span
			// random start
			randStart := rand.Int63n(timeLeft)
			news.Start = prev.Start + randStart
			// random duration
			timeLeft -= randStart
			news.Duration = rand.Int63n(timeLeft) + 1
			timeLeft -= news.Duration

			curSpans = append(curSpans, news)
		}

		spans = append(spans, curSpans...)
	}

	return spans
}

// RandomTrace generates a random trace with a depth from 1 to
// maxLevels of spans. Each level has at most maxSpans items.
func RandomTrace(maxLevels, maxSpans int) pb.Trace {
	t := pb.Trace{RandomSpan()}

	prevLevel := t
	maxDepth := 1 + rand.Intn(maxLevels)

	for i := 0; i < maxDepth; i++ {
		if len(prevLevel) > 0 {
			prevLevel = genNextLevel(prevLevel, maxSpans)
			t = append(t, prevLevel...)
		}
	}

	return t
}

// RandomTraceChunk generates a random trace chunk with a depth from 1 to
// maxLevels of spans. Each level has at most maxSpans items.
func RandomTraceChunk(maxLevels, maxSpans int) *pb.TraceChunk {
	return &pb.TraceChunk{
		Priority: int32(rand.Intn(3)),
		Origin:   "cloudrun",
		Spans:    RandomTrace(maxLevels, maxSpans),
	}
}

// GetTestTraces returns a []Trace that is composed by “traceN“ number
// of traces, each one composed by “size“ number of spans.
func GetTestTraces(traceN, size int, realisticIDs bool) pb.Traces {
	traces := pb.Traces{}

	r := rand.New(rand.NewSource(42))

	for i := 0; i < traceN; i++ {
		// Calculate a trace ID which is predictable (this is why we seed)
		// but still spreads on a wide spectrum so that, among other things,
		// sampling algorithms work in a realistic way.
		traceID := r.Uint64()

		trace := pb.Trace{}
		for j := 0; j < size; j++ {
			span := GetTestSpan()
			if realisticIDs {
				// Need to have different span IDs else traces are rejected
				// because they are not correct (indeed, a trace with several
				// spans boasting the same span ID is not valid)
				span.SpanID += uint64(j)
				span.TraceID = traceID
			}
			trace = append(trace, span)
		}
		traces = append(traces, trace)
	}
	return traces
}

// GetTestTracesV1 returns a []Trace that is composed by “traceN“ number
// of traces, each one composed by “size“ number of spans.
func GetTestTracesV1(traceN, size int, realisticIDs bool) *idx.InternalTracerPayload {
	strings := idx.NewStringTable()
	traces := &idx.InternalTracerPayload{Strings: strings}

	r := rand.New(rand.NewSource(42))

	for i := 0; i < traceN; i++ {
		traceID := make([]byte, 16)
		r.Read(traceID)
		chunk := idx.InternalTraceChunk{
			Strings: strings,
			// Calculate a trace ID which is predictable (this is why we seed)
			// but still spreads on a wide spectrum so that, among other things,
			// sampling algorithms work in a realistic way.
			TraceID: traceID,
		}

		spans := []*idx.InternalSpan{}
		for j := 0; j < size; j++ {
			span := GetTestSpanV1(strings)
			if realisticIDs {
				// Need to have different span IDs else traces are rejected
				// because they are not correct (indeed, a trace with several
				// spans boasting the same span ID is not valid)
				span.SetSpanID(span.SpanID() + uint64(j))
			}
			spans = append(spans, span)
		}
		chunk.Spans = spans
		traces.Chunks = append(traces.Chunks, &chunk)
	}
	return traces
}

// GetTestTraceChunksV1 returns a []TraceChunk that is composed by “traceN“ number
// of traces, each one composed by “size“ number of spans.
func GetTestTraceChunksV1(traceN, size int, realisticIDs bool) []*idx.InternalTraceChunk {
	traces := GetTestTracesV1(traceN, size, realisticIDs)
	traceChunks := make([]*idx.InternalTraceChunk, 0, len(traces.Chunks))
	traceChunks = append(traceChunks, traces.Chunks...)
	return traceChunks
}

// GetTestTraceChunks returns a []TraceChunk that is composed by “traceN“ number
// of traces, each one composed by “size“ number of spans.
func GetTestTraceChunks(traceN, size int, realisticIDs bool) []*pb.TraceChunk {
	traces := GetTestTraces(traceN, size, realisticIDs)
	traceChunks := make([]*pb.TraceChunk, 0, len(traces))
	for _, trace := range traces {
		traceChunks = append(traceChunks, &pb.TraceChunk{
			Spans: trace,
		})
	}
	return traceChunks
}

// TraceChunkWithSpan wraps a `span` with pb.TraceChunk
func TraceChunkWithSpan(span *pb.Span) *pb.TraceChunk {
	return &pb.TraceChunk{
		Spans:    []*pb.Span{span},
		Priority: int32(sampler.PriorityNone),
		Tags:     make(map[string]string),
	}
}

// TraceChunkWithSpans wraps `spans` with pb.TraceChunk
func TraceChunkWithSpans(spans []*pb.Span) *pb.TraceChunk {
	return &pb.TraceChunk{
		Spans:    spans,
		Priority: int32(sampler.PriorityNone),
	}
}

// TraceChunkWithSpanAndPriority wraps a `span` and `priority` with pb.TraceChunk
func TraceChunkWithSpanAndPriority(span *pb.Span, priority int32) *pb.TraceChunk {
	return &pb.TraceChunk{
		Spans:    []*pb.Span{span},
		Priority: priority,
	}
}

// TraceChunkV1WithSpanAndPriority wraps a `span` and `priority` with pb.TraceChunk
func TraceChunkV1WithSpanAndPriority(span *idx.InternalSpan, priority int32) *idx.InternalTraceChunk {
	return &idx.InternalTraceChunk{
		TraceID:    []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Strings:    span.Strings,
		Spans:      []*idx.InternalSpan{span},
		Priority:   priority,
		Attributes: map[uint32]*idx.AnyValue{},
	}
}

// TraceChunkWithSpansAndPriority wraps `spans` and `priority` with pb.TraceChunk
func TraceChunkWithSpansAndPriority(spans []*pb.Span, priority int32) *pb.TraceChunk {
	return &pb.TraceChunk{
		Spans:    spans,
		Priority: priority,
	}
}

// TracerPayloadWithChunk wraps `chunk` with pb.TraceChunk
func TracerPayloadWithChunk(chunk *pb.TraceChunk) *pb.TracerPayload {
	return &pb.TracerPayload{
		Chunks: []*pb.TraceChunk{chunk},
	}
}

// TracerPayloadV1WithChunk wraps `chunk` with idx.InternalTracerPayload
func TracerPayloadV1WithChunk(chunk *idx.InternalTraceChunk) *idx.InternalTracerPayload {
	return &idx.InternalTracerPayload{
		Strings: chunk.Strings,
		Chunks:  []*idx.InternalTraceChunk{chunk},
	}
}

// TracerPayloadWithChunks wraps `chunks` with pb.TraceChunk
func TracerPayloadWithChunks(chunks []*pb.TraceChunk) *pb.TracerPayload {
	return &pb.TracerPayload{
		Chunks: chunks,
	}
}

// TracerPayloadV1WithChunks wraps `chunks` with idx.InternalTracerPayload
func TracerPayloadV1WithChunks(chunks []*idx.InternalTraceChunk) *idx.InternalTracerPayload {
	return &idx.InternalTracerPayload{
		Strings: chunks[0].Strings,
		Chunks:  chunks,
	}
}
