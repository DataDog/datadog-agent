// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"
	"net"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

// otlpAgentSpan holds the span fields compared between the v04 and V1 OTLP paths.
type otlpAgentSpan struct {
	Service, Name, Resource, Type, Env string
	Error                              bool
	Priority                           int32
}

func otlpIdxTestRequest() ptraceotlp.ExportRequest {
	now := uint64(time.Now().UnixNano())
	return testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
		{
			LibName: "lib",
			Attributes: map[string]interface{}{
				"service.name":                "checkout",
				"service.version":             "1.2.3",
				"deployment.environment.name": "prod",
				"telemetry.sdk.language":      "go",
				"telemetry.sdk.version":       "1.30.0",
			},
			Spans: []*testutil.OTLPSpan{
				{
					TraceID: [16]byte{15: 1}, SpanID: [8]byte{7: 1}, Name: "GET /users/{id}",
					Kind: ptrace.SpanKindServer, Start: now - 2e6, End: now,
					Attributes: map[string]interface{}{
						"http.request.method": "GET", "http.route": "/users/{id}", "http.response.status_code": 200,
						"_dd.git.commit.sha": "abc123",
					},
				},
				{
					TraceID: [16]byte{15: 1}, SpanID: [8]byte{7: 2}, ParentID: [8]byte{7: 1}, Name: "SELECT",
					Kind: ptrace.SpanKindClient, Start: now - 1e6, End: now,
					Attributes: map[string]interface{}{"db.system": "postgresql", "db.statement": "SELECT * FROM users WHERE id = 42"},
				},
				{
					TraceID: [16]byte{15: 2}, SpanID: [8]byte{7: 3}, Name: "work",
					Kind: ptrace.SpanKindInternal, Start: now - 1e6, End: now,
					StatusCode: ptrace.StatusCodeError, StatusMsg: "boom",
					// An int attribute: today's conversion stores it in Metrics like a double. The git
					// commit SHA is per trace in v04 but per payload in V1, so both traces carry it.
					Attributes: map[string]interface{}{"_sampling_priority_v1": 2, "_dd.git.commit.sha": "abc123"},
				},
			},
		},
	})
}

// newOTLPTestAgent returns a test agent whose OTLP receiver serves gRPC on a local port, and a
// client connected to it.
func newOTLPTestAgent(t *testing.T, features ...string) (*Agent, ptraceotlp.GRPCClient) {
	t.Helper()
	cfg := config.New()
	cfg.Endpoints[0].APIKey = "test"
	cfg.Hostname = "test-host"
	// The test reads In / InV1 only once Export returns, so the receiver must not block on them.
	cfg.TraceBuffer = 10
	for _, f := range features {
		cfg.Features[f] = struct{}{}
	}
	tr, err := attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
	require.NoError(t, err)
	cfg.OTLPReceiver.AttributesTranslator = tr
	cfg.OTLPReceiver.BindHost = "localhost"
	cfg.OTLPReceiver.GRPCPort = testutil.FreeTCPPort(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	agnt := NewTestAgent(ctx, cfg, telemetry.NewNoopCollector())
	agnt.OTLPReceiver.Start()
	t.Cleanup(agnt.OTLPReceiver.Stop)
	conn, err := grpc.NewClient(net.JoinHostPort("localhost", strconv.Itoa(cfg.OTLPReceiver.GRPCPort)), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return agnt, ptraceotlp.NewGRPCClient(conn)
}

func exportOTLP(t *testing.T, client ptraceotlp.GRPCClient) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.Export(ctx, otlpIdxTestRequest())
	require.NoError(t, err)
}

func sortOTLPAgentSpans(spans []otlpAgentSpan) {
	sort.Slice(spans, func(i, j int) bool { return spans[i].Name+spans[i].Resource < spans[j].Name+spans[j].Resource })
}

// TestOTLPIdxProcessV1 sends the same OTLP request over gRPC through the direct decoder and
// ProcessV1, and through the kill switch's pdata / v04 path and Process, and checks that the
// written spans and stats inputs agree.
func TestOTLPIdxProcessV1(t *testing.T) {
	// V1 path.
	agnt, client := newOTLPTestAgent(t)
	exportOTLP(t, client)
	select {
	case p := <-agnt.InV1:
		agnt.ProcessV1(p)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a V1 payload")
	}
	assert.Empty(t, agnt.In, "unexpected v04 payload")
	tw := agnt.TraceWriterV1.(*mockTraceWriter)
	require.Len(t, tw.payloadsV1, 1)
	tp := tw.payloadsV1[0].TracerPayload
	assert.Equal(t, "test-host", tp.Hostname())
	assert.Equal(t, "prod", tp.Env())
	assert.Equal(t, "go", tp.LanguageName())
	assert.Equal(t, "otlp-1.30.0", tp.TracerVersion())
	var gotV1 []otlpAgentSpan
	for _, c := range tp.Chunks {
		for _, s := range c.Spans {
			gotV1 = append(gotV1, otlpAgentSpan{
				Service: s.Service(), Name: s.Name(), Resource: s.Resource(), Type: s.Type(), Env: s.Env(),
				Error: s.Error(), Priority: c.Priority,
			})
		}
	}
	statsV1 := agnt.Concentrator.(*mockConcentrator).statsV1
	require.Len(t, statsV1, 1)

	// v04 path.
	agnt04, client04 := newOTLPTestAgent(t, "disable_otlp_idx")
	exportOTLP(t, client04)
	select {
	case p := <-agnt04.In:
		agnt04.Process(p)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a v04 payload")
	}
	assert.Empty(t, agnt04.InV1, "unexpected V1 payload")
	tw04 := agnt04.TraceWriter.(*mockTraceWriter)
	require.Len(t, tw04.payloads, 1)
	var got04 []otlpAgentSpan
	for _, c := range tw04.payloads[0].TracerPayload.Chunks {
		for _, s := range c.Spans {
			got04 = append(got04, otlpAgentSpan{
				Service: s.Service, Name: s.Name, Resource: s.Resource, Type: s.Type, Env: s.Meta["env"],
				Error: s.Error != 0, Priority: c.Priority,
			})
		}
	}
	stats04 := agnt04.Concentrator.(*mockConcentrator).stats
	require.Len(t, stats04, 1)

	sortOTLPAgentSpans(gotV1)
	sortOTLPAgentSpans(got04)
	assert.Equal(t, got04, gotV1)
	assert.Len(t, gotV1, 3)
	// The payload-level fields that become stats aggregation keys.
	type statsKey struct {
		AppVersion, Env, Hostname, GitCommitSha string
		Priority                                int32
	}
	var keys04, keysV1 []statsKey
	for _, pt := range stats04[0].Traces {
		keys04 = append(keys04, statsKey{pt.AppVersion, pt.TracerEnv, pt.TracerHostname, pt.GitCommitSha, pt.TraceChunk.Priority})
	}
	for _, pt := range statsV1[0].Traces {
		keysV1 = append(keysV1, statsKey{pt.AppVersion, pt.TracerEnv, pt.TracerHostname, pt.GitCommitSha, pt.TraceChunk.Priority})
	}
	sortKeys := func(keys []statsKey) {
		sort.Slice(keys, func(i, j int) bool { return keys[i].Priority < keys[j].Priority })
	}
	sortKeys(keys04)
	sortKeys(keysV1)
	assert.Equal(t, keys04, keysV1, "stats input keys")
	require.Len(t, keysV1, 2)
	assert.Equal(t, statsKey{"1.2.3", "prod", "test-host", "abc123", 1}, keysV1[0])
	assert.Equal(t, statsKey{"1.2.3", "prod", "test-host", "abc123", 2}, keysV1[1], "int _sampling_priority_v1 sets the user priority")
	// The obfuscator ran on the V1 path too.
	for _, s := range gotV1 {
		assert.NotContains(t, s.Resource, "42")
	}
}
