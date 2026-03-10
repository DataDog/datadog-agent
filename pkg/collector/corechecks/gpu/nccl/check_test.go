// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nccl

import (
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func TestProcessEventCreatesCorrectTags(t *testing.T) {
	// Test buildTags function directly
	c := &Check{}

	event := NCCLInspectorEvent{
		PID:      12345,
		Rank:     0,
		NRanks:   8,
		Hostname: "worker-0",
		GPUUUID:  "GPU-abc123",
	}

	tags := c.buildTags(ParsedEvent{Event: event})

	// Without processTagger, should just have PID tag
	assert.Contains(t, tags, "pid:12345")
}

func TestBuildTagsWithoutProcessTagger(t *testing.T) {
	c := &Check{}

	event := NCCLInspectorEvent{
		PID:  12345,
		Rank: 3,
	}

	tags := c.buildTags(ParsedEvent{Event: event})

	// Without processTagger, should just have PID tag
	assert.Contains(t, tags, "pid:12345")
	assert.Len(t, tags, 1) // Only PID tag
}

func TestBuildTagsWithZeroPID(t *testing.T) {
	c := &Check{}

	event := NCCLInspectorEvent{
		PID:  0,
		Rank: 0,
	}

	tags := c.buildTags(ParsedEvent{Event: event})

	// PID 0 should still get a pid tag via fallback
	assert.Contains(t, tags, "pid:0")
}

func TestCheckNameConstant(t *testing.T) {
	assert.Equal(t, "nccl", CheckName)
}

// --- hang detection ---

func TestHangDetection_EmitsStaleMetricWithTags(t *testing.T) {
	snd := new(mocksender.MockSender)
	snd.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	c := &Check{
		lastSeenRank: map[string]rankStalenessEntry{
			"rank:0": {
				lastSeen: time.Now().Add(-30 * time.Second),
				parsed: ParsedEvent{
					Event: NCCLInspectorEvent{Rank: 0, PID: 100, Hostname: "worker-0"},
				},
			},
			"rank:1": {
				lastSeen: time.Now().Add(-5 * time.Second),
				parsed: ParsedEvent{
					Event: NCCLInspectorEvent{Rank: 1, PID: 200, Hostname: "worker-1"},
				},
			},
		},
	}

	now := time.Now()
	c.emitStalenessMetrics(snd, now)

	snd.AssertMetricTaggedWith(t, "Gauge", ncclMetricsNs+hangDetectionMetric, []string{"rank:0", "pid:100", "nccl_hostname:worker-0"})
	snd.AssertMetricTaggedWith(t, "Gauge", ncclMetricsNs+hangDetectionMetric, []string{"rank:1", "pid:200", "nccl_hostname:worker-1"})
}

func TestHangDetection_StalenessGrowsWithNoNewEvents(t *testing.T) {
	snd := new(mocksender.MockSender)
	snd.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	past := time.Now().Add(-60 * time.Second)
	c := &Check{
		lastSeenRank: map[string]rankStalenessEntry{
			"rank:2": {
				lastSeen: past,
				parsed: ParsedEvent{
					Event: NCCLInspectorEvent{Rank: 2, PID: 300},
				},
			},
		},
	}

	now := time.Now()
	c.emitStalenessMetrics(snd, now)

	// staleness should be ~60 seconds (at least 50 to allow for test timing)
	snd.AssertMetricInRange(t, "Gauge", ncclMetricsNs+hangDetectionMetric, 50, 70, "", []string{"rank:2"})
}

// --- intra-node rank divergence ---

func makeParsedEvent(commHash string, collSN int64, collective string, rank int, execTimeUS float64) ParsedEvent {
	return ParsedEvent{
		Event: NCCLInspectorEvent{
			ID:   commHash,
			Rank: rank,
			CollPerf: &CollectivePerf{
				Collective: collective,
				CollSN:     collSN,
				ExecTimeUS: execTimeUS,
			},
		},
		Filename: "nccl-rank0-pid1.jsonl",
	}
}

func TestRankDivergence_TwoRanksSameNode(t *testing.T) {
	snd := new(mocksender.MockSender)
	snd.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	events := []ParsedEvent{
		makeParsedEvent("hash1", 1, "AllReduce", 0, 1000.0),
		makeParsedEvent("hash1", 1, "AllReduce", 1, 2500.0), // rank 1 is slower
	}

	emitRankDivergence(snd, events)

	// divergence = 2500 - 1000 = 1500
	snd.AssertMetric(t, "Gauge", ncclMetricsNs+intraNodeDivergenceMetric, 1500.0, "",
		[]string{"collective:AllReduce", "n_ranks_observed:2"})
}

func TestRankDivergence_SingleRankNoMetric(t *testing.T) {
	snd := new(mocksender.MockSender)

	events := []ParsedEvent{
		makeParsedEvent("hash1", 1, "AllGather", 0, 500.0),
	}

	emitRankDivergence(snd, events)

	// With only one rank, divergence metric must NOT be emitted
	snd.AssertNotCalled(t, "Gauge", ncclMetricsNs+intraNodeDivergenceMetric,
		mock.Anything, mock.Anything, mock.Anything)
}

// TestSocketListenerNvidiaNestedFormatPreservesRank is a regression test for the bug
// where the socket listener called json.Unmarshal(line, &NCCLInspectorEvent{}) directly.
// NCCLInspectorEvent is a flat struct expecting top-level "rank", but the NVIDIA Inspector
// plugin sends a nested format with "rank" inside the "header" sub-object.  The flat
// unmarshal silently left Rank=0 for every event regardless of the actual rank, so only
// rank:0 ever appeared in Datadog even though all ranks were delivering events.
//
// The fix: use parseEvent() (which detects "header" and uses nvidiaInspectorEvent) instead
// of a bare json.Unmarshal.
func TestSocketListenerNvidiaNestedFormatPreservesRank(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "nccl.socket")

	sl, err := newSocketListener(socketPath)
	require.NoError(t, err)
	defer sl.Stop()

	// Real payload from worker-0 (rank:1) — copied verbatim from the NCCL Inspector dump.
	// "rank" lives inside "header", NOT at the top level of the JSON object.
	nvidiaEvent := `{"header":{"id":"0xad56c683b0c4ef","rank":1,"n_ranks":3,"nnodes":3},"metadata":{"inspector_output_format_version":"v4.0","git_rev":"standalone-build","hostname":"nanogpt-nccl-test-worker-0","pid":48,"dump_timestamp_us":1772585717263338},"coll_perf":{"coll":"AllReduce","coll_sn":8995,"coll_msg_size_bytes":2360832,"coll_exec_time_us":5663.0,"coll_timing_source":"kernel_gpu","coll_algobw_gbs":0.416887,"coll_busbw_gbs":0.555850}}`

	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	_, err = fmt.Fprintf(conn, "%s\n", nvidiaEvent)
	require.NoError(t, err)
	conn.Close()

	// Poll until the listener goroutine buffers the event.
	var events []ParsedEvent
	require.Eventually(t, func() bool {
		events = sl.Drain()
		return len(events) > 0
	}, 2*time.Second, 10*time.Millisecond)

	require.Len(t, events, 1)
	ev := events[0].Event

	// Core regression assertion: rank must be 1, not 0.
	assert.Equal(t, 1, ev.Rank,
		"rank must come from header.rank (NVIDIA nested format); flat json.Unmarshal always returned 0")

	// Also verify the other nested fields are correctly mapped.
	assert.Equal(t, "0xad56c683b0c4ef", ev.ID)
	assert.Equal(t, 48, ev.PID)
	assert.Equal(t, "nanogpt-nccl-test-worker-0", ev.Hostname)
	assert.Equal(t, 3, ev.NRanks)
	require.NotNil(t, ev.CollPerf)
	assert.Equal(t, "AllReduce", ev.CollPerf.Collective)
	assert.Equal(t, 5663.0, ev.CollPerf.ExecTimeUS)
	// Filename in ParsedEvent should reflect the actual rank, not 0.
	assert.Equal(t, "socket:rank1-pid48", events[0].Filename)
}

func TestNetworkTransferMetrics_AggregatesPerRankDirection(t *testing.T) {
	snd := new(mocksender.MockSender)
	snd.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	events := []ParsedEvent{
		// Rank 0, send, channel 0 — 5000us
		{Event: NCCLInspectorEvent{Rank: 0, ProxyOp: &ProxyOpPerf{IsSend: 1, ChannelID: 0, Peer: 1, NetTimeUS: 5000}}},
		// Rank 0, send, channel 1 — 8000us (max)
		{Event: NCCLInspectorEvent{Rank: 0, ProxyOp: &ProxyOpPerf{IsSend: 1, ChannelID: 1, Peer: 1, NetTimeUS: 8000}}},
		// Rank 0, recv, channel 0 — 3000us
		{Event: NCCLInspectorEvent{Rank: 0, ProxyOp: &ProxyOpPerf{IsSend: 0, ChannelID: 0, Peer: 2, NetTimeUS: 3000}}},
		// Rank 1, send — 2000us
		{Event: NCCLInspectorEvent{Rank: 1, ProxyOp: &ProxyOpPerf{IsSend: 1, ChannelID: 0, Peer: 0, NetTimeUS: 2000}}},
		// Coll event should be ignored
		{Event: NCCLInspectorEvent{Rank: 0, CollPerf: &CollectivePerf{Collective: "AllReduce"}}},
	}

	emitNetworkTransferMetrics(snd, events)

	// Rank 0 send: max(5000, 8000) = 8000
	snd.AssertMetric(t, "Gauge", ncclMetricsNs+networkMaxTransferTimeMetric, 8000.0, "",
		[]string{"rank:0", "direction:send"})
	// Rank 0 recv: 3000
	snd.AssertMetric(t, "Gauge", ncclMetricsNs+networkMaxTransferTimeMetric, 3000.0, "",
		[]string{"rank:0", "direction:recv"})
	// Rank 1 send: 2000
	snd.AssertMetric(t, "Gauge", ncclMetricsNs+networkMaxTransferTimeMetric, 2000.0, "",
		[]string{"rank:1", "direction:send"})
}

func TestNetworkTransferMetrics_NoProxyOpsNoMetric(t *testing.T) {
	snd := new(mocksender.MockSender)

	events := []ParsedEvent{
		{Event: NCCLInspectorEvent{Rank: 0, CollPerf: &CollectivePerf{Collective: "AllReduce"}}},
	}

	emitNetworkTransferMetrics(snd, events)

	snd.AssertNotCalled(t, "Gauge", ncclMetricsNs+networkMaxTransferTimeMetric,
		mock.Anything, mock.Anything, mock.Anything)
}

func TestRankDivergence_NilPerfSkipped(t *testing.T) {
	snd := new(mocksender.MockSender)

	events := []ParsedEvent{
		{Event: NCCLInspectorEvent{ID: "hash1", Rank: 0, CollPerf: nil}},
		{Event: NCCLInspectorEvent{ID: "hash1", Rank: 1, CollPerf: nil}},
	}

	emitRankDivergence(snd, events) // must not panic

	snd.AssertNotCalled(t, "Gauge", ncclMetricsNs+intraNodeDivergenceMetric,
		mock.Anything, mock.Anything, mock.Anything)
}
