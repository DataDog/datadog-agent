// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/test"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	otlpTestServerSpanID   = pcommon.SpanID([8]byte{0x24, 0x0, 0x31, 0xea, 0xd7, 0x50, 0xe5, 0xf3})
	otlpTestClientSpanID   = pcommon.SpanID([8]byte{0x24, 0x0, 0x31, 0xea, 0xd7, 0x50, 0xe5, 0xf4})
	otlpTestProducerSpanID = pcommon.SpanID([8]byte{0x24, 0x0, 0x31, 0xea, 0xd7, 0x50, 0xe5, 0xf5})
	otlpTestInternalSpanID = pcommon.SpanID([8]byte{0x24, 0x0, 0x31, 0xea, 0xd7, 0x50, 0xe5, 0xf6})
	otlpTestTrace1ID       = pcommon.TraceID([16]byte{0x72, 0xdf, 0x52, 0xa, 0xf2, 0xbd, 0xe7, 0xa5, 0x24, 0x0, 0x31, 0xea, 0xd7, 0x50, 0xe5, 0xf3})
	otlpTestTrace2ID       = pcommon.TraceID([16]byte{0x72, 0xdf, 0x52, 0xa, 0xf2, 0xbd, 0xe7, 0xa5, 0x24, 0x0, 0x31, 0xea, 0xd7, 0x50, 0xe5, 0xf4})
	testSpansNow           = uint64(time.Now().UnixNano())
	testSpans              = []*testutil.OTLPSpan{
		{
			Name:       "/path",
			Kind:       ptrace.SpanKindServer,
			Start:      testSpansNow,
			End:        testSpansNow + 200000000,
			Attributes: map[string]interface{}{"name": "server"},
			TraceID:    otlpTestTrace1ID,
			SpanID:     otlpTestServerSpanID,
		},
		{
			Name:       "internal",
			Kind:       ptrace.SpanKindInternal,
			Start:      testSpansNow + 10000,
			End:        testSpansNow + 200000000,
			Attributes: map[string]interface{}{"name": "internal"},
			TraceID:    otlpTestTrace1ID,
			SpanID:     otlpTestInternalSpanID,
			ParentID:   otlpTestServerSpanID,
		},
		{
			Name:       "request",
			Kind:       ptrace.SpanKindClient,
			Start:      testSpansNow + 20000,
			End:        testSpansNow + 200000000,
			Attributes: map[string]interface{}{"name": "client"},
			TraceID:    otlpTestTrace1ID,
			SpanID:     otlpTestClientSpanID,
			ParentID:   otlpTestInternalSpanID,
		},
		{
			Name:       "producer",
			Kind:       ptrace.SpanKindProducer,
			Start:      testSpansNow + 30000,
			End:        testSpansNow + 300000000,
			Attributes: map[string]interface{}{"name": "producer"},
			TraceID:    otlpTestTrace2ID,
			SpanID:     otlpTestProducerSpanID,
		},
	}
)

func TestOTLPIngest(t *testing.T) {
	var r test.Runner
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Shutdown(time.Second); err != nil {
			t.Log("shutdown: ", err)
		}
	}()

	t.Run("passthrough", func(t *testing.T) {
		port := testutil.FreeTCPPort(t)
		c := fmt.Sprintf(`
otlp_config:
  traces:
    internal_port: %d
  receiver:
    grpc:
      endpoint: 0.0.0.0:5111
apm_config:
  env: my-env
`, port)
		if err := r.RunAgent([]byte(c)); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", port), grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials())) //nolint:staticcheck // TODO (ASC) fix grpc.Dial is deprecated
		if err != nil {
			log.Fatal("Error dialing: ", err)
		}
		client := ptraceotlp.NewGRPCClient(conn)
		now := uint64(time.Now().UnixNano())
		pack := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
			{
				LibName:    "test",
				LibVersion: "0.1t",
				Attributes: map[string]interface{}{"service.name": "pylons"},
				Spans: []*testutil.OTLPSpan{
					{
						Name:       "/path",
						Kind:       ptrace.SpanKindServer,
						Start:      now,
						End:        now + 200000000,
						Attributes: map[string]interface{}{"name": "john"},
					},
				},
			},
		})
		_, err = client.Export(context.Background(), pack)
		if err != nil {
			log.Fatal("Error calling: ", err)
		}
		waitForTrace(t, &r, func(p *pb.AgentPayload) {
			assert := assert.New(t)
			assert.Equal(p.Env, "my-env")
			tps := otlpSpanStrAttrs(p, "name")
			if assert.Len(tps, 1) && assert.Len(tps[0], 1) && assert.Len(tps[0][0], 1) {
				assert.Equal("john", tps[0][0][0])
			}
		})
	})

	// The sampling decision maker sent by a Datadog SDK exporting OTLP must reach the intake,
	// through the agent's processing and payload encoding.
	t.Run("sampling-decision-maker", func(t *testing.T) {
		port := testutil.FreeTCPPort(t)
		c := fmt.Sprintf(`
otlp_config:
  traces:
    internal_port: %d
  receiver:
    grpc:
      endpoint: 0.0.0.0:5111
apm_config:
  env: my-env
`, port)
		if err := r.RunAgent([]byte(c)); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", port), grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials())) //nolint:staticcheck // TODO (ASC) fix grpc.Dial is deprecated
		if err != nil {
			log.Fatal("Error dialing: ", err)
		}
		client := ptraceotlp.NewGRPCClient(conn)
		now := uint64(time.Now().UnixNano())
		resource := func(service string, traceID byte, traceState string, attrs map[string]interface{}) testutil.OTLPResourceSpan {
			return testutil.OTLPResourceSpan{
				LibName:    "test",
				LibVersion: "0.1t",
				Attributes: map[string]interface{}{"service.name": service},
				Spans: []*testutil.OTLPSpan{{
					Name:       "/path",
					Kind:       ptrace.SpanKindServer,
					Start:      now,
					End:        now + 200000000,
					TraceID:    [16]byte{15: traceID}, // the agent keeps the low 64 bits
					SpanID:     [8]byte{7: traceID},
					TraceState: traceState,
					Attributes: attrs,
				}},
			}
		}
		// service -> expected decision maker
		want := map[string]string{
			"dm-attribute":  "-12", // Datadog SDK: decision maker attribute
			"dm-tracestate": "-11", // Datadog SDK: decision maker only in the W3C tracestate
			"dm-none":       "-4",  // upstream OTel SDK: priority without a decision maker
		}
		pack := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
			resource("dm-attribute", 1, "", map[string]interface{}{"_sampling_priority_v1": 2, "_dd.p.dm": "-12"}),
			resource("dm-tracestate", 2, "dd=s:2;t.dm:-11", map[string]interface{}{"_sampling_priority_v1": 2}),
			resource("dm-none", 3, "", map[string]interface{}{"sampling.priority": 2}),
		})
		if _, err = client.Export(context.Background(), pack); err != nil {
			log.Fatal("Error calling: ", err)
		}

		// service -> decision maker found at the intake (classic span meta and chunk tag, or idx SamplingMechanism)
		got := map[string]string{}
		timeout := time.After(5 * time.Second)
		for len(got) < len(want) {
			select {
			case p := <-r.Out():
				v, ok := p.(*pb.AgentPayload)
				if !ok {
					continue
				}
				for _, tp := range v.TracerPayloads {
					for _, chunk := range tp.Chunks {
						assert.Equal(t, int32(2), chunk.Priority)
						got[chunk.Spans[0].Service] = chunk.Spans[0].Meta["_dd.p.dm"]
						if dm, ok := chunk.Tags["_dd.p.dm"]; ok {
							assert.Equal(t, got[chunk.Spans[0].Service], dm, "chunk tag and span meta must agree")
						}
					}
				}
				for _, tp := range v.IdxTracerPayloads {
					for _, chunk := range tp.Chunks {
						assert.Equal(t, int32(2), chunk.GetPriority())
						got[idxStr(tp.Strings, chunk.Spans[0].GetServiceRef())] = fmt.Sprintf("-%d", chunk.GetSamplingMechanism())
					}
				}
			case <-timeout:
				t.Fatalf("timed out waiting for traces, got %v, log was:\n%s", got, r.AgentLog())
			}
		}
		assert.Equal(t, want, got)
	})

	// OTLP span events must reach the intake as native span events, through the agent's
	// processing (obfuscation, tag replacement, truncation) and payload encoding.
	t.Run("span-events", func(t *testing.T) {
		port := testutil.FreeTCPPort(t)
		c := fmt.Sprintf(`
otlp_config:
  traces:
    internal_port: %d
  receiver:
    grpc:
      endpoint: 0.0.0.0:5111
apm_config:
  env: my-env
  replace_tags:
    - name: "secret.token"
      pattern: "s3cr3t"
      repl: "?"
`, port)
		if err := r.RunAgent([]byte(c)); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", port), grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials())) //nolint:staticcheck // TODO (ASC) fix grpc.Dial is deprecated
		if err != nil {
			log.Fatal("Error dialing: ", err)
		}
		client := ptraceotlp.NewGRPCClient(conn)

		// Longer than the 25,000 characters a tag value is truncated to.
		stack := strings.Repeat("at com.example.Foo.bar(Foo.java:42)\n", 1000)
		now := uint64(time.Now().UnixNano())
		traces := ptrace.NewTraces()
		rs := traces.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("service.name", "span-events")
		span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetName("/path")
		span.SetKind(ptrace.SpanKindServer)
		span.SetTraceID([16]byte{15: 9})
		span.SetSpanID([8]byte{7: 9})
		span.SetStartTimestamp(pcommon.Timestamp(now))
		span.SetEndTimestamp(pcommon.Timestamp(now + 200000000))

		event := span.Events().AppendEmpty()
		event.SetName("all.types")
		event.SetTimestamp(pcommon.Timestamp(now + 1000))
		attrs := event.Attributes()
		attrs.PutStr("str", `quote " backslash \`)
		attrs.PutBool("bool", true)
		attrs.PutInt("int", -42)
		attrs.PutDouble("double", 2.5)
		attrs.PutEmptyBytes("bytes").FromRaw([]byte{0xde, 0xad})
		strs := attrs.PutEmptySlice("strs")
		strs.AppendEmpty().SetStr("a")
		strs.AppendEmpty().SetStr("b")
		attrs.PutEmptySlice("empty.array")
		mixed := attrs.PutEmptySlice("mixed")
		mixed.AppendEmpty().SetStr("one")
		mixed.AppendEmpty().SetInt(2)
		attrs.PutEmptyMap("map").PutStr("inner", "value")
		attrs.PutStr("card.number", "4166 6766 6766 6746")
		attrs.PutStr("secret.token", "token=s3cr3t")

		exception := span.Events().AppendEmpty()
		exception.SetName("exception")
		exception.SetTimestamp(pcommon.Timestamp(now + 2000))
		exception.Attributes().PutStr("exception.type", "OutOfMemoryError")
		exception.Attributes().PutStr("exception.stacktrace", stack)

		if _, err = client.Export(context.Background(), ptraceotlp.NewExportRequestFromTraces(traces)); err != nil {
			log.Fatal("Error calling: ", err)
		}

		want := map[string]map[string]string{
			"all.types": {
				"str":          `string:quote " backslash \`,
				"bool":         "bool:true",
				"int":          "int:-42",
				"double":       "double:2.5",
				"bytes":        "string:3q0=",
				"strs":         "array:[string:a string:b]",
				"empty.array":  "array:[]",
				"mixed.0":      "string:one",
				"mixed.1":      "int:2",
				"map.inner":    "string:value",
				"card.number":  "string:?",
				"secret.token": "string:token=?",
			},
			"exception": {
				"exception.type":       "string:OutOfMemoryError",
				"exception.stacktrace": "string:" + stack,
			},
		}
		timeout := time.After(5 * time.Second)
		for {
			select {
			case p := <-r.Out():
				v, ok := p.(*pb.AgentPayload)
				if !ok {
					continue
				}
				got, hasException, found := otlpSpanEvents(v, "span-events")
				if !found {
					continue
				}
				assert.Equal(t, want, got)
				assert.True(t, hasException, "_dd.span_events.has_exception must be set")
				return
			case <-timeout:
				t.Fatalf("timed out waiting for the span, log was:\n%s", r.AgentLog())
			}
		}
	})

	// regression test for DataDog/datadog-agent#11297
	t.Run("duplicate-spanID", func(t *testing.T) {
		port := testutil.FreeTCPPort(t)
		c := fmt.Sprintf(`
otlp_config:
  traces:
    internal_port: %d
  receiver:
    grpc:
      endpoint: 0.0.0.0:5111
apm_config:
  env: my-env
`, port)
		if err := r.RunAgent([]byte(c)); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", port), grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials())) //nolint:staticcheck // TODO (ASC) fix grpc.Dial is deprecated
		if err != nil {
			log.Fatal("Error dialing: ", err)
		}
		client := ptraceotlp.NewGRPCClient(conn)
		now := uint64(time.Now().UnixNano())
		pack := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
			{
				LibName:    "test",
				LibVersion: "0.1t",
				Attributes: map[string]interface{}{"service.name": "pylons"},
				Spans: []*testutil.OTLPSpan{
					{
						TraceID: testutil.OTLPFixedTraceID,
						SpanID:  testutil.OTLPFixedSpanID,
						Name:    "/path",
						Kind:    ptrace.SpanKindServer,
						Start:   now,
						End:     now + 200000000,
					},
					{
						TraceID: testutil.OTLPFixedTraceID,
						SpanID:  testutil.OTLPFixedSpanID,
						Name:    "/path",
						Kind:    ptrace.SpanKindServer,
						Start:   now,
						End:     now + 200000000,
					},
				},
			},
		})
		_, err = client.Export(context.Background(), pack)
		if err != nil {
			log.Fatal("Error calling: ", err)
		}
		timeout := time.After(1 * time.Second)
	loop:
		for {
			select {
			case <-timeout:
				t.Fatal("Timed out waiting for duplicate SpanID warning.")
			default:
				time.Sleep(10 * time.Millisecond)
				// The v1 (idx) normalizer, used by default, logs the span as
				// "SpanID: <id>, Service: <service>, ..."; the legacy normalizer
				// (convert-traces disabled) logs the pb.Span string form.
				agentLog := r.AgentLog()
				if duplicateSpanIDV1Log.MatchString(agentLog) ||
					strings.Contains(agentLog, `Found malformed trace with duplicate span ID (reason:duplicate_span_id): service:"pylons"`) {
					break loop
				}
			}
		}
	})

	// topLevelSpansAgentFn checks that the given agent payload matches with the testSpans input
	topLevelSpansAgentFn := func(v *pb.AgentPayload) {
		var serverSpan, internalSpan, clientSpan, producerSpan bool
		tps := otlpSpanStrAttrs(v, "name")
		if !assert.Len(t, tps, 1) {
			return
		}
		for _, chunk := range tps[0] {
			for _, name := range chunk {
				switch name {
				case "server":
					assert.Len(t, chunk, 3)
					serverSpan = true
				case "internal":
					assert.Len(t, chunk, 3)
					internalSpan = true
				case "client":
					assert.Len(t, chunk, 3)
					clientSpan = true
				case "producer":
					assert.Len(t, chunk, 1)
					producerSpan = true
				}
			}
		}
		assert.Equal(t, "my-env", v.Env)
		assert.Len(t, tps[0], 2)
		assert.True(t, serverSpan && internalSpan && clientSpan && producerSpan)
	}

	t.Run("top-level-by-span-kind", func(t *testing.T) {
		port := testutil.FreeTCPPort(t)
		c := fmt.Sprintf(`
otlp_config:
  traces:
    internal_port: %d
  receiver:
    grpc:
      endpoint: 0.0.0.0:5111
apm_config:
  env: my-env
  features: ["enable_otlp_compute_top_level_by_span_kind"]
`, port)
		if err := r.RunAgent([]byte(c)); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", port), grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials())) //nolint:staticcheck // TODO (ASC) fix grpc.Dial is deprecated
		if err != nil {
			log.Fatal("Error dialing: ", err)
		}
		client := ptraceotlp.NewGRPCClient(conn)

		pack := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
			{
				LibName:    "test",
				LibVersion: "0.1t",
				Attributes: map[string]interface{}{"service.name": "pylons"},
				Spans:      testSpans,
			},
		})
		_, err = client.Export(context.Background(), pack)
		if err != nil {
			log.Fatal("Error calling: ", err)
		}

		waitForStatsAndTraces(t, &r, 30*time.Second, func(v *pb.StatsPayload) {
			assert.Len(t, v.Stats, 1)
			assert.Len(t, v.Stats[0].Stats, 1)
			assert.Len(t, v.Stats[0].Stats[0].Stats, 3)
			var serverStats, clientStats, producerStats *pb.ClientGroupedStats
			for _, cgs := range v.Stats[0].Stats[0].Stats {
				switch cgs.SpanKind {
				case "server":
					serverStats = cgs
				case "client":
					clientStats = cgs
				case "producer":
					producerStats = cgs
				}
			}
			if serverStats == nil || clientStats == nil || producerStats == nil {
				t.Fatalf("Expected stats are missing from payload. serverStats: %v, clientStats: %v, producerStats: %v", serverStats, clientStats, producerStats)
			}
			assert.Equal(t, "/path", serverStats.Resource)
			assert.Equal(t, "server", serverStats.SpanKind)
			assert.EqualValues(t, 1, serverStats.TopLevelHits)
			assert.Equal(t, "request", clientStats.Resource)
			assert.Equal(t, "client", clientStats.SpanKind)
			assert.EqualValues(t, 0, clientStats.TopLevelHits)
			assert.Equal(t, "producer", producerStats.Resource)
			assert.Equal(t, "producer", producerStats.SpanKind)
			assert.EqualValues(t, 1, producerStats.TopLevelHits)
		}, topLevelSpansAgentFn)
	})

	t.Run("disable-top-level-by-span-kind", func(t *testing.T) {
		port := testutil.FreeTCPPort(t)
		c := fmt.Sprintf(`
otlp_config:
  traces:
    internal_port: %d
  receiver:
    grpc:
      endpoint: 0.0.0.0:5111
apm_config:
  env: my-env
  compute_stats_by_span_kind: false
`, port)
		if err := r.RunAgent([]byte(c)); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", port), grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials())) //nolint:staticcheck // TODO (ASC) fix grpc.Dial is deprecated
		if err != nil {
			log.Fatal("Error dialing: ", err)
		}
		client := ptraceotlp.NewGRPCClient(conn)
		pack := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
			{
				LibName:    "test",
				LibVersion: "0.1t",
				Attributes: map[string]interface{}{"service.name": "pylons"},
				Spans:      testSpans,
			},
		})
		_, err = client.Export(context.Background(), pack)
		if err != nil {
			log.Fatal("Error calling: ", err)
		}

		waitForStatsAndTraces(t, &r, 30*time.Second, func(v *pb.StatsPayload) {
			assert.Len(t, v.Stats, 1)
			assert.Len(t, v.Stats[0].Stats, 1)
			assert.Len(t, v.Stats[0].Stats[0].Stats, 2)
			var serverStats, producerStats *pb.ClientGroupedStats
			for _, cgs := range v.Stats[0].Stats[0].Stats {
				switch cgs.SpanKind {
				case "server":
					serverStats = cgs
				case "producer":
					producerStats = cgs
				}
			}
			if serverStats == nil || producerStats == nil {
				t.Fatalf("Expected stats are missing from payload. serverStats: %v, producerStats: %v", serverStats, producerStats)
			}
			assert.Equal(t, "/path", serverStats.Resource)
			assert.Equal(t, "server", serverStats.SpanKind)
			assert.EqualValues(t, 1, serverStats.TopLevelHits)
			assert.Equal(t, "producer", producerStats.Resource)
			assert.Equal(t, "producer", producerStats.SpanKind)
			assert.EqualValues(t, 1, producerStats.TopLevelHits)
		}, topLevelSpansAgentFn)
	})
}

// waitForStatsAndTraces waits on the out channel until it times out or receives both pb.StatsPayload and pb.AgentPayload.
// If the latter happens it will call statsFn and agentFn.
func waitForStatsAndTraces(t *testing.T, runner *test.Runner, wait time.Duration, statsFn func(payload *pb.StatsPayload), agentFn func(*pb.AgentPayload)) {
	timeout := time.After(wait)
	out := runner.Out()
	var gott, gots bool
	for {
		select {
		case p := <-out:
			if v, ok := p.(*pb.StatsPayload); ok {
				statsFn(v)
				gots = true
			}
			if v, ok := p.(*pb.AgentPayload); ok {
				agentFn(v)
				gott = true
			}
			if gott && gots {
				return
			}
		case <-timeout:
			t.Fatalf("timed out, log was:\n%s", runner.AgentLog())
		}
	}
}

// duplicateSpanIDV1Log matches the log line of the v1 (idx) normalizer for a trace of
// the "pylons" service with a duplicate span ID.
var duplicateSpanIDV1Log = regexp.MustCompile(`Found malformed trace with duplicate span ID \(reason:duplicate_span_id\): SpanID: \d+, Service: pylons, Name: `)

// otlpSpanStrAttrs returns, for every tracer payload in p, for every chunk, the value
// of the string attribute (legacy span meta) key of every span. It reads both the
// legacy payloads (convert-traces disabled) and the string-indexed payloads, which
// carry OTLP traffic by default; the legacy payloads come first.
func otlpSpanStrAttrs(p *pb.AgentPayload, key string) [][][]string {
	var tps [][][]string
	for _, tp := range p.TracerPayloads {
		var chunks [][]string
		for _, chunk := range tp.Chunks {
			values := make([]string, 0, len(chunk.Spans))
			for _, span := range chunk.Spans {
				values = append(values, span.Meta[key])
			}
			chunks = append(chunks, values)
		}
		tps = append(tps, chunks)
	}
	for _, tp := range p.IdxTracerPayloads {
		var chunks [][]string
		for _, chunk := range tp.Chunks {
			values := make([]string, 0, len(chunk.Spans))
			for _, span := range chunk.Spans {
				value, _ := idxStrAttr(tp.Strings, span.Attributes, key)
				values = append(values, value)
			}
			chunks = append(chunks, values)
		}
		tps = append(tps, chunks)
	}
	return tps
}

// otlpSpanEvents returns the span events of the span of the given service in p, by event name,
// with each attribute value rendered as "<type>:<value>", whether the span is tagged as having
// an exception span event, and whether the span was found. It reads both the v0.4 and the
// string-indexed payload formats.
func otlpSpanEvents(p *pb.AgentPayload, service string) (events map[string]map[string]string, hasException, found bool) {
	for _, tp := range p.TracerPayloads {
		for _, chunk := range tp.Chunks {
			for _, span := range chunk.Spans {
				if span.Service != service {
					continue
				}
				events = make(map[string]map[string]string, len(span.SpanEvents))
				for _, e := range span.SpanEvents {
					attrs := make(map[string]string, len(e.Attributes))
					for k, v := range e.Attributes {
						attrs[k] = renderAttributeAnyValue(v)
					}
					events[e.Name] = attrs
				}
				return events, span.Meta["_dd.span_events.has_exception"] == "true", true
			}
		}
	}
	for _, tp := range p.IdxTracerPayloads {
		for _, chunk := range tp.Chunks {
			for _, span := range chunk.Spans {
				if idxStr(tp.Strings, span.ServiceRef) != service {
					continue
				}
				events = make(map[string]map[string]string, len(span.Events))
				for _, e := range span.Events {
					attrs := make(map[string]string, len(e.Attributes))
					for k, v := range e.Attributes {
						attrs[idxStr(tp.Strings, k)] = renderIdxAnyValue(tp.Strings, v)
					}
					events[idxStr(tp.Strings, e.NameRef)] = attrs
				}
				v, _ := idxStrAttr(tp.Strings, span.Attributes, "_dd.span_events.has_exception")
				return events, v == "true", true
			}
		}
	}
	return nil, false, false
}

func renderAttributeAnyValue(v *pb.AttributeAnyValue) string {
	switch v.Type {
	case pb.AttributeAnyValue_STRING_VALUE:
		return "string:" + v.StringValue
	case pb.AttributeAnyValue_BOOL_VALUE:
		return fmt.Sprintf("bool:%t", v.BoolValue)
	case pb.AttributeAnyValue_INT_VALUE:
		return fmt.Sprintf("int:%d", v.IntValue)
	case pb.AttributeAnyValue_DOUBLE_VALUE:
		return fmt.Sprintf("double:%g", v.DoubleValue)
	case pb.AttributeAnyValue_ARRAY_VALUE:
		var values []string
		for _, e := range v.GetArrayValue().GetValues() {
			values = append(values, renderAttributeArrayValue(e))
		}
		return "array:[" + strings.Join(values, " ") + "]"
	}
	return fmt.Sprintf("unknown:%v", v)
}

func renderAttributeArrayValue(v *pb.AttributeArrayValue) string {
	switch v.Type {
	case pb.AttributeArrayValue_STRING_VALUE:
		return "string:" + v.StringValue
	case pb.AttributeArrayValue_BOOL_VALUE:
		return fmt.Sprintf("bool:%t", v.BoolValue)
	case pb.AttributeArrayValue_INT_VALUE:
		return fmt.Sprintf("int:%d", v.IntValue)
	case pb.AttributeArrayValue_DOUBLE_VALUE:
		return fmt.Sprintf("double:%g", v.DoubleValue)
	}
	return fmt.Sprintf("unknown:%v", v)
}

func renderIdxAnyValue(strs []string, v *idx.AnyValue) string {
	switch val := v.Value.(type) {
	case *idx.AnyValue_StringValueRef:
		return "string:" + idxStr(strs, val.StringValueRef)
	case *idx.AnyValue_BoolValue:
		return fmt.Sprintf("bool:%t", val.BoolValue)
	case *idx.AnyValue_IntValue:
		return fmt.Sprintf("int:%d", val.IntValue)
	case *idx.AnyValue_DoubleValue:
		return fmt.Sprintf("double:%g", val.DoubleValue)
	case *idx.AnyValue_ArrayValue:
		var values []string
		for _, e := range val.ArrayValue.GetValues() {
			values = append(values, renderIdxAnyValue(strs, e))
		}
		return "array:[" + strings.Join(values, " ") + "]"
	}
	return fmt.Sprintf("unknown:%v", v)
}
