// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package icmp provides the logic for parsing ICMP packets
package icmp

import (
	"fmt"
	"net"

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
	// Parser defines the interface for parsing
	// ICMP packets
	Parser interface {
		Match(header *ipv4.Header, packet []byte, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, innerIdentifier uint32, packetID uint16) (net.IP, error)
		Parse(header *ipv4.Header, packet []byte) (*Response, error)
	}

	// Response encapsulates the data from
	// an ICMP response packet needed for matching
	Response struct {
		SrcIP        net.IP
		DstIP        net.IP
		TypeCode     layers.ICMPv4TypeCode
		InnerIPID    uint16
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
)

// Matches checks if an ICMPResponse matches the expected response
// based on the local and remote IP, port, and identifier. In this context,
// identifier will either be the TCP sequence number OR the UDP checksum
func (i *Response) Matches(localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, innerIdentifier uint32, packetID uint16) bool {
	return localIP.Equal(i.InnerSrcIP) &&
		remoteIP.Equal(i.InnerDstIP) &&
		localPort == i.InnerSrcPort &&
		remotePort == i.InnerDstPort &&
		innerIdentifier == i.InnerIdentifier &&
		packetID == i.InnerIPID
}

func validatePacket(header *ipv4.Header, payload []byte) error {
	// in addition to parsing, it is probably not a bad idea to do some validation
	// so we can quickly ignore the ICMP packets we don't care about
	if len(payload) <= 0 {
		return fmt.Errorf("received empty ICMP packet")
	}

	if header.Protocol != IPProtoICMP || header.Version != 4 ||
		header.Src == nil || header.Dst == nil {
		return fmt.Errorf("invalid IP header for ICMP packet: %+v", header)
	}

	return nil
}
