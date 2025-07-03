// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package udp adds a UDP traceroute implementation to the agent
package udp

import (
	"fmt"
	"golang.org/x/net/ipv4"
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/icmp"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

type (
	// UDPv4 encapsulates the data needed to run
	// a UDPv4 traceroute
	UDPv4 struct {
		Target     net.IP
		TargetPort uint16
		srcIP      net.IP // calculated internally
		srcPort    uint16 // calculated internally
		NumPaths   uint16
		MinTTL     uint8
		MaxTTL     uint8
		Delay      time.Duration // delay between sending packets (not applicable if we go the serial send/receive route)
		Timeout    time.Duration // full timeout for all packets
		icmpParser icmp.Parser
		buffer     gopacket.SerializeBuffer

		// LoosenICMPSrc disables checking the source IP/port in ICMP payloads when enabled.
		// Reason: Some environments don't properly translate the payload of an ICMP TTL exceeded
		// packet meaning you can't trust the source address to correspond to your own private IP.
		LoosenICMPSrc bool
	}
)

// NewUDPv4 initializes a new UDPv4 traceroute instance
func NewUDPv4(target net.IP, targetPort uint16, numPaths uint16, minTTL uint8, maxTTL uint8, delay time.Duration, timeout time.Duration) *UDPv4 {
	icmpParser := icmp.NewICMPUDPParser()
	buffer := gopacket.NewSerializeBufferExpectedSize(36, 0)

	return &UDPv4{
		Target:     target,
		TargetPort: targetPort,
		NumPaths:   numPaths,
		MinTTL:     minTTL,
		MaxTTL:     maxTTL,
		srcIP:      net.IP{}, // avoid linter error on linux as it's only used on windows
		srcPort:    0,        // avoid linter error on linux as it's only used on windows
		Delay:      delay,
		Timeout:    timeout,
		icmpParser: icmpParser,
		buffer:     buffer,
	}
}

// Close doesn't to anything yet, but we should
// use this to close out long running sockets
// when we're done with a path test
func (u *UDPv4) Close() error {
	return nil
}

// createRawUDPBuffer creates a raw UDP packet with the specified parameters
//
// the nolint:unused is necessary because we don't yet use this outside the Windows implementation
func (u *UDPv4) createRawUDPBuffer(sourceIP net.IP, sourcePort uint16, destIP net.IP, destPort uint16, ttl int) (uint16, []byte, uint16, error) { //nolint:unused
	// if this function is modified in a way that changes the size,
	// update the NewSerializeBufferExpectedSize call in NewUDPv4
	udpLayer := &layers.UDP{
		SrcPort: layers.UDPPort(sourcePort),
		DstPort: layers.UDPPort(destPort),
	}
	udpPayload := []byte("NSMNC\x00\x00\x00")
	id := uint16(ttl) + destPort
	udpPayload[6] = byte((id >> 8) & 0xff)
	udpPayload[7] = byte(id & 0xff)

	// TODO: compute checksum before serialization so we
	// can set ID field of the IP header to detect NATs just
	// as is done in dublin-traceroute. Gopacket doesn't expose
	// the checksum computations and modifying the IP header after
	// serialization would change its checksum
	var ipLayer gopacket.SerializableLayer
	if destIP.To4() != nil {
		ipv4Layer := &layers.IPv4{
			Version:  4,
			Length:   20,
			TTL:      uint8(ttl),
			Id:       uint16(41821),
			Protocol: layers.IPProtocolUDP, // hard code UDP so other OSs can use it
			DstIP:    destIP,
			SrcIP:    sourceIP,
			Flags:    layers.IPv4DontFragment, // needed for dublin-traceroute-like NAT detection
		}
		err := udpLayer.SetNetworkLayerForChecksum(ipv4Layer)
		if err != nil {
			return 0, nil, 0, fmt.Errorf("failed to create packet checksum: %w", err)
		}
		ipLayer = ipv4Layer
	} else {
		// Create IPv6 header
		ipv6Layer := &layers.IPv6{
			Version:    6,
			HopLimit:   uint8(ttl),
			NextHeader: layers.IPProtocolUDP,
			SrcIP:      sourceIP,
			DstIP:      destIP,
		}
		err := udpLayer.SetNetworkLayerForChecksum(ipv6Layer)
		if err != nil {
			return 0, nil, 0, fmt.Errorf("failed to create packet checksum: %w", err)
		}
		ipLayer = ipv6Layer
	}
	// clear the gopacket.SerializeBuffer
	if len(u.buffer.Bytes()) > 0 {
		if err := u.buffer.Clear(); err != nil {
			u.buffer = gopacket.NewSerializeBuffer()
		}
	}
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	err := gopacket.SerializeLayers(u.buffer, opts,
		ipLayer,
		udpLayer,
		gopacket.Payload(udpPayload),
	)
	if err != nil {
		return 0, nil, 0, fmt.Errorf("failed to serialize packet: %w", err)
	}

	packet := u.buffer.Bytes()
	if destIP.To4() != nil {
		var ipHdr ipv4.Header
		if err := ipHdr.Parse(packet[:20]); err != nil {
			return 0, nil, 0, fmt.Errorf("failed to parse IP header: %w", err)
		}
		id = uint16(ipHdr.ID)
	}
	return id, packet, udpLayer.Checksum, nil
}
