// +build linux_bpf

package filter

import (
	"fmt"
	"reflect"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf/manager"
	"github.com/google/gopacket/afpacket"
)

// PacketSource provides a RAW_SOCKET attached to an eBPF SOCKET_FILTER
type PacketSource struct {
	*afpacket.TPacket
	socketFilter *manager.Probe
	socketFD     int
}

func NewPacketSource(filter *manager.Probe) (*PacketSource, error) {
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

	return &PacketSource{
		TPacket:      rawSocket,
		socketFilter: filter,
		socketFD:     socketFD,
	}, nil
}

func (p *PacketSource) Close() {
	if err := p.socketFilter.Detach(); err != nil {
		log.Errorf("error detaching socket filter: %s", err)
	}

	p.TPacket.Close()
}
