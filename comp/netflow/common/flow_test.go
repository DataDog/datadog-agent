// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlow_AggregationHash(t *testing.T) {
	allHash := make(map[uint64]bool)
	origFlow := Flow{
		Namespace:      "default",
		ExporterAddr:   []byte{127, 0, 0, 1},
		SrcAddr:        []byte{1, 2, 3, 4},
		DstAddr:        []byte{2, 3, 4, 5},
		IPProtocol:     6,
		SrcPort:        2000,
		DstPort:        80,
		InputInterface: 1,
		Tos:            0,
	}
	origHash := origFlow.AggregationHash()
	assert.Equal(t, uint64(0x5f66aff870a0f86a), origHash)
	allHash[origHash] = true

	flow := origFlow
	flow.Namespace = "my-new-ns"
	assert.NotEqual(t, origHash, flow.AggregationHash())
	allHash[flow.AggregationHash()] = true

	flow = origFlow
	flow.ExporterAddr = []byte{127, 0, 0, 2}
	assert.NotEqual(t, origHash, flow.AggregationHash())
	allHash[flow.AggregationHash()] = true

	flow = origFlow
	flow.SrcAddr = []byte{1, 2, 3, 5}
	assert.NotEqual(t, origHash, flow.AggregationHash())
	allHash[flow.AggregationHash()] = true

	flow = origFlow
	flow.DstAddr = []byte{2, 3, 4, 6}
	assert.NotEqual(t, origHash, flow.AggregationHash())
	allHash[flow.AggregationHash()] = true

	flow = origFlow
	flow.IPProtocol = 7
	assert.NotEqual(t, origHash, flow.AggregationHash())
	allHash[flow.AggregationHash()] = true

	flow = origFlow
	flow.SrcPort = 3000
	assert.NotEqual(t, origHash, flow.AggregationHash())
	allHash[flow.AggregationHash()] = true

	flow = origFlow
	flow.DstPort = 443
	assert.NotEqual(t, origHash, flow.AggregationHash())
	allHash[flow.AggregationHash()] = true

	flow = origFlow
	flow.InputInterface = 2
	assert.NotEqual(t, origHash, flow.AggregationHash())
	allHash[flow.AggregationHash()] = true

	flow = origFlow
	flow.Tos = 1
	assert.NotEqual(t, origHash, flow.AggregationHash())
	allHash[flow.AggregationHash()] = true

	// OutputInterface is not a key field, changing it should not change the hash
	flow = origFlow
	flow.OutputInterface = 1
	assert.Equal(t, origHash, flow.AggregationHash())
	allHash[flow.AggregationHash()] = true

	// EtherType is not a key field, changing it should not change the hash
	flow = origFlow
	flow.EtherType = 1
	assert.Equal(t, origHash, flow.AggregationHash())
	allHash[flow.AggregationHash()] = true

	// Should contain expected number of different hashes
	assert.Equal(t, 10, len(allHash))
}

func TestFlow_IsEqualFlowContext(t *testing.T) {
	origFlow := Flow{
		Namespace:      "default",
		ExporterAddr:   []byte{127, 0, 0, 1},
		SrcAddr:        []byte{1, 2, 3, 4},
		DstAddr:        []byte{2, 3, 4, 5},
		IPProtocol:     6,
		SrcPort:        2000,
		DstPort:        80,
		InputInterface: 1,
		Tos:            0,
		Bytes:          5,
	}

	otherFlow := Flow{
		Namespace:      "default",
		ExporterAddr:   []byte{127, 0, 0, 1},
		SrcAddr:        []byte{1, 2, 3, 4},
		DstAddr:        []byte{2, 3, 4, 5},
		IPProtocol:     6,
		SrcPort:        2000,
		DstPort:        80,
		InputInterface: 1,
		Tos:            0,
		Bytes:          10,
	}

	assert.True(t, IsEqualFlowContext(origFlow, otherFlow))

	flow := origFlow
	flow.Namespace = "abc"
	assert.False(t, IsEqualFlowContext(origFlow, flow))

	flow = origFlow
	flow.ExporterAddr = []byte{127, 0, 0, 2}
	assert.False(t, IsEqualFlowContext(origFlow, flow))

	flow = origFlow
	flow.SrcAddr = []byte{1, 2, 3, 5}
	assert.False(t, IsEqualFlowContext(origFlow, flow))

	flow = origFlow
	flow.DstAddr = []byte{2, 3, 4, 6}
	assert.False(t, IsEqualFlowContext(origFlow, flow))

	flow = origFlow
	flow.IPProtocol = 7
	assert.False(t, IsEqualFlowContext(origFlow, flow))

	flow = origFlow
	flow.SrcPort = 2001
	assert.False(t, IsEqualFlowContext(origFlow, flow))

	flow = origFlow
	flow.DstPort = 81
	assert.False(t, IsEqualFlowContext(origFlow, flow))

	flow = origFlow
	flow.InputInterface = 2
	assert.False(t, IsEqualFlowContext(origFlow, flow))

	flow = origFlow
	flow.Tos = 1
	assert.False(t, IsEqualFlowContext(origFlow, flow))

	flow = origFlow
	flow.Bytes = 999
	assert.True(t, IsEqualFlowContext(origFlow, flow))
}
