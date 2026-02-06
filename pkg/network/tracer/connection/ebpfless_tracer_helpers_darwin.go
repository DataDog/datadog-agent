// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package connection

import "github.com/DataDog/datadog-agent/pkg/network/filter"

// extractPacketType extracts the packet type from platform-specific PacketInfo
func extractPacketType(info filter.PacketInfo) uint8 {
	if darwinInfo, ok := info.(*filter.DarwinPacketInfo); ok {
		return darwinInfo.PktType
	}
	// Default to outgoing if we can't determine
	return filter.PACKET_OUTGOING
}
