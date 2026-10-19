// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"encoding/binary"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/otlpidx"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/transform"
)

// parityResource is one ResourceSpans of the parity fixtures.
type parityResource struct {
	name  string
	attrs map[string]any
}

var parityResources = []parityResource{
	{name: "host from attributes", attrs: map[string]any{
		"service.name": "checkout", "deployment.environment.name": "prod", "host.name": "web-1",
		"telemetry.sdk.language": "go", "telemetry.sdk.version": "1.30.0",
	}},
	{name: "fallback hostname", attrs: map[string]any{"service.name": "checkout"}},
	{name: "container tags", attrs: map[string]any{
		"service.name": "checkout", "container.id": "abc123", "k8s.pod.name": "checkout-7d9f",
		"k8s.namespace.name": "shop", "cloud.region": "eu-west-1",
		// Datadog-convention and custom container tags are consumed from the resource.
		"kube_deployment": "checkout", "datadog.container.tag.team": "payments",
	}},
	{name: "stats computed", attrs: map[string]any{"service.name": "checkout", "_dd.stats_computed": "true"}},
	{name: "stats not computed", attrs: map[string]any{"service.name": "checkout", "_dd.stats_computed": "false"}},
}

type parityConfig struct {
	name  string
	apply func(*config.AgentConfig)
}

var parityConfigs = []parityConfig{
	{name: "default", apply: func(*config.AgentConfig) {}},
	{name: "top level by kind", apply: func(c *config.AgentConfig) {
		c.Features["enable_otlp_compute_top_level_by_span_kind"] = struct{}{}
	}},
	{name: "probabilistic sampler enabled", apply: func(c *config.AgentConfig) { c.ProbabilisticSamplerEnabled = true }},
	{name: "50% otlp sampling", apply: func(c *config.AgentConfig) { c.OTLPReceiver.ProbabilisticSampling = 50 }},
	{name: "scope convention disabled", apply: func(c *config.AgentConfig) {
		c.Features["disable_otel_scope_convention"] = struct{}{}
	}},
}

func putParityAttrs(m pcommon.Map, from map[string]any) {
	for k, v := range from {
		switch x := v.(type) {
		case string:
			m.PutStr(k, x)
		case int:
			m.PutInt(k, int64(x))
		case float64:
			m.PutDouble(k, x)
		}
	}
}

// buildParityRequest builds one ResourceSpans with 16 traces of a server root, a client child and
// a producer child. Trace 3 has a user sampling priority; traces with an even index carry a
// tracestate with a head-sampling probability, and trace 5 also an explicit _sample_rate.
func buildParityRequest(res parityResource) ptraceotlp.ExportRequest {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	putParityAttrs(rs.Resource().Attributes(), res.attrs)
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("my.lib")
	ss.Scope().SetVersion("0.1")
	var id uint64
	for t := range 16 {
		var tid pcommon.TraceID
		binary.BigEndian.PutUint64(tid[:8], uint64(t+1))
		binary.BigEndian.PutUint64(tid[8:], uint64(t+1)*0x9E3779B97F4A7C15)
		var parent pcommon.SpanID
		for _, kind := range []ptrace.SpanKind{ptrace.SpanKindServer, ptrace.SpanKindClient, ptrace.SpanKindProducer} {
			id++
			var sid pcommon.SpanID
			binary.BigEndian.PutUint64(sid[:], id)
			span := ss.Spans().AppendEmpty()
			span.SetTraceID(tid)
			span.SetSpanID(sid)
			span.SetParentSpanID(parent)
			span.SetName("op")
			span.SetKind(kind)
			span.SetStartTimestamp(1000)
			span.SetEndTimestamp(2000)
			if t%2 == 0 {
				span.TraceState().FromRaw("ot=th:8")
			}
			if t == 5 {
				span.Attributes().PutDouble("_sample_rate", 0.25)
			}
			if t == 3 && kind == ptrace.SpanKindServer {
				span.Attributes().PutDouble("_sampling_priority_v1", 2)
			}
			parent = sid
		}
	}
	return ptraceotlp.NewExportRequestFromTraces(td)
}

func splitTags(s string) []string {
	if s == "" {
		return nil
	}
	tags := strings.Split(s, ",")
	slices.Sort(tags)
	return tags
}

func numericAttr(tp *idx.InternalTracerPayload, s *idx.InternalSpan, key string) (float64, bool) {
	ref := tp.Strings.Lookup(key)
	if ref == 0 {
		return 0, false
	}
	switch v := s.Attributes()[ref].GetValue().(type) {
	case *idx.AnyValue_DoubleValue:
		return v.DoubleValue, true
	case *idx.AnyValue_IntValue:
		return float64(v.IntValue), true
	}
	return 0, false
}

// TestOTLPIdxParity runs the same OTLP requests through today's ReceiveResourceSpans (v04) and
// through otlpidx.Decode, and compares the payload-, chunk- and sampling-level results across a
// configuration matrix. Per-span semantic mapping is covered by otlpidx's own differential test.
func TestOTLPIdxParity(t *testing.T) {
	for _, pc := range parityConfigs {
		for _, res := range parityResources {
			t.Run(pc.name+"/"+res.name, func(t *testing.T) {
				cfg := NewTestConfig(t)
				cfg.Hostname = "fallback-host"
				pc.apply(cfg)
				require.True(t, otlpidx.Supported(cfg))

				req := buildParityRequest(res)
				out := make(chan *Payload, 1)
				rcv := NewOTLPReceiver(out, nil, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
				wantSrc, err := rcv.ReceiveResourceSpans(context.Background(), req.Traces().ResourceSpans().At(0), http.Header{}, nil)
				require.NoError(t, err)
				want := <-out

				raw, err := req.MarshalProto()
				require.NoError(t, err)
				got, err := otlpidx.Decode(context.Background(), raw, otlpidx.Options{Conf: cfg})
				require.NoError(t, err)
				require.Len(t, got, 1)
				tp := got[0].TracerPayload

				assert.Equal(t, wantSrc, got[0].Source, "source")
				assert.Equal(t, want.TracerPayload.Hostname, tp.Hostname(), "hostname")
				assert.Equal(t, want.TracerPayload.Env, tp.Env(), "env")
				assert.Equal(t, want.TracerPayload.ContainerID, tp.ContainerID(), "container ID")
				assert.Equal(t, want.TracerPayload.LanguageName, tp.LanguageName(), "language")
				assert.Equal(t, want.TracerPayload.TracerVersion, tp.TracerVersion(), "tracer version")
				gotCtags, _ := tp.GetAttributeAsString("_dd.tags.container")
				assert.Equal(t, splitTags(want.TracerPayload.Tags["_dd.tags.container"]), splitTags(gotCtags), "container tags")
				gotStats := got[0].StatsComputed != nil && *got[0].StatsComputed
				assert.Equal(t, want.ClientComputedStats, gotStats, "client computed stats (no header)")
				assert.Equal(t, req.Traces().SpanCount(), got[0].SpanCount, "span count")

				wantChunks := make(map[uint64]*pb.TraceChunk)
				for _, c := range want.TracerPayload.Chunks {
					wantChunks[c.Spans[0].TraceID] = c
				}
				require.Len(t, tp.Chunks, len(wantChunks))
				wantSpans := make(map[uint64]*pb.Span)
				for _, c := range want.TracerPayload.Chunks {
					for _, s := range c.Spans {
						wantSpans[s.SpanID] = s
					}
				}
				for _, c := range tp.Chunks {
					wc := wantChunks[transform.OTelTraceIDToUint64([16]byte(c.TraceID))]
					require.NotNil(t, wc)
					assert.Equal(t, wc.Priority, c.Priority, "chunk priority")
					sr, _ := c.GetAttributeAsString("_dd.otlp_sr")
					assert.Equal(t, wc.Tags["_dd.otlp_sr"], sr, "_dd.otlp_sr")
					// _dd.p.dm is set on the first span of a kept chunk; V1 records it as the
					// chunk's sampling mechanism.
					var wantMechanism uint32
					for _, s := range wc.Spans {
						if dm, ok := s.Meta["_dd.p.dm"]; ok {
							wantMechanism = map[string]uint32{"-4": 4, "-9": 9}[dm]
						}
					}
					assert.Equal(t, wantMechanism, c.SamplingMechanism(), "sampling mechanism")

					for _, s := range c.Spans {
						ws := wantSpans[s.SpanID()]
						require.NotNil(t, ws)
						for _, key := range []string{"_top_level", "_dd.measured", "_sample_rate"} {
							wv, wok := ws.Metrics[key]
							gv, gok := numericAttr(tp, s, key)
							assert.Equal(t, wok, gok, "span %d %s presence", s.SpanID(), key)
							assert.Equal(t, wv, gv, "span %d %s", s.SpanID(), key)
						}
						for _, key := range []string{"otel.scope.name", "otel.library.name", "kube_deployment", "datadog.container.tag.team", "k8s.pod.name"} {
							_, wok := ws.Meta[key]
							ref := tp.Strings.Lookup(key)
							_, gok := s.Attributes()[ref]
							assert.Equal(t, wok, gok && ref != 0, "span %d attribute %s presence", s.SpanID(), key)
						}
					}
				}
			})
		}
	}
}

func TestOTLPIdxSampleMatchesReceiver(t *testing.T) {
	for _, rate := range []float64{0, 10, 50, 99, 100} {
		cfg := NewTestConfig(t)
		cfg.OTLPReceiver.ProbabilisticSampling = rate
		rcv := NewOTLPReceiver(make(chan *Payload), nil, cfg, &statsd.NoOpClient{}, &timing.NoopReporter{})
		assert.Equal(t, rcv.samplingRate(), otlpidx.SamplingRate(cfg))
		for tid := uint64(1); tid < 1<<62; tid = tid*3 + 7 {
			assert.Equal(t, rcv.sample(tid), otlpidx.Sample(tid, otlpidx.SamplingRate(cfg)), "rate %v tid %d", rate, tid)
		}
	}
}

func TestOTLPIdxSupported(t *testing.T) {
	cfg := NewTestConfig(t)
	assert.True(t, otlpidx.Supported(cfg))
	for _, legacy := range []func(*config.AgentConfig){
		func(c *config.AgentConfig) { c.Features["disable_receive_resource_spans_v2"] = struct{}{} },
		func(c *config.AgentConfig) { c.Features["disable_operation_and_resource_name_logic_v2"] = struct{}{} },
		func(c *config.AgentConfig) { c.Features["disable_otlp_container_tags_v2"] = struct{}{} },
		func(c *config.AgentConfig) { c.OTLPReceiver.SpanNameAsResourceName = true },
		func(c *config.AgentConfig) { c.OTLPReceiver.SpanNameRemappings = map[string]string{"a": "b"} },
	} {
		cfg := NewTestConfig(t)
		legacy(cfg)
		assert.False(t, otlpidx.Supported(cfg))
	}
}
