// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package udp adds a UDP traceroute implementation to the agent
package udp

type (
	// UDPv4 encapsulates the data needed to run
	// a UDPv4 traceroute
	UDPv4 struct {
		// Target   net.IP
		// srcIP    net.IP // calculated internally
		// srcPort  uint16 // calculated internally
		// DestPort uint16
		// NumPaths uint16
		// MinTTL   uint8
		// MaxTTL   uint8
		// Delay    time.Duration // delay between sending packets (not applicable if we go the serial send/receive route)
		// Timeout  time.Duration // full timeout for all packets
	}
)

// Close doesn't to anything yet, but we should
// use this to close out long running sockets
// when we're done with a path test
func (u *UDPv4) Close() error {
	return nil
}
