// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package filter

// Packet type constants for Darwin. These values are intentionally aligned with
// golang.org/x/sys/unix PACKET_* on Linux so that shared code (e.g. connDirectionFromPktType
// in ebpfless/tcp_utils.go) works consistently across platforms without conditional logic.
const (
	PacketHost      = 0 // To us
	PacketBroadcast = 1 // To all
	PacketMulticast = 2 // To group
	PacketOtherHost = 3 // To someone else
	PacketOutgoing  = 4 // Outgoing of any type
)
