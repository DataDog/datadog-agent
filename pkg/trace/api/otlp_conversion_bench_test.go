// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"fmt"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/transform"
)

// BenchmarkOTelSpanConversion compares, per span, the conversion of an OTel span
// into an idx span through a pb span (transform.OtelSpanToDDSpan, then
// ConvertToIdx of a one-span payload, as before the OTLP receiver emitted V1
// payloads) and directly (transform.OtelSpanToIdxSpan), over several span
// shapes.
func BenchmarkOTelSpanConversion(b *testing.B) {
	conf := config.New()
	res := pcommon.NewResource()
	rattrs := res.Attributes()
	rattrs.PutStr("service.name", "bench-service")
	rattrs.PutStr("deployment.environment.name", "bench")
	rattrs.PutStr("service.version", "1.2.3")
	rattrs.PutStr("telemetry.sdk.language", "go")
	rattrs.PutStr("host.name", "bench-host")
	lib := pcommon.NewInstrumentationScope()
	lib.SetName("bench-scope")
	lib.SetVersion("1.0.0")

	for _, shape := range otelSpanBenchShapes() {
		b.Run(shape.name+"/pb+ConvertToIdx", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ddspan := transform.OtelSpanToDDSpan(shape.span, res, lib, conf)
				_ = ConvertToIdx(&pb.TracerPayload{Chunks: []*pb.TraceChunk{{Spans: []*pb.Span{ddspan}}}}, "")
			}
		})
		b.Run(shape.name+"/native", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = transform.OtelSpanToIdxSpan(shape.span, res, lib, conf, idx.NewStringTable())
			}
		})
	}
}

type otelSpanBenchShape struct {
	name string
	span ptrace.Span
}

func otelSpanBenchShapes() []otelSpanBenchShape {
	now := time.Now().UnixNano()
	newSpan := func(kind ptrace.SpanKind, name string) ptrace.Span {
		s := ptrace.NewSpan()
		s.SetTraceID(pcommon.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
		s.SetSpanID(pcommon.SpanID{1, 2, 3, 4, 5, 6, 7, 8})
		s.SetParentSpanID(pcommon.SpanID{8, 7, 6, 5, 4, 3, 2, 1})
		s.SetKind(kind)
		s.SetName(name)
		s.SetStartTimestamp(pcommon.Timestamp(now - 1e6))
		s.SetEndTimestamp(pcommon.Timestamp(now))
		return s
	}

	fewAttrs := newSpan(ptrace.SpanKindInternal, "work")
	fewAttrs.Attributes().PutStr("component", "worker")
	fewAttrs.Attributes().PutInt("items", 3)

	manyAttrs := newSpan(ptrace.SpanKindInternal, "work")
	for i := 0; i < 40; i++ {
		manyAttrs.Attributes().PutStr(fmt.Sprintf("custom.string.%d", i), fmt.Sprintf("value-%d", i))
	}
	for i := 0; i < 10; i++ {
		manyAttrs.Attributes().PutDouble(fmt.Sprintf("custom.number.%d", i), float64(i))
	}

	eventsLinks := newSpan(ptrace.SpanKindServer, "handle")
	for i := 0; i < 3; i++ {
		e := eventsLinks.Events().AppendEmpty()
		e.SetName(fmt.Sprintf("event-%d", i))
		e.SetTimestamp(pcommon.Timestamp(now - 5e5))
		e.Attributes().PutStr("message", "something happened")
		e.Attributes().PutInt("attempt", int64(i))
		arr := e.Attributes().PutEmptySlice("tags")
		arr.AppendEmpty().SetStr("a")
		arr.AppendEmpty().SetStr("b")
		l := eventsLinks.Links().AppendEmpty()
		l.SetTraceID(pcommon.TraceID{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1})
		l.SetSpanID(pcommon.SpanID{byte(i), 2, 3, 4, 5, 6, 7, 8})
		l.TraceState().FromRaw("dd=s:1")
		l.Attributes().PutStr("link.reason", "follows")
	}
	exc := eventsLinks.Events().AppendEmpty()
	exc.SetName("exception")
	exc.Attributes().PutStr("exception.message", "boom")
	exc.Attributes().PutStr("exception.type", "Error")

	httpServer := newSpan(ptrace.SpanKindServer, "GET /users/:id")
	httpServer.Attributes().PutStr("http.request.method", "GET")
	httpServer.Attributes().PutStr("http.route", "/users/:id")
	httpServer.Attributes().PutStr("url.path", "/users/42")
	httpServer.Attributes().PutStr("url.scheme", "https")
	httpServer.Attributes().PutInt("http.response.status_code", 200)
	httpServer.Attributes().PutStr("user_agent.original", "bench/1.0")
	httpServer.Attributes().PutStr("client.address", "10.0.0.1")
	httpServer.Attributes().PutStr("network.protocol.version", "1.1")

	db := newSpan(ptrace.SpanKindClient, "SELECT users")
	db.Attributes().PutStr("db.system", "postgresql")
	db.Attributes().PutStr("db.namespace", "bench")
	db.Attributes().PutStr("db.query.text", "SELECT * FROM users WHERE id = 42")
	db.Attributes().PutStr("db.operation.name", "SELECT")
	db.Attributes().PutStr("server.address", "db.local")
	db.Attributes().PutInt("server.port", 5432)

	messaging := newSpan(ptrace.SpanKindProducer, "publish orders")
	messaging.Attributes().PutStr("messaging.system", "kafka")
	messaging.Attributes().PutStr("messaging.operation", "publish")
	messaging.Attributes().PutStr("messaging.destination.name", "orders")
	messaging.Attributes().PutStr("messaging.kafka.message.key", "order-42")
	messaging.Attributes().PutInt("messaging.kafka.destination.partition", 3)
	messaging.Attributes().PutStr("server.address", "kafka.local")

	return []otelSpanBenchShape{
		{"few-attributes", fewAttrs},
		{"many-attributes", manyAttrs},
		{"events-links", eventsLinks},
		{"http-server", httpServer},
		{"db-client", db},
		{"messaging-producer", messaging},
	}
}
