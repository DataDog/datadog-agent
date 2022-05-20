package common

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFlow_AggregationHash(t *testing.T) {
	origFlow := Flow{
		SrcAddr:        []byte{1, 2, 3, 4},
		DstAddr:        []byte{2, 3, 4, 5},
		IPProtocol:     6,
		SrcPort:        2000,
		DstPort:        80,
		InputInterface: 1,
		Tos:            0,
	}
	origHash := origFlow.AggregationHash()
	assert.Equal(t, uint64(0x9d1b9f25b350fdab), origHash)

	flow := origFlow
	flow.SrcAddr = []byte{1, 2, 3, 5}
	assert.NotEqual(t, origHash, flow.AggregationHash())

	flow = origFlow
	flow.DstAddr = []byte{2, 3, 4, 6}
	assert.NotEqual(t, origHash, flow.AggregationHash())

	flow = origFlow
	flow.IPProtocol = 7
	assert.NotEqual(t, origHash, flow.AggregationHash())

	flow = origFlow
	flow.SrcPort = 3000
	assert.NotEqual(t, origHash, flow.AggregationHash())

	flow = origFlow
	flow.DstPort = 443
	assert.NotEqual(t, origHash, flow.AggregationHash())

	flow = origFlow
	flow.InputInterface = 2
	assert.NotEqual(t, origHash, flow.AggregationHash())

	flow = origFlow
	flow.Tos = 1
	assert.NotEqual(t, origHash, flow.AggregationHash())

	// OutputInterface is not a key field, changing it should not change the hash
	flow = origFlow
	flow.OutputInterface = 1
	assert.Equal(t, origHash, flow.AggregationHash())

	// EtherType is not a key field, changing it should not change the hash
	flow = origFlow
	flow.EtherType = 1
	assert.Equal(t, origHash, flow.AggregationHash())
}
