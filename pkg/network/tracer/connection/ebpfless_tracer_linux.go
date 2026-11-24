// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && bpf

package connection

import (
	"errors"
	"fmt"

	"github.com/google/gopacket/afpacket"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
)

// nowNanoseconds returns a monotonic timestamp comparable to eBPF ktime.
func nowNanoseconds() (int64, error) {
	return ddebpf.NowNanoseconds()
}

const (
	// the segment length to read on Linux
	// mac header (with vlan) + ip header + tcp header
	segmentLen = 18 + 60 + 60
)

// createPacketSource creates a Linux-specific AFPacket packet source
func createPacketSource(_ *config.Config) (filter.PacketSource, error) {
	packetSrc, err := filter.NewAFPacketSource(
		8<<20, // 8 MB total space
		filter.OptSnapLen(segmentLen))
	if err != nil {
		return nil, fmt.Errorf("error creating AFPacket source: %w", err)
	}
	return packetSrc, nil
}

// isPlatformFatalPacketSourceError reports whether err is a fatal AF_PACKET error.
// afpacket.ErrPoll means poll reported POLLERR, POLLHUP or POLLNVAL on the packet socket
// or on the source's exit pipe. Nothing reads the socket's pending error, so a POLLERR
// stays set, and POLLNVAL or POLLHUP means a descriptor is closed: retrying cannot recover.
func isPlatformFatalPacketSourceError(err error) bool {
	return errors.Is(err, afpacket.ErrPoll)
}
