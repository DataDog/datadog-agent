// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package filter

import "golang.org/x/sys/unix"

// On Linux, alias the constants from the unix package for consistency with Darwin
const (
	PacketHost      = unix.PACKET_HOST
	PacketBroadcast = unix.PACKET_BROADCAST
	PacketMulticast = unix.PACKET_MULTICAST
	PacketOtherHost = unix.PACKET_OTHERHOST
	PacketOutgoing  = unix.PACKET_OUTGOING
)
