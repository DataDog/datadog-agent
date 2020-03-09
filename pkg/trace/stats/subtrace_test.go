// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package stats

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/stretchr/testify/assert"
)

func TestExtractSubtracesWithSimpleTrace(t *testing.T) {
	assert := assert.New(t)

	trace := pb.Trace{
		&pb.Span{SpanID: 1, ParentID: 0, Service: "s1"},
		&pb.Span{SpanID: 2, ParentID: 1, Service: "s2"},
		&pb.Span{SpanID: 3, ParentID: 2, Service: "s2"},
		&pb.Span{SpanID: 4, ParentID: 3, Service: "s2"},
		&pb.Span{SpanID: 5, ParentID: 1, Service: "s1"},
	}

	expected := []Subtrace{
		{trace[0], trace},
		{trace[1], []*pb.Span{trace[1], trace[2], trace[3]}},
	}

	traceutil.ComputeTopLevel(trace)
	subtraces := ExtractSubtraces(trace, trace[0])

	assert.Equal(len(expected), len(subtraces))

	subtracesMap := make(map[*pb.Span]Subtrace)
	for _, s := range subtraces {
		subtracesMap[s.Root] = s
	}

	for _, s := range expected {
		assert.ElementsMatch(s.Trace, subtracesMap[s.Root].Trace)
	}
}

func TestExtractSubtracesShouldIgnoreLeafTopLevel(t *testing.T) {
	assert := assert.New(t)

	trace := pb.Trace{
		&pb.Span{SpanID: 1, ParentID: 0, Service: "s1"},
		&pb.Span{SpanID: 2, ParentID: 1, Service: "s2"},
		&pb.Span{SpanID: 3, ParentID: 2, Service: "s2"},
		&pb.Span{SpanID: 4, ParentID: 1, Service: "s3"},
	}

	expected := []Subtrace{
		{trace[0], trace},
		{trace[1], []*pb.Span{trace[1], trace[2]}},
	}

	traceutil.ComputeTopLevel(trace)
	subtraces := ExtractSubtraces(trace, trace[0])

	assert.Equal(len(expected), len(subtraces))

	subtracesMap := make(map[*pb.Span]Subtrace)
	for _, s := range subtraces {
		subtracesMap[s.Root] = s
	}

	for _, s := range expected {
		assert.ElementsMatch(s.Trace, subtracesMap[s.Root].Trace)
	}
}

func TestExtractSubtracesWorksInSpiteOfCycles(t *testing.T) {
	assert := assert.New(t)

	trace := pb.Trace{
		&pb.Span{SpanID: 1, ParentID: 3, Service: "s1"},
		&pb.Span{SpanID: 2, ParentID: 1, Service: "s2"},
		&pb.Span{SpanID: 3, ParentID: 2, Service: "s2"},
	}

	expected := []Subtrace{
		{trace[0], trace},
		{trace[1], []*pb.Span{trace[1], trace[2]}},
	}

	traceutil.ComputeTopLevel(trace)
	subtraces := ExtractSubtraces(trace, trace[0])

	assert.Equal(len(expected), len(subtraces))

	subtracesMap := make(map[*pb.Span]Subtrace)
	for _, s := range subtraces {
		subtracesMap[s.Root] = s
	}

	for _, s := range expected {
		assert.ElementsMatch(s.Trace, subtracesMap[s.Root].Trace)
	}
}

// TestExtractSubtracesMeasuredSpans tests that subtraces are correctly
// extracted for measured spans.
func TestExtractSubtracesMeasuredSpans(t *testing.T) {
	assert := assert.New(t)

	trace := pb.Trace{
		&pb.Span{SpanID: 1, ParentID: 0, Service: "s1"},
		&pb.Span{SpanID: 2, ParentID: 1, Service: "s1"},
		// measured span has two child leaf spans
		&pb.Span{SpanID: 3, ParentID: 2, Service: "s1", Metrics: map[string]float64{"_dd.measured": 1.0}},
		&pb.Span{SpanID: 4, ParentID: 3, Service: "s2"},
		&pb.Span{SpanID: 5, ParentID: 3, Service: "s3"},
	}

	expected := []Subtrace{
		{trace[0], trace},
		{trace[2], []*pb.Span{trace[2], trace[3], trace[4]}},
	}

	traceutil.ComputeTopLevel(trace)
	subtraces := ExtractSubtraces(trace, trace[0])

	assert.Equal(len(expected), len(subtraces))

	subtracesMap := make(map[*pb.Span]Subtrace)
	for _, s := range subtraces {
		subtracesMap[s.Root] = s
	}

	for _, s := range expected {
		assert.ElementsMatch(s.Trace, subtracesMap[s.Root].Trace)
	}

}
