// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package filter

import (
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

type mockPacketCapture struct {
	data []byte
	ci   gopacket.CaptureInfo
	err  error
}

type mockPacketReader struct {
	packets <-chan mockPacketCapture
}

func (m *mockPacketReader) ZeroCopyReadPacketData() (data []byte, ci gopacket.CaptureInfo, err error) {
	capture, ok := <-m.packets
	if !ok {
		return nil, gopacket.CaptureInfo{}, afpacket.ErrCancelled
	}
	return capture.data, capture.ci, capture.err
}
func (m *mockPacketReader) GetPacketInfoBuffer() *AFPacketInfo {
	return &AFPacketInfo{}
}

func mockCaptureInfo(ancillaryData []interface{}) gopacket.CaptureInfo {
	return gopacket.CaptureInfo{
		Timestamp:      time.Now(),
		CaptureLength:  0,
		Length:         0,
		InterfaceIndex: 0,
		AncillaryData:  ancillaryData,
	}
}

func expectAncillaryPktType(t *testing.T, ancillaryData []interface{}, pktType uint8) {
	packets := make(chan mockPacketCapture, 1)
	packets <- mockPacketCapture{
		data: []byte{},
		ci:   mockCaptureInfo(ancillaryData),
		err:  nil,
	}
	close(packets)

	p := mockPacketReader{
		packets: packets,
	}

	visited := false

	err := visitPackets(&p, func(_ []byte, info PacketInfo, _ time.Time) error {
		// make sure the callback ran since it's responsible for the require call
		visited = true

		// convert to linux packet info
		pktInfo := info.(*AFPacketInfo)
		// use assert so that we close the exit channel on failure
		assert.Equal(t, pktType, pktInfo.PktType)

		return nil
	})
	require.NoError(t, err)
	require.True(t, visited)
}

func TestVisitingRegularPacketOutgoing(t *testing.T) {
	expectAncillaryPktType(t, []interface{}{
		afpacket.AncillaryPktType{
			Type: unix.PACKET_OUTGOING,
		},
	}, unix.PACKET_OUTGOING)
}

func TestVisitingVLANPacketOutgoing(t *testing.T) {
	expectAncillaryPktType(t, []interface{}{
		afpacket.AncillaryVLAN{
			VLAN: 0,
		},
		afpacket.AncillaryPktType{
			Type: unix.PACKET_OUTGOING,
		},
	}, unix.PACKET_OUTGOING)
}

func TestVisitingRegularPacketIncoming(t *testing.T) {
	expectAncillaryPktType(t, []interface{}{
		afpacket.AncillaryPktType{
			Type: unix.PACKET_HOST,
		},
	}, unix.PACKET_HOST)
}

func TestVisitingVLANPacketIncoming(t *testing.T) {
	expectAncillaryPktType(t, []interface{}{
		afpacket.AncillaryVLAN{
			VLAN: 0,
		},
		afpacket.AncillaryPktType{
			Type: unix.PACKET_HOST,
		},
	}, unix.PACKET_HOST)
}
