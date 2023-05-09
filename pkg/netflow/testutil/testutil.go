// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package testutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"

	"github.com/DataDog/datadog-agent/pkg/netflow/payload"
)

func SendUDPPacket(port uint16, data []byte) error {
	udpConn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	_, err = udpConn.Write(data)
	udpConn.Close()
	return err
}

func ExpectNetflow5Payloads(t *testing.T, mockEpForwrader *epforwarder.MockEventPlatformForwarder, now time.Time, host string, records int) {
	for i := 0; i < records; i++ {
		// language=json
		event := []byte(fmt.Sprintf(`
{
    "type": "netflow5",
    "sampling_rate": 0,
    "direction": "ingress",
    "start": %d,
    "end": %d,
    "bytes": 194,
    "packets": 10,
    "ether_type": "IPv4",
    "ip_protocol": "TCP",
    "device": {
        "namespace": "default"
    },
    "exporter": {
        "ip": "127.0.0.1"
    },
    "source": {
        "ip": "10.0.0.1",
        "port": "50000",
        "mac": "00:00:00:00:00:00",
        "mask": "0.0.0.0/0"
    },
    "destination": {
        "ip": "20.0.0.%d",
        "port": "8080",
        "mac": "00:00:00:00:00:00",
        "mask": "0.0.0.0/0"
    },
    "ingress": {
        "interface": {
            "index": 1
        }
    },
    "egress": {
        "interface": {
            "index": 7
        }
    },
    "host": "%s",
    "tcp_flags": [
        "SYN",
        "RST",
        "ACK"
    ],
    "next_hop": {
        "ip": "0.0.0.0"
    }
}
`, now.Unix(), now.Unix(), i, host))
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

func GetPacketFromPcap(file string, packetIndex int) ([]byte, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r, err := pcapgo.NewNgReader(f, pcapgo.DefaultNgReaderOptions)
	if err != nil {
		return nil, err
	}

	packetCount := 0
	for {
		data, captureInfo, err := r.ReadPacketData()
		fmt.Printf("%v\n", captureInfo)
		if err != nil {
			return nil, err
		}
		if packetCount == packetIndex {
			packet := gopacket.NewPacket(data, layers.LayerTypeUDP, gopacket.Default)
			//linkLayer := packet.LinkLayer()
			app := packet.ApplicationLayer()
			//content := app.LayerContents()
			content := app.LayerPayload()
			//application := app.LayerPayload()
			return content, nil
		}
		packetCount += 1
	}
}

//func GetPacketFromPcap(file string, packetIndex int) []byte {
//	// create reader
//	r, err := gopcap.Open(file)
//	if err != nil {
//		panic(err)
//	}
//	defer r.Close()
//
//	// loop over packets
//	packetCount := 0
//	for {
//		_, data, err := r.ReadNextPacket()
//		if err != nil {
//			if err == io.EOF {
//				println("EOF")
//				break
//			}
//			panic(err)
//		}
//		if packetCount == packetIndex {
//			return data
//		}
//	}
//	return nil
//}
