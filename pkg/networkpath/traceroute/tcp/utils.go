// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcp

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
	// ACK is the acknowledge TCP flag
	ACK = 1 << 4
	// RST is the reset TCP flag
	RST = 1 << 2
	// SYN is the synchronization TCP flag
	SYN = 1 << 1

	// IPProtoICMP is the ICMP protocol number
	IPProtoICMP = 1
	// IPProtoTCP is the TCP protocol number
	IPProtoTCP = 6
)

type (
	// canceledError is sent when a listener
	// is canceled
	canceledError string

	// icmpResponse encapsulates the data from
	// an ICMP response packet needed for matching
	icmpResponse struct {
		SrcIP        net.IP
		DstIP        net.IP
		TypeCode     layers.ICMPv4TypeCode
		InnerSrcIP   net.IP
		InnerDstIP   net.IP
		InnerSrcPort uint16
		InnerDstPort uint16
		InnerSeqNum  uint32
	}

	// tcpResponse encapsulates the data from a
	// TCP response needed for matching
	tcpResponse struct {
		SrcIP       net.IP
		DstIP       net.IP
		TCPResponse *layers.TCP
	}

	rawConnWrapper interface {
		SetReadDeadline(t time.Time) error
		ReadFrom(b []byte) (*ipv4.Header, []byte, *ipv4.ControlMessage, error)
		WriteTo(h *ipv4.Header, p []byte, cm *ipv4.ControlMessage) error
	}
)

func localAddrForHost(destIP net.IP, destPort uint16) (*net.UDPAddr, error) {
	// this is a quick way to get the local address for connecting to the host
	// using UDP as the network type to avoid actually creating a connection to
	// the host, just get the OS to give us a local IP and local ephemeral port
	conn, err := net.Dial("udp4", net.JoinHostPort(destIP.String(), strconv.Itoa(int(destPort))))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	localAddr := conn.LocalAddr()

	localUDPAddr, ok := localAddr.(*net.UDPAddr)
	if !ok {
		return nil, fmt.Errorf("invalid address type for %s: want %T, got %T", localAddr, localUDPAddr, localAddr)
	}

	return localUDPAddr, nil
}

// createRawTCPSyn creates a TCP packet with the specified parameters
func createRawTCPSyn(sourceIP net.IP, sourcePort uint16, destIP net.IP, destPort uint16, seqNum uint32, ttl int) (*ipv4.Header, []byte, error) {
	ipHdr, packet, hdrlen, err := createRawTCPSynBuffer(sourceIP, sourcePort, destIP, destPort, seqNum, ttl)
	if err != nil {
		return nil, nil, err
	}

	return ipHdr, packet[:hdrlen], nil
}

func createRawTCPSynBuffer(sourceIP net.IP, sourcePort uint16, destIP net.IP, destPort uint16, seqNum uint32, ttl int) (*ipv4.Header, []byte, int, error) {
	ipLayer := &layers.IPv4{
		Version:  4,
		Length:   20,
		TTL:      uint8(ttl),
		Id:       uint16(41821),
		Protocol: 6,
		DstIP:    destIP,
		SrcIP:    sourceIP,
	}

	tcpLayer := &layers.TCP{
		SrcPort: layers.TCPPort(sourcePort),
		DstPort: layers.TCPPort(destPort),
		Seq:     seqNum,
		Ack:     0,
		SYN:     true,
		Window:  1024,
	}

	err := tcpLayer.SetNetworkLayerForChecksum(ipLayer)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to create packet checksum: %w", err)
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	err = gopacket.SerializeLayers(buf, opts,
		ipLayer,
		tcpLayer,
	)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to serialize packet: %w", err)
	}
	packet := buf.Bytes()

	var ipHdr ipv4.Header
	if err := ipHdr.Parse(packet[:20]); err != nil {
		return nil, nil, 0, fmt.Errorf("failed to parse IP header: %w", err)
	}

	return &ipHdr, packet, 20, nil
}


// parseICMP takes in an IPv4 header and payload and tries to convert to an ICMP
// message, it returns all the fields from the packet we need to validate it's the response
// we're looking for
func parseICMP(header *ipv4.Header, payload []byte) (*icmpResponse, error) {
	// in addition to parsing, it is probably not a bad idea to do some validation
	// so we can ignore the ICMP packets we don't care about
	icmpResponse := icmpResponse{}

	if header.Protocol != IPProtoICMP || header.Version != 4 ||
		header.Src == nil || header.Dst == nil {
		return nil, fmt.Errorf("invalid IP header for ICMP packet: %+v", header)
	}
	icmpResponse.SrcIP = header.Src
	icmpResponse.DstIP = header.Dst

	var icmpv4Layer layers.ICMPv4
	decoded := []gopacket.LayerType{}
	icmpParser := gopacket.NewDecodingLayerParser(layers.LayerTypeICMPv4, &icmpv4Layer)
	icmpParser.IgnoreUnsupported = true // ignore unsupported layers, we will decode them in the next step
	if err := icmpParser.DecodeLayers(payload, &decoded); err != nil {
		return nil, fmt.Errorf("failed to decode ICMP packet: %w", err)
	}
	// since we ignore unsupported layers, we need to check if we actually decoded
	// anything
	if len(decoded) < 1 {
		return nil, fmt.Errorf("failed to decode ICMP packet, no layers decoded")
	}
	icmpResponse.TypeCode = icmpv4Layer.TypeCode

	var icmpPayload []byte
	if len(icmpv4Layer.Payload) < 40 {
		log.Tracef("Payload length %d is less than 40, extending...\n", len(icmpv4Layer.Payload))
		icmpPayload = make([]byte, 40)
		copy(icmpPayload, icmpv4Layer.Payload)
		// we have to set this in order for the TCP
		// parser to work
		icmpPayload[32] = 5 << 4 // set data offset
	} else {
		icmpPayload = icmpv4Layer.Payload
	}

	// a separate parser is needed to decode the inner IP and TCP headers because
	// gopacket doesn't support this type of nesting in a single decoder
	var innerIPLayer layers.IPv4
	var innerTCPLayer layers.TCP
	innerIPParser := gopacket.NewDecodingLayerParser(layers.LayerTypeIPv4, &innerIPLayer, &innerTCPLayer)
	if err := innerIPParser.DecodeLayers(icmpPayload, &decoded); err != nil {
		return nil, fmt.Errorf("failed to decode inner ICMP payload: %w", err)
	}
	icmpResponse.InnerSrcIP = innerIPLayer.SrcIP
	icmpResponse.InnerDstIP = innerIPLayer.DstIP
	icmpResponse.InnerSrcPort = uint16(innerTCPLayer.SrcPort)
	icmpResponse.InnerDstPort = uint16(innerTCPLayer.DstPort)
	icmpResponse.InnerSeqNum = innerTCPLayer.Seq

	return &icmpResponse, nil
}

func parseTCP(header *ipv4.Header, payload []byte) (*tcpResponse, error) {
	tcpResponse := tcpResponse{}

	if header.Protocol != IPProtoTCP || header.Version != 4 ||
		header.Src == nil || header.Dst == nil {
		return nil, fmt.Errorf("invalid IP header for TCP packet: %+v", header)
	}
	tcpResponse.SrcIP = header.Src
	tcpResponse.DstIP = header.Dst

	var tcpLayer layers.TCP
	decoded := []gopacket.LayerType{}
	tcpParser := gopacket.NewDecodingLayerParser(layers.LayerTypeTCP, &tcpLayer)
	if err := tcpParser.DecodeLayers(payload, &decoded); err != nil {
		return nil, fmt.Errorf("failed to decode TCP packet: %w", err)
	}
	tcpResponse.TCPResponse = &tcpLayer

	return &tcpResponse, nil
}

func icmpMatch(localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32, response *icmpResponse) bool {
	return localIP.Equal(response.InnerSrcIP) &&
		remoteIP.Equal(response.InnerDstIP) &&
		localPort == response.InnerSrcPort &&
		remotePort == response.InnerDstPort &&
		seqNum == response.InnerSeqNum
}

func tcpMatch(localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32, response *tcpResponse) bool {
	flagsCheck := (response.TCPResponse.SYN && response.TCPResponse.ACK) || response.TCPResponse.RST
	sourcePort := uint16(response.TCPResponse.SrcPort)
	destPort := uint16(response.TCPResponse.DstPort)

	return remoteIP.Equal(response.SrcIP) &&
		remotePort == sourcePort &&
		localIP.Equal(response.DstIP) &&
		localPort == destPort &&
		seqNum == response.TCPResponse.Ack-1 &&
		flagsCheck
}

func (c canceledError) Error() string {
	return string(c)
}
