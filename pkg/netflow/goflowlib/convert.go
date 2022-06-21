package goflowlib

import (
	flowpb "github.com/netsampler/goflow2/pb"

	"github.com/DataDog/datadog-agent/pkg/netflow/common"
)

// ConvertFlow convert goflow flow structure to internal flow structure
func ConvertFlow(srcFlow *flowpb.FlowMessage, namespace string) *common.Flow {
	return &common.Flow{
		Namespace:       namespace,
		FlowType:        convertFlowType(srcFlow.Type),
		SamplingRate:    srcFlow.SamplingRate,
		Direction:       srcFlow.FlowDirection,
		ExporterAddr:    srcFlow.SamplerAddress,
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
		SrcPort:         srcFlow.SrcPort,
		DstPort:         srcFlow.DstPort,
		InputInterface:  srcFlow.InIf,
		OutputInterface: srcFlow.OutIf,
		Tos:             srcFlow.IPTos,
		NextHop:         srcFlow.NextHop,
		TCPFlags:        srcFlow.TCPFlags,
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
