// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package udp

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/packets"
)

//nolint:unused // This is used, but not on all platforms yet
func (u *UDPv4) newTracerouteDriver() (*udpDriver, error) {
	targetAddr, ok := common.UnmappedAddrFromSlice(u.Target)
	if !ok {
		return nil, fmt.Errorf("failed to get netipAddr for target %s", u.Target)
	}
	u.Target = targetAddr.AsSlice()

	sink, err := packets.NewSinkUnix(targetAddr)
	if err != nil {
		return nil, fmt.Errorf("Traceroute failed to make SinkUnix: %w", err)
	}

	source, err := packets.NewAFPacketSource()
	if err != nil {
		sink.Close()
		return nil, fmt.Errorf("Traceroute failed to make AFPacketSource: %w", err)
	}

	return newUDPDriver(u, sink, source), nil
}
