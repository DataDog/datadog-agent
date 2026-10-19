// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/mem"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
)

const otlpExportMethod = "/opentelemetry.proto.collector.trace.v1.TraceService/Export"

// otlpIdxHarness is an OTLPReceiver serving gRPC on a local port, with a client connected to it.
type otlpIdxHarness struct {
	rcv   *OTLPReceiver
	out   chan *Payload
	outV1 chan *PayloadV1
	stats *teststatsd.Client
	conn  *grpc.ClientConn
}

func newOTLPIdxHarness(t *testing.T, withOutV1 bool, features ...string) *otlpIdxHarness {
	t.Helper()
	cfg := NewTestConfig(t)
	cfg.Hostname = "test-host"
	for _, f := range features {
		cfg.Features[f] = struct{}{}
	}
	cfg.OTLPReceiver.BindHost = "localhost"
	cfg.OTLPReceiver.GRPCPort = testutil.FreeTCPPort(t)
	h := &otlpIdxHarness{
		out:   make(chan *Payload, 10),
		outV1: make(chan *PayloadV1, 10),
		stats: &teststatsd.Client{},
	}
	var outV1 chan<- *PayloadV1
	if withOutV1 {
		outV1 = h.outV1
	}
	h.rcv = NewOTLPReceiver(h.out, outV1, cfg, h.stats, &timing.NoopReporter{})
	h.rcv.Start()
	t.Cleanup(h.rcv.Stop)
	conn, err := grpc.NewClient(net.JoinHostPort(cfg.OTLPReceiver.BindHost, strconv.Itoa(cfg.OTLPReceiver.GRPCPort)), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	h.conn = conn
	return h
}

func (h *otlpIdxHarness) export(t *testing.T, req ptraceotlp.ExportRequest) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := ptraceotlp.NewGRPCClient(h.conn).Export(ctx, req)
	return err
}

// exportRaw sends b as the serialized request, bypassing client-side marshaling.
func (h *otlpIdxHarness) exportRaw(t *testing.T, b []byte) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var resp []byte
	return h.conn.Invoke(ctx, otlpExportMethod, &b, &resp, grpc.ForceCodecV2(rawBytesCodec{}))
}

// rawBytesCodec sends and receives *[]byte messages unchanged.
type rawBytesCodec struct{}

func (rawBytesCodec) Marshal(v any) (mem.BufferSlice, error) {
	return mem.BufferSlice{mem.SliceBuffer(*v.(*[]byte))}, nil
}

func (rawBytesCodec) Unmarshal(data mem.BufferSlice, v any) error {
	*v.(*[]byte) = data.Materialize()
	return nil
}

func (rawBytesCodec) Name() string { return "proto" }

func receiveV1(t *testing.T, ch <-chan *PayloadV1) *PayloadV1 {
	t.Helper()
	select {
	case p := <-ch:
		return p
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a V1 payload")
		return nil
	}
}

func receiveV04(t *testing.T, ch <-chan *Payload) *Payload {
	t.Helper()
	select {
	case p := <-ch:
		return p
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a v04 payload")
		return nil
	}
}

func assertNoPayloads(t *testing.T, h *otlpIdxHarness) {
	t.Helper()
	assert.Empty(t, h.out, "unexpected v04 payload")
	assert.Empty(t, h.outV1, "unexpected V1 payload")
}

func gaugeLast(stats *teststatsd.Client, name string) float64 {
	if g, ok := stats.GetGaugeSummaries()[name]; ok {
		return g.Last
	}
	return -1
}

func countSum(stats *teststatsd.Client, name string) int64 {
	if c, ok := stats.GetCountSummaries()[name]; ok {
		return c.Sum
	}
	return 0
}

func TestOTLPIdxGRPCExport(t *testing.T) {
	h := newOTLPIdxHarness(t, true)
	assert.Equal(t, 1.0, gaugeLast(h.stats, "datadog.trace_agent.otlp.idx_enabled"))
	svc, ok := h.rcv.grpcsrv.GetServiceInfo()["opentelemetry.proto.collector.trace.v1.TraceService"]
	require.True(t, ok)
	assert.Equal(t, "Export", svc.Methods[0].Name)

	req := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
		{
			LibName: "lib",
			Attributes: map[string]interface{}{
				"service.name":           "svc-a",
				"telemetry.sdk.language": "go",
				"telemetry.sdk.version":  "1.2.3",
				"_dd.stats_computed":     "true",
			},
			Spans: []*testutil.OTLPSpan{
				{Name: "a1", TraceID: [16]byte{15: 1}, SpanID: [8]byte{7: 1}},
				{Name: "a2", TraceID: [16]byte{15: 1}, SpanID: [8]byte{7: 2}},
				{Name: "a3", TraceID: [16]byte{15: 2}, SpanID: [8]byte{7: 3}},
			},
		},
		{
			LibName:    "lib",
			Attributes: map[string]interface{}{"service.name": "svc-b"},
			Spans:      []*testutil.OTLPSpan{{Name: "b1", TraceID: [16]byte{15: 3}, SpanID: [8]byte{7: 4}}},
		},
	})
	require.NoError(t, h.export(t, req))

	a := receiveV1(t, h.outV1)
	b := receiveV1(t, h.outV1)
	assertNoPayloads(t, h)

	assert.Equal(t, "opentelemetry_grpc_v1", a.Source.EndpointVersion)
	assert.Equal(t, "go", a.Source.Lang)
	assert.Equal(t, "otlp-1.2.3", a.Source.TracerVersion)
	assert.True(t, a.ClientComputedStats, "_dd.stats_computed resource attribute")
	assert.False(t, a.ClientComputedTopLevel)
	assert.Len(t, a.TracerPayload.Chunks, 2)
	assert.Equal(t, "test-host", a.TracerPayload.Hostname())
	assert.Equal(t, "svc-a", a.TracerPayload.Chunks[0].Spans[0].Service())

	assert.Equal(t, "otlp-", b.Source.TracerVersion)
	assert.False(t, b.ClientComputedStats)
	assert.Len(t, b.TracerPayload.Chunks, 1)
	assert.Equal(t, "svc-b", b.TracerPayload.Chunks[0].Spans[0].Service())

	assert.Equal(t, int64(1), countSum(h.stats, "datadog.trace_agent.otlp.payload"))
	assert.Equal(t, int64(4), countSum(h.stats, "datadog.trace_agent.otlp.spans"))
	assert.Equal(t, int64(3), countSum(h.stats, "datadog.trace_agent.otlp.traces"))
	assert.Equal(t, int64(0), countSum(h.stats, "datadog.trace_agent.otlp.idx_fallback"))
}

func TestOTLPIdxComputedTopLevel(t *testing.T) {
	h := newOTLPIdxHarness(t, true, "enable_otlp_compute_top_level_by_span_kind")
	req := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
		{Attributes: map[string]interface{}{}, Spans: []*testutil.OTLPSpan{{Name: "s"}}},
	})
	require.NoError(t, h.export(t, req))
	assert.True(t, receiveV1(t, h.outV1).ClientComputedTopLevel)
}

// TestOTLPIdxGate checks that the kill switch, legacy options the decoder doesn't implement, and
// a receiver without a V1 channel keep the pdata / v04 path.
func TestOTLPIdxGate(t *testing.T) {
	for _, tt := range []struct {
		name      string
		withOutV1 bool
		features  []string
	}{
		{name: "kill switch", withOutV1: true, features: []string{"disable_otlp_idx"}},
		{name: "legacy receive resource spans", withOutV1: true, features: []string{"disable_receive_resource_spans_v2"}},
		{name: "legacy container tags", withOutV1: true, features: []string{"disable_otlp_container_tags_v2"}},
		{name: "no V1 channel"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newOTLPIdxHarness(t, tt.withOutV1, tt.features...)
			assert.Equal(t, 0.0, gaugeLast(h.stats, "datadog.trace_agent.otlp.idx_enabled"))
			req := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
				{Attributes: map[string]interface{}{"service.name": "svc"}, Spans: []*testutil.OTLPSpan{{Name: "s"}}},
			})
			require.NoError(t, h.export(t, req))
			p := receiveV04(t, h.out)
			assert.Equal(t, "svc", p.TracerPayload.Chunks[0].Spans[0].Service)
			assertNoPayloads(t, h)
		})
	}
}

// TestOTLPIdxFallback checks that a request the direct decoder rejects but pdata accepts is still
// processed, through the v04 path.
func TestOTLPIdxFallback(t *testing.T) {
	h := newOTLPIdxHarness(t, true)
	// Nesting deeper than the decoder's limit, which pdata accepts.
	var nested interface{} = "leaf"
	for range 40 {
		nested = map[string]interface{}{"k": nested}
	}
	req := testutil.NewOTLPTracesRequest([]testutil.OTLPResourceSpan{
		{
			Attributes: map[string]interface{}{"service.name": "svc"},
			Spans:      []*testutil.OTLPSpan{{Name: "s", Attributes: map[string]interface{}{"deep": nested}}},
		},
	})
	b, err := req.MarshalProto()
	require.NoError(t, err)
	require.NoError(t, ptraceotlp.NewExportRequest().UnmarshalProto(b), "pdata must accept the request")

	require.NoError(t, h.exportRaw(t, b))
	p := receiveV04(t, h.out)
	assert.Equal(t, "svc", p.TracerPayload.Chunks[0].Spans[0].Service)
	assertNoPayloads(t, h)
	assert.Equal(t, int64(1), countSum(h.stats, "datadog.trace_agent.otlp.idx_fallback"))
}

// TestOTLPIdxInvalidRequest checks that bytes neither decoder accepts fail the RPC, as they do
// with the pdata codec, and emit nothing.
func TestOTLPIdxInvalidRequest(t *testing.T) {
	h := newOTLPIdxHarness(t, true)
	// A length-delimited ResourceSpans field whose length exceeds the message.
	err := h.exportRaw(t, []byte{0x0a, 0x10, 0x01})
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
	assertNoPayloads(t, h)
}
