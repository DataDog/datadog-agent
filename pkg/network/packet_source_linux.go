// +build linux_bpf

package network

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf/manager"
	"github.com/google/gopacket/afpacket"
)

var _ PacketSource = &packetSource{}

// packetSource provides a RAW_SOCKET attached to an eBPF SOCKET_FILTER
type packetSource struct {
	*afpacket.TPacket
	socketFilter *manager.Probe
	socketFD     int

	exit chan struct{}

	// telemetry
	polls     int64
	processed int64
	captured  int64
	dropped   int64
}

func NewPacketSource(filter *manager.Probe) (*packetSource, error) {
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

	// The underlying socket file descriptor is private, hence the use of reflection
	socketFD := int(reflect.ValueOf(rawSocket).Elem().FieldByName("fd").Int())

	// Attaches DNS socket filter to the RAW_SOCKET
	filter.SocketFD = socketFD
	if err := filter.Attach(); err != nil {
		return nil, fmt.Errorf("error attaching filter to socket: %s", err)
	}

	ps := &packetSource{
		TPacket:      rawSocket,
		socketFilter: filter,
		socketFD:     socketFD,
		exit:         make(chan struct{}),
	}
	go ps.pollStats()

	return ps, nil
}

func (p *packetSource) Stats() map[string]int64 {
	return map[string]int64{
		"socket_polls":      atomic.LoadInt64(&p.polls),
		"packets_processed": atomic.LoadInt64(&p.processed),
		"packets_captured":  atomic.LoadInt64(&p.captured),
		"packets_dropped":   atomic.LoadInt64(&p.dropped),
	}
}

func (p *packetSource) VisitPackets(exit <-chan struct{}, visit func([]byte, time.Time) error) error {
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

func (p *packetSource) Close() {
	close(p.exit)
	if err := p.socketFilter.Detach(); err != nil {
		log.Errorf("error detaching socket filter: %s", err)
	}

	p.TPacket.Close()
}

func (p *packetSource) pollStats() {
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

			atomic.AddInt64(&p.polls, sourceStats.Polls-prevPolls)
			atomic.AddInt64(&p.processed, sourceStats.Packets-prevProcessed)
			atomic.AddInt64(&p.captured, int64(socketStats.Packets())-prevCaptured)
			atomic.AddInt64(&p.dropped, int64(socketStats.Drops())-prevDropped)

			prevPolls = sourceStats.Polls
			prevProcessed = sourceStats.Packets
			prevCaptured = int64(socketStats.Packets())
			prevDropped = int64(socketStats.Drops())
		case <-p.exit:
			return
		}
	}
}
