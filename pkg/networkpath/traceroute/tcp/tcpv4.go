// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tcp adds a TCP traceroute implementation to the agent
package tcp

import (
	"fmt"
	"net"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"golang.org/x/net/ipv4"
)

type (
	// TCPv4 encapsulates the data needed to run
	// a TCPv4 traceroute
	TCPv4 struct {
		Target   net.IP
		srcIP    net.IP // calculated internally
		srcPort  uint16 // calculated internally
		DestPort uint16
		NumPaths uint16
		MinTTL   uint8
		MaxTTL   uint8
		Delay    time.Duration // delay between sending packets (not applicable if we go the serial send/receive route)
		Timeout  time.Duration // full timeout for all packets
		buffer   gopacket.SerializeBuffer
	}
)

// NewTCPv4 initializes a new TCPv4 traceroute instance
func NewTCPv4(target net.IP, targetPort uint16, numPaths uint16, minTTL uint8, maxTTL uint8, delay time.Duration, timeout time.Duration) *TCPv4 {
	buffer := gopacket.NewSerializeBufferExpectedSize(40, 0)

	return &TCPv4{
		Target:   target,
		DestPort: targetPort,
		NumPaths: numPaths,
		MinTTL:   minTTL,
		MaxTTL:   maxTTL,
		Delay:    delay,
		Timeout:  timeout,
		buffer:   buffer,
	}
}

// Close doesn't to anything yet, but we should
// use this to close out long running sockets
// when we're done with a path test
func (t *TCPv4) Close() error {
	return nil
}

// createRawTCPSyn creates a TCP packet with the specified parameters
func (t *TCPv4) createRawTCPSyn(seqNum uint32, ttl int) (*ipv4.Header, []byte, error) {
	ipHdr, packet, hdrlen, err := t.createRawTCPSynBuffer(seqNum, ttl)
	if err != nil {
		return nil, nil, err
	}

	return ipHdr, packet[hdrlen:], nil
}

func (t *TCPv4) createRawTCPSynBuffer(seqNum uint32, ttl int) (*ipv4.Header, []byte, int, error) {
	// if this function is modified in a way that changes the size,
	// update the NewSerializeBufferExpectedSize call in NewTCPv4
	ipLayer := &layers.IPv4{
		Version:  4,
		Length:   20,
		TTL:      uint8(ttl),
		Id:       uint16(41821),
		Protocol: 6,
		DstIP:    t.Target,
		SrcIP:    t.srcIP,
	}

	tcpLayer := &layers.TCP{
		SrcPort: layers.TCPPort(t.srcPort),
		DstPort: layers.TCPPort(t.DestPort),
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
	if len(t.buffer.Bytes()) > 0 {
		if err = t.buffer.Clear(); err != nil {
			t.buffer = gopacket.NewSerializeBuffer()
		}
	}
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	err = gopacket.SerializeLayers(t.buffer, opts,
		ipLayer,
		tcpLayer,
	)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to serialize packet: %w", err)
	}
	packet := t.buffer.Bytes()

	var ipHdr ipv4.Header
	if err := ipHdr.Parse(packet[:20]); err != nil {
		return nil, nil, 0, fmt.Errorf("failed to parse IP header: %w", err)
	}

	return &ipHdr, packet, 20, nil
}
