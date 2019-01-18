// +build !windows

package writer

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/stretchr/testify/assert"
)

func TestPeriodicFlush(t *testing.T) {
	assert := assert.New(t)

	testFlushPeriod := 100 * time.Millisecond

	// Create a trace writer, its incoming channel and the endpoint that receives the payloads
	traceWriter, traceChannel, testEndpoint, _ := testTraceWriter()
	// Periodically flushing every 100ms
	traceWriter.conf.FlushPeriod = testFlushPeriod
	traceWriter.Start()

	// Send a single trace that does not go over the span limit
	testSampledTrace := randomTracePackage(2, 2)
	traceChannel <- testSampledTrace

	// Wait for twice the flush period
	time.Sleep(2 * testFlushPeriod)

	// Check that we received 1 payload that was flushed due to periodical flushing and that it matches the
	// data we sent to the writer
	receivedPayloads := testEndpoint.SuccessPayloads()
	expectedHeaders := map[string]string{
		"X-Datadog-Reported-Languages": strings.Join(info.Languages(), "|"),
		"Content-Type":                 "application/x-protobuf",
		"Content-Encoding":             "gzip",
	}
	assert.Len(receivedPayloads, 1, "We expected 1 payload")
	assertPayloads(assert, traceWriter, expectedHeaders, []*TracePackage{testSampledTrace},
		testEndpoint.SuccessPayloads())

	// Wrap up
	close(traceChannel)
	traceWriter.Stop()
}

func TestPeriodicStats(t *testing.T) {
	assert := assert.New(t)

	testFlushPeriod := 100 * time.Millisecond

	// Create a trace writer, its incoming channel and the endpoint that receives the payloads
	traceWriter, traceChannel, testEndpoint, statsClient := testTraceWriter()
	traceWriter.conf.FlushPeriod = 100 * time.Millisecond
	traceWriter.conf.UpdateInfoPeriod = 100 * time.Millisecond
	traceWriter.conf.MaxSpansPerPayload = 10
	traceWriter.Start()

	var (
		expectedNumPayloads       int64
		expectedNumSpans          int64
		expectedNumTraces         int64
		expectedNumBytes          int64
		expectedNumErrors         int64
		expectedMinNumRetries     int64
		expectedNumSingleMaxSpans int64
	)

	// Send a bunch of sampled traces that should go together in a single payload
	payload1SampledTraces := []*TracePackage{
		randomTracePackage(2, 0),
		randomTracePackage(2, 0),
		randomTracePackage(2, 0),
	}
	expectedNumPayloads++
	expectedNumSpans += 6
	expectedNumTraces += 3
	expectedNumBytes += calculateTracePayloadSize(payload1SampledTraces)

	for _, sampledTrace := range payload1SampledTraces {
		traceChannel <- sampledTrace
	}

	// Send a single trace that goes over the span limit
	payload2SampledTraces := []*TracePackage{
		randomTracePackage(20, 0),
	}
	expectedNumPayloads++
	expectedNumSpans += 20
	expectedNumTraces++
	expectedNumBytes += calculateTracePayloadSize(payload2SampledTraces)
	expectedNumSingleMaxSpans++

	for _, sampledTrace := range payload2SampledTraces {
		traceChannel <- sampledTrace
	}

	// Wait for twice the flush period
	time.Sleep(2 * testFlushPeriod)

	// Send a third payload with other 3 traces with an errored out endpoint
	testEndpoint.SetError(fmt.Errorf("non retriable error"))
	payload3SampledTraces := []*TracePackage{
		randomTracePackage(2, 0),
		randomTracePackage(2, 0),
		randomTracePackage(2, 0),
	}

	expectedNumErrors++
	expectedNumTraces += 3
	expectedNumSpans += 6
	expectedNumBytes += calculateTracePayloadSize(payload3SampledTraces)

	for _, sampledTrace := range payload3SampledTraces {
		traceChannel <- sampledTrace
	}

	// Wait for twice the flush period
	time.Sleep(2 * testFlushPeriod)

	// And then send a fourth payload with other 3 traces with an errored out endpoint but retriable
	testEndpoint.SetError(&retriableError{
		err:      fmt.Errorf("non retriable error"),
		endpoint: testEndpoint,
	})
	payload4SampledTraces := []*TracePackage{
		randomTracePackage(2, 0),
		randomTracePackage(2, 0),
		randomTracePackage(2, 0),
	}

	expectedMinNumRetries++
	expectedNumTraces += 3
	expectedNumSpans += 6
	expectedNumBytes += calculateTracePayloadSize(payload4SampledTraces)

	for _, sampledTrace := range payload4SampledTraces {
		traceChannel <- sampledTrace
	}

	// Wait for twice the flush period to see at least one retry
	time.Sleep(2 * testFlushPeriod)

	// Close and stop
	close(traceChannel)
	traceWriter.Stop()

	// Then we expect some counts to have been sent to the stats client for each update tick (there should have been
	// at least 3 ticks)
	countSummaries := statsClient.GetCountSummaries()

	// Payload counts
	payloadSummary := countSummaries["datadog.trace_agent.trace_writer.payloads"]
	assert.True(len(payloadSummary.Calls) >= 3, "There should have been multiple payload count calls")
	assert.Equal(expectedNumPayloads, payloadSummary.Sum)

	// Traces counts
	tracesSummary := countSummaries["datadog.trace_agent.trace_writer.traces"]
	assert.True(len(tracesSummary.Calls) >= 3, "There should have been multiple traces count calls")
	assert.Equal(expectedNumTraces, tracesSummary.Sum)

	// Spans counts
	spansSummary := countSummaries["datadog.trace_agent.trace_writer.spans"]
	assert.True(len(spansSummary.Calls) >= 3, "There should have been multiple spans count calls")
	assert.Equal(expectedNumSpans, spansSummary.Sum)

	// Bytes counts
	bytesSummary := countSummaries["datadog.trace_agent.trace_writer.bytes"]
	assert.True(len(bytesSummary.Calls) >= 3, "There should have been multiple bytes count calls")
	// FIXME: Is GZIP non-deterministic? Why won't equal work here?
	assert.True(math.Abs(float64(expectedNumBytes-bytesSummary.Sum)) < 100., "Bytes should be within expectations")

	// Retry counts
	retriesSummary := countSummaries["datadog.trace_agent.trace_writer.retries"]
	assert.True(len(retriesSummary.Calls) >= 2, "There should have been multiple retries count calls")
	assert.True(retriesSummary.Sum >= expectedMinNumRetries)

	// Error counts
	errorsSummary := countSummaries["datadog.trace_agent.trace_writer.errors"]
	assert.True(len(errorsSummary.Calls) >= 3, "There should have been multiple errors count calls")
	assert.Equal(expectedNumErrors, errorsSummary.Sum)

	// Single trace max spans
	singleMaxSpansSummary := countSummaries["datadog.trace_agent.trace_writer.single_max_spans"]
	assert.True(len(singleMaxSpansSummary.Calls) >= 3, "There should have been multiple single max spans count calls")
	assert.Equal(expectedNumSingleMaxSpans, singleMaxSpansSummary.Sum)
}
