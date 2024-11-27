// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tcp adds a TCP traceroute implementation to the agent
package tcp

import (
	"net"
	"time"

	"github.com/google/gopacket/layers"
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
	}

	// Results encapsulates a response from the TCP
	// traceroute
	Results struct {
		Source     net.IP
		SourcePort uint16
		Target     net.IP
		DstPort    uint16
		Hops       []*Hop
	}

	// Hop encapsulates information about a single
	// hop in a TCP traceroute
	Hop struct {
		IP       net.IP
		Port     uint16
		ICMPType layers.ICMPv4TypeCode
		RTT      time.Duration
		IsDest   bool
	}
)

// Close doesn't to anything yet, but we should
// use this to close out long running sockets
// when we're done with a path test
func (t *TCPv4) Close() error {
	return nil
}
