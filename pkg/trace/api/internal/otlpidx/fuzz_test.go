// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otlpidx

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/protobuf/reflect/protoreflect"

	tracepb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

// fuzzOptions are the decoder options every fuzz input is decoded with: the raw decode, the
// feature flags that change the semantic mapping, and payload-level resource attributes.
var fuzzOptions = sync.OnceValue(func() []Options {
	conf := func(apply func(*config.AgentConfig)) *config.AgentConfig {
		c := newTestConf()
		apply(c)
		return c
	}
	return []Options{
		{Conf: testConf(), SkipSemantics: true},
		{Conf: testConf()},
		{Conf: testConf(), ResourceAttributesOnPayload: true},
		{Conf: conf(func(c *config.AgentConfig) {
			c.Features["enable_otlp_compute_top_level_by_span_kind"] = struct{}{}
			c.Features["disable_otel_scope_convention"] = struct{}{}
		})},
		{Conf: conf(func(c *config.AgentConfig) {
			c.ProbabilisticSamplerEnabled = true
			c.OTLPReceiver.ProbabilisticSampling = 50
		})},
	}
})

// fuzzSeeds returns serialized requests covering every fixture in the package.
func fuzzSeeds(f *testing.F) [][]byte {
	marshal := func(td ptrace.Traces) []byte {
		b, err := ptraceotlp.NewExportRequestFromTraces(td).MarshalProto()
		if err != nil {
			f.Fatal(err)
		}
		return b
	}
	seeds := [][]byte{{}, buildRequest(f)}
	all := ptrace.NewTraces()
	for i, fx := range semanticsFixtures {
		td := ptrace.NewTraces()
		for _, dst := range []ptrace.ResourceSpansSlice{td.ResourceSpans(), all.ResourceSpans()} {
			rs := dst.AppendEmpty()
			rs.Resource().Attributes().PutStr("service.name", "res-svc")
			putAttrs(rs.Resource().Attributes(), fx.res)
			ss := rs.ScopeSpans().AppendEmpty()
			ss.Scope().SetName("my.lib")
			ss.Scope().SetVersion("0.1")
			putAttrs(ss.Scope().Attributes(), fx.scope)
			span := ss.Spans().AppendEmpty()
			span.SetTraceID(traceA)
			span.SetSpanID(pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, byte(i + 1)})
			span.SetParentSpanID(pcommon.SpanID{0, 0, 0, 0, 0, 0, 1, 0})
			span.SetName(fx.spanName)
			span.SetKind(fx.kind)
			span.SetStartTimestamp(1000)
			span.SetEndTimestamp(2000)
			span.Status().SetCode(fx.status)
			span.Status().SetMessage(fx.statusMsg)
			putAttrs(span.Attributes(), fx.attrs)
		}
		seeds = append(seeds, marshal(td))
	}
	return append(seeds, marshal(all))
}

// FuzzDecodeExportRequest checks that the decoder never panics, that every string reference in
// its output is in bounds, and that it agrees with pdata. The decoder may reject encodings pdata
// tolerates (e.g. an unterminated group in an unknown field), since the receiver then falls back
// to pdata, but whenever it succeeds it must produce the same payloads and spans, and it must
// decode pdata's own encoding of any request pdata accepts (unless attributes nest deeper than
// the decoder allows).
func FuzzDecodeExportRequest(f *testing.F) {
	for _, s := range fuzzSeeds(f) {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		req := ptraceotlp.NewExportRequest()
		pdataErr := req.UnmarshalProto(b)
		var canonical []byte
		if pdataErr == nil {
			var err error
			if canonical, err = req.MarshalProto(); err != nil {
				t.Fatalf("re-marshalling the pdata request: %v", err)
			}
		}
		for _, opts := range fuzzOptions() {
			if ps, err := Decode(context.Background(), b, opts); err == nil {
				for _, p := range ps {
					checkRefs(t, p.TracerPayload)
				}
				if pdataErr == nil {
					compareWithPdata(t, req.Traces(), ps, opts.SkipSemantics)
				}
			}
			if pdataErr != nil {
				continue
			}
			ps, err := Decode(context.Background(), canonical, opts)
			if err != nil {
				if !errors.Is(err, errMaxDepth) {
					t.Fatalf("the decoder (%+v) rejects pdata's encoding of the request: %v", opts, err)
				}
				continue
			}
			compareWithPdata(t, req.Traces(), ps, opts.SkipSemantics)
		}
	})
}

// checkRefs fails when any string reference in tp is outside its string table. StringTable.Get
// panics on such a reference, so it would crash the writer or a downstream reader.
func checkRefs(t *testing.T, tp *idx.InternalTracerPayload) {
	t.Helper()
	pb := tp.ToProto()
	n := uint64(len(pb.Strings))
	var walk func(m protoreflect.Message, path string)
	walk = func(m protoreflect.Message, path string) {
		m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
			p := path + "." + string(fd.Name())
			switch {
			case fd.IsMap():
				v.Map().Range(func(k protoreflect.MapKey, mv protoreflect.Value) bool {
					if fd.MapKey().Kind() == protoreflect.Uint32Kind && k.Uint() >= n {
						t.Fatalf("%s: key ref %d out of range (%d strings)", p, k.Uint(), n)
					}
					if fd.MapValue().Message() != nil {
						walk(mv.Message(), p+"[]")
					}
					return true
				})
			case fd.IsList():
				if fd.Message() == nil {
					return true
				}
				l := v.List()
				for i := range l.Len() {
					walk(l.Get(i).Message(), fmt.Sprintf("%s[%d]", p, i))
				}
			case fd.Message() != nil:
				walk(v.Message(), p)
			case fd.Kind() == protoreflect.Uint32Kind && strings.HasSuffix(string(fd.Name()), "Ref"):
				if v.Uint() >= n {
					t.Fatalf("%s: ref %d out of range (%d strings)", p, v.Uint(), n)
				}
			}
			return true
		})
	}
	walk(pb.ProtoReflect(), "payload")
	// Marshal as the V1 trace writer does. Unlike proto.Marshal, it doesn't reject invalid UTF-8,
	// which OTLP strings may contain and which today's v04 path also forwards as-is.
	prepared := []*tracepb.PreparedTracerPayload{tracepb.PrepareTracerPayload(pb)}
	if _, err := tracepb.MarshalAgentPayloadPrepared(&tracepb.AgentPayload{}, prepared); err != nil {
		t.Fatalf("marshalling the decoded payload: %v", err)
	}
}

type fuzzSpanKey struct {
	traceID          string
	spanID, parentID uint64
	start            uint64
	name             string
}

// compareWithPdata checks that ps has one payload per ResourceSpans of td, each with the same
// spans (by trace ID, span ID, parent ID and start time, and name when raw is set, since the
// semantic mapping replaces it with the operation name).
func compareWithPdata(t *testing.T, td ptrace.Traces, ps []Payload, raw bool) {
	t.Helper()
	rss := td.ResourceSpans()
	if len(ps) != rss.Len() {
		t.Fatalf("decoded %d payloads, pdata has %d resource spans", len(ps), rss.Len())
	}
	for i, p := range ps {
		var want []fuzzSpanKey
		sss := rss.At(i).ScopeSpans()
		for j := range sss.Len() {
			spans := sss.At(j).Spans()
			for k := range spans.Len() {
				s := spans.At(k)
				tid, sid, pid := s.TraceID(), s.SpanID(), s.ParentSpanID()
				key := fuzzSpanKey{
					traceID:  string(tid[:]),
					spanID:   spanIDFromBytes(sid[:]),
					parentID: spanIDFromBytes(pid[:]),
					start:    uint64(s.StartTimestamp()),
				}
				if raw {
					key.name = s.Name()
				}
				want = append(want, key)
			}
		}
		var got []fuzzSpanKey
		for _, c := range p.TracerPayload.Chunks {
			for _, s := range c.Spans {
				key := fuzzSpanKey{
					traceID:  string(c.TraceID),
					spanID:   s.SpanID(),
					parentID: s.ParentID(),
					start:    s.Start(),
				}
				if raw {
					key.name = s.Name()
				}
				got = append(got, key)
			}
		}
		if p.SpanCount != len(want) {
			t.Fatalf("payload %d: SpanCount %d, pdata has %d spans", i, p.SpanCount, len(want))
		}
		cmp := func(a, b fuzzSpanKey) int {
			return strings.Compare(fmt.Sprint(a), fmt.Sprint(b))
		}
		slices.SortFunc(want, cmp)
		slices.SortFunc(got, cmp)
		if !slices.Equal(want, got) {
			t.Fatalf("payload %d spans differ:\n pdata:   %+v\n decoded: %+v", i, want, got)
		}
	}
}
