package writer

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
)

const (
	testHostname = "agent-test-host"
	testEnv      = "testing"
)

func TestTraceWriter(t *testing.T) {
	srv := newTestServer()
	cfg := &config.AgentConfig{
		Hostname:   testHostname,
		DefaultEnv: testEnv,
		Endpoints: []*config.Endpoint{{
			APIKey: "123",
			Host:   srv.URL,
		}},
	}

	t.Run("ok", func(t *testing.T) {
		testSpans := []*SampledSpans{
			randomSampledSpans(20, 8),
			randomSampledSpans(10, 0),
			randomSampledSpans(40, 5),
		}
		// Use a flush threshold that allows the first two entries to not overflow,
		// but overflow on the third.
		defer useFlushThreshold(testSpans[0].size() + testSpans[1].size() + 10)()
		in := make(chan *SampledSpans, 100)
		tw := NewTraceWriter(cfg, in)
		go tw.Run()
		for _, ss := range testSpans {
			in <- ss
		}
		tw.Stop()
		// One payload flushes due to overflowing the threshold, and the second one
		// because of stop.
		assert.Equal(t, 2, srv.Accepted())
		payloadContains(t, srv.Payloads()[0], testSpans[:2])
		payloadContains(t, srv.Payloads()[1], testSpans[2:])
	})
}

// useFlushThreshold sets n as the number of bytes to be used as the flush threshold
// and returns a function to restore it.
func useFlushThreshold(n int) func() {
	old := payloadFlushThreshold
	payloadFlushThreshold = n
	return func() { payloadFlushThreshold = old }
}

// randomSampledSpans returns a set of spans sampled spans and events events.
func randomSampledSpans(spans, events int) *SampledSpans {
	realisticIDs := true
	trace := testutil.GetTestTraces(1, spans, realisticIDs)[0]
	return &SampledSpans{
		Trace:  trace,
		Events: trace[:events],
	}
}

// payloadContains checks that the given payload contains the given set of sampled spans.
func payloadContains(t *testing.T, p *payload, sampledSpans []*SampledSpans) {
	assert := assert.New(t)
	gzipr, err := gzip.NewReader(bytes.NewReader(p.body))
	assert.NoError(err)
	slurp, err := ioutil.ReadAll(gzipr)
	assert.NoError(err)
	var payload pb.TracePayload
	err = proto.Unmarshal(slurp, &payload)
	assert.NoError(err)
	assert.Equal(payload.HostName, testHostname)
	assert.Equal(payload.Env, testEnv)
	for _, ss := range sampledSpans {
		var found bool
		for _, trace := range payload.Traces {
			if reflect.DeepEqual(trace.Spans, ([]*pb.Span)(ss.Trace)) {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("payload didn't contain given traces")
		}
		for _, event := range ss.Events {
			assert.Contains(payload.Transactions, event)
		}
	}
}
