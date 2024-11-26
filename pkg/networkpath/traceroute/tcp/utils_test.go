// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package tcp

import (
	"net"
	"reflect"
	"testing"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/ipv4"
)

var (
	srcIP = net.ParseIP("1.2.3.4")
	dstIP = net.ParseIP("5.6.7.8")

	innerSrcIP = net.ParseIP("10.0.0.1")
	innerDstIP = net.ParseIP("192.168.1.1")
)

func Test_createRawTCPSyn(t *testing.T) {
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

func Test_parseICMP(t *testing.T) {
	ipv4Header := createMockIPv4Header(srcIP, dstIP, 1)
	icmpLayer := createMockICMPLayer(layers.ICMPv4CodeTTLExceeded)
	innerIPv4Layer := createMockIPv4Layer(innerSrcIP, innerDstIP, layers.IPProtocolTCP)
	innerTCPLayer := createMockTCPLayer(12345, 443, 28394, 12737, true, true, true)

	tt := []struct {
		description string
		inHeader    *ipv4.Header
		inPayload   []byte
		expected    *icmpResponse
		errMsg      string
	}{
		{
			description: "empty IPv4 layer should return an error",
			inHeader:    &ipv4.Header{},
			inPayload:   []byte{},
			expected:    nil,
			errMsg:      "invalid IP header for ICMP packet",
		},
		{
			description: "missing ICMP layer should return an error",
			inHeader:    ipv4Header,
			inPayload:   []byte{},
			expected:    nil,
			errMsg:      "failed to decode ICMP packet",
		},
		{
			description: "missing inner layers should return an error",
			inHeader:    ipv4Header,
			inPayload:   createMockICMPPacket(nil, icmpLayer, nil, nil, false),
			expected:    nil,
			errMsg:      "failed to decode inner ICMP payload",
		},
		{
			description: "ICMP packet with partial TCP header should create icmpResponse",
			inHeader:    ipv4Header,
			inPayload:   createMockICMPPacket(nil, icmpLayer, innerIPv4Layer, innerTCPLayer, true),
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
			inHeader:    ipv4Header,
			inPayload:   createMockICMPPacket(nil, icmpLayer, innerIPv4Layer, innerTCPLayer, true),
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
			actual, err := parseICMP(test.inHeader, test.inPayload)
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

func Test_parseTCP(t *testing.T) {
	ipv4Header := createMockIPv4Header(srcIP, dstIP, 6) // 6 is TCP
	tcpLayer := createMockTCPLayer(12345, 443, 28394, 12737, true, true, true)

	// full packet
	encodedTCPLayer, fullTCPPacket := createMockTCPPacket(ipv4Header, tcpLayer, false)

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
			description: "missing TCP layer should return an error",
			inHeader:    ipv4Header,
			inPayload:   []byte{},
			expected:    nil,
			errMsg:      "failed to decode TCP packet",
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
			assert.Equal(t, structFieldCount(test.expected), structFieldCount(actual))
			assert.Truef(t, test.expected.SrcIP.Equal(actual.SrcIP), "mismatch source IPs: expected %s, got %s", test.expected.SrcIP.String(), actual.SrcIP.String())
			assert.Truef(t, test.expected.DstIP.Equal(actual.DstIP), "mismatch dest IPs: expected %s, got %s", test.expected.DstIP.String(), actual.DstIP.String())
			assert.Equal(t, test.expected.TCPResponse, actual.TCPResponse)
		})
	}
}

func BenchmarkParseTCP(b *testing.B) {
	ipv4Header := createMockIPv4Header(srcIP, dstIP, 6) // 6 is TCP
	tcpLayer := createMockTCPLayer(12345, 443, 28394, 12737, true, true, true)

	// full packet
	_, fullTCPPacket := createMockTCPPacket(ipv4Header, tcpLayer, false)

	tp := newTCPParser()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := tp.parseTCP(ipv4Header, fullTCPPacket)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func createMockIPv4Header(srcIP, dstIP net.IP, protocol int) *ipv4.Header {
	return &ipv4.Header{
		Version:  4,
		Src:      srcIP,
		Dst:      dstIP,
		Protocol: protocol,
		TTL:      64,
		Len:      8,
	}
}

func createMockICMPPacket(ipLayer *layers.IPv4, icmpLayer *layers.ICMPv4, innerIP *layers.IPv4, innerTCP *layers.TCP, partialTCPHeader bool) []byte {
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

	// if partialTCP is set, truncate
	// the payload to include only the
	// first 8 bytes of the TCP header
	if partialTCPHeader {
		payload = payload[:32]
	}

	buf := gopacket.NewSerializeBuffer()
	gopacket.SerializeLayers(buf, opts,
		icmpLayer,
		gopacket.Payload(payload),
	)

	icmpBytes := buf.Bytes()
	if ipLayer == nil {
		return icmpBytes
	}

	buf = gopacket.NewSerializeBuffer()
	gopacket.SerializeLayers(buf, opts,
		ipLayer,
		gopacket.Payload(icmpBytes),
	)

	return buf.Bytes()
}

func createMockTCPPacket(ipHeader *ipv4.Header, tcpLayer *layers.TCP, includeHeader bool) (*layers.TCP, []byte) {
	ipLayer := &layers.IPv4{
		Version:  4,
		SrcIP:    ipHeader.Src,
		DstIP:    ipHeader.Dst,
		Protocol: layers.IPProtocol(ipHeader.Protocol),
		TTL:      64,
		Length:   8,
	}
	tcpLayer.SetNetworkLayerForChecksum(ipLayer)
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if includeHeader {
		gopacket.SerializeLayers(buf, opts,
			ipLayer,
			tcpLayer,
		)
	} else {
		gopacket.SerializeLayers(buf, opts,
			tcpLayer,
		)
	}

	pkt := gopacket.NewPacket(buf.Bytes(), layers.LayerTypeTCP, gopacket.Default)

	// return encoded TCP layer here
	return pkt.Layer(layers.LayerTypeTCP).(*layers.TCP), buf.Bytes()
}

func createMockIPv4Layer(srcIP, dstIP net.IP, protocol layers.IPProtocol) *layers.IPv4 {
	return &layers.IPv4{
		SrcIP:    srcIP,
		DstIP:    dstIP,
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
