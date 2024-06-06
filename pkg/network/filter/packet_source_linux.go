// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

//nolint:revive // TODO(NET) Fix revive linter
package filter

import (
	"fmt"
	"os"
	"reflect"
	"syscall"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/google/gopacket"
	"github.com/google/gopacket/afpacket"
	"github.com/google/gopacket/layers"
	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const telemetryModuleName = "network_tracer__filter"

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

	exit chan struct{}
}

type OptSnapLen int

// NewPacketSource creates an AFPacketSource using the provided BPF filter
func NewPacketSource(mbSize int, opts ...interface{}) (*AFPacketSource, error) {
	snapLen := 4096
	for _, opt := range opts {
		switch o := opt.(type) {
		case OptSnapLen:
			snapLen = int(o)
			if snapLen <= 0 || snapLen > 65536 {
				return nil, fmt.Errorf("snap len should be between 0 and 65536")
			}
		default:
			return nil, fmt.Errorf("unknown option %+v", opt)
		}
	}

	frameSize, blockSize, numBlocks, err := afpacketComputeSize(mbSize, snapLen, os.Getpagesize())
	if err != nil {
		return nil, fmt.Errorf("error computing mmap'ed buffer parameters: %w", err)
	}

	log.Debugf("creating tpacket source with frame_size=%d block_size=%d num_blocks=%d", frameSize, blockSize, numBlocks)
	rawSocket, err := afpacket.NewTPacket(
		afpacket.OptPollTimeout(time.Second),
		afpacket.OptFrameSize(frameSize),
		afpacket.OptBlockSize(blockSize),
		afpacket.OptNumBlocks(numBlocks),
		afpacket.OptAddPktType(true),
	)

	if err != nil {
		return nil, fmt.Errorf("error creating raw socket: %s", err)
	}

	ps := &AFPacketSource{
		TPacket: rawSocket,
		exit:    make(chan struct{}),
	}
	go ps.pollStats()

	return ps, nil
}

func (p *AFPacketSource) SetEbpf(filter *manager.Probe) {
	// The underlying socket file descriptor is private, hence the use of reflection
	// Point socket filter program to the RAW_SOCKET file descriptor
	// Note the filter attachment itself is triggered by the ebpf.Manager
	filter.SocketFD = int(reflect.ValueOf(p.TPacket).Elem().FieldByName("fd").Int())
}

func (p *AFPacketSource) SetBPF(filter []bpf.RawInstruction) error {
	return p.TPacket.SetBPF(filter)
}

// VisitPackets starts reading packets from the source
func (p *AFPacketSource) VisitPackets(exit <-chan struct{}, visit func([]byte, uint8, time.Time) error) error {
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

		//log.Tracef("packet on interface %d, pkt type %d", stats.InterfaceIndex, stats.AncillaryData[0].(afpacket.AncillaryPktType).Type)
		if err := visit(data, stats.AncillaryData[0].(afpacket.AncillaryPktType).Type, stats.Timestamp); err != nil {
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

// afpacketComputeSize computes the block_size and the num_blocks in such a way that the
// allocated mmap buffer is close to but smaller than target_size_mb.
// The restriction is that the block_size must be divisible by both the
// frame size and page size.
//
// See https://www.kernel.org/doc/Documentation/networking/packet_mmap.txt
func afpacketComputeSize(targetSizeMb, snaplen, pageSize int) (frameSize, blockSize, numBlocks int, err error) {
	frameSize = tpacketAlign(unix.TPACKET_HDRLEN) + tpacketAlign(snaplen)
	if frameSize <= pageSize {
		frameSize = int(nextPowerOf2(int64(frameSize)))
		if frameSize <= pageSize {
			blockSize = pageSize
		}
	} else {
		// align frameSize to pageSize
		frameSize = (frameSize + pageSize - 1) & ^(pageSize - 1)
		blockSize = frameSize
	}

	// convert to bytes from MB
	targetSize := targetSizeMb << 20
	numBlocks = targetSize / blockSize
	if numBlocks == 0 {
		return 0, 0, 0, fmt.Errorf("buffer size is too small")
	}

	blockSizeInc := blockSize
	for numBlocks > afpacket.DefaultNumBlocks {
		blockSize += blockSizeInc
		numBlocks = targetSize / blockSize
	}

	return frameSize, blockSize, numBlocks, nil
}

func tpacketAlign(x int) int {
	return (x + unix.TPACKET_ALIGNMENT - 1) & ^(unix.TPACKET_ALIGNMENT - 1)
}

func nextPowerOf2(v int64) int64 {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v |= v >> 32
	v++

	return v
}
