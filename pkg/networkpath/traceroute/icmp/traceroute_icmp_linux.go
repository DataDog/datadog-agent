// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package icmp

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/packets"
	"net/netip"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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

	sink, err := packets.NewSinkUnix(p.Target)
	if err != nil {
		return nil, fmt.Errorf("runICMPTraceroute failed to make SinkUnix: %w", err)
	}

	source, err := packets.NewAFPacketSource()
	if err != nil {
		sink.Close()
		return nil, fmt.Errorf("runICMPTraceroute failed to make ICMP raw conn: %w", err)
	}

	// create the raw packet connection which watches for TCP/ICMP responses
	driver, err := newICMPDriver(p, local.AddrPort().Addr(), sink, source)
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
