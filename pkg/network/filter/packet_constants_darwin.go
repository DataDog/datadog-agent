// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package filter

// Define packet type constants for Darwin (these exist in unix package on Linux)
// We define them here to match the Linux values for compatibility
const (
	PacketHost      = 0 // To us
	PacketBroadcast = 1 // To all
	PacketMulticast = 2 // To group
	PacketOtherHost = 3 // To someone else
	PacketOutgoing  = 4 // Outgoing of any type
)
