// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcp

import (
	"errors"
	"fmt"
	"net"
	"syscall"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"golang.org/x/net/ipv4"
)

type (
	// tcpResponse encapsulates the data from a
	// TCP response needed for matching
	tcpResponse struct {
		SrcIP       net.IP
		DstIP       net.IP
		TCPResponse layers.TCP
	}

	// tcpParser encapsulates everything needed to
	// decode TCP packets off the wire into structs
	tcpParser struct {
		layer               layers.TCP
		decoded             []gopacket.LayerType
		decodingLayerParser *gopacket.DecodingLayerParser
	}
)

// reserveLocalPort reserves an ephemeral TCP port
// and returns both the listener and port because the
// listener should be held until the port is no longer
// in use
func reserveLocalPort() (uint16, net.Listener, error) {
	// Create a TCP listener with port 0 to get a random port from the OS
	// and reserve it for the duration of the traceroute
	tcpListener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create TCP listener: %w", err)
	}
	tcpAddr := tcpListener.Addr().(*net.TCPAddr)

	return uint16(tcpAddr.Port), tcpListener, nil
}

// createRawTCPSyn creates a TCP packet with the specified parameters
func createRawTCPSyn(sourceIP net.IP, sourcePort uint16, destIP net.IP, destPort uint16, seqNum uint32, ttl int) (*ipv4.Header, []byte, error) {
	ipHdr, packet, hdrlen, err := createRawTCPSynBuffer(sourceIP, sourcePort, destIP, destPort, seqNum, ttl)
	if err != nil {
		return nil, nil, err
	}

	return ipHdr, packet[hdrlen:], nil
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

func newTCPParser() *tcpParser {
	tcpParser := &tcpParser{}
	tcpParser.decodingLayerParser = gopacket.NewDecodingLayerParser(layers.LayerTypeTCP, &tcpParser.layer)
	return tcpParser
}

func (tp *tcpParser) parseTCP(header *ipv4.Header, payload []byte) (*tcpResponse, error) {
	if header.Protocol != syscall.IPPROTO_TCP || header.Version != 4 ||
		header.Src == nil || header.Dst == nil {
		return nil, fmt.Errorf("invalid IP header for TCP packet: %+v", header)
	}
	if len(payload) <= 0 {
		return nil, errors.New("received empty TCP payload")
	}

	if err := tp.decodingLayerParser.DecodeLayers(payload, &tp.decoded); err != nil {
		return nil, fmt.Errorf("failed to decode TCP packet: %w", err)
	}

	resp := &tcpResponse{
		SrcIP:       header.Src,
		DstIP:       header.Dst,
		TCPResponse: tp.layer,
	}
	// make sure the TCP layer is cleared between runs
	tp.layer = layers.TCP{}

	return resp, nil
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
