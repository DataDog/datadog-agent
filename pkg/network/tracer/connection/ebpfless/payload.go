// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package ebpfless contains supporting code for the ebpfless tracer
package ebpfless

import (
	"errors"
	"fmt"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"

	"github.com/DataDog/datadog-agent/pkg/network"
)

var errZeroLengthUDPPacket = errors.New("UDP packet with length 0")
var errZeroLengthIPPacket = errors.New("IP packet with length 0")

// UDPPayloadLen returns the UDP payload length from a layers.UDP object
func UDPPayloadLen(udp *layers.UDP) (uint16, error) {
	if udp.Length == 0 {
		return 0, errZeroLengthUDPPacket
	}

	// Length includes the header (8 bytes),
	// so we need to exclude that here
	return udp.Length - 8, nil
}

// TCPPayloadLen returns the TCP payload length from a layers.TCP object
func TCPPayloadLen(family network.ConnectionFamily, ip4 *layers.IPv4, ip6 *layers.IPv6, tcp *layers.TCP) (uint16, error) {
	var ipl uint16
	var err error
	switch family {
	case network.AFINET:
		ipl, err = ipv4PayloadLen(ip4)
	case network.AFINET6:
		ipl, err = ipv6PayloadLen(ip6)
	default:
		return 0, fmt.Errorf("unknown family %s", family)
	}

	if err != nil {
		return 0, nil
	}

	if ipl == 0 {
		return 0, errZeroLengthIPPacket
	}

	// the data offset field in the TCP header specifies
	// the length of the TCP header in 32 bit words, so
	// subtracting that here to get the payload size
	//
	// see https://en.wikipedia.org/wiki/Transmission_Control_Protocol#TCP_segment_structure
	return ipl - uint16(tcp.DataOffset)*4, nil
}

func ipv4PayloadLen(ip4 *layers.IPv4) (uint16, error) {
	// the IHL field specifies the the size of the IP
	// header in 32 bit words, so subtracting that here
	// to get the payload size
	//
	// see https://en.wikipedia.org/wiki/IPv4#Header
	return ip4.Length - uint16(ip4.IHL)*4, nil
}

func ipv6PayloadLen(ip6 *layers.IPv6) (uint16, error) {
	if ip6.NextHeader == layers.IPProtocolUDP || ip6.NextHeader == layers.IPProtocolTCP {
		return ip6.Length, nil
	}

	var ipExt layers.IPv6ExtensionSkipper
	parser := gopacket.NewDecodingLayerParser(gopacket.LayerTypePayload, &ipExt)
	decoded := make([]gopacket.LayerType, 0, 1)
	l := ip6.Length
	payload := ip6.Payload
	for len(payload) > 0 {
		err := parser.DecodeLayers(payload, &decoded)
		if err != nil {
			return 0, fmt.Errorf("error decoding with ipv6 extension skipper: %w", err)
		}

		if len(decoded) == 0 {
			return l, nil
		}

		l -= uint16(len(ipExt.Contents))
		if ipExt.NextHeader == layers.IPProtocolTCP || ipExt.NextHeader == layers.IPProtocolUDP {
			break
		}

		payload = ipExt.Payload
	}

	return l, nil
}
