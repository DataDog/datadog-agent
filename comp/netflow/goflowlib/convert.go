// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package goflowlib provides converters between the goflow library and the
// types used internally for netflow at Datadog.
package goflowlib

import (
	"encoding/hex"
	"github.com/DataDog/datadog-agent/comp/netflow/config"
	flowpb "github.com/netsampler/goflow2/v2/pb"
	protoproducer "github.com/netsampler/goflow2/v2/producer/proto"
	"google.golang.org/protobuf/encoding/protowire"
	"strings"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

// ConvertFlow convert goflow flow structure to internal flow structure
func ConvertFlow(srcFlow *protoproducer.ProtoProducerMessage, namespace string, fieldsById map[int32]config.NetFlowMapping) *common.Flow {
	return &common.Flow{
		Namespace:    namespace,
		FlowType:     convertFlowType(srcFlow.Type),
		SequenceNum:  srcFlow.SequenceNum,
		SamplingRate: srcFlow.SamplingRate,
		// Direction:       srcFlow.FlowDirection FIXME : should support direction,
		ExporterAddr:     srcFlow.SamplerAddress, // Sampler is renamed to Exporter since it's a more commonly used
		StartTimestamp:   srcFlow.TimeFlowStartNs / 1000000000,
		EndTimestamp:     srcFlow.TimeFlowEndNs / 1000000000,
		Bytes:            srcFlow.Bytes,
		Packets:          srcFlow.Packets,
		SrcAddr:          srcFlow.SrcAddr,
		DstAddr:          srcFlow.DstAddr,
		SrcMac:           srcFlow.SrcMac,
		DstMac:           srcFlow.DstMac,
		SrcMask:          srcFlow.SrcNet,
		DstMask:          srcFlow.DstNet,
		EtherType:        srcFlow.Etype,
		IPProtocol:       srcFlow.Proto,
		SrcPort:          int32(srcFlow.SrcPort),
		DstPort:          int32(srcFlow.DstPort),
		InputInterface:   srcFlow.InIf,
		OutputInterface:  srcFlow.OutIf,
		Tos:              srcFlow.IpTos,
		NextHop:          srcFlow.NextHop,
		TCPFlags:         srcFlow.TcpFlags,
		AdditionalFields: mapUnknown(srcFlow, fieldsById),
	}
}

func convertFlowType(flowType flowpb.FlowMessage_FlowType) common.FlowType {
	var flowTypeStr common.FlowType
	switch flowType {
	case flowpb.FlowMessage_SFLOW_5:
		flowTypeStr = common.TypeSFlow5
	case flowpb.FlowMessage_NETFLOW_V5:
		flowTypeStr = common.TypeNetFlow5
	case flowpb.FlowMessage_NETFLOW_V9:
		flowTypeStr = common.TypeNetFlow9
	case flowpb.FlowMessage_IPFIX:
		flowTypeStr = common.TypeIPFIX
	default:
		flowTypeStr = common.TypeUnknown
	}
	return flowTypeStr
}

// mapUnknown gets custom fields from goflow protobuf
func mapUnknown(m *protoproducer.ProtoProducerMessage, fieldsById map[int32]config.NetFlowMapping) map[string]interface{} {
	unkMap := make(map[string]interface{})

	fmr := m.ProtoReflect()
	unk := fmr.GetUnknown()
	var offset int
	for offset < len(unk) {
		num, dataType, length := protowire.ConsumeTag(unk[offset:])
		offset += length
		length = protowire.ConsumeFieldValue(num, dataType, unk[offset:])
		data := unk[offset : offset+length]
		offset += length

		// we check if the index is listed in the config
		if field, ok := fieldsById[int32(num)]; ok {
			var value interface{}
			if dataType == protowire.VarintType {
				v, _ := protowire.ConsumeVarint(data)
				value = v
			} else if dataType == protowire.BytesType {
				v, _ := protowire.ConsumeString(data)
				if field.Type == common.String {
					value = strings.TrimRight(v, "\x00") // Remove trailing null chars
				} else {
					value = hex.EncodeToString([]byte(v))
				}
			} else {
				continue
			}

			unkMap[field.Destination] = value

		}
	}
	return unkMap
}
