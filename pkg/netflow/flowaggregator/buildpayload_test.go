package flowaggregator

import (
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/netflow/common"
	"github.com/DataDog/datadog-agent/pkg/netflow/payload"
	"github.com/stretchr/testify/assert"
	"net"
	"testing"
)

func Test_buildPayload(t *testing.T) {
	coreconfig.Datadog.Set("hostname", "my-hostname")
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
				ExporterAddr:    net.IP([]byte{127, 0, 0, 1}).String(),
				StartTimestamp:  1234568,
				EndTimestamp:    1234569,
				Bytes:           10,
				Packets:         2,
				SrcAddr:         net.IP([]byte{10, 10, 10, 10}).String(),
				DstAddr:         net.IP([]byte{10, 10, 10, 20}).String(),
				SrcMac:          uint64(10),
				DstMac:          uint64(20),
				SrcMask:         uint32(10),
				DstMask:         uint32(20),
				EtherType:       uint32(1),
				IPProtocol:      uint32(6),
				SrcPort:         uint32(2000),
				DstPort:         uint32(80),
				InputInterface:  10,
				OutputInterface: 20,
				Tos:             3,
				NextHop:         net.IP([]byte{10, 10, 10, 30}).String(),
			},
			expectedPayload: payload.FlowPayload{
				FlowType:     "netflow9",
				SamplingRate: 10,
				Direction:    "egress",
				Start:        1234568,
				End:          1234569,
				Bytes:        10,
				Packets:      2,
				EtherType:    "1",
				IPProtocol:   "6",
				Exporter: payload.Exporter{
					IP: "127.0.0.1",
				},
				Source: payload.Endpoint{
					IP:   "10.10.10.10",
					Port: 2000,
					Mac:  "00:00:00:00:00:00",
					Mask: "0.0.0.0/24",
				},
				Destination: payload.Endpoint{IP: "10.10.10.20",
					Port: 80,
					Mac:  "",
					Mask: "",
				},
				Ingress:   payload.ObservationPoint{Interface: payload.Interface{Index: 10}},
				Egress:    payload.ObservationPoint{Interface: payload.Interface{Index: 20}},
				Namespace: "my-namespace",
				Host:      "my-hostname",
				TCPFlags:  []string{"SYN", "ACK"},
				NextHop: payload.NextHop{
					IP: "10.10.10.30",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flowPayload := buildPayload(&tt.flow)
			assert.Equal(t, tt.expectedPayload, flowPayload)
		})
	}
}
