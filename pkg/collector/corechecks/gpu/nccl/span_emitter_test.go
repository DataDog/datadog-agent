// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nccl

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

func TestCollTraceIDStability(t *testing.T) {
	id1 := collTraceID("0xdeadbeef", 42)
	id2 := collTraceID("0xdeadbeef", 42)
	assert.Equal(t, id1, id2, "same inputs must produce same trace ID")
}

func TestCollTraceIDSharedAcrossRanks(t *testing.T) {
	// All ranks performing the same collective (same commID + seqNum) must share a trace ID.
	commID := "0xad56c683b0c4ef"
	seqNum := int64(100)

	id0 := collTraceID(commID, seqNum)
	id1 := collTraceID(commID, seqNum)
	id7 := collTraceID(commID, seqNum)

	assert.Equal(t, id0, id1)
	assert.Equal(t, id0, id7)
}

func TestCollSpanIDUniquePerRank(t *testing.T) {
	commID := "0xad56c683b0c4ef"
	seqNum := int64(100)

	ids := make(map[uint64]bool)
	for rank := 0; rank < 8; rank++ {
		id := collSpanID(commID, rank, seqNum)
		assert.False(t, ids[id], "span ID for rank %d collides with another rank", rank)
		ids[id] = true
	}
}

func TestCollTraceIDDiffersAcrossSeqNums(t *testing.T) {
	commID := "0xad56c683b0c4ef"
	id1 := collTraceID(commID, 1)
	id2 := collTraceID(commID, 2)
	assert.NotEqual(t, id1, id2)
}

func TestExtractTag(t *testing.T) {
	tags := []string{"env:prod", "service:pytorch-training", "pod_name:worker-0"}
	assert.Equal(t, "prod", extractTag(tags, "env"))
	assert.Equal(t, "pytorch-training", extractTag(tags, "service"))
	assert.Equal(t, "worker-0", extractTag(tags, "pod_name"))
	assert.Equal(t, "", extractTag(tags, "version"))
}

func TestExtractTagValueWithColon(t *testing.T) {
	// Tag values may contain colons (e.g. GPU UUIDs).
	tags := []string{"gpu_uuid:GPU-abc:def:123"}
	assert.Equal(t, "GPU-abc:def:123", extractTag(tags, "gpu_uuid"))
}

func TestBuildSpanFields(t *testing.T) {
	perf := &CollectivePerf{
		Collective:      "AllReduce",
		CollSN:          42,
		MsgSizeBytes:    1048576,
		ExecTimeUS:      234.5,
		TimingSource:    "kernel_gpu",
		AlgoBandwidthGB: 85.6,
		BusBandwidthGB:  71.3,
	}
	event := NCCLInspectorEvent{
		ID:              "0xdeadbeef",
		Rank:            2,
		NRanks:          8,
		Hostname:        "worker-2",
		GPUUUID:         "GPU-abc123",
		DumpTimestampUS: 1000000234, // end time; start = end - duration
		CollPerf:        perf,
	}
	tags := []string{"service:pytorch-training", "env:prod", "rank:2"}

	span := buildSpan(event, tags)

	assert.Equal(t, "pytorch-training", span.Service)
	assert.Equal(t, "AllReduce", span.Name)
	assert.Equal(t, "rank:2", span.Resource)
	assert.Equal(t, ncclSpanType, span.Type)

	// Timing: start = dump_timestamp_us*1000 - exec_time_us*1000
	expectedDurationNS := int64(234.5 * 1000)
	expectedStartNS := int64(1000000234)*1000 - expectedDurationNS
	assert.Equal(t, expectedStartNS, span.Start)
	assert.Equal(t, expectedDurationNS, span.Duration)

	assert.Equal(t, uint64(0), span.ParentID)

	assert.Equal(t, "0xdeadbeef", span.Meta["nccl.comm_id"])
	assert.Equal(t, "kernel_gpu", span.Meta["nccl.timing_source"])
	assert.Equal(t, "worker-2", span.Meta["nccl.hostname"])
	assert.Equal(t, "GPU-abc123", span.Meta["nccl.gpu_uuid"])
	assert.Equal(t, "pytorch-training", span.Meta["service"])
	assert.Equal(t, "prod", span.Meta["env"])

	assert.Equal(t, float64(1), span.Metrics["_sampling_priority_v1"])
	assert.InDelta(t, 234.5, span.Metrics["nccl.exec_time_us"], 0.001)
	assert.InDelta(t, 85.6, span.Metrics["nccl.algo_bandwidth_gbps"], 0.001)
	assert.InDelta(t, 71.3, span.Metrics["nccl.bus_bandwidth_gbps"], 0.001)
	assert.Equal(t, float64(1048576), span.Metrics["nccl.msg_size_bytes"])
	assert.Equal(t, float64(2), span.Metrics["nccl.rank"])
	assert.Equal(t, float64(8), span.Metrics["nccl.n_ranks"])

	assert.Equal(t, collTraceID("0xdeadbeef", 42), span.TraceID)
	assert.Equal(t, collSpanID("0xdeadbeef", 2, 42), span.SpanID)
}

func TestBuildSpanServiceFallback(t *testing.T) {
	perf := &CollectivePerf{Collective: "AllGather", CollSN: 1}
	event := NCCLInspectorEvent{ID: "0x1", CollPerf: perf}

	span := buildSpan(event, nil)
	assert.Equal(t, ncclSpanService, span.Service, "should fall back to 'nccl' when service tag absent")
	assert.Equal(t, "AllGather", span.Name)
	assert.Equal(t, "rank:0", span.Resource)
}

func TestEmitTracesPostsToTraceAgent(t *testing.T) {
	var gotBody []byte
	var gotContentType string
	var gotTraceCount string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotTraceCount = r.Header.Get("X-Datadog-Trace-Count")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	se := &spanEmitter{
		client: srv.Client(),
		url:    srv.URL + "/v0.4/traces",
	}

	span := &pb.Span{
		Service:  "nccl",
		Name:     "AllReduce",
		Resource: "rank:0",
		TraceID:  1,
		SpanID:   2,
		Start:    1000,
		Duration: 500,
	}
	traces := pb.Traces{pb.Trace{span}}

	err := se.emitTraces(traces)
	require.NoError(t, err)

	assert.Equal(t, "application/msgpack", gotContentType)
	assert.Equal(t, "1", gotTraceCount)
	assert.NotEmpty(t, gotBody)
}
