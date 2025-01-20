// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package udp adds a UDP traceroute implementation to the agent
package udp

import (
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
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
		icmpParser *common.ICMPParser
	}
)

// NewUDPv4 initializes a new UDPv4 traceroute instance
func NewUDPv4(target net.IP, targetPort uint16, numPaths uint16, minTTL uint8, maxTTL uint8, delay time.Duration, timeout time.Duration) *UDPv4 {
	icmpParser := common.NewICMPUDPParser()

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
	}
}

// Close doesn't to anything yet, but we should
// use this to close out long running sockets
// when we're done with a path test
func (u *UDPv4) Close() error {
	return nil
}
