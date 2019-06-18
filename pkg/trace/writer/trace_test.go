package writer

import (
	"bytes"
	"compress/gzip"
	"math"
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
		traceWriter.Start()

		tracePkg := randomTracePackage(1, 1)
		size := calculateTracePayloadEstimatedSize([]*TracePackage{tracePkg})
		defer func(old int) {
			payloadFlushThreshold = old // reset original setting
		}(payloadFlushThreshold)
		payloadFlushThreshold = int(size + size + 1)

		// Send a few sampled traces through the writer
		sampledTraces := []*TracePackage{
			// these two will not trigger a flush, because they are
			// below the size threshold.
			tracePkg,
			tracePkg,
			// this one will trigger a flush of the previous two,
			// and of itself because of the big size.
			randomTracePackage(10, 1),
			// this one will also trigger a flush of itself.
			randomTracePackage(15, 1),
			// this one will be flushed at shutdown.
			tracePkg,
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

		// we should have 4 payloads based on the configured flush threshold for this test.
		assert.Len(testEndpoint.SuccessPayloads(), 4, "We expected 4 different payloads")
		assertPayloads(assert, traceWriter, expectedHeaders, sampledTraces, testEndpoint.SuccessPayloads())
	})
}

func calculateTracePayloadEstimatedSize(sampledTraces []*TracePackage) int {
	var size int
	for _, pkg := range sampledTraces {
		size += pkg.size()
	}
	return size
}

func calculateTracePayloadSize(sampledTraces []*TracePackage) int {
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
	return len(serialized)
}

func assertPayloads(
	assert *assert.Assertions,
	traceWriter *TraceWriter,
	expectedHeaders map[string]string,
	sampledTraces []*TracePackage,
	payloads []*payload,
) {
	var (
		expectedTraces []pb.Trace
		expectedEvents []*pb.Span
	)
	for _, sampledTrace := range sampledTraces {
		expectedTraces = append(expectedTraces, sampledTrace.Trace)

		for _, event := range sampledTrace.Events {
			expectedEvents = append(expectedEvents, event)
		}
	}

	var (
		expectedTraceIdx int
		expectedEventIdx int
	)
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
			size := pb.Trace(tracePayload.Transactions).Msgsize()
			for _, tt := range tracePayload.Traces {
				size += pb.Trace(tt.Spans).Msgsize()
			}
			assert.True(size <= payloadFlushThreshold)
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

	trace := testutil.GetTestTraces(1, numSpans, true)[0]

	events := make([]*pb.Span, 0, numEvents)

	for _, span := range trace[:numEvents] {
		events = append(events, span)
	}

	return &TracePackage{
		Trace:  trace,
		Events: events,
	}
}

func BenchmarkHandleSampledTrace(b *testing.B) {
	// ensure we never flush, as that would increase the scope of the benchmark
	defer func(old int) {
		payloadFlushThreshold = old
	}(payloadFlushThreshold)
	payloadFlushThreshold = math.MaxInt64
	tw := TraceWriter{sender: newMockSender()}
	pkg := randomTracePackage(2, 2)
	for i := 0; i < b.N; i++ {
		tw.handleSampledTrace(pkg)
	}
}
