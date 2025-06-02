// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package tcp

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/ipv4"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/testutils"
)

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
				SrcIP:   srcIP,
				DstIP:   dstIP,
				SrcPort: uint16(encodedTCPLayer.SrcPort),
				DstPort: uint16(encodedTCPLayer.DstPort),
				SYN:     encodedTCPLayer.SYN,
				ACK:     encodedTCPLayer.ACK,
				RST:     encodedTCPLayer.RST,
				AckNum:  encodedTCPLayer.Ack,
			},
			errMsg: "",
		},
	}

	tp := newParser()
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
			assert.Equal(t, test.expected.SrcPort, actual.SrcPort)
			assert.Equal(t, test.expected.DstPort, actual.DstPort)
			assert.Equal(t, test.expected.SYN, actual.SYN)
			assert.Equal(t, test.expected.ACK, actual.ACK)
			assert.Equal(t, test.expected.RST, actual.RST)
			assert.Equal(t, test.expected.AckNum, actual.AckNum)
		})
	}
}

func Test_MatchTCP(t *testing.T) {
	srcPort := uint16(12345)
	dstPort := uint16(443)
	seqNum := uint32(2549)
	mockHeader := testutils.CreateMockIPv4Header(dstIP, srcIP, 6)
	_, synackBytes := testutils.CreateMockTCPPacket(mockHeader, testutils.CreateMockTCPLayer(dstPort, srcPort, 123, seqNum+1, true, true, false), false)
	_, rstackBytes := testutils.CreateMockTCPPacket(mockHeader, testutils.CreateMockTCPLayer(dstPort, srcPort, 456, seqNum+1, false, true, true), false)
	_, rstBytes := testutils.CreateMockTCPPacket(mockHeader, testutils.CreateMockTCPLayer(dstPort, srcPort, 789, 0, false, false, true), false)

	tts := []struct {
		description string
		header      *ipv4.Header
		payload     []byte
		localIP     net.IP
		localPort   uint16
		remoteIP    net.IP
		remotePort  uint16
		seqNum      uint32
		// expected
		expectedIP     net.IP
		expectedErrMsg string
	}{
		{
			description: "protocol not TCP returns an error",
			header: &ipv4.Header{
				Protocol: 17, // UDP
			},
			expectedIP:     net.IP{},
			expectedErrMsg: "expected a TCP packet",
		},
		{
			description:    "non-matching source IP returns mismatch error",
			header:         testutils.CreateMockIPv4Header(dstIP, net.ParseIP("2.2.2.2"), 6),
			localIP:        srcIP,
			remoteIP:       dstIP,
			expectedIP:     net.IP{},
			expectedErrMsg: "TCP packet doesn't match",
		},
		{
			description:    "non-matching destination IP returns mismatch error",
			header:         testutils.CreateMockIPv4Header(net.ParseIP("2.2.2.2"), srcIP, 6),
			localIP:        srcIP,
			remoteIP:       dstIP,
			expectedIP:     net.IP{},
			expectedErrMsg: "TCP packet doesn't match",
		},
		{
			description:    "bad TCP payload returns an error",
			header:         mockHeader,
			localIP:        srcIP,
			remoteIP:       dstIP,
			expectedIP:     net.IP{},
			expectedErrMsg: "TCP parse error",
		},
		{
			description:    "non-matching TCP payload returns mismatch error",
			header:         mockHeader,
			payload:        synackBytes,
			localIP:        srcIP,
			localPort:      srcPort,
			remoteIP:       dstIP,
			remotePort:     9001,
			seqNum:         seqNum,
			expectedIP:     net.IP{},
			expectedErrMsg: "TCP packet doesn't match",
		},
		{
			description:    "matching SYNACK payload returns destination IP",
			header:         mockHeader,
			payload:        synackBytes,
			localIP:        srcIP,
			localPort:      srcPort,
			remoteIP:       dstIP,
			remotePort:     dstPort,
			seqNum:         seqNum,
			expectedIP:     dstIP,
			expectedErrMsg: "",
		},
		{
			description:    "non-matching SYNACK ack number returns mismatch error",
			header:         mockHeader,
			payload:        synackBytes,
			localIP:        srcIP,
			localPort:      srcPort,
			remoteIP:       dstIP,
			remotePort:     dstPort,
			seqNum:         seqNum + 123,
			expectedIP:     net.IP{},
			expectedErrMsg: "TCP packet doesn't match",
		},
		{
			description:    "matching RSTACK payload returns destination IP",
			header:         mockHeader,
			payload:        rstackBytes,
			localIP:        srcIP,
			localPort:      srcPort,
			remoteIP:       dstIP,
			remotePort:     dstPort,
			seqNum:         seqNum,
			expectedIP:     dstIP,
			expectedErrMsg: "",
		},
		{
			description:    "non-matching RSTACK ack number returns mismatch error",
			header:         mockHeader,
			payload:        rstackBytes,
			localIP:        srcIP,
			localPort:      srcPort,
			remoteIP:       dstIP,
			remotePort:     dstPort,
			seqNum:         seqNum + 123,
			expectedIP:     net.IP{},
			expectedErrMsg: "TCP packet doesn't match",
		},
		{
			description:    "RST payload returns destination IP even though the packet has ack=0",
			header:         mockHeader,
			payload:        rstBytes,
			localIP:        srcIP,
			localPort:      srcPort,
			remoteIP:       dstIP,
			remotePort:     dstPort,
			seqNum:         seqNum,
			expectedIP:     dstIP,
			expectedErrMsg: "",
		},
	}

	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			tp := newParser()
			// packetID is not used for TCP matching
			packetID := uint16(2222)
			actualIP, err := tp.MatchTCP(test.header, test.payload, test.localIP, test.localPort, test.remoteIP, test.remotePort, test.seqNum, packetID)
			if test.expectedErrMsg != "" {
				require.Error(t, err)
				assert.True(t, strings.Contains(err.Error(), test.expectedErrMsg), fmt.Sprintf("expected %q, got %q", test.expectedErrMsg, err.Error()))
				return
			}
			require.NoError(t, err)
			assert.Truef(t, test.expectedIP.Equal(actualIP), "mismatch IPs: expected %s, got %s", test.expectedIP.String(), actualIP.String())
		})
	}
}

func BenchmarkParse(b *testing.B) {
	ipv4Header := testutils.CreateMockIPv4Header(srcIP, dstIP, 6) // 6 is TCP
	tcpLayer := testutils.CreateMockTCPLayer(12345, 443, 28394, 12737, true, true, true)

	// full packet
	_, fullTCPPacket := testutils.CreateMockTCPPacket(ipv4Header, tcpLayer, false)

	tp := newParser()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := tp.parseTCP(ipv4Header, fullTCPPacket)
		if err != nil {
			b.Fatal(err)
		}
	}
}
