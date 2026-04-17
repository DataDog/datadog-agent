// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/format"
	"github.com/DataDog/datadog-agent/comp/netflow/payload"
)

// buildMergedPayload builds a MergedFlowPayload from a group of reporter flows that share
// the same 5-tuple. Flow-level fields (src/dst, protocol) are taken from the first reporter;
// per-reporter fields (bytes, exporter, interfaces, etc.) populate the Reporters list.
// Ghost reporters (Bytes == 0, Packets == 0) are included so the platform can use them as
// metadata when assigning flow_role — they should not be counted.
//
// Callers must ensure reporters is non-empty; this function panics otherwise.
func buildMergedPayload(reporters []*common.Flow, hostname string, flushTime time.Time) payload.MergedFlowPayload {
	first := reporters[0]
	p := payload.MergedFlowPayload{
		FlushTimestamp: flushTime.UnixMilli(),
		EtherType:      format.EtherType(first.EtherType),
		IPProtocol:     format.IPProtocol(first.IPProtocol),
		Source: payload.Endpoint{
			IP:                 format.IPAddr(first.SrcAddr),
			Port:               format.Port(first.SrcPort),
			Mac:                format.MacAddress(first.SrcMac),
			Mask:               format.CIDR(first.SrcAddr, first.SrcMask),
			ReverseDNSHostname: first.SrcReverseDNSHostname,
		},
		Destination: payload.Endpoint{
			IP:                 format.IPAddr(first.DstAddr),
			Port:               format.Port(first.DstPort),
			Mac:                format.MacAddress(first.DstMac),
			Mask:               format.CIDR(first.DstAddr, first.DstMask),
			ReverseDNSHostname: first.DstReverseDNSHostname,
		},
		Host: hostname,
	}
	for _, r := range reporters {
		p.Reporters = append(p.Reporters, payload.ReporterPayload{
			FlowType:     string(r.FlowType),
			SamplingRate: r.SamplingRate,
			Direction:    format.Direction(r.Direction),
			Start:        r.StartTimestamp,
			End:          r.EndTimestamp,
			Bytes:        r.Bytes,
			Packets:      r.Packets,
			Device:       payload.Device{Namespace: r.Namespace},
			Exporter:     payload.Exporter{IP: format.IPAddr(r.ExporterAddr)},
			Ingress:      payload.ObservationPoint{Interface: payload.Interface{Index: r.InputInterface}},
			Egress:       payload.ObservationPoint{Interface: payload.Interface{Index: r.OutputInterface}},
			TCPFlags:     format.TCPFlags(r.TCPFlags),
			NextHop:      payload.NextHop{IP: format.IPAddr(r.NextHop)},
			AdditionalFields: r.AdditionalFields,
		})
	}
	return p
}

func buildPayload(aggFlow *common.Flow, hostname string, flushTime time.Time) payload.FlowPayload {
	return payload.FlowPayload{
		// TODO: Implement Tos
		FlushTimestamp: flushTime.UnixMilli(),
		FlowType:       string(aggFlow.FlowType),
		SamplingRate:   aggFlow.SamplingRate,
		Direction:      format.Direction(aggFlow.Direction),
		Device: payload.Device{
			Namespace: aggFlow.Namespace,
		},
		Exporter: payload.Exporter{
			IP: format.IPAddr(aggFlow.ExporterAddr),
		},
		Start:      aggFlow.StartTimestamp,
		End:        aggFlow.EndTimestamp,
		Bytes:      aggFlow.Bytes,
		Packets:    aggFlow.Packets,
		EtherType:  format.EtherType(aggFlow.EtherType),
		IPProtocol: format.IPProtocol(aggFlow.IPProtocol),
		Source: payload.Endpoint{
			IP:                 format.IPAddr(aggFlow.SrcAddr),
			Port:               format.Port(aggFlow.SrcPort),
			Mac:                format.MacAddress(aggFlow.SrcMac),
			Mask:               format.CIDR(aggFlow.SrcAddr, aggFlow.SrcMask),
			ReverseDNSHostname: aggFlow.SrcReverseDNSHostname,
		},
		Destination: payload.Endpoint{
			IP:                 format.IPAddr(aggFlow.DstAddr),
			Port:               format.Port(aggFlow.DstPort),
			Mac:                format.MacAddress(aggFlow.DstMac),
			Mask:               format.CIDR(aggFlow.DstAddr, aggFlow.DstMask),
			ReverseDNSHostname: aggFlow.DstReverseDNSHostname,
		},
		Ingress: payload.ObservationPoint{
			Interface: payload.Interface{
				Index: aggFlow.InputInterface,
			},
		},
		Egress: payload.ObservationPoint{
			Interface: payload.Interface{
				Index: aggFlow.OutputInterface,
			},
		},
		Host:     hostname,
		TCPFlags: format.TCPFlags(aggFlow.TCPFlags),
		NextHop: payload.NextHop{
			IP: format.IPAddr(aggFlow.NextHop),
		},
		AdditionalFields: aggFlow.AdditionalFields,
	}
}
