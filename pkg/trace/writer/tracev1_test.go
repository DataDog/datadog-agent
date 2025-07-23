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
	"testing"

	compression "github.com/DataDog/datadog-agent/comp/trace/compression/def"
	gzip "github.com/DataDog/datadog-agent/comp/trace/compression/impl-gzip"
	zstd "github.com/DataDog/datadog-agent/comp/trace/compression/impl-zstd"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
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
		t.Run(fmt.Sprintf("encoding:%s", tc.compressor.Encoding()), func(t *testing.T) {
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
			// Use a flush threshold that allows the first two entries to not overflow,
			// but overflow on the third.
			defer useFlushThreshold(testSpans[0].Size + testSpans[1].Size + 10)()
			tw := NewTraceWriterV1(cfg, mockSampler, mockSampler, mockSampler, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, &timing.NoopReporter{}, tc.compressor)
			for _, ss := range testSpans {
				tw.WriteChunksV1(ss)
			}
			tw.Stop()
			// One payload flushes due to overflowing the threshold, and the second one
			// because of stop.
			assert.Equal(t, 2, srv.Accepted())
			payloadsContainV1(t, srv.Payloads(), testSpans, tc.compressor)
		})
	}
}

// randomSampledSpans returns a set of spans sampled spans and events events.
func randomSampledSpansV1(spans, events int) *SampledChunksV1 {
	realisticIDs := true
	trace := testutil.GetTestTracesV1(1, spans, realisticIDs)
	return &SampledChunksV1{
		TracerPayload: trace,
		Size:          trace.Msgsize(), // TODO: what's up with the "events" here?
		SpanCount:     int64(spans),
		EventCount:    int64(events),
	}
}

// payloadsContain checks that the given payloads contain the given set of sampled spans.
func payloadsContainV1(t *testing.T, payloads []*payload, sampledSpans []*SampledChunksV1, compressor compression.Component) {
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
		all.IdxTracerPayloads = append(all.IdxTracerPayloads, payload.IdxTracerPayloads...)
	}
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
