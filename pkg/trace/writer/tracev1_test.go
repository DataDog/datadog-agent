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

	// Create a tracer payload with 4 chunks (matching the 4-way split logic)
	strings := idx.NewStringTable()
	chunk1 := &idx.InternalTraceChunk{
		Strings:  strings,
		Priority: 1,
		TraceID:  []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Spans:    []*idx.InternalSpan{testutil.GetTestSpanV1(strings)},
	}
	chunk1.SetStringAttribute("chunk1", "value1")
	chunk2 := &idx.InternalTraceChunk{
		Strings:  strings,
		Priority: 1,
		TraceID:  []byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17},
		Spans:    []*idx.InternalSpan{testutil.GetTestSpanV1(strings)},
	}
	chunk2.SetStringAttribute("chunk2", "value2")
	chunk3 := &idx.InternalTraceChunk{
		Strings:  strings,
		Priority: 1,
		TraceID:  []byte{3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18},
		Spans:    []*idx.InternalSpan{testutil.GetTestSpanV1(strings)},
	}
	chunk3.SetStringAttribute("chunk3", "value3")
	chunk4 := &idx.InternalTraceChunk{
		Strings:  strings,
		Priority: 1,
		TraceID:  []byte{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19},
		Spans:    []*idx.InternalSpan{testutil.GetTestSpanV1(strings)},
	}
	chunk4.SetStringAttribute("chunk4", "value4")
	payload := &idx.InternalTracerPayload{
		Strings: strings,
		Chunks:  []*idx.InternalTraceChunk{chunk1, chunk2, chunk3, chunk4},
	}

	// Convert to proto to measure total payload size
	protoPayload := payload.ToProto()
	totalSize := proto.Size(protoPayload)

	// Set threshold to 1 so any payload triggers a split
	// This ensures our 4-chunk payload gets split into 4 separate payloads
	defer useFlushThreshold(1)()

	ss := &SampledChunksV1{
		TracerPayload: payload,
		SpanCount:     4,
		EventCount:    0,
	}

	tw := NewTraceWriterV1(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, compressor)
	tw.WriteChunksV1(ss)

	// After WriteChunksV1, the payload should be split into 4 separate payloads
	// Each payload contains 1 chunk from the original 4-chunk payload
	assert.Equal(t, 4, srv.Accepted(), "Expected 4 payloads (one per chunk) due to 4-way split, got payloads with total size %d", totalSize)

	tw.Stop()

	// Verify we still have 4 payloads after stop (nothing buffered)
	assert.Equal(t, 4, srv.Accepted(), "Expected 4 payloads total after stop")

	// Verify each payload contains exactly 1 chunk
	payloads := srv.Payloads()
	require.Len(t, payloads, 4)

	for i, p := range payloads {
		ap, err := deserializePayload(*p, compressor)
		require.NoError(t, err)
		totalChunks := 0
		for _, tp := range ap.IdxTracerPayloads {
			totalChunks += len(tp.Chunks)
			if i == 0 {
				// Verify we've removed unused strings from split payloads
				assert.NotContains(t, tp.Strings, "chunk2")
				assert.NotContains(t, tp.Strings, "chunk3")
				assert.NotContains(t, tp.Strings, "chunk4")
			} else if i == 1 {
				assert.NotContains(t, tp.Strings, "chunk1")
				assert.NotContains(t, tp.Strings, "chunk3")
				assert.NotContains(t, tp.Strings, "chunk4")
			}
		}
		assert.Equal(t, 1, totalChunks, "Payload %d should contain exactly 1 chunk", i)
	}
}

// TestTraceWriterV1PayloadSplittingFewerThan4Chunks verifies that when a payload
// has fewer than 4 chunks but is too large, each chunk is sent separately.
func TestTraceWriterV1PayloadSplittingFewerThan4Chunks(t *testing.T) {
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

	// Create a tracer payload with only 2 chunks (fewer than 4)
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
	payload := &idx.InternalTracerPayload{
		Strings: strings,
		Chunks:  []*idx.InternalTraceChunk{chunk1, chunk2},
	}

	// Set threshold to 1 so the payload triggers a split
	defer useFlushThreshold(1)()

	ss := &SampledChunksV1{
		TracerPayload: payload,
		SpanCount:     2,
		EventCount:    0,
	}

	tw := NewTraceWriterV1(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, compressor)
	tw.WriteChunksV1(ss)

	// With fewer than 4 chunks, each chunk should be sent separately
	// So we expect 2 payloads (one per chunk)
	assert.Equal(t, 2, srv.Accepted(), "Expected 2 payloads (one per chunk) when splitting payload with fewer than 4 chunks")

	tw.Stop()

	// Verify we have 2 payloads after stop
	assert.Equal(t, 2, srv.Accepted(), "Expected 2 payloads total after stop")

	// Verify each payload contains exactly 1 chunk
	payloads := srv.Payloads()
	require.Len(t, payloads, 2)

	for i, p := range payloads {
		ap, err := deserializePayload(*p, compressor)
		require.NoError(t, err)
		totalChunks := 0
		for _, tp := range ap.IdxTracerPayloads {
			totalChunks += len(tp.Chunks)
		}
		assert.Equal(t, 1, totalChunks, "Payload %d should contain exactly 1 chunk", i)
	}
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
	preparedPayload := pb.PrepareTracerPayload(bigPayload)

	w.mu.Lock()
	w.preparedPayloadsV1 = append(w.preparedPayloadsV1, preparedPayload)
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
	assert.Equal("123", tw.senders[0].apiKeyManager.Get())
	assert.Equal(url, tw.senders[0].cfg.url)

	tw.UpdateAPIKey("invalid", "foo")
	assert.Equal("123", tw.senders[0].apiKeyManager.Get())
	assert.Equal(url, tw.senders[0].cfg.url)

	tw.UpdateAPIKey("123", "foo")
	assert.Equal("foo", tw.senders[0].apiKeyManager.Get())
	assert.Equal(url, tw.senders[0].cfg.url)
}
