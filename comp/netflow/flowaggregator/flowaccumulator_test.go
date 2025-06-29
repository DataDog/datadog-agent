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

// TestStartTime mocks time.Now
var TestStartTime = func() time.Time {
	layout := "2006-01-02 15:04:05"
	str := "2000-01-01 00:00:00"
	t, _ := time.Parse(layout, str)
	return t
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
	acc := newFlowAccumulator(common.DefaultAggregatorFlushInterval, common.DefaultAggregatorFlushInterval, common.DefaultAggregatorPortRollupThreshold, false, logger, rdnsQuerier)
	acc.add(flowA1, TestStartTime())
	acc.add(flowA2, TestStartTime())
	acc.add(flowB1, TestStartTime())

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
	assert.Equal(t, map[string]any{"custom_field": "test", "custom_field_2": "second_flow_field"}, wrappedFlowA.flow.AdditionalFields) // Keeping first value seen for key `custom_field`

	wrappedFlowB := acc.flows[flowB1.AggregationHash()]
	assert.Equal(t, []byte{10, 10, 10, 10}, wrappedFlowB.flow.SrcAddr)
	assert.Equal(t, []byte{10, 10, 10, 30}, wrappedFlowB.flow.DstAddr)
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
	acc.add(flowA1, TestStartTime())
	acc.add(flowA2, TestStartTime())

	flowB1a := flowB1
	acc.add(&flowB1a, TestStartTime())
	flowB1b := flowB1 // send flowB1 twice to test that it's not counted twice by portRollup tracker
	acc.add(&flowB1b, TestStartTime())
	assert.Equal(t, uint16(1), acc.portRollup.GetSourceToDestPortCount([]byte{10, 10, 10, 10}, []byte{10, 10, 10, 30}, 80))
	assert.Equal(t, uint16(1), acc.portRollup.GetDestToSourcePortCount([]byte{10, 10, 10, 10}, []byte{10, 10, 10, 30}, 2001))

	flowB2 := flowB1
	flowB2.DstPort = 2002
	acc.add(&flowB2, TestStartTime())
	flowB3 := flowB1
	flowB3.DstPort = 2003
	acc.add(&flowB3, TestStartTime())
	flowB4 := flowB1
	flowB4.DstPort = 2004
	acc.add(&flowB4, TestStartTime())
	flowB5 := flowB1
	flowB5.DstPort = 2005
	acc.add(&flowB5, TestStartTime())
	flowB6 := flowB1
	flowB6.DstPort = 2006
	acc.add(&flowB6, TestStartTime())

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
	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())
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
	acc.add(flow, TestStartTime())

	// Then
	assert.Equal(t, 1, len(acc.flows))

	flowContext := acc.flows[flow.AggregationHash()]

	// test that nextFlush is set to the first flush time and that lastSuccessfulFlush has not been changed
	assert.Equal(t, TestStartTime().Add(acc.flowFlushInterval), flowContext.nextFlush)
	assert.Equal(t, zeroTime, flowContext.lastSuccessfulFlush)

	// test first flush
	// set flush time
	firstFlushTime := TestStartTime().Add(acc.flowFlushInterval)
	acc.flush(firstFlushTime) // successful flush
	flowContext = acc.flows[flow.AggregationHash()]
	assert.Equal(t, firstFlushTime.Add(acc.flowFlushInterval), flowContext.nextFlush)
	assert.Equal(t, firstFlushTime, flowContext.lastSuccessfulFlush)

	// test skip flush if nextFlush is not reached yet
	timeBeforeNextFlush := TestStartTime().Add(acc.flowFlushInterval + (1 * time.Second))
	acc.flush(timeBeforeNextFlush) // no flush because nextFlush is not reached yet
	flowContext = acc.flows[flow.AggregationHash()]
	assert.Equal(t, firstFlushTime.Add(acc.flowFlushInterval), flowContext.nextFlush)
	assert.Equal(t, firstFlushTime, flowContext.lastSuccessfulFlush)

	// test flush with no new flow after nextFlush is reached
	secondFlushTime := TestStartTime().Add(acc.flowFlushInterval + (acc.flowContextTTL / 2))
	acc.flush(secondFlushTime)
	flowContext = acc.flows[flow.AggregationHash()]
	assert.Equal(t, TestStartTime().Add(acc.flowFlushInterval*2), flowContext.nextFlush)
	// lastSuccessfulFlush time doesn't change because there is no new flow
	assert.Equal(t, firstFlushTime, flowContext.lastSuccessfulFlush)

	// test flush with new flow after nextFlush is reached
	thirdFlushTime := TestStartTime().Add(acc.flowFlushInterval*2 + (1 * time.Second))
	acc.add(flow, thirdFlushTime)
	acc.flush(thirdFlushTime) // successful flush
	flowContext = acc.flows[flow.AggregationHash()]
	assert.Equal(t, TestStartTime().Add(acc.flowFlushInterval*3), flowContext.nextFlush)
	assert.Equal(t, thirdFlushTime, flowContext.lastSuccessfulFlush)

	// test flush with TTL reached (now+ttl is equal last successful flush) to clean up entry
	ttlExpirationTime := thirdFlushTime.Add(flowContextTTL + 1*time.Second)
	acc.flush(ttlExpirationTime)
	_, ok := acc.flows[flow.AggregationHash()]
	assert.False(t, ok)

	// test flush with TTL reached for a newly added flow
	// This test case verifies that when a new flow is added and flushed,
	// it will be cleaned up if no new flows are seen for the TTL period
	acc.add(flow, TestStartTime())
	flowContext = acc.flows[flow.AggregationHash()]
	// Wait for the flush interval before flushing
	flushTime := TestStartTime().Add(acc.flowFlushInterval)
	acc.flush(flushTime)
	flowContext = acc.flows[flow.AggregationHash()]
	finalTTLExpirationTime := flushTime.Add(flowContextTTL + 1*time.Second)
	acc.flush(finalTTLExpirationTime)
	_, ok = acc.flows[flow.AggregationHash()]
	assert.False(t, ok)
}

func Test_flowAccumulator_detectHashCollision(t *testing.T) {
	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())
	synFlag := uint32(2)
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
