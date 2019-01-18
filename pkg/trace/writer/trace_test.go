package writer

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"

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
