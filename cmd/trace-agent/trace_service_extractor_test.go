package main

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/stretchr/testify/assert"
)

func TestTracerServiceExtractor(t *testing.T) {
	assert := assert.New(t)

	testChan := make(chan pb.ServicesMetadata)
	testExtractor := NewTraceServiceExtractor(testChan)

	trace := pb.Trace{
		&pb.Span{TraceID: 1, SpanID: 1, ParentID: 0, Service: "service-a", Type: "type-a"},
		&pb.Span{TraceID: 1, SpanID: 2, ParentID: 1, Service: "service-b", Type: "type-b"},
		&pb.Span{TraceID: 1, SpanID: 3, ParentID: 1, Service: "service-c", Type: "type-c"},
		&pb.Span{TraceID: 1, SpanID: 4, ParentID: 3, Service: "service-c", Type: "ignore"},
	}

	traceutil.ComputeTopLevel(trace)
	wt := agent.NewWeightedTrace(trace, trace[0])

	go func() {
		testExtractor.Process(wt)
	}()

	metadata := <-testChan

	// Result should only contain information derived from top-level spans
	assert.Equal(metadata, pb.ServicesMetadata{
		"service-a": {"app_type": "type-a"},
		"service-b": {"app_type": "type-b"},
		"service-c": {"app_type": "type-c"},
	})
}
