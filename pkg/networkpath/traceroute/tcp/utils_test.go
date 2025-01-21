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

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/testutils"
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

func Test_parseTCP(t *testing.T) {
	ipv4Header := testutils.CreateMockIPv4Header(srcIP, dstIP, 6) // 6 is TCP
	tcpLayer := testutils.CreateMockTCPLayer(12345, 443, 28394, 12737, true, true, true)

	// full packet
	encodedTCPLayer, fullTCPPacket := testutils.CreateMockTCPPacket(ipv4Header, tcpLayer, false)

	tt := []struct {
		description string
		inHeader    *ipv4.Header
		inPayload   []byte
		expected    *tcpResponse
		errMsg      string
	}{
		{
			description: "empty IPv4 layer should return an error",
			inHeader:    &ipv4.Header{},
			inPayload:   []byte{},
			expected:    nil,
			errMsg:      "invalid IP header for TCP packet",
		},
		{
			description: "nil TCP layer should return an error",
			inHeader:    ipv4Header,
			expected:    nil,
			errMsg:      "received empty TCP payload",
		},
		{
			description: "missing TCP layer should return an error",
			inHeader:    ipv4Header,
			inPayload:   []byte{},
			expected:    nil,
			errMsg:      "received empty TCP payload",
		},
		{
			description: "full TCP packet should create tcpResponse",
			inHeader:    ipv4Header,
			inPayload:   fullTCPPacket,
			expected: &tcpResponse{
				SrcIP:       srcIP,
				DstIP:       dstIP,
				TCPResponse: *encodedTCPLayer,
			},
			errMsg: "",
		},
	}

	tp := newTCPParser()
	for _, test := range tt {
		t.Run(test.description, func(t *testing.T) {
			actual, err := tp.parseTCP(test.inHeader, test.inPayload)
			if test.errMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.errMsg)
				assert.Nil(t, actual)
				return
			}
			require.Nil(t, err)
			require.NotNil(t, actual)
			// assert.Equal doesn't handle net.IP well
			assert.Equal(t, testutils.StructFieldCount(test.expected), testutils.StructFieldCount(actual))
			assert.Truef(t, test.expected.SrcIP.Equal(actual.SrcIP), "mismatch source IPs: expected %s, got %s", test.expected.SrcIP.String(), actual.SrcIP.String())
			assert.Truef(t, test.expected.DstIP.Equal(actual.DstIP), "mismatch dest IPs: expected %s, got %s", test.expected.DstIP.String(), actual.DstIP.String())
			assert.Equal(t, test.expected.TCPResponse, actual.TCPResponse)
		})
	}
}

func BenchmarkParseTCP(b *testing.B) {
	ipv4Header := testutils.CreateMockIPv4Header(srcIP, dstIP, 6) // 6 is TCP
	tcpLayer := testutils.CreateMockTCPLayer(12345, 443, 28394, 12737, true, true, true)

	// full packet
	_, fullTCPPacket := testutils.CreateMockTCPPacket(ipv4Header, tcpLayer, false)

	tp := newTCPParser()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := tp.parseTCP(ipv4Header, fullTCPPacket)
		if err != nil {
			b.Fatal(err)
		}
	}
}
