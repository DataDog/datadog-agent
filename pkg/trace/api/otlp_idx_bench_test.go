// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"encoding/binary"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"

	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/header"
	"github.com/DataDog/datadog-agent/pkg/trace/api/internal/otlpidx"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
)

// otlpBenchProfile describes the shape of a synthetic OTLP export request. Spans get unique
// trace/span IDs so chunk grouping is exercised (the older generateTraceRequest helper puts
// every span in a single trace).
type otlpBenchProfile struct {
	name          string
	traces        int
	spansPerTrace int
	extraAttrs    int // generic string attributes per span, in addition to the semconv set
	attrLen       int
	resAttrs      int // generic resource attributes, in addition to the semconv set
	events        bool
}

var otlpBenchProfiles = []otlpBenchProfile{
	{name: "small", traces: 1, spansPerTrace: 5, extraAttrs: 2, attrLen: 16, resAttrs: 2},
	{name: "typical", traces: 10, spansPerTrace: 10, extraAttrs: 10, attrLen: 24, resAttrs: 10, events: true},
	{name: "attr_heavy", traces: 10, spansPerTrace: 100, extraAttrs: 100, attrLen: 100, resAttrs: 100},
}

func buildOTLPBenchRequest(p otlpBenchProfile) ptraceotlp.ExportRequest {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ra := rs.Resource().Attributes()
	ra.PutStr("service.name", "checkout")
	ra.PutStr("service.version", "1.4.2")
	ra.PutStr("deployment.environment.name", "prod")
	ra.PutStr("telemetry.sdk.language", "go")
	ra.PutStr("telemetry.sdk.version", "1.30.0")
	ra.PutStr("host.name", "ip-10-0-0-1")
	ra.PutStr("container.id", strings.Repeat("c", 64))
	for j := 0; j < p.resAttrs; j++ {
		ra.PutStr("res_key_"+strconv.Itoa(j), strings.Repeat("r", p.attrLen))
	}
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp")
	ss.Scope().SetVersion("0.55.0")
	var id uint64
	for t := 0; t < p.traces; t++ {
		var tid pcommon.TraceID
		binary.BigEndian.PutUint64(tid[:8], uint64(t+1))
		binary.BigEndian.PutUint64(tid[8:], uint64(t+1)*7919)
		var parent pcommon.SpanID
		for s := 0; s < p.spansPerTrace; s++ {
			id++
			var sid pcommon.SpanID
			binary.BigEndian.PutUint64(sid[:], id)
			span := ss.Spans().AppendEmpty()
			span.SetTraceID(tid)
			span.SetSpanID(sid)
			span.SetParentSpanID(parent)
			parent = sid
			span.SetStartTimestamp(pcommon.Timestamp(1_700_000_000_000_000_000 + id*1000))
			span.SetEndTimestamp(pcommon.Timestamp(1_700_000_000_000_000_000 + id*1000 + 500))
			a := span.Attributes()
			if s == 0 {
				span.SetName("GET /api/cart/{id}")
				span.SetKind(ptrace.SpanKindServer)
				a.PutStr("http.request.method", "GET")
				a.PutStr("http.route", "/api/cart/{id}")
				a.PutStr("url.path", "/api/cart/"+strconv.FormatUint(id, 10))
				a.PutInt("http.response.status_code", 200)
				a.PutStr("server.address", "checkout.internal")
			} else {
				span.SetName("SELECT cart")
				span.SetKind(ptrace.SpanKindClient)
				a.PutStr("db.system", "postgresql")
				a.PutStr("db.statement", "SELECT * FROM cart WHERE id = $1")
				a.PutStr("db.namespace", "shop")
				a.PutStr("server.address", "db.internal")
				a.PutInt("server.port", 5432)
			}
			a.PutDouble("sample.ratio", 0.25)
			a.PutBool("cache.hit", s%2 == 0)
			for j := 0; j < p.extraAttrs; j++ {
				a.PutStr("key_"+strconv.Itoa(j), strings.Repeat("x", p.attrLen))
			}
			if p.events && s == p.spansPerTrace-1 {
				ev := span.Events().AppendEmpty()
				ev.SetName("exception")
				ev.SetTimestamp(span.EndTimestamp())
				ev.Attributes().PutStr("exception.type", "io.EOF")
				ev.Attributes().PutStr("exception.message", "unexpected EOF")
				span.Status().SetCode(ptrace.StatusCodeError)
				span.Status().SetMessage("unexpected EOF")
			}
		}
	}
	return ptraceotlp.NewExportRequestFromTraces(td)
}

func newDrainingOTLPReceiver(b *testing.B) (*OTLPReceiver, func()) {
	out := make(chan *Payload, 100)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-out:
			case <-done:
				return
			}
		}
	}()
	r := NewOTLPReceiver(out, nil, NewBenchmarkTestConfig(b), &statsd.NoOpClient{}, &timing.NoopReporter{})
	return r, func() { close(done) }
}

// BenchmarkOTLPIngest compares, from identical serialized ExportTraceServiceRequest bytes:
//   - pdata_unmarshal: pdata proto decode only (the floor of today's path)
//   - pdata_v04: pdata decode + receiveResourceSpansV2 (today's full receiver path, pb.Span output)
//   - pdata_v04_to_idx: today's path + ConvertToIdx (what reaching V1 costs without a new decoder)
//   - direct_idx_raw: direct decoder, raw decode only (no semantic mapping)
//   - direct_idx: direct decoder with semantic mapping, resource attributes copied into every span
//     (the default)
//   - direct_idx_res_on_payload: direct decoder with semantic mapping, resource attributes stored
//     once on the payload
func BenchmarkOTLPIngest(b *testing.B) {
	md := http.Header{header.Lang: {"go"}}
	for _, p := range otlpBenchProfiles {
		req := buildOTLPBenchRequest(p)
		raw, err := req.MarshalProto()
		require.NoError(b, err)

		b.Run(p.name+"/pdata_unmarshal", func(b *testing.B) {
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				in := ptraceotlp.NewExportRequest()
				if err := in.UnmarshalProto(raw); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(p.name+"/pdata_v04", func(b *testing.B) {
			r, stop := newDrainingOTLPReceiver(b)
			defer stop()
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				in := ptraceotlp.NewExportRequest()
				if err := in.UnmarshalProto(raw); err != nil {
					b.Fatal(err)
				}
				r.processRequest(context.Background(), md, in)
			}
		})

		b.Run(p.name+"/pdata_v04_to_idx", func(b *testing.B) {
			out := make(chan *Payload, 1)
			r := NewOTLPReceiver(out, nil, NewBenchmarkTestConfig(b), &statsd.NoOpClient{}, &timing.NoopReporter{})
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				in := ptraceotlp.NewExportRequest()
				if err := in.UnmarshalProto(raw); err != nil {
					b.Fatal(err)
				}
				rss := in.Traces().ResourceSpans()
				for j := 0; j < rss.Len(); j++ {
					if _, err := r.ReceiveResourceSpans(context.Background(), rss.At(j), md, nil); err != nil {
						b.Fatal(err)
					}
					_ = ConvertToIdx((<-out).TracerPayload, "otlp")
				}
			}
		})

		for _, v := range []struct {
			name string
			opts otlpidx.Options
		}{
			{"direct_idx_raw", otlpidx.Options{SkipSemantics: true}},
			{"direct_idx", otlpidx.Options{}},
			{"direct_idx_res_on_payload", otlpidx.Options{ResourceAttributesOnPayload: true}},
		} {
			b.Run(p.name+"/"+v.name, func(b *testing.B) {
				opts := v.opts
				opts.Conf = NewBenchmarkTestConfig(b)
				b.SetBytes(int64(len(raw)))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := otlpidx.Decode(context.Background(), raw, opts); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
