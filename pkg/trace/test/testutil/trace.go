// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package testutil

import (
	"math/rand"

	"github.com/DataDog/datadog-agent/pkg/trace/traces"
)

// genNextLevel generates a new level for the trace tree structure,
// having maxSpans as the max number of spans for this level
func genNextLevel(prevLevel []traces.Span, maxSpans int) []traces.Span {
	var spans []traces.Span
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

		timeLeft := prev.Duration()

		// create the spans
		curSpans := make([]traces.Span, 0, childSpans)
		for j := 0; j < childSpans && timeLeft > 0; j++ {
			news := RandomSpan()
			news.SetTraceID(prev.TraceID())
			news.SetParentID(prev.SpanID())

			// distribute durations in prev span
			// random start
			randStart := rand.Int63n(timeLeft)
			news.SetStart(prev.Start() + randStart)
			// random duration
			timeLeft -= randStart
			news.SetDuration(rand.Int63n(timeLeft) + 1)
			timeLeft -= news.Duration()

			curSpans = append(curSpans, news)
		}

		spans = append(spans, curSpans...)
	}

	return spans
}

// RandomTrace generates a random trace with a depth from 1 to
// maxLevels of spans. Each level has at most maxSpans items.
func RandomTrace(maxLevels, maxSpans int) traces.Trace {
	t := traces.NewTrace([]traces.Span{RandomSpan()})

	prevLevel := t
	maxDepth := 1 + rand.Intn(maxLevels)

	for i := 0; i < maxDepth; i++ {
		if len(prevLevel.Spans) > 0 {
			prevLevel = traces.NewTrace(genNextLevel(prevLevel.Spans, maxSpans))
			t.Spans = append(t.Spans, prevLevel.Spans...)
		}
	}

	return t
}

// GetTestTraces returns a []Trace that is composed by ``traceN`` number
// of traces, each one composed by ``size`` number of spans.
func GetTestTraces(traceN, size int, realisticIDs bool) []traces.Trace {
	var (
		gen = make([]traces.Trace, 0, traceN)
		r   = rand.New(rand.NewSource(42))
	)
	for i := 0; i < traceN; i++ {
		// Calculate a trace ID which is predictable (this is why we seed)
		// but still spreads on a wide spectrum so that, among other things,
		// sampling algorithms work in a realistic way.
		traceID := r.Uint64()

		trace := traces.Trace{}
		for j := 0; j < size; j++ {
			span := GetTestSpan()
			if realisticIDs {
				// Need to have different span IDs else traces are rejected
				// because they are not correct (indeed, a trace with several
				// spans boasting the same span ID is not valid)
				span.SetSpanID(span.SpanID() + uint64(j))
				span.SetTraceID(traceID)
			}
			trace.Spans = append(trace.Spans, span)
		}
		gen = append(gen, trace)
	}
	return gen
}
