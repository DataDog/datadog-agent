// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/portrollup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/stretchr/testify/assert"
)

// MockTimeNow mocks time.Now
var MockTimeNow = func() time.Time {
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
	logger := fxutil.Test[log.Component](t, log.MockModule)
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
	acc := newFlowAccumulator(common.DefaultAggregatorFlushInterval, common.DefaultAggregatorFlushInterval, common.DefaultAggregatorPortRollupThreshold, false, logger)
	acc.add(flowA1)
	acc.add(flowA2)
	acc.add(flowB1)

	// Then
	assert.Equal(t, 2, len(acc.flows))

	wrappedFlowA := acc.flows[flowA1.AggregationHash()]
	assert.Equal(t, []byte{10, 10, 10, 10}, wrappedFlowA.flow.SrcAddr)
	assert.Equal(t, []byte{10, 10, 10, 20}, wrappedFlowA.flow.DstAddr)
	assert.Equal(t, uint64(30), wrappedFlowA.flow.Bytes)
	assert.Equal(t, uint64(6), wrappedFlowA.flow.Packets)
	assert.Equal(t, uint64(1234568), wrappedFlowA.flow.StartTimestamp)
	assert.Equal(t, uint64(1234579), wrappedFlowA.flow.EndTimestamp)
	assert.Equal(t, synAckFlag, wrappedFlowA.flow.TCPFlags)

	wrappedFlowB := acc.flows[flowB1.AggregationHash()]
	assert.Equal(t, []byte{10, 10, 10, 10}, wrappedFlowB.flow.SrcAddr)
	assert.Equal(t, []byte{10, 10, 10, 30}, wrappedFlowB.flow.DstAddr)
}

func Test_flowAccumulator_portRollUp(t *testing.T) {
	logger := fxutil.Test[log.Component](t, log.MockModule)
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
	acc := newFlowAccumulator(common.DefaultAggregatorFlushInterval, common.DefaultAggregatorFlushInterval, 3, false, logger)
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

	wrappedFlowA := acc.flows[flowA1.AggregationHash()]
	assert.Equal(t, []byte{10, 10, 10, 10}, wrappedFlowA.flow.SrcAddr)
	assert.Equal(t, []byte{10, 10, 10, 20}, wrappedFlowA.flow.DstAddr)
	assert.Equal(t, uint64(30), wrappedFlowA.flow.Bytes)
	assert.Equal(t, uint64(6), wrappedFlowA.flow.Packets)
	assert.Equal(t, uint64(1234568), wrappedFlowA.flow.StartTimestamp)
	assert.Equal(t, uint64(1234579), wrappedFlowA.flow.EndTimestamp)
	assert.Equal(t, synAckFlag, wrappedFlowA.flow.TCPFlags)

	assert.Equal(t, uint64(20), acc.flows[flowB1.AggregationHash()].flow.Packets)
	assert.Equal(t, int32(2001), acc.flows[flowB1.AggregationHash()].flow.DstPort)
	assert.Equal(t, uint64(10), acc.flows[flowB2.AggregationHash()].flow.Packets)
	assert.Equal(t, int32(2002), acc.flows[flowB2.AggregationHash()].flow.DstPort)
	// flowB3, B4, B5, B6 are aggregated into one flow with DstPort = 0
	assert.Equal(t, uint64(40), acc.flows[flowBwithPortRollup.AggregationHash()].flow.Packets)
	assert.Equal(t, int32(-1), acc.flows[flowBwithPortRollup.AggregationHash()].flow.DstPort)
}

func Test_flowAccumulator_flush(t *testing.T) {
	logger := fxutil.Test[log.Component](t, log.MockModule)
	timeNow = MockTimeNow
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
	acc := newFlowAccumulator(flushInterval, flowContextTTL, common.DefaultAggregatorPortRollupThreshold, false, logger)
	acc.add(flow)

	// Then
	assert.Equal(t, 1, len(acc.flows))

	wrappedFlow := acc.flows[flow.AggregationHash()]

	assert.Equal(t, MockTimeNow(), wrappedFlow.nextFlush)
	assert.Equal(t, zeroTime, wrappedFlow.lastSuccessfulFlush)

	// test first flush
	// set flush time
	flushTime1 := MockTimeNow().Add(10 * time.Second)
	setMockTimeNow(flushTime1)
	acc.flush()
	wrappedFlow = acc.flows[flow.AggregationHash()]
	assert.Equal(t, MockTimeNow().Add(acc.flowFlushInterval), wrappedFlow.nextFlush)
	assert.Equal(t, MockTimeNow().Add(10*time.Second), wrappedFlow.lastSuccessfulFlush)

	// test skip flush if nextFlush is not reached yet
	flushTime2 := MockTimeNow().Add(15 * time.Second)
	setMockTimeNow(flushTime2)
	acc.flush()
	wrappedFlow = acc.flows[flow.AggregationHash()]
	assert.Equal(t, MockTimeNow().Add(acc.flowFlushInterval), wrappedFlow.nextFlush)
	assert.Equal(t, MockTimeNow().Add(10*time.Second), wrappedFlow.lastSuccessfulFlush)

	// test flush with no new flow after nextFlush is reached
	flushTime3 := MockTimeNow().Add(acc.flowFlushInterval + (1 * time.Second))
	setMockTimeNow(flushTime3)
	acc.flush()
	wrappedFlow = acc.flows[flow.AggregationHash()]
	assert.Equal(t, MockTimeNow().Add(acc.flowFlushInterval*2), wrappedFlow.nextFlush)
	// lastSuccessfulFlush time doesn't change because there is no new flow
	assert.Equal(t, MockTimeNow().Add(10*time.Second), wrappedFlow.lastSuccessfulFlush)

	// test flush with new flow after nextFlush is reached
	flushTime4 := MockTimeNow().Add(acc.flowFlushInterval*2 + (1 * time.Second))
	setMockTimeNow(flushTime4)
	acc.add(flow)
	acc.flush()
	wrappedFlow = acc.flows[flow.AggregationHash()]
	assert.Equal(t, MockTimeNow().Add(acc.flowFlushInterval*3), wrappedFlow.nextFlush)
	assert.Equal(t, flushTime4, wrappedFlow.lastSuccessfulFlush)

	// test flush with TTL reached (now+ttl is equal last successful flush) to clean up entry
	flushTime5 := flushTime4.Add(flowContextTTL + 1*time.Second)
	setMockTimeNow(flushTime5)
	acc.flush()
	_, ok := acc.flows[flow.AggregationHash()]
	assert.False(t, ok)

	// test flush with TTL reached (now+ttl is after last successful flush) to clean up entry
	setMockTimeNow(MockTimeNow())
	acc.add(flow)
	acc.flush()
	flushTime6 := MockTimeNow().Add(flowContextTTL + 1*time.Second)
	setMockTimeNow(flushTime6)
	acc.flush()
	_, ok = acc.flows[flow.AggregationHash()]
	assert.False(t, ok)
}
