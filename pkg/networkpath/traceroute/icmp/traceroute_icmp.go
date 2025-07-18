// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package icmp has icmp tracerouting logic
package icmp

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/packets"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NotSupportedError means not on a supported platform.
type NotSupportedError struct {
	Err error
}

func (e *NotSupportedError) Error() string {
	return fmt.Sprintf("icmp not supported by the target: %s", e.Err)
}
func (e *NotSupportedError) Unwrap() error {
	return e.Err
}

// Params is the ICMP traceroute parameters
type Params struct {
	// Target is the IP:port to traceroute
	Target netip.Addr
	// ParallelParams are the standard params for parallel traceroutes
	ParallelParams common.TracerouteParallelParams
}

// MaxTimeout returns the sum of all timeouts/delays for an ICMP traceroute
func (p Params) MaxTimeout() time.Duration {
	ttl := time.Duration(p.ParallelParams.MaxTTL - p.ParallelParams.MinTTL)
	return ttl * p.ParallelParams.MaxTimeout()
}

func (p Params) validate() error {
	addr := p.Target
	if !addr.IsValid() {
		return fmt.Errorf("ICMP traceroute provided invalid IP address")
	}
	return nil
}

type icmpResult struct {
	LocalAddr netip.AddrPort
	Hops      []*common.ProbeResponse
}

func runICMPTraceroute(ctx context.Context, p Params) (*icmpResult, error) {
	err := p.validate()
	if err != nil {
		return nil, fmt.Errorf("invalid icmp driver params: %w", err)
	}

	local, udpConn, err := common.LocalAddrForHost(p.Target.AsSlice(), 80)
	if err != nil {
		return nil, fmt.Errorf("failed to get local addr: %w", err)
	}
	udpConn.Close()
	deadline := time.Now().Add(p.MaxTimeout())
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	// get this platform's Source and Sink implementations
	handle, err := packets.NewSourceSink(common.IPFamily(p.Target))
	if err != nil {
		return nil, fmt.Errorf("TCP Traceroute failed to make NewSourceSink: %w", err)
	}

	// create the raw packet connection which watches for TCP/ICMP responses
	driver, err := newICMPDriver(p, local.AddrPort().Addr(), handle.Sink, handle.Source)
	if err != nil {
		return nil, fmt.Errorf("failed to init icmp driver: %w", err)
	}
	defer driver.Close()

	log.Debugf("icmp traceroute dialing %s", p.Target)
	// this actually runs the traceroute
	resp, err := common.TracerouteParallel(ctx, driver, p.ParallelParams)
	if err != nil {
		return nil, fmt.Errorf("icmp traceroute failed: %w", err)
	}

	result := &icmpResult{
		LocalAddr: local.AddrPort(),
		Hops:      resp,
	}
	return result, nil
}

// RunICMPTraceroute fully executes a ICMP traceroute using the given parameters
func RunICMPTraceroute(ctx context.Context, p Params) (*common.Results, error) {
	icmpResult, err := runICMPTraceroute(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("icmp traceroute failed: %w", err)
	}

	hops, err := common.ToHops(p.ParallelParams.TracerouteParams, icmpResult.Hops)
	if err != nil {
		return nil, fmt.Errorf("icmp traceroute ToHops failed: %w", err)
	}

	result := &common.Results{
		Source:     icmpResult.LocalAddr.Addr().AsSlice(),
		SourcePort: icmpResult.LocalAddr.Port(),
		Target:     p.Target.AsSlice(),
		Hops:       hops,
		Tags:       []string{"icmp"},
	}

	return result, nil
}
