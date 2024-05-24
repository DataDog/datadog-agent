// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcp

import (
	"net"
	"reflect"
	"strings"
	"testing"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseIPv4Layer(t *testing.T) {
	srcIP := net.ParseIP("1.2.3.4")
	dstIP := net.ParseIP("5.6.7.8")
	ipv4Layer := createMockIPv4Layer(srcIP, dstIP, 1)
	buf := gopacket.NewSerializeBuffer()
	gopacket.SerializeLayers(buf, gopacket.SerializeOptions{},
		ipv4Layer,
	)
	ipv4Packet := gopacket.NewPacket(buf.Bytes(), layers.LayerTypeIPv4, gopacket.Default)

	tt := []struct {
		description string
		input       gopacket.Packet
		srcIP       net.IP
		dstIP       net.IP
		errMsg      string
	}{
		{
			description: "empty IPv4 layer should return an error",
			input:       gopacket.NewPacket([]byte{}, layers.LayerTypeTCP, gopacket.Default),
			srcIP:       net.IP{},
			dstIP:       net.IP{},
			errMsg:      "packet does not contain an IPv4 layer",
		},
		{
			description: "proper IPv4 layer parsing grabs source and dest IPs",
			input:       ipv4Packet,
			srcIP:       srcIP,
			dstIP:       dstIP,
			errMsg:      "",
		},
	}

	for _, test := range tt {
		t.Run(test.description, func(t *testing.T) {
			actualSrc, actualDst, err := parseIPv4Layer(test.input)
			if test.errMsg != "" {
				require.Error(t, err)
				assert.True(t, strings.Contains(err.Error(), test.errMsg))
				return
			}
			require.Nil(t, err)
			assert.True(t, test.srcIP.Equal(actualSrc))
			assert.True(t, test.dstIP.Equal(actualDst))
		})
	}
}

func TestParseICMPLayer(t *testing.T) {
	srcIP := net.ParseIP("1.2.3.4")
	dstIP := net.ParseIP("5.6.7.8")
	innerSrcIP := net.ParseIP("10.0.0.1")
	innerDstIP := net.ParseIP("192.168.1.1")
	ipv4Layer := createMockIPv4Layer(srcIP, dstIP, layers.IPProtocolICMPv4)
	icmpLayer := createMockICMPLayer(layers.ICMPv4CodeTTLExceeded)
	innerIPv4Layer := createMockIPv4Layer(innerSrcIP, innerDstIP, layers.IPProtocolTCP)
	innerTCPLayer := createMockTCPLayer(12345, 443, 28394, 12737, true, true, true)

	// create packet without an ICMP layer
	buf := gopacket.NewSerializeBuffer()
	gopacket.SerializeLayers(buf, gopacket.SerializeOptions{},
		ipv4Layer,
	)
	missingICMPLayer := gopacket.NewPacket(buf.Bytes(), layers.LayerTypeIPv4, gopacket.Default)

	tt := []struct {
		description string
		input       gopacket.Packet
		expected    *icmpResponse
		errMsg      string
	}{
		{
			description: "empty IPv4 layer should return an error",
			input:       gopacket.NewPacket([]byte{}, layers.LayerTypeTCP, gopacket.Default),
			expected:    nil,
			errMsg:      "packet does not contain an IPv4 layer",
		},
		{
			description: "missing ICMP layer should return an error",
			input:       missingICMPLayer,
			expected:    nil,
			errMsg:      "packet does not contain an ICMP layer",
		},
		{
			description: "missing inner layers should return an error",
			input:       createMockICMPPacket(t, ipv4Layer, icmpLayer, nil, nil, false),
			expected:    nil,
			errMsg:      "failed to decode ICMP payload",
		},
		{
			description: "ICMP packet with partial TCP header should create icmpResponse",
			input:       createMockICMPPacket(t, ipv4Layer, icmpLayer, innerIPv4Layer, innerTCPLayer, true),
			expected: &icmpResponse{
				SrcIP:        srcIP,
				DstIP:        dstIP,
				InnerSrcIP:   innerSrcIP,
				InnerDstIP:   innerDstIP,
				InnerSrcPort: 12345,
				InnerDstPort: 443,
				InnerSeqNum:  28394,
			},
			errMsg: "",
		},
		{
			description: "full ICMP packet should create icmpResponse",
			input:       createMockICMPPacket(t, ipv4Layer, icmpLayer, innerIPv4Layer, innerTCPLayer, false),
			expected: &icmpResponse{
				SrcIP:        srcIP,
				DstIP:        dstIP,
				InnerSrcIP:   innerSrcIP,
				InnerDstIP:   innerDstIP,
				InnerSrcPort: 12345,
				InnerDstPort: 443,
				InnerSeqNum:  28394,
			},
			errMsg: "",
		},
	}

	for _, test := range tt {
		t.Run(test.description, func(t *testing.T) {
			actual, err := parseICMPPacket(test.input)
			if test.errMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.errMsg)
				assert.Nil(t, actual)
				return
			}
			require.Nil(t, err)
			require.NotNil(t, actual)
			// assert.Equal doesn't handle net.IP well
			assert.Equal(t, structFieldCount(test.expected), structFieldCount(actual))
			assert.Truef(t, test.expected.SrcIP.Equal(actual.SrcIP), "mismatch source IPs: expected %s, got %s", test.expected.SrcIP.String(), actual.SrcIP.String())
			assert.Truef(t, test.expected.DstIP.Equal(actual.DstIP), "mismatch dest IPs: expected %s, got %s", test.expected.DstIP.String(), actual.DstIP.String())
			assert.Truef(t, test.expected.InnerSrcIP.Equal(actual.InnerSrcIP), "mismatch inner source IPs: expected %s, got %s", test.expected.InnerSrcIP.String(), actual.InnerSrcIP.String())
			assert.Truef(t, test.expected.InnerDstIP.Equal(actual.InnerDstIP), "mismatch inner dest IPs: expected %s, got %s", test.expected.InnerDstIP.String(), actual.InnerDstIP.String())
			assert.Equal(t, test.expected.InnerSrcPort, actual.InnerSrcPort)
			assert.Equal(t, test.expected.InnerDstPort, actual.InnerDstPort)
			assert.Equal(t, test.expected.InnerSeqNum, actual.InnerSeqNum)
		})
	}
}

func createMockICMPPacket(t *testing.T, ipLayer *layers.IPv4, icmpLayer *layers.ICMPv4, innerIP *layers.IPv4, innerTCP *layers.TCP, partialTCPHeader bool) gopacket.Packet {
	innerBuf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}

	innerLayers := make([]gopacket.SerializableLayer, 0, 2)
	if innerIP != nil {
		innerLayers = append(innerLayers, innerIP)
	}
	if innerTCP != nil {
		innerLayers = append(innerLayers, innerTCP)
		if innerIP != nil {
			innerTCP.SetNetworkLayerForChecksum(innerIP)
		}
	}

	gopacket.SerializeLayers(innerBuf, opts,
		innerLayers...,
	)
	payload := innerBuf.Bytes()

	innerPkt := gopacket.NewPacket(payload, layers.LayerTypeIPv4, gopacket.Default)
	for _, layer := range innerPkt.Layers() {
		t.Logf("Inner Layer %s: %+v", layer.LayerType().String(), layer)
	}

	// if partialTCP is set, truncate
	// the payload to include only the
	// first 8 bytes of the TCP header
	if partialTCPHeader {
		payload = payload[:32]
	}

	buf := gopacket.NewSerializeBuffer()
	gopacket.SerializeLayers(buf, opts,
		ipLayer,
		icmpLayer,
		gopacket.Payload(payload),
	)

	pkt := gopacket.NewPacket(buf.Bytes(), layers.LayerTypeIPv4, gopacket.Default)

	return pkt
}

func createMockIPv4Layer(srcIP, dstIP net.IP, protocol layers.IPProtocol) *layers.IPv4 {
	return &layers.IPv4{
		SrcIP:    srcIP,
		DstIP:    dstIP,
		IHL:      5,
		Version:  4,
		Protocol: protocol,
	}
}

func createMockICMPLayer(typeCode layers.ICMPv4TypeCode) *layers.ICMPv4 {
	return &layers.ICMPv4{
		TypeCode: typeCode,
	}
}

func createMockTCPLayer(srcPort uint16, dstPort uint16, seqNum uint32, ackNum uint32, syn bool, ack bool, rst bool) *layers.TCP {
	return &layers.TCP{
		SrcPort: layers.TCPPort(srcPort),
		DstPort: layers.TCPPort(dstPort),
		Seq:     seqNum,
		Ack:     ackNum,
		SYN:     syn,
		ACK:     ack,
		RST:     rst,
	}
}

func structFieldCount(v interface{}) int {
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return -1
	}

	return val.NumField()
}
