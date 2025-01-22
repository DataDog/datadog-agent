// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package tcp

import (
	"fmt"
	"net"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/ipv4"
)

var (
	srcIP = net.ParseIP("1.2.3.4")
	dstIP = net.ParseIP("5.6.7.8")
)

func Test_reserveLocalPort(t *testing.T) {
	// WHEN we reserve a local port
	port, listener, err := reserveLocalPort()
	require.NoError(t, err)
	defer listener.Close()
	require.NotNil(t, listener)

	// THEN we should not be able to get another connection
	// on the same port
	conn2, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	assert.Error(t, err)
	assert.Nil(t, conn2)
}

func Test_createRawTCPSyn(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Test_createRawTCPSyn is broken on macOS")
	}

	srcIP := net.ParseIP("1.2.3.4")
	dstIP := net.ParseIP("5.6.7.8")
	srcPort := uint16(12345)
	dstPort := uint16(80)
	seqNum := uint32(1000)
	ttl := 64

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

	ipHeader, pktBytes, err := createRawTCPSyn(srcIP, srcPort, dstIP, dstPort, seqNum, ttl)
	require.NoError(t, err)
	assert.Equal(t, expectedIPHeader, ipHeader)
	assert.Equal(t, expectedPktBytes, pktBytes)
}

func Test_createRawTCPSynBuffer(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Test_createRawTCPSyn is broken on macOS")
	}

	srcIP := net.ParseIP("1.2.3.4")
	dstIP := net.ParseIP("5.6.7.8")
	srcPort := uint16(12345)
	dstPort := uint16(80)
	seqNum := uint32(1000)
	ttl := 64

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
		0x45, 0x0, 0x0, 0x28, 0xa3, 0x5d, 0x0, 0x0, 0x40, 0x6, 0xc7, 0x5f, 0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8, 0x30, 0x39, 0x0, 0x50, 0x0, 0x0, 0x3, 0xe8, 0x0, 0x0, 0x0, 0x0, 0x50, 0x2, 0x4, 0x0, 0x67, 0x5e, 0x0, 0x0,
	}

	ipHeader, pktBytes, headerLength, err := createRawTCPSynBuffer(srcIP, srcPort, dstIP, dstPort, seqNum, ttl)

	require.NoError(t, err)
	assert.Equal(t, expectedIPHeader, ipHeader)
	assert.Equal(t, 20, headerLength)
	assert.Equal(t, expectedPktBytes, pktBytes)
}
