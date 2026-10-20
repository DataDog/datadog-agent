// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otlpidx

import (
	"strconv"
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
	"component":      true,
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
	scope     map[string]any
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
		// Status2Error's HTTP status code fallback reads span attributes only.
		name: "error status with resource status code", kind: ptrace.SpanKindClient, spanName: "GET", status: ptrace.StatusCodeError,
		res: map[string]any{"http.status_code": "503"},
	},
	{
		name: "error status with string span status code", kind: ptrace.SpanKindClient, spanName: "GET", status: ptrace.StatusCodeError,
		attrs: map[string]any{"http.status_code": "503"},
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
		name: "unknown kind", kind: ptrace.SpanKind(42), spanName: "noop",
	},
	{
		name: "sampling.priority numeric", kind: ptrace.SpanKindServer, spanName: "s",
		attrs: map[string]any{"sampling.priority": 2},
	},
	{
		name: "sampling.priority string", kind: ptrace.SpanKindServer, spanName: "s",
		attrs: map[string]any{"sampling.priority": "2"},
	},
	{
		name: "sampling.priority from resource stays an attribute", kind: ptrace.SpanKindServer, spanName: "s",
		res: map[string]any{"sampling.priority": 2},
	},
	{
		name: "analytics.event string", kind: ptrace.SpanKindServer, spanName: "s",
		attrs: map[string]any{"analytics.event": "true"},
	},
	{
		name: "analytics.event bool false", kind: ptrace.SpanKindServer, spanName: "s",
		attrs: map[string]any{"analytics.event": false},
	},
	{
		name: "analytics.event invalid", kind: ptrace.SpanKindServer, spanName: "s",
		attrs: map[string]any{"analytics.event": "maybe"},
	},
	{
		name: "analytics.event numeric", kind: ptrace.SpanKindServer, spanName: "s",
		attrs: map[string]any{"analytics.event": 1},
	},
	{
		name: "analytics.event from resource", kind: ptrace.SpanKindServer, spanName: "s",
		res: map[string]any{"analytics.event": "true"},
	},
	{
		name: "analytics.event span wins over resource", kind: ptrace.SpanKindServer, spanName: "s",
		res:   map[string]any{"analytics.event": "true"},
		attrs: map[string]any{"analytics.event": "false"},
	},
	{
		name: "analytics.event invalid span falls back to resource", kind: ptrace.SpanKindServer, spanName: "s",
		res:   map[string]any{"analytics.event": "false"},
		attrs: map[string]any{"analytics.event": "maybe"},
	},
	{
		name: "analytics.event from scope stays an attribute", kind: ptrace.SpanKindServer, spanName: "s",
		scope: map[string]any{"analytics.event": "true"},
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
	{
		name: "composite span attributes", kind: ptrace.SpanKindServer, spanName: "s",
		attrs: map[string]any{
			"process.command_args": []any{"/app", "--port=8080", 3},
			"nested":               map[string]any{"inner": "v", "n": 1, "deeper": map[string]any{"x": true}},
			"payload":              []byte{0xde, 0xad, 0xbe, 0xef},
			"empty.list":           []any{},
			"flag":                 false,
		},
	},
	{
		name: "typed resource attributes", kind: ptrace.SpanKindServer, spanName: "s",
		res: map[string]any{
			"process.pid": 42, "host.cpu.ratio": 0.5, "res.flag": true,
			"process.command_args": []any{"/app", "-v"}, "res.map": map[string]any{"a": "b"},
			"http.request.method": "GET",
		},
		attrs: map[string]any{"count": 3},
	},
	{
		name: "db.name from resource wins over db.namespace", kind: ptrace.SpanKindClient, spanName: "q",
		res:   map[string]any{"db.name": "res-db"},
		attrs: map[string]any{"db.system": "postgresql", "db.namespace": "span-ns"},
	},
	{
		name: "scope attributes overwrite", kind: ptrace.SpanKindServer, spanName: "s",
		status: ptrace.StatusCodeError, statusMsg: "failed",
		res:   map[string]any{"res.shared": "res"},
		attrs: map[string]any{"shared": "span", "count": 3, "label": "span-label"},
		scope: map[string]any{
			"scope.attr": "v", "shared": "scope", "res.shared": "scope", "error.msg": "scope-err",
			"count": 5, "label": 7, "ratio": 0.5, "flag": true, "list": []any{"a", 1},
		},
	},
	{
		name: "scope db.name wins over db.namespace", kind: ptrace.SpanKindClient, spanName: "q",
		attrs: map[string]any{"db.system": "postgresql", "db.namespace": "span-ns"},
		scope: map[string]any{"db.name": "scope-db"},
	},
	{
		name: "scope promoted keys", kind: ptrace.SpanKindServer, spanName: "s",
		res:   map[string]any{"deployment.environment.name": "res-env", "component": "res-comp"},
		attrs: map[string]any{"env": "span-env", "component": "span-comp", "version": "span-ver"},
		scope: map[string]any{"env": "scope-env", "version": 7, "component": "scope-comp", "span.kind": "weird"},
	},
	{
		name: "scope span.kind", kind: ptrace.SpanKindServer, spanName: "s",
		scope: map[string]any{"span.kind": "consumer"},
	},
	{
		name: "span promoted keys", kind: ptrace.SpanKindServer, spanName: "s",
		res: map[string]any{
			"deployment.environment.name": "dep-env", "env": "res-env", "version": "res-ver", "component": "res-comp",
		},
		attrs: map[string]any{"env": "span-env", "component": "span-comp", "span.kind": "client", "version": 3},
	},
	{
		name: "resource promoted keys", kind: ptrace.SpanKindClient, spanName: "s",
		res: map[string]any{
			"env": "res-env", "version": "res-ver", "service.version": "1.0", "component": "res-comp", "span.kind": "server",
		},
	},
}

func putAttrs(m pcommon.Map, from map[string]any) {
	for k, v := range from {
		putValue(m.PutEmpty(k), v)
	}
}

func putValue(dst pcommon.Value, v any) {
	switch x := v.(type) {
	case string:
		dst.SetStr(x)
	case int:
		dst.SetInt(int64(x))
	case float64:
		dst.SetDouble(x)
	case bool:
		dst.SetBool(x)
	case []byte:
		dst.SetEmptyBytes().FromRaw(x)
	case []any:
		s := dst.SetEmptySlice()
		for _, e := range x {
			putValue(s.AppendEmpty(), e)
		}
	case map[string]any:
		putAttrs(dst.SetEmptyMap(), x)
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
			putAttrs(ss.Scope().Attributes(), fx.scope)
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
				tps, err := decodeForTest(b, opts)
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
		if fx.res != nil || fx.scope != nil {
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
	tps, err := decodeForTest(b)
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

// TestSamplingPriorityAttribute checks that a numeric sampling.priority span attribute sets the
// chunk's user priority, as today's conversion does through the _sampling_priority_v1 metric.
func TestSamplingPriorityAttribute(t *testing.T) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "svc")
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(traceA)
	span.SetSpanID(pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, 1})
	span.Attributes().PutInt("sampling.priority", 2)
	b, err := ptraceotlp.NewExportRequestFromTraces(td).MarshalProto()
	require.NoError(t, err)
	tps, err := decodeForTest(b)
	require.NoError(t, err)
	c := tps[0].Chunks[0]
	assert.Equal(t, int32(2), c.Priority)
	assert.Equal(t, uint32(samplingMechanismManual), c.SamplingMechanism())
}

// TestSamplingPriorityDDAttribute checks that a _sampling_priority_v1 span attribute sets the user
// priority whether it is an int or a double, as today's lookup over the span's Metrics does, and
// that a resource attribute doesn't, as today's resource attributes are only Meta.
func TestSamplingPriorityDDAttribute(t *testing.T) {
	for _, tc := range []struct {
		name         string
		span, res    any
		wantPriority int32
	}{
		{name: "int span attribute", span: 2, wantPriority: 2},
		{name: "double span attribute", span: 2.0, wantPriority: 2},
		{name: "resource attribute", res: 2.0, wantPriority: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			td := ptrace.NewTraces()
			rs := td.ResourceSpans().AppendEmpty()
			rs.Resource().Attributes().PutStr("service.name", "svc")
			if tc.res != nil {
				putValue(rs.Resource().Attributes().PutEmpty("_sampling_priority_v1"), tc.res)
			}
			span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
			span.SetTraceID(traceA)
			span.SetSpanID(pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, 1})
			if tc.span != nil {
				putValue(span.Attributes().PutEmpty("_sampling_priority_v1"), tc.span)
			}
			b, err := ptraceotlp.NewExportRequestFromTraces(td).MarshalProto()
			require.NoError(t, err)
			tps, err := decodeForTest(b)
			require.NoError(t, err)
			assert.Equal(t, tc.wantPriority, tps[0].Chunks[0].Priority)
		})
	}
}

// TestPromotedSpanFields checks that the payload and chunk fields today's Process fills in from
// the spans are set by the decoder, as ProcessV1 expects: env and app version from the root span
// first, the git commit SHA from any span, and origin and APM mode from the root span only.
func TestPromotedSpanFields(t *testing.T) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "svc")
	spans := rs.ScopeSpans().AppendEmpty().Spans()
	root := spans.AppendEmpty()
	root.SetTraceID(traceA)
	root.SetSpanID(pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, 1})
	root.Attributes().PutStr("deployment.environment.name", "staging")
	root.Attributes().PutStr("_dd.origin", "synthetics")
	root.Attributes().PutStr("_dd.apm_mode", "edge")
	child := spans.AppendEmpty()
	child.SetTraceID(traceA)
	child.SetSpanID(pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, 2})
	child.SetParentSpanID(pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, 1})
	child.Attributes().PutStr("service.version", "2.0")
	child.Attributes().PutStr("_dd.git.commit.sha", "abc123")
	child.Attributes().PutStr("_dd.origin", "not-root")
	b, err := ptraceotlp.NewExportRequestFromTraces(td).MarshalProto()
	require.NoError(t, err)
	tps, err := decodeForTest(b)
	require.NoError(t, err)
	tp := tps[0]
	assert.Equal(t, "staging", tp.Env())
	assert.Equal(t, "2.0", tp.AppVersion())
	sha, _ := tp.GetAttributeAsString("_dd.git.commit.sha")
	assert.Equal(t, "abc123", sha)
	mode, _ := tp.GetAttributeAsString("_dd.apm_mode")
	assert.Equal(t, "edge", mode)
	assert.Equal(t, "synthetics", tp.Chunks[0].Origin())
}

// TestScopeAttributesNative checks that scope attributes keep their native type, apply to every
// span of their scope only, and overwrite span attributes and promoted span fields.
func TestScopeAttributesNative(t *testing.T) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "svc")
	ss := rs.ScopeSpans().AppendEmpty()
	putAttrs(ss.Scope().Attributes(), map[string]any{
		"count": 5, "ratio": 0.5, "flag": true, "list": []any{"a"}, "component": "scope-comp",
	})
	span := ss.Spans().AppendEmpty()
	span.SetTraceID(traceA)
	span.SetSpanID(pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, 1})
	putAttrs(span.Attributes(), map[string]any{"count": "span", "component": "span-comp"})
	other := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	other.SetTraceID(traceA)
	other.SetSpanID(pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, 2})

	b, err := ptraceotlp.NewExportRequestFromTraces(td).MarshalProto()
	require.NoError(t, err)
	tps, err := decodeForTest(b)
	require.NoError(t, err)
	spans := tps[0].Chunks[0].Spans
	require.Len(t, spans, 2)

	// InternalSpan.GetAttribute reports any key in the string table as present, so read the map.
	has := func(s *idx.InternalSpan, k string) (*idx.AnyValue, bool) {
		v, ok := s.Attributes()[tps[0].Strings.Lookup(k)]
		return v, ok
	}
	got := spans[0]
	attr := func(k string) any {
		v, ok := has(got, k)
		require.True(t, ok, k)
		return v.Value
	}
	assert.Equal(t, int64(5), attr("count").(*idx.AnyValue_IntValue).IntValue)
	assert.Equal(t, 0.5, attr("ratio").(*idx.AnyValue_DoubleValue).DoubleValue)
	assert.True(t, attr("flag").(*idx.AnyValue_BoolValue).BoolValue)
	assert.Len(t, attr("list").(*idx.AnyValue_ArrayValue).ArrayValue.Values, 1)
	assert.Equal(t, "scope-comp", got.Component())
	_, ok := has(got, "component")
	assert.False(t, ok, "promoted key kept as attribute")

	for _, k := range []string{"count", "ratio", "flag", "list"} {
		_, ok := has(spans[1], k)
		assert.False(t, ok, "scope attribute %q leaked to another scope", k)
	}
	assert.Empty(t, spans[1].Component())
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
	assert.Equal(t, want.Meta["component"], got.Component(), "component")
	// Like ConvertToIdx, "unspecified" (and any unknown name) maps to internal.
	assert.Equal(t, spanKindFromName(want.Meta["span.kind"]), got.Kind(), "span.kind %q", want.Meta["span.kind"])

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
	// A scope attribute overwrites a span attribute of the same key in Meta, but v04 keeps the
	// span's numeric value in Metrics too. V1 has one value, which must match one of them.
	for k, wv := range want.Meta {
		if v04OnlyKeys[k] {
			continue
		}
		v, ok := effective(k)
		if !assert.True(t, ok, "missing meta key %q", k) {
			continue
		}
		if wm, inMetrics := want.Metrics[k]; inMetrics && matchesMetric(v, wm) {
			continue
		}
		// Arrays and maps from scope attributes stay native, while v04 has their JSON string.
		if s, ok := scalarString(tp, v); ok {
			assert.Equal(t, wv, s, "meta %q", k)
		}
	}
	for k, wv := range want.Metrics {
		// Like ConvertToIdx, numeric promoted keys are dropped.
		if promotedKey(k) != 0 {
			continue
		}
		v, ok := effective(k)
		if !assert.True(t, ok, "missing metric key %q", k) {
			continue
		}
		if wm, inMeta := want.Meta[k]; inMeta {
			if s, ok := scalarString(tp, v); ok && s == wm {
				continue
			}
		}
		assert.True(t, matchesMetric(v, wv), "metric %q: want %v, got %v", k, wv, v.Value)
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

// matchesMetric reports whether v is numeric and equal to want.
func matchesMetric(v *idx.AnyValue, want float64) bool {
	switch x := v.Value.(type) {
	case *idx.AnyValue_IntValue:
		return float64(x.IntValue) == want
	case *idx.AnyValue_DoubleValue:
		return x.DoubleValue == want
	}
	return false
}

// scalarString formats a string, bool or numeric attribute like pcommon.Value.AsString, the form
// v04 stores Meta values in.
func scalarString(tp *idx.InternalTracerPayload, v *idx.AnyValue) (string, bool) {
	switch x := v.Value.(type) {
	case *idx.AnyValue_StringValueRef:
		return tp.Strings.Get(x.StringValueRef), true
	case *idx.AnyValue_BoolValue:
		return strconv.FormatBool(x.BoolValue), true
	case *idx.AnyValue_IntValue:
		return strconv.FormatInt(x.IntValue, 10), true
	case *idx.AnyValue_DoubleValue:
		return strconv.FormatFloat(x.DoubleValue, 'f', -1, 64), true
	}
	return "", false
}
