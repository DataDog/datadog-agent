// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && linux_bpf

package connection

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
)

const (
	// the segment length to read on Linux
	// mac header (with vlan) + ip header + tcp header
	segmentLen = 18 + 60 + 60
)

// createPacketSource creates a Linux-specific AFPacket packet source
func createPacketSource(cfg *config.Config) (filter.PacketSource, error) {
	packetSrc, err := filter.NewAFPacketSource(
		8<<20, // 8 MB total space
		filter.OptSnapLen(segmentLen))
	if err != nil {
		return nil, fmt.Errorf("error creating AFPacket source: %w", err)
	}
	return packetSrc, nil
}
