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
)

var errZeroLengthUDPPacket = errors.New("UDP packet with length 0")
var errZeroLengthIPPacket = errors.New("IP packet with length 0")

// Layers holds a set of network layers for a packet
type Layers struct {
	IP4 *layers.IPv4
	IP6 *layers.IPv6
	UDP *layers.UDP
	TCP *layers.TCP
}

// Reset resets a Layers object
func (l *Layers) Reset(ip4 *layers.IPv4, ip6 *layers.IPv6, udp *layers.UDP, tcp *layers.TCP) {
	l.IP4 = ip4
	l.IP6 = ip6
	l.UDP = udp
	l.TCP = tcp
}

// PayloadLen returns the length of the application
// payload given the set of layers in `Layers`
func (l Layers) PayloadLen() (uint16, error) {
	if len(l.UDP.Payload) > 0 || len(l.UDP.Contents) > 0 {
		if l.UDP.Length == 0 {
			return 0, errZeroLengthUDPPacket
		}

		// Length includes the header (8 bytes),
		// so we need to exclude that here
		return l.UDP.Length - 8, nil
	}

	var ipl uint16
	var err error
	if l.IP4 != nil {
		ipl, err = ipv4PayloadLen(l.IP4)
	} else if l.IP6 != nil {
		ipl, err = ipv6PayloadLen(l.IP6)
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
	return ipl - uint16(l.TCP.DataOffset)*4, nil
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
