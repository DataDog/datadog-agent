// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package filter

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"testing"
	"time"
)

type mockPacketReader struct {
	data []byte
	ci   gopacket.CaptureInfo
	err  error
}

func (m *mockPacketReader) ZeroCopyReadPacketData() (data []byte, ci gopacket.CaptureInfo, err error) {
	return m.data, m.ci, m.err
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
	exit := make(chan struct{}, 1)
	exit <- struct{}{}
	p := mockPacketReader{
		data: []byte{},
		ci:   mockCaptureInfo(ancillaryData),
		err:  nil,
	}

	err := visitPackets(&p, exit, func(_ []byte, info PacketInfo, _ time.Time) error {
		// convert to linux packet info
		pktInfo := info.(*AFPacketInfo)
		require.Equal(t, pktType, pktInfo.PktType)
		// trigger exit so it only reads one packet
		exit <- struct{}{}
		return nil
	})
	require.NoError(t, err)
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
