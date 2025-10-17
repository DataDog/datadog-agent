// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file contains V2 versions of the OTLP tests.
// These tests verify the new translation logic when the disable_otlp_translations_v2 feature flag is NOT set.
// The original tests in otlp_test.go verify backward compatibility with the old translation logic.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/transform"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv127 "go.opentelemetry.io/otel/semconv/v1.27.0"
	semconv "go.opentelemetry.io/otel/semconv/v1.6.1"
)

func TestOTLPMetrics_OTelCompliantTranslation(t *testing.T) {
	t.Helper()
	assert := assert.New(t)
	cfg := NewTestConfig(t)
	cfg.AgentVersion = "v1.0.0"
	cfg.Hostname = "test-host"
	cfg.Features["enable_otel_compliant_translation"] = struct{}{}
	stats := &teststatsd.Client{}

	out := make(chan *Payload, 1)
	rcv := NewOTLPReceiver(out, cfg, stats, &timing.NoopReporter{})
	req := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
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
	})

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

	rcv.Export(context.Background(), req)

	calls := stats.CountCalls
	assert.Equal(5, len(calls))
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.spans", Value: 3, Tags: []string{"tracer_version:otlp-", "endpoint_version:opentelemetry_grpc_v1"}, Rate: 1})
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.spans", Value: 2, Tags: []string{"tracer_version:otlp-", "endpoint_version:opentelemetry_grpc_v1"}, Rate: 1})
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.traces", Value: 1, Tags: []string{"tracer_version:otlp-", "endpoint_version:opentelemetry_grpc_v1"}, Rate: 1})
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.traces", Value: 2, Tags: []string{"tracer_version:otlp-", "endpoint_version:opentelemetry_grpc_v1"}, Rate: 1})
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.payload", Value: 1, Tags: []string{"endpoint_version:opentelemetry_grpc_v1"}, Rate: 1})
}

func TestOTLPMetricsEmitMisconfiguredSignalsCount_OTelCompliantTranslation(t *testing.T) {
	t.Helper()
	assert := assert.New(t)
	cfg := NewTestConfig(t)
	cfg.AgentVersion = "v1.0.0"
	cfg.Hostname = "test-host"
	cfg.Features["enable_otel_compliant_translation"] = struct{}{}
	stats := &teststatsd.Client{}

	out := make(chan *Payload, 1)
	rcv := NewOTLPReceiver(out, cfg, stats, &timing.NoopReporter{})
	req := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
		{
			LibName:    "libname",
			LibVersion: "1.2",
			Attributes: map[string]interface{}{"operation.name": "wrong"},
			Spans: []*testutil.OTLPSpan{
				{Name: "1", Attributes: map[string]interface{}{"service.name": "wrong"}},
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
	})

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

	rcv.Export(context.Background(), req)

	calls := stats.CountCalls
	assert.Equal(6, len(calls))
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.spans", Value: 3, Tags: []string{"tracer_version:otlp-", "endpoint_version:opentelemetry_grpc_v1"}, Rate: 1})
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.spans", Value: 2, Tags: []string{"tracer_version:otlp-", "endpoint_version:opentelemetry_grpc_v1"}, Rate: 1})
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.traces", Value: 1, Tags: []string{"tracer_version:otlp-", "endpoint_version:opentelemetry_grpc_v1"}, Rate: 1})
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.traces", Value: 2, Tags: []string{"tracer_version:otlp-", "endpoint_version:opentelemetry_grpc_v1"}, Rate: 1})
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.payload", Value: 1, Tags: []string{"endpoint_version:opentelemetry_grpc_v1"}, Rate: 1})
	assert.Contains(calls, teststatsd.MetricsArgs{Name: "datadog.trace_agent.otlp.misconfigured_attributes", Value: 4, Tags: []string{"tracer_version:otlp-", "endpoint_version:opentelemetry_grpc_v1", "host:test-host"}, Rate: 1})
}

func TestOTLPSpanNameV2_OTelCompliantTranslation(t *testing.T) {
	cfg := NewTestConfig(t)
	cfg.Features["enable_otel_compliant_translation"] = struct{}{}
	out := make(chan *Payload, 1)
	rcv := NewOTLPReceiver(out, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
	require := require.New(t)
	for _, tt := range []struct {
		in []testutil.OTLPResourceSpan
		fn func(*pb.TracerPayload)
	}{
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{{
						Attributes: map[string]interface{}{string(semconv.ContainerIDKey): "http.method"},
					}},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("internal", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Kind:       ptrace.SpanKindServer,
							Attributes: map[string]interface{}{string(semconv.HTTPMethodKey): "GET"},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("http.server.request", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Kind:       ptrace.SpanKindClient,
							Attributes: map[string]interface{}{string(semconv.HTTPMethodKey): "GET"},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("http.client.request", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Kind:       ptrace.SpanKindClient,
							Attributes: map[string]interface{}{string(semconv.DBSystemKey): "mysql"},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("mysql.query", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Attributes: map[string]interface{}{string(semconv.DBSystemKey): "mysql"},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("internal", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Attributes: map[string]interface{}{string(semconv.MessagingSystemKey): "kafka"},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("internal", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Attributes: map[string]interface{}{
								string(semconv.MessagingSystemKey):    "kafka",
								string(semconv.MessagingOperationKey): "send",
							},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("internal", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Kind: ptrace.SpanKindClient,
							Attributes: map[string]interface{}{
								string(semconv.MessagingSystemKey):    "kafka",
								string(semconv.MessagingOperationKey): "send",
							},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("kafka.send", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Kind: ptrace.SpanKindServer,
							Attributes: map[string]interface{}{
								string(semconv.RPCSystemKey): "aws-api",
							},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("aws-api.server.request", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Kind: ptrace.SpanKindClient,
							Attributes: map[string]interface{}{
								string(semconv.RPCSystemKey): "aws-api",
							},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("aws.client.request", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Kind: ptrace.SpanKindClient,
							Attributes: map[string]interface{}{
								string(semconv.RPCSystemKey): "aws-api",
							},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("aws.client.request", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Kind: ptrace.SpanKindClient,
							Attributes: map[string]interface{}{
								string(semconv.RPCSystemKey):  "aws-api",
								string(semconv.RPCServiceKey): "s3",
							},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("aws.s3.request", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Kind: ptrace.SpanKindClient,
							Attributes: map[string]interface{}{
								string(semconv.RPCSystemKey): "grpc",
							},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("grpc.client.request", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Kind: ptrace.SpanKindServer,
							Attributes: map[string]interface{}{
								string(semconv.RPCSystemKey): "grpc",
							},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("grpc.server.request", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Kind: ptrace.SpanKindClient,
							Attributes: map[string]interface{}{
								string(semconv.FaaSInvokedProviderKey): "gcp",
								string(semconv.FaaSInvokedNameKey):     "foo",
							},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("gcp.foo.invoke", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Attributes: map[string]interface{}{
								string(semconv.FaaSInvokedProviderKey): "gcp",
								string(semconv.FaaSInvokedNameKey):     "foo",
							},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("internal", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Kind: ptrace.SpanKindServer,
							Attributes: map[string]interface{}{
								string(semconv.FaaSTriggerKey): "timer",
							},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("timer.invoke", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Attributes: map[string]interface{}{
								string(semconv.FaaSTriggerKey): "timer",
							},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("internal", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Attributes: map[string]interface{}{
								"graphql.operation.type": "query",
							},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("graphql.server.request", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Kind: ptrace.SpanKindServer,
							Attributes: map[string]interface{}{
								"network.protocol.name": "tcp",
							},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("tcp.server.request", out.Chunks[0].Spans[0].Name)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{
							Kind:       ptrace.SpanKindServer,
							Attributes: map[string]interface{}{},
						},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("server.request", out.Chunks[0].Spans[0].Name)
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			rspans := testutil.NewOTLPTracesRequest(tt.in).Traces().ResourceSpans().At(0)
			rcv.ReceiveResourceSpans(context.Background(), rspans, http.Header{}, nil)
			timeout := time.After(500 * time.Millisecond)
			select {
			case <-timeout:
				t.Fatal("timed out")
			case p := <-out:
				tt.fn(p.TracerPayload)
			}
		})
	}
}

func TestCreateChunks_OTelCompliantTranslation(t *testing.T) {
	tests := []struct {
		probabilisticSamplerEnabled bool
	}{
		{
			probabilisticSamplerEnabled: false,
		},
		{
			probabilisticSamplerEnabled: true,
		},
	}
	for _, tt := range tests {
		var names []string
		if tt.probabilisticSamplerEnabled {
			names = append(names, "ProbabilisticSamplerEnabled")
		} else {
			names = append(names, "ProbabilisticSamplerDisabled")
		}
		t.Run(strings.Join(names, " "), func(t *testing.T) {
			cfg := NewTestConfig(t)
			cfg.Features["enable_otel_compliant_translation"] = struct{}{}
			cfg.OTLPReceiver.ProbabilisticSampling = 50
			cfg.ProbabilisticSamplerEnabled = tt.probabilisticSamplerEnabled
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
			require.Len(t, chunks, len(traces))
			for _, c := range chunks {
				if tt.probabilisticSamplerEnabled {
					require.Emptyf(t, c.Spans[0].Meta["_dd.p.dm"], "decision maker must be empty")
					require.Equalf(t, int32(sampler.PriorityNone), c.Priority, "priority must be none")
				} else {
					id := c.Spans[0].TraceID
					require.ElementsMatch(t, c.Spans, traces[id])
					require.Equal(t, "0.50", c.Tags["_dd.otlp_sr"])
					switch id {
					case traceID1:
						require.Equal(t, "-9", c.Spans[0].Meta["_dd.p.dm"], "traceID1: dm must be -9")
						require.Equal(t, int32(1), c.Priority, "traceID1: priority must be 1")
					case traceID2:
						require.Empty(t, c.Spans[0].Meta["_dd.p.dm"], "traceID2: dm must be empty")
						require.Equal(t, int32(0), c.Priority, "traceID2: priority must be 0")
					case traceID3:
						require.Equal(t, "-4", c.Spans[0].Meta["_dd.p.dm"], "traceID3: dm must be -4")
						require.Equal(t, int32(2), c.Priority, "traceID3: priority must be 2")
					}
				}
			}
		})
	}
}

func TestOTLPReceiveResourceSpans_OTelCompliantTranslation(t *testing.T) {
	cfg := NewTestConfig(t)
	cfg.Features["enable_otel_compliant_translation"] = struct{}{}
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
					Attributes: map[string]interface{}{string(semconv.DeploymentEnvironmentKey): "depenv"},
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
					Attributes: map[string]interface{}{string(semconv.DeploymentEnvironmentKey): "spanenv"},
					Spans: []*testutil.OTLPSpan{
						{Attributes: map[string]interface{}{}},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("spanenv", out.Env)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{"deployment.environment.name": "res-env"},
					Spans: []*testutil.OTLPSpan{
						{Attributes: map[string]interface{}{}},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("res-env", out.Env)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{"datadog.host.name": "dd.host"},
				},
			},
			fn: func(out *pb.TracerPayload) {
				require.Equal("dd.host", out.Hostname)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{string(semconv.ContainerIDKey): "1234cid"},
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
						string(semconv.K8SPodUIDKey):          "1234cid",
						string(semconv.K8SJobNameKey):         "kubejob",
						string(semconv.ContainerImageNameKey): "lorem-ipsum",
						string(semconv.ContainerImageTagKey):  "v2.0",
						string("datadog.container.tag.team"):  "otel",
					},
					Spans: []*testutil.OTLPSpan{
						{
							TraceID: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
							Name:    "first",
							Attributes: map[string]interface{}{
								// We should not fetch container tags from Span Attributes.
								string(semconv.K8SContainerNameKey): "lorem-ipsum",
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
						{Attributes: map[string]interface{}{string(semconv.K8SPodUIDKey): "123cid"}},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				// V2: container ID only read from resource attributes, not span attributes
				require.Equal("", out.ContainerID)
			},
		},
		{
			in: []testutil.OTLPResourceSpan{
				{
					LibName:    "libname",
					LibVersion: "1.2",
					Attributes: map[string]interface{}{},
					Spans: []*testutil.OTLPSpan{
						{Attributes: map[string]interface{}{string(semconv.ContainerIDKey): "23cid"}},
					},
				},
			},
			fn: func(out *pb.TracerPayload) {
				// V2: container ID only read from resource attributes, not span attributes
				require.Equal("", out.ContainerID)
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
			rcv.ReceiveResourceSpans(context.Background(), rspans, http.Header{}, nil)
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
			rcv.ReceiveResourceSpans(context.Background(), rspans, header, nil)
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
					Spans:      []*testutil.OTLPSpan{{Attributes: map[string]interface{}{string(semconv.K8SPodUIDKey): "123cid"}}},
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
					Spans: []*testutil.OTLPSpan{{Attributes: map[string]interface{}{string(semconv.K8SPodUIDKey): "123cid"}}},
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
			Spans:      []*testutil.OTLPSpan{{Attributes: map[string]interface{}{string(semconv.K8SPodUIDKey): "123cid"}}},
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

func TestOTLPHostname_OTelCompliantTranslation(t *testing.T) {
	t.Helper()
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

	for _, tt := range testcases {
		cfg := NewTestConfig(t)
		cfg.Features["enable_otel_compliant_translation"] = struct{}{}
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
		}).Traces().ResourceSpans().At(0), http.Header{}, nil)
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

func TestOTLPReceiver_OTelCompliantTranslation(t *testing.T) {
	t.Helper()
	t.Run("New", func(t *testing.T) {
		cfg := NewTestConfig(t)
		cfg.Features["enable_otel_compliant_translation"] = struct{}{}
		assert.NotNil(t, NewOTLPReceiver(nil, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{}).conf)
	})

	t.Run("Start/nil", func(t *testing.T) {
		cfg := NewTestConfig(t)
		cfg.Features["enable_otel_compliant_translation"] = struct{}{}
		o := NewOTLPReceiver(nil, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
		o.Start()
		defer o.Stop()
		assert.Nil(t, o.grpcsrv)
	})

	t.Run("Start/grpc", func(t *testing.T) {
		port := testutil.FreeTCPPort(t)
		cfg := NewTestConfig(t)
		cfg.Features["enable_otel_compliant_translation"] = struct{}{}
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
		cfg.Features["enable_otel_compliant_translation"] = struct{}{}
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
				assert.Equal(t, "opentelemetry_grpc_v1", p.Source.EndpointVersion)
				assert.Len(t, p.TracerPayload.Chunks, 1)
				ps[i] = p
			case <-timeout:
				t.Fatal("timed out")
			}
		}
	})
}

func TestOTLPHelpers_OTelCompliantTranslation(t *testing.T) {
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
			got := transform.Status2Error(status, tt.events, span.Meta)
			assert.Equal(tt.out.Error, got)
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
				meta: map[string]string{string(semconv.RPCServiceKey): "SVC", string(semconv.RPCMethodKey): "M"},
				out:  "M SVC",
			},
			{
				meta: map[string]string{string(semconv.RPCMethodKey): "M"},
				out:  "M",
			},
			{
				meta: map[string]string{"graphql.operation.name": "myQuery"},
				out:  "",
			},
			{
				meta: map[string]string{"graphql.operation.type": "query"},
				out:  "query",
			},
			{
				meta: map[string]string{"graphql.operation.type": "query", "graphql.operation.name": "myQuery"},
				out:  "query myQuery",
			},
		} {
			assert.Equal(t, tt.out, resourceFromTags(tt.meta))
		}
	})

	// test spanKind2Type moved to pkg/trace/traceutil/otel_util_test.go

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

func TestOTelSpanToDDSpan_OTelCompliantTranslation(t *testing.T) {
	cfg := NewTestConfig(t)
	now := uint64(otlpTestSpan.StartTimestamp())
	cfg.Features["enable_otel_compliant_translation"] = struct{}{}
	for i, tt := range []struct {
		name                       string
		rattr                      map[string]string
		libname                    string
		libver                     string
		sattr                      map[string]string
		in                         ptrace.Span
		operationName              string
		resourceName               string
		out                        *pb.Span
		outTags                    map[string]string
		topLevelOutMetrics         map[string]float64
		ignoreMissingDatadogFields bool
		wrongPlaceKeysCount        int
	}{
		{
			name: "error extraction from exception events with span links marshaling",
			rattr: map[string]string{
				"service.name":    "pylons",
				"service.version": "v1.2.3",
				"env":             "staging",
			},
			libname:       "ddtracer",
			libver:        "v2",
			in:            otlpTestSpan,
			operationName: "server.request",
			resourceName:  "/path",
			out: &pb.Span{
				Service:  "pylons",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Error:    1,
				Meta: map[string]string{
					"name":                          "john",
					"otel.trace_id":                 "72df520af2bde7a5240031ead750e5f3",
					"env":                           "staging",
					"otel.status_code":              "Error",
					"otel.status_description":       "Error",
					"otel.library.name":             "ddtracer",
					"otel.library.version":          "v2",
					"service.version":               "v1.2.3",
					"w3c.tracestate":                "state",
					"version":                       "v1.2.3",
					"events":                        `[{"time_unix_nano":123,"name":"boom","attributes":{"key":"Out of memory","accuracy":2.4},"dropped_attributes_count":2},{"time_unix_nano":456,"name":"exception","attributes":{"exception.message":"Out of memory","exception.type":"mem","exception.stacktrace":"1/2/3"},"dropped_attributes_count":2}]`,
					"_dd.span_links":                `[{"trace_id":"fedcba98765432100123456789abcdef","span_id":"abcdef0123456789","tracestate":"dd=asdf256,ee=jkl;128", "attributes":{"a1":"v1","a2":"v2"},"dropped_attributes_count":24},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210","attributes":{"a3":"v2","a4":"v4"}},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210","dropped_attributes_count":2},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210"}]`,
					"error.msg":                     "Out of memory",
					"error.type":                    "mem",
					"error.stack":                   "1/2/3",
					"span.kind":                     "server",
					"_dd.span_events.has_exception": "true",
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
			name: "peer.service attribute with error handling and span links",
			rattr: map[string]string{
				"service.version": "v1.2.3",
				"service.name":    "myservice",
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
					"name":         "john",
					"peer.service": "userbase",
					"http.method":  "GET",
					"http.route":   "/path",
					"approx":       1.2,
					"count":        2,
					"span.kind":    "server",
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
			operationName: "http.server.request",
			resourceName:  "GET /path",
			out: &pb.Span{
				Service:  "myservice",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Error:    1,
				Meta: map[string]string{
					"name":                          "john",
					"otel.trace_id":                 "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":              "Error",
					"otel.status_description":       "Error",
					"otel.library.name":             "ddtracer",
					"otel.library.version":          "v2",
					"service.version":               "v1.2.3",
					"w3c.tracestate":                "state",
					"version":                       "v1.2.3",
					"events":                        "[{\"time_unix_nano\":123,\"name\":\"boom\",\"attributes\":{\"message\":\"Out of memory\",\"accuracy\":2.4},\"dropped_attributes_count\":2},{\"time_unix_nano\":456,\"name\":\"exception\",\"attributes\":{\"exception.message\":\"Out of memory\",\"exception.type\":\"mem\",\"exception.stacktrace\":\"1/2/3\"},\"dropped_attributes_count\":2}]",
					"_dd.span_links":                `[{"trace_id":"fedcba98765432100123456789abcdef","span_id":"abcdef0123456789","tracestate":"dd=asdf256,ee=jkl;128","attributes":{"a1":"v1","a2":"v2"},"dropped_attributes_count":24},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210","attributes":{"a3":"v2","a4":"v4"}},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210","dropped_attributes_count":2},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210"}]`,
					"error.msg":                     "Out of memory",
					"error.type":                    "mem",
					"error.stack":                   "1/2/3",
					"http.method":                   "GET",
					"http.route":                    "/path",
					"peer.service":                  "userbase",
					"span.kind":                     "server",
					"_dd.span_events.has_exception": "true",
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
			name: "test all HTTP attributes",
			rattr: map[string]string{
				attributes.APMConventionKeys.Service(): "myservice",
				"service.version":                      "v1.2.3",
				"env":                                  "staging",
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
					"name":                              "john",
					"http.method":                       "GET",
					"http.route":                        "/path",
					"approx":                            1.2,
					"count":                             2,
					"analytics.event":                   "false",
					string(semconv127.ClientAddressKey): "sample_client_address",
					string(semconv127.HTTPResponseBodySizeKey):   "sample_content_length",
					string(semconv127.HTTPResponseStatusCodeKey): "200",
					string(semconv127.HTTPRequestBodySizeKey):    "sample_content_length",
					"http.request.header.referrer":               "sample_referrer",
					string(semconv127.NetworkProtocolVersionKey): "sample_version",
					string(semconv127.ServerAddressKey):          "sample_server_name",
					string(semconv127.URLFullKey):                "sample_url",
					string(semconv127.UserAgentOriginalKey):      "sample_useragent",
					"http.request.header.example":                "test",
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
			operationName: "http.server.request",
			resourceName:  "GET /path",
			out: &pb.Span{
				Service:  "myservice",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Error:    1,
				Meta: map[string]string{
					"name":                          "john",
					"env":                           "staging",
					"otel.status_code":              "Error",
					"otel.status_description":       "Error",
					"otel.library.name":             "ddtracer",
					"otel.library.version":          "v2",
					"service.version":               "v1.2.3",
					"w3c.tracestate":                "state",
					"version":                       "v1.2.3",
					"otel.trace_id":                 "72df520af2bde7a5240031ead750e5f3",
					"events":                        "[{\"time_unix_nano\":123,\"name\":\"boom\",\"attributes\":{\"message\":\"Out of memory\",\"accuracy\":2.4},\"dropped_attributes_count\":2},{\"time_unix_nano\":456,\"name\":\"exception\",\"attributes\":{\"exception.message\":\"Out of memory\",\"exception.type\":\"mem\",\"exception.stacktrace\":\"1/2/3\"},\"dropped_attributes_count\":2}]",
					"_dd.span_links":                `[{"trace_id":"fedcba98765432100123456789abcdef","span_id":"abcdef0123456789","tracestate":"dd=asdf256,ee=jkl;128","attributes":{"a1":"v1","a2":"v2"},"dropped_attributes_count":24},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210","attributes":{"a3":"v2","a4":"v4"}},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210","dropped_attributes_count":2},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210"}]`,
					"error.msg":                     "Out of memory",
					"error.type":                    "mem",
					"error.stack":                   "1/2/3",
					"http.method":                   "GET",
					"http.route":                    "/path",
					"span.kind":                     "server",
					"_dd.span_events.has_exception": "true",
					"http.client_ip":                "sample_client_address",
					"http.response.content_length":  "sample_content_length",
					"http.request.content_length":   "sample_content_length",
					"http.referrer":                 "sample_referrer",
					"http.version":                  "sample_version",
					"http.server_name":              "sample_server_name",
					"http.url":                      "sample_url",
					"http.useragent":                "sample_useragent",
					"http.request.headers.example":  "test",
					"http.status_code":              "200",

					// Original OTLP keys
					string(semconv127.ClientAddressKey):          "sample_client_address",
					string(semconv127.HTTPResponseBodySizeKey):   "sample_content_length",
					string(semconv127.HTTPResponseStatusCodeKey): "200",
					string(semconv127.HTTPRequestBodySizeKey):    "sample_content_length",
					"http.request.header.referrer":               "sample_referrer",
					string(semconv127.NetworkProtocolVersionKey): "sample_version",
					string(semconv127.ServerAddressKey):          "sample_server_name",
					string(semconv127.URLFullKey):                "sample_url",
					string(semconv127.UserAgentOriginalKey):      "sample_useragent",
					"http.request.header.example":                "test",
				},
				Metrics: map[string]float64{
					"approx":                               1.2,
					"count":                                2,
					sampler.KeySamplingRateEventExtraction: 0,
					"http.status_code":                     200,
				},
				Type: "web",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level":                           1,
				"approx":                               1.2,
				"count":                                2,
				sampler.KeySamplingRateEventExtraction: 0,
				"http.status_code":                     200,
			},
		}, {
			name: "explicit DD convention keys with analytics and container tags",
			rattr: map[string]string{
				"env":          "staging",
				"service.name": "mongo",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				Name:  "/path",
				Start: now,
				End:   now + 200000000,
				Attributes: map[string]interface{}{
					"operation.name":                    "READ",
					"resource.name":                     "/path",
					"span.type":                         "db",
					"name":                              "john",
					string(semconv.ContainerIDKey):      "cid",
					string(semconv.K8SContainerNameKey): "k8s-container",
					"http.method":                       "GET",
					"http.route":                        "/path",
					"approx":                            1.2,
					"count":                             2,
					"analytics.event":                   true,
				},
			}),
			operationName: "read",
			resourceName:  "/path",
			out: &pb.Span{
				Service:  "mongo",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"env":                               "staging",
					string(semconv.ContainerIDKey):      "cid",
					string(semconv.K8SContainerNameKey): "k8s-container",
					"http.method":                       "GET",
					"http.route":                        "/path",
					"otel.status_code":                  "Unset",
					"otel.library.name":                 "ddtracer",
					"otel.library.version":              "v2",
					"name":                              "john",
					"otel.trace_id":                     "72df520af2bde7a5240031ead750e5f3",
					"span.kind":                         "unspecified",
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
			name: "HTTP server call: connection dropped before response body was sent",
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
				},
			}),
			operationName: "ddtracer.server",
			resourceName:  "POST",
			out: &pb.Span{
				Service:  "document-uploader",
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
					"http.method":               "POST",
					"url.path":                  "/uploads/4",
					"url.scheme":                "https",
					"http.route":                "/uploads/:document_id",
					"http.status_code":          "201",
					"error.msg":                 "Created",
					"otel.trace_id":             "72df520af2bde7a5240031ead750e5f3",
					"span.kind":                 "unspecified",
					"http.request.method":       "POST",
					"http.response.status_code": "201",
				},
				Metrics: map[string]float64{
					"http.status_code": 201,
				},
				Type: "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level":       1,
				"http.status_code": 201,
			},
		},
		{
			name: "HTTP error with old semconv (v1.17)",
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
			operationName: "ddtracer.server",
			resourceName:  "POST",
			out: &pb.Span{
				Service:  "document-uploader",
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
					"error.msg":            "Created",
					"http.method":          "POST",
					"url.path":             "/uploads/4",
					"url.scheme":           "https",
					"http.route":           "/uploads/:document_id",
					"http.status_code":     "201",
					"error.type":           "WebSocketDisconnect",
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"span.kind":            "unspecified",
				},
				Metrics: map[string]float64{
					"http.status_code": 201,
				},
				Type: "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level":       1,
				"http.status_code": 201,
			},
		},
		{
			name: "datadog.* namespace precedence from resource attributes",
			rattr: map[string]string{
				attributes.DDNamespaceKeys.Service(): "test-service",
				attributes.DDNamespaceKeys.Env():     "test-env",
				attributes.DDNamespaceKeys.Version(): "test-version",
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
					attributes.DDNamespaceKeys.OperationName():  "test-name",
					attributes.DDNamespaceKeys.ResourceName():   "test-resource",
					attributes.DDNamespaceKeys.SpanType():       "test-type",
					attributes.DDNamespaceKeys.SpanKind():       "test-kind",
					attributes.DDNamespaceKeys.HTTPStatusCode(): 404,
				},
			}),
			operationName: "test-name",
			resourceName:  "test-resource",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Error:    0,
				Meta: map[string]string{
					"env":                  "test-env",
					"version":              "test-version",
					"span.kind":            "test-kind",
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"w3c.tracestate":       "state",
					"http.status_code":     "404",
				},
				Metrics: map[string]float64{
					"http.status_code": 404,
				},
				Type: "test-type",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level":       1,
				"http.status_code": 404,
			},
		},
		{
			name: "ignoreMissingDatadogFields flag - expected keys are skipped, others still populated",
			rattr: map[string]string{
				"service.name":    "myservice",
				"service.version": "v1.2.3",
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
					"name":                              "john",
					"http.method":                       "GET",
					"http.route":                        "/path",
					"approx":                            1.2,
					"count":                             2,
					"analytics.event":                   "false",
					string(semconv127.ClientAddressKey): "sample_client_address",
					string(semconv127.HTTPResponseBodySizeKey):   "sample_content_length",
					string(semconv127.HTTPResponseStatusCodeKey): "202",
					string(semconv127.HTTPRequestBodySizeKey):    "sample_content_length",
					"http.request.header.referrer":               "sample_referrer",
					string(semconv127.NetworkProtocolVersionKey): "sample_version",
					string(semconv127.ServerAddressKey):          "sample_server_name",
					string(semconv127.URLFullKey):                "sample_url",
					string(semconv127.UserAgentOriginalKey):      "sample_useragent",
					"http.request.header.example":                "test",
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
			operationName: "server",
			resourceName:  "/path",
			out: &pb.Span{
				Service:  "otlpresourcenoservicename",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Error:    1,
				Meta: map[string]string{
					"name":                          "john",
					"otel.status_code":              "Error",
					"otel.status_description":       "Error",
					"otel.library.name":             "ddtracer",
					"otel.library.version":          "v2",
					"w3c.tracestate":                "state",
					"otel.trace_id":                 "72df520af2bde7a5240031ead750e5f3",
					"events":                        "[{\"time_unix_nano\":123,\"name\":\"boom\",\"attributes\":{\"message\":\"Out of memory\",\"accuracy\":2.4},\"dropped_attributes_count\":2},{\"time_unix_nano\":456,\"name\":\"exception\",\"attributes\":{\"exception.message\":\"Out of memory\",\"exception.type\":\"mem\",\"exception.stacktrace\":\"1/2/3\"},\"dropped_attributes_count\":2}]",
					"_dd.span_links":                `[{"trace_id":"fedcba98765432100123456789abcdef","span_id":"abcdef0123456789","tracestate":"dd=asdf256,ee=jkl;128","attributes":{"a1":"v1","a2":"v2"},"dropped_attributes_count":24},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210","attributes":{"a3":"v2","a4":"v4"}},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210","dropped_attributes_count":2},{"trace_id":"abcdef0123456789abcdef0123456789","span_id":"fedcba9876543210"}]`,
					"http.route":                    "/path",
					"_dd.span_events.has_exception": "true",
					"http.method":                   "GET",
					"http.client_ip":                "sample_client_address",
					"http.response.content_length":  "sample_content_length",
					"http.request.content_length":   "sample_content_length",
					"http.referrer":                 "sample_referrer",
					"http.version":                  "sample_version",
					"http.server_name":              "sample_server_name",
					"http.url":                      "sample_url",
					"http.useragent":                "sample_useragent",
					"http.request.headers.example":  "test",
					"span.kind":                     "unspecified",
					"error.msg":                     "Out of memory",
					"error.type":                    "mem",
					"error.stack":                   "1/2/3",

					// Original keys
					string(semconv127.ClientAddressKey):          "sample_client_address",
					string(semconv127.HTTPResponseBodySizeKey):   "sample_content_length",
					string(semconv127.HTTPRequestBodySizeKey):    "sample_content_length",
					"http.request.header.referrer":               "sample_referrer",
					string(semconv127.NetworkProtocolVersionKey): "sample_version",
					string(semconv127.ServerAddressKey):          "sample_server_name",
					string(semconv127.URLFullKey):                "sample_url",
					string(semconv127.UserAgentOriginalKey):      "sample_useragent",
					"http.request.header.example":                "test",

					//ignored fields
					"otel.span.http.response.status_code": "202",
					"otel.resource.service.version":       "v1.2.3",
				},
				Metrics: map[string]float64{
					"approx":                               1.2,
					"count":                                2,
					sampler.KeySamplingRateEventExtraction: 0,
				},
				Type: "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level":                           1,
				"approx":                               1.2,
				"count":                                2,
				sampler.KeySamplingRateEventExtraction: 0,
			},
			ignoreMissingDatadogFields: true,
			wrongPlaceKeysCount:        2,
		},
		{
			name: "OTel Collector gRPC span with RPC semantic conventions",
			rattr: map[string]string{
				"service.instance.id": "02aa8742-d8a2-46b3-87d1-1ccaeb48e0ab",
				"service.name":        "otelcol",
				"service.version":     "0.123.0",
			},
			libname: "go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc",
			libver:  "0.60.0",
			sattr: map[string]string{
				"otelcol.component.id":   "otlp",
				"otelcol.component.kind": "Receiver",
			},
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "opentelemetry.proto.collector.trace.v1.TraceService/Export",
				Kind:    ptrace.SpanKindServer,
				Start:   now,
				End:     now + 2000000,
				Attributes: map[string]any{
					"net.sock.peer.addr":   "127.0.0.1",
					"net.sock.peer.port":   63333,
					"rpc.grpc.status_code": 0,
					"rpc.method":           "Export",
					"rpc.service":          "opentelemetry.proto.collector.trace.v1.TraceService",
					"rpc.system":           "grpc",
				},
			}),
			out: &pb.Span{
				Service:  "otelcol",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				Start:    int64(now),
				Duration: 2000000,
				Meta: map[string]string{
					"service.instance.id": "02aa8742-d8a2-46b3-87d1-1ccaeb48e0ab",
					"service.version":     "0.123.0",
					"version":             "0.123.0",

					"otel.library.name":      "go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc",
					"otel.library.version":   "0.60.0",
					"otelcol.component.id":   "otlp",
					"otelcol.component.kind": "Receiver",

					"net.sock.peer.addr": "127.0.0.1",
					"rpc.method":         "Export",
					"rpc.service":        "opentelemetry.proto.collector.trace.v1.TraceService",
					"rpc.system":         "grpc",

					"span.kind":        "server",
					"otel.status_code": "Unset",
					"otel.trace_id":    "72df520af2bde7a5240031ead750e5f3",
				},
				Metrics: map[string]float64{
					"net.sock.peer.port":   63333,
					"rpc.grpc.status_code": 0,
				},
				Type: "web",
			},
			operationName: "grpc.server.request",
			resourceName:  "Export opentelemetry.proto.collector.trace.v1.TraceService",
			topLevelOutMetrics: map[string]float64{
				"_top_level":           1,
				"net.sock.peer.port":   63333,
				"rpc.grpc.status_code": 0,
			},
		},
		// Class 1: DD convention keys in correct location (resource)
		{
			name: "DD convention key service.name in resource attributes",
			rattr: map[string]string{
				"service.name": "test-service",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"span.kind":            "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
		},
		{
			name: "DD convention key version in resource attributes",
			rattr: map[string]string{
				"service.name": "test-service",
				"version":      "v1.2.3",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"version":              "v1.2.3",
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"span.kind":            "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
		},
		{
			name: "DD convention key env in resource attributes",
			rattr: map[string]string{
				"service.name": "test-service",
				"env":          "production",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"env":                  "production",
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"span.kind":            "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
		},
		// Class 1b: DD convention keys in WRONG location (span instead of resource)
		{
			name: "DD convention key service.name in span attributes (wrong location)",
			rattr: map[string]string{
				"service.name": "resource-service",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					"service.name": "span-service",
				},
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "resource-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"otel.span.service.name": "span-service",
					"otel.trace_id":          "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":       "Unset",
					"otel.library.name":      "ddtracer",
					"otel.library.version":   "v2",
					"span.kind":              "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
			wrongPlaceKeysCount: 1,
		},
		{
			name: "DD convention key version in span attributes (wrong location)",
			rattr: map[string]string{
				"service.name": "test-service",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					"version": "v9.9.9",
				},
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"otel.span.version":    "v9.9.9",
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"span.kind":            "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
			wrongPlaceKeysCount: 1,
		},
		{
			name: "DD convention key env in span attributes (wrong location)",
			rattr: map[string]string{
				"service.name": "test-service",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					"env": "staging",
				},
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"otel.span.env":        "staging",
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"span.kind":            "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
			wrongPlaceKeysCount: 1,
		},
		// Class 2: datadog.* namespace keys
		{
			name: "datadog.service.name in resource attributes",
			rattr: map[string]string{
				attributes.DDNamespaceKeys.Service(): "dd-service",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "dd-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"span.kind":            "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
		},
		{
			name: "datadog.env in resource attributes",
			rattr: map[string]string{
				"service.name":                   "test-service",
				attributes.DDNamespaceKeys.Env(): "dd-prod",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"env":                  "dd-prod",
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"span.kind":            "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
		},
		{
			name: "datadog.operation.name in span attributes",
			rattr: map[string]string{
				"service.name": "test-service",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					attributes.DDNamespaceKeys.OperationName(): "custom-operation",
				},
			}),
			operationName: "custom-operation",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"span.kind":            "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
		},
		{
			name: "datadog.service.name in span attributes (wrong location)",
			rattr: map[string]string{
				"service.name": "resource-service",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					attributes.DDNamespaceKeys.Service(): "span-dd-service",
				},
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "resource-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"otel.span.datadog.service": "span-dd-service",
					"otel.trace_id":             "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":          "Unset",
					"otel.library.name":         "ddtracer",
					"otel.library.version":      "v2",
					"span.kind":                 "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
			wrongPlaceKeysCount: 1,
		},
		// Class 3: OTel->DD mapped keys in correct location
		{
			name: "service.version in resource maps to version - both conventions keys are preserved",
			rattr: map[string]string{
				"service.name":    "test-service",
				"service.version": "v2.0.0",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"version":              "v2.0.0",
					"service.version":      "v2.0.0",
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"span.kind":            "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
		},
		{
			name: "deployment.environment.name in resource maps to env - both conventions keys are preserved",
			rattr: map[string]string{
				"service.name":                "test-service",
				"deployment.environment.name": "staging",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"env":                         "staging",
					"deployment.environment.name": "staging",
					"otel.trace_id":               "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":            "Unset",
					"otel.library.name":           "ddtracer",
					"otel.library.version":        "v2",
					"span.kind":                   "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
		},
		{
			name: "http.response.status_code in span maps to http.status_code - both conventions keys are preserved",
			rattr: map[string]string{
				"service.name": "test-service",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					string(semconv127.HTTPResponseStatusCodeKey): "404",
				},
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"http.status_code":                           "404",
					string(semconv127.HTTPResponseStatusCodeKey): "404",
					"otel.trace_id":                              "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":                           "Unset",
					"otel.library.name":                          "ddtracer",
					"otel.library.version":                       "v2",
					"span.kind":                                  "unspecified",
				},
				Metrics: map[string]float64{
					"http.status_code": 404,
				},
				Type: "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level":       1,
				"http.status_code": 404,
			},
		},
		// Class 4: OTel->DD mapped keys in WRONG location
		{
			name: "service.version in span attributes (wrong location)",
			rattr: map[string]string{
				"service.name": "test-service",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					"service.version": "v8.8.8",
				},
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"otel.span.service.version": "v8.8.8",
					"otel.trace_id":             "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":          "Unset",
					"otel.library.name":         "ddtracer",
					"otel.library.version":      "v2",
					"span.kind":                 "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
			wrongPlaceKeysCount: 1,
		},
		{
			name: "deployment.environment.name in span attributes (wrong location)",
			rattr: map[string]string{
				"service.name": "test-service",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					"deployment.environment.name": "test",
				},
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"otel.span.deployment.environment.name": "test",
					"otel.trace_id":                         "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":                      "Unset",
					"otel.library.name":                     "ddtracer",
					"otel.library.version":                  "v2",
					"span.kind":                             "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
			wrongPlaceKeysCount: 1,
		},
		{
			name: "http.response.status_code in resource attributes (wrong location)",
			rattr: map[string]string{
				"service.name": "test-service",
				string(semconv127.HTTPResponseStatusCodeKey): "500",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"otel.resource." + string(semconv127.HTTPResponseStatusCodeKey): "500",
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"span.kind":            "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
			wrongPlaceKeysCount: 1,
		},
		// Class 5: Unknown keys with collisions
		{
			name: "unknown key in resource only",
			rattr: map[string]string{
				"service.name": "test-service",
				"custom.key":   "resource-value",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"custom.key":           "resource-value",
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"span.kind":            "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
		},
		{
			name: "unknown key in span only",
			rattr: map[string]string{
				"service.name": "test-service",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					"custom.key": "span-value",
				},
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"custom.key":           "span-value",
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"span.kind":            "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
		},
		{
			name: "unknown key collision - span wins, resource prefixed",
			rattr: map[string]string{
				"service.name": "test-service",
				"custom.key":   "resource-value",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					"custom.key": "span-value",
				},
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"custom.key":               "span-value",
					"otel.resource.custom.key": "resource-value",
					"otel.trace_id":            "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":         "Unset",
					"otel.library.name":        "ddtracer",
					"otel.library.version":     "v2",
					"span.kind":                "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
			wrongPlaceKeysCount: 0, // This setup results in more data than before, so no need to count it as "misconfigured"
		},

		// Class 6: Precedence tests - datadog.* > APM convention > OTel semconv
		{
			name: "service precedence: datadog.service > service.name",
			rattr: map[string]string{
				attributes.DDNamespaceKeys.Service(): "dd-service",
				"service.name":                       "apm-service",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "dd-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"span.kind":            "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
		},
		{
			name: "env precedence: datadog.env > env > deployment.environment.name",
			rattr: map[string]string{
				attributes.DDNamespaceKeys.Env(): "dd-env",
				"env":                            "apm-env",
				string(semconv127.DeploymentEnvironmentNameKey): "otel-env",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "otlpresourcenoservicename",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"env": "dd-env",
					string(semconv127.DeploymentEnvironmentNameKey): "otel-env",
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"span.kind":            "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
		},
		{
			name: "version precedence: datadog.version > version > service.version",
			rattr: map[string]string{
				"service.name":                       "test-service",
				attributes.DDNamespaceKeys.Version(): "dd-v1.0.0",
				"version":                            "apm-v2.0.0",
				string(semconv127.ServiceVersionKey): "otel-v3.0.0",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"version":                            "dd-v1.0.0",
					string(semconv127.ServiceVersionKey): "otel-v3.0.0",
					"otel.trace_id":                      "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":                   "Unset",
					"otel.library.name":                  "ddtracer",
					"otel.library.version":               "v2",
					"span.kind":                          "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
		},
		{
			name: "http.status_code precedence: datadog.http_status_code > http.status_code > http.response.status_code",
			rattr: map[string]string{
				"service.name": "test-service",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					attributes.DDNamespaceKeys.HTTPStatusCode():  500,
					"http.status_code":                           "404",
					string(semconv127.HTTPResponseStatusCodeKey): "200",
				},
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"http.status_code":                           "500",
					string(semconv127.HTTPResponseStatusCodeKey): "200",
					"otel.trace_id":                              "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":                           "Unset",
					"otel.library.name":                          "ddtracer",
					"otel.library.version":                       "v2",
					"span.kind":                                  "unspecified",
				},
				Metrics: map[string]float64{
					"http.status_code": 500,
				},
				Type: "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level":       1,
				"http.status_code": 500,
			},
		},
		{
			name: "operation.name precedence: datadog.operation_name > operation.name",
			rattr: map[string]string{
				"service.name": "test-service",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					attributes.DDNamespaceKeys.OperationName(): "dd-operation",
					"operation.name": "apm-operation",
				},
			}),
			operationName: "dd-operation",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"span.kind":            "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
		},
		{
			name: "resource.name precedence: datadog.resource_name > resource.name",
			rattr: map[string]string{
				"service.name": "test-service",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					attributes.DDNamespaceKeys.ResourceName(): "dd-resource",
					"resource.name": "apm-resource",
				},
			}),
			operationName: "internal",
			resourceName:  "dd-resource",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"span.kind":            "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
		},
		{
			name: "span.type precedence: datadog.span.type > span.type",
			rattr: map[string]string{
				"service.name": "test-service",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					attributes.DDNamespaceKeys.SpanType(): "dd-type",
					"span.type":                           "apm-type",
				},
			}),
			operationName: "internal",
			resourceName:  "test-span",
			out: &pb.Span{
				Service:  "test-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"span.kind":            "unspecified",
				},
				Metrics: map[string]float64{},
				Type:    "dd-type",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level": 1,
			},
		},
		{
			name: "mixed precedence test - all levels present for multiple fields",
			rattr: map[string]string{
				attributes.DDNamespaceKeys.Service(): "dd-service",
				"service.name":                       "apm-service",
				attributes.DDNamespaceKeys.Env():     "dd-env",
				"env":                                "apm-env",
				string(semconv127.DeploymentEnvironmentNameKey): "otel-env",
				attributes.DDNamespaceKeys.Version():            "dd-version",
				"version":                                       "apm-version",
				string(semconv127.ServiceVersionKey):            "otel-version",
			},
			libname: "ddtracer",
			libver:  "v2",
			in: testutil.NewOTLPSpan(&testutil.OTLPSpan{
				TraceID: otlpTestTraceID,
				SpanID:  otlpTestSpanID,
				Name:    "test-span",
				Start:   now,
				End:     now + 200000000,
				Attributes: map[string]interface{}{
					attributes.DDNamespaceKeys.OperationName(): "dd-op",
					attributes.DDNamespaceKeys.ResourceName():  "dd-res",
					"operation.name":   "apm-op",
					"resource.name":    "apm-res",
					"http.status_code": "200",
					string(semconv127.HTTPResponseStatusCodeKey): "201",
					attributes.DDNamespaceKeys.HTTPStatusCode():  "404",
				},
			}),
			operationName: "dd-op",
			resourceName:  "dd-res",
			out: &pb.Span{
				Service:  "dd-service",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					// Resource attributes
					"env": "dd-env",
					string(semconv127.DeploymentEnvironmentNameKey): "otel-env",
					"version":                            "dd-version",
					string(semconv127.ServiceVersionKey): "otel-version",
					// Span attributes
					"http.status_code":                           "404",
					string(semconv127.HTTPResponseStatusCodeKey): "201",
					// Standard metadata
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"span.kind":            "unspecified",
				},
				Metrics: map[string]float64{
					"http.status_code": 404,
				},
				Type: "custom",
			},
			topLevelOutMetrics: map[string]float64{
				"_top_level":       1,
				"http.status_code": 404,
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			cfg.OTLPReceiver.IgnoreMissingDatadogFields = tt.ignoreMissingDatadogFields
			cfg.Features["enable_otel_compliant_translation"] = struct{}{}
			o := NewOTLPReceiver(nil, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
			lib := pcommon.NewInstrumentationScope()
			lib.SetName(tt.libname)
			lib.SetVersion(tt.libver)
			for k, v := range tt.sattr {
				lib.Attributes().PutStr(k, v)
			}
			assert := assert.New(t)
			want := tt.out
			res := pcommon.NewResource()
			for k, v := range tt.rattr {
				res.Attributes().PutStr(k, v)
			}
			got, wrongPlaceKeysCount := transform.OtelSpanToDDSpan(tt.in, res, lib, o.conf)
			assert.Equal(tt.wrongPlaceKeysCount, wrongPlaceKeysCount)
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
			for k, v := range got.Meta {
				if k != "events" && k != "_dd.span_links" {
					assert.Equal(want.Meta[k], v, fmt.Sprintf("(%d) Meta %v:%v", i, k, v))
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
			want.Name = tt.operationName
			want.Resource = tt.resourceName
			assert.Equal(want, got, i)

			// test new top-level identification feature flag
			o.conf.Features["enable_otlp_compute_top_level_by_span_kind"] = struct{}{}
			got, wrongPlaceKeysCount = transform.OtelSpanToDDSpan(tt.in, res, lib, o.conf)
			assert.Equal(tt.wrongPlaceKeysCount, wrongPlaceKeysCount)
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

func TestOTelSpanToDDSpanSetPeerService_OTelCompliantTranslation(t *testing.T) {
	now := uint64(otlpTestSpan.StartTimestamp())
	cfg := NewTestConfig(t)
	cfg.Features["enable_otel_compliant_translation"] = struct{}{}
	o := NewOTLPReceiver(nil, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
	for i, tt := range []struct {
		rattr               map[string]string
		libname             string
		libver              string
		in                  ptrace.Span
		out                 *pb.Span
		operationName       string
		resourceName        string
		wrongPlaceKeysCount int
	}{
		{
			rattr: map[string]string{
				"version":      "v1.2.3",
				"service.name": "myservice",
				"env":          "prod",
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
					"peer.service": "userbase",
				},
			}),
			operationName: "server.request",
			resourceName:  "/path",
			out: &pb.Span{
				Service:  "myservice",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"env":                  "prod",
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"version":              "v1.2.3",
					"peer.service":         "userbase",
					"span.kind":            "server",
				},
				Type:    "web",
				Metrics: map[string]float64{},
			},
		},
		{
			rattr: map[string]string{
				"version":      "v1.2.3",
				"service.name": "myservice",
				"env":          "prod",
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
					"db.instance":  "postgres",
					"peer.service": "userbase",
				},
			}),
			operationName: "server.request",
			resourceName:  "/path",
			out: &pb.Span{
				Service:  "myservice",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"db.instance":          "postgres",
					"env":                  "prod",
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"version":              "v1.2.3",
					"peer.service":         "userbase",
					"span.kind":            "server",
				},
				Type:    "web",
				Metrics: map[string]float64{},
			},
		},
		{
			rattr: map[string]string{
				"version":      "v1.2.3",
				"service.name": "myservice",
				"env":          "prod",
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
					"db.system":     "postgres",
					"net.peer.name": "remotehost",
				},
			}),
			operationName: "postgres.query",
			resourceName:  "/path",
			out: &pb.Span{
				Service:  "myservice",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"env":                  "prod",
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"version":              "v1.2.3",
					"db.system":            "postgres",
					"net.peer.name":        "remotehost",
					"span.kind":            "client",
				},
				Type:    "db",
				Metrics: map[string]float64{},
			},
		},
		{
			rattr: map[string]string{
				"version":      "v1.2.3",
				"service.name": "myservice",
				"env":          "prod",
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
					"rpc.service":   "GetInstance",
					"net.peer.name": "remotehost",
				},
			}),
			operationName: "client.request",
			resourceName:  "/path",
			out: &pb.Span{
				Service:  "myservice",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"env":                  "prod",
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"version":              "v1.2.3",
					"rpc.service":          "GetInstance",
					"net.peer.name":        "remotehost",
					"span.kind":            "client",
				},
				Type:    "http",
				Metrics: map[string]float64{},
			},
		},
		{
			rattr: map[string]string{
				"version":      "v1.2.3",
				"service.name": "myservice",
				"env":          "prod",
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
					"net.peer.name": "remotehost",
				},
			}),
			operationName: "server.request",
			resourceName:  "/path",
			out: &pb.Span{
				Service:  "myservice",
				TraceID:  2594128270069917171,
				SpanID:   2594128270069917171,
				ParentID: 0,
				Start:    int64(now),
				Duration: 200000000,
				Meta: map[string]string{
					"env":                  "prod",
					"otel.trace_id":        "72df520af2bde7a5240031ead750e5f3",
					"otel.status_code":     "Unset",
					"otel.library.name":    "ddtracer",
					"otel.library.version": "v2",
					"version":              "v1.2.3",
					"net.peer.name":        "remotehost",
					"span.kind":            "server",
				},
				Type:    "web",
				Metrics: map[string]float64{},
			},
		},
		{
			rattr: map[string]string{
				"version":                "v1.2.3",
				"service.name":           "myservice",
				"deployment.environment": "prod",
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
				},
			}),
			operationName: "server.request",
			resourceName:  "/path",
			out: &pb.Span{
				Service:  "myservice",
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
				"version":                "v1.2.3",
				"service.name":           "myservice",
				"deployment.environment": "prod",
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
				},
			}),
			operationName: "server.request",
			resourceName:  "/path",
			out: &pb.Span{
				Service:  "myservice",
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
			got, wrongPlaceKeysCount := transform.OtelSpanToDDSpan(tt.in, res, lib, o.conf)
			assert.Equal(tt.wrongPlaceKeysCount, wrongPlaceKeysCount)
			want := tt.out
			want.Name = tt.operationName
			want.Resource = tt.resourceName
			assert.Equal(want, got, i)
		})
	}
}

func BenchmarkProcessRequestOTelCompliantTranslation(b *testing.B) {
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
	cfg.Features["enable_otel_compliant_translation"] = struct{}{}
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

func BenchmarkProcessRequestTopLevelOTelCompliantTranslation(b *testing.B) {
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
	cfg.Features["enable_otel_compliant_translation"] = struct{}{}
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
