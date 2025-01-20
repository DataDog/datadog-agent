// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common contains common functionality for both TCP and UDP
// traceroute implementations
package common

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"golang.org/x/net/ipv4"
)

const (
	// IPProtoICMP is the IP protocol number for ICMP
	// we create our own constant here because there are
	// different imports for the constant in different
	// operating systems
	IPProtoICMP = 1
)

type (
	// Results encapsulates a response from the
	// traceroute
	Results struct {
		Source     net.IP
		SourcePort uint16
		Target     net.IP
		DstPort    uint16
		Hops       []*Hop
	}

	// Hop encapsulates information about a single
	// hop in a traceroute
	Hop struct {
		IP       net.IP
		Port     uint16
		ICMPType layers.ICMPv4TypeCode
		RTT      time.Duration
		IsDest   bool
	}

	// CanceledError is sent when a listener
	// is canceled
	CanceledError string

	// ICMPResponse encapsulates the data from
	// an ICMP response packet needed for matching
	ICMPResponse struct {
		SrcIP        net.IP
		DstIP        net.IP
		TypeCode     layers.ICMPv4TypeCode
		InnerSrcIP   net.IP
		InnerDstIP   net.IP
		InnerSrcPort uint16
		InnerDstPort uint16
		// InnerIdentifier will be populated with
		// an additional identifcation field for matching
		// received packets. For TCP packets, this is the
		// sequence number. For UDP packets, this is the
		// checksum, a uint16 cast to a uint32.
		InnerIdentifier uint32
	}

	// ICMPParser encapsulates the data and logic
	// for parsing ICMP packets with embedded TCP
	// or UDP packets
	ICMPParser struct {
		icmpLayer     layers.ICMPv4
		innerIPLayer  layers.IPv4
		innerUDPLayer layers.UDP
		innerTCPLayer layers.TCP
		innerPayload  gopacket.Payload
		isTCP         bool
		decoded       []gopacket.LayerType
		// packetParser is parser for the ICMP segment of the packet
		packetParser *gopacket.DecodingLayerParser
		// innerPacketParser is necessary for ICMP packets
		// because gopacket does not allow the payload of
		// an ICMP packet to be decoded in the same parser
		innerPacketParser *gopacket.DecodingLayerParser
		icmpResponse      *ICMPResponse
	}
)

func (c CanceledError) Error() string {
	return string(c)
}

// LocalAddrForHost takes in a destionation IP and port and returns the local
// address that should be used to connect to the destination. The returned connection
// should be closed by the caller when the the local UDP port is no longer needed
func LocalAddrForHost(destIP net.IP, destPort uint16) (*net.UDPAddr, net.Conn, error) {
	// this is a quick way to get the local address for connecting to the host
	// using UDP as the network type to avoid actually creating a connection to
	// the host, just get the OS to give us a local IP and local ephemeral port
	conn, err := net.Dial("udp4", net.JoinHostPort(destIP.String(), strconv.Itoa(int(destPort))))
	if err != nil {
		return nil, nil, err
	}
	localAddr := conn.LocalAddr()

	localUDPAddr, ok := localAddr.(*net.UDPAddr)
	if !ok {
		return nil, nil, fmt.Errorf("invalid address type for %s: want %T, got %T", localAddr, localUDPAddr, localAddr)
	}

	return localUDPAddr, conn, nil
}

// NewICMPTCPParser creates a new ICMPParser that can parse ICMP packets with
// embedded TCP packets
func NewICMPTCPParser() *ICMPParser {
	icmpParser := &ICMPParser{}
	icmpParser.packetParser = gopacket.NewDecodingLayerParser(layers.LayerTypeICMPv4, &icmpParser.icmpLayer, &icmpParser.innerPayload)
	icmpParser.innerPacketParser = gopacket.NewDecodingLayerParser(layers.LayerTypeIPv4, &icmpParser.innerIPLayer, &icmpParser.innerTCPLayer)
	// TODO: can we ignore unsupported layers?
	//icmpParser.packetParser.IgnoreUnsupported = true
	icmpParser.isTCP = true
	return icmpParser
}

// NewICMPUDPParser creates a new ICMPParser that can parse ICMP packets with
// embedded UDP packets
func NewICMPUDPParser() *ICMPParser {
	icmpParser := &ICMPParser{}
	icmpParser.packetParser = gopacket.NewDecodingLayerParser(layers.LayerTypeICMPv4, &icmpParser.icmpLayer)
	icmpParser.innerPacketParser = gopacket.NewDecodingLayerParser(layers.LayerTypeIPv4, &icmpParser.innerIPLayer, &icmpParser.innerUDPLayer)
	// TODO: can we ignore unsupported layers?
	icmpParser.packetParser.IgnoreUnsupported = true
	return icmpParser
}

// Parse parses an ICMP packet and returns the appropriate ICMPResponse
// depending on the configuration of the parser
func (p *ICMPParser) Parse(header *ipv4.Header, payload []byte) (*ICMPResponse, error) {
	// in addition to parsing, it is probably not a bad idea to do some validation
	// so we can ignore the ICMP packets we don't care about
	if header.Protocol != IPProtoICMP || header.Version != 4 ||
		header.Src == nil || header.Dst == nil {
		return nil, fmt.Errorf("invalid IP header for ICMP packet: %+v", header)
	}

	// clear layers between each run
	p.icmpLayer = layers.ICMPv4{}
	p.innerIPLayer = layers.IPv4{}
	p.innerTCPLayer = layers.TCP{}
	p.innerUDPLayer = layers.UDP{}
	p.innerPayload = gopacket.Payload{}

	p.icmpResponse = &ICMPResponse{} // ensure we get a fresh ICMPResponse each run
	p.icmpResponse.SrcIP = header.Src
	p.icmpResponse.DstIP = header.Dst

	if len(payload) <= 0 {
		return nil, fmt.Errorf("received empty ICMP packet")
	}

	if p.isTCP {
		return p.parseWithInnerTCP(payload)
	}

	return p.parseWithInnerUDP(payload)
}

func (p *ICMPParser) parseWithInnerTCP(payload []byte) (*ICMPResponse, error) {
	p.decoded = []gopacket.LayerType{}
	if err := p.packetParser.DecodeLayers(payload, &p.decoded); err != nil {
		return nil, fmt.Errorf("failed to decode ICMP packet: %w", err)
	}
	// since we ignore unsupported layers, we need to check if we actually decoded
	// anything
	if len(p.decoded) < 1 {
		return nil, fmt.Errorf("failed to decode ICMP packet, no layers decoded")
	}
	p.icmpResponse.TypeCode = p.icmpLayer.TypeCode

	var icmpPayload []byte
	if len(p.icmpLayer.Payload) < 40 {
		log.Tracef("Payload length %d is less than 40, extending...\n", len(p.icmpLayer.Payload))
		icmpPayload = make([]byte, 40)
		copy(icmpPayload, p.icmpLayer.Payload)
		// we have to set this in order for the inner
		// parser to work
		icmpPayload[32] = 5 << 4 // set data offset
	} else {
		icmpPayload = p.icmpLayer.Payload
	}

	// a separate parser is needed to decode the inner IP and TCP headers because
	// gopacket doesn't support this type of nesting in a single decoder
	if err := p.innerPacketParser.DecodeLayers(icmpPayload, &p.decoded); err != nil {
		return nil, fmt.Errorf("failed to decode inner ICMP payload: %w", err)
	}
	p.icmpResponse.InnerSrcIP = p.innerIPLayer.SrcIP
	p.icmpResponse.InnerDstIP = p.innerIPLayer.DstIP
	p.icmpResponse.InnerSrcPort = uint16(p.innerTCPLayer.SrcPort)
	p.icmpResponse.InnerDstPort = uint16(p.innerTCPLayer.DstPort)
	p.icmpResponse.InnerIdentifier = p.innerTCPLayer.Seq

	return p.icmpResponse, nil
}

func (p *ICMPParser) parseWithInnerUDP(payload []byte) (*ICMPResponse, error) {
	p.decoded = []gopacket.LayerType{}
	if err := p.packetParser.DecodeLayers(payload, &p.decoded); err != nil {
		return nil, fmt.Errorf("failed to decode ICMP packet: %w", err)
	}
	// since we ignore unsupported layers, we need to check if we actually decoded
	// anything
	if len(p.decoded) < 1 {
		return nil, fmt.Errorf("failed to decode ICMP packet, no layers decoded")
	}
	p.icmpResponse.TypeCode = p.icmpLayer.TypeCode

	// a separate parser is needed to decode the inner IP and UDP headers because
	// gopacket doesn't support this type of nesting in a single decoder
	if err := p.innerPacketParser.DecodeLayers(p.icmpLayer.Payload, &p.decoded); err != nil {
		return nil, fmt.Errorf("failed to decode inner ICMP payload: %w", err)
	}

	p.icmpResponse.InnerSrcIP = p.innerIPLayer.SrcIP
	p.icmpResponse.InnerDstIP = p.innerIPLayer.DstIP
	p.icmpResponse.InnerSrcPort = uint16(p.innerUDPLayer.SrcPort)
	p.icmpResponse.InnerDstPort = uint16(p.innerUDPLayer.DstPort)
	// the packet's checksum is used as the identifier for UDP packets
	p.icmpResponse.InnerIdentifier = uint32(p.innerUDPLayer.Checksum)

	return p.icmpResponse, nil
}

// ICMPMatch checks if an ICMP response matches the expected response
// based on the local and remote IP, port, and identifier. In this context,
// identifier will either be the TCP sequence number OR
func ICMPMatch(localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, innerIdentifier uint32, response *ICMPResponse) bool {
	basicEq := localIP.Equal(response.InnerSrcIP) &&
		remoteIP.Equal(response.InnerDstIP) &&
		localPort == response.InnerSrcPort &&
		remotePort == response.InnerDstPort

	idEq := innerIdentifier == response.InnerIdentifier

	// TODO: revert before merge
	if !basicEq {
		log.Warnf("ICMP Match failed on basic eq")
		log.Warnf("Expected source: %s\nExpected dest: %s\nExpected source port: %d\nExpected destination port: %d\n", localIP, remoteIP, localPort, remotePort)
		log.Warnf("source: %s\ndest: %s\nsource port: %d\ndestination port: %d\n", response.InnerSrcIP, response.InnerDstIP, response.InnerSrcPort, response.InnerDstPort)
	}

	if !idEq {
		log.Warnf("ICMP Match failed on identifier equality: got %d, wanted %d", response.InnerIdentifier, innerIdentifier)
	}

	return basicEq && idEq

	// return localIP.Equal(response.InnerSrcIP) &&
	// 	remoteIP.Equal(response.InnerDstIP) &&
	// 	localPort == response.InnerSrcPort &&
	// 	remotePort == response.InnerDstPort &&
	// 	innerIdentifier == response.InnerIdentifier
}
