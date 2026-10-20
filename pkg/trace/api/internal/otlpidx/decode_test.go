// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otlpidx

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/protobuf/encoding/protowire"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

var (
	traceA = pcommon.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	traceB = pcommon.TraceID{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
)

// newTestConf returns a default trace-agent config with an attributes translator, as the OTLP
// receiver has.
func newTestConf() *config.AgentConfig {
	conf := config.New()
	tr, err := attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
	if err != nil {
		panic(err)
	}
	conf.OTLPReceiver.AttributesTranslator = tr
	return conf
}

var testConf = sync.OnceValue(newTestConf)

// decodeForTest decodes b with opts (the default test config unless opts sets Conf) and returns
// the tracer payloads.
func decodeForTest(b []byte, opts ...Options) ([]*idx.InternalTracerPayload, error) {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.Conf == nil {
		o.Conf = testConf()
	}
	ps, err := Decode(context.Background(), b, o)
	if err != nil {
		return nil, err
	}
	tps := make([]*idx.InternalTracerPayload, len(ps))
	for i, p := range ps {
		tps[i] = p.TracerPayload
	}
	return tps, nil
}

func buildRequest(t testing.TB) []byte {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "svc")
	rs.Resource().Attributes().PutStr("deployment.environment.name", "prod")
	rs.Resource().Attributes().PutStr("telemetry.sdk.language", "go")
	rs.Resource().Attributes().PutStr("telemetry.sdk.version", "1.30.0")
	rs.Resource().Attributes().PutStr("shared", "from-resource")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("lib")
	ss.Scope().SetVersion("1.2.3")

	s1 := ss.Spans().AppendEmpty()
	s1.SetTraceID(traceA)
	s1.SetSpanID(pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, 1})
	s1.SetName("GET /users")
	s1.SetKind(ptrace.SpanKindServer)
	s1.SetStartTimestamp(100)
	s1.SetEndTimestamp(250)
	s1.TraceState().FromRaw("dd=s:1")
	s1.Status().SetCode(ptrace.StatusCodeError)
	s1.Status().SetMessage("boom")
	attrs := s1.Attributes()
	attrs.PutStr("shared", "from-span")
	attrs.PutInt("http.response.status_code", 500)
	attrs.PutDouble("ratio", 0.5)
	attrs.PutBool("flag", true)
	attrs.PutEmptyBytes("raw").FromRaw([]byte{0xde, 0xad})
	arr := attrs.PutEmptySlice("list")
	arr.AppendEmpty().SetStr("a")
	arr.AppendEmpty().SetInt(7)
	m := attrs.PutEmptyMap("nested")
	m.PutStr("inner", "v")
	ev := s1.Events().AppendEmpty()
	ev.SetName("exception")
	ev.SetTimestamp(200)
	ev.Attributes().PutStr("exception.message", "bad")
	l := s1.Links().AppendEmpty()
	l.SetTraceID(traceB)
	l.SetSpanID(pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, 9})
	l.TraceState().FromRaw("k=v")
	l.SetFlags(1)
	l.Attributes().PutStr("link.attr", "x")

	s2 := ss.Spans().AppendEmpty()
	s2.SetTraceID(traceB)
	s2.SetSpanID(pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, 2})
	s2.SetParentSpanID(pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, 3})
	s2.SetName("child")
	s2.SetKind(ptrace.SpanKindClient)
	s2.Attributes().PutDouble("_sampling_priority_v1", 2)

	s3 := ss.Spans().AppendEmpty()
	s3.SetTraceID(traceA)
	s3.SetSpanID(pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, 4})
	s3.SetName("other")

	b, err := ptraceotlp.NewExportRequestFromTraces(td).MarshalProto()
	require.NoError(t, err)
	return b
}

func attr(t *testing.T, tp *idx.InternalTracerPayload, attrs map[uint32]*idx.AnyValue, key string) *idx.AnyValue {
	t.Helper()
	v, ok := attrs[tp.Strings.Lookup(key)]
	require.True(t, ok, "missing attribute %q", key)
	return v
}

func hasAttr(tp *idx.InternalTracerPayload, attrs map[uint32]*idx.AnyValue, key string) bool {
	ref := tp.Strings.Lookup(key)
	if ref == 0 {
		return false
	}
	_, ok := attrs[ref]
	return ok
}

func str(tp *idx.InternalTracerPayload, v *idx.AnyValue) string {
	return tp.Strings.Get(v.GetStringValueRef())
}

func TestDecodeExportRequest(t *testing.T) {
	tps, err := decodeForTest(buildRequest(t))
	require.NoError(t, err)
	require.Len(t, tps, 1)
	tp := tps[0]
	assert.Equal(t, "prod", tp.Env())
	assert.Equal(t, "go", tp.LanguageName())
	assert.Equal(t, "otlp-1.30.0", tp.TracerVersion())

	// Resource attributes are copied into spans by default (see TestResourceAttributesCopiedToSpans).
	assert.Empty(t, tp.Attributes)

	require.Len(t, tp.Chunks, 2)
	chunkA, chunkB := tp.Chunks[0], tp.Chunks[1]
	assert.Equal(t, traceA[:], chunkA.TraceID)
	assert.Equal(t, traceB[:], chunkB.TraceID)
	require.Len(t, chunkA.Spans, 2)
	require.Len(t, chunkB.Spans, 1)
	assert.Equal(t, int32(1), chunkA.Priority)
	assert.Equal(t, uint32(9), chunkA.SamplingMechanism())
	v, ok := chunkA.GetAttributeAsString("_dd.otlp_sr")
	assert.True(t, ok)
	assert.Equal(t, "1.00", v)
	assert.Equal(t, int32(2), chunkB.Priority, "user-set sampling priority")
	assert.Equal(t, uint32(4), chunkB.SamplingMechanism())

	s1 := chunkA.Spans[0]
	assert.Equal(t, uint64(1), s1.SpanID())
	assert.Equal(t, "svc", s1.Service())
	assert.Equal(t, "server.request", s1.Name())
	assert.Equal(t, "GET /users", s1.Resource())
	assert.Equal(t, "web", s1.Type())
	assert.Equal(t, "prod", s1.Env())
	assert.Equal(t, idx.SpanKind_SPAN_KIND_SERVER, s1.Kind())
	assert.Equal(t, uint64(100), s1.Start())
	assert.Equal(t, uint64(150), s1.Duration())
	assert.True(t, s1.Error())

	ps := s1.Attributes()
	assert.Equal(t, "from-span", str(tp, attr(t, tp, ps, "shared")), "span attributes take precedence over resource")
	assert.Equal(t, int64(500), attr(t, tp, ps, "http.status_code").GetIntValue())
	assert.False(t, hasAttr(tp, ps, "http.response.status_code"), "OTel key is renamed to the DD key")
	assert.Equal(t, 0.5, attr(t, tp, ps, "ratio").GetDoubleValue())
	assert.True(t, attr(t, tp, ps, "flag").GetBoolValue())
	// Bytes, arrays and maps are stored as strings, like today's conversion (pcommon AsString).
	assert.Equal(t, "3q0=", str(tp, attr(t, tp, ps, "raw")))
	assert.Equal(t, `["a",7]`, str(tp, attr(t, tp, ps, "list")))
	assert.Equal(t, `{"inner":"v"}`, str(tp, attr(t, tp, ps, "nested")))
	assert.Equal(t, "dd=s:1", str(tp, attr(t, tp, ps, "w3c.tracestate")))
	assert.Equal(t, "Error", str(tp, attr(t, tp, ps, "otel.status_code")))
	assert.Equal(t, "boom", str(tp, attr(t, tp, ps, "otel.status_description")))
	assert.Equal(t, "boom", str(tp, attr(t, tp, ps, "error.msg")))
	assert.Equal(t, "lib", str(tp, attr(t, tp, ps, "otel.scope.name")))
	assert.Equal(t, "lib", str(tp, attr(t, tp, ps, "otel.library.name")))
	assert.Equal(t, "1.2.3", str(tp, attr(t, tp, ps, "otel.library.version")))
	assert.Equal(t, "0102030405060708090a0b0c0d0e0f10", str(tp, attr(t, tp, ps, "otel.trace_id")))
	assert.Equal(t, "true", str(tp, attr(t, tp, ps, "_dd.span_events.has_exception")))

	events := s1.Events()
	require.Len(t, events, 1)
	assert.Equal(t, uint64(200), events[0].Time())
	assert.Equal(t, "exception", events[0].Name())

	links := s1.Links()
	require.Len(t, links, 1)
	assert.Equal(t, traceB[:], links[0].TraceID())
	assert.Equal(t, uint64(9), links[0].SpanID())
	assert.Equal(t, "k=v", links[0].Tracestate())
	assert.Equal(t, uint32(1), links[0].Flags())

	s2 := chunkB.Spans[0]
	assert.Equal(t, uint64(3), s2.ParentID())
	assert.Equal(t, idx.SpanKind_SPAN_KIND_CLIENT, s2.Kind())
	assert.Equal(t, "client.request", s2.Name())
	assert.Equal(t, "http", s2.Type())
	assert.False(t, s2.Error())
}

func TestDecodeSkipSemantics(t *testing.T) {
	tps, err := decodeForTest(buildRequest(t), Options{SkipSemantics: true})
	require.NoError(t, err)
	s1 := tps[0].Chunks[0].Spans[0]
	assert.Equal(t, "svc", s1.Service())
	assert.Equal(t, "GET /users", s1.Name())
	assert.Equal(t, "GET /users", s1.Resource())
}

// TestDecodeOutOfOrderFields checks that a ResourceSpans whose resource is encoded after its
// scope spans still applies the resource attributes to every span.
func TestDecodeOutOfOrderFields(t *testing.T) {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "late-svc")
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(traceA)
	span.SetSpanID(pcommon.SpanID{1})

	resource, err := ptraceotlp.NewExportRequestFromTraces(td).MarshalProto()
	require.NoError(t, err)
	// Re-encode the single ResourceSpans with its fields reversed.
	_, _, n := protowire.ConsumeTag(resource)
	rsBytes, _ := protowire.ConsumeBytes(resource[n:])
	var fields [][]byte
	for b := rsBytes; len(b) > 0; {
		_, _, tn := protowire.ConsumeTag(b)
		_, vn := protowire.ConsumeBytes(b[tn:])
		fields = append(fields, b[:tn+vn])
		b = b[tn+vn:]
	}
	var reversed []byte
	for i := len(fields) - 1; i >= 0; i-- {
		reversed = append(reversed, fields[i]...)
	}
	req := protowire.AppendTag(nil, fExportResourceSpans, protowire.BytesType)
	req = protowire.AppendBytes(req, reversed)

	tps, err := decodeForTest(req)
	require.NoError(t, err)
	require.Len(t, tps, 1)
	require.Len(t, tps[0].Chunks, 1)
	assert.Equal(t, "late-svc", tps[0].Chunks[0].Spans[0].Service())
}

// TestDecodeLengthBoundaries covers attribute values around the lengths where the fast field
// decoding path (one- and two-byte length prefixes) hands over to the generic one.
func TestDecodeLengthBoundaries(t *testing.T) {
	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(traceA)
	span.SetSpanID(pcommon.SpanID{1})
	lengths := []int{0, 1, 120, 127, 128, 300, 16380, 16383, 16384, 20000}
	for _, n := range lengths {
		span.Attributes().PutStr(fmt.Sprintf("k%d", n), strings.Repeat("v", n))
		span.Attributes().PutInt(fmt.Sprintf("i%d", n), int64(n))
	}
	b, err := ptraceotlp.NewExportRequestFromTraces(td).MarshalProto()
	require.NoError(t, err)
	tps, err := decodeForTest(b)
	require.NoError(t, err)
	tp := tps[0]
	attrs := tp.Chunks[0].Spans[0].Attributes()
	for _, n := range lengths {
		assert.Equal(t, strings.Repeat("v", n), str(tp, attr(t, tp, attrs, fmt.Sprintf("k%d", n))))
		assert.Equal(t, int64(n), attr(t, tp, attrs, fmt.Sprintf("i%d", n)).GetIntValue())
	}
}

func TestDecodeMalformed(t *testing.T) {
	b := buildRequest(t)
	for i := 1; i < len(b); i++ {
		// Truncated input must error or decode cleanly, never panic.
		_, _ = decodeForTest(b[:i])
	}
	_, err := decodeForTest([]byte{0x0a, 0xff})
	assert.Error(t, err)
}

// TestResourceAttributesCopiedToSpans covers the default per-span copy mode and guards against
// handing the same *AnyValue to several spans: string table compaction and attribute mapping
// mutate values in place.
func TestResourceAttributesCopiedToSpans(t *testing.T) {
	tps, err := decodeForTest(buildRequest(t))
	require.NoError(t, err)
	tp := tps[0]
	assert.Empty(t, tp.Attributes)
	s1, s2 := tp.Chunks[0].Spans[0], tp.Chunks[1].Spans[0]
	assert.Equal(t, "from-span", str(tp, attr(t, tp, s1.Attributes(), "shared")))
	assert.Equal(t, "from-resource", str(tp, attr(t, tp, s2.Attributes(), "shared")))
	assert.False(t, hasAttr(tp, s2.Attributes(), "service.name"), "DD convention keys are dropped")
	a := attr(t, tp, tp.Chunks[0].Spans[1].Attributes(), "deployment.environment.name")
	b := attr(t, tp, s2.Attributes(), "deployment.environment.name")
	assert.Equal(t, "prod", str(tp, a))
	assert.NotSame(t, a, b)
	assert.NotSame(t, a.GetValue(), b.GetValue())
	assert.Equal(t, "svc", s2.Service())
}

func TestResourceAttributesOnPayload(t *testing.T) {
	tps, err := decodeForTest(buildRequest(t), Options{ResourceAttributesOnPayload: true})
	require.NoError(t, err)
	tp := tps[0]
	assert.Equal(t, "from-resource", str(tp, attr(t, tp, tp.Attributes, "shared")))
	assert.False(t, hasAttr(tp, tp.Attributes, "service.name"), "DD convention keys are dropped")
	s1, s2 := tp.Chunks[0].Spans[0], tp.Chunks[1].Spans[0]
	assert.Equal(t, "from-span", str(tp, attr(t, tp, s1.Attributes(), "shared")))
	assert.False(t, hasAttr(tp, s2.Attributes(), "shared"), "resource attributes are not copied into spans")
	assert.False(t, hasAttr(tp, s2.Attributes(), "deployment.environment.name"))
	assert.Equal(t, "svc", s2.Service())
	assert.Equal(t, "prod", s2.Env())
}

// pbBytes encodes a length-delimited field.
func pbBytes(num protowire.Number, parts ...[]byte) []byte {
	b := protowire.AppendTag(nil, num, protowire.BytesType)
	return protowire.AppendBytes(b, slices.Concat(parts...))
}

// pbVarint encodes a varint field.
func pbVarint(num protowire.Number, v uint64) []byte {
	return protowire.AppendVarint(protowire.AppendTag(nil, num, protowire.VarintType), v)
}

// TestDecodeStrict checks that the decoder rejects what pdata rejects (wire types, ID lengths,
// field 0), as well as repeated singular messages, which protobuf merges and the decoder leaves
// to the pdata fallback, and that it reads the deprecated instrumentation_library_spans when
// scope_spans is absent, as pdata does.
func TestDecodeStrict(t *testing.T) {
	str := func(s string) []byte { return []byte(s) }
	kv := pbBytes(fResourceAttributes, pbBytes(fKeyValueKey, str("service.name")), pbBytes(fKeyValueValue, pbBytes(fAnyString, str("svc"))))
	resource := pbBytes(fResourceSpansResource, kv)
	span := func(fields ...[]byte) []byte {
		return pbBytes(fScopeSpansSpans, slices.Concat(append([][]byte{
			pbBytes(fSpanTraceID, traceA[:]), pbBytes(fSpanSpanID, []byte{0, 0, 0, 0, 0, 0, 0, 1}), pbBytes(fSpanName, str("op")),
		}, fields...)...))
	}
	scopeSpans := pbBytes(fResourceSpansScopeSpans, span())
	request := func(rs ...[]byte) []byte { return pbBytes(fExportResourceSpans, rs...) }

	for _, tc := range []struct {
		name string
		req  []byte
	}{
		{"varint where a message is expected", pbVarint(fExportResourceSpans, 1)},
		{"varint span name", request(resource, pbBytes(fResourceSpansScopeSpans, span(pbVarint(fSpanName, 1))))},
		{"bytes span kind", request(resource, pbBytes(fResourceSpansScopeSpans, span(pbBytes(fSpanKind, str("x")))))},
		{"varint dropped count ignored by the decoder", request(resource, pbBytes(fResourceSpansScopeSpans, span(pbBytes(10, str("x")))))},
		{"7-byte span ID", request(resource, pbBytes(fResourceSpansScopeSpans, span(pbBytes(fSpanSpanID, make([]byte, 7)))))},
		{"9-byte link span ID", request(resource, pbBytes(fResourceSpansScopeSpans, span(pbBytes(fSpanLinks, pbBytes(fLinkSpanID, make([]byte, 9))))))},
		{"field 0", request(resource, scopeSpans, pbBytes(0, str("x")))},
		{"repeated resource", request(resource, resource, scopeSpans)},
		{"repeated scope", request(resource, pbBytes(fResourceSpansScopeSpans, pbBytes(fScopeSpansScope), pbBytes(fScopeSpansScope), span()))},
		{"repeated attribute value", request(pbBytes(fResourceSpansResource, pbBytes(fResourceAttributes, pbBytes(fKeyValueKey, str("k")),
			pbBytes(fKeyValueValue, pbBytes(fAnyString, str("a"))), pbBytes(fKeyValueValue, pbBytes(fAnyString, str("b"))))), scopeSpans)},
		{"scope_spans and deprecated spans", request(resource, scopeSpans, pbBytes(fResourceSpansDeprecatedScopeSpans, span()))},
		{"group", request(resource, scopeSpans, protowire.AppendTag(protowire.AppendTag(nil, 99, protowire.StartGroupType), 99, protowire.EndGroupType))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeForTest(tc.req)
			assert.Error(t, err)
		})
	}

	t.Run("deprecated instrumentation_library_spans", func(t *testing.T) {
		tps, err := decodeForTest(request(resource, pbBytes(fResourceSpansDeprecatedScopeSpans, span())))
		require.NoError(t, err)
		require.Len(t, tps[0].Chunks, 1)
		require.Len(t, tps[0].Chunks[0].Spans, 1)
		assert.Equal(t, "svc", tps[0].Chunks[0].Spans[0].Service())
		// pdata reads it the same way.
		req := ptraceotlp.NewExportRequest()
		require.NoError(t, req.UnmarshalProto(request(resource, pbBytes(fResourceSpansDeprecatedScopeSpans, span()))))
		assert.Equal(t, 1, req.Traces().SpanCount())
	})
	t.Run("link without trace ID", func(t *testing.T) {
		tps, err := decodeForTest(request(resource, pbBytes(fResourceSpansScopeSpans, span(pbBytes(fSpanLinks, pbBytes(fLinkSpanID, make([]byte, 8)))))))
		require.NoError(t, err)
		assert.Equal(t, make([]byte, 16), tps[0].Chunks[0].Spans[0].Links()[0].TraceID())
	})
}

// TestDecodeResourceSpansLimit checks that a request of up to maxResourceSpans ResourceSpans
// decodes, and that one more is rejected (so the receiver falls back to pdata) rather than
// allocating a payload for each.
func TestDecodeResourceSpansLimit(t *testing.T) {
	emptyResourceSpans := func(n int) []byte {
		var b []byte
		for range n {
			b = protowire.AppendTag(b, fExportResourceSpans, protowire.BytesType)
			b = protowire.AppendBytes(b, nil)
		}
		return b
	}
	tps, err := decodeForTest(emptyResourceSpans(maxResourceSpans))
	require.NoError(t, err)
	assert.Len(t, tps, maxResourceSpans)
	_, err = decodeForTest(emptyResourceSpans(maxResourceSpans + 1))
	assert.ErrorIs(t, err, ErrTooManyResourceSpans)
}
