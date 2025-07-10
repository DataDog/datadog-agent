// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package packets

import (
	"net"
	"testing"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func clearLayer(layer *layers.BaseLayer) {
	layer.Contents = nil
	layer.Payload = nil
}

// SerializeLayers doesn't populate these fields, so we exclude them from equality comparison
func clearBuffers(parser *FrameParser) {
	clearLayer(&parser.IP4.BaseLayer)
	clearLayer(&parser.TCP.BaseLayer)
	clearLayer(&parser.ICMP4.BaseLayer)
}

func TestFrameParserTCP(t *testing.T) {
	ip4 := &layers.IPv4{
		Version:  4,
		TTL:      123,
		SrcIP:    net.ParseIP("127.0.0.1"),
		DstIP:    net.ParseIP("127.0.0.2"),
		Id:       41821,
		Protocol: layers.IPProtocolTCP,
	}
	tcp := &layers.TCP{
		SrcPort: layers.TCPPort(345),
		DstPort: layers.TCPPort(678),
		ACK:     true,
		PSH:     true,
		Seq:     1234,
		Ack:     5678,
		Window:  1024,
		Options: []layers.TCPOption{},
	}
	err := tcp.SetNetworkLayerForChecksum(ip4)
	require.NoError(t, err)
	payload := gopacket.Payload([]byte{123})

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
	err = gopacket.SerializeLayers(buf, opts, ip4, tcp, payload)
	require.NoError(t, err)

	parser := NewFrameParser()

	err = parser.Parse(buf.Bytes())
	require.NoError(t, err)

	clearBuffers(parser)

	require.EqualExportedValues(t, ip4, &parser.IP4)
	require.EqualExportedValues(t, tcp, &parser.TCP)
	require.Equal(t, payload, parser.Payload)
}

func TestFrameParserICMP4(t *testing.T) {
	tcp := &layers.TCP{
		SrcPort: layers.TCPPort(345),
		DstPort: layers.TCPPort(678),
		Seq:     1234,
		Ack:     5678,
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths: true,
	}
	err := gopacket.SerializeLayers(buf, opts, tcp, gopacket.Payload(nil))
	require.NoError(t, err)
	tcpBytes := buf.Bytes()[:8]

	ip4 := &layers.IPv4{
		Version:  4,
		TTL:      123,
		SrcIP:    net.ParseIP("127.0.0.1"),
		DstIP:    net.ParseIP("127.0.0.2"),
		Id:       41821,
		Protocol: layers.IPProtocolICMPv4,
	}
	icmp4 := &layers.ICMPv4{
		TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeTimeExceeded, layers.ICMPv4CodeTTLExceeded),
	}
	payload := gopacket.Payload(tcpBytes)

	buf = gopacket.NewSerializeBuffer()
	opts = gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
	err = gopacket.SerializeLayers(buf, opts, ip4, icmp4, payload)
	require.NoError(t, err)

	parser := NewFrameParser()

	err = parser.Parse(buf.Bytes())
	require.NoError(t, err)

	clearBuffers(parser)

	require.EqualExportedValues(t, ip4, &parser.IP4)
	require.EqualExportedValues(t, icmp4, &parser.ICMP4)

	tcpInfo, err := ParseTCPFirstBytes(parser.Payload)
	require.NoError(t, err)

	expectedInfo := TCPInfo{
		SrcPort: uint16(tcp.SrcPort),
		DstPort: uint16(tcp.DstPort),
		Seq:     tcp.Seq,
	}
	require.Equal(t, expectedInfo, tcpInfo)
}

func TestFrameParserUnrecognizedPacket(t *testing.T) {
	ip4 := &layers.IPv4{
		Version:  4,
		TTL:      123,
		SrcIP:    net.ParseIP("127.0.0.1"),
		DstIP:    net.ParseIP("127.0.0.2"),
		Id:       41821,
		Protocol: layers.IPProtocolSCTP,
	}
	sctp := &layers.SCTP{
		SrcPort: 42,
		DstPort: 123,
	}
	payload := gopacket.Payload([]byte{42})

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
	err := gopacket.SerializeLayers(buf, opts, ip4, sctp, payload)
	require.NoError(t, err)

	parser := NewFrameParser()

	err = parser.Parse(buf.Bytes())
	require.ErrorIs(t, err, ignoredLayerErr)
}

func TestFrameParserTLSPacket(t *testing.T) {
	// tests a TLS packet which has one extra layer we don't care about
	ip4 := &layers.IPv4{
		Version:  4,
		TTL:      123,
		SrcIP:    net.ParseIP("127.0.0.1"),
		DstIP:    net.ParseIP("127.0.0.2"),
		Id:       41821,
		Protocol: layers.IPProtocolTCP,
	}
	tcp := &layers.TCP{
		SrcPort: layers.TCPPort(345),
		DstPort: layers.TCPPort(678),
		ACK:     true,
		PSH:     true,
		Seq:     1234,
		Ack:     5678,
		Window:  1024,
		Options: []layers.TCPOption{},
	}
	err := tcp.SetNetworkLayerForChecksum(ip4)
	require.NoError(t, err)

	tls := &layers.TLS{}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
	err = gopacket.SerializeLayers(buf, opts, ip4, tcp, tls)
	require.NoError(t, err)

	parser := NewFrameParser()

	err = parser.Parse(buf.Bytes())
	require.NoError(t, err)

	clearBuffers(parser)

	require.EqualExportedValues(t, ip4, &parser.IP4)
	require.EqualExportedValues(t, tcp, &parser.TCP)
}
