// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package writer

import (
	"bytes"
	"io"
	"net/url"
	"runtime"
	"testing"

	compression "github.com/DataDog/datadog-agent/comp/trace/compression/def"
	gzip "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"
	zstd "github.com/DataDog/datadog-agent/comp/trace/compression/impl-zstd"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// mock sampler
type MockSampler struct {
	TargetTPS float64
	Enabled   bool
}

func (s MockSampler) IsEnabled() bool {
	return s.Enabled
}

func (s MockSampler) GetTargetTPS() float64 {
	return s.TargetTPS
}

var mockSampler = MockSampler{TargetTPS: 5, Enabled: true}

func TestTraceWriterV1(t *testing.T) {
	testCases := []struct {
		compressor compression.Component
	}{
		{gzip.NewComponent()},
		{zstd.NewComponent()},
	}
	for _, tc := range testCases {
		t.Run("encoding:"+tc.compressor.Encoding(), func(t *testing.T) {
			srv := newTestServer()
			defer srv.Close()
			cfg := &config.AgentConfig{
				Hostname:   testHostname,
				DefaultEnv: testEnv,
				Endpoints: []*config.Endpoint{{
					APIKey: "123",
					Host:   srv.URL,
				}},
				TraceWriter: &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40, FlushPeriodSeconds: 1_000},
			}
			testSpans := []*SampledChunksV1{
				randomSampledSpansV1(20, 8),
				randomSampledSpansV1(10, 0),
				randomSampledSpansV1(40, 5),
			}
			tw := NewTraceWriterV1(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, tc.compressor)
			for _, ss := range testSpans {
				tw.WriteChunksV1(ss)
			}
			tw.Stop()
			// All payloads should be flushed on stop
			assert.GreaterOrEqual(t, srv.Accepted(), 1)
			payloadsContainV1(t, srv.Payloads(), testSpans, tc.compressor)
		})
	}
}

// useFlushThreshold sets n as the number of bytes to be used as the flush threshold
// and returns a function to restore it.
func useFlushThreshold(n int) func() {
	old := MaxPayloadSize
	MaxPayloadSize = n
	return func() { MaxPayloadSize = old }
}

func TestTraceWriterV1PayloadSplitting(t *testing.T) {
	compressor := zstd.NewComponent()
	srv := newTestServer()
	defer srv.Close()
	cfg := &config.AgentConfig{
		Hostname:   testHostname,
		DefaultEnv: testEnv,
		Endpoints: []*config.Endpoint{{
			APIKey: "123",
			Host:   srv.URL,
		}},
		TraceWriter:         &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40, FlushPeriodSeconds: 1_000},
		SynchronousFlushing: true,
	}

	// Create a tracer payload with 3 chunks
	strings := idx.NewStringTable()
	chunk1 := &idx.InternalTraceChunk{
		Strings:  strings,
		Priority: 1,
		TraceID:  []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Spans:    []*idx.InternalSpan{testutil.GetTestSpanV1(strings)},
	}
	chunk2 := &idx.InternalTraceChunk{
		Strings:  strings,
		Priority: 1,
		TraceID:  []byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17},
		Spans:    []*idx.InternalSpan{testutil.GetTestSpanV1(strings)},
	}
	chunk3 := &idx.InternalTraceChunk{
		Strings:  strings,
		Priority: 1,
		TraceID:  []byte{3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18},
		Spans:    []*idx.InternalSpan{testutil.GetTestSpanV1(strings)},
	}
	payload := &idx.InternalTracerPayload{
		Strings: strings,
		Chunks:  []*idx.InternalTraceChunk{chunk1, chunk2, chunk3},
	}

	// Convert to proto to measure chunk sizes
	protoPayload := payload.ToProto()
	chunk1Size := protoPayload.Chunks[0].SizeVT()
	chunk2Size := protoPayload.Chunks[1].SizeVT()
	chunk3Size := protoPayload.Chunks[2].SizeVT()

	// Set threshold so chunk1 + chunk2 fit, but adding chunk3 triggers a flush
	// The threshold needs to be > chunk1+chunk2 but < chunk1+chunk2+chunk3
	threshold := chunk1Size + chunk2Size + chunk3Size - 1
	defer useFlushThreshold(threshold)()

	ss := &SampledChunksV1{
		TracerPayload: payload,
		SpanCount:     3,
		EventCount:    0,
	}

	tw := NewTraceWriterV1(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, compressor)
	tw.WriteChunksV1(ss)

	// After WriteChunksV1, the first two chunks should have been flushed
	// because adding chunk3 caused the threshold to be exceeded
	assert.Equal(t, 1, srv.Accepted(), "Expected one payload to be flushed mid-write (containing first 2 chunks)")

	tw.Stop()

	// After Stop, the remaining chunk should be flushed
	assert.Equal(t, 2, srv.Accepted(), "Expected two payloads total: one mid-write flush, one on stop")

	// Verify the payloads contain the expected chunks
	payloads := srv.Payloads()
	require.Len(t, payloads, 2)

	// First payload should have 2 chunks
	firstPayload, err := deserializePayload(*payloads[0], compressor)
	require.NoError(t, err)
	totalChunksInFirst := 0
	for _, tp := range firstPayload.IdxTracerPayloads {
		totalChunksInFirst += len(tp.Chunks)
	}
	assert.Equal(t, 2, totalChunksInFirst, "First payload should contain 2 chunks")

	// Second payload should have 1 chunk
	secondPayload, err := deserializePayload(*payloads[1], compressor)
	require.NoError(t, err)
	totalChunksInSecond := 0
	for _, tp := range secondPayload.IdxTracerPayloads {
		totalChunksInSecond += len(tp.Chunks)
	}
	assert.Equal(t, 1, totalChunksInSecond, "Second payload should contain 1 chunk")
}

func TestTraceWriterV1RemovedChunkUnreferencedStringsRemoved(t *testing.T) {
	compressor := zstd.NewComponent()
	srv := newTestServer()
	defer srv.Close()
	cfg := &config.AgentConfig{
		Hostname:   testHostname,
		DefaultEnv: testEnv,
		Endpoints: []*config.Endpoint{{
			APIKey: "123",
			Host:   srv.URL,
		}},
		TraceWriter: &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40, FlushPeriodSeconds: 1_000},
	}
	ss := randomSampledSpansV1(20, 8)
	// Attach an unreferenced string, this is possible because we don't track when a trace chunk is unsent from a tracer payload
	ss.TracerPayload.Strings.Add("SECRET_STRING")
	tw := NewTraceWriterV1(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, compressor)
	tw.WriteChunksV1(ss)
	tw.Stop()
	assert.Equal(t, 1, srv.Accepted())
	mapPayloads(t, srv.Payloads(), compressor, func(all *pb.AgentPayload) {
		for _, tp := range all.IdxTracerPayloads {
			assert.NotContains(t, tp.Strings, "SECRET_STRING")
		}
	})
}

// randomSampledSpans returns a set of spans sampled spans and events events.
func randomSampledSpansV1(spans, events int) *SampledChunksV1 {
	realisticIDs := true
	trace := testutil.GetTestTracesV1(1, spans, realisticIDs)
	return &SampledChunksV1{
		TracerPayload: trace,
		SpanCount:     int64(spans),
		EventCount:    int64(events),
	}
}

func mapPayloads(t *testing.T, payloads []*payload, compressor compression.Component, f func(*pb.AgentPayload)) {
	all := &pb.AgentPayload{}
	for _, p := range payloads {
		var slurp []byte
		assert := assert.New(t)
		reader, err := compressor.NewReader(p.body)
		assert.NoError(err)
		defer reader.Close()

		slurp, err = io.ReadAll(reader)

		assert.NoError(err)
		var payload pb.AgentPayload
		err = proto.Unmarshal(slurp, &payload)
		assert.NoError(err)
		assert.Equal(payload.HostName, testHostname)
		assert.Equal(payload.Env, testEnv)
		all.IdxTracerPayloads = append(all.IdxTracerPayloads, payload.IdxTracerPayloads...)
	}
	f(all)
}

// payloadsContain checks that the given payloads contain the given set of sampled spans.
func payloadsContainV1(t *testing.T, payloads []*payload, sampledSpans []*SampledChunksV1, compressor compression.Component) {
	t.Helper()
	mapPayloads(t, payloads, compressor, func(all *pb.AgentPayload) {
		for _, ss := range sampledSpans {
			var found bool
			for _, tracerPayload := range all.IdxTracerPayloads {
				for _, trace := range tracerPayload.Chunks {
					if bytes.Equal(ss.TracerPayload.Chunks[0].TraceID, trace.TraceID) {
						found = true
						break
					}
				}
			}

			if !found {
				t.Fatal("payloads didn't contain given traces")
			}
		}
	})
}

func TestTraceWriterV1FlushSync(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	cfg := &config.AgentConfig{
		Hostname:   testHostname,
		DefaultEnv: testEnv,
		Endpoints: []*config.Endpoint{{
			APIKey: "123",
			Host:   srv.URL,
		}},
		TraceWriter:         &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40, FlushPeriodSeconds: 1_000},
		SynchronousFlushing: true,
	}
	t.Run("ok", func(t *testing.T) {
		testSpans := []*SampledChunksV1{
			randomSampledSpansV1(20, 8),
			randomSampledSpansV1(10, 0),
			randomSampledSpansV1(40, 5),
		}
		tw := NewTraceWriterV1(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, gzip.NewComponent())
		for _, ss := range testSpans {
			tw.WriteChunksV1(ss)
		}

		// No payloads should be sent before flushing
		assert.Equal(t, 0, srv.Accepted())
		tw.FlushSync()
		// Now all trace payloads should be sent
		assert.Equal(t, 1, srv.Accepted())
		payloadsContainV1(t, srv.Payloads(), testSpans, tw.compressor)
	})
}

func TestTraceWriterV1ResetBuffer(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	cfg := &config.AgentConfig{
		Hostname:   testHostname,
		DefaultEnv: testEnv,
		Endpoints: []*config.Endpoint{{
			APIKey: "123",
			Host:   srv.URL,
		}},
		TraceWriter:         &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40, FlushPeriodSeconds: 1_000},
		SynchronousFlushing: true,
	}

	w := NewTraceWriterV1(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, gzip.NewComponent())

	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	assert.Less(t, m.HeapInuse, uint64(50*1e6))

	// Create a large payload with a big string in the string table
	bigPayload := &idx.TracerPayload{
		Strings: []string{string(make([]byte, 50*1e6))},
	}

	w.mu.Lock()
	w.tracerPayloadsV1 = append(w.tracerPayloadsV1, bigPayload)
	w.mu.Unlock()

	runtime.GC()
	runtime.ReadMemStats(&m)
	assert.Greater(t, m.HeapInuse, uint64(50*1e6))

	w.mu.Lock()
	w.resetBufferV1()
	w.mu.Unlock()

	runtime.GC()
	runtime.ReadMemStats(&m)
	assert.Less(t, m.HeapInuse, uint64(50*1e6))
}

func TestTraceWriterV1SyncStop(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	cfg := &config.AgentConfig{
		Hostname:   testHostname,
		DefaultEnv: testEnv,
		Endpoints: []*config.Endpoint{{
			APIKey: "123",
			Host:   srv.URL,
		}},
		TraceWriter:         &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40, FlushPeriodSeconds: 1_000},
		SynchronousFlushing: true,
	}
	t.Run("ok", func(t *testing.T) {
		testSpans := []*SampledChunksV1{
			randomSampledSpansV1(20, 8),
			randomSampledSpansV1(10, 0),
			randomSampledSpansV1(40, 5),
		}
		tw := NewTraceWriterV1(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, gzip.NewComponent())
		for _, ss := range testSpans {
			tw.WriteChunksV1(ss)
		}

		// No payloads should be sent before flushing
		assert.Equal(t, 0, srv.Accepted())
		tw.Stop()
		// Now all trace payloads should be sent
		assert.Equal(t, 1, srv.Accepted())
		payloadsContainV1(t, srv.Payloads(), testSpans, tw.compressor)
	})
}

func TestTraceWriterV1AgentPayload(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	cfg := &config.AgentConfig{
		Hostname:   testHostname,
		DefaultEnv: testEnv,
		Endpoints: []*config.Endpoint{{
			APIKey: "123",
			Host:   srv.URL,
		}},
		TraceWriter:         &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40, FlushPeriodSeconds: 1_000},
		SynchronousFlushing: true,
	}

	// helper function to send a chunk to the writer and force a synchronous flush
	sendRandomSpanAndFlush := func(t *testing.T, tw *TraceWriterV1) {
		tw.WriteChunksV1(randomSampledSpansV1(20, 8))
		err := tw.FlushSync()
		assert.Nil(t, err)
	}
	// helper function to parse the received payload and inspect the TPS that were filled by the writer
	assertExpectedTps := func(t *testing.T, priorityTps float64, errorTps float64, rareEnabled bool, compressor compression.Component) {
		require.Len(t, srv.payloads, 1)
		ap, err := deserializePayload(*srv.payloads[0], compressor)
		assert.Nil(t, err)
		assert.Equal(t, priorityTps, ap.TargetTPS)
		assert.Equal(t, errorTps, ap.ErrorTPS)
		assert.Equal(t, rareEnabled, ap.RareSamplerEnabled)
		srv.payloads = nil
	}

	t.Run("static TPS config", func(t *testing.T) {
		tw := NewTraceWriterV1(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, gzip.NewComponent())
		defer tw.Stop()
		sendRandomSpanAndFlush(t, tw)
		assertExpectedTps(t, 5, 5, true, tw.compressor)
	})

	t.Run("dynamic TPS config", func(t *testing.T) {
		prioritySampler := &MockSampler{TargetTPS: 5}
		errorSampler := &MockSampler{TargetTPS: 6}
		rareSampler := &MockSampler{Enabled: false}

		tw := NewTraceWriterV1(cfg, prioritySampler, errorSampler, rareSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, gzip.NewComponent())
		defer tw.Stop()
		sendRandomSpanAndFlush(t, tw)
		assertExpectedTps(t, 5, 6, false, tw.compressor)

		// simulate a remote config update
		prioritySampler.TargetTPS = 42
		errorSampler.TargetTPS = 15
		rareSampler.Enabled = true

		sendRandomSpanAndFlush(t, tw)
		assertExpectedTps(t, 42, 15, true, tw.compressor)
	})
}

// deserializePayload decompresses a payload and deserializes it into a pb.AgentPayload.
func deserializePayload(p payload, compressor compression.Component) (*pb.AgentPayload, error) {
	reader, err := compressor.NewReader(p.body)
	if err != nil {
		return nil, err
	}

	defer reader.Close()
	uncompressedBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	var agentPayload pb.AgentPayload
	err = proto.Unmarshal(uncompressedBytes, &agentPayload)
	if err != nil {
		return nil, err
	}
	return &agentPayload, nil
}

func TestTraceWriterV1UpdateAPIKey(t *testing.T) {
	assert := assert.New(t)
	srv := newTestServer()
	defer srv.Close()
	cfg := &config.AgentConfig{
		Hostname:   testHostname,
		DefaultEnv: testEnv,
		Endpoints: []*config.Endpoint{{
			APIKey: "123",
			Host:   srv.URL,
		}},
		TraceWriter: &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40, FlushPeriodSeconds: 1_000},
	}

	tw := NewTraceWriterV1(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, zstd.NewComponent())
	defer tw.Stop()

	url, err := url.Parse(srv.URL + pathTraces)
	assert.NoError(err)

	assert.Len(tw.senders, 1)
	assert.Equal("123", tw.senders[0].cfg.apiKey)
	assert.Equal(url, tw.senders[0].cfg.url)

	tw.UpdateAPIKey("invalid", "foo")
	assert.Equal("123", tw.senders[0].cfg.apiKey)
	assert.Equal(url, tw.senders[0].cfg.url)

	tw.UpdateAPIKey("123", "foo")
	assert.Equal("foo", tw.senders[0].cfg.apiKey)
	assert.Equal(url, tw.senders[0].cfg.url)
}

func TestTraceWriterAPMMode(t *testing.T) {
	testCases := []struct {
		name        string
		configValue string
		expected    string
	}{
		{
			name:        "default-empty",
			configValue: "",
			expected:    "",
		},
		{
			name:        "edge",
			configValue: "edge",
			expected:    "edge",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServer()
			defer srv.Close()
			cfg := &config.AgentConfig{
				Hostname:   testHostname,
				DefaultEnv: testEnv,
				Endpoints: []*config.Endpoint{{
					APIKey: "123",
					Host:   srv.URL,
				}},
				TraceWriter:         &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40},
				SynchronousFlushing: true,
				APMMode:             tc.configValue,
			}
			tw := NewTraceWriterV1(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, gzip.NewComponent())
			defer tw.Stop()

			// Send a span and force flush
			tw.WriteChunksV1(randomSampledSpansV1(20, 8))
			err := tw.FlushSync()
			assert.Nil(t, err)

			// Verify the AgentPayload has the correct APMMode
			require.Len(t, srv.payloads, 1)
			ap, err := deserializePayload(*srv.payloads[0], tw.compressor)
			assert.Nil(t, err)
			v, ok := ap.Tags[tagAPMMode]
			// If APMMode is not set, the tag should not be present
			if tc.expected == "" {
				assert.False(t, ok)
				assert.Empty(t, v)
			} else {
				// If APMMode is set, the tag should be present and equal to the expected value
				assert.True(t, ok)
				assert.Equal(t, tc.expected, v)
			}
		})
	}
}
