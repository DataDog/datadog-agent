// +build !windows

package writer

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
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

	defer func(old int) {
		payloadFlushThreshold = old // reset original setting
	}(payloadFlushThreshold)

	testFlushPeriod := 100 * time.Millisecond

	// Create a trace writer, its incoming channel and the endpoint that receives the payloads
	traceWriter, traceChannel, testEndpoint, statsClient := testTraceWriter()
	traceWriter.conf.FlushPeriod = testFlushPeriod
	traceWriter.conf.UpdateInfoPeriod = 100 * time.Millisecond
	traceWriter.Start()

	want := make(map[string]int) // maps metric name to expected sum

	payload1 := []*TracePackage{
		randomTracePackage(10, 0),
		randomTracePackage(10, 0),
		randomTracePackage(10, 0),
	}

	// this payload will fit
	payloadFlushThreshold = int(calculateTracePayloadEstimatedSize(payload1))

	for _, trace := range payload1 {
		traceChannel <- trace
	}

	want["spans"] = 30
	want["traces"] = len(payload1)
	want["bytes_estimated"] = calculateTracePayloadEstimatedSize(payload1)
	want["bytes_uncompressed"] = calculateTracePayloadSize(payload1)

	pkg1 := randomTracePackage(90, 0)
	traceChannel <- pkg1 // pushes us over AND flushes itself due to huge size

	want["payloads"] = 2
	want["traces"]++
	want["spans"] += 90
	want["bytes_estimated"] += calculateTracePayloadEstimatedSize([]*TracePackage{pkg1})
	want["bytes_uncompressed"] += calculateTracePayloadSize([]*TracePackage{pkg1})
	want["single_max_size"]++

	// this will fit again
	for _, trace := range payload1 {
		traceChannel <- trace
	}

	time.Sleep(2 * testFlushPeriod) // flush

	want["payloads"]++
	want["traces"] += len(payload1)
	want["spans"] += 30 // spans in payload1
	want["bytes_estimated"] += calculateTracePayloadEstimatedSize(payload1)
	want["bytes_uncompressed"] += calculateTracePayloadSize(payload1)

	// trigger some errors
	testEndpoint.SetError(errors.New("non retriable error"))

	for _, trace := range payload1 {
		traceChannel <- trace
	}

	time.Sleep(2 * testFlushPeriod) // flush

	want["errors"] = 1
	want["traces"] += len(payload1)
	want["spans"] += 30 // spans in payload1
	want["bytes_estimated"] += calculateTracePayloadEstimatedSize(payload1)
	want["bytes_uncompressed"] += calculateTracePayloadSize(payload1)

	// trigger retriable error
	testEndpoint.SetError(&retriableError{
		err:      errors.New("retriable error"),
		endpoint: testEndpoint,
	})

	payload4 := []*TracePackage{
		randomTracePackage(2, 0),
		randomTracePackage(2, 0),
	}

	payloadFlushThreshold = payload4[0].size() - 1 // each entry will cause a flush

	for _, sampledTrace := range payload4 {
		traceChannel <- sampledTrace
	}

	close(traceChannel)
	traceWriter.Stop()

	want["retries"] = 1
	want["traces"] += len(payload4)
	want["spans"] += 4
	want["single_max_size"] += 2
	want["bytes_estimated"] += calculateTracePayloadEstimatedSize(payload4)
	want["bytes_uncompressed"] += calculateTracePayloadSize([]*TracePackage{payload4[0]})
	want["bytes_uncompressed"] += calculateTracePayloadSize([]*TracePackage{payload4[1]})

	// Close and stop

	expectStats := summaryCompareFunc(assert, statsClient)
	for key, sum := range want {
		expectStats("datadog.trace_agent.trace_writer."+key, 5, sum)
	}
}

func summaryCompareFunc(assert *assert.Assertions, statsClient *testutil.TestStatsClient) func(name string, calls int, sum int) {
	summaries := statsClient.GetCountSummaries()
	return func(name string, minCalls int, sum int) {
		summary, ok := summaries[name]
		assert.True(ok, fmt.Sprintf("%s not found", name))
		calls := len(summary.Calls)
		assert.True(calls >= minCalls, fmt.Sprintf("%s: expected at least %d calls, got %d", name, minCalls, calls))
		assert.EqualValues(sum, summary.Sum, fmt.Sprintf("%s: expected %d, got %d", name, sum, summary.Sum))
	}
}
