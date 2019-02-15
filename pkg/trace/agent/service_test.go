package agent

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/stretchr/testify/assert"
)

func TestServiceMapper(t *testing.T) {
	assert := assert.New(t)

	mapper, in, out := testMapper()
	mapper.Start()
	defer mapper.Stop()

	input := pb.ServicesMetadata{"service-a": {"app_type": "type-a"}}
	in <- input
	output := <-out

	// When the service is ingested for the first time, we simply propagate it
	// to the output channel and add an entry to the cache map
	assert.Equal(input, output)

	// This entry will result in a cache-hit and therefore will be filtered out
	in <- pb.ServicesMetadata{"service-a": {"app_type": "SOMETHING_DIFFERENT"}}

	// This represents a new service and thus will be cached and propagated to the outbound channel
	newService := pb.ServicesMetadata{"service-b": {"app_type": "type-b"}}
	in <- newService
	output = <-out

	assert.Equal(newService, output)
}

func TestCachePolicy(t *testing.T) {
	assert := assert.New(t)

	mapper, in, out := testMapper()
	mapper.Start()
	defer mapper.Stop()

	input := pb.ServicesMetadata{"service-a": {"app_type": "type-a"}}
	in <- input
	output := <-out

	// A new service entry should propagate the metadata the the outbound channel
	assert.Equal(input, output)

	// A service entry that is already in cache should only be propagated IF:
	// - Current version does NOT have "app"
	// - New version DOES have "app"

	// This first attempt won't be propagated to the writer
	firstAttempt := pb.ServicesMetadata{"service-a": {"app_type": "FIRST_ATTEMPT"}}
	in <- firstAttempt

	// But this second will
	secondAttempt := pb.ServicesMetadata{"service-a": {"app_type": "SECOND_ATTEMPT", "app": "app-a"}}
	in <- secondAttempt

	output = <-out
	assert.Equal(secondAttempt, output)
}

func testMapper() (mapper *ServiceMapper, in, out chan pb.ServicesMetadata) {
	in = make(chan pb.ServicesMetadata, 1)
	out = make(chan pb.ServicesMetadata, 1)
	mapper = NewServiceMapper(in, out)

	return mapper, in, out
}

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
	wt := stats.NewWeightedTrace(trace, trace[0])

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
