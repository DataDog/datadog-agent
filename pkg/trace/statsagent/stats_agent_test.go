// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package statsagent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/ptrace"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

func generateTraces(ctx context.Context, traceCount int, spanCount int, attrCount int, attrLength int) ptrace.Traces {
	traces := make([]testutil.OTLPResourceSpan, traceCount)
	for k := 0; k < traceCount; k++ {
		spans := make([]*testutil.OTLPSpan, spanCount)
		for i := 0; i < spanCount; i++ {
			attributes := make(map[string]interface{})
			for j := 0; j < attrCount; j++ {
				attributes["key_"+strconv.Itoa(j)] = strings.Repeat("x", 100)
			}

			spans[i] = &testutil.OTLPSpan{
				Name:       "/path",
				TraceState: "state",
				Kind:       ptrace.SpanKindServer,
				Attributes: attributes,
				StatusCode: ptrace.StatusCodeOk,
			}
		}
		rattributes := make(map[string]interface{})
		for j := 0; j < attrCount; j++ {
			rattributes["key_"+strconv.Itoa(j)] = strings.Repeat("x", 100)
		}
		rattributes["service.name"] = "test-service"
		rattributes["deployment.environment"] = "test-env"
		rspans := testutil.OTLPResourceSpan{
			Spans:      spans,
			LibName:    "stats-agent-test",
			LibVersion: "0.0.1",
			Attributes: rattributes,
		}
		traces[k] = rspans
	}
	req := testutil.NewOTLPTracesRequest(traces)
	return req.Traces()
}

func generateDDTraces(traceCount int, spanCount int, attrCount int, attrLength int) *api.Payload {
	now := time.Now().UnixNano()
	rs := info.NewReceiverStats()
	ts := rs.GetTagStats(info.Tags{
		Lang:          "go",
		LangVersion:   "1.20",
		TracerVersion: "1.0",
	})
	payload := &api.Payload{
		Source: ts,
		TracerPayload: &pb.TracerPayload{
			ContainerID:     "",
			LanguageName:    "go",
			LanguageVersion: "1.20",
			TracerVersion:   "1.0",
			RuntimeID:       "abc",
			Chunks:          nil,
			Env:             "test-env",
			Hostname:        "test-host",
			AppVersion:      "",
		},
	}
	for tc_i := 0; tc_i < traceCount; tc_i++ {
		traceChunk := &pb.TraceChunk{
			Priority: 2,
			Origin:   "someorigin",
		}
		for s_i := 0; s_i < spanCount; s_i++ {
			meta := map[string]string{}
			for j := 0; j < attrCount; j++ {
				meta["key_"+strconv.Itoa(j)] = strings.Repeat("x", attrLength)
			}
			span := &pb.Span{
				Service:  "test-service",
				Name:     "/path",
				Resource: "/path",
				TraceID:  uint64(123 + tc_i),
				SpanID:   uint64(555 + s_i),
				ParentID: uint64(0),
				Start:    now,
				Duration: 200_000_000,
				Meta:     meta,
				Metrics:  nil,
				Type:     "web",
			}
			traceChunk.Spans = append(traceChunk.Spans, span)
		}
		payload.TracerPayload.Chunks = append(payload.TracerPayload.Chunks, traceChunk)
	}
	return payload
}

func createAgent(ctx context.Context, out chan *pb.StatsPayload) (StatsAgent, error) {
	cfg := &StatsAgentConfig{
		ComputeStatsBySpanKind: true,
		PeerTagsAggregation:    true,
	}
	agent, err := New(ctx, cfg, out, &statsd.NoOpClient{})
	if err != nil {
		return nil, fmt.Errorf("failed to create stats agent: %v", err)
	}
	return agent, nil
}

func TestStatsAgent(t *testing.T) {
	out := make(chan *pb.StatsPayload, 1)
	ctx := context.Background()
	agent, err := createAgent(ctx, out)
	if err != nil {
		t.Fatalf("failed to create stats agent: %v", err)
		return
	}

	defer agent.Stop()
	go agent.Start()
	tr := generateTraces(ctx, 1, 1, 1, 1)
	agent.ComputeStats(ctx, tr)
	timeOut := time.After(30 * time.Second)
	for {
		select {
		case stats := <-out:
			if stats == nil || len(stats.Stats) == 0 {
				continue
			}
			assert.Equal(t, stats.Stats[0].Stats[0].Stats[0].Hits, uint64(1))
			return
		case <-timeOut:
			t.Fatalf("timeout waiting for stats payload")
			return
		}
	}
}

func TestStatsAgentDD(t *testing.T) {
	out := make(chan *pb.StatsPayload, 1)
	ctx := context.Background()
	agent, err := createAgent(ctx, out)
	if err != nil {
		t.Fatalf("failed to create stats agent: %v", err)
		return
	}

	defer agent.Stop()
	go agent.Start()
	payload := generateDDTraces(1, 1, 1, 1)
	agent.ComputeDDStats(ctx, payload)
	timeOut := time.After(30 * time.Second)
	for {
		select {
		case stats := <-out:
			if stats == nil || len(stats.Stats) == 0 {
				continue
			}
			assert.Equal(t, stats.Stats[0].Stats[0].Stats[0].Hits, uint64(1))
			return
		case <-timeOut:
			t.Fatalf("timeout waiting for stats payload")
			return
		}
	}
}

func Benchmark_statsagent(b *testing.B) {
	largeTraces := generateTraces(context.Background(), 10, 100, 100, 100)
	out := make(chan *pb.StatsPayload, 1)
	ctx := context.Background()
	agent, err := createAgent(ctx, out)
	defer agent.Stop()
	agent.Start() // go?

	if err != nil {
		b.Fatalf("failed to create stats agent: %v", err)
		return
	}
	// drain the channel
	go func() {
		for {
			<-out
		}
	}()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agent.ComputeStats(ctx, largeTraces)
	}
}

func Benchmark_DD_statsagent(b *testing.B) {
	var largePayloads []*api.Payload
	for i := 0; i < b.N; i++ {
		// We need to generate payloads up front since if we don't make copies we'll end up concurrently modifying
		// the internal meta/metric map of the same span in different processing goroutines
		largePayloads = append(largePayloads, generateDDTraces(10, 100, 100, 100))
	}
	out := make(chan *pb.StatsPayload, 1)
	ctx := context.Background()
	agent, err := createAgent(ctx, out)
	defer agent.Stop()
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered in f", r)
		}
	}()
	agent.Start()

	if err != nil {
		b.Fatalf("failed to create stats agent: %v", err)
		return
	}
	// drain the channel
	go func() {
		for {
			<-out
		}
	}()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		agent.ComputeDDStats(ctx, largePayloads[i])
	}
}
