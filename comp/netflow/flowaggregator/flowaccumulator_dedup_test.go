// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"testing"
	"time"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
	rdnsquerierfxmock "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestDedupAccumulator creates a dedupFlowAccumulator with sensible defaults for testing.
func newTestDedupAccumulator(t *testing.T, flushInterval, flowContextTTL time.Duration) *dedupFlowAccumulator {
	t.Helper()
	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())
	flushConfig := common.FlushConfig{
		FlowCollectionDuration: flushInterval,
	}
	return newDedupFlowAccumulator(flushConfig, ImmediateFlowScheduler{flushConfig: flushConfig}, flowContextTTL, common.DefaultAggregatorPortRollupThreshold, true, logger, rdnsQuerier)
}

// makeFlow builds a flow with the given 5-tuple identity and exporter. Port rollup is
// disabled in dedup tests so ports are used as-is.
func makeFlow(srcIP, dstIP []byte, srcPort, dstPort int32, exporterIP []byte, namespace string) *common.Flow {
	return &common.Flow{
		FlowType:     common.TypeNetFlow9,
		ExporterAddr: exporterIP,
		Namespace:    namespace,
		SrcAddr:      srcIP,
		DstAddr:      dstIP,
		SrcPort:      srcPort,
		DstPort:      dstPort,
		IPProtocol:   6,
		Bytes:        100,
		Packets:      10,
	}
}

func Test_dedupAccumulator_groupsByFiveTuple(t *testing.T) {
	timeNow = MockTimeNow
	acc := newTestDedupAccumulator(t, 60*time.Second, 120*time.Second)

	// Two reporters for the same 5-tuple (different exporters).
	flowA := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 1}, "ns1")
	flowB := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 2}, "ns1")
	// A different 5-tuple entirely.
	flowC := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 3}, 1234, 443, []byte{192, 168, 1, 1}, "ns1")

	acc.Add(flowA)
	acc.Add(flowB)
	acc.Add(flowC)

	// flowA and flowB share a DeduplicationHash, flowC does not.
	assert.Equal(t, flowA.DeduplicationHash(), flowB.DeduplicationHash())
	assert.NotEqual(t, flowA.DeduplicationHash(), flowC.DeduplicationHash())

	// Two groups should exist.
	assert.Len(t, acc.fiveTupleGroups, 2)

	// The group for the shared 5-tuple should contain both reporter hashes.
	groupHashes := acc.fiveTupleGroups[flowA.DeduplicationHash()]
	assert.Len(t, groupHashes, 2)
	assert.Contains(t, groupHashes, flowA.PerReporterHash())
	assert.Contains(t, groupHashes, flowB.PerReporterHash())

	// The group for flowC should contain exactly one hash.
	assert.Len(t, acc.fiveTupleGroups[flowC.DeduplicationHash()], 1)
}

func Test_dedupAccumulator_flushesAllReportersTogether(t *testing.T) {
	timeNow = MockTimeNow
	flushInterval := 60 * time.Second
	acc := newTestDedupAccumulator(t, flushInterval, 120*time.Second)

	flowA := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 1}, "ns1")
	flowB := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 2}, "ns1")
	flowB.Bytes = 200

	acc.Add(flowA)
	acc.Add(flowB)

	// Flush after the interval elapses.
	flushTime := MockTimeNow().Add(flushInterval + time.Second)
	groups := acc.Flush(common.FlushContext{FlushTime: flushTime})

	require.Len(t, groups, 1, "one group for the shared 5-tuple")

	group := groups[0]
	assert.Len(t, group.Reporters, 2, "both reporters should be flushed together")
	assert.Empty(t, group.GhostReporters, "no ghosts on first flush")

	// Verify both reporters' data is present.
	var totalBytes uint64
	for _, r := range group.Reporters {
		totalBytes += r.Bytes
	}
	assert.Equal(t, uint64(300), totalBytes)
}

func Test_dedupAccumulator_ghostReportersFromPreviousCycle(t *testing.T) {
	timeNow = MockTimeNow
	flushInterval := 60 * time.Second
	acc := newTestDedupAccumulator(t, flushInterval, 120*time.Second)

	flowA := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 1}, "ns1")
	flowB := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 2}, "ns1")

	// Cycle 1: both reporters active.
	acc.Add(flowA)
	acc.Add(flowB)
	flush1 := MockTimeNow().Add(flushInterval + time.Second)
	groups1 := acc.Flush(common.FlushContext{FlushTime: flush1})
	require.Len(t, groups1, 1)
	assert.Len(t, groups1[0].Reporters, 2)
	assert.Empty(t, groups1[0].GhostReporters)

	// Cycle 2: only reporter A sends new data.
	flowA2 := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 1}, "ns1")
	flowA2.Bytes = 50
	acc.Add(flowA2)
	flush2 := flush1.Add(flushInterval + time.Second)
	groups2 := acc.Flush(common.FlushContext{FlushTime: flush2})

	require.Len(t, groups2, 1)
	group := groups2[0]
	assert.Len(t, group.Reporters, 1, "only A is active this cycle")

	// Ghosts from cycle 1 should be present with zeroed bytes/packets.
	require.Len(t, group.GhostReporters, 2, "both cycle-1 reporters become ghosts")
	for _, ghost := range group.GhostReporters {
		assert.Equal(t, uint64(0), ghost.Bytes, "ghost bytes should be zeroed")
		assert.Equal(t, uint64(0), ghost.Packets, "ghost packets should be zeroed")
	}
}

func Test_dedupAccumulator_ghostsExpireAfterOneCycle(t *testing.T) {
	timeNow = MockTimeNow
	flushInterval := 60 * time.Second
	acc := newTestDedupAccumulator(t, flushInterval, 120*time.Second)

	flowA := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 1}, "ns1")
	flowB := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 2}, "ns1")

	// Cycle 1: both reporters.
	acc.Add(flowA)
	acc.Add(flowB)
	flush1 := MockTimeNow().Add(flushInterval + time.Second)
	acc.Flush(common.FlushContext{FlushTime: flush1})

	// Cycle 2: only A. Ghosts = {A, B} from cycle 1.
	acc.Add(makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 1}, "ns1"))
	flush2 := flush1.Add(flushInterval + time.Second)
	groups2 := acc.Flush(common.FlushContext{FlushTime: flush2})
	require.Len(t, groups2, 1)
	assert.Len(t, groups2[0].GhostReporters, 2)

	// Cycle 3: only A again. Ghosts should now be {A} from cycle 2 only — B is gone.
	acc.Add(makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 1}, "ns1"))
	flush3 := flush2.Add(flushInterval + time.Second)
	groups3 := acc.Flush(common.FlushContext{FlushTime: flush3})
	require.Len(t, groups3, 1)
	assert.Len(t, groups3[0].GhostReporters, 1, "only the previous cycle's active reporter appears as ghost")
}

func Test_dedupAccumulator_lateJoinerFlushedWithGroup(t *testing.T) {
	timeNow = MockTimeNow
	flushInterval := 60 * time.Second
	acc := newTestDedupAccumulator(t, flushInterval, 120*time.Second)

	// Reporter A arrives first.
	flowA := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 1}, "ns1")
	acc.Add(flowA)

	// Reporter B joins 10s later — still within the same flush window.
	flowB := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 2}, "ns1")
	setMockTimeNow(MockTimeNow().Add(10 * time.Second))
	acc.Add(flowB)

	// B should have inherited A's flush time, so both flush together.
	flushTime := MockTimeNow().Add(flushInterval + time.Second)
	groups := acc.Flush(common.FlushContext{FlushTime: flushTime})

	require.Len(t, groups, 1)
	assert.Len(t, groups[0].Reporters, 2, "late joiner B should flush with A")
}

func Test_dedupAccumulator_lateJoinerAfterFlush(t *testing.T) {
	timeNow = MockTimeNow
	flushInterval := 60 * time.Second
	acc := newTestDedupAccumulator(t, flushInterval, 120*time.Second)

	// Reporter A arrives and flushes.
	flowA := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 1}, "ns1")
	acc.Add(flowA)
	flush1 := MockTimeNow().Add(flushInterval + time.Second)
	groups1 := acc.Flush(common.FlushContext{FlushTime: flush1})
	require.Len(t, groups1, 1)
	assert.Len(t, groups1[0].Reporters, 1)

	// Reporter B joins after the flush — A's flowContext exists but has flow==nil.
	// B should still inherit A's next flush time from the dead context.
	setMockTimeNow(flush1.Add(10 * time.Second))
	flowB := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 2}, "ns1")
	acc.Add(flowB)

	// Also add new data for A.
	flowA2 := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 1}, "ns1")
	flowA2.Bytes = 50
	acc.Add(flowA2)

	// Next flush: both A and B should be present.
	flush2 := flush1.Add(flushInterval + time.Second)
	groups2 := acc.Flush(common.FlushContext{FlushTime: flush2})
	require.Len(t, groups2, 1)
	assert.Len(t, groups2[0].Reporters, 2, "B that joined after flush should be included")
}

func Test_dedupAccumulator_emptyFlushProducesNothing(t *testing.T) {
	timeNow = MockTimeNow
	acc := newTestDedupAccumulator(t, 60*time.Second, 120*time.Second)

	// Flushing with no flows added should produce nothing.
	groups := acc.Flush(common.FlushContext{FlushTime: MockTimeNow().Add(time.Hour)})
	assert.Empty(t, groups)
	assert.Empty(t, acc.fiveTupleGroups)
}

func Test_dedupAccumulator_deadContextCleanedOnNextGroupFlush(t *testing.T) {
	timeNow = MockTimeNow
	flushInterval := 60 * time.Second
	flowContextTTL := 120 * time.Second
	acc := newTestDedupAccumulator(t, flushInterval, flowContextTTL)

	// Two reporters in one group.
	flowA := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 1}, "ns1")
	flowB := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 2}, "ns1")

	acc.Add(flowA)
	acc.Add(flowB)

	// Flush cycle 1: both active.
	flush1 := MockTimeNow().Add(flushInterval + time.Second)
	groups := acc.Flush(common.FlushContext{FlushTime: flush1})
	require.Len(t, groups, 1)
	assert.Len(t, groups[0].Reporters, 2)

	// Only reporter A sends new data. Reporter B's dead context will expire.
	flush2 := flush1.Add(flowContextTTL + flushInterval + time.Second)
	flowA2 := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 1}, "ns1")
	acc.Add(flowA2)

	groups2 := acc.Flush(common.FlushContext{FlushTime: flush2})
	require.Len(t, groups2, 1)
	assert.Len(t, groups2[0].Reporters, 1, "only A is active")

	// B's dead context should now be cleaned up (TTL expired), but the group
	// persists because A is still alive.
	dedupHash := flowA.DeduplicationHash()
	assert.Len(t, acc.fiveTupleGroups[dedupHash], 1, "only A's hash remains in the group")
	_, bExists := acc.flows[flowB.PerReporterHash()]
	assert.False(t, bExists, "B's dead context should be deleted after TTL")
}

func Test_dedupAccumulator_multipleGroupsFlushIndependently(t *testing.T) {
	timeNow = MockTimeNow
	flushInterval := 60 * time.Second
	acc := newTestDedupAccumulator(t, flushInterval, 120*time.Second)

	// Group 1: 10.0.0.1 -> 10.0.0.2:80
	flow1A := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 1}, "ns1")
	// Group 2: 10.0.0.1 -> 10.0.0.3:443
	flow2A := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 3}, 1234, 443, []byte{192, 168, 1, 1}, "ns1")

	acc.Add(flow1A)
	acc.Add(flow2A)

	flushTime := MockTimeNow().Add(flushInterval + time.Second)
	groups := acc.Flush(common.FlushContext{FlushTime: flushTime})

	assert.Len(t, groups, 2, "each 5-tuple group should flush independently")
	for _, g := range groups {
		assert.Len(t, g.Reporters, 1)
	}
}

func Test_dedupAccumulator_accumulationWithinReporter(t *testing.T) {
	timeNow = MockTimeNow
	flushInterval := 60 * time.Second
	acc := newTestDedupAccumulator(t, flushInterval, 120*time.Second)

	// Same exporter sends two records for the same flow — should accumulate (same PerReporterHash).
	flow1 := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 1}, "ns1")
	flow1.Bytes = 100
	flow1.Packets = 10

	flow2 := makeFlow([]byte{10, 0, 0, 1}, []byte{10, 0, 0, 2}, 1234, 80, []byte{192, 168, 1, 1}, "ns1")
	flow2.Bytes = 200
	flow2.Packets = 20

	acc.Add(flow1)
	acc.Add(flow2)

	flushTime := MockTimeNow().Add(flushInterval + time.Second)
	groups := acc.Flush(common.FlushContext{FlushTime: flushTime})

	require.Len(t, groups, 1)
	require.Len(t, groups[0].Reporters, 1, "same reporter should accumulate, not create two entries")
	assert.Equal(t, uint64(300), groups[0].Reporters[0].Bytes)
	assert.Equal(t, uint64(30), groups[0].Reporters[0].Packets)
}

func Test_DeduplicationHash_sameFlowDifferentExporters(t *testing.T) {
	flowA := &common.Flow{
		SrcAddr:    []byte{10, 0, 0, 1},
		DstAddr:    []byte{10, 0, 0, 2},
		SrcPort:    1234,
		DstPort:    80,
		IPProtocol: 6,
		// Different exporter / namespace / interface — should NOT affect dedup hash.
		ExporterAddr:   []byte{192, 168, 1, 1},
		Namespace:      "ns1",
		InputInterface: 1,
	}
	flowB := &common.Flow{
		SrcAddr:        []byte{10, 0, 0, 1},
		DstAddr:        []byte{10, 0, 0, 2},
		SrcPort:        1234,
		DstPort:        80,
		IPProtocol:     6,
		ExporterAddr:   []byte{192, 168, 1, 99},
		Namespace:      "ns2",
		InputInterface: 42,
	}

	assert.Equal(t, flowA.DeduplicationHash(), flowB.DeduplicationHash(),
		"same 5-tuple should produce same DeduplicationHash regardless of exporter")
	assert.NotEqual(t, flowA.PerReporterHash(), flowB.PerReporterHash(),
		"different exporters should produce different PerReporterHash")
}

func Test_DeduplicationHash_differentFiveTuples(t *testing.T) {
	base := &common.Flow{
		SrcAddr:    []byte{10, 0, 0, 1},
		DstAddr:    []byte{10, 0, 0, 2},
		SrcPort:    1234,
		DstPort:    80,
		IPProtocol: 6,
	}
	baseHash := base.DeduplicationHash()

	// Changing any 5-tuple field should change the hash.
	diffSrc := *base
	diffSrc.SrcAddr = []byte{10, 0, 0, 99}
	assert.NotEqual(t, baseHash, diffSrc.DeduplicationHash())

	diffDst := *base
	diffDst.DstAddr = []byte{10, 0, 0, 99}
	assert.NotEqual(t, baseHash, diffDst.DeduplicationHash())

	diffSrcPort := *base
	diffSrcPort.SrcPort = 9999
	assert.NotEqual(t, baseHash, diffSrcPort.DeduplicationHash())

	diffDstPort := *base
	diffDstPort.DstPort = 443
	assert.NotEqual(t, baseHash, diffDstPort.DeduplicationHash())

	diffProto := *base
	diffProto.IPProtocol = 17
	assert.NotEqual(t, baseHash, diffProto.DeduplicationHash())
}

func Test_zeroedSnapshots(t *testing.T) {
	flows := []*common.Flow{
		{Bytes: 100, Packets: 10, SrcAddr: []byte{10, 0, 0, 1}, ExporterAddr: []byte{192, 168, 1, 1}},
		{Bytes: 200, Packets: 20, SrcAddr: []byte{10, 0, 0, 2}, ExporterAddr: []byte{192, 168, 1, 2}},
	}

	snapshots := zeroedSnapshots(flows)

	require.Len(t, snapshots, 2)
	for i, snap := range snapshots {
		assert.Equal(t, uint64(0), snap.Bytes, "snapshot %d bytes should be zero", i)
		assert.Equal(t, uint64(0), snap.Packets, "snapshot %d packets should be zero", i)
		// Non-metric fields should be preserved.
		assert.Equal(t, flows[i].SrcAddr, snap.SrcAddr)
		assert.Equal(t, flows[i].ExporterAddr, snap.ExporterAddr)
	}

	// Verify snapshots are independent copies — mutating original shouldn't affect snapshot.
	flows[0].Bytes = 999
	assert.Equal(t, uint64(0), snapshots[0].Bytes)
}
