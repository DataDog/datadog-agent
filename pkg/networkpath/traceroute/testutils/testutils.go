// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package testutils

import (
	"net"
	"reflect"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"golang.org/x/net/ipv4"
)

// CreateMockIPv4Header creates a mock IPv4 header for testing
func CreateMockIPv4Header(srcIP, dstIP net.IP, protocol int) *ipv4.Header {
	return &ipv4.Header{
		Version:  4,
		Src:      srcIP,
		Dst:      dstIP,
		Protocol: protocol,
		TTL:      64,
		Len:      8,
	}
}

// CreateMockICMPWithTCPPacket creates a mock ICMP packet for testing
func CreateMockICMPWithTCPPacket(ipLayer *layers.IPv4, icmpLayer *layers.ICMPv4, innerIP *layers.IPv4, innerTCP *layers.TCP, partialTCPHeader bool) []byte {
	innerBuf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}

	innerLayers := make([]gopacket.SerializableLayer, 0, 2)
	if innerIP != nil {
		innerLayers = append(innerLayers, innerIP)
	}
	if innerTCP != nil {
		innerLayers = append(innerLayers, innerTCP)
		if innerIP != nil {
			innerTCP.SetNetworkLayerForChecksum(innerIP) // nolint: errcheck
		}
	}

	gopacket.SerializeLayers(innerBuf, opts, // nolint: errcheck
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
	gopacket.SerializeLayers(buf, opts, // nolint: errcheck
		icmpLayer,
		gopacket.Payload(payload),
	)

	icmpBytes := buf.Bytes()
	if ipLayer == nil {
		return icmpBytes
	}

	buf = gopacket.NewSerializeBuffer()
	gopacket.SerializeLayers(buf, opts, // nolint: errcheck
		ipLayer,
		gopacket.Payload(icmpBytes),
	)

	return buf.Bytes()
}

// CreateMockICMPWithUDPPacket creates a mock ICMP packet for testing
func CreateMockICMPWithUDPPacket(ipLayer *layers.IPv4, icmpLayer *layers.ICMPv4, innerIP *layers.IPv4, innerUDP *layers.UDP) []byte {
	innerBuf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}

	innerLayers := make([]gopacket.SerializableLayer, 0, 2)
	if innerIP != nil {
		innerLayers = append(innerLayers, innerIP)
	}
	if innerUDP != nil {
		innerLayers = append(innerLayers, innerUDP)
		if innerIP != nil {
			innerUDP.SetNetworkLayerForChecksum(innerIP) // nolint: errcheck
		}
	}

	gopacket.SerializeLayers(innerBuf, opts, // nolint: errcheck
		innerLayers...,
	)
	payload := innerBuf.Bytes()

	buf := gopacket.NewSerializeBuffer()
	gopacket.SerializeLayers(buf, opts, // nolint: errcheck
		icmpLayer,
		gopacket.Payload(payload),
	)

	icmpBytes := buf.Bytes()
	if ipLayer == nil {
		return icmpBytes
	}

	buf = gopacket.NewSerializeBuffer()
	gopacket.SerializeLayers(buf, opts, // nolint: errcheck
		ipLayer,
		gopacket.Payload(icmpBytes),
	)

	return buf.Bytes()
}

// CreateMockTCPPacket creates a mock TCP packet for testing
func CreateMockTCPPacket(ipHeader *ipv4.Header, tcpLayer *layers.TCP, includeHeader bool) (*layers.TCP, []byte) {
	ipLayer := &layers.IPv4{
		Version:  4,
		SrcIP:    ipHeader.Src,
		DstIP:    ipHeader.Dst,
		Protocol: layers.IPProtocol(ipHeader.Protocol),
		TTL:      64,
		Length:   8,
	}
	tcpLayer.SetNetworkLayerForChecksum(ipLayer) // nolint: errcheck
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if includeHeader {
		gopacket.SerializeLayers(buf, opts, // nolint: errcheck
			ipLayer,
			tcpLayer,
		)
	} else {
		gopacket.SerializeLayers(buf, opts, // nolint: errcheck
			tcpLayer,
		)
	}

	pkt := gopacket.NewPacket(buf.Bytes(), layers.LayerTypeTCP, gopacket.Default)

	// return encoded TCP layer here
	return pkt.Layer(layers.LayerTypeTCP).(*layers.TCP), buf.Bytes()
}

// CreateMockIPv4Layer creates a mock IPv4 layer for testing
func CreateMockIPv4Layer(packetID uint16, srcIP, dstIP net.IP, protocol layers.IPProtocol) *layers.IPv4 {
	return &layers.IPv4{
		Id:       packetID,
		SrcIP:    srcIP,
		DstIP:    dstIP,
		Version:  4,
		Protocol: protocol,
	}
}

// CreateMockICMPLayer creates a mock ICMP layer for testing
func CreateMockICMPLayer(respType uint8, respCode uint8) *layers.ICMPv4 {
	typeCode := layers.CreateICMPv4TypeCode(respType, respCode)
	return &layers.ICMPv4{
		TypeCode: typeCode,
	}
}

// CreateMockTCPLayer creates a mock TCP layer for testing
func CreateMockTCPLayer(srcPort uint16, dstPort uint16, seqNum uint32, ackNum uint32, syn bool, ack bool, rst bool) *layers.TCP {
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

// CreateMockUDPLayer creates a mock UDP layer for testing
func CreateMockUDPLayer(srcPort uint16, dstPort uint16, checksum uint16) *layers.UDP {
	return &layers.UDP{
		SrcPort:  layers.UDPPort(srcPort),
		DstPort:  layers.UDPPort(dstPort),
		Checksum: checksum,
	}
}

// StructFieldCount returns the number of fields in a struct
func StructFieldCount(v interface{}) int {
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return -1
	}

	return val.NumField()
}
