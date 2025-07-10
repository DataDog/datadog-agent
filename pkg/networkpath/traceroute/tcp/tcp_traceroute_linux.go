// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package tcp

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/packets"
)

func (t *TCPv4) newTracerouteDriver() (*tcpDriver, error) {
	targetAddr, ok := common.UnmappedAddrFromSlice(t.Target)
	if !ok {
		return nil, fmt.Errorf("failed to get netipAddr for target %s", t.Target)
	}
	t.Target = targetAddr.AsSlice()

	sink, err := packets.NewSinkUnix(targetAddr)
	if err != nil {
		return nil, fmt.Errorf("Traceroute failed to make SinkUnix: %w", err)
	}

	source, err := packets.NewAFPacketSource()
	if err != nil {
		sink.Close()
		return nil, fmt.Errorf("Traceroute failed to make AFPacketSource: %w", err)
	}

	return newTCPDriver(t, sink, source), nil
}
