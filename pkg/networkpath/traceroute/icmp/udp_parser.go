// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package icmp

import (
	"errors"
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"golang.org/x/net/ipv4"
)

type (
	// UDPParser encapsulates the data and logic
	// for parsing ICMP packets with embedded UDP
	// data
	UDPParser struct {
		icmpLayer     layers.ICMPv4
		innerIPLayer  layers.IPv4
		innerUDPLayer layers.UDP
		innerTCPLayer layers.TCP
		// packetParser is parser for the ICMP segment of the packet
		packetParser *gopacket.DecodingLayerParser
		// innerPacketParser is necessary for ICMP packets
		// because gopacket does not allow the payload of
		// an ICMP packet to be decoded in the same parser
		innerPacketParser *gopacket.DecodingLayerParser
		icmpResponse      *Response
	}
)

// NewICMPUDPParser creates a new ICMPParser that can parse ICMP packets with
// embedded UDP packets
func NewICMPUDPParser() Parser {
	icmpParser := &UDPParser{}
	icmpParser.packetParser = gopacket.NewDecodingLayerParser(layers.LayerTypeICMPv4, &icmpParser.icmpLayer)
	icmpParser.innerPacketParser = gopacket.NewDecodingLayerParser(layers.LayerTypeIPv4, &icmpParser.innerIPLayer, &icmpParser.innerUDPLayer)
	icmpParser.packetParser.IgnoreUnsupported = true
	icmpParser.innerPacketParser.IgnoreUnsupported = true
	return icmpParser
}

// Match encapsulates to logic to both parse and match an ICMP packet
func (p *UDPParser) Match(header *ipv4.Header, packet []byte, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, innerIdentifier uint32, packetID uint16) (net.IP, error) {
	if header.Protocol != IPProtoICMP {
		return net.IP{}, errors.New("expected an ICMP packet")
	}
	icmpResponse, err := p.Parse(header, packet)
	if err != nil {
		return net.IP{}, fmt.Errorf("ICMP parse error: %w", err)
	}
	if !icmpResponse.Matches(localIP, localPort, remoteIP, remotePort, innerIdentifier, packetID) {
		return net.IP{}, common.MismatchError("ICMP packet doesn't match")
	}

	return icmpResponse.SrcIP, nil
}

// Parse parses an ICMP packet with embedded UDP data and returns a Response
func (p *UDPParser) Parse(header *ipv4.Header, payload []byte) (*Response, error) {
	if err := validatePacket(header, payload); err != nil {
		return nil, err
	}

	// clear layers between each run
	p.icmpLayer = layers.ICMPv4{}
	p.innerIPLayer = layers.IPv4{}
	p.innerTCPLayer = layers.TCP{}
	p.innerUDPLayer = layers.UDP{}

	p.icmpResponse = &Response{} // ensure we get a fresh ICMPResponse each run
	p.icmpResponse.SrcIP = header.Src
	p.icmpResponse.DstIP = header.Dst

	decoded := []gopacket.LayerType{}
	if err := p.packetParser.DecodeLayers(payload, &decoded); err != nil {
		return nil, fmt.Errorf("failed to decode ICMP packet: %w", err)
	}
	// since we ignore unsupported layers, we need to check if we actually decoded
	// anything
	if len(decoded) < 1 {
		return nil, fmt.Errorf("failed to decode ICMP packet, no layers decoded")
	}
	p.icmpResponse.TypeCode = p.icmpLayer.TypeCode

	// a separate parser is needed to decode the inner IP and UDP headers because
	// gopacket doesn't support this type of nesting in a single decoder
	if err := p.innerPacketParser.DecodeLayers(p.icmpLayer.Payload, &decoded); err != nil {
		return nil, fmt.Errorf("failed to decode inner ICMP payload: %w", err)
	}

	p.icmpResponse.InnerSrcIP = p.innerIPLayer.SrcIP
	p.icmpResponse.InnerDstIP = p.innerIPLayer.DstIP
	p.icmpResponse.InnerSrcPort = uint16(p.innerUDPLayer.SrcPort)
	p.icmpResponse.InnerDstPort = uint16(p.innerUDPLayer.DstPort)
	// the packet's checksum is used as the identifier for UDP packets
	p.icmpResponse.InnerIdentifier = uint32(p.innerUDPLayer.Checksum)
	p.icmpResponse.InnerIPID = p.innerIPLayer.Id

	return p.icmpResponse, nil
}
