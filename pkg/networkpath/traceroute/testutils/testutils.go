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

func CreateMockICMPPacket(ipLayer *layers.IPv4, icmpLayer *layers.ICMPv4, innerIP *layers.IPv4, innerTCP *layers.TCP, partialTCPHeader bool) []byte {
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

func CreateMockTCPPacket(ipHeader *ipv4.Header, tcpLayer *layers.TCP, includeHeader bool) (*layers.TCP, []byte) {
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

func CreateMockIPv4Layer(srcIP, dstIP net.IP, protocol layers.IPProtocol) *layers.IPv4 {
	return &layers.IPv4{
		SrcIP:    srcIP,
		DstIP:    dstIP,
		Version:  4,
		Protocol: protocol,
	}
}

func CreateMockICMPLayer(typeCode layers.ICMPv4TypeCode) *layers.ICMPv4 {
	return &layers.ICMPv4{
		TypeCode: typeCode,
	}
}

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
