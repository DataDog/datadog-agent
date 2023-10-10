// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/payload"
)

func Test_buildPayload(t *testing.T) {
	curTime := time.Now()
	tests := []struct {
		name            string
		flow            common.Flow
		expectedPayload payload.FlowPayload
	}{
		{
			name: "base case",
			flow: common.Flow{
				Namespace:       "my-namespace",
				FlowType:        common.TypeNetFlow9,
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
				EtherType:       uint32(0x0800),
				IPProtocol:      uint32(6),
				SrcPort:         2000,
				DstPort:         80,
				InputInterface:  10,
				OutputInterface: 20,
				Tos:             3,
				NextHop:         []byte{10, 10, 10, 30},
				TCPFlags:        uint32(19), // 19 = SYN,ACK,FIN
			},
			expectedPayload: payload.FlowPayload{
				FlushTimestamp: curTime.UnixMilli(),
				FlowType:       "netflow9",
				SamplingRate:   10,
				Direction:      "egress",
				Start:          1234568,
				End:            1234569,
				Bytes:          10,
				Packets:        2,
				EtherType:      "IPv4",
				IPProtocol:     "TCP",
				Device: payload.Device{
					Namespace: "my-namespace",
				},
				Exporter: payload.Exporter{
					IP: "127.0.0.1",
				},
				Source: payload.Endpoint{
					IP:   "10.10.10.10",
					Port: "2000",
					Mac:  "00:00:00:00:00:0a",
					Mask: "10.0.0.0/10",
				},
				Destination: payload.Endpoint{IP: "10.10.10.20",
					Port: "80",
					Mac:  "00:00:00:00:00:14",
					Mask: "10.10.0.0/20",
				},
				Ingress:  payload.ObservationPoint{Interface: payload.Interface{Index: 10}},
				Egress:   payload.ObservationPoint{Interface: payload.Interface{Index: 20}},
				Host:     "my-hostname",
				TCPFlags: []string{"FIN", "SYN", "ACK"},
				NextHop: payload.NextHop{
					IP: "10.10.10.30",
				},
			},
		},
		{
			name: "ephemeral source port",
			flow: common.Flow{
				Namespace:       "my-namespace",
				FlowType:        common.TypeNetFlow9,
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
				EtherType:       uint32(0x0800),
				IPProtocol:      uint32(6),
				SrcPort:         -1,
				DstPort:         80,
				InputInterface:  10,
				OutputInterface: 20,
				Tos:             3,
				NextHop:         []byte{10, 10, 10, 30},
				TCPFlags:        uint32(19), // 19 = SYN,ACK,FIN
			},
			expectedPayload: payload.FlowPayload{
				FlushTimestamp: curTime.UnixMilli(),
				FlowType:       "netflow9",
				SamplingRate:   10,
				Direction:      "egress",
				Start:          1234568,
				End:            1234569,
				Bytes:          10,
				Packets:        2,
				EtherType:      "IPv4",
				IPProtocol:     "TCP",
				Device: payload.Device{
					Namespace: "my-namespace",
				},
				Exporter: payload.Exporter{
					IP: "127.0.0.1",
				},
				Source: payload.Endpoint{
					IP:   "10.10.10.10",
					Port: "*",
					Mac:  "00:00:00:00:00:0a",
					Mask: "10.0.0.0/10",
				},
				Destination: payload.Endpoint{IP: "10.10.10.20",
					Port: "80",
					Mac:  "00:00:00:00:00:14",
					Mask: "10.10.0.0/20",
				},
				Ingress:  payload.ObservationPoint{Interface: payload.Interface{Index: 10}},
				Egress:   payload.ObservationPoint{Interface: payload.Interface{Index: 20}},
				Host:     "my-hostname",
				TCPFlags: []string{"FIN", "SYN", "ACK"},
				NextHop: payload.NextHop{
					IP: "10.10.10.30",
				},
			},
		},
		{
			name: "ephemeral source port",
			flow: common.Flow{
				Namespace:       "my-namespace",
				FlowType:        common.TypeNetFlow9,
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
				EtherType:       uint32(0x0800),
				IPProtocol:      uint32(6),
				SrcPort:         2000,
				DstPort:         -1,
				InputInterface:  10,
				OutputInterface: 20,
				Tos:             3,
				NextHop:         []byte{10, 10, 10, 30},
				TCPFlags:        uint32(19), // 19 = SYN,ACK,FIN
			},
			expectedPayload: payload.FlowPayload{
				FlushTimestamp: curTime.UnixMilli(),
				FlowType:       "netflow9",
				SamplingRate:   10,
				Direction:      "egress",
				Start:          1234568,
				End:            1234569,
				Bytes:          10,
				Packets:        2,
				EtherType:      "IPv4",
				IPProtocol:     "TCP",
				Device: payload.Device{
					Namespace: "my-namespace",
				},
				Exporter: payload.Exporter{
					IP: "127.0.0.1",
				},
				Source: payload.Endpoint{
					IP:   "10.10.10.10",
					Port: "2000",
					Mac:  "00:00:00:00:00:0a",
					Mask: "10.0.0.0/10",
				},
				Destination: payload.Endpoint{IP: "10.10.10.20",
					Port: "*",
					Mac:  "00:00:00:00:00:14",
					Mask: "10.10.0.0/20",
				},
				Ingress:  payload.ObservationPoint{Interface: payload.Interface{Index: 10}},
				Egress:   payload.ObservationPoint{Interface: payload.Interface{Index: 20}},
				Host:     "my-hostname",
				TCPFlags: []string{"FIN", "SYN", "ACK"},
				NextHop: payload.NextHop{
					IP: "10.10.10.30",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flowPayload := buildPayload(&tt.flow, "my-hostname", curTime)
			assert.Equal(t, tt.expectedPayload, flowPayload)
		})
	}
}
