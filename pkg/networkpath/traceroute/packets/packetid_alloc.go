// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package packets

import (
	"math/rand/v2"
	"sync/atomic"
)

var curPacketID atomic.Uint32

// RandomizePacketIDBase randomizes the packet ID. Only used in tests to avoid the same
// packet ID being used every time.
func RandomizePacketIDBase() {
	curPacketID.Store(rand.Uint32())
}

// AllocPacketID allocates a new packet ID range, and returns the start of the range.
// This is used to avoid collisions when multiple traceroutes are running in parallel.
func AllocPacketID(maxTTL uint8) uint16 {
	maxTTL32 := uint32(maxTTL)
	// bump up the packetID by the range of the TTL
	// we need to subtract the maxTTL to get the start of the range
	next := curPacketID.Add(maxTTL32) - maxTTL32
	return uint16(next)
}
