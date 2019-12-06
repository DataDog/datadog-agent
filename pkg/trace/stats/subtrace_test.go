// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package stats

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/stretchr/testify/assert"
)

func TestExtractTopLevelSubtracesWithSimpleTrace(t *testing.T) {
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
	subtraces := ExtractTopLevelSubtraces(trace, trace[0])

	assert.Equal(len(expected), len(subtraces))

	subtracesMap := make(map[*pb.Span]Subtrace)
	for _, s := range subtraces {
		subtracesMap[s.Root] = s
	}

	for _, s := range expected {
		assert.ElementsMatch(s.Trace, subtracesMap[s.Root].Trace)
	}
}

func TestExtractTopLevelSubtracesShouldNotIgnoreLeafTopLevel(t *testing.T) {
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
		{trace[2], []*pb.Span{}},
	}

	traceutil.ComputeTopLevel(trace)
	subtraces := ExtractTopLevelSubtraces(trace, trace[0])

	assert.Equal(len(expected), len(subtraces))

	subtracesMap := make(map[*pb.Span]Subtrace)
	for _, s := range subtraces {
		subtracesMap[s.Root] = s
	}

	for _, s := range expected {
		assert.ElementsMatch(s.Trace, subtracesMap[s.Root].Trace)
	}
}

func TestExtractTopLevelSubtracesWorksInSpiteOfCycles(t *testing.T) {
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
	subtraces := ExtractTopLevelSubtraces(trace, trace[0])

	assert.Equal(len(expected), len(subtraces))

	subtracesMap := make(map[*pb.Span]Subtrace)
	for _, s := range subtraces {
		subtracesMap[s.Root] = s
	}

	for _, s := range expected {
		assert.ElementsMatch(s.Trace, subtracesMap[s.Root].Trace)
	}
}
