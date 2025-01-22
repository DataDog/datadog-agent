// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcp

import (
	"fmt"
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"golang.org/x/net/ipv4"
)

var (
	buf = gopacket.NewSerializeBuffer()
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

	// clear the gopacket.SerializeBuffer
	if len(buf.Bytes()) > 0 {
		buf.Clear() // nolint:errcheck
	}
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
