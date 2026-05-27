// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && pcap && cgo

// Package capture provides a remote packet capture (PCAP) module for the Datadog Agent.
// It attaches a TC eBPF hook to a network interface, compiles a BPF filter to eBPF
// bytecode at runtime using cbpfc, and writes captured packets in PCAP file format.
package capture

import (
	"context"
	"io"
	"net"
	"time"
)

const (
	// defaultSnapLen is the default number of bytes captured per packet.
	defaultSnapLen = 65535
	// defaultRingBufferSize is the default eBPF ring buffer size (8 MiB).
	defaultRingBufferSize = 8 * 1024 * 1024
	// progPrefix is the eBPF program name prefix used for identification and cleanup.
	progPrefix = "dd_pcap_"
)

// CaptureDirection controls whether ingress, egress, or both directions are captured.
type CaptureDirection int

const (
	// DirectionBoth captures packets in both directions.
	DirectionBoth CaptureDirection = iota
	// DirectionIngress captures only incoming packets.
	DirectionIngress
	// DirectionEgress captures only outgoing packets.
	DirectionEgress
)

// CaptureConfig holds all configuration needed to create a Capturer.
type CaptureConfig struct {
	// Filter is a tcpdump-style BPF filter expression (empty string = capture all).
	Filter string
	// Iface is the network interface to capture on.
	Iface *net.Interface
	// Output receives the PCAP-formatted capture stream.
	Output io.Writer
	// Duration is the maximum capture duration. 0 means no limit.
	Duration time.Duration
	// MaxPackets is the maximum number of packets to capture. 0 means no limit.
	MaxPackets uint64
	// SnapLen is the maximum number of bytes to capture per packet. 0 defaults to 65535.
	SnapLen uint32
	// RingBufferSize is the eBPF ring buffer size in bytes. 0 defaults to 8 MiB.
	RingBufferSize int
	// Direction controls which traffic direction is captured.
	Direction CaptureDirection
}

// applyDefaults fills in zero-value fields with their defaults.
func (c *CaptureConfig) applyDefaults() {
	if c.SnapLen == 0 {
		c.SnapLen = defaultSnapLen
	}
	if c.RingBufferSize == 0 {
		c.RingBufferSize = defaultRingBufferSize
	}
}

// RawPacket holds a single captured packet with metadata.
type RawPacket struct {
	// Timestamp is when the packet was captured.
	Timestamp time.Time
	// Data is the captured packet bytes (up to SnapLen bytes of the original).
	Data []byte
	// OrigLen is the original on-wire length of the packet before any truncation.
	OrigLen uint32
	// IfIndex is the interface index on which the packet was captured.
	IfIndex uint32
	// Ingress is true if the packet was received (ingress), false if transmitted (egress).
	Ingress bool
}

// CaptureStats is a thread-safe snapshot of capture statistics.
type CaptureStats struct {
	// PacketsCaptured is the total number of packets written to Output.
	PacketsCaptured uint64
	// PacketsDropped is the number of packets dropped due to ring buffer overflow.
	PacketsDropped uint64
	// BytesCaptured is the total number of packet payload bytes captured.
	BytesCaptured uint64
	// StartTime is when Start was called.
	StartTime time.Time
	// EndTime is when Stop was called (zero if still running).
	EndTime time.Time
	// Errors is the number of non-fatal errors encountered during capture.
	Errors uint64
}

// Capturer is the primary interface for packet capture.
type Capturer interface {
	// Start begins capture: writes the PCAP global header to Output, attaches the TC
	// eBPF hook to the target interface, and starts draining the ring buffer in the
	// background. It returns immediately; capture runs until Stop is called or the
	// context is cancelled.
	Start(ctx context.Context) error

	// Stop detaches the TC hook, drains any remaining packets from the ring buffer,
	// finalises statistics, and returns. It is safe to call Stop multiple times.
	Stop() error

	// Stats returns a point-in-time snapshot of capture statistics. It is safe
	// to call concurrently with Start, Stop, and other Stats calls.
	Stats() CaptureStats
}

// NewCapturer creates a new Capturer from the provided configuration.
// It validates the configuration and compiles the BPF filter to eBPF bytecode,
// but does not attach any hooks until Start is called.
func NewCapturer(cfg CaptureConfig) (Capturer, error) {
	return newCapturer(cfg)
}
