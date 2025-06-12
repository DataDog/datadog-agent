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

func setMockTimeNow(newTime time.Time) {
	timeNow = func() time.Time {
		return newTime
	}
}

var initialTime = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

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
	acc := newFlowAccumulator(common.DefaultAggregatorFlushInterval, common.DefaultAggregatorFlushInterval, common.DefaultAggregatorPortRollupThreshold, false, logger, rdnsQuerier)
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
	acc := newFlowAccumulator(common.DefaultAggregatorFlushInterval, common.DefaultAggregatorFlushInterval, 3, false, logger, rdnsQuerier)
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
	setMockTimeNow(initialTime)
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
	acc := newFlowAccumulator(flushInterval, flowContextTTL, common.DefaultAggregatorPortRollupThreshold, false, logger, rdnsQuerier)
	acc.add(flow) // t=0, nextFlush=60, lastSuccessfulFlush=0

	// Then
	assert.Equal(t, 1, len(acc.flows))

	flowCtx := acc.flows[flow.AggregationHash()]
	expectedNextFlush := timeNow().Add(flushInterval)
	expectedLastSuccessfulFlush := zeroTime
	assert.Equal(t, expectedNextFlush, flowCtx.nextFlush)
	assert.Equal(t, expectedLastSuccessfulFlush, flowCtx.lastSuccessfulFlush)

	// test skip flush if nextFlush is not reached yet
	setMockTimeNow(initialTime.Add(flushInterval - 10*time.Second)) // t=50, nextFlush=60, lastSuccessfulFlush=0
	acc.flush()                                                     // NOT flushed
	flowCtx = acc.flows[flow.AggregationHash()]
	assert.Equal(t, expectedNextFlush, flowCtx.nextFlush)
	assert.Equal(t, expectedLastSuccessfulFlush, flowCtx.lastSuccessfulFlush)

	// test first flush
	setMockTimeNow(initialTime.Add(flushInterval + 1*time.Second)) // t=61, nextFlush=60, lastSuccessfulFlush=0
	acc.flush()                                                    // flushed, nextFlush=120, lastSuccessfulFlush=61
	flowCtx = acc.flows[flow.AggregationHash()]
	expectedNextFlush = expectedNextFlush.Add(flushInterval)
	expectedLastSuccessfulFlush = timeNow()
	assert.Equal(t, expectedNextFlush, flowCtx.nextFlush)
	assert.Equal(t, expectedLastSuccessfulFlush, flowCtx.lastSuccessfulFlush)
	// the flow was flushed so flowCtx.Flow should be nil
	assert.Nil(t, flowCtx.flow)

	// test flush with no new flow after nextFlush is reached
	setMockTimeNow(initialTime.Add(2*flushInterval + 1*time.Second)) // t=121 nextFlush=120, lastSuccessfulFlush=61
	acc.flush()                                                      // flushed, nextFlush=180, lastSuccessfulFlush=121
	expectedNextFlush = expectedNextFlush.Add(flushInterval)
	flowCtx = acc.flows[flow.AggregationHash()]
	assert.Equal(t, expectedNextFlush, flowCtx.nextFlush)
	assert.Equal(t, expectedLastSuccessfulFlush, flowCtx.lastSuccessfulFlush)

	acc.add(flow) // t=121, nextFlush=180, lastSuccessfulFlush=121

	// test flush with new flow after nextFlush is reached
	setMockTimeNow(initialTime.Add(3*flushInterval + 1*time.Second)) // t=181 nextFlush=180, lastSuccessfulFlush=121
	expectedNextFlush = expectedNextFlush.Add(flushInterval)
	expectedLastSuccessfulFlush = timeNow()
	acc.flush() // flushed, nextFlush=240, lastSuccessfulFlush=181
	flowCtx = acc.flows[flow.AggregationHash()]
	assert.Equal(t, expectedNextFlush, flowCtx.nextFlush)
	assert.Equal(t, expectedLastSuccessfulFlush, flowCtx.lastSuccessfulFlush)
	// the flow was flushed so flowCtx.Flow should be nil
	assert.Nil(t, flowCtx.flow)

	// test flush with TTL reached (now+ttl is equal last successful flush) to clean up entry
	setMockTimeNow(initialTime.Add(3*flushInterval + flowContextTTL + 2*time.Second)) // t=242 nextFlush=240, lastSuccessfulFlush=181
	acc.flush()                                                                       // NOT flushed, flowContext cleaned up
	_, ok := acc.flows[flow.AggregationHash()]
	assert.False(t, ok)

	// test flush with TTL reached (now+ttl is after last successful flush) to clean up entry
	setMockTimeNow(initialTime)
	acc.add(flow)
	setMockTimeNow(initialTime.Add(flushInterval))                                  // t=60 nextFlush=60, lastSuccessfulFlush=0
	acc.flush()                                                                     // flushed, nextFlush=120, lastSuccessfulFlush=60
	setMockTimeNow(initialTime.Add(flushInterval + flowContextTTL + 1*time.Second)) // t=121 nextFlush=120, lastSuccessfulFlush=60
	acc.flush()                                                                     // NOT flushed, flowContext cleaned up
	_, ok = acc.flows[flow.AggregationHash()]
	assert.False(t, ok)
}

func Test_flowAccumulator_detectHashCollision(t *testing.T) {
	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())
	synFlag := uint32(2)
	setMockTimeNow(initialTime)
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
	acc := newFlowAccumulator(flushInterval, flowContextTTL, common.DefaultAggregatorPortRollupThreshold, false, logger, rdnsQuerier)

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
