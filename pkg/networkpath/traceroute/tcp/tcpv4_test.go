// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package tcp

import (
	"net"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/ipv4"
)

func TestCreateRawTCPSyn(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("TestCreateRawTCPSyn is broken on macOS")
	}

	srcIP := net.ParseIP("1.2.3.4")
	dstIP := net.ParseIP("5.6.7.8")
	srcPort := uint16(12345)
	dstPort := uint16(80)
	seqNum := uint32(1000)
	packetID := uint16(41821)
	ttl := 64

	tcp := NewTCPv4(dstIP, dstPort, 1, 1, 1, 0, 0, false)
	tcp.srcIP = srcIP
	tcp.srcPort = srcPort

	expectedIPHeader := &ipv4.Header{
		Version:  4,
		TTL:      ttl,
		ID:       41821,
		Protocol: 6,
		Dst:      dstIP,
		Src:      srcIP,
		Len:      20,
		TotalLen: 40,
		Checksum: 51039,
	}

	expectedPktBytes := []byte{
		0x30, 0x39, 0x0, 0x50, 0x0, 0x0, 0x3, 0xe8, 0x0, 0x0, 0x0, 0x0, 0x50, 0x2, 0x4, 0x0, 0x67, 0x5e, 0x0, 0x0,
	}

	ipHeader, pktBytes, err := tcp.createRawTCPSyn(packetID, seqNum, ttl)
	require.NoError(t, err)
	assert.Equal(t, expectedIPHeader, ipHeader)
	assert.Equal(t, expectedPktBytes, pktBytes)
}

func TestCreateRawTCPSynBuffer(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("TestCreateRawTCPSyn is broken on macOS")
	}

	srcIP := net.ParseIP("1.2.3.4")
	dstIP := net.ParseIP("5.6.7.8")
	srcPort := uint16(12345)
	dstPort := uint16(80)
	seqNum := uint32(1000)
	packetID := uint16(41821)
	ttl := 64

	tcp := NewTCPv4(dstIP, dstPort, 1, 1, 1, 0, 0, false)
	tcp.srcIP = srcIP
	tcp.srcPort = srcPort

	expectedIPHeader := &ipv4.Header{
		Version:  4,
		TTL:      ttl,
		ID:       int(packetID),
		Protocol: 6,
		Dst:      dstIP,
		Src:      srcIP,
		Len:      20,
		TotalLen: 40,
		Checksum: 51039,
	}

	expectedPktBytes := []byte{
		0x45, 0x0, 0x0, 0x28, 0xa3, 0x5d, 0x0, 0x0, 0x40, 0x6, 0xc7, 0x5f, 0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x30, 0x39, 0x0, 0x50, 0x0, 0x0, 0x3, 0xe8, 0x0, 0x0, 0x0, 0x0, 0x50, 0x2, 0x4, 0x0, 0x67, 0x5e, 0x0, 0x0,
	}

	ipHeader, pktBytes, headerLength, err := tcp.createRawTCPSynBuffer(packetID, seqNum, ttl)

	require.NoError(t, err)
	assert.Equal(t, expectedIPHeader, ipHeader)
	assert.Equal(t, 20, headerLength)
	assert.Equal(t, expectedPktBytes, pktBytes)
}

func TestCreateRawTCPSynBufferPacketID(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("TestCreateRawTCPSyn is broken on macOS")
	}
	srcIP := net.ParseIP("1.2.3.4")
	dstIP := net.ParseIP("5.6.7.8")
	srcPort := uint16(12345)
	dstPort := uint16(80)
	seqNum := uint32(1000)
	packetID := uint16(54321)
	ttl := 64

	tcp := NewTCPv4(dstIP, dstPort, 1, 1, 1, 0, 0, false)
	tcp.srcIP = srcIP
	tcp.srcPort = srcPort

	ipHeader, _, _, err := tcp.createRawTCPSynBuffer(packetID, seqNum, ttl)

	require.NoError(t, err)

	// check that when we re-parse the packet ID, it has the ID we expect
	require.Equal(t, ipHeader.ID, int(packetID))
}
