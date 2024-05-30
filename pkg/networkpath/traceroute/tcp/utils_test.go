// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// go:build test

package tcp

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/ipv4"
)

var (
	srcIP = net.ParseIP("1.2.3.4")
	dstIP = net.ParseIP("5.6.7.8")
)

type (
	mockRawConn struct {
		setReadDeadlineErr error
		readDeadline       time.Time

		readTimeoutCount int
		readFromErr      error
		header           *ipv4.Header
		payload          []byte
		cm               *ipv4.ControlMessage

		writeDelay time.Duration
		writeToErr error
	}

	mockTimeoutErr string
)

func Test_handlePackets(t *testing.T) {
	tt := []struct {
		description string
		// input
		ctxTimeout time.Duration
		conn       rawConnWrapper
		listener   string
		localIP    net.IP
		localPort  uint16
		remoteIP   net.IP
		remotePort uint16
		seqNum     uint32
		// output
		expectedIP       net.IP
		expectedPort     uint16
		expectedTypeCode layers.ICMPv4TypeCode
		errMsg           string
	}{
		{
			description: "canceled context returns canceledErr",
			ctxTimeout:  300 * time.Millisecond,
			conn: &mockRawConn{
				readTimeoutCount: 100,
				readFromErr:      errors.New("bad test error"),
			},
			errMsg: "canceled",
		},
		{
			description: "set timeout error returns an error",
			ctxTimeout:  300 * time.Millisecond,
			conn: &mockRawConn{
				setReadDeadlineErr: errors.New("good test error"),
				readTimeoutCount:   100,
				readFromErr:        errors.New("bad error"),
			},
			errMsg: "good test error",
		},
		{
			description: "non-timeout read error returns an error",
			ctxTimeout:  1 * time.Second,
			conn: &mockRawConn{
				readFromErr: errors.New("test read error"),
			},
			errMsg: "test read error",
		},
		{
			description: "invalid listener returns unsupported listener",
			ctxTimeout:  1 * time.Second,
			conn: &mockRawConn{
				header:  &ipv4.Header{},
				payload: nil,
			},
			listener: "invalid",
			errMsg:   "unsupported",
		},
		{
			description: "failed ICMP parsing eventuallly returns cancel timeout",
			ctxTimeout:  500 * time.Millisecond,
			conn: &mockRawConn{
				header:  &ipv4.Header{},
				payload: nil,
			},
			listener: "icmp",
			errMsg:   "canceled",
		},
		{
			description: "failed TCP parsing eventuallly returns cancel timeout",
			ctxTimeout:  500 * time.Millisecond,
			conn: &mockRawConn{
				header:  &ipv4.Header{},
				payload: nil,
			},
			listener: "tcp",
			errMsg:   "canceled",
		},
	}

	for _, test := range tt {
		t.Run(test.description, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), test.ctxTimeout)
			defer cancel()
			actualIP, actualPort, actualTypeCode, err := handlePackets(ctx, test.conn, test.listener, test.localIP, test.localPort, test.remoteIP, test.remotePort, test.seqNum)
			if test.errMsg != "" {
				require.Error(t, err)
				assert.True(t, strings.Contains(err.Error(), test.errMsg))
				return
			}
			require.NoError(t, err)
			assert.Truef(t, test.expectedIP.Equal(actualIP), "mismatch source IPs: expected %s, got %s", test.expectedIP.String(), actualIP.String())
			assert.Equal(t, test.expectedPort, actualPort)
			assert.Equal(t, test.expectedTypeCode, actualTypeCode)
		})
	}
}

func Test_parseIPv4Layer(t *testing.T) {
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

func Test_parseICMPPacket(t *testing.T) {
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
			input:       createMockICMPPacket(ipv4Layer, icmpLayer, nil, nil, false),
			expected:    nil,
			errMsg:      "failed to decode ICMP payload",
		},
		{
			description: "ICMP packet with partial TCP header should create icmpResponse",
			input:       createMockICMPPacket(ipv4Layer, icmpLayer, innerIPv4Layer, innerTCPLayer, true),
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
			input:       createMockICMPPacket(ipv4Layer, icmpLayer, innerIPv4Layer, innerTCPLayer, false),
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

func Test_parseTCPPacket(t *testing.T) {
	ipv4Layer := createMockIPv4Layer(srcIP, dstIP, layers.IPProtocolTCP)
	tcpLayer := createMockTCPLayer(12345, 443, 28394, 12737, true, true, true)

	// create packet without an ICMP layer
	buf := gopacket.NewSerializeBuffer()
	gopacket.SerializeLayers(buf, gopacket.SerializeOptions{},
		ipv4Layer,
	)
	missingTCPLayer := gopacket.NewPacket(buf.Bytes(), layers.LayerTypeIPv4, gopacket.Default)

	// full packet
	encodedTCPLayer, fullTCPPacket := createMockTCPPacket(ipv4Layer, tcpLayer)

	tt := []struct {
		description string
		input       gopacket.Packet
		expected    *tcpResponse
		errMsg      string
	}{
		{
			description: "empty IPv4 layer should return an error",
			input:       gopacket.NewPacket([]byte{}, layers.LayerTypeTCP, gopacket.Default),
			expected:    nil,
			errMsg:      "packet does not contain an IPv4 layer",
		},
		{
			description: "missing TCP layer should return an error",
			input:       missingTCPLayer,
			expected:    nil,
			errMsg:      "packet does not contain a TCP layer",
		},
		{
			description: "full TCP packet should create tcpResponse",
			input:       fullTCPPacket,
			expected: &tcpResponse{
				SrcIP:       srcIP,
				DstIP:       dstIP,
				TCPResponse: encodedTCPLayer,
			},
			errMsg: "",
		},
	}

	for _, test := range tt {
		t.Run(test.description, func(t *testing.T) {
			actual, err := parseTCPPacket(test.input)
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

func (m *mockRawConn) SetReadDeadline(t time.Time) error {
	if m.setReadDeadlineErr != nil {
		return m.setReadDeadlineErr
	}
	m.readDeadline = t

	return nil
}
func (m *mockRawConn) ReadFrom(_ []byte) (*ipv4.Header, []byte, *ipv4.ControlMessage, error) {
	if m.readTimeoutCount > 0 {
		m.readTimeoutCount--
		time.Sleep(time.Until(m.readDeadline))
		return nil, nil, nil, &net.OpError{Err: mockTimeoutErr("test timeout error")}
	}
	if m.readFromErr != nil {
		return nil, nil, nil, m.readFromErr
	}

	return m.header, m.payload, m.cm, nil
}

func (m *mockRawConn) WriteTo(_ *ipv4.Header, _ []byte, _ *ipv4.ControlMessage) error {
	time.Sleep(m.writeDelay)
	return m.writeToErr
}

func (me mockTimeoutErr) Error() string {
	return string(me)
}

func (me mockTimeoutErr) Timeout() bool {
	return true
}

func createMockICMPPacket(ipLayer *layers.IPv4, icmpLayer *layers.ICMPv4, innerIP *layers.IPv4, innerTCP *layers.TCP, partialTCPHeader bool) gopacket.Packet {
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
		ipLayer,
		icmpLayer,
		gopacket.Payload(payload),
	)

	pkt := gopacket.NewPacket(buf.Bytes(), layers.LayerTypeIPv4, gopacket.Default)

	return pkt
}

func createMockTCPPacket(ipLayer *layers.IPv4, tcpLayer *layers.TCP) (*layers.TCP, gopacket.Packet) {
	tcpLayer.SetNetworkLayerForChecksum(ipLayer)
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	gopacket.SerializeLayers(buf, opts,
		ipLayer,
		tcpLayer,
	)

	pkt := gopacket.NewPacket(buf.Bytes(), layers.LayerTypeIPv4, gopacket.Default)

	// return encoded TCP layer here
	return pkt.Layer(layers.LayerTypeTCP).(*layers.TCP), pkt
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
