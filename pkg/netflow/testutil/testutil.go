// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package testutil

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	"github.com/stretchr/testify/assert"
	"net"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"

	"github.com/DataDog/datadog-agent/pkg/netflow/payload"
)

//go:embed pcap_recordings/netflow5.pcapng
var netflow5pcapng []byte

//go:embed pcap_recordings/netflow9.pcapng
var netflow9pcapng []byte

//go:embed pcap_recordings/sflow.pcapng
var sflowpcapng []byte

func SendUDPPacket(port uint16, data []byte) error {
	udpConn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	_, err = udpConn.Write(data)
	udpConn.Close()
	return err
}

func ExpectNetflow5Payloads(t *testing.T, mockEpForwrader *epforwarder.MockEventPlatformForwarder) {
	events := [][]byte{
		[]byte(`
{
    "flush_timestamp": 1550505606000,
    "type": "netflow5",
    "sampling_rate": 0,
    "direction": "ingress",
    "start": 1683712725,
    "end": 1683712725,
    "bytes": 10,
    "packets": 1,
    "ether_type": "IPv4",
    "ip_protocol": "TCP",
    "device": {
        "namespace": "default"
    },
    "exporter": {
        "ip": "127.0.0.1"
    },
    "source": {
        "ip": "10.154.20.12",
        "port": "22",
        "mac": "00:00:00:00:00:00",
        "mask": "0.0.0.0/0"
    },
    "destination": {
        "ip": "0.0.0.92",
        "port": "81",
        "mac": "00:00:00:00:00:00",
        "mask": "0.0.0.0/0"
    },
    "ingress": {
        "interface": {
            "index": 0
        }
    },
    "egress": {
        "interface": {
            "index": 0
        }
    },
    "host": "my-hostname",
    "next_hop": {
        "ip": "0.0.0.0"
    }
}
`),
		[]byte(`
{
    "flush_timestamp": 1550505606000,
    "type": "netflow5",
    "sampling_rate": 0,
    "direction": "ingress",
    "start": 1683712725,
    "end": 1683712725,
    "bytes": 10,
    "packets": 1,
    "ether_type": "IPv4",
    "ip_protocol": "TCP",
    "device": {
        "namespace": "default"
    },
    "exporter": {
        "ip": "127.0.0.1"
    },
    "source": {
        "ip": "10.154.20.12",
        "port": "22",
        "mac": "00:00:00:00:00:00",
        "mask": "0.0.0.0/0"
    },
    "destination": {
        "ip": "0.0.0.93",
        "port": "81",
        "mac": "00:00:00:00:00:00",
        "mask": "0.0.0.0/0"
    },
    "ingress": {
        "interface": {
            "index": 0
        }
    },
    "egress": {
        "interface": {
            "index": 0
        }
    },
    "host": "my-hostname",
    "next_hop": {
        "ip": "0.0.0.0"
    }
}
`),
	}
	for _, event := range events {
		compactEvent := new(bytes.Buffer)
		err := json.Compact(compactEvent, event)
		assert.NoError(t, err)

		var p payload.FlowPayload
		err = json.Unmarshal(event, &p)
		assert.NoError(t, err)
		payloadBytes, _ := json.Marshal(p)
		m := &message.Message{Content: payloadBytes}

		mockEpForwrader.EXPECT().SendEventPlatformEventBlocking(m, epforwarder.EventTypeNetworkDevicesNetFlow).Return(nil)
	}
}

func GetPacketFromPcap(pcapdata []byte, layer gopacket.Decoder, packetIndex int) ([]byte, error) {
	reader := bytes.NewReader(pcapdata)

	r, err := pcapgo.NewNgReader(reader, pcapgo.DefaultNgReaderOptions)
	if err != nil {
		return nil, err
	}

	packetCount := 0
	for {
		data, _, err := r.ReadPacketData()
		if err != nil {
			return nil, err
		}
		if packetCount == packetIndex {
			packet := gopacket.NewPacket(data, layer, gopacket.Default)
			app := packet.ApplicationLayer()
			content := app.Payload()
			return content, nil
		}
		packetCount += 1
	}
}

func GetNetFlow5Packet() ([]byte, error) {
	return GetPacketFromPcap(netflow5pcapng, layers.LayerTypeLoopback, 0)
}

func GetNetFlow9Packet() ([]byte, error) {
	return GetPacketFromPcap(netflow9pcapng, layers.LayerTypeLoopback, 0)
}

func GetSFlow5Packet() ([]byte, error) {
	return GetPacketFromPcap(sflowpcapng, layers.LayerTypeEthernet, 1)
}
