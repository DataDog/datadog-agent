package writer

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	writerconfig "github.com/DataDog/datadog-agent/pkg/trace/writer/config"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
)

var testHostName = "testhost"
var testEnv = "testenv"

func TestTraceWriter(t *testing.T) {
	t.Run("payload flushing", func(t *testing.T) {
		assert := assert.New(t)

		// Create a trace writer, its incoming channel and the endpoint that receives the payloads
		traceWriter, traceChannel, testEndpoint, _ := testTraceWriter()
		// Set a maximum of 4 spans per payload
		traceWriter.conf.MaxSpansPerPayload = 4
		traceWriter.Start()

		// Send a few sampled traces through the writer
		sampledTraces := []*TracePackage{
			// These 2 should be grouped together in a single payload
			randomTracePackage(1, 1),
			randomTracePackage(1, 1),
			// This one should be on its own in a single payload
			randomTracePackage(3, 1),
			// This one should be on its own in a single payload
			randomTracePackage(5, 1),
			// This one should be on its own in a single payload
			randomTracePackage(1, 1),
		}
		for _, sampledTrace := range sampledTraces {
			traceChannel <- sampledTrace
		}

		// Stop the trace writer to force everything to flush
		close(traceChannel)
		traceWriter.Stop()

		expectedHeaders := map[string]string{
			"X-Datadog-Reported-Languages": strings.Join(info.Languages(), "|"),
			"Content-Type":                 "application/x-protobuf",
			"Content-Encoding":             "gzip",
		}

		// Ensure that the number of payloads and their contents match our expectations. The MaxSpansPerPayload we
		// set to 4 at the beginning should have been respected whenever possible.
		assert.Len(testEndpoint.SuccessPayloads(), 4, "We expected 4 different payloads")
		assertPayloads(assert, traceWriter, expectedHeaders, sampledTraces, testEndpoint.SuccessPayloads())
	})

	t.Run("periodic flushing", func(t *testing.T) {
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
	})

	t.Run("periodic stats reporting", func(t *testing.T) {
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
	})
}

func calculateTracePayloadSize(sampledTraces []*TracePackage) int64 {
	apiTraces := make([]*pb.APITrace, len(sampledTraces))

	for i, trace := range sampledTraces {
		apiTraces[i] = traceutil.APITrace(trace.Trace)
	}

	tracePayload := pb.TracePayload{
		HostName: testHostName,
		Env:      testEnv,
		Traces:   apiTraces,
	}

	serialized, _ := proto.Marshal(&tracePayload)

	compressionBuffer := bytes.Buffer{}
	gz, err := gzip.NewWriterLevel(&compressionBuffer, gzip.BestSpeed)

	if err != nil {
		panic(err)
	}

	_, err = gz.Write(serialized)
	gz.Close()

	if err != nil {
		panic(err)
	}

	return int64(len(compressionBuffer.Bytes()))
}

func assertPayloads(assert *assert.Assertions, traceWriter *TraceWriter, expectedHeaders map[string]string,
	sampledTraces []*TracePackage, payloads []*payload) {

	var expectedTraces []pb.Trace
	var expectedEvents []*pb.Span

	for _, sampledTrace := range sampledTraces {
		expectedTraces = append(expectedTraces, sampledTrace.Trace)

		for _, event := range sampledTrace.Events {
			expectedEvents = append(expectedEvents, event)
		}
	}

	var expectedTraceIdx int
	var expectedEventIdx int

	for _, payload := range payloads {
		assert.Equal(expectedHeaders, payload.headers, "Payload headers should match expectation")

		var tracePayload pb.TracePayload
		payloadBuffer := bytes.NewBuffer(payload.bytes)
		gz, err := gzip.NewReader(payloadBuffer)
		assert.NoError(err, "Gzip reader should work correctly")
		uncompressedBuffer := bytes.Buffer{}
		_, err = uncompressedBuffer.ReadFrom(gz)
		gz.Close()
		assert.NoError(err, "Should uncompress ok")
		assert.NoError(proto.Unmarshal(uncompressedBuffer.Bytes(), &tracePayload), "Unmarshalling should work correctly")

		assert.Equal(testEnv, tracePayload.Env, "Envs should match")
		assert.Equal(testHostName, tracePayload.HostName, "Hostnames should match")

		numSpans := 0

		for _, seenAPITrace := range tracePayload.Traces {
			numSpans += len(seenAPITrace.Spans)

			if !assert.True(proto.Equal(traceutil.APITrace(expectedTraces[expectedTraceIdx]), seenAPITrace),
				"Unmarshalled trace should match expectation at index %d", expectedTraceIdx) {
				return
			}

			expectedTraceIdx++
		}

		for _, seenTransaction := range tracePayload.Transactions {
			numSpans++

			if !assert.True(proto.Equal(expectedEvents[expectedEventIdx], seenTransaction),
				"Unmarshalled transaction should match expectation at index %d", expectedTraceIdx) {
				return
			}

			expectedEventIdx++
		}

		// If there's more than 1 trace or transaction in this payload, don't let it go over the limit. Otherwise,
		// a single trace+transaction combination is allows to go over the limit.
		if len(tracePayload.Traces) > 1 || len(tracePayload.Transactions) > 1 {
			assert.True(numSpans <= traceWriter.conf.MaxSpansPerPayload)
		}
	}
}

func testTraceWriter() (*TraceWriter, chan *TracePackage, *testEndpoint, *testutil.TestStatsClient) {
	payloadChannel := make(chan *TracePackage)
	conf := &config.AgentConfig{
		Hostname:          testHostName,
		DefaultEnv:        testEnv,
		TraceWriterConfig: writerconfig.DefaultTraceWriterConfig(),
	}
	traceWriter := NewTraceWriter(conf, payloadChannel)
	testEndpoint := &testEndpoint{}
	traceWriter.sender.setEndpoint(testEndpoint)
	testStatsClient := metrics.Client.(*testutil.TestStatsClient)
	testStatsClient.Reset()

	return traceWriter, payloadChannel, testEndpoint, testStatsClient
}

func randomTracePackage(numSpans, numEvents int) *TracePackage {
	if numSpans < numEvents {
		panic("can't have more events than spans in a RandomSampledTrace")
	}

	trace := testutil.GetTestTrace(1, numSpans, true)[0]

	events := make([]*pb.Span, 0, numEvents)

	for _, span := range trace[:numEvents] {
		events = append(events, span)
	}

	return &TracePackage{
		Trace:  trace,
		Events: events,
	}
}
