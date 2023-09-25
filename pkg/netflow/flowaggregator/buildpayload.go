// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/netflow/common"
	"github.com/DataDog/datadog-agent/pkg/netflow/format"
	"github.com/DataDog/datadog-agent/pkg/netflow/payload"
)

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
			IP:   format.IPAddr(aggFlow.SrcAddr),
			Port: format.Port(aggFlow.SrcPort),
			Mac:  format.MacAddress(aggFlow.SrcMac),
			Mask: format.CIDR(aggFlow.SrcAddr, aggFlow.SrcMask),
		},
		Destination: payload.Endpoint{
			IP:   format.IPAddr(aggFlow.DstAddr),
			Port: format.Port(aggFlow.DstPort),
			Mac:  format.MacAddress(aggFlow.DstMac),
			Mask: format.CIDR(aggFlow.DstAddr, aggFlow.DstMask),
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
	}
}
