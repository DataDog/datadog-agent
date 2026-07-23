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

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	gopcap "github.com/google/gopacket/pcap"
	"github.com/google/gopacket/pcapgo"
)

func doCapture(ctx context.Context, inputs RunCaptureInputs) (packetCount int, fileSizeBytes int64, actualDuration time.Duration, pcapPath string, err error) {
	ifaceName, err := resolveInterfaceName(inputs.Interface)
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("resolving network interface: %w", err)
	}

	handle, err := gopcap.OpenLive(ifaceName, int32(inputs.SnapLen), true, 100*time.Millisecond)
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("opening capture handle: %w", err)
	}
	defer handle.Close()

	if err = handle.SetBPFFilter(inputs.BPFFilter); err != nil {
		return 0, 0, 0, "", fmt.Errorf("setting BPF filter %q: %w", inputs.BPFFilter, err)
	}

	tmpFile, err := os.CreateTemp(os.TempDir(), "dd-pcap-*.pcap")
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	writer := pcapgo.NewWriter(tmpFile)
	if err = writer.WriteFileHeader(uint32(inputs.SnapLen), layers.LinkTypeEthernet); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return 0, 0, 0, "", fmt.Errorf("writing pcap header: %w", err)
	}

	deadline := time.Duration(inputs.DurationSecs) * time.Second
	captureCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	packetSource.Lazy = true
	packetSource.NoCopy = true

	startTime := time.Now()
	count := 0

	for {
		select {
		case <-captureCtx.Done():
			goto done
		default:
		}

		packet, err := packetSource.NextPacket()
		if err != nil {
			if captureCtx.Err() != nil {
				break
			}
			continue
		}

		ci := packet.Metadata().CaptureInfo
		if writeErr := writer.WritePacket(ci, packet.Data()); writeErr != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return 0, 0, 0, "", fmt.Errorf("writing packet: %w", writeErr)
		}

		count++
		if count >= inputs.MaxPackets {
			break
		}
	}

done:
	actualDuration = time.Since(startTime)

	if err = tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return 0, 0, 0, "", fmt.Errorf("flushing pcap file: %w", err)
	}

	fi, err := tmpFile.Stat()
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return 0, 0, 0, "", fmt.Errorf("stat pcap file: %w", err)
	}

	if err = tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return 0, 0, 0, "", fmt.Errorf("closing pcap file: %w", err)
	}

	return count, fi.Size(), actualDuration, tmpPath, nil
}

func resolveInterfaceName(ifaceName string) (string, error) {
	if ifaceName != "" {
		_, err := net.InterfaceByName(ifaceName)
		if err != nil {
			return "", fmt.Errorf("interface %q not found: %w", ifaceName, err)
		}
		return ifaceName, nil
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("listing network interfaces: %w", err)
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		return iface.Name, nil
	}

	return "", fmt.Errorf("no suitable network interface found")
}
