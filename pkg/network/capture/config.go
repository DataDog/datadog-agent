// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && pcap && cgo

// Package capture provides a remote packet capture (PCAP) module for the Datadog Agent.
// It opens a libpcap live capture, trims every packet to its protocol headers,
// and writes the result in PCAP file format.
package capture

import (
	"context"
	"io"
	"time"
)

const (
	// anyInterface is libpcap's pseudo-device that captures on every interface.
	anyInterface = "any"
	// maxSnapLen is both the default and the ceiling for the number of bytes
	// libpcap copies per packet. It covers the headers of common packet shapes,
	// including IPv6 SYNs on VLAN networks and IPv4 in VXLAN overlays.
	maxSnapLen = 128
	// defaultBufferSize is the libpcap kernel buffer size (8 MiB).
	defaultBufferSize = 8 * 1024 * 1024
	// readTimeout bounds how long a read blocks, so the drain loop can observe
	// stop conditions on an idle interface.
	readTimeout = 200 * time.Millisecond
)

// CaptureConfig holds all configuration needed to create a Capturer.
type CaptureConfig struct {
	// Filter is a tcpdump-style BPF filter expression (empty string = capture all).
	Filter string
	// Interface is the network interface to capture on. Empty means every
	// interface (libpcap's "any" device).
	Interface string
	// Output receives the PCAP-formatted capture stream.
	Output io.Writer
	// Duration is the maximum capture duration. 0 means no limit.
	Duration time.Duration
	// MaxPackets is the maximum number of packets to capture. 0 means no limit.
	MaxPackets uint64
	// MaxBytes is the maximum size of the produced PCAP file, in bytes. 0 means
	// no limit.
	//
	// This is a hard cap, not a threshold: the file never exceeds MaxBytes. A
	// packet that would push it over is not written and the capture stops
	// instead, so the last packet in the file is always complete. Accounting
	// covers the real on-disk size — the 24-byte global header plus a 16-byte
	// record header per packet — because the caller's budget is the size of the
	// artefact it has to upload and store, not the packet bytes alone.
	//
	// One exception: the 24-byte global header is written before the drain loop
	// starts, so it is a floor. A MaxBytes below it produces a 24-byte file, and
	// any MaxBytes too small for the header plus one record produces a valid but
	// empty capture. Real budgets are orders of magnitude larger; the property
	// worth relying on is that the output is always a readable pcap.
	MaxBytes uint64
	// SnapLen is the maximum number of bytes libpcap copies per packet. 0 and
	// any value above maxSnapLen mean maxSnapLen. Every packet is additionally
	// trimmed to its protocol headers before it is written (see headerLen), so
	// SnapLen bounds memory, not output.
	SnapLen uint32
}

// applyDefaults fills in zero-value fields with their defaults and enforces
// the snap length ceiling.
func (c *CaptureConfig) applyDefaults() {
	if c.Interface == "" {
		c.Interface = anyInterface
	}
	if c.SnapLen == 0 || c.SnapLen > maxSnapLen {
		c.SnapLen = maxSnapLen
	}
}

// RawPacket holds a single captured packet with metadata.
type RawPacket struct {
	// Timestamp is when the packet was captured.
	Timestamp time.Time
	// Data is the captured packet bytes (the protocol headers of the original).
	Data []byte
	// OrigLen is the original on-wire length of the packet before any truncation.
	OrigLen uint32
}

// CaptureStats is a thread-safe snapshot of capture statistics.
type CaptureStats struct {
	// PacketsCaptured is the total number of packets written to Output.
	PacketsCaptured uint64
	// PacketsDropped is the number of packets the kernel dropped because the
	// capture buffer was full, as reported by libpcap.
	PacketsDropped uint64
	// BytesCaptured is the total number of packet bytes written to Output.
	BytesCaptured uint64
	// HeadersTruncated is the number of packets whose headers did not fit in
	// SnapLen, so the written packet stops at the last complete header.
	HeadersTruncated uint64
	// StartTime is when Start was called.
	StartTime time.Time
	// EndTime is when Stop was called (zero if still running).
	EndTime time.Time
	// Errors is the number of non-fatal errors encountered during capture.
	Errors uint64
}

// Capturer is the primary interface for packet capture.
type Capturer interface {
	// Start begins capture: opens the libpcap handle, writes the PCAP global
	// header to Output, and starts reading packets in the background. It
	// returns immediately; capture runs until Stop is called or the context is
	// cancelled.
	Start(ctx context.Context) error

	// Stop ends the capture, closes the libpcap handle, finalises statistics,
	// and returns. It is safe to call Stop multiple times.
	Stop() error

	// Done is closed once the capture has ended on its own — on Duration,
	// MaxPackets, MaxBytes, context cancellation or end of input — or after
	// Stop. It is never closed if Start failed.
	Done() <-chan struct{}

	// Stats returns a point-in-time snapshot of capture statistics. It is safe
	// to call concurrently with Start, Stop, and other Stats calls.
	Stats() CaptureStats
}

// NewCapturer creates a new Capturer from the provided configuration.
// It validates the configuration and the BPF filter syntax, but does not open
// a capture until Start is called.
func NewCapturer(cfg CaptureConfig) (Capturer, error) {
	return newCapturer(cfg)
}
