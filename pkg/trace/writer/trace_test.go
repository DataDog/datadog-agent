// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package writer

import (
	"compress/gzip"
	"io/ioutil"
	"reflect"
	"runtime"
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
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
		TraceWriter: &config.WriterConfig{ConnectionLimit: 200, QueueSize: 40},
	}

	t.Run("ok", func(t *testing.T) {
		testSpans := []*SampledChunks{
			randomSampledSpans(20, 8),
			randomSampledSpans(10, 0),
			randomSampledSpans(40, 5),
		}
		// Use a flush threshold that allows the first two entries to not overflow,
		// but overflow on the third.
		defer useFlushThreshold(testSpans[0].Size + testSpans[1].Size + 10)()
		tw := NewTraceWriter(cfg)
		tw.In = make(chan *SampledChunks)
		go tw.Run()
		for _, ss := range testSpans {
			tw.In <- ss
		}
		tw.Stop()
		// One payload flushes due to overflowing the threshold, and the second one
		// because of stop.
		assert.Equal(t, 2, srv.Accepted())
		payloadsContain(t, srv.Payloads(), testSpans)
	})
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
	tw := NewTraceWriter(cfg)
	tw.In = make(chan *SampledChunks, 100)
	go tw.Run()

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOpsPerWorker; j++ {
				for _, ss := range testSpans {
					tw.In <- ss
				}
			}
		}()
	}

	wg.Wait()
	tw.Stop()
	payloadsContain(t, srv.Payloads(), testSpans)
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
func payloadsContain(t *testing.T, payloads []*payload, sampledSpans []*SampledChunks) {
	t.Helper()
	var all pb.AgentPayload
	for _, p := range payloads {
		assert := assert.New(t)
		gzipr, err := gzip.NewReader(p.body)
		assert.NoError(err)
		slurp, err := ioutil.ReadAll(gzipr)
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
				if reflect.DeepEqual(trace, ss.TracerPayload.Chunks[0]) {
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
		tw := NewTraceWriter(cfg)
		go tw.Run()
		for _, ss := range testSpans {
			tw.In <- ss
		}

		// No payloads should be sent before flushing
		assert.Equal(t, 0, srv.Accepted())
		tw.FlushSync()
		// Now all trace payloads should be sent
		assert.Equal(t, 1, srv.Accepted())
		payloadsContain(t, srv.Payloads(), testSpans)
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

	w := NewTraceWriter(cfg)

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

	w.resetBuffer()
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
		tw := NewTraceWriter(cfg)
		go tw.Run()
		for _, ss := range testSpans {
			tw.In <- ss
		}

		// No payloads should be sent before flushing
		assert.Equal(t, 0, srv.Accepted())
		tw.Stop()
		// Now all trace payloads should be sent
		assert.Equal(t, 1, srv.Accepted())
		payloadsContain(t, srv.Payloads(), testSpans)
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
		tw := NewTraceWriter(cfg)
		err := tw.FlushSync()
		assert.NotNil(t, err)
	})
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
		s.Marshal()
	}
}
