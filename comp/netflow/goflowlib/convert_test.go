// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package goflowlib

import (
	"testing"

	flowpb "github.com/netsampler/goflow2/pb"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

func Test_convertFlowType(t *testing.T) {
	tests := []struct {
		goflowFlowType   flowpb.FlowMessage_FlowType
		expectedFlowType common.FlowType
	}{
		{
			goflowFlowType:   flowpb.FlowMessage_SFLOW_5,
			expectedFlowType: common.TypeSFlow5,
		},
		{
			goflowFlowType:   flowpb.FlowMessage_NETFLOW_V5,
			expectedFlowType: common.TypeNetFlow5,
		},
		{
			goflowFlowType:   flowpb.FlowMessage_NETFLOW_V9,
			expectedFlowType: common.TypeNetFlow9,
		},
		{
			goflowFlowType:   flowpb.FlowMessage_IPFIX,
			expectedFlowType: common.TypeIPFIX,
		},
		{
			goflowFlowType:   99,
			expectedFlowType: common.TypeUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(string(tt.expectedFlowType), func(t *testing.T) {
			assert.Equal(t, tt.expectedFlowType, convertFlowType(tt.goflowFlowType))
		})
	}
}

func TestConvertFlow(t *testing.T) {
	srcFlow := flowpb.FlowMessage{
		Type:           flowpb.FlowMessage_NETFLOW_V9,
		TimeReceived:   1234567,
		SequenceNum:    20,
		SamplingRate:   10,
		FlowDirection:  1,
		SamplerAddress: []byte{127, 0, 0, 1},
		TimeFlowStart:  1234568,
		TimeFlowEnd:    1234569,
		Bytes:          10,
		Packets:        2,
		SrcAddr:        []byte{10, 10, 10, 10},
		DstAddr:        []byte{10, 10, 10, 20},
		SrcMac:         uint64(10),
		DstMac:         uint64(20),
		SrcNet:         uint32(10),
		DstNet:         uint32(20),
		Etype:          uint32(1),
		Proto:          uint32(6),
		SrcPort:        uint32(2000),
		DstPort:        uint32(80),
		InIf:           10,
		OutIf:          20,
		IpTos:          3,
		NextHop:        []byte{10, 10, 10, 30},
	}
	expectedFlow := common.Flow{
		Namespace:       "my-ns",
		FlowType:        common.TypeNetFlow9,
		SequenceNum:     20,
		SamplingRate:    10,
		Direction:       1,
		ExporterAddr:    []byte{127, 0, 0, 1},
		StartTimestamp:  1234568,
		EndTimestamp:    1234569,
		Bytes:           10,
		Packets:         2,
		SrcAddr:         []byte{10, 10, 10, 10},
		DstAddr:         []byte{10, 10, 10, 20},
		SrcMac:          uint64(10),
		DstMac:          uint64(20),
		SrcMask:         uint32(10),
		DstMask:         uint32(20),
		EtherType:       uint32(1),
		IPProtocol:      uint32(6),
		SrcPort:         2000,
		DstPort:         80,
		InputInterface:  10,
		OutputInterface: 20,
		Tos:             3,
		NextHop:         []byte{10, 10, 10, 30},
	}
	actualFlow := ConvertFlow(&srcFlow, "my-ns")
	assert.Equal(t, expectedFlow, *actualFlow)
}
