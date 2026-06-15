// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package goflowlib

import (
	"context"
	"errors"

	"go.uber.org/atomic"

	flowpb "github.com/netsampler/goflow2/pb"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

// Used to map biflow byte/packet counts through additionalFields
const (
	biflowInitiatorOctets  = "datadog.initiator_octets"
	biflowResponderOctets  = "datadog.responder_octets"
	biflowInitiatorPackets = "datadog.initiator_packets"
	biflowResponderPackets = "datadog.responder_packets"
)

// AggregatorFormatDriver is used as goflow formatter to forward flow data to aggregator/EP Forwarder
type AggregatorFormatDriver struct {
	namespace         string
	flowAggIn         chan *common.Flow
	listenerFlowCount *atomic.Int64
}

// NewAggregatorFormatDriver returns a new AggregatorFormatDriver
func NewAggregatorFormatDriver(flowAgg chan *common.Flow, namespace string, listenerFlowCount *atomic.Int64) *AggregatorFormatDriver {
	return &AggregatorFormatDriver{
		namespace:         namespace,
		flowAggIn:         flowAgg,
		listenerFlowCount: listenerFlowCount,
	}
}

// Prepare desc
func (d *AggregatorFormatDriver) Prepare() error {
	return nil
}

// Init desc
func (d *AggregatorFormatDriver) Init(context.Context) error {
	return nil
}

// Format desc
func (d *AggregatorFormatDriver) Format(data interface{}) ([]byte, []byte, error) {
	switch flow := data.(type) {
	case *flowpb.FlowMessage:
		d.listenerFlowCount.Add(1)
		d.flowAggIn <- ConvertFlow(flow, d.namespace)
	case *common.FlowMessageWithAdditionalFields:
		fwd, rev := splitBiflow(ConvertFlowWithAdditionalFields(flow, d.namespace))
		d.listenerFlowCount.Add(1)
		d.flowAggIn <- fwd
		if rev != nil {
			d.listenerFlowCount.Add(1)
			d.flowAggIn <- rev
		}
	default:
		return nil, nil, errors.New("message is not flowpb.FlowMessage or common.FlowMessageWithAdditionalFields")
	}

	return nil, nil, nil
}

// Detects bidirectional flow records and splits them into two unidirectional flows
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
		if hasRespOctets && respOctets > 0 {
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
