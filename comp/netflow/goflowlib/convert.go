// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package goflowlib provides converters between the goflow library and the
// types used internally for netflow at Datadog.
package goflowlib

import (
	flowpb "github.com/netsampler/goflow2/pb"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

// ConvertFlow convert goflow flow structure to internal flow structure
func ConvertFlow(srcFlow *flowpb.FlowMessage, namespace string) *common.Flow {
	return &common.Flow{
		Namespace:       namespace,
		FlowType:        convertFlowType(srcFlow.Type),
		SequenceNum:     srcFlow.SequenceNum,
		SamplingRate:    srcFlow.SamplingRate,
		Direction:       srcFlow.FlowDirection,
		ExporterAddr:    srcFlow.SamplerAddress, // Sampler is renamed to Exporter since it's a more commonly used
		StartTimestamp:  srcFlow.TimeFlowStart,
		EndTimestamp:    srcFlow.TimeFlowEnd,
		Bytes:           srcFlow.Bytes,
		Packets:         srcFlow.Packets,
		SrcAddr:         srcFlow.SrcAddr,
		DstAddr:         srcFlow.DstAddr,
		SrcMac:          srcFlow.SrcMac,
		DstMac:          srcFlow.DstMac,
		SrcMask:         srcFlow.SrcNet,
		DstMask:         srcFlow.DstNet,
		EtherType:       srcFlow.Etype,
		IPProtocol:      srcFlow.Proto,
		SrcPort:         int32(srcFlow.SrcPort),
		DstPort:         int32(srcFlow.DstPort),
		InputInterface:  srcFlow.InIf,
		OutputInterface: srcFlow.OutIf,
		Tos:             srcFlow.IpTos,
		NextHop:         srcFlow.NextHop,
		TCPFlags:        srcFlow.TcpFlags,
	}
}

// ConvertFlowWithCustomFields convert goflow flow structure and additional fields to internal flow structure
func ConvertFlowWithCustomFields(srcFlow *common.FlowMessageWithAdditionalFields, namespace string) *common.Flow {
	flow := ConvertFlow(srcFlow.FlowMessage, namespace)
	applyCustomFields(flow, srcFlow.AdditionalFields)
	return flow
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

func applyCustomFields(flow *common.Flow, additionalFields common.AdditionalFields) {
	for destination, fieldValue := range additionalFields {
		applied := applyCustomField(flow, destination, fieldValue)
		if applied {
			// We replaced a field of common.Flow with an additional field, no need to keep it in the map
			delete(additionalFields, destination)
		}
	}
	flow.AdditionalFields = additionalFields
}

func applyCustomField(flow *common.Flow, destination string, fieldValue any) bool {
	// Make sure FlowFieldsTypes includes the type of the following fields
	switch destination {
	case "direction":
		setValue(&flow.Direction, fieldValue)
	case "start":
		setValue(&flow.StartTimestamp, fieldValue)
	case "end":
		setValue(&flow.EndTimestamp, fieldValue)
	case "bytes":
		setValue(&flow.Bytes, fieldValue)
	case "packets":
		setValue(&flow.Packets, fieldValue)
	case "ether_type":
		setValue(&flow.EtherType, fieldValue)
	case "ip_protocol":
		setValue(&flow.IPProtocol, fieldValue)
	case "exporter.ip":
		setValue(&flow.ExporterAddr, fieldValue)
	case "source.ip":
		setValue(&flow.SrcAddr, fieldValue)
	case "source.port":
		var port uint64
		setValue(&port, fieldValue)
		flow.SrcPort = int32(port)
	case "source.mac":
		setValue(&flow.SrcMac, fieldValue)
	case "source.mask":
		setValue(&flow.SrcMask, fieldValue)
	case "destination.ip":
		setValue(&flow.DstAddr, fieldValue)
	case "destination.port":
		var port uint64
		setValue(&port, fieldValue)
		flow.DstPort = int32(port)
	case "destination.mac":
		setValue(&flow.DstMac, fieldValue)
	case "destination.mask":
		setValue(&flow.DstMask, fieldValue)
	case "ingress.interface":
		setValue(&flow.InputInterface, fieldValue)
	case "egress.interface":
		setValue(&flow.OutputInterface, fieldValue)
	case "tcp_flags":
		setValue(&flow.TCPFlags, fieldValue)
	case "next_hop.ip":
		setValue(&flow.NextHop, fieldValue)
	case "tos":
		setValue(&flow.Tos, fieldValue)
	default:
		return false
	}
	return true
}

type flowFieldsTypes interface {
	uint64 | uint32 | []byte
}

func setValue[T flowFieldsTypes](field *T, value any) {
	if v, ok := value.(T); ok {
		*field = v
	}
}
