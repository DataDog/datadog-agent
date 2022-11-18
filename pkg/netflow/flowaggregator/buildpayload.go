// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"github.com/DataDog/datadog-agent/pkg/netflow/common"
	"github.com/DataDog/datadog-agent/pkg/netflow/enrichment"
	"github.com/DataDog/datadog-agent/pkg/netflow/payload"
	"github.com/DataDog/datadog-agent/pkg/netflow/portrollup"
)

func buildPayload(aggFlow *common.Flow, hostname string) payload.FlowPayload {
	return payload.FlowPayload{
		// TODO: Implement Tos
		FlowType:     string(aggFlow.FlowType),
		SamplingRate: aggFlow.SamplingRate,
		Direction:    enrichment.RemapDirection(aggFlow.Direction),
		Device: payload.Device{
			IP:        common.IPBytesToString(aggFlow.DeviceAddr),
			Namespace: aggFlow.Namespace,
		},
		Start:      aggFlow.StartTimestamp,
		End:        aggFlow.EndTimestamp,
		Bytes:      aggFlow.Bytes,
		Packets:    aggFlow.Packets,
		EtherType:  enrichment.MapEtherType(aggFlow.EtherType),
		IPProtocol: enrichment.MapIPProtocol(aggFlow.IPProtocol),
		Source: payload.Endpoint{
			IP:   common.IPBytesToString(aggFlow.SrcAddr),
			Port: portrollup.PortToString(aggFlow.SrcPort),
			Mac:  enrichment.FormatMacAddress(aggFlow.SrcMac),
			Mask: enrichment.FormatMask(aggFlow.SrcAddr, aggFlow.SrcMask),
		},
		Destination: payload.Endpoint{
			IP:   common.IPBytesToString(aggFlow.DstAddr),
			Port: portrollup.PortToString(aggFlow.DstPort),
			Mac:  enrichment.FormatMacAddress(aggFlow.DstMac),
			Mask: enrichment.FormatMask(aggFlow.DstAddr, aggFlow.DstMask),
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
		TCPFlags: enrichment.FormatFCPFlags(aggFlow.TCPFlags),
		NextHop: payload.NextHop{
			IP: common.IPBytesToString(aggFlow.NextHop),
		},
	}
}
