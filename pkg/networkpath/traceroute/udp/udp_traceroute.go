// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package udp

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/packets"
)

// Traceroute runs a UDP traceroute
func (u *UDPv4) Traceroute() (*common.Results, error) {
	targetAddr, ok := common.UnmappedAddrFromSlice(u.Target)
	if !ok {
		return nil, fmt.Errorf("failed to get netipAddr for target %s", u.Target)
	}
	u.Target = targetAddr.AsSlice()

	addr, conn, err := common.LocalAddrForHost(u.Target, u.TargetPort)
	if err != nil {
		return nil, fmt.Errorf("UDP Traceroute failed to get local address for target: %w", err)
	}
	defer conn.Close()
	u.srcIP = addr.IP
	u.srcPort = uint16(addr.Port)

	// get this platform's Source and Sink implementations
	handle, err := packets.NewSourceSink(common.IPFamily(targetAddr))
	if err != nil {
		return nil, fmt.Errorf("UDP Traceroute failed to make NewSourceSink: %w", err)
	}
	if handle.MustClosePort {
		conn.Close()
	}

	driver := newUDPDriver(u, handle.Sink, handle.Source)

	params := common.TracerouteParallelParams{
		TracerouteParams: common.TracerouteParams{
			MinTTL:            u.MinTTL,
			MaxTTL:            u.MaxTTL,
			TracerouteTimeout: u.Timeout,
			PollFrequency:     100 * time.Millisecond,
			SendDelay:         u.Delay,
		},
	}
	resp, err := common.TracerouteParallel(context.Background(), driver, params)
	if err != nil {
		return nil, err
	}

	hops, err := common.ToHops(params.TracerouteParams, resp)
	if err != nil {
		return nil, fmt.Errorf("UDP traceroute ToHops failed: %w", err)
	}

	result := &common.Results{
		Source:     u.srcIP,
		SourcePort: u.srcPort,
		Target:     u.Target,
		DstPort:    u.TargetPort,
		Hops:       hops,
		Tags:       nil,
	}

	return result, nil
}
