package flowaggregator

import (
	"github.com/DataDog/datadog-agent/pkg/netflow/common"
	"github.com/stretchr/testify/assert"
	"net"
	"testing"
	"time"
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
	// Given
	flowA1 := &common.Flow{
		FlowType:       common.TypeNetFlow9,
		ExporterAddr:   net.IP([]byte{127, 0, 0, 1}).String(),
		StartTimestamp: 1234568,
		EndTimestamp:   1234569,
		Bytes:          20,
		Packets:        4,
		SrcAddr:        net.IP([]byte{10, 10, 10, 10}).String(),
		DstAddr:        net.IP([]byte{10, 10, 10, 20}).String(),
		IPProtocol:     uint32(6),
		SrcPort:        uint32(2000),
		DstPort:        uint32(80),
	}
	flowA2 := &common.Flow{
		FlowType:       common.TypeNetFlow9,
		ExporterAddr:   net.IP([]byte{127, 0, 0, 1}).String(),
		StartTimestamp: 1234578,
		EndTimestamp:   1234579,
		Bytes:          10,
		Packets:        2,
		SrcAddr:        net.IP([]byte{10, 10, 10, 10}).String(),
		DstAddr:        net.IP([]byte{10, 10, 10, 20}).String(),
		IPProtocol:     uint32(6),
		SrcPort:        uint32(2000),
		DstPort:        uint32(80),
	}
	flowB1 := &common.Flow{
		FlowType:       common.TypeNetFlow9,
		ExporterAddr:   net.IP([]byte{127, 0, 0, 1}).String(),
		StartTimestamp: 1234568,
		EndTimestamp:   1234569,
		Bytes:          10,
		Packets:        2,
		SrcAddr:        net.IP([]byte{10, 10, 10, 10}).String(),
		// different destination addr
		DstAddr:    net.IP([]byte{10, 10, 10, 30}).String(),
		IPProtocol: uint32(6),
		SrcPort:    uint32(2000),
		DstPort:    uint32(80),
	}

	// When
	acc := newFlowAccumulator(60)
	acc.add(flowA1)
	acc.add(flowA2)
	acc.add(flowB1)

	// Then
	assert.Equal(t, 2, len(acc.flows))

	wrappedFlowA := acc.flows[flowA1.AggregationHash()]
	assert.Equal(t, net.IP([]byte{10, 10, 10, 10}).String(), wrappedFlowA.flow.SrcAddr)
	assert.Equal(t, net.IP([]byte{10, 10, 10, 20}).String(), wrappedFlowA.flow.DstAddr)
	assert.Equal(t, uint64(30), wrappedFlowA.flow.Bytes)
	assert.Equal(t, uint64(6), wrappedFlowA.flow.Packets)
	assert.Equal(t, uint64(1234568), wrappedFlowA.flow.StartTimestamp)
	assert.Equal(t, uint64(1234579), wrappedFlowA.flow.EndTimestamp)

	wrappedFlowB := acc.flows[flowB1.AggregationHash()]
	assert.Equal(t, net.IP([]byte{10, 10, 10, 10}).String(), wrappedFlowB.flow.SrcAddr)
	assert.Equal(t, net.IP([]byte{10, 10, 10, 30}).String(), wrappedFlowB.flow.DstAddr)
}

func Test_flowAccumulator_flush(t *testing.T) {
	timeNow = MockTimeNow
	zeroTime := time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
	flushInterval := 60 * time.Second

	// Given
	flow := &common.Flow{
		FlowType:       common.TypeNetFlow9,
		ExporterAddr:   net.IP([]byte{127, 0, 0, 1}).String(),
		StartTimestamp: 1234568,
		EndTimestamp:   1234569,
		Bytes:          20,
		Packets:        4,
		SrcAddr:        net.IP([]byte{10, 10, 10, 10}).String(),
		DstAddr:        net.IP([]byte{10, 10, 10, 20}).String(),
		IPProtocol:     uint32(6),
		SrcPort:        uint32(2000),
		DstPort:        uint32(80),
	}

	// When
	acc := newFlowAccumulator(flushInterval)
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

	// test flush with TTL reached to clean up entry
	flushTime5 := flushTime4.Add((acc.flowContextTTL + 1) * time.Second)
	setMockTimeNow(flushTime5)
	acc.flush()
	_, ok := acc.flows[flow.AggregationHash()]
	assert.False(t, ok)
}
