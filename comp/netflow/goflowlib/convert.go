// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package goflowlib provides converters between the goflow library and the
// types used internally for netflow at Datadog.
package goflowlib

import (
	"encoding/hex"
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

// Used to map biflow byte/packet counts through additionalFields
const (
	biflowInitiatorOctets  = "datadog.initiator_octets"
	biflowResponderOctets  = "datadog.responder_octets"
	biflowInitiatorPackets = "datadog.initiator_packets"
	biflowResponderPackets = "datadog.responder_packets"
)

// ConvertFlowWithAdditionalFields converts a flow with additional fields to internal flow structure(s).
// When enableBiflowParsing is true and biflow sentinels are present, a reverse flow is also returned.
func ConvertFlowWithAdditionalFields(srcFlow *common.FlowMessageWithAdditionalFields, namespace string, enableBiflowParsing bool) (*common.Flow, *common.Flow) {
	flow := ConvertFlow(srcFlow.FlowMessage, namespace)
	applyAdditionalFields(flow, srcFlow.AdditionalFields)
	if enableBiflowParsing {
		return splitBiflow(flow)
	}
	return flow, nil
}

// splitBiflow detects bidirectional flow records and splits them into two unidirectional flows.
func splitBiflow(flow *common.Flow) (*common.Flow, *common.Flow) {
	if flow.AdditionalFields == nil {
		return flow, nil
	}
	initOctets, hasInitOctets := flow.AdditionalFields[biflowInitiatorOctets].(uint64)
	initPkts, _ := flow.AdditionalFields[biflowInitiatorPackets].(uint64)
	respOctets, hasRespOctets := flow.AdditionalFields[biflowResponderOctets].(uint64)
	respPkts, _ := flow.AdditionalFields[biflowResponderPackets].(uint64)

	delete(flow.AdditionalFields, biflowInitiatorOctets)
	delete(flow.AdditionalFields, biflowInitiatorPackets)
	delete(flow.AdditionalFields, biflowResponderOctets)
	delete(flow.AdditionalFields, biflowResponderPackets)

	var revBytes, revPkts uint64
	hasRev := false

	if hasInitOctets {
		flow.Bytes = initOctets
		flow.Packets = initPkts
		if hasRespOctets && (respOctets > 0 || respPkts > 0) {
			revBytes, revPkts, hasRev = respOctets, respPkts, true
		}
	}

	if !hasRev {
		return flow, nil
	}

	// copy flow and swap src/dst for reverse direction
	rev := *flow
	rev.SrcAddr = append([]byte(nil), flow.DstAddr...)
	rev.DstAddr = append([]byte(nil), flow.SrcAddr...)
	rev.SrcPort, rev.DstPort = flow.DstPort, flow.SrcPort
	rev.SrcMac, rev.DstMac = flow.DstMac, flow.SrcMac
	rev.SrcMask, rev.DstMask = flow.DstMask, flow.SrcMask
	rev.SrcReverseDNSHostname = flow.DstReverseDNSHostname
	rev.DstReverseDNSHostname = flow.SrcReverseDNSHostname
	rev.InputInterface, rev.OutputInterface = flow.OutputInterface, flow.InputInterface
	rev.Bytes, rev.Packets = revBytes, revPkts
	rev.Direction = 1 // egress

	if flow.AdditionalFields != nil {
		rev.AdditionalFields = make(common.AdditionalFields, len(flow.AdditionalFields))
		for k, v := range flow.AdditionalFields {
			rev.AdditionalFields[k] = v
		}
	}
	return flow, &rev
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

func applyAdditionalFields(flow *common.Flow, additionalFields common.AdditionalFields) {
	if additionalFields == nil {
		return
	}

	processedFields := make(common.AdditionalFields)
	for destination, fieldValue := range additionalFields {
		applied := applyAdditionalField(flow, destination, fieldValue)
		if !applied {
			// Additional field need to be stored in the new map
			if field, ok := fieldValue.([]byte); ok {
				// Write []byte as hex string for readability
				processedFields[destination] = bytesToHexString(field)
			} else {
				processedFields[destination] = fieldValue
			}
		}
	}
	flow.AdditionalFields = processedFields
}

func applyAdditionalField(flow *common.Flow, destination string, fieldValue any) bool {
	// Make sure DefaultFieldTypes includes the type of the following fields
	switch destination {
	case "direction":
		setInt(&flow.Direction, fieldValue)
	case "start":
		setInt(&flow.StartTimestamp, fieldValue)
	case "end":
		setInt(&flow.EndTimestamp, fieldValue)
	case "bytes":
		setInt(&flow.Bytes, fieldValue)
	case "packets":
		setInt(&flow.Packets, fieldValue)
	case "ether_type":
		setInt(&flow.EtherType, fieldValue)
	case "ip_protocol":
		setInt(&flow.IPProtocol, fieldValue)
	case "exporter.ip":
		setBytes(&flow.ExporterAddr, fieldValue)
	case "source.ip":
		setBytes(&flow.SrcAddr, fieldValue)
	case "source.port":
		var port uint64
		setInt(&port, fieldValue)
		flow.SrcPort = int32(port)
	case "source.mac":
		setInt(&flow.SrcMac, fieldValue)
	case "source.mask":
		setInt(&flow.SrcMask, fieldValue)
	case "destination.ip":
		setBytes(&flow.DstAddr, fieldValue)
	case "destination.port":
		var port uint64
		setInt(&port, fieldValue)
		flow.DstPort = int32(port)
	case "destination.mac":
		setInt(&flow.DstMac, fieldValue)
	case "destination.mask":
		setInt(&flow.DstMask, fieldValue)
	case "ingress.interface":
		setInt(&flow.InputInterface, fieldValue)
	case "egress.interface":
		setInt(&flow.OutputInterface, fieldValue)
	case "tcp_flags":
		setInt(&flow.TCPFlags, fieldValue)
	case "next_hop.ip":
		setBytes(&flow.NextHop, fieldValue)
	case "tos":
		setInt(&flow.Tos, fieldValue)
	default:
		return false
	}
	return true
}

func setInt[T uint64 | uint32](field *T, value any) {
	if v, ok := value.(uint64); ok {
		*field = T(v)
	}
}

func setBytes(field *[]byte, value any) {
	if v, ok := value.([]byte); ok {
		*field = v
	}
}

func bytesToHexString(value []byte) string {
	return hex.EncodeToString(value)
}
