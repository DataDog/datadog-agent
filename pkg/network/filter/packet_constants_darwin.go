// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package filter

// Define packet type constants for Darwin (these exist in unix package on Linux)
// We define them here to match the Linux values for compatibility
const (
	PACKET_HOST      = 0 // To us
	PACKET_BROADCAST = 1 // To all
	PACKET_MULTICAST = 2 // To group
	PACKET_OTHERHOST = 3 // To someone else
	PACKET_OUTGOING  = 4 // Outgoing of any type
)
