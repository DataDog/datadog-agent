// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && linux_bpf

package connection

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"

	"github.com/DataDog/datadog-agent/pkg/network/filter"
)

// extractPacketType extracts the packet type from platform-specific PacketInfo
func extractPacketType(info filter.PacketInfo) uint8 {
	if afInfo, ok := info.(*filter.AFPacketInfo); ok {
		return afInfo.PktType
	}
	// Default to outgoing if we can't determine
	return filter.PacketOutgoing
}

// extractLayerType returns the gopacket layer type for this packet.
// Linux AF_PACKET always delivers Ethernet frames.
func extractLayerType(_ filter.PacketInfo) gopacket.LayerType {
	return layers.LayerTypeEthernet
}
