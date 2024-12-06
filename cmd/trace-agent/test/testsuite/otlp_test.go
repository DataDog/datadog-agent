// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"context"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/test"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
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
			assert.Len(p.TracerPayloads, 1)
			assert.Len(p.TracerPayloads[0].Chunks, 1)
			assert.Len(p.TracerPayloads[0].Chunks[0].Spans, 1)
			assert.Equal(p.TracerPayloads[0].Chunks[0].Spans[0].Meta["name"], "john")
		})
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
				if strings.Contains(r.AgentLog(), `Found malformed trace with duplicate span ID (reason:duplicate_span_id): service:"pylons"`) {
					break loop
				}
			}
		}
	})

	// topLevelSpansAgentFn checks that the given agent payload matches with the testSpans input
	topLevelSpansAgentFn := func(v *pb.AgentPayload) {
		var serverSpan, internalSpan, clientSpan, producerSpan *pb.Span
		for _, chunk := range v.TracerPayloads[0].Chunks {
			for _, span := range chunk.Spans {
				switch span.Meta["name"] {
				case "server":
					assert.Len(t, chunk.Spans, 3)
					serverSpan = span
				case "internal":
					assert.Len(t, chunk.Spans, 3)
					internalSpan = span
				case "client":
					assert.Len(t, chunk.Spans, 3)
					clientSpan = span
				case "producer":
					assert.Len(t, chunk.Spans, 1)
					producerSpan = span
				}
			}
		}
		assert.Equal(t, "my-env", v.Env)
		assert.Len(t, v.TracerPayloads, 1)
		assert.Len(t, v.TracerPayloads[0].Chunks, 2)
		assert.True(t, serverSpan != nil && internalSpan != nil && clientSpan != nil && producerSpan != nil)
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
