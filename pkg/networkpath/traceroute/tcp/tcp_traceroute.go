// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package tcp

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
)

// Traceroute runs a TCP traceroute
func (t *TCPv4) Traceroute() (*common.Results, error) {
	addr, conn, err := common.LocalAddrForHost(t.Target, t.DestPort)
	if err != nil {
		return nil, fmt.Errorf("TCP Traceroute failed to get local address for target: %w", err)
	}
	conn.Close() // we don't need the UDP port here
	t.srcIP = addr.IP

	// reserve a local port so that the tcpDriver has free reign to safely send packets on it
	port, tcpListener, err := reserveLocalPort()
	if err != nil {
		return nil, fmt.Errorf("TCP Traceroute failed to create TCP listener: %w", err)
	}
	defer tcpListener.Close()
	t.srcPort = port

	// get this platform's tcpDriver implementation
	driver, err := t.newTracerouteDriver()
	if err != nil {
		return nil, fmt.Errorf("TCP Traceroute failed to getTracerouteDriver: %w", err)
	}

	params := common.TracerouteSerialParams{
		TracerouteParams: common.TracerouteParams{
			MinTTL:            t.MinTTL,
			MaxTTL:            t.MaxTTL,
			TracerouteTimeout: t.Timeout,
			PollFrequency:     100 * time.Millisecond,
			SendDelay:         t.Delay,
		},
	}
	resp, err := common.TracerouteSerial(context.Background(), driver, params)
	if err != nil {
		return nil, err
	}

	hops, err := common.ToHops(params.TracerouteParams, resp)
	if err != nil {
		return nil, fmt.Errorf("SYN traceroute ToHops failed: %w", err)
	}

	result := &common.Results{
		Source:     t.srcIP,
		SourcePort: t.srcPort,
		Target:     t.Target,
		DstPort:    t.DestPort,
		Hops:       hops,
		Tags:       []string{"tcp_method:syn", fmt.Sprintf("paris_traceroute_mode_enabled:%t", t.ParisTracerouteMode)},
	}

	return result, nil
}
