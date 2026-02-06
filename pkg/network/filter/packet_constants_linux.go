// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package filter

import "golang.org/x/sys/unix"

// On Linux, alias the constants from the unix package for consistency with Darwin
const (
	PACKET_HOST      = unix.PACKET_HOST
	PACKET_BROADCAST = unix.PACKET_BROADCAST
	PACKET_MULTICAST = unix.PACKET_MULTICAST
	PACKET_OTHERHOST = unix.PACKET_OTHERHOST
	PACKET_OUTGOING  = unix.PACKET_OUTGOING
)
