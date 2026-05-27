// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && pcap && cgo

package com_datadoghq_remoteaction_pcap

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/capture"
)

// doCapture runs a real packet capture on Linux using the capture package.
// It returns the packet count, file size in bytes, and actual capture duration.
func doCapture(ctx context.Context, inputs RunCaptureInputs) (packetCount int, fileSizeBytes int64, actualDuration time.Duration, err error) {
	iface, err := resolveInterface(inputs.Interface)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("resolving network interface: %w", err)
	}

	tmpFile, err := os.CreateTemp(os.TempDir(), "dd-pcap-*.pcap")
	if err != nil {
		return 0, 0, 0, fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		os.Remove(tmpPath)
	}()

	cfg := capture.CaptureConfig{
		Filter:     inputs.BPFFilter,
		Iface:      iface,
		Output:     tmpFile,
		Duration:   time.Duration(inputs.DurationSecs) * time.Second,
		MaxPackets: uint64(inputs.MaxPackets),
		SnapLen:    uint32(inputs.SnapLen),
		Direction:  capture.DirectionBoth,
	}

	capturer, err := capture.NewCapturer(cfg)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("creating capturer: %w", err)
	}

	startTime := time.Now()

	if err = capturer.Start(ctx); err != nil {
		return 0, 0, 0, fmt.Errorf("starting capture: %w", err)
	}

	// Wait for the capture duration to elapse or for the context to be cancelled.
	// The drain loop inside the capturer already respects Duration and MaxPackets,
	// so we just need to wait for whichever fires first.
	select {
	case <-ctx.Done():
	case <-time.After(time.Duration(inputs.DurationSecs) * time.Second):
	}

	if stopErr := capturer.Stop(); stopErr != nil {
		// Best-effort: log but don't override any earlier error.
		_ = stopErr
	}

	stats := capturer.Stats()
	actualDuration = time.Since(startTime)

	// Flush and stat the temp file to get the written size.
	if flushErr := tmpFile.Sync(); flushErr != nil {
		return 0, 0, 0, fmt.Errorf("flushing temp file: %w", flushErr)
	}

	fi, err := tmpFile.Stat()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("stat temp file: %w", err)
	}

	return int(stats.PacketsCaptured), fi.Size(), actualDuration, nil
}

// resolveInterface returns the net.Interface to capture on.
// If ifaceName is non-empty it looks up that interface by name; otherwise it
// picks the first non-loopback, up interface (i.e. the default route interface
// heuristic).
func resolveInterface(ifaceName string) (*net.Interface, error) {
	if ifaceName != "" {
		iface, err := net.InterfaceByName(ifaceName)
		if err != nil {
			return nil, fmt.Errorf("interface %q not found: %w", ifaceName, err)
		}
		return iface, nil
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("listing network interfaces: %w", err)
	}

	for i := range ifaces {
		iface := &ifaces[i]
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		return iface, nil
	}

	return nil, fmt.Errorf("no suitable network interface found")
}
