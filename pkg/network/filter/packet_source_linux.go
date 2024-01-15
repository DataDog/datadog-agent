// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

//nolint:revive // TODO(NET) Fix revive linter
package filter

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"reflect"
	"syscall"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/layers"
	"golang.org/x/net/bpf"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const telemetryModuleName = "network_tracer__dns"

// Telemetry
var packetSourceTelemetry = struct {
	polls     *telemetry.StatCounterWrapper
	processed *telemetry.StatCounterWrapper
	captured  *telemetry.StatCounterWrapper
	dropped   *telemetry.StatCounterWrapper
}{
	telemetry.NewStatCounterWrapper(telemetryModuleName, "polled_packets", []string{}, "Counter measuring the number of polled packets"),
	telemetry.NewStatCounterWrapper(telemetryModuleName, "processed_packets", []string{}, "Counter measuring the number of processed packets"),
	telemetry.NewStatCounterWrapper(telemetryModuleName, "captured_packets", []string{}, "Counter measuring the number of captured packets"),
	telemetry.NewStatCounterWrapper(telemetryModuleName, "dropped_packets", []string{}, "Counter measuring the number of dropped packets"),
}

// AFPacketSource provides a RAW_SOCKET attached to an eBPF SOCKET_FILTER
type AFPacketSource struct {
	*afpacket.TPacket
	socketFilter *manager.Probe

	exit chan struct{}
}

// NewPacketSource creates an AFPacketSource using the provided BPF filter
func NewPacketSource(filter *manager.Probe, bpfFilter []bpf.RawInstruction) (*AFPacketSource, error) {
	rawSocket, err := afpacket.NewTPacket(
		afpacket.OptPollTimeout(1*time.Second),
		// This setup will require ~4Mb that is mmap'd into the process virtual space
		// More information here: https://www.kernel.org/doc/Documentation/networking/packet_mmap.txt
		afpacket.OptFrameSize(4096),
		afpacket.OptBlockSize(4096*128),
		afpacket.OptNumBlocks(8),
	)
	if err != nil {
		return nil, fmt.Errorf("error creating raw socket: %s", err)
	}

	if filter != nil {
		// The underlying socket file descriptor is private, hence the use of reflection
		// Point socket filter program to the RAW_SOCKET file descriptor
		// Note the filter attachment itself is triggered by the ebpf.Manager
		filter.SocketFD = int(reflect.ValueOf(rawSocket).Elem().FieldByName("fd").Int())
	} else {
		err = rawSocket.SetBPF(bpfFilter)
		if err != nil {
			return nil, fmt.Errorf("error setting classic bpf filter: %w", err)
		}
	}

	ps := &AFPacketSource{
		TPacket:      rawSocket,
		socketFilter: filter,
		exit:         make(chan struct{}),
	}
	go ps.pollStats()

	return ps, nil
}

// VisitPackets starts reading packets from the source
func (p *AFPacketSource) VisitPackets(exit <-chan struct{}, visit func([]byte, time.Time) error) error {
	for {
		// allow the read loop to be prematurely interrupted
		select {
		case <-exit:
			return nil
		default:
		}

		data, stats, err := p.ZeroCopyReadPacketData()

		// Immediately retry for EAGAIN
		if err == syscall.EAGAIN {
			continue
		}

		if err == afpacket.ErrTimeout {
			return nil
		}

		if err != nil {
			return err
		}

		if err := visit(data, stats.Timestamp); err != nil {
			return err
		}
	}
}

// PacketType is the gopacket.LayerType for this source
func (p *AFPacketSource) PacketType() gopacket.LayerType {
	return layers.LayerTypeEthernet
}

// Close stops packet reading
func (p *AFPacketSource) Close() {
	close(p.exit)
	p.TPacket.Close()
}

func (p *AFPacketSource) pollStats() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var (
		prevPolls     int64
		prevProcessed int64
		prevCaptured  int64
		prevDropped   int64
	)

	for {
		select {
		case <-ticker.C:
			sourceStats, _ := p.TPacket.Stats()            // off TPacket
			_, socketStats, err := p.TPacket.SocketStats() // off TPacket
			if err != nil {
				log.Errorf("error polling socket stats: %s", err)
				continue
			}

			packetSourceTelemetry.polls.Add(sourceStats.Polls - prevPolls)
			packetSourceTelemetry.processed.Add(sourceStats.Packets - prevProcessed)
			packetSourceTelemetry.captured.Add(int64(socketStats.Packets()) - prevCaptured)
			packetSourceTelemetry.dropped.Add(int64(socketStats.Drops()) - prevDropped)

			prevPolls = sourceStats.Polls
			prevProcessed = sourceStats.Packets
			prevCaptured = int64(socketStats.Packets())
			prevDropped = int64(socketStats.Drops())
		case <-p.exit:
			return
		}
	}
}
