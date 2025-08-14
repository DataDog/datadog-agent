// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package testutil provides some utilities for integration testing portions of
// the netflow tooling.
package testutil

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder"
)

//go:embed pcap_recordings/netflow5.pcapng
var netflow5pcapng []byte

//go:embed pcap_recordings/netflow9.pcapng
var netflow9pcapng []byte

//go:embed pcap_recordings/sflow.pcapng
var sflowpcapng []byte

// SendUDPPacket sends data to a local port over UDP.
func SendUDPPacket(port uint16, data []byte) error {
	udpConn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	_, err = udpConn.Write(data)
	udpConn.Close()
	return err
}

// SetupEventCapture sets up mock expectations to capture events sent to the event platform forwarder
func SetupEventCapture(epForwarder *eventplatformimpl.MockEventPlatformForwarder, netflowEventCount, metadataEventCount int) (*[][]byte, *[][]byte) {
	var capturedNetflowEvents [][]byte
	var capturedMetadataEvents [][]byte

	if netflowEventCount > 0 {
		epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-netflow").DoAndReturn( // JMW
			func(msg *message.Message, _ string) error {
				capturedNetflowEvents = append(capturedNetflowEvents, msg.GetContent())
				return nil
			}).Times(netflowEventCount)
	}

	if metadataEventCount > 0 {
		epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").DoAndReturn(
			func(msg *message.Message, _ string) error {
				capturedMetadataEvents = append(capturedMetadataEvents, msg.GetContent())
				return nil
			}).Times(metadataEventCount)
	}

	return &capturedNetflowEvents, &capturedMetadataEvents
}

// ValidateAndRemoveTimestamps validates that timestamp fields are within reasonable bounds
// and removes them from JSON for comparison. Both flush_timestamp and collect_timestamp
// are removed if they exist.
func ValidateAndRemoveTimestamps(t *testing.T, jsonBytes []byte, testStartTime time.Time) ([]byte, error) {
	var data map[string]any
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return nil, err
	}

	testEndTime := time.Now()

	// Validate and remove flush_timestamp if it exists (in milliseconds)
	if flushTimestamp, ok := data["flush_timestamp"].(float64); ok {
		flushTime := time.UnixMilli(int64(flushTimestamp))
		assert.True(t, flushTime.After(testStartTime) || flushTime.Equal(testStartTime),
			"flush_timestamp %v should be >= test start time %v", flushTime, testStartTime)
		assert.True(t, flushTime.Before(testEndTime) || flushTime.Equal(testEndTime),
			"flush_timestamp %v should be <= test end time %v", flushTime, testEndTime)
		delete(data, "flush_timestamp")
	}

	// Validate and remove collect_timestamp if it exists (in seconds)
	if collectTimestamp, ok := data["collect_timestamp"].(float64); ok {
		collectTime := time.Unix(int64(collectTimestamp), 0)
		assert.True(t, collectTime.After(testStartTime) || collectTime.Equal(testStartTime),
			"collect_timestamp %v should be >= test start time %v", collectTime, testStartTime)
		assert.True(t, collectTime.Before(testEndTime) || collectTime.Equal(testEndTime),
			"collect_timestamp %v should be <= test end time %v", collectTime, testEndTime)
		delete(data, "collect_timestamp")
	}

	return json.Marshal(data)
}

// ExpectNetflow5Payloads expects the payloads that should result from our
// recorded pcap files, using dynamic timestamp validation.
func ExpectNetflow5Payloads(t *testing.T, mockEpForwarder forwarder.MockComponent, testStartTime time.Time) {
	expectedEvents := [][]byte{
		[]byte(`
{
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
        "mask": "0.0.0.0/0",
        "reverse_dns_hostname": "hostname-10.154.20.12"
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
        "mask": "0.0.0.0/0",
        "reverse_dns_hostname": "hostname-10.154.20.12"
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

	// JMW use setupEventCapture to capture the events?
	// Capture events using DoAndReturn
	var capturedEvents [][]byte
	for range expectedEvents {
		mockEpForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), eventplatform.EventTypeNetworkDevicesNetFlow).DoAndReturn( // JMW
			func(msg *message.Message, _ string) error {
				capturedEvents = append(capturedEvents, msg.GetContent())
				return nil
			})
	}

	// Validate captured events after they're collected
	t.Cleanup(func() {
		assert.Len(t, capturedEvents, len(expectedEvents), "Expected exactly %d netflow events", len(expectedEvents))

		// Compact expected events for comparison
		expectedEventStrings := make([]string, len(expectedEvents))
		for i, expectedEvent := range expectedEvents {
			compactExpectedEvent := new(bytes.Buffer)
			err := json.Compact(compactExpectedEvent, expectedEvent)
			assert.NoError(t, err)
			expectedEventStrings[i] = compactExpectedEvent.String()
		}

		// Validate and remove timestamps from captured events
		capturedEventStrings := make([]string, len(capturedEvents))
		for i, capturedEvent := range capturedEvents {
			capturedEventWithoutTimestamp, err := ValidateAndRemoveTimestamps(t, capturedEvent, testStartTime)
			assert.NoError(t, err)
			capturedEventStrings[i] = string(capturedEventWithoutTimestamp)
		}

		// Verify that each expected event has a corresponding captured event (order-independent)
		for _, expectedEventStr := range expectedEventStrings {
			found := false
			for _, capturedEventStr := range capturedEventStrings {
				// Use a temporary test to check JSON equality without failing the main test
				tempT := &testing.T{}
				if assert.JSONEq(tempT, expectedEventStr, capturedEventStr) {
					found = true
					break
				}
			}
			assert.True(t, found, "Expected event not found in captured events: %s", expectedEventStr)
		}
	})
}

// ExpectPayloadWithAdditionalFields expects the payloads that should result from our
// recorded Netflow9 pcap file with inverted source and destination ports and icmp_type custom field.
func ExpectPayloadWithAdditionalFields(t *testing.T, mockEpForwarder forwarder.MockComponent, testStartTime time.Time) {
	expectedEvent := []byte(`
{
  "bytes": 114702,
  "destination": {
    "ip": "53.0.97.192",
    "port": "5915",
    "mac": "00:00:00:00:00:00",
    "mask": "0.0.0.0/0"
  },
  "device": {
    "namespace": "default"
  },
  "direction": "ingress",
  "egress": {
    "interface": {
      "index": 4352
    }
  },
  "end": 1675541179,
  "ether_type": "IPv4",
  "exporter": {
    "ip": "127.0.0.1"
  },
  "host": "my-hostname",
  "icmp_type": "1200",
  "ingress": {
    "interface": {
      "index": 27505
    }
  },
  "ip_protocol": "ICMP",
  "next_hop": {
    "ip": ""
  },
  "packets": 840155153,
  "sampling_rate": 0,
  "source": {
    "ip": "2.10.65.0",
    "port": "0",
    "mac": "00:00:00:00:00:00",
    "mask": "0.0.0.0/0"
  },
  "start": 1675307019,
  "type": "netflow9"
}
`)

	// JMWSAME
	// Capture all 29 events
	var capturedEvents [][]byte
	mockEpForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), eventplatform.EventTypeNetworkDevicesNetFlow).DoAndReturn(
		func(msg *message.Message, _ string) error {
			capturedEvents = append(capturedEvents, msg.GetContent())
			return nil
		}).Times(29)

	// Validate that at least one captured event matches the expected payload
	t.Cleanup(func() {
		assert.Len(t, capturedEvents, 29, "Expected exactly 29 netflow events")

		// Compact expected event for comparison
		compactExpectedEvent := new(bytes.Buffer)
		err := json.Compact(compactExpectedEvent, expectedEvent)
		assert.NoError(t, err)
		expectedEventStr := compactExpectedEvent.String()

		// Check if at least one captured event matches the expected payload
		found := false
		for _, capturedEvent := range capturedEvents {
			// Validate and remove timestamps from captured event
			capturedEventWithoutTimestamp, err := ValidateAndRemoveTimestamps(t, capturedEvent, testStartTime)
			if err != nil {
				continue // Skip events that fail timestamp validation
			}

			// Use a temporary test to check JSON equality without failing the main test
			tempT := &testing.T{}
			if assert.JSONEq(tempT, expectedEventStr, string(capturedEventWithoutTimestamp)) {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected event not found in any of the captured events: %s", expectedEventStr)
	})
}

// GetPacketFromPCAP parses PCAP data into an actual packet.
func GetPacketFromPCAP(pcapdata []byte, layer gopacket.Decoder, packetIndex int) ([]byte, error) {
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
		packetCount++
	}
}

// GetNetFlow5Packet parses our saved netflow5 packet.
func GetNetFlow5Packet() ([]byte, error) {
	return GetPacketFromPCAP(netflow5pcapng, layers.LayerTypeLoopback, 0)
}

// GetNetFlow9Packet parses our saved netflow9 packet.
func GetNetFlow9Packet() ([]byte, error) {
	return GetPacketFromPCAP(netflow9pcapng, layers.LayerTypeLoopback, 0)
}

// GetSFlow5Packet parses our saved sflow5 packet.
func GetSFlow5Packet() ([]byte, error) {
	return GetPacketFromPCAP(sflowpcapng, layers.LayerTypeEthernet, 1)
}
