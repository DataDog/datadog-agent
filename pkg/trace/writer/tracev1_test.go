// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package writer

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"runtime"
	"strings"
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

func TestTraceWriterV1IgnoresEmptyPayload(t *testing.T) {
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

	tw.WriteChunksV1(&SampledChunksV1{
		TracerPayload: &idx.InternalTracerPayload{Strings: idx.NewStringTable()},
	})
	tw.Stop()

	assert.Zero(t, srv.Accepted())
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

	// Create a tracer payload with 4 chunks
	st := idx.NewStringTable()
	chunk1 := &idx.InternalTraceChunk{
		Strings:  st,
		Priority: 1,
		TraceID:  []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Spans:    []*idx.InternalSpan{testutil.GetTestSpanV1(st)},
	}
	chunk1.SetStringAttribute("chunk1", "value1")
	chunk2 := &idx.InternalTraceChunk{
		Strings:  st,
		Priority: 1,
		TraceID:  []byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17},
		Spans:    []*idx.InternalSpan{testutil.GetTestSpanV1(st)},
	}
	chunk2.SetStringAttribute("chunk2", "value2")
	chunk3 := &idx.InternalTraceChunk{
		Strings:  st,
		Priority: 1,
		TraceID:  []byte{3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18},
		Spans:    []*idx.InternalSpan{testutil.GetTestSpanV1(st)},
	}
	chunk3.SetStringAttribute("chunk3", "value3")
	chunk4 := &idx.InternalTraceChunk{
		Strings:  st,
		Priority: 1,
		TraceID:  []byte{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19},
		Spans:    []*idx.InternalSpan{testutil.GetTestSpanV1(st)},
	}
	chunk4.SetStringAttribute("chunk4", "value4")
	payload := &idx.InternalTracerPayload{
		Strings: st,
		Chunks:  []*idx.InternalTraceChunk{chunk1, chunk2, chunk3, chunk4},
	}

	// Convert to proto to measure total payload size
	protoPayload := payload.ToProto()
	totalSize := proto.Size(protoPayload)

	// Set threshold to 1 so any payload triggers a split
	// No two chunks fit together, so each chunk is sent in its own payload
	defer useFlushThreshold(1)()

	ss := &SampledChunksV1{
		TracerPayload: payload,
		SpanCount:     4,
		EventCount:    0,
	}

	tw := NewTraceWriterV1(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, compressor)
	tw.WriteChunksV1(ss)

	// After WriteChunksV1, with a threshold of 1, the payload should be split into 4 separate payloads
	// Each payload contains 1 chunk from the original 4-chunk payload
	assert.Equal(t, 4, srv.Accepted(), "Expected 4 payloads (one per chunk) with a threshold of 1, got payloads with total size %d", totalSize)

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

// TestTraceWriterV1PayloadSplittingTwoChunks verifies that when a payload with two chunks
// is too large and no two chunks fit together, each chunk is sent separately.
func TestTraceWriterV1PayloadSplittingTwoChunks(t *testing.T) {
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

	// Create a tracer payload with only 2 chunks
	st := idx.NewStringTable()
	chunk1 := &idx.InternalTraceChunk{
		Strings:  st,
		Priority: 1,
		TraceID:  []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Spans:    []*idx.InternalSpan{testutil.GetTestSpanV1(st)},
	}
	chunk2 := &idx.InternalTraceChunk{
		Strings:  st,
		Priority: 1,
		TraceID:  []byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17},
		Spans:    []*idx.InternalSpan{testutil.GetTestSpanV1(st)},
	}
	payload := &idx.InternalTracerPayload{
		Strings: st,
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

	// With a threshold of 1, each chunk should be sent separately
	// So we expect 2 payloads (one per chunk)
	assert.Equal(t, 2, srv.Accepted(), "Expected 2 payloads (one per chunk) with a threshold of 1")

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

// sizedPayloadV1 returns a tracer payload with one single-span chunk per entry in sizes, each carrying
// about sizes[i] bytes of unique data. Chunks are identified by their trace ID: {id, i}.
func sizedPayloadV1(id byte, sizes ...int) *SampledChunksV1 {
	st := idx.NewStringTable()
	tp := &idx.InternalTracerPayload{Strings: st}
	for i, s := range sizes {
		span := testutil.GetTestSpanV1(st)
		span.SetStringAttribute("data", fmt.Sprintf("%d-%d-", id, i)+strings.Repeat("x", s))
		tp.Chunks = append(tp.Chunks, &idx.InternalTraceChunk{
			Strings:  st,
			Priority: 1,
			TraceID:  []byte{id, byte(i), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			Spans:    []*idx.InternalSpan{span},
		})
	}
	return &SampledChunksV1{TracerPayload: tp, SpanCount: int64(len(sizes))}
}

// sentRequestV1 describes one request received by the test server.
type sentRequestV1 struct {
	chunks      []string // "<payload id>-<chunk index>"
	tracerBytes int      // serialized size of the tracer payloads it carries
}

func sentRequestsV1(t *testing.T, srv *testServer, compressor compression.Component) []sentRequestV1 {
	t.Helper()
	var out []sentRequestV1
	for _, p := range srv.Payloads() {
		ap, err := deserializePayload(*p, compressor)
		require.NoError(t, err)
		var r sentRequestV1
		for _, tp := range ap.IdxTracerPayloads {
			r.tracerBytes += proto.Size(tp)
			for _, c := range tp.Chunks {
				r.chunks = append(r.chunks, fmt.Sprintf("%d-%d", c.TraceID[0], c.TraceID[1]))
			}
		}
		out = append(out, r)
	}
	return out
}

// TestTraceWriterV1PayloadSplittingBySize verifies that the writer never sends the held buffer together
// with a payload that does not fit next to it, and that oversized payloads are split by bytes: every
// request either fits in MaxPayloadSize or carries a single chunk.
func TestTraceWriterV1PayloadSplittingBySize(t *testing.T) {
	const limit = 10_000
	defer useFlushThreshold(limit)()

	for _, tc := range []struct {
		name     string
		buffered [][]int // payloads written first, each fitting in the buffer
		incoming []int   // chunk sizes of the payload that overflows the buffer
		want     [][]string
	}{
		{
			name:     "held buffer is not sent with a large single chunk",
			buffered: [][]int{{2_500}, {2_500}, {2_500}},
			incoming: []int{20_000},
			want:     [][]string{{"1-0", "2-0", "3-0"}, {"9-0"}},
		},
		{
			name:     "held buffer is not sent with the first group of a split payload",
			buffered: [][]int{{2_500}, {2_500}, {2_500}},
			incoming: []int{20_000, 500, 500, 500},
			want:     [][]string{{"1-0", "2-0", "3-0"}, {"9-0"}, {"9-1"}, {"9-2", "9-3"}},
		},
		{
			name:     "large chunks are not grouped by count",
			incoming: []int{100, 100, 100, 100, 100, 100, 8_000, 8_000},
			// The grouping of the small chunks follows the halving done by the writer; the
			// contract checked below is that the two large chunks are sent separately.
			want: [][]string{{"9-0", "9-1", "9-2", "9-3"}, {"9-4", "9-5"}, {"9-6"}, {"9-7"}},
		},
		{
			name:     "payload that fits once the buffer is flushed is held, not split",
			buffered: [][]int{{8_000}},
			incoming: []int{1_000, 1_000, 1_000, 1_000},
			want:     [][]string{{"1-0"}, {"9-0", "9-1", "9-2", "9-3"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			compressor := zstd.NewComponent()
			srv := newTestServer()
			defer srv.Close()
			cfg := &config.AgentConfig{
				Hostname:            testHostname,
				DefaultEnv:          testEnv,
				Endpoints:           []*config.Endpoint{{APIKey: "123", Host: srv.URL}},
				TraceWriter:         &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40, FlushPeriodSeconds: 1_000},
				SynchronousFlushing: true,
			}
			tw := NewTraceWriterV1(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, compressor)
			bufferedSize := 0
			for i, sizes := range tc.buffered {
				ss := sizedPayloadV1(byte(i+1), sizes...)
				bufferedSize += pb.PrepareTracerPayload(ss.TracerPayload.ToProto()).Size
				tw.WriteChunksV1(ss)
			}
			require.LessOrEqual(t, bufferedSize, limit, "buffered payloads must fit in the buffer")
			tw.WriteChunksV1(sizedPayloadV1(9, tc.incoming...))
			require.NoError(t, tw.FlushSync())
			tw.Stop()

			var got [][]string
			for _, r := range sentRequestsV1(t, srv, compressor) {
				if len(r.chunks) > 1 {
					assert.LessOrEqual(t, r.tracerBytes, limit, "request with chunks %v is above the limit", r.chunks)
				}
				got = append(got, r.chunks)
			}
			assert.Equal(t, tc.want, got)
		})
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
		reader, err := compressor.NewReader(p.body)
		require.NoError(t, err, "payload body is not valid %s", compressor.Encoding())

		slurp, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		require.NoError(t, readErr)
		require.NoError(t, closeErr)

		var payload pb.AgentPayload
		require.NoError(t, proto.Unmarshal(slurp, &payload))
		assert.Equal(t, testHostname, payload.HostName)
		assert.Equal(t, testEnv, payload.Env)
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

func TestTraceWriterV1OTelGateway(t *testing.T) {
	testCases := []struct {
		name        string
		otelGateway bool
	}{
		{
			name:        "gateway-disabled",
			otelGateway: false,
		},
		{
			name:        "gateway-enabled",
			otelGateway: true,
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
				OTelGateway:         tc.otelGateway,
			}
			tw := NewTraceWriterV1(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, gzip.NewComponent())
			defer tw.Stop()

			tw.WriteChunksV1(randomSampledSpansV1(20, 8))
			err := tw.FlushSync()
			assert.Nil(t, err)

			require.Len(t, srv.payloads, 1)
			ap, err := deserializePayload(*srv.payloads[0], tw.compressor)
			assert.Nil(t, err)
			v, ok := ap.Tags[tagOTelGateway]
			if tc.otelGateway {
				assert.True(t, ok)
				assert.Equal(t, "true", v)
			} else {
				assert.False(t, ok)
				assert.Empty(t, v)
			}
		})
	}
}

// TestTraceWriterV1BytesMetricsMultipleSenders verifies that bytes_uncompressed is
// counted once per sender (same scope as bytes), so the two metrics remain
// consistent regardless of the number of configured endpoints.
func TestTraceWriterV1BytesMetricsMultipleSenders(t *testing.T) {
	srv := newTestServer()
	defer srv.Close()
	cfg := &config.AgentConfig{
		Hostname:   testHostname,
		DefaultEnv: testEnv,
		Endpoints: []*config.Endpoint{
			{APIKey: "123", Host: srv.URL},
			{APIKey: "123", Host: srv.URL},
		},
		SynchronousFlushing: true,
		TraceWriter:         &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40, FlushPeriodSeconds: 100},
	}
	defer useFlushThreshold(1)()
	tw := NewTraceWriterV1(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, passthroughCompressor{})

	tw.WriteChunksV1(randomSampledSpansV1(20, 8))
	require.NoError(t, tw.FlushSync())

	bytes := tw.statsLastMinute.Bytes.Load()
	bytesUncompressed := tw.statsLastMinute.BytesUncompressed.Load()
	assert.Greater(t, bytes, int64(0))
	// With a passthrough compressor, compressed == uncompressed per payload.
	// Both metrics are counted once per sender, so they must be equal.
	assert.Equal(t, bytes, bytesUncompressed)

	tw.Stop()
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
