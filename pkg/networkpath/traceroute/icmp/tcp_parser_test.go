// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package icmp

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/ipv4"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/testutils"
)

var (
	srcIP = net.ParseIP("1.2.3.4")
	dstIP = net.ParseIP("5.6.7.8")

	innerSrcIP = net.ParseIP("10.0.0.1")
	innerDstIP = net.ParseIP("192.168.1.1")
)

func TestTCPMatch(t *testing.T) {
	srcPort := uint16(12345)
	dstPort := uint16(443)
	seqNum := uint32(2549)
	packetID := uint16(333)
	mockHeader := testutils.CreateMockIPv4Header(srcIP, dstIP, 1)
	icmpLayer := testutils.CreateMockICMPLayer(layers.ICMPv4TypeTimeExceeded, layers.ICMPv4CodeTTLExceeded)
	innerIPv4Layer := testutils.CreateMockIPv4Layer(packetID, innerSrcIP, innerDstIP, layers.IPProtocolTCP)
	innerTCPLayer := testutils.CreateMockTCPLayer(srcPort, dstPort, seqNum, 12737, true, true, true)
	icmpBytes := testutils.CreateMockICMPWithTCPPacket(nil, icmpLayer, innerIPv4Layer, innerTCPLayer, true)

	tts := []struct {
		description string
		header      *ipv4.Header
		payload     []byte
		localIP     net.IP
		localPort   uint16
		remoteIP    net.IP
		remotePort  uint16
		seqNum      uint32
		packetID    uint16
		// expected
		expectedIP     net.IP
		expectedErrMsg string
	}{
		{
			description: "protocol not ICMP returns an error",
			header: &ipv4.Header{
				Protocol: 17, // UDP
			},
			expectedIP:     net.IP{},
			expectedErrMsg: "expected an ICMP packet",
		},
		{
			description:    "bad ICMP payload returns an error",
			header:         mockHeader,
			localIP:        srcIP,
			remoteIP:       dstIP,
			expectedIP:     net.IP{},
			expectedErrMsg: "ICMP parse error",
		},
		{
			description:    "non-matching ICMP payload returns mismatch error",
			header:         mockHeader,
			payload:        icmpBytes,
			localIP:        srcIP,
			localPort:      srcPort,
			remoteIP:       dstIP,
			remotePort:     9001,
			seqNum:         seqNum,
			packetID:       packetID,
			expectedIP:     net.IP{},
			expectedErrMsg: "ICMP packet doesn't match",
		},
		{
			description:    "matching ICMP payload returns destination IP",
			header:         mockHeader,
			payload:        icmpBytes,
			localIP:        innerSrcIP,
			localPort:      srcPort,
			remoteIP:       innerDstIP,
			remotePort:     dstPort,
			seqNum:         seqNum,
			packetID:       packetID,
			expectedIP:     srcIP,
			expectedErrMsg: "",
		},
		{
			description:    "different packet ID doesn't match",
			header:         mockHeader,
			payload:        icmpBytes,
			localIP:        innerSrcIP,
			localPort:      srcPort,
			remoteIP:       innerDstIP,
			remotePort:     dstPort,
			seqNum:         seqNum,
			packetID:       packetID + 1,
			expectedIP:     srcIP,
			expectedErrMsg: "ICMP packet doesn't match",
		},
	}

	for _, test := range tts {
		t.Run(test.description, func(t *testing.T) {
			icmpParser := NewICMPTCPParser()
			actualIP, err := icmpParser.Match(test.header, test.payload, test.localIP, test.localPort, test.remoteIP, test.remotePort, test.seqNum, test.packetID)
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

func TestTCPParse(t *testing.T) {
	ipv4Header := testutils.CreateMockIPv4Header(srcIP, dstIP, 1)
	icmpLayer := testutils.CreateMockICMPLayer(layers.ICMPv4TypeTimeExceeded, layers.ICMPv4CodeTTLExceeded)
	innerIPv4Layer := testutils.CreateMockIPv4Layer(333, innerSrcIP, innerDstIP, layers.IPProtocolTCP)
	innerTCPLayer := testutils.CreateMockTCPLayer(12345, 443, 28394, 12737, true, true, true)

	tt := []struct {
		description string
		inHeader    *ipv4.Header
		inPayload   []byte
		expected    *Response
		errMsg      string
	}{
		{
			description: "empty IPv4 layer should return an error",
			inHeader:    &ipv4.Header{},
			inPayload:   []byte{1},
			expected:    nil,
			errMsg:      "invalid IP header for ICMP packet",
		},
		{
			description: "nil ICMP layer should return an error",
			inHeader:    ipv4Header,
			inPayload:   nil,
			expected:    nil,
			errMsg:      "received empty ICMP packet",
		},
		{
			description: "empty ICMP layer should return an error",
			inHeader:    ipv4Header,
			inPayload:   []byte{},
			expected:    nil,
			errMsg:      "received empty ICMP packet",
		},
		{
			description: "missing inner layers should return an error",
			inHeader:    ipv4Header,
			inPayload:   testutils.CreateMockICMPWithTCPPacket(nil, icmpLayer, nil, nil, false),
			expected:    nil,
			errMsg:      "failed to decode inner ICMP payload",
		},
		{
			description: "ICMP packet with partial TCP header should create icmpResponse",
			inHeader:    ipv4Header,
			inPayload:   testutils.CreateMockICMPWithTCPPacket(nil, icmpLayer, innerIPv4Layer, innerTCPLayer, true),
			expected: &Response{
				SrcIP:           srcIP,
				DstIP:           dstIP,
				InnerSrcIP:      innerSrcIP,
				InnerDstIP:      innerDstIP,
				InnerSrcPort:    12345,
				InnerDstPort:    443,
				InnerIdentifier: 28394,
			},
			errMsg: "",
		},
		{
			description: "full ICMP packet should create icmpResponse",
			inHeader:    ipv4Header,
			inPayload:   testutils.CreateMockICMPWithTCPPacket(nil, icmpLayer, innerIPv4Layer, innerTCPLayer, true),
			expected: &Response{
				SrcIP:           srcIP,
				DstIP:           dstIP,
				InnerSrcIP:      innerSrcIP,
				InnerDstIP:      innerDstIP,
				InnerSrcPort:    12345,
				InnerDstPort:    443,
				InnerIdentifier: 28394,
			},
			errMsg: "",
		},
	}

	for _, test := range tt {
		t.Run(test.description, func(t *testing.T) {
			icmpParser := NewICMPTCPParser()
			actual, err := icmpParser.Parse(test.inHeader, test.inPayload)
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
			assert.Truef(t, test.expected.InnerSrcIP.Equal(actual.InnerSrcIP), "mismatch inner source IPs: expected %s, got %s", test.expected.InnerSrcIP.String(), actual.InnerSrcIP.String())
			assert.Truef(t, test.expected.InnerDstIP.Equal(actual.InnerDstIP), "mismatch inner dest IPs: expected %s, got %s", test.expected.InnerDstIP.String(), actual.InnerDstIP.String())
			assert.Equal(t, test.expected.InnerSrcPort, actual.InnerSrcPort)
			assert.Equal(t, test.expected.InnerDstPort, actual.InnerDstPort)
			assert.Equal(t, test.expected.InnerIdentifier, actual.InnerIdentifier)
		})
	}
}
