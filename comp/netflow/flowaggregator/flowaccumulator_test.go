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
	"github.com/DataDog/datadog-agent/comp/netflow/portrollup"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
	rdnsquerierfxmock "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/stretchr/testify/assert"
)

var initialTime = func() time.Time {
	layout := "2006-01-02 15:04:05"
	str := "2000-01-01 00:00:00"
	t, _ := time.Parse(layout, str)
	return t
}

func setMockTimeNow(newTime time.Time) {
	timeNow = func() time.Time {
		return newTime
	}
}

func Test_flowAccumulator_add(t *testing.T) {
	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())
	synFlag := uint32(2)
	ackFlag := uint32(16)
	synAckFlag := synFlag | ackFlag

	// Given
	flowA1 := &common.Flow{
		FlowType:       common.TypeNetFlow9,
		ExporterAddr:   []byte{127, 0, 0, 1},
		StartTimestamp: 1234568,
		EndTimestamp:   1234569,
		Bytes:          20,
		Packets:        4,
		SrcAddr:        []byte{10, 10, 10, 10},
		DstAddr:        []byte{10, 10, 10, 20},
		IPProtocol:     uint32(6),
		SrcPort:        2000,
		DstPort:        80,
		TCPFlags:       synFlag,
		AdditionalFields: map[string]any{
			"custom_field": "test",
		},
	}
	flowA2 := &common.Flow{
		FlowType:       common.TypeNetFlow9,
		ExporterAddr:   []byte{127, 0, 0, 1},
		StartTimestamp: 1234578,
		EndTimestamp:   1234579,
		Bytes:          10,
		Packets:        2,
		SrcAddr:        []byte{10, 10, 10, 10},
		DstAddr:        []byte{10, 10, 10, 20},
		IPProtocol:     uint32(6),
		SrcPort:        2000,
		DstPort:        80,
		TCPFlags:       ackFlag,
		AdditionalFields: map[string]any{
			"custom_field":   "another_test",
			"custom_field_2": "second_flow_field",
		},
	}
	flowB1 := &common.Flow{
		FlowType:       common.TypeNetFlow9,
		ExporterAddr:   []byte{127, 0, 0, 1},
		StartTimestamp: 1234568,
		EndTimestamp:   1234569,
		Bytes:          10,
		Packets:        2,
		SrcAddr:        []byte{10, 10, 10, 10},
		// different destination addr
		DstAddr:    []byte{10, 10, 10, 30},
		IPProtocol: uint32(6),
		SrcPort:    2000,
		DstPort:    80,
	}

	// When
	acc := newFlowAccumulator(common.DefaultAggregatorFlushInterval, common.DefaultAggregatorFlushInterval, common.DefaultAggregatorPortRollupThreshold, false, false, false, false, false, false, false, false, 0, logger, rdnsQuerier)
	acc.add(flowA1)
	acc.add(flowA2)
	acc.add(flowB1)

	// Then
	assert.Equal(t, 2, len(acc.flows))

	flowCtxA := acc.flows[flowA1.AggregationHash()]
	assert.Equal(t, []byte{10, 10, 10, 10}, flowCtxA.flow.SrcAddr)
	assert.Equal(t, []byte{10, 10, 10, 20}, flowCtxA.flow.DstAddr)
	assert.Equal(t, uint64(30), flowCtxA.flow.Bytes)
	assert.Equal(t, uint64(6), flowCtxA.flow.Packets)
	assert.Equal(t, uint64(1234568), flowCtxA.flow.StartTimestamp)
	assert.Equal(t, uint64(1234579), flowCtxA.flow.EndTimestamp)
	assert.Equal(t, synAckFlag, flowCtxA.flow.TCPFlags)
	assert.Equal(t, map[string]any{"custom_field": "test", "custom_field_2": "second_flow_field"}, flowCtxA.flow.AdditionalFields) // Keeping first value seen for key `custom_field`

	flowCtxB := acc.flows[flowB1.AggregationHash()]
	assert.Equal(t, []byte{10, 10, 10, 10}, flowCtxB.flow.SrcAddr)
	assert.Equal(t, []byte{10, 10, 10, 30}, flowCtxB.flow.DstAddr)
}

func Test_flowAccumulator_portRollUp(t *testing.T) {
	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())
	synFlag := uint32(2)
	ackFlag := uint32(16)
	synAckFlag := synFlag | ackFlag

	// Given
	flowA1 := &common.Flow{
		FlowType:       common.TypeNetFlow9,
		ExporterAddr:   []byte{127, 0, 0, 1},
		StartTimestamp: 1234568,
		EndTimestamp:   1234569,
		Bytes:          20,
		Packets:        4,
		SrcAddr:        []byte{10, 10, 10, 10},
		DstAddr:        []byte{10, 10, 10, 20},
		IPProtocol:     uint32(6),
		SrcPort:        1000,
		DstPort:        80,
		TCPFlags:       synFlag,
	}
	flowA2 := &common.Flow{
		FlowType:       common.TypeNetFlow9,
		ExporterAddr:   []byte{127, 0, 0, 1},
		StartTimestamp: 1234578,
		EndTimestamp:   1234579,
		Bytes:          10,
		Packets:        2,
		SrcAddr:        []byte{10, 10, 10, 10},
		DstAddr:        []byte{10, 10, 10, 20},
		IPProtocol:     uint32(6),
		SrcPort:        1000,
		DstPort:        80,
		TCPFlags:       ackFlag,
	}
	flowB1 := common.Flow{
		FlowType:       common.TypeNetFlow9,
		ExporterAddr:   []byte{127, 0, 0, 1},
		StartTimestamp: 1234568,
		EndTimestamp:   1234569,
		Bytes:          100,
		Packets:        10,
		SrcAddr:        []byte{10, 10, 10, 10},
		DstAddr:        []byte{10, 10, 10, 30},
		IPProtocol:     uint32(6),
		SrcPort:        80,
		DstPort:        2001,
	}

	// When
	acc := newFlowAccumulator(common.DefaultAggregatorFlushInterval, common.DefaultAggregatorFlushInterval, 3, false, false, false, false, false, false, false, false, 0, logger, rdnsQuerier)
	acc.add(flowA1)
	acc.add(flowA2)

	flowB1a := flowB1
	acc.add(&flowB1a)
	flowB1b := flowB1 // send flowB1 twice to test that it's not counted twice by portRollup tracker
	acc.add(&flowB1b)
	assert.Equal(t, uint16(1), acc.portRollup.GetSourceToDestPortCount([]byte{10, 10, 10, 10}, []byte{10, 10, 10, 30}, 80))
	assert.Equal(t, uint16(1), acc.portRollup.GetDestToSourcePortCount([]byte{10, 10, 10, 10}, []byte{10, 10, 10, 30}, 2001))

	flowB2 := flowB1
	flowB2.DstPort = 2002
	acc.add(&flowB2)
	flowB3 := flowB1
	flowB3.DstPort = 2003
	acc.add(&flowB3)
	flowB4 := flowB1
	flowB4.DstPort = 2004
	acc.add(&flowB4)
	flowB5 := flowB1
	flowB5.DstPort = 2005
	acc.add(&flowB5)
	flowB6 := flowB1
	flowB6.DstPort = 2006
	acc.add(&flowB6)

	flowBwithPortRollup := flowB1
	flowBwithPortRollup.DstPort = portrollup.EphemeralPort

	assert.Equal(t, uint16(3), acc.portRollup.GetSourceToDestPortCount([]byte{10, 10, 10, 10}, []byte{10, 10, 10, 30}, 80))

	// Then
	assert.Equal(t, 4, len(acc.flows))

	flowCtxA := acc.flows[flowA1.AggregationHash()]
	assert.Equal(t, []byte{10, 10, 10, 10}, flowCtxA.flow.SrcAddr)
	assert.Equal(t, []byte{10, 10, 10, 20}, flowCtxA.flow.DstAddr)
	assert.Equal(t, uint64(30), flowCtxA.flow.Bytes)
	assert.Equal(t, uint64(6), flowCtxA.flow.Packets)
	assert.Equal(t, uint64(1234568), flowCtxA.flow.StartTimestamp)
	assert.Equal(t, uint64(1234579), flowCtxA.flow.EndTimestamp)
	assert.Equal(t, synAckFlag, flowCtxA.flow.TCPFlags)

	assert.Equal(t, uint64(20), acc.flows[flowB1.AggregationHash()].flow.Packets)
	assert.Equal(t, int32(2001), acc.flows[flowB1.AggregationHash()].flow.DstPort)
	assert.Equal(t, uint64(10), acc.flows[flowB2.AggregationHash()].flow.Packets)
	assert.Equal(t, int32(2002), acc.flows[flowB2.AggregationHash()].flow.DstPort)
	// flowB3, B4, B5, B6 are aggregated into one flow with DstPort = 0
	assert.Equal(t, uint64(40), acc.flows[flowBwithPortRollup.AggregationHash()].flow.Packets)
	assert.Equal(t, int32(-1), acc.flows[flowBwithPortRollup.AggregationHash()].flow.DstPort)
}

func Test_flowAccumulator_flush(t *testing.T) {
	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())
	timeNow = initialTime
	zeroTime := time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
	flushInterval := 60 * time.Second
	flowContextTTL := 60 * time.Second

	// Given
	flow := &common.Flow{
		FlowType:       common.TypeNetFlow9,
		ExporterAddr:   []byte{127, 0, 0, 1},
		StartTimestamp: 1234568,
		EndTimestamp:   1234569,
		Bytes:          20,
		Packets:        4,
		SrcAddr:        []byte{10, 10, 10, 10},
		DstAddr:        []byte{10, 10, 10, 20},
		IPProtocol:     uint32(6),
		SrcPort:        2000,
		DstPort:        80,
	}

	// When
	acc := newFlowAccumulator(flushInterval, flowContextTTL, common.DefaultAggregatorPortRollupThreshold, false, false, false, false, false, false, false, false, 0, logger, rdnsQuerier)
	acc.add(flow)

	// Then
	assert.Equal(t, 1, len(acc.flows))

	flowCtx := acc.flows[flow.AggregationHash()]

	assert.Equal(t, timeNow().Add(flushInterval), flowCtx.nextFlush)
	assert.Equal(t, zeroTime, flowCtx.lastSuccessfulFlush)

	// test first flush
	// set flush time
	setMockTimeNow(initialTime().Add(flushInterval)) // t=60, nextFlush=60, lastSuccessfulFlush=0
	acc.flush()                                      // flushed, nextFlush=120, lastSuccessfulFlush=60
	flowCtx = acc.flows[flow.AggregationHash()]
	expectedNextFlush := timeNow().Add(flushInterval)
	assert.Equal(t, expectedNextFlush, flowCtx.nextFlush)

	expectedLastSuccessfulFlush := timeNow()
	assert.Equal(t, expectedLastSuccessfulFlush, flowCtx.lastSuccessfulFlush)

	// test skip flush if nextFlush is not reached yet
	setMockTimeNow(initialTime().Add(flushInterval + (5 * time.Second))) // t=65, nextFlush=120, lastSuccessfulFlush=60
	acc.flush()                                                          // NOT flushed
	flowCtx = acc.flows[flow.AggregationHash()]
	assert.Equal(t, expectedNextFlush, flowCtx.nextFlush)
	assert.Equal(t, expectedLastSuccessfulFlush, flowCtx.lastSuccessfulFlush)

	// test flush with no new flow after nextFlush is reached
	setMockTimeNow(initialTime().Add(2 * flushInterval)) // t=120 nextFlush=120, lastSuccessfulFlush=60
	acc.flush()                                          // flushed, nextFlush=180, lastSuccessfulFlush=120
	expectedNextFlush = expectedNextFlush.Add(flushInterval)
	flowCtx = acc.flows[flow.AggregationHash()]
	assert.Equal(t, expectedNextFlush, flowCtx.nextFlush)
	assert.Equal(t, expectedLastSuccessfulFlush, flowCtx.lastSuccessfulFlush)

	// test flush with new flow after nextFlush is reached
	setMockTimeNow(initialTime().Add(3 * flushInterval)) // t=180 nextFlush=180, lastSuccessfulFlush=120
	expectedLastSuccessfulFlush = timeNow()
	expectedNextFlush = expectedLastSuccessfulFlush.Add(flushInterval)
	acc.add(flow)
	acc.flush() // flushed, nextFlush=240, lastSuccessfulFlush=180
	flowCtx = acc.flows[flow.AggregationHash()]
	assert.Equal(t, expectedNextFlush, flowCtx.nextFlush)
	assert.Equal(t, expectedLastSuccessfulFlush, flowCtx.lastSuccessfulFlush)

	// test flush with TTL reached (now+ttl is equal last successful flush) to clean up entry
	setMockTimeNow(initialTime().Add(3*flushInterval + flowContextTTL + 1*time.Second)) // ts=241 nextFlush=240, lastSuccessfulFlush=180
	acc.flush()                                                                         // NOT flushed, flowContext cleaned up
	_, ok := acc.flows[flow.AggregationHash()]
	assert.False(t, ok)

	// test flush with TTL reached (now+ttl is after last successful flush) to clean up entry
	setMockTimeNow(initialTime())
	acc.add(flow)
	setMockTimeNow(initialTime().Add(flushInterval))                                  // ts=60 nextFlush=60, lastSuccessfulFlush=0
	acc.flush()                                                                       // flushed, nextFlush=120, lastSuccessfulFlush=60
	setMockTimeNow(initialTime().Add(flushInterval + flowContextTTL + 1*time.Second)) // ts=121 nextFlush=120, lastSuccessfulFlush=60
	acc.flush()                                                                       // NOT flushed, flowContext cleaned up
	_, ok = acc.flows[flow.AggregationHash()]
	assert.False(t, ok)
}

func Test_flowAccumulator_detectHashCollision(t *testing.T) {
	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())
	synFlag := uint32(2)
	timeNow = initialTime
	flushInterval := 60 * time.Second
	flowContextTTL := 60 * time.Second

	// Given
	flowA1 := &common.Flow{
		FlowType:       common.TypeNetFlow9,
		ExporterAddr:   []byte{127, 0, 0, 1},
		StartTimestamp: 1234568,
		EndTimestamp:   1234569,
		Bytes:          20,
		Packets:        4,
		SrcAddr:        []byte{10, 10, 10, 10},
		DstAddr:        []byte{10, 10, 10, 20},
		IPProtocol:     uint32(6),
		SrcPort:        1000,
		DstPort:        80,
		TCPFlags:       synFlag,
	}
	flowA2 := &common.Flow{
		FlowType:       common.TypeNetFlow9,
		ExporterAddr:   []byte{127, 0, 0, 1},
		StartTimestamp: 1234568,
		EndTimestamp:   1234569,
		Bytes:          20,
		Packets:        4,
		SrcAddr:        []byte{10, 10, 10, 10},
		DstAddr:        []byte{10, 10, 10, 20},
		IPProtocol:     uint32(6),
		SrcPort:        1000,
		DstPort:        80,
		TCPFlags:       synFlag,
	}
	flowB1 := &common.Flow{
		FlowType:       common.TypeNetFlow9,
		ExporterAddr:   []byte{127, 0, 0, 1},
		StartTimestamp: 1234568,
		EndTimestamp:   1234569,
		Bytes:          100,
		Packets:        10,
		SrcAddr:        []byte{10, 10, 10, 10},
		DstAddr:        []byte{10, 10, 10, 30},
		IPProtocol:     uint32(6),
		SrcPort:        80,
		DstPort:        2001,
	}

	// When
	acc := newFlowAccumulator(flushInterval, flowContextTTL, common.DefaultAggregatorPortRollupThreshold, false, false, false, false, false, false, false, false, 0, logger, rdnsQuerier)

	// Then
	assert.Equal(t, uint64(0), acc.hashCollisionFlowCount.Load())

	// test valid hash collision (same flow object) does not increment flow count
	aggHash1 := flowA1.AggregationHash()
	acc.detectHashCollision(aggHash1, *flowA1, *flowA1)
	assert.Equal(t, uint64(0), acc.hashCollisionFlowCount.Load())

	// test valid hash collision (same data, new flow object) does not increment flow count
	// Note: not a realistic use case as hashes will be different, but testing for completeness
	aggHash2 := flowA2.AggregationHash()
	acc.detectHashCollision(aggHash2, *flowA1, *flowA2)
	assert.Equal(t, uint64(0), acc.hashCollisionFlowCount.Load())

	// test invalid hash collision (different flow context, same hash) increments flow count
	aggHash3 := flowB1.AggregationHash()
	acc.detectHashCollision(aggHash3, *flowA1, *flowB1)
	assert.Equal(t, uint64(1), acc.hashCollisionFlowCount.Load())
}

func TestFlowAccumulator_AggregationHashConfigOption(t *testing.T) {
	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())

	flow := &common.Flow{
		Namespace:      "test-ns",
		ExporterAddr:   []byte{192, 168, 1, 1},
		SrcAddr:        []byte{10, 0, 0, 1},
		DstAddr:        []byte{10, 0, 0, 2},
		SrcPort:        1234,
		DstPort:        80,
		IPProtocol:     6,
		Tos:            0,
		InputInterface: 1,
	}

	// Test with sync pool disabled (original implementation)
	accOriginal := newFlowAccumulator(common.DefaultAggregatorFlushInterval, common.DefaultAggregatorFlushInterval, common.DefaultAggregatorPortRollupThreshold, false, false, false, false, false, false, false, false, 0, logger, rdnsQuerier)
	hashOriginal := accOriginal.getAggregationHash(flow)

	// Test with sync pool enabled (optimized implementation)
	accSyncPool := newFlowAccumulator(common.DefaultAggregatorFlushInterval, common.DefaultAggregatorFlushInterval, common.DefaultAggregatorPortRollupThreshold, false, false, false, true, false, false, false, false, 0, logger, rdnsQuerier)
	hashSyncPool := accSyncPool.getAggregationHash(flow)

	// Both should produce the same hash
	assert.Equal(t, hashOriginal, hashSyncPool, "Both hash implementations should produce the same result")

	// Verify that the configuration is respected
	assert.False(t, accOriginal.aggregationHashUseSyncPool, "Original accumulator should have sync pool disabled")
	assert.True(t, accSyncPool.aggregationHashUseSyncPool, "SyncPool accumulator should have sync pool enabled")
}

func TestFlowAccumulator_InlineHashCollisionDetectionConfigOption(t *testing.T) {
	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())

	// Test with inline hash collision detection disabled (uses goroutine)
	accAsync := newFlowAccumulator(common.DefaultAggregatorFlushInterval, common.DefaultAggregatorFlushInterval, common.DefaultAggregatorPortRollupThreshold, false, false, false, false, false, false, false, false, 0, logger, rdnsQuerier)

	// Test with inline hash collision detection enabled (runs inline)
	accInline := newFlowAccumulator(common.DefaultAggregatorFlushInterval, common.DefaultAggregatorFlushInterval, common.DefaultAggregatorPortRollupThreshold, false, false, true, false, false, false, false, false, 0, logger, rdnsQuerier)

	// Verify that the configuration is respected
	assert.False(t, accAsync.inlineHashCollisionDetection, "Async accumulator should have inline hash collision detection disabled")
	assert.True(t, accInline.inlineHashCollisionDetection, "Inline accumulator should have inline hash collision detection enabled")
}
