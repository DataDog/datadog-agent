// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package sack has selective ACK-based tracerouting logic
package sack

import (
	"fmt"
	"net/netip"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
)

// NotSupportedError means the target did not respond with the SACK Permitted
// TCP option, or we couldn't establish a TCP connection to begin with
type NotSupportedError struct {
	Err error
}

func (e *NotSupportedError) Error() string {
	return fmt.Sprintf("SACK not supported by the target: %s", e.Err)
}
func (e *NotSupportedError) Unwrap() error {
	return e.Err
}

// Params is the SACK traceroute parameters
type Params struct {
	// Target is the IP:port to traceroute
	Target netip.AddrPort
	// HandshakeTimeout is how long to wait for a handshake SYNACK to be seen
	HandshakeTimeout time.Duration
	// FinTimeout is how much extra time to allow for FIN to finish
	FinTimeout time.Duration
	// ParallelParams are the standard params for parallel traceroutes
	ParallelParams common.TracerouteParallelParams
	// LoosenICMPSrc disables checking the source IP/port in ICMP payloads when enabled.
	// Reason: Some environments don't properly translate the payload of an ICMP TTL exceeded
	// packet meaning you can't trust the source address to correspond to your own private IP.
	LoosenICMPSrc bool
}

// MaxTimeout returns the sum of all timeouts/delays for a SACK traceroute
func (p Params) MaxTimeout() time.Duration {
	return p.HandshakeTimeout + p.FinTimeout + p.ParallelParams.MaxTimeout()
}
