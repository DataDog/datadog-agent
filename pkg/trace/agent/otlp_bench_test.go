// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/shirou/gopsutil/v4/process"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	zstdcomp "github.com/DataDog/datadog-agent/comp/trace/compression/impl-zstd"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// BenchmarkOTLPEndToEnd measures OTLP traces going through the whole agent
// pipeline: OTLPReceiver.ReceiveResourceSpans, then Process and TraceWriter
// (legacy pipeline, disable-convert-traces) or ProcessV1 and TraceWriterV1 (V1
// pipeline, the default), with the trace writer serializing, compressing and
// sending the payloads to a local intake. One iteration processes one batch.
// The writer compresses with zstd, as the trace-agent, the otel-agent and the
// serverless agent do.
//
// The writer runs in synchronous mode, and a synchronous flush waits for the
// senders by polling every 10ms. It is flushed every ~10000 spans, so that this
// doesn't dominate small batches, but the legacy writer also flushes whenever
// its buffer fills, so sec/op includes waits that are off the processing path in
// the default (asynchronous) agent. The cpu-ns/op metric, the CPU time of the
// whole process per iteration (the local intake included, a ~1% overhead), is
// the one to compare the pipelines with.
//
// The receive-only variants stop after the OTLP receiver, to tell its cost
// apart from the one of the rest of the pipeline.
func BenchmarkOTLPEndToEnd(b *testing.B) {
	pkglog.SetupLogger(pkglog.Default(), "off")
	for _, size := range []struct{ spans, resources int }{
		{1, 1},
		{100, 1},
		{1000, 1},
		{1000, 4},
	} {
		traces := otlpBenchTraces(size.spans, size.resources)
		for _, pipeline := range []string{"legacy", "v1"} {
			for _, receiveOnly := range []bool{false, true} {
				name := fmt.Sprintf("spans=%d/resources=%d/%s", size.spans, size.resources, pipeline)
				if receiveOnly {
					name += "-receive-only"
				}
				b.Run(name, func(b *testing.B) {
					runOTLPEndToEndBenchmark(b, traces, pipeline == "legacy", receiveOnly)
				})
			}
		}
	}
}

func runOTLPEndToEndBenchmark(b *testing.B, traces ptrace.Traces, legacy, receiveOnly bool) {
	intake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer intake.Close()

	cfg := config.New()
	cfg.Endpoints[0].Host = intake.URL
	cfg.Endpoints[0].APIKey = "test"
	cfg.ReceiverPort = 0
	cfg.SynchronousFlushing = true
	cfg.Hostname = "bench-host"
	cfg.Obfuscation.Cache.Enabled = false
	cfg.TraceBuffer = traces.ResourceSpans().Len()
	translator, err := attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
	if err != nil {
		b.Fatal(err)
	}
	cfg.OTLPReceiver.AttributesTranslator = translator
	if legacy {
		cfg.Features["disable-convert-traces"] = struct{}{}
	}
	a := NewAgent(context.Background(), cfg, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, zstdcomp.NewComponent())
	go a.StatsWriter.Run()
	defer func() {
		a.TraceWriter.Stop()
		a.TraceWriterV1.Stop()
		a.StatsWriter.Stop()
	}()

	ctx := context.Background()
	rspans := traces.ResourceSpans()
	flushEvery := max(1, 10000/traces.SpanCount())
	flush := func() {
		var err error
		if legacy {
			err = a.TraceWriter.FlushSync()
		} else {
			err = a.TraceWriterV1.FlushSync()
		}
		if err != nil {
			b.Fatal(err)
		}
	}
	proc, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	cpuStart := processCPUTime(b, proc)
	for i := 0; i < b.N; i++ {
		for j := 0; j < rspans.Len(); j++ {
			if _, err := a.OTLPReceiver.ReceiveResourceSpans(ctx, rspans.At(j), http.Header{}, nil); err != nil {
				b.Fatal(err)
			}
			switch {
			case legacy && receiveOnly:
				<-a.In
			case legacy:
				a.Process(<-a.In)
			case receiveOnly:
				<-a.InV1
			default:
				a.ProcessV1(<-a.InV1)
			}
		}
		if !receiveOnly && (i+1)%flushEvery == 0 {
			flush()
		}
	}
	if !receiveOnly {
		flush()
	}
	b.ReportMetric(float64(processCPUTime(b, proc)-cpuStart)/float64(b.N), "cpu-ns/op")
}

// processCPUTime returns the user and system CPU time the process has used.
func processCPUTime(b *testing.B, proc *process.Process) time.Duration {
	times, err := proc.Times()
	if err != nil {
		b.Fatal(err)
	}
	return time.Duration((times.User + times.System) * float64(time.Second))
}

// otlpBenchTraces returns read-only OTLP traces of the given number of spans,
// split over the given number of resources, in traces of up to 10 spans: an
// HTTP server root span, HTTP client, database and messaging spans, and
// internal spans, with a few attributes each and some span events and links.
func otlpBenchTraces(spans, resources int) ptrace.Traces {
	rng := rand.New(rand.NewSource(42))
	now := time.Now().UnixNano()
	td := ptrace.NewTraces()
	perResource := spans / resources
	for r := 0; r < resources; r++ {
		rs := td.ResourceSpans().AppendEmpty()
		attrs := rs.Resource().Attributes()
		attrs.PutStr("service.name", fmt.Sprintf("bench-service-%d", r))
		attrs.PutStr("deployment.environment.name", "bench")
		attrs.PutStr("service.version", "1.2.3")
		attrs.PutStr("telemetry.sdk.language", "go")
		attrs.PutStr("telemetry.sdk.version", "1.30.0")
		attrs.PutStr("host.name", "bench-host")
		attrs.PutStr("container.id", fmt.Sprintf("bench-container-%d", r))
		attrs.PutStr("k8s.pod.name", fmt.Sprintf("bench-pod-%d", r))
		ss := rs.ScopeSpans().AppendEmpty()
		ss.Scope().SetName("go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp")
		ss.Scope().SetVersion("0.50.0")
		var traceID pcommon.TraceID
		var rootID pcommon.SpanID
		for i := 0; i < perResource; i++ {
			s := ss.Spans().AppendEmpty()
			var spanID pcommon.SpanID
			rng.Read(spanID[:])
			if i%10 == 0 {
				rng.Read(traceID[:])
				rootID = spanID
			} else {
				s.SetParentSpanID(rootID)
			}
			s.SetTraceID(traceID)
			s.SetSpanID(spanID)
			s.SetStartTimestamp(pcommon.Timestamp(now - 1e6))
			s.SetEndTimestamp(pcommon.Timestamp(now))
			sattrs := s.Attributes()
			switch {
			case i%10 == 0:
				s.SetKind(ptrace.SpanKindServer)
				s.SetName("GET /users/:id")
				sattrs.PutStr("http.request.method", "GET")
				sattrs.PutStr("http.route", "/users/:id")
				sattrs.PutStr("url.path", fmt.Sprintf("/users/%d", rng.Intn(1000)))
				sattrs.PutInt("http.response.status_code", 200)
				sattrs.PutStr("user_agent.original", "bench/1.0")
				sattrs.PutStr("client.address", "10.0.0.1")
			case i%10 < 3:
				s.SetKind(ptrace.SpanKindClient)
				s.SetName("SELECT users")
				sattrs.PutStr("db.system", "postgresql")
				sattrs.PutStr("db.namespace", "bench")
				sattrs.PutStr("db.query.text", fmt.Sprintf("SELECT * FROM users WHERE id = %d", rng.Intn(1000)))
				sattrs.PutStr("server.address", "db.local")
				sattrs.PutInt("server.port", 5432)
			case i%10 < 5:
				s.SetKind(ptrace.SpanKindClient)
				s.SetName("GET")
				sattrs.PutStr("http.request.method", "GET")
				sattrs.PutStr("url.full", "http://backend.local/api/items")
				sattrs.PutStr("url.template", "/api/items")
				sattrs.PutInt("http.response.status_code", 200)
				sattrs.PutStr("server.address", "backend.local")
			case i%10 < 7:
				s.SetKind(ptrace.SpanKindProducer)
				s.SetName("publish orders")
				sattrs.PutStr("messaging.system", "kafka")
				sattrs.PutStr("messaging.operation", "publish")
				sattrs.PutStr("messaging.destination.name", "orders")
				l := s.Links().AppendEmpty()
				l.SetTraceID(traceID)
				l.SetSpanID(rootID)
				l.Attributes().PutStr("link.reason", "batch")
			default:
				s.SetKind(ptrace.SpanKindInternal)
				s.SetName("render")
				sattrs.PutStr("component", "templates")
				sattrs.PutDouble("render.ratio", rng.Float64())
				e := s.Events().AppendEmpty()
				e.SetName("cache.miss")
				e.SetTimestamp(pcommon.Timestamp(now - 5e5))
				e.Attributes().PutStr("cache.key", "users")
				e.Attributes().PutInt("cache.size", 42)
			}
		}
	}
	td.MarkReadOnly()
	return td
}
