// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testsuite

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
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

// otlpIngestMode is a trace-agent OTLP ingest path: the direct decoder into the V1 (idx)
// representation, which is the default, or the pdata / v04 path behind the disable_otlp_idx kill
// switch.
type otlpIngestMode struct {
	name     string
	features []string
	idx      bool
}

var otlpIngestModes = []otlpIngestMode{
	{name: "idx", idx: true},
	{name: "v04", features: []string{"disable_otlp_idx"}},
}

// otlpAgentConfig returns the agent configuration for an OTLP test on the given internal port,
// with the mode's features and extra apm_config lines.
func otlpAgentConfig(mode otlpIngestMode, port int, features []string, apmConfig string) []byte {
	var quoted []string
	for _, f := range append(append([]string(nil), mode.features...), features...) {
		quoted = append(quoted, strconv.Quote(f))
	}
	return []byte(fmt.Sprintf(`
otlp_config:
  traces:
    internal_port: %d
  receiver:
    grpc:
      endpoint: 0.0.0.0:5111
apm_config:
  env: my-env
  features: [%s]
%s`, port, strings.Join(quoted, ", "), apmConfig))
}

// otlpPayloadSpans returns the "name" attribute of every span, grouped by chunk, of the agent
// payload's single tracer payload, checking it was written in the mode's format.
func otlpPayloadSpans(t *testing.T, mode otlpIngestMode, p *pb.AgentPayload) [][]string {
	t.Helper()
	var chunks [][]string
	if mode.idx {
		assert.Empty(t, p.TracerPayloads, "v04 tracer payloads in idx mode")
		if !assert.Len(t, p.IdxTracerPayloads, 1) {
			return nil
		}
		tp := p.IdxTracerPayloads[0]
		for _, c := range tp.Chunks {
			var names []string
			for _, s := range c.Spans {
				name, _ := idxStrAttr(tp.Strings, s.Attributes, "name")
				names = append(names, name)
			}
			chunks = append(chunks, names)
		}
		return chunks
	}
	assert.Empty(t, p.IdxTracerPayloads, "idx tracer payloads in v04 mode")
	if !assert.Len(t, p.TracerPayloads, 1) {
		return nil
	}
	for _, c := range p.TracerPayloads[0].Chunks {
		var names []string
		for _, s := range c.Spans {
			names = append(names, s.Meta["name"])
		}
		chunks = append(chunks, names)
	}
	return chunks
}

// duplicateSpanIDLogged reports whether the agent log has the normalizer's duplicate span ID
// warning for the pylons service. The v04 and V1 normalizers format the span differently.
func duplicateSpanIDLogged(mode otlpIngestMode, agentLog string) bool {
	const prefix = "Found malformed trace with duplicate span ID (reason:duplicate_span_id): "
	for _, line := range strings.Split(agentLog, "\n") {
		i := strings.Index(line, prefix)
		if i < 0 {
			continue
		}
		rest := line[i+len(prefix):]
		if mode.idx && strings.Contains(rest, "Service: pylons,") || !mode.idx && strings.HasPrefix(rest, `service:"pylons"`) {
			return true
		}
	}
	return false
}

func otlpClient(t *testing.T, port int) ptraceotlp.GRPCClient {
	t.Helper()
	conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", port), grpc.WithBlock(), grpc.WithTransportCredentials(insecure.NewCredentials())) //nolint:staticcheck // TODO (ASC) fix grpc.Dial is deprecated
	if err != nil {
		log.Fatal("Error dialing: ", err)
	}
	t.Cleanup(func() { conn.Close() })
	return ptraceotlp.NewGRPCClient(conn)
}

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
	for _, mode := range otlpIngestModes {
		t.Run(mode.name, func(t *testing.T) {
			testOTLPIngest(t, &r, mode)
		})
	}
}

func testOTLPIngest(t *testing.T, r *test.Runner, mode otlpIngestMode) {
	t.Run("passthrough", func(t *testing.T) {
		port := testutil.FreeTCPPort(t)
		if err := r.RunAgent(otlpAgentConfig(mode, port, nil, "")); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		client := otlpClient(t, port)
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
		_, err := client.Export(context.Background(), pack)
		if err != nil {
			log.Fatal("Error calling: ", err)
		}
		waitForTrace(t, r, func(p *pb.AgentPayload) {
			assert.Equal(t, "my-env", p.Env)
			assert.Equal(t, [][]string{{"john"}}, otlpPayloadSpans(t, mode, p))
		})
	})

	// regression test for DataDog/datadog-agent#11297
	t.Run("duplicate-spanID", func(t *testing.T) {
		port := testutil.FreeTCPPort(t)
		if err := r.RunAgent(otlpAgentConfig(mode, port, nil, "")); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		client := otlpClient(t, port)
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
		_, err := client.Export(context.Background(), pack)
		if err != nil {
			log.Fatal("Error calling: ", err)
		}
		timeout := time.After(1 * time.Second)
	loop:
		for {
			select {
			case <-timeout:
				t.Fatalf("Timed out waiting for duplicate SpanID warning, log was:\n%s", r.AgentLog())
			default:
				time.Sleep(10 * time.Millisecond)
				if duplicateSpanIDLogged(mode, r.AgentLog()) {
					break loop
				}
			}
		}
	})

	// topLevelSpansAgentFn checks that the given agent payload matches with the testSpans input
	topLevelSpansAgentFn := func(v *pb.AgentPayload) {
		assert.Equal(t, "my-env", v.Env)
		chunks := otlpPayloadSpans(t, mode, v)
		for _, c := range chunks {
			sort.Strings(c)
		}
		sort.Slice(chunks, func(i, j int) bool { return len(chunks[i]) < len(chunks[j]) })
		assert.Equal(t, [][]string{{"producer"}, {"client", "internal", "server"}}, chunks)
	}

	t.Run("top-level-by-span-kind", func(t *testing.T) {
		port := testutil.FreeTCPPort(t)
		if err := r.RunAgent(otlpAgentConfig(mode, port, []string{"enable_otlp_compute_top_level_by_span_kind"}, "")); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		client := otlpClient(t, port)
		pack := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
			{
				LibName:    "test",
				LibVersion: "0.1t",
				Attributes: map[string]interface{}{"service.name": "pylons"},
				Spans:      testSpans,
			},
		})
		_, err := client.Export(context.Background(), pack)
		if err != nil {
			log.Fatal("Error calling: ", err)
		}

		waitForStatsAndTraces(t, r, 30*time.Second, func(v *pb.StatsPayload) {
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
		if err := r.RunAgent(otlpAgentConfig(mode, port, nil, "  compute_stats_by_span_kind: false\n")); err != nil {
			t.Fatal(err)
		}
		defer r.KillAgent()

		client := otlpClient(t, port)
		pack := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
			{
				LibName:    "test",
				LibVersion: "0.1t",
				Attributes: map[string]interface{}{"service.name": "pylons"},
				Spans:      testSpans,
			},
		})
		_, err := client.Export(context.Background(), pack)
		if err != nil {
			log.Fatal("Error calling: ", err)
		}

		waitForStatsAndTraces(t, r, 30*time.Second, func(v *pb.StatsPayload) {
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
