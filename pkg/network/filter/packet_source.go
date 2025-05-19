// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package filter exposes interfaces and implementations for packet capture
package filter

import (
	"time"

	"github.com/google/gopacket"
)

// PacketInfo holds OS dependent packet information
// about a packet
type PacketInfo interface{}

// PacketSource reads raw packet data
type PacketSource interface {
	// VisitPackets reads all new raw packets that are available, invoking the given callback for each packet.
	// If no packet is available, VisitPacket blocks until OptPollTimeout and returns.
	// The format of the packet is dependent on the implementation of PacketSource -- i.e. it may be an ethernet frame, or a IP frame.
	// The data buffer is reused between invocations of VisitPacket and thus should not be pointed to.
	// If the PacketSource is closed, VisitPackets will stop reading.
	VisitPackets(visitor func(data []byte, info PacketInfo, timestamp time.Time) error) error

	// LayerType returns the type of packet this source reads
	LayerType() gopacket.LayerType

	// Close closes the packet source. This will cancel VisitPackets if it is currently polling.
	// Close() will not return until after VisitPackets has been canceled/returned.
	Close()
}
