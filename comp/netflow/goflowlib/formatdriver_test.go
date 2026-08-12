// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package goflowlib

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

func baseFlow() *common.Flow {
	return &common.Flow{
		SrcAddr:          []byte{10, 0, 0, 1},
		DstAddr:          []byte{10, 0, 0, 2},
		SrcPort:          1234,
		DstPort:          80,
		InputInterface:   1,
		OutputInterface:  2,
		AdditionalFields: common.AdditionalFields{},
	}
}

func TestSplitBiflow(t *testing.T) {

	// biflow
	flow := baseFlow()
	flow.AdditionalFields[biflowInitiatorOctets] = uint64(1000)
	flow.AdditionalFields[biflowInitiatorPackets] = uint64(10)
	flow.AdditionalFields[biflowResponderOctets] = uint64(500)
	flow.AdditionalFields[biflowResponderPackets] = uint64(5)
	flow.AdditionalFields["my_custom_field"] = "overnight_oats"

	fwd, rev := splitBiflow(flow)

	require.NotNil(t, fwd)
	require.NotNil(t, rev)

	assert.Equal(t, uint64(1000), fwd.Bytes)
	assert.Equal(t, uint64(10), fwd.Packets)
	assert.Equal(t, []byte{10, 0, 0, 1}, fwd.SrcAddr)
	assert.Equal(t, []byte{10, 0, 0, 2}, fwd.DstAddr)
	assert.Equal(t, int32(1234), fwd.SrcPort)
	assert.Equal(t, int32(80), fwd.DstPort)

	assert.Equal(t, uint64(500), rev.Bytes)
	assert.Equal(t, uint64(5), rev.Packets)
	assert.Equal(t, []byte{10, 0, 0, 2}, rev.SrcAddr)
	assert.Equal(t, []byte{10, 0, 0, 1}, rev.DstAddr)
	assert.Equal(t, int32(80), rev.SrcPort)
	assert.Equal(t, int32(1234), rev.DstPort)
	assert.Equal(t, uint32(2), rev.InputInterface)
	assert.Equal(t, uint32(1), rev.OutputInterface)
	assert.Equal(t, uint32(1), rev.Direction)

	assert.Equal(t, "overnight_oats", fwd.AdditionalFields["my_custom_field"])
	assert.Equal(t, 1, len(fwd.AdditionalFields)) // all of the biflow fields should be deleted

	// uniflow
	flow = baseFlow()
	flow.AdditionalFields[biflowInitiatorOctets] = uint64(1000)
	flow.AdditionalFields[biflowInitiatorPackets] = uint64(10)
	flow.AdditionalFields[biflowResponderOctets] = uint64(0)

	fwd, rev = splitBiflow(flow)

	require.NotNil(t, fwd)
	assert.Nil(t, rev)
	assert.Equal(t, uint64(1000), fwd.Bytes)
	assert.Equal(t, uint64(10), fwd.Packets)
}
