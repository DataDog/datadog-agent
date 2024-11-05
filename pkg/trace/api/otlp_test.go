// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/DataDog/datadog-agent/pkg/trace/transform"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	semconv "go.opentelemetry.io/collector/semconv/v1.6.1"
)

var otlpTestSpanConfig = &testutil.OTLPSpan{
	TraceState: "state",
	Name:       "/path",
	Kind:       ptrace.SpanKindServer,
	Attributes: map[string]interface{}{
		"name":   "john",
		"approx": 1.2,
		"count":  2,
	},
	Events: []testutil.OTLPSpanEvent{
		{
			Timestamp: 123,
			Name:      "boom",
			Attributes: map[string]interface{}{
				"key":      "Out of memory",
				"accuracy": 2.4,
			},
			Dropped: 2,
		},
		{
			Timestamp: 456,
			Name:      "exception",
			Attributes: map[string]interface{}{
				"exception.message":    "Out of memory",
				"exception.type":       "mem",
				"exception.stacktrace": "1/2/3",
			},
			Dropped: 2,
		},
	},
	Links: []testutil.OTLPSpanLink{
		{
			TraceID:    "fedcba98765432100123456789abcdef",
			SpanID:     "abcdef0123456789",
			TraceState: "dd=asdf256,ee=jkl;128",
			Attributes: map[string]interface{}{
				"a1": "v1",
				"a2": "v2",
			},
			Dropped: 24,
		},
		{
			TraceID:    "abcdef0123456789abcdef0123456789",
			SpanID:     "fedcba9876543210",
			TraceState: "",
			Attributes: map[string]interface{}{
				"a3": "v2",
				"a4": "v4",
			},
			Dropped: 0,
		},
		{
			TraceID:    "abcdef0123456789abcdef0123456789",
			SpanID:     "fedcba9876543210",
			TraceState: "",
			Attributes: map[string]interface{}{},
			Dropped:    2,
		},
		{
			TraceID:    "abcdef0123456789abcdef0123456789",
			SpanID:     "fedcba9876543210",
			TraceState: "",
			Attributes: map[string]interface{}{},
			Dropped:    0,
		},
	},
	StatusMsg:  "Error",
	StatusCode: ptrace.StatusCodeError,
}

var otlpTestSpan = testutil.NewOTLPSpan(otlpTestSpanConfig)

var otlpTestTracesRequest = testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
	{
		LibName:    "libname",
		LibVersion: "v1.2.3",
		Attributes: map[string]interface{}{
			"service.name": "mongodb",
			"binary":       "rundb",
		},
		Spans: []*testutil.OTLPSpan{otlpTestSpanConfig},
	},
	{
		LibName:    "othername",
		LibVersion: "v1.2.0",
		Attributes: map[string]interface{}{
			"service.name": "pylons",
			"binary":       "runweb",
		},
		Spans: []*testutil.OTLPSpan{otlpTestSpanConfig},
	},
})

func NewTestConfig(t *testing.T) *config.AgentConfig {
	cfg := config.New()
	attributesTranslator, err := attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
	require.NoError(t, err)
	cfg.OTLPReceiver.AttributesTranslator = attributesTranslator
	return cfg
}

func NewBenchmarkTestConfig(b *testing.B) *config.AgentConfig {
	cfg := config.New()
	attributesTranslator, err := attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
	require.NoError(b, err)
	cfg.OTLPReceiver.AttributesTranslator = attributesTranslator
	return cfg
}

func TestOTLPMetrics(t *testing.T) {
	t.Run("ReceiveResourceSpansV1", func(t *testing.T) {
		testOTLPMetrics(false, t)
	})

	t.Run("ReceiveResourceSpansV2", func(t *testing.T) {
		testOTLPMetrics(true, t)
	})
}

func testOTLPMetrics(enableReceiveResourceSpansV2 bool, t *testing.T) {
	assert := assert.New(t)
	cfg := NewTestConfig(t)
	if enableReceiveResourceSpansV2 {
		cfg.Features["enable_receive_resource_spans_v2"] = struct{}{}
	}
	stats := &teststatsd.Client{}

	out := make(chan *Payload, 1)
	rcv := NewOTLPReceiver(out, cfg, stats, &timing.NoopReporter{})
	rspans := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
		{
			LibName:    "libname",
			LibVersion: "1.2",
			Attributes: map[string]interface{}{},
			Spans: []*testutil.OTLPSpan{
				{Name: "1"},
				{Name: "2"},
				{Name: "3"},
			},
		},
		{
			LibName:    "other-libname",
			LibVersion: "2.1",
			Attributes: map[string]interface{}{},
			Spans: []*testutil.OTLPSpan{
				{Name: "4", TraceID: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}},
				{Name: "5", TraceID: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}},
			},
		},
	}).Traces().ResourceSpans()

	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-out:
			case <-stop:
				return
			}
		}
	}()
	defer close(stop)

	rcv.ReceiveResourceSpans(context.Background(), rspans.At(0), http.Header{})
	rcv.ReceiveResourceSpans(context.Background(), rspans.At(1), http.Header{})

	calls := stats.CountCalls
	assert.Equal(4, len(calls))
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.spans", Value: 3, Tags: []string{"tracer_version:otlp-", "endpoint_version:opentelemetry_grpc_v1"}, Rate: 1})
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.spans", Value: 2, Tags: []string{"tracer_version:otlp-", "endpoint_version:opentelemetry_grpc_v1"}, Rate: 1})
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.traces", Value: 1, Tags: []string{"tracer_version:otlp-", "endpoint_version:opentelemetry_grpc_v1"}, Rate: 1})
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.traces", Value: 2, Tags: []string{"tracer_version:otlp-", "endpoint_version:opentelemetry_grpc_v1"}, Rate: 1})
}

func TestOTLPNameRemapping(t *testing.T) {
	t.Run("ReceiveResourceSpansV1", func(t *testing.T) {
		testOTLPNameRemapping(false, t)
	})

	t.Run("ReceiveResourceSpansV2", func(t *testing.T) {
		testOTLPNameRemapping(true, t)
	})
}

func testOTLPNameRemapping(enableReceiveResourceSpansV2 bool, t *testing.T) {
	cfg := NewTestConfig(t)
	if enableReceiveResourceSpansV2 {
		cfg.Features["enable_receive_resource_spans_v2"] = struct{}{}
	}
	cfg.OTLPReceiver.SpanNameRemappings = map[string]string{"libname.unspecified": "new"}
	out := make(chan *Payload, 1)
	rcv := NewOTLPReceiver(out, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
	rcv.ReceiveResourceSpans(context.Background(), testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
		{
			LibName:    "libname",
			LibVersion: "1.2",
			Attributes: map[string]interface{}{},
			Spans: []*testutil.OTLPSpan{
				{Name: "asd"},
			},
		},
	}).Traces().ResourceSpans().At(0), http.Header{})
	timeout := time.After(500 * time.Millisecond)
	select {
	case <-timeout:
		t.Fatal("timed out")
	case p := <-out:
		assert.Equal(t, "new", p.TracerPayload.Chunks[0].Spans[0].Name)
	}
}

func TestCreateChunks(t *testing.T) {
	t.Run("ReceiveResourceSpansV1", func(t *testing.T) {
		testCreateChunk(false, t)
	})

	t.Run("ReceiveResourceSpansV2", func(t *testing.T) {
		testCreateChunk(true, t)
	})
}

func testCreateChunk(enableReceiveResourceSpansV2 bool, t *testing.T) {
	cfg := NewTestConfig(t)
	if enableReceiveResourceSpansV2 {
		cfg.Features["enable_receive_resource_spans_v2"] = struct{}{}
	}
	cfg.OTLPReceiver.ProbabilisticSampling = 50
	o := NewOTLPReceiver(nil, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
	const (
		traceID1 = 123           // sampled by 50% rate
		traceID2 = 1237892138897 // not sampled by 50% rate
		traceID3 = 1237892138898 // not sampled by 50% rate
	)
	traces := map[uint64]pb.Trace{
		traceID1: {{TraceID: traceID1, SpanID: 1}, {TraceID: traceID1, SpanID: 2}},
		traceID2: {{TraceID: traceID2, SpanID: 1}, {TraceID: traceID2, SpanID: 2}},
		traceID3: {{TraceID: traceID3, SpanID: 1}, {TraceID: traceID3, SpanID: 2}},
	}
	priorities := map[uint64]sampler.SamplingPriority{
		traceID3: sampler.PriorityUserKeep,
	}
	chunks := o.createChunks(traces, priorities)
	var found int
	for _, c := range chunks {
		id := c.Spans[0].TraceID
		require.ElementsMatch(t, c.Spans, traces[id])
		require.Equal(t, "0.50", c.Tags["_dd.otlp_sr"])
		switch id {
		case traceID1:
			//nolint:revive // TODO(OTEL) Fix revive linter
			found += 1
			require.Equal(t, "-9", c.Spans[0].Meta["_dd.p.dm"])
			require.Equal(t, int32(1), c.Priority)
		case traceID2:
			found += 2
			require.Equal(t, "-9", c.Spans[0].Meta["_dd.p.dm"])
			require.Equal(t, int32(0), c.Priority)
		case traceID3:
			found += 3
			require.Equal(t, "-4", c.Spans[0].Meta["_dd.p.dm"])
			require.Equal(t, int32(2), c.Priority)
		}
	}
	require.Equal(t, 6, found)
}

func TestOTLPReceiveResourceSpans(t *testing.T) {
	t.Run("ReceiveResourceSpansV1", func(t *testing.T) {
		testOTLPReceiveResourceSpans(false, t)
	})

	t.Run("ReceiveResourceSpansV2", func(t *testing.T) {
		testOTLPReceiveResourceSpans(true, t)
	})
}

func testOTLPReceiveResourceSpans(enableReceiveResourceSpansV2 bool, t *testing.T) {
	cfg := NewTestConfig(t)
	if enableReceiveResourceSpansV2 {
		cfg.Features["enable_receive_resource_spans_v2"] = struct{}{}
	}
	out := make(chan *Payload, 1)
	rcv := NewOTLPReceiver(out, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
	require := require.New(t)
	for _, tt := range []struct {
		enableReceiveResourceSpansV2 bool
		in                           []testutil.OTLPResourceSpan
		fn                           func(*pb.TracerPayload)
	}{
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{string(semconv.AttributeDeploymentEnvironment): "depenv"},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("depenv", out.Env)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{"deployment.environment.name": "staging"},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("staging", out.Env)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{Attributes: map[string]interface{}{string(semconv.AttributeDeploymentEnvironment): "spanenv"}},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				if !enableReceiveResourceSpansV2 {
					require.Equal("spanenv", out.Env)
				}
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{Attributes: map[string]interface{}{"deployment.environment.name": "spanenv2"}},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				if !enableReceiveResourceSpansV2 {
					require.Equal("spanenv2", out.Env)
				}
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{"_dd.hostname": "dd.host"},
				},
			},
			fn: func(out *pb.TracerPayload) {
				if !enableReceiveResourceSpansV2 {
					require.Equal("dd.host", out.Hostname)
				}
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{string(semconv.AttributeContainerID): "1234cid"},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("1234cid", out.ContainerID)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{
						string(semconv.AttributeK8SPodUID):          "1234cid",
						string(semconv.AttributeK8SJobName):         "kubejob",
						string(semconv.AttributeContainerImageName): "lorem-ipsum",
						string(semconv.AttributeContainerImageTag):  "v2.0",
						string("datadog.container.tag.team"):        "otel",
					},
					Spans: []*testutil.OTLPSpan{
						{
							TraceID: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
							Name:    "first",
							Attributes: map[string]interface{}{
								// We should not fetch container tags from Span Attributes.
								string(semconv.AttributeK8SContainerName): "lorem-ipsum",
							},
						},
						{
							TraceID: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 17},
							SpanID:  [8]byte{10, 10, 11, 12, 13, 14, 15, 16},
							Name:    "second",
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("1234cid", out.ContainerID)
				require.Equal(map[string]string{
					"kube_job":   "kubejob",
					"image_name": "lorem-ipsum",
					"image_tag":  "v2.0",
					"team":       "otel",
				}, unflatten(out.Tags[tagContainersTags]))
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{Attributes: map[string]interface{}{string(semconv.AttributeK8SPodUID): "123cid"}},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				if !enableReceiveResourceSpansV2 {
					require.Equal("123cid", out.ContainerID)
				}
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{Attributes: map[string]interface{}{string(semconv.AttributeContainerID): "23cid"}},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				if !enableReceiveResourceSpansV2 {
					require.Equal("23cid", out.ContainerID)
				}
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					Spans: []*testutil.OTLPSpan{
						{
							TraceID: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
							Name:    "first",
						},
						{
							TraceID: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 17},
							SpanID:  [8]byte{10, 10, 11, 12, 13, 14, 15, 16},
							Name:    "second",
						},
						{
							TraceID: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 17},
							SpanID:  [8]byte{9, 10, 11, 12, 13, 14, 15, 16},
							Name:    "third",
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Len(out.Chunks, 2)
				if len(out.Chunks[0].Spans) == 2 {
					// it seems the chunks ended up in the wrong order; that's fine
					// switch them to ensure assertions are correct
					out.Chunks[0], out.Chunks[1] = out.Chunks[1], out.Chunks[0]
				}
				require.Equal(uint64(0x90a0b0c0d0e0f10), out.Chunks[0].Spans[0].TraceID)
				require.Len(out.Chunks[1].Spans, 2)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					Spans: []*testutil.OTLPSpan{
						{
							TraceID:    [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
							Name:       "first",
							Attributes: map[string]interface{}{"_sampling_priority_v1": -1},
						},
						{
							TraceID:    [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 17},
							SpanID:     [8]byte{10, 10, 11, 12, 13, 14, 15, 16},
							Name:       "second",
							Attributes: map[string]interface{}{"_sampling_priority_v1": 2},
						},
						{
							TraceID:    [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 17},
							SpanID:     [8]byte{9, 10, 11, 12, 13, 14, 15, 16},
							Name:       "third",
							Attributes: map[string]interface{}{"_sampling_priority_v1": 3},
						},
						{
							TraceID:    [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 18},
							SpanID:     [8]byte{9, 10, 11, 12, 13, 14, 15, 16},
							Name:       "third",
							Attributes: map[string]interface{}{"_sampling_priority_v1": 0},
						},
						{
							TraceID: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 19},
							SpanID:  [8]byte{9, 10, 11, 12, 13, 14, 15, 16},
							Name:    "third",
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Len(out.Chunks, 4) // 4 traces total
				// expected priorities by TraceID
				traceIDPriority := map[uint64]int32{
					0x90a0b0c0d0e0f10: -1,
					0x90a0b0c0d0e0f11: 3,
					0x90a0b0c0d0e0f12: 0,
					0x90a0b0c0d0e0f13: 1,
				}
				for i := 0; i < 4; i++ {
					traceID := out.Chunks[i].Spans[0].TraceID
					p, ok := traceIDPriority[traceID]
					require.True(ok, fmt.Sprintf("%v trace ID not found", traceID))
					require.Equal(p, out.Chunks[i].Priority)
				}
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					Spans: []*testutil.OTLPSpan{
						{
							TraceID:    [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
							Name:       "first",
							Attributes: map[string]interface{}{"sampling.priority": -1},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Len(out.Chunks, 1)
				require.Equal(int32(-1), out.Chunks[0].Priority)
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			rspans := testutil.NewOTLPTracesRequest(tt.in).Traces().ResourceSpans().At(0)
			rcv.ReceiveResourceSpans(context.Background(), rspans, http.Header{})
			timeout := time.After(500 * time.Millisecond)
			select {
			case <-timeout:
				t.Fatal("timed out")
			case p := <-out:
				tt.fn(p.TracerPayload)
			}
		})
	}

	// testAndExpect tests the ReceiveResourceSpans method by feeding it the given spans and header and running
	// the fn function on the outputted payload. It waits for the payload up to 500ms after which it times out.
	testAndExpect := func(spans []testutil.OTLPResourceSpan, header http.Header, fn func(p *Payload)) func(t *testing.T) {
		return func(t *testing.T) {
			rspans := testutil.NewOTLPTracesRequest(spans).Traces().ResourceSpans().At(0)
			rcv.ReceiveResourceSpans(context.Background(), rspans, header)
			timeout := time.After(500 * time.Millisecond)
			select {
			case <-timeout:
				t.Fatal("timed out")
			case p := <-out:
				fn(p)
			}
		}
	}

	t.Run("ClientComputedStats", func(t *testing.T) {
		testSpans := [...][]testutil.OTLPResourceSpan{
			{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans:      []*testutil.OTLPSpan{{Attributes: map[string]interface{}{string(semconv.AttributeK8SPodUID): "123cid"}}},
				},
			},
			{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{
						// these spans are marked as having had stats computed
						keyStatsComputed: "true",
					},
					Spans: []*testutil.OTLPSpan{{Attributes: map[string]interface{}{string(semconv.AttributeK8SPodUID): "123cid"}}},
				},
			},
		}

		t.Run("default", testAndExpect(testSpans[0], http.Header{}, func(p *Payload) {
			require.False(p.ClientComputedStats)
		}))

		t.Run("header", testAndExpect(testSpans[0], http.Header{
			header.ComputedStats: []string{"true"},
		}, func(p *Payload) {
			require.True(p.ClientComputedStats)
		}))

		t.Run("resource", testAndExpect(testSpans[1], http.Header{}, func(p *Payload) {
			require.True(p.ClientComputedStats)
		}))
	})

	t.Run("ClientComputedTopLevel", func(t *testing.T) {
		testSpans := []testutil.OTLPResourceSpan{{
			LibName:    "libname",
			LibVersion: "1.2",
			Attributes: map[string]interface{}{},
			Spans:      []*testutil.OTLPSpan{{Attributes: map[string]interface{}{string(semconv.AttributeK8SPodUID): "123cid"}}},
		}}

		t.Run("default", testAndExpect(testSpans, http.Header{}, func(p *Payload) {
			require.False(p.ClientComputedTopLevel)
		}))

		cfg.Features["enable_otlp_compute_top_level_by_span_kind"] = struct{}{}

		t.Run("withFeatureFlag", testAndExpect(testSpans, http.Header{}, func(p *Payload) {
			require.True(p.ClientComputedTopLevel)
		}))

		t.Run("header", testAndExpect(testSpans, http.Header{
			header.ComputedTopLevel: []string{"true"},
		}, func(p *Payload) {
			require.True(p.ClientComputedTopLevel)
		}))

		t.Run("headerWithFeatureFlag", testAndExpect(testSpans, http.Header{
			header.ComputedTopLevel: []string{"true"},
		}, func(p *Payload) {
			require.True(p.ClientComputedTopLevel)
		}))
	})
}

func TestOTLPSetAttributes(t *testing.T) {
	t.Run("SetMetaOTLP", func(t *testing.T) {
		s := &pb.Span{Meta: make(map[string]string), Metrics: make(map[string]float64)}

		transform.SetMetaOTLP(s, "a", "b")
		require.Equal(t, "b", s.Meta["a"])

		transform.SetMetaOTLP(s, "operation.name", "on")
		require.Equal(t, "on", s.Name)

		transform.SetMetaOTLP(s, "service.name", "sn")
		require.Equal(t, "sn", s.Service)

		transform.SetMetaOTLP(s, "span.type", "st")
		require.Equal(t, "st", s.Type)

		transform.SetMetaOTLP(s, "analytics.event", "true")
		require.Equal(t, float64(1), s.Metrics[sampler.KeySamplingRateEventExtraction])

		transform.SetMetaOTLP(s, "analytics.event", "false")
		require.Equal(t, float64(0), s.Metrics[sampler.KeySamplingRateEventExtraction])
	})

	t.Run("SetMetricOTLP", func(t *testing.T) {
		s := &pb.Span{Meta: make(map[string]string), Metrics: make(map[string]float64)}

		transform.SetMetricOTLP(s, "a", 1)
		require.Equal(t, float64(1), s.Metrics["a"])

		transform.SetMetricOTLP(s, "sampling.priority", 2)
		require.Equal(t, float64(2), s.Metrics["_sampling_priority_v1"])

		transform.SetMetricOTLP(s, "_sampling_priority_v1", 3)
		require.Equal(t, float64(3), s.Metrics["_sampling_priority_v1"])
	})
}

func unflatten(str string) map[string]string {
	parts := strings.Split(str, ",")
	m := make(map[string]string, len(parts))
	if len(str) == 0 {
		return m
	}
	for _, p := range parts {
		parts2 := strings.SplitN(p, ":", 2)
		k := parts2[0]
		if k == "" {
			continue
		}
		if len(parts2) > 1 {
			m[k] = parts2[1]
		} else {
			m[k] = ""
		}
	}
	return m
}

func TestUnflatten(t *testing.T) {
	for in, out := range map[string]map[string]string{
		"a:b": {
			"a": "b",
		},
		"a:b,c:d": {
			"a": "b",
			"c": "d",
		},
		"a:b,c:d:e": {
			"a": "b",
			"c": "d:e",
		},
		"a:b,c": {
			"a": "b",
			"c": "",
		},
		"a:b,": {
			"a": "b",
		},
		"bogus": {
			"bogus": "",
		},
		"": {},
	} {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, unflatten(in), out)
		})
	}
}

func TestOTLPHostname(t *testing.T) {
	t.Run("ReceiveResourceSpansV1", func(t *testing.T) {
		testOTLPHostname(false, t)
	})

	t.Run("ReceiveResourceSpansV2", func(t *testing.T) {
		testOTLPHostname(true, t)
	})
}

func testOTLPHostname(enableReceiveResourceSpansV2 bool, t *testing.T) {
	testcases := []struct {
		config, resource, span string
		out                    string
	}{
		{
			config:   "config-hostname",
			resource: "resource-hostname",
			span:     "span-hostname",
			out:      "resource-hostname",
		},
		{
			config: "config-hostname",
			out:    "config-hostname",
		},
	}

	if !enableReceiveResourceSpansV2 {
		testcases = append(testcases, struct {
			config, resource, span string
			out                    string
		}{
			config: "config-hostname",
			span:   "span-hostname",
			out:    "span-hostname",
		})
	}
	for _, tt := range testcases {
		cfg := NewTestConfig(t)
		if enableReceiveResourceSpansV2 {
			cfg.Features["enable_receive_resource_spans_v2"] = struct{}{}
		}
		cfg.Hostname = tt.config
		out := make(chan *Payload, 1)
		rcv := NewOTLPReceiver(out, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
		rattr := map[string]interface{}{}
		if tt.resource != "" {
			rattr["datadog.host.name"] = tt.resource
		}
		sattr := map[string]interface{}{}
		if tt.span != "" {
			rattr["_dd.hostname"] = tt.span
		}
		src := rcv.ReceiveResourceSpans(context.Background(), testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
			{
				LibName:    "a",
				LibVersion: "1.2",
				Attributes: rattr,
				Spans:      []*testutil.OTLPSpan{{Attributes: sattr}},
			},
		}).Traces().ResourceSpans().At(0), http.Header{})
		assert.Equal(t, src.Kind, source.HostnameKind)
		assert.Equal(t, src.Identifier, tt.out)
		timeout := time.After(500 * time.Millisecond)
		select {
		case <-timeout:
			t.Fatal("timed out")
		case p := <-out:
			assert.Equal(t, tt.out, p.TracerPayload.Hostname)
		}
	}
}

func TestOTLPReceiver(t *testing.T) {
	t.Run("ReceiveResourceSpansV1", func(t *testing.T) {
		testOTLPReceiver(false, t)
	})

	t.Run("ReceiveResourceSpansV2", func(t *testing.T) {
		testOTLPReceiver(true, t)
	})
}

func testOTLPReceiver(enableReceiveResourceSpansV2 bool, t *testing.T) {
	t.Run("New", func(t *testing.T) {
		cfg := NewTestConfig(t)
		if enableReceiveResourceSpansV2 {
			cfg.Features["enable_receive_resource_spans_v2"] = struct{}{}
		}
		assert.NotNil(t, NewOTLPReceiver(nil, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{}).conf)
	})

	t.Run("Start/nil", func(t *testing.T) {
		cfg := NewTestConfig(t)
		if enableReceiveResourceSpansV2 {
			cfg.Features["enable_receive_resource_spans_v2"] = struct{}{}
		}
		o := NewOTLPReceiver(nil, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
		o.Start()
		defer o.Stop()
		assert.Nil(t, o.grpcsrv)
	})

	t.Run("Start/grpc", func(t *testing.T) {
		port := testutil.FreeTCPPort(t)
		cfg := NewTestConfig(t)
		if enableReceiveResourceSpansV2 {
			cfg.Features["enable_receive_resource_spans_v2"] = struct{}{}
		}
		cfg.OTLPReceiver = &config.OTLP{
			BindHost: "localhost",
			GRPCPort: port,
		}
		o := NewOTLPReceiver(nil, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
		o.Start()
		defer o.Stop()
		assert := assert.New(t)
		assert.NotNil(o.grpcsrv)
		svc, ok := o.grpcsrv.GetServiceInfo()["opentelemetry.proto.collector.trace.v1.TraceService"]
		assert.True(ok)
		assert.Equal("opentelemetry/proto/collector/trace/v1/trace_service.proto", svc.Metadata)
		assert.Equal("Export", svc.Methods[0].Name)
	})

	t.Run("processRequest", func(t *testing.T) {
		out := make(chan *Payload, 5)
		cfg := NewTestConfig(t)
		if enableReceiveResourceSpansV2 {
			cfg.Features["enable_receive_resource_spans_v2"] = struct{}{}
		}
		o := NewOTLPReceiver(out, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
		o.processRequest(context.Background(), http.Header(map[string][]string{
			header.Lang:        {"go"},
			header.ContainerID: {"containerdID"},
		}), otlpTestTracesRequest)
		ps := make([]*Payload, 2)
		timeout := time.After(time.Second / 2)
		for i := 0; i < 2; i++ {
			select {
			case p := <-out:
				if !enableReceiveResourceSpansV2 {
					assert.Equal(t, "go", p.Source.Lang)
				}
				assert.Equal(t, "opentelemetry_grpc_v1", p.Source.EndpointVersion)
				assert.Len(t, p.TracerPayload.Chunks, 1)
				ps[i] = p
			case <-timeout:
				t.Fatal("timed out")
			}
		}
	})
}

var (
	otlpTestSpanID  = pcommon.SpanID([8]byte{0x24, 0x0, 0x31, 0xea, 0xd7, 0x50, 0xe5, 0xf3})
	otlpTestTraceID = pcommon.TraceID([16]byte{0x72, 0xdf, 0x52, 0xa, 0xf2, 0xbd, 0xe7, 0xa5, 0x24, 0x0, 0x31, 0xea, 0xd7, 0x50, 0xe5, 0xf3})
)

func TestOTLPHelpers(t *testing.T) {
	t.Run("byteArrayToUint64", func(t *testing.T) {
		assert.Equal(t, uint64(0x240031ead750e5f3), traceutil.OTelTraceIDToUint64([16]byte(otlpTestTraceID)))
		assert.Equal(t, uint64(0x240031ead750e5f3), traceutil.OTelSpanIDToUint64([8]byte(otlpTestSpanID)))
	})

	t.Run("spanKindNames", func(t *testing.T) {
		for in, out := range map[ptrace.SpanKind]string{
			ptrace.SpanKindUnspecified: "unspecified",
			ptrace.SpanKindInternal:    "internal",
			ptrace.SpanKindServer:      "server",
			ptrace.SpanKindClient:      "client",
			ptrace.SpanKindProducer:    "producer",
			ptrace.SpanKindConsumer:    "consumer",
			99:                         "unspecified",
		} {
			assert.Equal(t, out, traceutil.OTelSpanKindName(in))
		}
	})

	t.Run("status2Error", func(t *testing.T) {
		for _, tt := range []*struct {
			status ptrace.StatusCode
			msg    string
			events ptrace.SpanEventSlice
			out    pb.Span
		}{
			{
				status: ptrace.StatusCodeError,
				events: makeEventsSlice("exception", map[string]any{
					"exception.message":    "Out of memory",
					"exception.type":       "mem",
					"exception.stacktrace": "1/2/3",
				}, 0, 0),
				out: pb.Span{
					Error: 1,
					Meta: map[string]string{
						"error.msg":   "Out of memory",
						"error.type":  "mem",
						"error.stack": "1/2/3",
					},
				},
			},
			{
				status: ptrace.StatusCodeError,
				events: makeEventsSlice("exception", map[string]any{
					"exception.message": "Out of memory",
				}, 0, 0),
				out: pb.Span{
					Error: 1,
					Meta:  map[string]string{"error.msg": "Out of memory"},
				},
			},
			{
				status: ptrace.StatusCodeError,
				events: makeEventsSlice("EXCEPTION", map[string]any{
					"exception.message": "Out of memory",
				}, 0, 0),
				out: pb.Span{
					Error: 1,
					Meta:  map[string]string{"error.msg": "Out of memory"},
				},
			},
			{
				status: ptrace.StatusCodeError,
				events: makeEventsSlice("OTher", map[string]any{
					"exception.message": "Out of memory",
				}, 0, 0),
				out: pb.Span{Error: 1},
			},
			{
				status: ptrace.StatusCodeError,
				events: ptrace.NewSpanEventSlice(),
				out:    pb.Span{Error: 1},
			},
			{
				status: ptrace.StatusCodeError,
				msg:    "Error number #24",
				events: ptrace.NewSpanEventSlice(),
				out:    pb.Span{Error: 1, Meta: map[string]string{"error.msg": "Error number #24"}},
			},
			{
				status: ptrace.StatusCodeOk,
				events: ptrace.NewSpanEventSlice(),
				out:    pb.Span{Error: 0},
			},
			{
				status: ptrace.StatusCodeOk,
				events: makeEventsSlice("exception", map[string]any{
					"exception.message":    "Out of memory",
					"exception.type":       "mem",
					"exception.stacktrace": "1/2/3",
				}, 0, 0),
				out: pb.Span{Error: 0},
			},
		} {
			assert := assert.New(t)
			span := pb.Span{Meta: make(map[string]string)}
			status := ptrace.NewStatus()
			status.SetCode(tt.status)
			status.SetMessage(tt.msg)
			transform.Status2Error(status, tt.events, &span)
			assert.Equal(tt.out.Error, span.Error)
			for _, prop := range []string{"error.msg", "error.type", "error.stack"} {
				if v, ok := tt.out.Meta[prop]; ok {
					assert.Equal(v, span.Meta[prop])
				} else {
					_, ok := span.Meta[prop]
					assert.False(ok, prop)
				}
			}
		}
	})

	t.Run("resourceFromTags", func(t *testing.T) {
		for _, tt := range []struct {
			meta map[string]string
			out  string
		}{
			{
				meta: nil,
				out:  "",
			},
			{
				meta: map[string]string{"http.method": "GET"},
				out:  "GET",
			},
			{
				meta: map[string]string{"http.method": "POST", "http.route": "/settings"},
				out:  "POST /settings",
			},
			{
				meta: map[string]string{"http.method": "POST", "grpc.path": "/settings"},
				out:  "POST /settings",
			},
			{
				meta: map[string]string{"messaging.operation": "DO"},
				out:  "DO",
			},
			{
				meta: map[string]string{"messaging.operation": "DO", "messaging.destination": "OP"},
				out:  "DO OP",
			},
			{
				meta: map[string]string{"messaging.operation": "DO", "messaging.destination.name": "OP"},
				out:  "DO OP",
			},
			{
				meta: map[string]string{"messaging.operation": "process", "messaging.destination.name": "Queue1", "messaging.destination": "Queue2"},
				out:  "process Queue2",
			},
			{
				meta: map[string]string{semconv.AttributeRPCService: "SVC", semconv.AttributeRPCMethod: "M"},
				out:  "M SVC",
			},
			{
				meta: map[string]string{semconv.AttributeRPCMethod: "M"},
				out:  "M",
			},
		} {
			assert.Equal(t, tt.out, resourceFromTags(tt.meta))
		}
	})

	t.Run("spanKind2Type", func(t *testing.T) {
		for _, tt := range []struct {
			kind ptrace.SpanKind
			meta map[string]string
			out  string
		}{
			{
				kind: ptrace.SpanKindServer,
				out:  "web",
			},
			{
				kind: ptrace.SpanKindClient,
				out:  "http",
			},
			{
				kind: ptrace.SpanKindClient,
				meta: map[string]string{"db.system": "redis"},
				out:  "cache",
			},
			{
				kind: ptrace.SpanKindClient,
				meta: map[string]string{"db.system": "memcached"},
				out:  "cache",
			},
			{
				kind: ptrace.SpanKindClient,
				meta: map[string]string{"db.system": "other"},
				out:  "db",
			},
			{
				kind: ptrace.SpanKindProducer,
				out:  "custom",
			},
			{
				kind: ptrace.SpanKindConsumer,
				out:  "custom",
			},
			{
				kind: ptrace.SpanKindInternal,
				out:  "custom",
			},
			{
				kind: ptrace.SpanKindUnspecified,
				out:  "custom",
			},
		} {
			assert.Equal(t, tt.out, spanKind2Type(tt.kind, &pb.Span{Meta: tt.meta}))
		}
	})

	t.Run("tagsFromHeaders", func(t *testing.T) {
		out := tagsFromHeaders(http.Header(map[string][]string{
			header.Lang:                  {"go"},
			header.LangVersion:           {"1.14"},
			header.LangInterpreter:       {"x"},
			header.LangInterpreterVendor: {"y"},
		}))
		assert.Equal(t, []string{"endpoint_version:opentelemetry_grpc_v1", "lang:go", "lang_version:1.14", "interpreter:x", "lang_vendor:y"}, out)
	})
}

func TestOTLPConvertSpan(t *testing.T) {
	t.Run("ReceiveResourceSpansV1", func(t *testing.T) {
		testOTLPConvertSpan(false, t)
	})

	t.Run("ReceiveResourceSpansV2", func(t *testing.T) {
		testOTLPConvertSpan(true, t)
	})
}

func testOTLPConvertSpan(enableReceiveResourceSpansV2 bool, t *testing.T) {
	now := uint64(otlpTestSpan.StartTimestamp())
	cfg := NewTestConfig(t)
	if enableReceiveResourceSpansV2 {
		cfg.Features["enable_receive_resource_spans_v2"] = struct{}{}
	}
	o := NewOTLPReceiver(nil, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
	for i, tt := range []struct {
		rattr              map[string]string
		libname            string
		libver             string
		in                 ptrace.Span
		out                *pb.Span
		outTags            map[string]string
		topLevelOutMetrics map[string]float64
	}{
		{
			rattr: map[string]string{
				"service.name":    "pylons",
				"service.version": "v1.2.3",
				"env":             "staging",
			},
			libname: "ddtracer",
			libver:  "v2",
			in:      otlpTestSpan,
			out: &pb.Span{
				Service:  "pylons",
				Name:     "ddtracer.server",
				Resource: "/path",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Error:    1,
				Meta: map[string]string{
					"name":                    "john",
					"otel.trace_id":           "72df520af2bde7a5240031ead750e5f3",
					"env":                     "staging",
					"otel.status_code":        "Error",
					"otel.status_description": "Error",
					"otel.library.name":       "ddtracer",
					"otel.library.version":    "v2",
					"service.version":         "v1.2.3",
					"w3c.tracestate":          "state",
					"version":                 "v1.2.3",
					"events":                  `[{"time_unix_nano":123,"name":"boom","attributes":{"key":"Out of memory","accuracy":2.4},"dropped_attributes_count":2},{"time_unix_nano":456,"name":"exception","attributes":{"exception.message":"Out of memory","exception.type":"mem","exception.stacktrace":"1/2/3"},"dropped_attributes_count":2}]`,
					"_dd.span_links":          `[{"trace_id":"fedcba98765432100123456789abcdef","span_id":"abcdef0123456789","trace_state":"dd=asdf256,ee=jkl;128", "attributes":{"a1":"v1","a2":"v2"},"dropped_attributes_count":24},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210","attributes":{"a3":"v2","a4":"v4"}},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210","dropped_attributes_count":2},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210"}]`,
					"error.msg":               "Out of memory",
					"error.type":              "mem",
					"error.stack":             "1/2/3",
					"span.kind":               "server",
				},
				Metrics: map[string]float64{
					"approx": 1.2,
					"count":  2,
				},
				Type: "web",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
				"approx":     1.2,
				"count":      2,
			},
		}, {
			rattr: map[string]string{
				"service.version": "v1.2.3",
				"service.name":    "myservice",
				"peer.service":    "mypeerservice",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID:    otlpTestTraceID,
				SpanID:     otlpTestSpanID,
				TraceState: "state",
				Name:       "/path",
				Kind:       ptrace.SpanKindServer,
				Start:      now,
				End:        now + 200000000,
				Attributes: map[string]interface{}{
					"name":                   "john",
					"peer.service":           "userbase",
					"deployment.environment": "prod",
					"http.method":            "GET",
					"http.route":             "/path",
					"approx":                 1.2,
					"count":                  2,
					"span.kind":              "server",
				},
				Events: []testutil.OTLPSpanEvent{
					{
						Timestamp: 123,
						Name:      "boom",
						Attributes: map[string]interface{}{
							"message":  "Out of memory",
							"accuracy": 2.4,
						},
						Dropped: 2,
					},
					{
						Timestamp: 456,
						Name:      "exception",
						Attributes: map[string]interface{}{
							"exception.message":    "Out of memory",
							"exception.type":       "mem",
							"exception.stacktrace": "1/2/3",
						},
						Dropped: 2,
					},
				},
				Links: []testutil.OTLPSpanLink{
					{
						TraceID:    "fedcba98765432100123456789abcdef",
						SpanID:     "abcdef0123456789",
						TraceState: "dd=asdf256,ee=jkl;128",
						Attributes: map[string]interface{}{
							"a1": "v1",
							"a2": "v2",
						},
						Dropped: 24,
					},
					{
						TraceID:    "abcdef0123456789abcdef0123456789",
						SpanID:     "fedcba9876543210",
						TraceState: "",
						Attributes: map[string]interface{}{
							"a3": "v2",
							"a4": "v4",
						},
						Dropped: 0,
					},
					{
						TraceID:    "abcdef0123456789abcdef0123456789",
						SpanID:     "fedcba9876543210",
						TraceState: "",
						Attributes: map[string]interface{}{},
						Dropped:    2,
					},
					{
						TraceID:    "abcdef0123456789abcdef0123456789",
						SpanID:     "fedcba9876543210",
						TraceState: "",
						Attributes: map[string]interface{}{},
						Dropped:    0,
					},
				},
				StatusMsg:  "Error",
				StatusCode: ptrace.StatusCodeError,
			}),
			out: &pb.Span{
				Service:  "myservice",
				Name:     "ddtracer.server",
				Resource: "GET /path",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Error:    1,
				Meta: map[string]string{
					"name":                    "john",
					"env":                     "prod",
					"deployment.environment":  "prod",
					"otel.trace_id":           "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":        "Error",
					"otel.status_description": "Error",
					"otel.library.name":       "ddtracer",
					"otel.library.version":    "v2",
					"service.version":         "v1.2.3",
					"w3c.tracestate":          "state",
					"version":                 "v1.2.3",
					"events":                  "[{\"time_unix_nano\":123,\"name\":\"boom\",\"attributes\":{\"message\":\"Out of memory\",\"accuracy\":2.4},\"dropped_attributes_count\":2},{\"time_unix_nano\":456,\"name\":\"exception\",\"attributes\":{\"exception.message\":\"Out of memory\",\"exception.type\":\"mem\",\"exception.stacktrace\":\"1/2/3\"},\"dropped_attributes_count\":2}]",
					"_dd.span_links":          `[{"trace_id":"fedcba98765432100123456789abcdef","span_id":"abcdef0123456789","trace_state":"dd=asdf256,ee=jkl;128","attributes":{"a1":"v1","a2":"v2"},"dropped_attributes_count":24},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210","attributes":{"a3":"v2","a4":"v4"}},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210","dropped_attributes_count":2},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210"}]`,
					"error.msg":               "Out of memory",
					"error.type":              "mem",
					"error.stack":             "1/2/3",
					"http.method":             "GET",
					"http.route":              "/path",
					"peer.service":            "userbase",
					"span.kind":               "server",
				},
				Metrics: map[string]float64{
					"approx": 1.2,
					"count":  2,
				},
				Type: "web",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
				"approx":     1.2,
				"count":      2,
			},
		}, {
			rattr: map[string]string{
				"service.name":    "myservice",
				"service.version": "v1.2.3",
				"env":             "staging",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID:    otlpTestTraceID,
				SpanID:     otlpTestSpanID,
				TraceState: "state",
				Name:       "/path",
				Kind:       ptrace.SpanKindServer,
				Start:      now,
				End:        now + 200000000,
				Attributes: map[string]interface{}{
					"name":            "john",
					"http.method":     "GET",
					"http.route":      "/path",
					"approx":          1.2,
					"count":           2,
					"analytics.event": "false",
					"service.name":    "pylons",
				},
				Events: []testutil.OTLPSpanEvent{
					{
						Timestamp: 123,
						Name:      "boom",
						Attributes: map[string]interface{}{
							"message":  "Out of memory",
							"accuracy": 2.4,
						},
						Dropped: 2,
					},
					{
						Timestamp: 456,
						Name:      "exception",
						Attributes: map[string]interface{}{
							"exception.message":    "Out of memory",
							"exception.type":       "mem",
							"exception.stacktrace": "1/2/3",
						},
						Dropped: 2,
					},
				},
				Links: []testutil.OTLPSpanLink{
					{
						TraceID:    "fedcba98765432100123456789abcdef",
						SpanID:     "abcdef0123456789",
						TraceState: "dd=asdf256,ee=jkl;128",
						Attributes: map[string]interface{}{
							"a1": "v1",
							"a2": "v2",
						},
						Dropped: 24,
					},
					{
						TraceID:    "abcdef0123456789abcdef0123456789",
						SpanID:     "fedcba9876543210",
						TraceState: "",
						Attributes: map[string]interface{}{
							"a3": "v2",
							"a4": "v4",
						},
						Dropped: 0,
					},
					{
						TraceID:    "abcdef0123456789abcdef0123456789",
						SpanID:     "fedcba9876543210",
						TraceState: "",
						Attributes: map[string]interface{}{},
						Dropped:    2,
					},
					{
						TraceID:    "abcdef0123456789abcdef0123456789",
						SpanID:     "fedcba9876543210",
						TraceState: "",
						Attributes: map[string]interface{}{},
						Dropped:    0,
					},
				},
				StatusMsg:  "Error",
				StatusCode: ptrace.StatusCodeError,
			}),
			out: &pb.Span{
				Service:  "pylons",
				Name:     "ddtracer.server",
				Resource: "GET /path",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Error:    1,
				Meta: map[string]string{
					"name":                    "john",
					"env":                     "staging",
					"otel.status_code":        "Error",
					"otel.status_description": "Error",
					"otel.library.name":       "ddtracer",
					"otel.library.version":    "v2",
					"service.version":         "v1.2.3",
					"w3c.tracestate":          "state",
					"version":                 "v1.2.3",
					"otel.trace_id":           "72df520af2bde7a5240031ead750e5f3",
					"events":                  "[{\"time_unix_nano\":123,\"name\":\"boom\",\"attributes\":{\"message\":\"Out of memory\",\"accuracy\":2.4},\"dropped_attributes_count\":2},{\"time_unix_nano\":456,\"name\":\"exception\",\"attributes\":{\"exception.message\":\"Out of memory\",\"exception.type\":\"mem\",\"exception.stacktrace\":\"1/2/3\"},\"dropped_attributes_count\":2}]",
					"_dd.span_links":          `[{"trace_id":"fedcba98765432100123456789abcdef","span_id":"abcdef0123456789","trace_state":"dd=asdf256,ee=jkl;128","attributes":{"a1":"v1","a2":"v2"},"dropped_attributes_count":24},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210","attributes":{"a3":"v2","a4":"v4"}},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210","dropped_attributes_count":2},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210"}]`,
					"error.msg":               "Out of memory",
					"error.type":              "mem",
					"error.stack":             "1/2/3",
					"http.method":             "GET",
					"http.route":              "/path",
					"span.kind":               "server",
				},
				Metrics: map[string]float64{
					"approx":                               1.2,
					"count":                                2,
					sampler.KeySamplingRateEventExtraction: 0,
				},
				Type: "web",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level":                           1,
				"approx":                               1.2,
				"count":                                2,
				sampler.KeySamplingRateEventExtraction: 0,
			},
		}, {
			rattr: map[string]string{
				"env": "staging",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				Name:  "/path",
				Start: now,
				End:   now + 200000000,
				Attributes: map[string]interface{}{
					"service.name":                    "mongo",
					"operation.name":                  "READ",
					"resource.name":                   "/path",
					"span.type":                       "db",
					"name":                            "john",
					semconv.AttributeContainerID:      "cid",
					semconv.AttributeK8SContainerName: "k8s-container",
					"http.method":                     "GET",
					"http.route":                      "/path",
					"approx":                          1.2,
					"count":                           2,
					"analytics.event":                 true,
				},
			}),
			out: &pb.Span{
				Service:  "mongo",
				Name:     "READ",
				Resource: "/path",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"env":                             "staging",
					semconv.AttributeContainerID:      "cid",
					semconv.AttributeK8SContainerName: "k8s-container",
					"http.method":                     "GET",
					"http.route":                      "/path",
					"otel.status_code":                "Unset",
					"otel.library.name":               "ddtracer",
					"otel.library.version":            "v2",
					"name":                            "john",
					"otel.trace_id":                   "72df520af2bde7a5240031ead750e5f3",
					"span.kind":                       "unspecified",
				},
				Metrics: map[string]float64{
					"approx":                               1.2,
					"count":                                2,
					sampler.KeySamplingRateEventExtraction: 1,
				},
				Type: "db",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level":                           1,
				"approx":                               1.2,
				"count":                                2,
				sampler.KeySamplingRateEventExtraction: 1,
			},
		},
		{
			rattr: map[string]string{
				"env":          "staging",
				"service.name": "document-uploader",
			},
			libname: "ddtracer",
			libver:  "v2",
			// Modified version of:
			// https://opentelemetry.io/docs/specs/semconv/http/http-spans/#http-server-call-connection-dropped-before-response-body-was-sent
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				Name:       "POST /uploads/:document_id",
				Start:      now,
				End:        now + 200000000,
				StatusCode: ptrace.StatusCodeError,
				Attributes: map[string]interface{}{
					"operation.name":            "ddtracer.server",
					"http.request.method":       "POST",
					"url.path":                  "/uploads/4",
					"url.scheme":                "https",
					"http.route":                "/uploads/:document_id",
					"http.response.status_code": "201",
					"error.type":                "WebSocketDisconnect",
				},
			}),
			out: &pb.Span{
				Service:  "document-uploader",
				Name:     "ddtracer.server",
				Resource: "POST /uploads/:document_id",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Error:    1,
				Meta: map[string]string{
					"env":                       "staging",
					"otel.library.name":         "ddtracer",
					"otel.library.version":      "v2",
					"otel.status_code":          "Error",
					"error.msg":                 "201",
					"http.request.method":       "POST",
					"http.method":               "POST",
					"url.path":                  "/uploads/4",
					"url.scheme":                "https",
					"http.route":                "/uploads/:document_id",
					"http.response.status_code": "201",
					"http.status_code":          "201",
					"error.type":                "WebSocketDisconnect",
					"otel.trace_id":             "72df520af2bde7a5240031ead750e5f3",
					"span.kind":                 "unspecified",
				},
				Type: "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
		},
		{
			rattr: map[string]string{
				"env":          "staging",
				"service.name": "document-uploader",
			},
			libname: "ddtracer",
			libver:  "v2",
			// Modified version of:
			// https://opentelemetry.io/docs/specs/semconv/http/http-spans/#http-server-call-connection-dropped-before-response-body-was-sent
			// Using both old and new semantic conventions.
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				Name:       "POST /uploads/:document_id",
				Start:      now,
				End:        now + 200000000,
				StatusCode: ptrace.StatusCodeError,
				Attributes: map[string]interface{}{
					"operation.name":            "ddtracer.server",
					"http.request.method":       "POST",
					"http.method":               "SHOULD NOT BE USED FOR RESOURCE NAME",
					"url.path":                  "/uploads/4",
					"url.scheme":                "https",
					"http.route":                "/uploads/:document_id",
					"http.response.status_code": "201",
					"http.status_code":          "SHOULD NOT BE USED FOR ERROR.MSG",
					"error.type":                "WebSocketDisconnect",
				},
			}),
			out: &pb.Span{
				Service:  "document-uploader",
				Name:     "ddtracer.server",
				Resource: "POST /uploads/:document_id",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Error:    1,
				Meta: map[string]string{
					"env":                       "staging",
					"otel.library.name":         "ddtracer",
					"otel.library.version":      "v2",
					"otel.status_code":          "Error",
					"error.msg":                 "201",
					"http.request.method":       "POST",
					"http.method":               "POST",
					"url.path":                  "/uploads/4",
					"url.scheme":                "https",
					"http.route":                "/uploads/:document_id",
					"http.response.status_code": "201",
					"http.status_code":          "201",
					"error.type":                "WebSocketDisconnect",
					"otel.trace_id":             "72df520af2bde7a5240031ead750e5f3",
					"span.kind":                 "unspecified",
				},
				Type: "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
		},
		{
			rattr: map[string]string{
				"env":          "staging",
				"service.name": "document-uploader",
			},
			libname: "ddtracer",
			libver:  "v2",
			// Modified version of:
			// https://opentelemetry.io/docs/specs/semconv/http/http-spans/#http-server-call-connection-dropped-before-response-body-was-sent
			// Using old semantic conventions.
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				Name:       "POST /uploads/:document_id",
				Start:      now,
				End:        now + 200000000,
				StatusCode: ptrace.StatusCodeError,
				Attributes: map[string]interface{}{
					"operation.name":   "ddtracer.server",
					"http.method":      "POST",
					"url.path":         "/uploads/4",
					"url.scheme":       "https",
					"http.route":       "/uploads/:document_id",
					"http.status_code": "201",
					"error.type":       "WebSocketDisconnect",
				},
			}),
			out: &pb.Span{
				Service:  "document-uploader",
				Name:     "ddtracer.server",
				Resource: "POST /uploads/:document_id",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Error:    1,
				Meta: map[string]string{
					"env":                  "staging",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"otel.status_code":     "Error",
					"error.msg":            "201",
					"http.method":          "POST",
					"url.path":             "/uploads/4",
					"url.scheme":           "https",
					"http.route":           "/uploads/:document_id",
					"http.status_code":     "201",
					"error.type":           "WebSocketDisconnect",
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"span.kind":            "unspecified",
				},
				Type: "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			lib := pcommon.NewInstrumentationScope()
			lib.SetName(tt.libname)
			lib.SetVersion(tt.libver)
			assert := assert.New(t)
			want := tt.out
			res := pcommon.NewResource()
			for k, v := range tt.rattr {
				res.Attributes().PutStr(k, v)
			}
			var got *pb.Span
			if enableReceiveResourceSpansV2 {
				got = transform.OtelSpanToDDSpan(tt.in, res, lib, o.conf, nil)
			} else {
				got = o.convertSpan(tt.rattr, lib, tt.in)
			}
			if len(want.Meta) != len(got.Meta) {
				t.Fatalf("(%d) Meta count mismatch:\n%#v", i, got.Meta)
			}
			for k, v := range want.Meta {
				switch k {
				case "events":
					// events contain maps with no guaranteed order of
					// traversal; best to unpack to compare
					var gote, wante []testutil.OTLPSpanEvent
					if err := json.Unmarshal([]byte(v), &wante); err != nil {
						t.Fatalf("(%d) Error unmarshalling: %v", i, err)
					}
					if err := json.Unmarshal([]byte(got.Meta[k]), &gote); err != nil {
						t.Fatalf("(%d) Error unmarshalling: %v", i, err)
					}
					assert.Equal(wante, gote)
				case "_dd.span_links":
					// links contain maps with no guaranteed order of
					// traversal; best to unpack to compare
					var gotl, wantl []testutil.OTLPSpanLink
					if err := json.Unmarshal([]byte(v), &wantl); err != nil {
						t.Fatalf("(%d) Error unmarshalling: %v", i, err)
					}
					if err := json.Unmarshal([]byte(got.Meta[k]), &gotl); err != nil {
						t.Fatalf("(%d) Error unmarshalling: %v", i, err)
					}
					assert.Equal(wantl, gotl)
				default:
					assert.Equal(v, got.Meta[k], fmt.Sprintf("(%d) Meta %v:%v", i, k, v))
				}
			}
			if len(want.Metrics) != len(got.Metrics) {
				t.Fatalf("(%d) Metrics count mismatch:\n\n%v\n\n%v", i, want.Metrics, got.Metrics)
			}
			for k, v := range want.Metrics {
				assert.Equal(v, got.Metrics[k], fmt.Sprintf("(%d) Metric %v:%v", i, k, v))
			}
			want.Meta = nil
			want.Metrics = nil
			got.Meta = nil
			got.Metrics = nil
			assert.Equal(want, got, i)

			// test new top-level identification feature flag
			o.conf.Features["enable_otlp_compute_top_level_by_span_kind"] = struct{}{}
			if enableReceiveResourceSpansV2 {
				got = transform.OtelSpanToDDSpan(tt.in, res, lib, o.conf, nil)
			} else {
				got = o.convertSpan(tt.rattr, lib, tt.in)
			}
			wantMetrics := tt.topLevelOutMetrics
			if len(wantMetrics) != len(got.Metrics) {
				t.Fatalf("(%d) Metrics count mismatch:\n\n%v\n\n%v", i, wantMetrics, got.Metrics)
			}
			for k, v := range wantMetrics {
				assert.Equal(v, got.Metrics[k], fmt.Sprintf("(%d) Metric %v:%v", i, k, v))
			}
			delete(o.conf.Features, "enable_otlp_compute_top_level_by_span_kind")
		})
	}
}

func TestFlatten(t *testing.T) {
	for _, tt := range []map[string]string{
		{"a": "b", "c": "d"},
		{"x": "y"},
	} {
		assert.Equal(t, unflatten(flatten(tt).String()), tt)
	}
	assert.Equal(t, flatten(map[string]string{}).String(), "")
	assert.Equal(t, flatten(nil).String(), "")
}

func TestAppendTags(t *testing.T) {
	var str strings.Builder
	appendTags(&str, "a:b,c:d")
	assert.Equal(t, str.String(), "a:b,c:d")
	appendTags(&str, "e:f,g:h")
	assert.Equal(t, str.String(), "a:b,c:d,e:f,g:h")
	appendTags(&str, "i:j")
	assert.Equal(t, str.String(), "a:b,c:d,e:f,g:h,i:j")
}

func TestOTLPConvertSpanSetPeerService(t *testing.T) {
	t.Run("ReceiveResourceSpansV1", func(t *testing.T) {
		testOTLPConvertSpanSetPeerService(false, t)
	})

	t.Run("ReceiveResourceSpansV2", func(t *testing.T) {
		testOTLPConvertSpanSetPeerService(true, t)
	})
}
func testOTLPConvertSpanSetPeerService(enableReceiveResourceSpansV2 bool, t *testing.T) {
	now := uint64(otlpTestSpan.StartTimestamp())
	cfg := NewTestConfig(t)
	if enableReceiveResourceSpansV2 {
		cfg.Features["receive_resource_spans_v2"] = struct{}{}
	}
	o := NewOTLPReceiver(nil, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
	for i, tt := range []struct {
		rattr   map[string]string
		libname string
		libver  string
		in      ptrace.Span
		out     *pb.Span
	}{
		{
			rattr: map[string]string{
				"service.version": "v1.2.3",
				"service.name":    "myservice",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "/path",
				Kind:    ptrace.SpanKindServer,
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					"peer.service":           "userbase",
					"deployment.environment": "prod",
				},
			}),
			out: &pb.Span{
				Service:  "myservice",
				Name:     "ddtracer.server",
				Resource: "/path",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"env":                    "prod",
					"deployment.environment": "prod",
					"otel.trace_id":          "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":       "Unset",
					"otel.library.name":      "ddtracer",
					"otel.library.version":   "v2",
					"service.version":        "v1.2.3",
					"version":                "v1.2.3",
					"peer.service":           "userbase",
					"span.kind":              "server",
				},
				Type:    "web",
				Metrics: map[string]float64{},
			},
		},
		{
			rattr: map[string]string{
				"service.version": "v1.2.3",
				"service.name":    "myservice",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "/path",
				Kind:    ptrace.SpanKindServer,
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					"db.instance":            "postgres",
					"peer.service":           "userbase",
					"deployment.environment": "prod",
				},
			}),
			out: &pb.Span{
				Service:  "myservice",
				Name:     "ddtracer.server",
				Resource: "/path",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"db.instance":            "postgres",
					"env":                    "prod",
					"deployment.environment": "prod",
					"otel.trace_id":          "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":       "Unset",
					"otel.library.name":      "ddtracer",
					"otel.library.version":   "v2",
					"service.version":        "v1.2.3",
					"version":                "v1.2.3",
					"peer.service":           "userbase",
					"span.kind":              "server",
				},
				Type:    "web",
				Metrics: map[string]float64{},
			},
		},
		{
			rattr: map[string]string{
				"service.version": "v1.2.3",
				"service.name":    "myservice",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "/path",
				Kind:    ptrace.SpanKindClient,
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					"db.system":              "postgres",
					"net.peer.name":          "remotehost",
					"deployment.environment": "prod",
				},
			}),
			out: &pb.Span{
				Service:  "myservice",
				Name:     "ddtracer.client",
				Resource: "/path",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"env":                    "prod",
					"deployment.environment": "prod",
					"otel.trace_id":          "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":       "Unset",
					"otel.library.name":      "ddtracer",
					"otel.library.version":   "v2",
					"service.version":        "v1.2.3",
					"version":                "v1.2.3",
					"db.system":              "postgres",
					"net.peer.name":          "remotehost",
					"span.kind":              "client",
				},
				Type:    "db",
				Metrics: map[string]float64{},
			},
		},
		{
			rattr: map[string]string{
				"service.version": "v1.2.3",
				"service.name":    "myservice",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "/path",
				Kind:    ptrace.SpanKindClient,
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					"rpc.service":            "GetInstance",
					"net.peer.name":          "remotehost",
					"deployment.environment": "prod",
				},
			}),
			out: &pb.Span{
				Service:  "myservice",
				Name:     "ddtracer.client",
				Resource: "/path",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"env":                    "prod",
					"deployment.environment": "prod",
					"otel.trace_id":          "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":       "Unset",
					"otel.library.name":      "ddtracer",
					"otel.library.version":   "v2",
					"service.version":        "v1.2.3",
					"version":                "v1.2.3",
					"rpc.service":            "GetInstance",
					"net.peer.name":          "remotehost",
					"span.kind":              "client",
				},
				Type:    "http",
				Metrics: map[string]float64{},
			},
		},
		{
			rattr: map[string]string{
				"service.version": "v1.2.3",
				"service.name":    "myservice",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "/path",
				Kind:    ptrace.SpanKindServer,
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					"net.peer.name":          "remotehost",
					"deployment.environment": "prod",
				},
			}),
			out: &pb.Span{
				Service:  "myservice",
				Name:     "ddtracer.server",
				Resource: "/path",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"env":                    "prod",
					"deployment.environment": "prod",
					"otel.trace_id":          "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":       "Unset",
					"otel.library.name":      "ddtracer",
					"otel.library.version":   "v2",
					"service.version":        "v1.2.3",
					"version":                "v1.2.3",
					"net.peer.name":          "remotehost",
					"span.kind":              "server",
				},
				Type:    "web",
				Metrics: map[string]float64{},
			},
		},
		{
			rattr: map[string]string{
				"service.version": "v1.2.3",
				"service.name":    "myservice",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "/path",
				Kind:    ptrace.SpanKindServer,
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					"aws.dynamodb.table_names": "my-table",
					"deployment.environment":   "prod",
				},
			}),
			out: &pb.Span{
				Service:  "myservice",
				Name:     "ddtracer.server",
				Resource: "/path",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"env":                      "prod",
					"deployment.environment":   "prod",
					"otel.trace_id":            "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":         "Unset",
					"otel.library.name":        "ddtracer",
					"otel.library.version":     "v2",
					"service.version":          "v1.2.3",
					"version":                  "v1.2.3",
					"aws.dynamodb.table_names": "my-table",
					"span.kind":                "server",
				},
				Type:    "web",
				Metrics: map[string]float64{},
			},
		},
		{
			rattr: map[string]string{
				"service.version": "v1.2.3",
				"service.name":    "myservice",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "/path",
				Kind:    ptrace.SpanKindServer,
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					"faas.document.collection": "my-s3-bucket",
					"deployment.environment":   "prod",
				},
			}),
			out: &pb.Span{
				Service:  "myservice",
				Name:     "ddtracer.server",
				Resource: "/path",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"env":                      "prod",
					"deployment.environment":   "prod",
					"otel.trace_id":            "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":         "Unset",
					"otel.library.name":        "ddtracer",
					"otel.library.version":     "v2",
					"service.version":          "v1.2.3",
					"version":                  "v1.2.3",
					"faas.document.collection": "my-s3-bucket",
					"span.kind":                "server",
				},
				Type:    "web",
				Metrics: map[string]float64{},
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			lib := pcommon.NewInstrumentationScope()
			lib.SetName(tt.libname)
			lib.SetVersion(tt.libver)
			assert := assert.New(t)
			res := pcommon.NewResource()
			for k, v := range tt.rattr {
				res.Attributes().PutStr(k, v)
			}
			var got *pb.Span
			if enableReceiveResourceSpansV2 {
				got = transform.OtelSpanToDDSpan(tt.in, res, lib, o.conf, nil)
			} else {
				got = o.convertSpan(tt.rattr, lib, tt.in)
			}
			assert.Equal(tt.out, got, i)
		})
	}
}

// TestResourceAttributesMap is a regression test ensuring that the resource attributes map
// passed to convertSpan is not modified by it.
func TestResourceAttributesMap(t *testing.T) {
	t.Run("ReceiveResourceSpansV1", func(t *testing.T) {
		testResourceAttributesMap(false, t)
	})

	t.Run("ReceiveResourceSpansV2", func(t *testing.T) {
		testResourceAttributesMap(true, t)
	})
}

func testResourceAttributesMap(enableReceiveResourceSpansV2 bool, t *testing.T) {
	rattr := map[string]string{"key": "val"}
	lib := pcommon.NewInstrumentationScope()
	span := testutil.NewOTLPSpan(&testutil.OTLPSpan{})
	cfg := NewTestConfig(t)
	if enableReceiveResourceSpansV2 {
		cfg.Features["enable_receive_resource_spans_v2"] = struct{}{}
	}
	NewOTLPReceiver(nil, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{}).convertSpan(rattr, lib, span)
	assert.Len(t, rattr, 1) // ensure "rattr" has no new entries
	assert.Equal(t, "val", rattr["key"])
}

func makeEventsSlice(name string, attrs map[string]any, timestamp int, dropped uint32) ptrace.SpanEventSlice {
	s := ptrace.NewSpanEventSlice()
	e := s.AppendEmpty()
	e.SetName(name)
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, ok := e.Attributes().Get(k)
		if !ok {
			switch attrs[k].(type) {
			case []any:
				s := e.Attributes().PutEmptySlice(k)
				if v, ok := attrs[k].([]any); ok {
					s.FromRaw(v)
				}
			default:
				if v, ok := attrs[k].(string); ok {
					e.Attributes().PutStr(k, v)
				}
			}
		}
	}
	e.SetTimestamp(pcommon.Timestamp(timestamp))
	e.SetDroppedAttributesCount(dropped)
	return s
}

func TestMarshalEvents(t *testing.T) {
	for _, tt := range []struct {
		in  ptrace.SpanEventSlice
		out string
	}{
		{
			in: makeEventsSlice("", map[string]any{
				"message": "OOM",
			}, 0, 3),
			out: `[{
					"attributes": {"message":"OOM"},
					"dropped_attributes_count":3
				}]`,
		}, {
			in:  makeEventsSlice("boom", nil, 0, 0),
			out: `[{"name":"boom"}]`,
		}, {
			in: makeEventsSlice("boom", map[string]any{
				"message": "OOM",
			}, 0, 3),
			out: `[{
					"name":"boom",
					"attributes": {"message":"OOM"},
					"dropped_attributes_count":3
				}]`,
		}, {
			in: makeEventsSlice("boom", map[string]any{
				"message": "OOM",
			}, 123, 2),
			out: `[{
					"time_unix_nano":123,
					"name":"boom",
					"attributes": { "message":"OOM" },
					"dropped_attributes_count":2
				}]`,
		}, {
			in:  makeEventsSlice("", nil, 0, 2),
			out: `[{"dropped_attributes_count":2}]`,
		}, {
			in: makeEventsSlice("", map[string]any{
				"message":  "OOM",
				"accuracy": "2.40",
			}, 123, 2),
			out: `[{
					"time_unix_nano":123,
					"attributes": {
						"accuracy":"2.40",
						"message":"OOM"
					},
					"dropped_attributes_count":2
				}]`,
		}, {
			in: makeEventsSlice("boom", map[string]any{
				"message":  "OOM",
				"accuracy": "2.40",
			}, 123, 0),
			out: `[{
					"time_unix_nano":123,
					"name":"boom",
					"attributes": {
						"accuracy":"2.40",
						"message":"OOM"
					}
				}]`,
		}, {
			in: makeEventsSlice("boom", nil, 123, 2),
			out: `[{
					"time_unix_nano":123,
					"name":"boom",
					"dropped_attributes_count":2
				}]`,
		}, {
			in: makeEventsSlice("boom", map[string]any{
				"message":  "OOM",
				"accuracy": "2.4",
			}, 123, 2),
			out: `[{
					"time_unix_nano":123,
					"name":"boom",
					"attributes": {
						"accuracy":"2.4",
						"message":"OOM"
					},
					"dropped_attributes_count":2
				}]`,
		}, {
			in: (func() ptrace.SpanEventSlice {
				e1 := makeEventsSlice("boom", map[string]any{
					"message":  "OOM",
					"accuracy": "2.4",
				}, 123, 2)
				e2 := makeEventsSlice("exception", map[string]any{
					"exception.message":    "OOM",
					"exception.stacktrace": "1/2/3",
					"exception.type":       "mem",
				}, 456, 2)
				e2.MoveAndAppendTo(e1)
				return e1
			})(),
			out: `[{
					"time_unix_nano":123,
					"name":"boom",
					"attributes": {
						"accuracy":"2.4",
						"message":"OOM"
					},
					"dropped_attributes_count":2
				}, {
					"time_unix_nano":456,
					"name":"exception",
					"attributes": {
						"exception.message":"OOM",
						"exception.stacktrace":"1/2/3",
						"exception.type":"mem"
					},
					"dropped_attributes_count":2
				}]`,
		},
	} {
		assert.Equal(t, trimSpaces(tt.out), transform.MarshalEvents(tt.in))
	}
}

func TestMarshalJSONUnsafeEvents(t *testing.T) {
	name := `something:"nested"`
	key := `abc\def\`
	val := []any{`test\"1\`, `/test2\\`}

	events := makeEventsSlice(name, map[string]any{
		key: val,
	}, 0, 3)

	jsonName, err := json.Marshal(name)
	if err != nil {
		t.Fatal("Failure parsing name")
	}
	jsonKey, err := json.Marshal(key)
	if err != nil {
		t.Fatal("Failure parsing key")
	}
	jsonVal, err := json.Marshal(val)
	if err != nil {
		t.Fatal("Failure parsing val")
	}

	out := fmt.Sprintf(`[{
				"name": %v,
				"attributes": {%v: %v},
				"dropped_attributes_count":3
			}]`, string(jsonName), string(jsonKey), string(jsonVal))

	assert.Equal(t, trimSpaces(out), transform.MarshalEvents(events))
}

func trimSpaces(str string) string {
	var out strings.Builder
	for _, ch := range str {
		if !unicode.IsSpace(ch) {
			out.WriteRune(ch)
		}
	}
	return out.String()
}

//nolint:revive // TODO(OTEL) Fix revive linter
func makeSpanLinkSlice(t *testing.T, traceId, spanId, traceState string, attrs map[string]string, dropped uint32) ptrace.SpanLinkSlice {
	s := ptrace.NewSpanLinkSlice()
	l := s.AppendEmpty()
	buf, err := hex.DecodeString(traceId)
	if err != nil {
		t.Fatal(err)
	}
	l.SetTraceID(*(*pcommon.TraceID)(buf))
	buf, err = hex.DecodeString(spanId)
	if err != nil {
		t.Fatal(err)
	}
	l.SetSpanID(*(*pcommon.SpanID)(buf))
	l.TraceState().FromRaw(traceState)
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, ok := l.Attributes().Get(k)
		if !ok {
			l.Attributes().PutStr(k, attrs[k])
		}
	}
	l.SetDroppedAttributesCount(dropped)
	return s
}

func TestMakeSpanLinkSlice(t *testing.T) {
	in := makeSpanLinkSlice(t, "fedcba98765432100123456789abcdef", "abcdef1234567890", "dd=asdf256", map[string]string{"k1": "v1", "k2": "v2"}, 42)

	out := ptrace.NewSpanLinkSlice()
	l := out.AppendEmpty()
	bh := make([]byte, 8)
	bl := make([]byte, 8)
	binary.BigEndian.PutUint64(bh, 0xfedcba9876543210)
	binary.BigEndian.PutUint64(bl, 0x0123456789abcdef)
	l.SetTraceID(*(*pcommon.TraceID)(append(bh, bl...)))
	binary.BigEndian.PutUint64(bl, 0xabcdef1234567890)
	l.SetSpanID(*(*pcommon.SpanID)(bl))
	l.TraceState().FromRaw("dd=asdf256")
	l.Attributes().PutStr("k1", "v1")
	l.Attributes().PutStr("k2", "v2")
	l.SetDroppedAttributesCount(42)

	assert.Equal(t, out, in)
}

func TestMarshalSpanLinks(t *testing.T) {
	for _, tt := range []struct {
		in  ptrace.SpanLinkSlice
		out string
	}{

		{
			in: makeSpanLinkSlice(t, "fedcba98765432100123456789abcdef", "abcdef0123456789", "", map[string]string{}, 0),
			out: `[{
					"trace_id": "fedcba98765432100123456789abcdef",
					"span_id":  "abcdef0123456789"
				}]`,
		}, {
			in: makeSpanLinkSlice(t, "fedcba98765432100123456789abcdef", "abcdef0123456789", "dd=asdf256", map[string]string{}, 0),
			out: `[{
					"trace_id":    "fedcba98765432100123456789abcdef",
					"span_id":     "abcdef0123456789",
					"trace_state": "dd=asdf256"
				}]`,
		}, {
			in: makeSpanLinkSlice(t, "fedcba98765432100123456789abcdef", "abcdef0123456789", "dd=asdf256", map[string]string{"k1": "v1"}, 0),
			out: `[{
					"trace_id":    "fedcba98765432100123456789abcdef",
					"span_id":     "abcdef0123456789",
					"trace_state": "dd=asdf256",
					"attributes":  {"k1": "v1"}
				}]`,
		}, {
			in: makeSpanLinkSlice(t, "fedcba98765432100123456789abcdef", "abcdef0123456789", "dd=asdf256", map[string]string{}, 42),
			out: `[{
					"trace_id":                 "fedcba98765432100123456789abcdef",
					"span_id":                  "abcdef0123456789",
					"trace_state":              "dd=asdf256",
					"dropped_attributes_count": 42
				}]`,
		}, {
			in: makeSpanLinkSlice(t, "fedcba98765432100123456789abcdef", "abcdef0123456789", "dd=asdf256", map[string]string{"k1": "v1"}, 42),
			out: `[{
					"trace_id":                 "fedcba98765432100123456789abcdef",
					"span_id":                  "abcdef0123456789",
					"trace_state":              "dd=asdf256",
					"attributes":               {"k1": "v1"},
					"dropped_attributes_count": 42
				}]`,
		}, {
			in: makeSpanLinkSlice(t, "fedcba98765432100123456789abcdef", "abcdef0123456789", "", map[string]string{"k1": "v1"}, 0),
			out: `[{
					"trace_id":   "fedcba98765432100123456789abcdef",
					"span_id":    "abcdef0123456789",
					"attributes": {"k1": "v1"}
				}]`,
		}, {
			in: makeSpanLinkSlice(t, "fedcba98765432100123456789abcdef", "abcdef0123456789", "", map[string]string{"k1": "v1"}, 42),
			out: `[{
					"trace_id":                 "fedcba98765432100123456789abcdef",
					"span_id":                  "abcdef0123456789",
					"attributes":               {"k1": "v1"},
					"dropped_attributes_count": 42
				}]`,
		}, {
			in: makeSpanLinkSlice(t, "fedcba98765432100123456789abcdef", "abcdef0123456789", "", map[string]string{}, 42),
			out: `[{
					"trace_id":                 "fedcba98765432100123456789abcdef",
					"span_id":                  "abcdef0123456789",
					"dropped_attributes_count": 42
				}]`,
		}, {
			in: makeSpanLinkSlice(t, "fedcba98765432100123456789abcdef", "abcdef0123456789", "dd=asdf256,ee=jkl;128", map[string]string{
				"k1": "v1",
				"k2": "v2",
			}, 57),
			out: `[{
					"trace_id":                 "fedcba98765432100123456789abcdef",
					"span_id":                  "abcdef0123456789",
					"trace_state":              "dd=asdf256,ee=jkl;128",
					"attributes":               {"k1": "v1", "k2": "v2"},
					"dropped_attributes_count": 57
				}]`,
		}, {

			in: (func() ptrace.SpanLinkSlice {
				s1 := makeSpanLinkSlice(t, "fedcba98765432100123456789abcdef", "0123456789abcdef", "dd=asdf256,ee=jkl;128", map[string]string{"k1": "v1"}, 611187)
				s2 := makeSpanLinkSlice(t, "abcdef01234567899876543210fedcba", "fedcba9876543210", "", map[string]string{"k1": "v10", "k2": "v20"}, 0)
				s2.MoveAndAppendTo(s1)
				return s1
			})(),
			out: `[{
					"trace_id":                 "fedcba98765432100123456789abcdef",
					"span_id":                  "0123456789abcdef",
					"trace_state":              "dd=asdf256,ee=jkl;128",
					"attributes":               {"k1": "v1"},
					"dropped_attributes_count": 611187
			       }, {
					"trace_id":                 "abcdef01234567899876543210fedcba",
					"span_id":                  "fedcba9876543210",
					"attributes":               {"k1": "v10", "k2": "v20"}
			       }]`,
		},
	} {
		assert.Equal(t, trimSpaces(tt.out), transform.MarshalLinks(tt.in))
	}
}

func TestComputeTopLevelAndMeasured(t *testing.T) {
	testCases := []struct {
		span        *pb.Span
		spanKind    ptrace.SpanKind
		hasTopLevel bool
		hasMeasured bool
	}{
		{
			&pb.Span{TraceID: 1, SpanID: 1, ParentID: 0, Service: "mcnulty", Type: "web"},
			ptrace.SpanKindServer,
			true,
			false,
		},
		{
			&pb.Span{TraceID: 1, SpanID: 2, ParentID: 1, Service: "mcnulty", Type: "sql"},
			ptrace.SpanKindClient,
			false,
			true,
		},
		{
			&pb.Span{TraceID: 1, SpanID: 3, ParentID: 2, Service: "master-db", Type: "sql"},
			ptrace.SpanKindConsumer,
			true,
			false,
		},
		{
			&pb.Span{TraceID: 1, SpanID: 4, ParentID: 1, Service: "redis", Type: "redis"},
			ptrace.SpanKindProducer,
			false,
			true,
		},
		{
			&pb.Span{TraceID: 1, SpanID: 1, ParentID: 0, Service: "mcnulty", Type: ""},
			ptrace.SpanKindClient,
			true,
			true,
		},
		{
			&pb.Span{TraceID: 1, SpanID: 1, ParentID: 0, Service: "mcnulty", Type: ""},
			ptrace.SpanKindInternal,
			true,
			false,
		},
		{
			&pb.Span{TraceID: 1, SpanID: 4, ParentID: 1, Service: "mcnulty", Type: ""},
			ptrace.SpanKindInternal,
			false,
			false,
		},
	}

	assert := assert.New(t)
	for _, tc := range testCases {
		computeTopLevelAndMeasured(tc.span, tc.spanKind)
		assert.Equal(tc.hasTopLevel, traceutil.HasTopLevel(tc.span))
		assert.Equal(tc.hasMeasured, traceutil.IsMeasured(tc.span))
	}
}

func generateTraceRequest(traceCount int, spanCount int, attrCount int, attrLength int) ptraceotlp.ExportRequest {
	traces := make([]testutil.OTLPResourceSpan, traceCount)
	for k := 0; k < traceCount; k++ {
		spans := make([]*testutil.OTLPSpan, spanCount)
		for i := 0; i < spanCount; i++ {
			attributes := make(map[string]interface{})
			for j := 0; j < attrCount; j++ {
				attributes["key_"+strconv.Itoa(j)] = strings.Repeat("x", attrLength)
			}

			spans[i] = &testutil.OTLPSpan{
				Name:       "/path",
				TraceState: "state",
				Kind:       ptrace.SpanKindServer,
				Attributes: attributes,
				StatusCode: ptrace.StatusCodeOk,
			}
		}
		rattributes := make(map[string]interface{})
		for j := 0; j < attrCount; j++ {
			rattributes["key_"+strconv.Itoa(j)] = strings.Repeat("x", attrLength)
		}
		rattributes["service.name"] = "test-service"
		rattributes["deployment.environment"] = "test-env"
		rspans := testutil.OTLPResourceSpan{
			Spans:      spans,
			LibName:    "stats-agent-test",
			LibVersion: "0.0.1",
			Attributes: rattributes,
		}
		traces[k] = rspans
	}
	return testutil.NewOTLPTracesRequest(traces)
}

func BenchmarkProcessRequestV1(b *testing.B) {
	benchmarkProcessRequest(false, b)
}

func BenchmarkProcessRequestV2(b *testing.B) {
	benchmarkProcessRequest(true, b)
}

func benchmarkProcessRequest(enableReceiveResourceSpansV2 bool, b *testing.B) {
	largeTraces := generateTraceRequest(10, 100, 100, 100)
	metadata := http.Header(map[string][]string{
		header.Lang:        {"go"},
		header.ContainerID: {"containerdID"},
	})
	out := make(chan *Payload, 100)
	end := make(chan struct{})
	go func() {
		defer close(end)
		for {
			select {
			case <-out:
				// drain
			case <-end:
				return
			}
		}
	}()

	cfg := NewBenchmarkTestConfig(b)
	if enableReceiveResourceSpansV2 {
		cfg.Features["enable_receive_resource_spans_v2"] = struct{}{}
	}
	r := NewOTLPReceiver(out, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.processRequest(context.Background(), metadata, largeTraces)
	}
	b.StopTimer()
	end <- struct{}{}
	<-end
}

func BenchmarkProcessRequestTopLevelV1(b *testing.B) {
	benchmarkProcessRequestTopLevel(false, b)
}

func BenchmarkProcessRequestTopLevelV2(b *testing.B) {
	benchmarkProcessRequestTopLevel(true, b)
}

func benchmarkProcessRequestTopLevel(enableReceiveResourceSpansV2 bool, b *testing.B) {
	largeTraces := generateTraceRequest(10, 100, 100, 100)
	metadata := http.Header(map[string][]string{
		header.Lang:        {"go"},
		header.ContainerID: {"containerdID"},
	})
	out := make(chan *Payload, 100)
	end := make(chan struct{})
	go func() {
		defer close(end)
		for {
			select {
			case <-out:
				// drain
			case <-end:
				return
			}
		}
	}()

	cfg := NewBenchmarkTestConfig(b)
	if enableReceiveResourceSpansV2 {
		cfg.Features["enable_receive_resource_spans_v2"] = struct{}{}
	}
	cfg.Features["enable_otlp_compute_top_level_by_span_kind"] = struct{}{}
	r := NewOTLPReceiver(out, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.processRequest(context.Background(), metadata, largeTraces)
	}
	b.StopTimer()
	end <- struct{}{}
	<-end
}
