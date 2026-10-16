// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otlpidx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/transform"
)

// v04OnlyKeys are Meta keys today's conversion emits that the V1 decoder represents natively
// (span fields, native events and links) rather than as attributes.
var v04OnlyKeys = map[string]bool{
	"span.kind":      true,
	"env":            true,
	"version":        true,
	"events":         true,
	"_dd.span_links": true,
	"_dd.p.dm":       true, // chunk sampling mechanism in V1
	"_dd.otlp_sr":    true, // chunk attribute in V1
}

type semanticsFixture struct {
	name      string
	kind      ptrace.SpanKind
	spanName  string
	status    ptrace.StatusCode
	statusMsg string
	res       map[string]any
	attrs     map[string]any
}

var semanticsFixtures = []semanticsFixture{
	{
		name: "http server", kind: ptrace.SpanKindServer, spanName: "GET /users/{id}",
		attrs: map[string]any{
			"http.request.method": "GET", "http.route": "/users/{id}", "url.path": "/users/42",
			"http.response.status_code": 200, "server.address": "api.internal", "user_agent.original": "curl/8",
			"client.address": "10.0.0.2", "http.request.header.referrer": "https://example.com",
			"http.request.header.x-foo": "bar",
		},
	},
	{
		name: "http client error", kind: ptrace.SpanKindClient, spanName: "POST", status: ptrace.StatusCodeError,
		attrs: map[string]any{
			"http.request.method": "POST", "url.template": "/api/{x}", "url.full": "https://h/api/1",
			"http.response.status_code": 503,
		},
	},
	{
		name: "db client", kind: ptrace.SpanKindClient, spanName: "SELECT",
		attrs: map[string]any{"db.system": "postgresql", "db.statement": "SELECT 1", "db.namespace": "shop"},
	},
	{
		name: "messaging producer", kind: ptrace.SpanKindProducer, spanName: "publish orders",
		attrs: map[string]any{"messaging.system": "kafka", "messaging.operation": "publish", "messaging.destination.name": "orders"},
	},
	{
		name: "grpc server", kind: ptrace.SpanKindServer, spanName: "Greeter/SayHello",
		attrs: map[string]any{"rpc.system": "grpc", "rpc.service": "Greeter", "rpc.method": "SayHello", "rpc.grpc.status_code": 0},
	},
	{
		name: "grpc code from resource", kind: ptrace.SpanKindClient, spanName: "Greeter/SayHello",
		res:   map[string]any{"rpc.grpc.status_code": "2", "http.status_code": "404"},
		attrs: map[string]any{"rpc.system": "grpc"},
	},
	{
		name: "aws client", kind: ptrace.SpanKindClient, spanName: "S3.GetObject",
		attrs: map[string]any{"rpc.system": "aws-api", "rpc.service": "S3", "rpc.method": "GetObject"},
	},
	{
		name: "graphql", kind: ptrace.SpanKindServer, spanName: "query Q",
		attrs: map[string]any{"graphql.operation.type": "query", "graphql.operation.name": "Q"},
	},
	{
		name: "dd overrides", kind: ptrace.SpanKindInternal, spanName: "work",
		attrs: map[string]any{
			"operation.name": "Custom.Op", "resource.name": "custom-res", "span.type": "worker",
			"service.name": "span-svc", "_dd.measured": 1,
		},
	},
	{
		name: "unspecified kind", kind: ptrace.SpanKindUnspecified, spanName: "noop",
	},
	{
		name: "legacy resource keys", kind: ptrace.SpanKindServer, spanName: "handler",
		status: ptrace.StatusCodeError, statusMsg: "failed",
		res:   map[string]any{"deployment.environment": "Staging", "service.version": "2.0"},
		attrs: map[string]any{"error.type": "Timeout"},
	},
	{
		name: "span overrides resource", kind: ptrace.SpanKindServer, spanName: "s",
		res:   map[string]any{"shared": "res", "deployment.environment.name": "res-env"},
		attrs: map[string]any{"shared": "span", "deployment.environment.name": "span-env", "count": 3, "ratio": 0.25, "ok": true},
	},
}

func putAttrs(m pcommon.Map, from map[string]any) {
	for k, v := range from {
		switch x := v.(type) {
		case string:
			m.PutStr(k, x)
		case int:
			m.PutInt(k, int64(x))
		case float64:
			m.PutDouble(k, x)
		case bool:
			m.PutBool(k, x)
		}
	}
}

// TestSemanticsMatchV04 is a differential test: the same OTLP span goes through today's
// transform.OtelSpanToDDSpan and through the direct decoder, and the Datadog-visible results
// must agree. Attribute types may differ (native AnyValue vs Meta strings / Metrics doubles),
// so string attributes are compared by value and numeric attributes against Metrics.
func TestSemanticsMatchV04(t *testing.T) {
	cfg := config.New()
	for _, fx := range semanticsFixtures {
		t.Run(fx.name, func(t *testing.T) {
			td := ptrace.NewTraces()
			rs := td.ResourceSpans().AppendEmpty()
			rs.Resource().Attributes().PutStr("service.name", "res-svc")
			putAttrs(rs.Resource().Attributes(), fx.res)
			ss := rs.ScopeSpans().AppendEmpty()
			ss.Scope().SetName("my.lib")
			ss.Scope().SetVersion("0.1")
			span := ss.Spans().AppendEmpty()
			span.SetTraceID(traceA)
			span.SetSpanID(pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, 7})
			span.SetName(fx.spanName)
			span.SetKind(fx.kind)
			span.SetStartTimestamp(1000)
			span.SetEndTimestamp(2000)
			span.Status().SetCode(fx.status)
			span.Status().SetMessage(fx.statusMsg)
			putAttrs(span.Attributes(), fx.attrs)

			want := transform.OtelSpanToDDSpan(span, rs.Resource(), ss.Scope(), cfg)

			b, err := ptraceotlp.NewExportRequestFromTraces(td).MarshalProto()
			require.NoError(t, err)
			for _, opts := range []Options{{}, {ResourceAttributesOnPayload: true}} {
				tps, err := DecodeExportRequestWithOptions(b, opts)
				require.NoError(t, err)
				assertMatchesV04(t, want, tps[0], tps[0].Chunks[0].Spans[0])
			}
		})
	}
}

// TestSemanticsMatchV04MultiSpan decodes the fixtures without resource attributes as spans of a
// single payload, so per-payload decoder state carried from span to span is exercised.
func TestSemanticsMatchV04MultiSpan(t *testing.T) {
	cfg := config.New()
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "res-svc")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("my.lib")
	var names []string
	for i, fx := range semanticsFixtures {
		if fx.res != nil {
			continue
		}
		span := ss.Spans().AppendEmpty()
		span.SetTraceID(traceA)
		span.SetSpanID(pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, byte(i + 1)})
		span.SetName(fx.spanName)
		span.SetKind(fx.kind)
		span.SetStartTimestamp(1000)
		span.SetEndTimestamp(2000)
		span.Status().SetCode(fx.status)
		span.Status().SetMessage(fx.statusMsg)
		putAttrs(span.Attributes(), fx.attrs)
		names = append(names, fx.name)
	}
	b, err := ptraceotlp.NewExportRequestFromTraces(td).MarshalProto()
	require.NoError(t, err)
	tps, err := DecodeExportRequest(b)
	require.NoError(t, err)
	tp := tps[0]
	require.Len(t, tp.Chunks[0].Spans, len(names))
	for i, name := range names {
		t.Run(name, func(t *testing.T) {
			want := transform.OtelSpanToDDSpan(ss.Spans().At(i), rs.Resource(), ss.Scope(), cfg)
			assertMatchesV04(t, want, tp, tp.Chunks[0].Spans[i])
		})
	}
}

// assertMatchesV04 checks that a decoded span carries the same Datadog-visible values as want,
// the result of today's conversion.
func assertMatchesV04(t *testing.T, want *pb.Span, tp *idx.InternalTracerPayload, got *idx.InternalSpan) {
	t.Helper()
	assert.Equal(t, want.Service, got.Service(), "service")
	assert.Equal(t, want.Name, got.Name(), "name")
	assert.Equal(t, want.Resource, got.Resource(), "resource")
	assert.Equal(t, want.Type, got.Type(), "type")
	assert.Equal(t, want.Error == 1, got.Error(), "error")
	assert.Equal(t, want.Meta["env"], got.Env(), "env")
	assert.Equal(t, want.Meta["version"], got.Version(), "version")

	// Effective attributes: span first, then payload (resource) attributes.
	effective := func(key string) (*idx.AnyValue, bool) {
		ref := tp.Strings.Lookup(key)
		if ref == 0 {
			return nil, false
		}
		if v, ok := got.Attributes()[ref]; ok {
			return v, true
		}
		v, ok := tp.Attributes[ref]
		return v, ok
	}
	for k, wv := range want.Meta {
		if v04OnlyKeys[k] {
			continue
		}
		v, ok := effective(k)
		if !assert.True(t, ok, "missing meta key %q", k) {
			continue
		}
		if s, isStr := v.Value.(*idx.AnyValue_StringValueRef); isStr {
			assert.Equal(t, wv, tp.Strings.Get(s.StringValueRef), "meta %q", k)
		}
	}
	for k, wv := range want.Metrics {
		v, ok := effective(k)
		if !assert.True(t, ok, "missing metric key %q", k) {
			continue
		}
		switch x := v.Value.(type) {
		case *idx.AnyValue_IntValue:
			assert.Equal(t, wv, float64(x.IntValue), "metric %q", k)
		case *idx.AnyValue_DoubleValue:
			assert.Equal(t, wv, x.DoubleValue, "metric %q", k)
		default:
			t.Errorf("metric %q has non-numeric type %T", k, v.Value)
		}
	}
	// No extra keys beyond what today's conversion emits.
	check := func(attrs map[uint32]*idx.AnyValue, level string) {
		for ref := range attrs {
			k := tp.Strings.Get(ref)
			_, inMeta := want.Meta[k]
			_, inMetrics := want.Metrics[k]
			assert.True(t, inMeta || inMetrics, "unexpected %s attribute %q", level, k)
		}
	}
	check(got.Attributes(), "span")
	check(tp.Attributes, "payload")
}
