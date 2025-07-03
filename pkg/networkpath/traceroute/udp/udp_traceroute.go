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
)

// Traceroute runs a TCP traceroute
func (t *UDPv4) Traceroute() (*common.Results, error) {
	addr, conn, err := common.LocalAddrForHost(t.Target, t.TargetPort)
	if err != nil {
		return nil, fmt.Errorf("UDP Traceroute failed to get local address for target: %w", err)
	}
	defer conn.Close()
	t.srcIP = addr.IP
	t.srcPort = uint16(addr.Port)

	// get this platform's tcpDriver implementation
	driver, err := t.newTracerouteDriver()
	if err != nil {
		return nil, fmt.Errorf("UDP Traceroute failed to getTracerouteDriver: %w", err)
	}

	params := common.TracerouteParallelParams{
		TracerouteParams: common.TracerouteParams{
			MinTTL:            t.MinTTL,
			MaxTTL:            t.MaxTTL,
			TracerouteTimeout: t.Timeout,
			PollFrequency:     100 * time.Millisecond,
			SendDelay:         t.Delay,
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
		Source:     t.srcIP,
		SourcePort: t.srcPort,
		Target:     t.Target,
		DstPort:    t.TargetPort,
		Hops:       hops,
		Tags:       nil,
	}

	return result, nil
}
