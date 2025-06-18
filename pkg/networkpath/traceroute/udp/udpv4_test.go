// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package udp

import (
	"net"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/ipv4"
)

func TestCreateRawUDPBuffer(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("TestCreateRawTCPSyn is broken on macOS")
	}

	srcIP := net.ParseIP("1.2.3.4")
	dstIP := net.ParseIP("5.6.7.8")
	srcPort := uint16(12345)
	dstPort := uint16(33434)
	ttl := 4

	udp := NewUDPv4(dstIP, dstPort, 1, 1, 1, 0, 0)
	udp.srcIP = srcIP
	udp.srcPort = srcPort

	expectedIPHeader := &ipv4.Header{
		Version:  4,
		TTL:      ttl, // this will be replaced for each test
		ID:       41821,
		Protocol: 17,
		Dst:      dstIP,
		Src:      srcIP,
		Len:      20,
		TotalLen: 36,
		Checksum: 50008,
		Flags:    2, // Don't fragment flag set
	}

	// most of this is just copied from the output of the function
	// we don't need to test gopacket's ability to serialize a packet
	// we need to ensure that the logic that calculates the payload is correct
	// which means we have to check the last 8 bytes of the packet, really just
	// the last two
	expectedPktBytes := []byte{
		0x45, 0x0, 0x0, 0x24, 0xa3, 0x5d, 0x40, 0x0, 0x4, 0x11, 0xc3, 0x58, 0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x30, 0x39, 0x82, 0x9a, 0x0, 0x10, 0xdb, 0xa6, 0x4e, 0x53, 0x4d, 0x4e, 0x43, 0x0, 0x82, 0x9e,
	}

	// based on bytes 26-27 of expectedPktBytes
	expectedChecksum := uint16(0xdba6)

	ipHeaderID, pktBytes, actualChecksum, err := udp.createRawUDPBuffer(udp.srcIP, udp.srcPort, udp.Target, udp.TargetPort, ttl)

	require.NoError(t, err)
	assert.Equal(t, uint16(expectedIPHeader.ID), ipHeaderID)
	assert.Equal(t, expectedPktBytes, pktBytes)
	assert.Equal(t, expectedChecksum, actualChecksum)
}
