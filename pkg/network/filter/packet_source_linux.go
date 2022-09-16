// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package filter

import (
	"fmt"
	"reflect"
	"syscall"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/layers"
	"go.uber.org/atomic"
	"golang.org/x/net/bpf"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// AFPacketSource provides a RAW_SOCKET attached to an eBPF SOCKET_FILTER
type AFPacketSource struct {
	*afpacket.TPacket
	socketFilter *manager.Probe

	exit chan struct{}

	// telemetry
	polls     *atomic.Int64
	processed *atomic.Int64
	captured  *atomic.Int64
	dropped   *atomic.Int64
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
		polls:        atomic.NewInt64(0),
		processed:    atomic.NewInt64(0),
		captured:     atomic.NewInt64(0),
		dropped:      atomic.NewInt64(0),
	}
	go ps.pollStats()

	return ps, nil
}

// Stats returns statistics about the AFPacketSource
func (p *AFPacketSource) Stats() map[string]int64 {
	return map[string]int64{
		"socket_polls":      p.polls.Load(),
		"packets_processed": p.processed.Load(),
		"packets_captured":  p.captured.Load(),
		"packets_dropped":   p.dropped.Load(),
	}
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

			p.polls.Add(sourceStats.Polls - prevPolls)
			p.processed.Add(sourceStats.Packets - prevProcessed)
			p.captured.Add(int64(socketStats.Packets()) - prevCaptured)
			p.dropped.Add(int64(socketStats.Drops()) - prevDropped)

			prevPolls = sourceStats.Polls
			prevProcessed = sourceStats.Packets
			prevCaptured = int64(socketStats.Packets())
			prevDropped = int64(socketStats.Drops())
		case <-p.exit:
			return
		}
	}
}
