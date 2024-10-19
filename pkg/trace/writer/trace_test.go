// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package writer

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"sync"
	"testing"

	compression "github.com/DataDog/datadog-agent/comp/trace/compression/def"
	gzip "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"
	zstd "github.com/DataDog/datadog-agent/comp/trace/compression/impl-zstd"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"

	"github.com/DataDog/datadog-go/v5/statsd"
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

func TestTraceWriter(t *testing.T) {
	testCases := []struct {
		compressor compression.Component
	}{
		{gzip.NewComponent()},
		{zstd.NewComponent()},
	}

	for _, tc := range testCases {
		srv := newTestServer()
		cfg := &config.AgentConfig{
			Hostname:   testHostname,
			DefaultEnv: testEnv,
			Endpoints: []*config.Endpoint{{
				APIKey: "123",
				Host:   srv.URL,
			}},
			TraceWriter: &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40},
		}

		t.Run(fmt.Sprintf("encoding:%s", tc.compressor.Encoding()), func(t *testing.T) {
			testSpans := []*SampledChunks{
				randomSampledSpans(20, 8),
				randomSampledSpans(10, 0),
				randomSampledSpans(40, 5),
			}
			// Use a flush threshold that allows the first two entries to not overflow,
			// but overflow on the third.
			defer useFlushThreshold(testSpans[0].Size + testSpans[1].Size + 10)()
			tw := NewTraceWriter(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, tc.compressor)
			for _, ss := range testSpans {
				tw.WriteChunks(ss)
			}
			tw.Stop()
			// One payload flushes due to overflowing the threshold, and the second one
			// because of stop.
			assert.Equal(t, 2, srv.Accepted())
			payloadsContain(t, srv.Payloads(), testSpans, tc.compressor)
		})
	}
}

func TestTraceWriterMultipleEndpointsConcurrent(t *testing.T) {
	var (
		srv = newTestServer()
		cfg = &config.AgentConfig{
			Hostname:   testHostname,
			DefaultEnv: testEnv,
			Endpoints: []*config.Endpoint{
				{
					APIKey: "123",
					Host:   srv.URL,
				},
				{
					APIKey: "123",
					Host:   srv.URL,
				},
			},
			TraceWriter: &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40},
		}
		numWorkers      = 10
		numOpsPerWorker = 100
	)

	testSpans := []*SampledChunks{
		randomSampledSpans(20, 8),
		randomSampledSpans(10, 0),
		randomSampledSpans(40, 5),
	}
	tw := NewTraceWriter(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, gzip.NewComponent())

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOpsPerWorker; j++ {
				for _, ss := range testSpans {
					tw.WriteChunks(ss)
				}
			}
		}()
	}

	wg.Wait()
	tw.Stop()
	payloadsContain(t, srv.Payloads(), testSpans, tw.compressor)
}

// useFlushThreshold sets n as the number of bytes to be used as the flush threshold
// and returns a function to restore it.
func useFlushThreshold(n int) func() {
	old := MaxPayloadSize
	MaxPayloadSize = n
	return func() { MaxPayloadSize = old }
}

// randomSampledSpans returns a set of spans sampled spans and events events.
func randomSampledSpans(spans, events int) *SampledChunks {
	realisticIDs := true
	traceChunk := testutil.GetTestTraceChunks(1, spans, realisticIDs)[0]
	return &SampledChunks{
		TracerPayload: &pb.TracerPayload{Chunks: []*pb.TraceChunk{traceChunk}},
		Size:          pb.Trace(traceChunk.Spans).Msgsize() + pb.Trace(traceChunk.Spans[:events]).Msgsize(),
		SpanCount:     int64(len(traceChunk.Spans)),
	}
}

// payloadsContain checks that the given payloads contain the given set of sampled spans.
func payloadsContain(t *testing.T, payloads []*payload, sampledSpans []*SampledChunks, compressor compression.Component) {
	t.Helper()
	var all pb.AgentPayload
	for _, p := range payloads {
		assert := assert.New(t)
		var slurp []byte

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
		all.TracerPayloads = append(all.TracerPayloads, payload.TracerPayloads...)
	}
	for _, ss := range sampledSpans {
		var found bool
		for _, tracerPayload := range all.TracerPayloads {
			for _, trace := range tracerPayload.Chunks {
				if proto.Equal(trace, ss.TracerPayload.Chunks[0]) {
					found = true
					break
				}
			}
		}

		if !found {
			t.Fatal("payloads didn't contain given traces")
		}
	}
}

func TestTraceWriterFlushSync(t *testing.T) {
	srv := newTestServer()
	cfg := &config.AgentConfig{
		Hostname:   testHostname,
		DefaultEnv: testEnv,
		Endpoints: []*config.Endpoint{{
			APIKey: "123",
			Host:   srv.URL,
		}},
		TraceWriter:         &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40},
		SynchronousFlushing: true,
	}
	t.Run("ok", func(t *testing.T) {
		testSpans := []*SampledChunks{
			randomSampledSpans(20, 8),
			randomSampledSpans(10, 0),
			randomSampledSpans(40, 5),
		}
		tw := NewTraceWriter(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, gzip.NewComponent())
		for _, ss := range testSpans {
			tw.WriteChunks(ss)
		}

		// No payloads should be sent before flushing
		assert.Equal(t, 0, srv.Accepted())
		tw.FlushSync()
		// Now all trace payloads should be sent
		assert.Equal(t, 1, srv.Accepted())
		payloadsContain(t, srv.Payloads(), testSpans, tw.compressor)
	})
}

func TestResetBuffer(t *testing.T) {
	srv := newTestServer()
	cfg := &config.AgentConfig{
		Hostname:   testHostname,
		DefaultEnv: testEnv,
		Endpoints: []*config.Endpoint{{
			APIKey: "123",
			Host:   srv.URL,
		}},
		TraceWriter:         &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40},
		SynchronousFlushing: true,
	}

	w := NewTraceWriter(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, gzip.NewComponent())

	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	assert.Less(t, m.HeapInuse, uint64(50*1e6))

	bigPayload := &pb.TracerPayload{
		ContainerID: string(make([]byte, 50*1e6)),
	}

	w.tracerPayloads = append(w.tracerPayloads, bigPayload)

	runtime.GC()
	runtime.ReadMemStats(&m)
	assert.Greater(t, m.HeapInuse, uint64(50*1e6))

	w.mu.Lock()
	w.resetBuffer()
	w.mu.Unlock()

	runtime.GC()
	runtime.ReadMemStats(&m)
	assert.Less(t, m.HeapInuse, uint64(50*1e6))
}

func TestTraceWriterSyncStop(t *testing.T) {
	srv := newTestServer()
	cfg := &config.AgentConfig{
		Hostname:   testHostname,
		DefaultEnv: testEnv,
		Endpoints: []*config.Endpoint{{
			APIKey: "123",
			Host:   srv.URL,
		}},
		TraceWriter:         &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40},
		SynchronousFlushing: true,
	}
	t.Run("ok", func(t *testing.T) {
		testSpans := []*SampledChunks{
			randomSampledSpans(20, 8),
			randomSampledSpans(10, 0),
			randomSampledSpans(40, 5),
		}
		tw := NewTraceWriter(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, gzip.NewComponent())
		for _, ss := range testSpans {
			tw.WriteChunks(ss)
		}

		// No payloads should be sent before flushing
		assert.Equal(t, 0, srv.Accepted())
		tw.Stop()
		// Now all trace payloads should be sent
		assert.Equal(t, 1, srv.Accepted())
		payloadsContain(t, srv.Payloads(), testSpans, tw.compressor)
	})
}

func TestTraceWriterSyncNoop(t *testing.T) {
	srv := newTestServer()
	cfg := &config.AgentConfig{
		Hostname:   testHostname,
		DefaultEnv: testEnv,
		Endpoints: []*config.Endpoint{{
			APIKey: "123",
			Host:   srv.URL,
		}},
		TraceWriter:         &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40},
		SynchronousFlushing: false,
	}
	t.Run("ok", func(t *testing.T) {
		tw := NewTraceWriter(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, gzip.NewComponent())
		err := tw.FlushSync()
		assert.NotNil(t, err)
	})
}

func TestTraceWriterAgentPayload(t *testing.T) {
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
	}

	// helper function to send a chunk to the writer and force a synchronous flush
	sendRandomSpanAndFlush := func(t *testing.T, tw *TraceWriter) {
		tw.WriteChunks(randomSampledSpans(20, 8))
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
		tw := NewTraceWriter(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, gzip.NewComponent())
		defer tw.Stop()
		sendRandomSpanAndFlush(t, tw)
		assertExpectedTps(t, 5, 5, true, tw.compressor)
	})

	t.Run("dynamic TPS config", func(t *testing.T) {
		prioritySampler := &MockSampler{TargetTPS: 5}
		errorSampler := &MockSampler{TargetTPS: 6}
		rareSampler := &MockSampler{Enabled: false}

		tw := NewTraceWriter(cfg, prioritySampler, errorSampler, rareSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, gzip.NewComponent())
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

func TestTraceWriterUpdateAPIKey(t *testing.T) {
	assert := assert.New(t)
	srv := newTestServer()
	cfg := &config.AgentConfig{
		Hostname:   testHostname,
		DefaultEnv: testEnv,
		Endpoints: []*config.Endpoint{{
			APIKey: "123",
			Host:   srv.URL,
		}},
		TraceWriter: &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40},
	}

	tw := NewTraceWriter(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, zstd.NewComponent())
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

// BenchmarkMapDelete-8   	35125977	        28.85 ns/op	       0 B/op	       0 allocs/op
// BenchmarkMapDelete-8   	100000000	        14.36 ns/op	       0 B/op	       0 allocs/op
func BenchmarkMapDelete(b *testing.B) {
	m := map[string]float64{
		"hello.world.1": 1,
		"hello.world.2": 1,
		"hello.world.3": 1,
		"hello.world.4": 1,
		"hello.world.5": 1,
		"hello.world.6": 1,
		"hello.world.7": 1,
		"hello.world.8": 1,
	}
	for n := 0; n < b.N; n++ {
		m["_sampling_priority_v1"] = 1
		//delete(m, "_sampling_priority_v1")
	}
}

// BenchmarkSpanProto-8   	 2124880	       567.1 ns/op	     256 B/op	       1 allocs/op
// BenchmarkSpanProto-8   	 2222722	       528.4 ns/op	     208 B/op	       1 allocs/op
func BenchmarkSpanProto(b *testing.B) {
	s := pb.Span{
		Metrics: map[string]float64{
			"hello.world.1": 1,
			"hello.world.2": 1,
			"hello.world.3": 1,
			"hello.world.4": 1,
			"hello.world.5": 1,
			"hello.world.6": 1,
			"hello.world.7": 1,
			"hello.world.8": 1,
			//"_sampling_priority_v1": 1,
		},
	}
	for n := 0; n < b.N; n++ {
		//proto.Marshal(&s)
		s.MarshalVT()
	}
}

func BenchmarkSerialize(b *testing.B) {
	for _, tt := range []struct {
		name        string
		traceChunks []*pb.TraceChunk
	}{
		{
			name:        "large",
			traceChunks: testutil.GetTestTraceChunks(10, 100, true),
		},
		{
			name:        "small",
			traceChunks: testutil.GetTestTraceChunks(2, 2, true),
		},
	} {
		b.Run(tt.name, func(b *testing.B) {
			ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				io.Copy(io.Discard, r.Body)
				r.Body.Close()
			}))
			defer ts.Close()
			cfg := &config.AgentConfig{
				Hostname:   testHostname,
				DefaultEnv: testEnv,
				Endpoints: []*config.Endpoint{{
					APIKey: "123",
					Host:   ts.URL,
				}},
				TraceWriter: &config.WriterConfig{},
			}
			tw := NewTraceWriter(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, gzip.NewComponent())
			defer tw.Stop()

			// Avoid the overhead of the senders so we're just measuring serialization
			stopSenders(tw.senders)
			tw.senders = nil

			payloads := []*pb.TracerPayload{
				{Chunks: tt.traceChunks},
			}
			p := pb.AgentPayload{
				AgentVersion:       tw.agentVersion,
				HostName:           tw.hostname,
				Env:                tw.env,
				TargetTPS:          tw.prioritySampler.GetTargetTPS(),
				ErrorTPS:           tw.errorsSampler.GetTargetTPS(),
				RareSamplerEnabled: tw.rareSampler.IsEnabled(),
				TracerPayloads:     payloads,
			}

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				tw.serialize(&p)
			}
		})
	}
}
