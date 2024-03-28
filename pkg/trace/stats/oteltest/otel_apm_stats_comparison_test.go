// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oteltest

import (
	"context"
	"testing"
	"time"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"github.com/google/go-cmp/cmp"
	"github.com/tj/assert"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	semconv "go.opentelemetry.io/collector/semconv/v1.6.1"
	"go.opentelemetry.io/otel/metric/noop"
	"google.golang.org/protobuf/testing/protocmp"
)

// Comparison test to ensure APM stats generated from 2 different OTel ingestion paths are consistent.
func TestOTelAPMStatsMatch(t *testing.T) {
	ctx := context.Background()
	set := componenttest.NewNopTelemetrySettings()
	set.MeterProvider = noop.NewMeterProvider()
	attributesTranslator, err := attributes.NewTranslator(set)
	assert.NoError(t, err)
	tcfg := getTraceAgentCfg(attributesTranslator)

	// Set up 2 output channels for APM stats, and start 2 fake trace agent to conduct a comparison test
	now := time.Now()
	statschan1 := make(chan *pb.StatsPayload, 100)
	fakeAgent1 := getAndStartFakeAgent(ctx, tcfg, statschan1, now)
	defer fakeAgent1.Stop()
	statschan2 := make(chan *pb.StatsPayload, 100)
	fakeAgent2 := getAndStartFakeAgent(ctx, tcfg, statschan2, now)
	defer fakeAgent2.Stop()

	traces := getTestTraces()

	// fakeAgent1 has OTLP traces go through the old pipeline: ReceiveResourceSpan -> TraceWriter -> ... ->  Concentrator.Run
	fakeAgent1.Ingest(ctx, traces)

	// fakeAgent2 calls the new API in Concentrator that directly calculates APM stats for OTLP traces
	fakeAgent2.Concentrator.ProcessOTLPTraces(traces)

	// Verify APM stats generated from 2 paths are consistent
	var sps1 []*pb.StatsPayload
	var sps2 []*pb.StatsPayload
	var cgss1 []*pb.ClientGroupedStats
	var cgss2 []*pb.ClientGroupedStats
	for len(sps1) < 1 || len(sps2) < 1 {
		select {
		case sp1 := <-statschan1:
			if len(sp1.Stats) > 0 {
				assert.Len(t, sp1.Stats, 1)
				assert.Len(t, sp1.Stats[0].Stats, 1)
				assert.Len(t, sp1.Stats[0].Stats[0].Stats, 3)
				cgss1 = sp1.Stats[0].Stats[0].Stats
				sps1 = append(sps1, sp1)
			}
		case sp2 := <-statschan2:
			if len(sp2.Stats) > 0 {
				assert.Len(t, sp2.Stats, 1)
				assert.Len(t, sp2.Stats[0].Stats, 1)
				assert.Len(t, sp2.Stats[0].Stats[0].Stats, 3)
				cgss2 = sp2.Stats[0].Stats[0].Stats
				sps2 = append(sps2, sp2)
			}
		}
	}

	if diff := cmp.Diff(
		sps1,
		sps2,
		protocmp.Transform(),
		// ProcessOTLPTraces adds container tags to ClientStatsPayload, other fields should match.
		protocmp.IgnoreFields(&pb.ClientStatsPayload{}, "tags"),
		// []*ClientGroupedStats can be out of order
		protocmp.IgnoreFields(&pb.ClientStatsBucket{}, "stats")); diff != "" {
		t.Errorf("Diff between APM stats received:\n%v", diff)
	}
	assert.ElementsMatch(t, cgss1, cgss2)
	assert.ElementsMatch(t, sps2[0].Stats[0].Tags, []string{"kube_container_name:k8s_container", "container_id:test_cid", "kube_cluster_name:k8s_cluster"})
}

func getTraceAgentCfg(attributesTranslator *attributes.Translator) *traceconfig.AgentConfig {
	acfg := traceconfig.New()
	acfg.OTLPReceiver.AttributesTranslator = attributesTranslator
	acfg.ComputeStatsBySpanKind = true
	acfg.PeerTagsAggregation = true
	acfg.Features["enable_cid_stats"] = struct{}{}
	return acfg
}

func getAndStartFakeAgent(ctx context.Context, tcfg *traceconfig.AgentConfig, statschan chan *pb.StatsPayload, now time.Time) *TraceAgent {
	fakeAgent := NewAgentWithConfig(ctx, tcfg, statschan, now)
	fakeAgent.Start()
	return fakeAgent
}

var (
	traceId = [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanId1 = [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	spanId2 = [8]byte{2, 2, 3, 4, 5, 6, 7, 8}
	spanId3 = [8]byte{3, 2, 3, 4, 5, 6, 7, 8}
)

func getTestTraces() ptrace.Traces {
	traces := ptrace.NewTraces()
	rspan := traces.ResourceSpans().AppendEmpty()
	rattrs := rspan.Resource().Attributes()
	rattrs.PutStr(semconv.AttributeContainerID, "test_cid")
	rattrs.PutStr(semconv.AttributeServiceName, "test_SerVIce!@#$%")
	rattrs.PutStr(semconv.AttributeDeploymentEnvironment, "teSt_eNv^&*()")
	rattrs.PutStr(semconv.AttributeK8SClusterName, "k8s_cluster")
	rattrs.PutStr(semconv.AttributeK8SNodeName, "k8s_node")
	rattrs.PutStr(semconv.AttributeK8SContainerName, "k8s_container")

	sspan := rspan.ScopeSpans().AppendEmpty()

	root := sspan.Spans().AppendEmpty()
	root.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	root.SetKind(ptrace.SpanKindClient)
	root.SetName("root")
	root.SetTraceID(traceId)
	root.SetSpanID(spanId1)
	rootattrs := root.Attributes()
	rootattrs.PutStr("resource.name", "test_resource")
	rootattrs.PutStr("operation.name", "test_opeR@aT^&*ion")
	rootattrs.PutInt(semconv.AttributeHTTPStatusCode, 404)
	rootattrs.PutStr(semconv.AttributePeerService, "test_pe^er_seR%Vice")
	rootattrs.PutStr(semconv.AttributeDBSystem, "redis")
	root.Status().SetCode(ptrace.StatusCodeError)

	child1 := sspan.Spans().AppendEmpty()
	child1.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	child1.SetKind(ptrace.SpanKindServer) // OTel spans with SpanKindServer are top-level
	child1.SetName("child1")
	child1.SetTraceID(traceId)
	child1.SetSpanID(spanId2)
	child1.SetParentSpanID(spanId1)
	child1attrs := child1.Attributes()
	child1attrs.PutInt(semconv.AttributeHTTPStatusCode, 200)
	child1attrs.PutStr(semconv.AttributeHTTPMethod, "GET")
	child1attrs.PutStr(semconv.AttributeHTTPRoute, "/home")
	child1.Status().SetCode(ptrace.StatusCodeError)
	child1.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now()))

	child2 := sspan.Spans().AppendEmpty()
	child2.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	child2.SetKind(ptrace.SpanKindProducer) // OTel spans with SpanKindProducer get APM stats
	child2.SetName("child2")
	child2.SetTraceID(traceId)
	child2.SetSpanID(spanId3)
	child2.SetParentSpanID(spanId1)
	child2attrs := child2.Attributes()
	child2attrs.PutStr(semconv.AttributeRPCMethod, "test_method")
	child2attrs.PutStr(semconv.AttributeRPCService, "test_rpc_svc")
	child2.Status().SetCode(ptrace.StatusCodeError)
	child2.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now()))

	root.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now()))

	return traces
}
