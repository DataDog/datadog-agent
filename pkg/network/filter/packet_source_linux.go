// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package filter exposes interfaces and implementations for packet capture
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

const (
	telemetryModuleName = "network_tracer__filter"
	defaultSnapLen      = 4096
)

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

// AFPacketInfo holds information about a packet
type AFPacketInfo struct {
	// PktType corresponds to sll_pkttype in the
	// sockaddr_ll struct; see packet(7)
	// https://man7.org/linux/man-pages/man7/packet.7.html
	PktType uint8
}

// OptSnapLen specifies the maximum length of the packet to read
//
// Defaults to 4096 bytes
type OptSnapLen int

// NewAFPacketSource creates an AFPacketSource using the provided BPF filter
func NewAFPacketSource(size int, opts ...interface{}) (*AFPacketSource, error) {
	snapLen := defaultSnapLen
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

	frameSize, blockSize, numBlocks, err := afpacketComputeSize(size, snapLen, os.Getpagesize())
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

// SetEbpf attaches an eBPF socket filter to the AFPacketSource
func (p *AFPacketSource) SetEbpf(filter *manager.Probe) error {
	// The underlying socket file descriptor is private, hence the use of reflection
	// Point socket filter program to the RAW_SOCKET file descriptor
	// Note the filter attachment itself is triggered by the ebpf.Manager
	f := reflect.ValueOf(p.TPacket).Elem().FieldByName("fd")
	if !f.IsValid() {
		return fmt.Errorf("could not find fd field in TPacket object")
	}

	if !f.CanInt() {
		return fmt.Errorf("fd TPacket field is not an int")
	}

	filter.SocketFD = int(f.Int())
	return nil
}

// SetBPF attaches a (classic) BPF socket filter to the AFPacketSource
func (p *AFPacketSource) SetBPF(filter []bpf.RawInstruction) error {
	return p.TPacket.SetBPF(filter)
}

type zeroCopyPacketReader interface {
	ZeroCopyReadPacketData() (data []byte, ci gopacket.CaptureInfo, err error)
}

// AFPacketVisitor is the callback that AFPacketSource will trigger for packets
// The data buffer is reused between calls, so be careful
type AFPacketVisitor = func(data []byte, info PacketInfo, t time.Time) error

func visitPackets(p zeroCopyPacketReader, exit <-chan struct{}, visit AFPacketVisitor) error {
	pktInfo := &AFPacketInfo{}
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

		for _, data := range stats.AncillaryData {
			// if addPktType = true, AncillaryData will contain an AncillaryPktType element;
			// however, it might not be the first element, so scan through.
			pktType, ok := data.(afpacket.AncillaryPktType)
			if ok {
				pktInfo.PktType = pktType.Type
			}
		}
		if err := visit(data, pktInfo, stats.Timestamp); err != nil {
			return err
		}
	}
}

// VisitPackets starts reading packets from the source
func (p *AFPacketSource) VisitPackets(exit <-chan struct{}, visit AFPacketVisitor) error {
	return visitPackets(p, exit, visit)
}

// LayerType is the gopacket.LayerType for this source
func (p *AFPacketSource) LayerType() gopacket.LayerType {
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
func afpacketComputeSize(targetSize, snaplen, pageSize int) (frameSize, blockSize, numBlocks int, err error) {
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

// nextPowerOf2 rounds up `v` to the next power of 2
//
// Taken from Hacker's Delight by Henry S. Warren, Jr.,
// https://en.wikipedia.org/wiki/Hacker%27s_Delight
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
