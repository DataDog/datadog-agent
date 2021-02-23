// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package writer

import (
	"compress/gzip"
	"io/ioutil"
	"reflect"
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
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
		testSpans := []*SampledSpans{
			randomSampledSpans(20, 8),
			randomSampledSpans(10, 0),
			randomSampledSpans(40, 5),
		}
		// Use a flush threshold that allows the first two entries to not overflow,
		// but overflow on the third.
		defer useFlushThreshold(testSpans[0].Size + testSpans[1].Size + 10)()
		tw := NewTraceWriter(cfg)
		tw.In = make(chan *SampledSpans)
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

	testSpans := []*SampledSpans{
		randomSampledSpans(20, 8),
		randomSampledSpans(10, 0),
		randomSampledSpans(40, 5),
	}
	tw := NewTraceWriter(cfg)
	tw.In = make(chan *SampledSpans, 100)
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
func randomSampledSpans(spans, events int) *SampledSpans {
	realisticIDs := true
	trace := testutil.GetTestTraces(1, spans, realisticIDs)[0]
	return &SampledSpans{
		Traces:    []*pb.APITrace{traceutil.APITrace(trace)},
		Events:    trace[:events],
		Size:      trace.Msgsize() + pb.Trace(trace[:events]).Msgsize(),
		SpanCount: int64(len(trace)),
	}
}

// payloadsContain checks that the given payloads contain the given set of sampled spans.
func payloadsContain(t *testing.T, payloads []*payload, sampledSpans []*SampledSpans) {
	t.Helper()
	var all pb.TracePayload
	for _, p := range payloads {
		assert := assert.New(t)
		gzipr, err := gzip.NewReader(p.body)
		assert.NoError(err)
		slurp, err := ioutil.ReadAll(gzipr)
		assert.NoError(err)
		var payload pb.TracePayload
		err = proto.Unmarshal(slurp, &payload)
		assert.NoError(err)
		assert.Equal(payload.HostName, testHostname)
		assert.Equal(payload.Env, testEnv)
		all.Traces = append(all.Traces, payload.Traces...)
		all.Transactions = append(all.Transactions, payload.Transactions...)
	}
	for _, ss := range sampledSpans {
		var found bool
		for _, trace := range all.Traces {
			if reflect.DeepEqual(trace.Spans, ss.Traces[0].Spans) {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("payloads didn't contain given traces")
		}
		for _, event := range ss.Events {
			assert.Contains(t, all.Transactions, event)
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
		testSpans := []*SampledSpans{
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
		testSpans := []*SampledSpans{
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
