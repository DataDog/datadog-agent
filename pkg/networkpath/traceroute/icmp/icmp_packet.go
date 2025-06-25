// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package icmp

import (
	"encoding/binary"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/packets"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

type icmpPacketGen struct {
	ipPair packets.IPPair
}

func (s *icmpPacketGen) generatePacketV4(ttl uint8, echoID uint16) (*layers.IPv4, *layers.ICMPv4, error) {
	ipLayer := &layers.IPv4{
		Version:  4,
		TTL:      ttl,
		SrcIP:    s.ipPair.SrcAddr.AsSlice(),
		DstIP:    s.ipPair.DstAddr.AsSlice(),
		Id:       echoID,
		Protocol: layers.IPProtocolICMPv4,
	}
	icmpLayer := &layers.ICMPv4{
		TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeEchoRequest, 0),
		Id:       echoID,
		Seq:      uint16(ttl),
	}
	return ipLayer, icmpLayer, nil
}

func (s *icmpPacketGen) generatePacketV6(ttl uint8, echoID uint16) (*layers.IPv6, *layers.ICMPv6, []byte, error) {
	ipLayer := &layers.IPv6{
		Version:    6,
		HopLimit:   ttl,
		SrcIP:      s.ipPair.SrcAddr.AsSlice(),
		DstIP:      s.ipPair.DstAddr.AsSlice(),
		NextHeader: layers.IPProtocolICMPv6,
	}
	icmpLayer := &layers.ICMPv6{
		TypeCode: layers.CreateICMPv6TypeCode(layers.ICMPv6TypeEchoRequest, 0),
	}

	// Construct payload: ID (2 bytes) + Seq (2 bytes) + data
	payload := make([]byte, 5)
	binary.BigEndian.PutUint16(payload[0:2], echoID)
	binary.BigEndian.PutUint16(payload[2:4], uint16(ttl))
	payload[4] = ttl // Extra data byte (same as IPv4 version)

	return ipLayer, icmpLayer, payload, nil
}

func (s *icmpPacketGen) generate(ttl uint8, echoID uint16, ipv6 bool) ([]byte, error) {
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}

	if ipv6 {
		ip6, icmpv6, payload, err := s.generatePacketV6(ttl, echoID)
		if err != nil {
			return nil, fmt.Errorf("failed to generate IPv6 packet: %w", err)
		}
		err = icmpv6.SetNetworkLayerForChecksum(ip6)
		if err != nil {
			return nil, fmt.Errorf("failed to SetNetworkLayerForChecksum IPv6 packet: %w", err)
		}
		err = gopacket.SerializeLayers(buf, opts, ip6, icmpv6, gopacket.Payload(payload))
		if err != nil {
			return nil, fmt.Errorf("failed to serialize IPv6 packet: %w", err)
		}
		return buf.Bytes(), nil
	}
	ip4, icmpv4, err := s.generatePacketV4(ttl, echoID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate IPv4 packet: %w", err)
	}
	err = gopacket.SerializeLayers(buf, opts, ip4, icmpv4, gopacket.Payload([]byte{ttl}))
	if err != nil {
		return nil, fmt.Errorf("failed to serialize IPv4 packet: %w", err)
	}
	return buf.Bytes(), nil
}
