// +build linux_bpf

package ebpf

import (
	"fmt"
	"io"
	"net"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/gopacket/afpacket"
	bpflib "github.com/iovisor/gobpf/elf"
)

const (
	dnsCacheTTL              = 3 * time.Minute
	dnsCacheExpirationPeriod = 1 * time.Minute
	dnsCacheSize             = 100000
	packetBufferSize         = 100
)

var _ ReverseDNS = &SocketFilterSnooper{}

// SocketFilterSnooper is a DNS traffic snooper built on top of an eBPF SOCKET_FILTER
type SocketFilterSnooper struct {
	parser       *dnsParser
	source       *afpacket.TPacket
	socketFilter *bpflib.SocketFilter
	socketFD     int
	cache        *reverseDNSCache
	exit         chan struct{}
	wg           sync.WaitGroup

	// telemetry
	packets        int64
	polls          int64
	decodingErrors int64
}

// NewSocketFilterSnooper returns a new SocketFilterSnooper
func NewSocketFilterSnooper(filter *bpflib.SocketFilter) (*SocketFilterSnooper, error) {
	packetSrc, err := afpacket.NewTPacket(
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
	socketFD := int(reflect.ValueOf(packetSrc).Elem().FieldByName("fd").Int())
	if err := bpflib.AttachSocketFilter(filter, socketFD); err != nil {
		return nil, fmt.Errorf("error attaching filter to socket: %s", err)
	}

	cache := newReverseDNSCache(dnsCacheSize, dnsCacheTTL, dnsCacheExpirationPeriod)
	snooper := &SocketFilterSnooper{
		parser:       newDNSParser(),
		source:       packetSrc,
		socketFilter: filter,
		socketFD:     socketFD,
		cache:        cache,
		exit:         make(chan struct{}),
	}

	// Start consuming packets
	snooper.wg.Add(1)
	go func() {
		snooper.poll()
		snooper.wg.Done()
	}()

	return snooper, nil
}

// Resolve IPs to Names
func (s *SocketFilterSnooper) Resolve(connections []ConnectionStats) map[util.Address][]string {
	return s.cache.Get(connections, time.Now())
}

func (s *SocketFilterSnooper) GetStats() map[string]int64 {
	socketStats, _ := s.source.Stats()
	prevPolls := atomic.SwapInt64(&s.polls, socketStats.Polls)
	prevPackets := atomic.SwapInt64(&s.packets, socketStats.Packets)

	stats := s.cache.Stats()
	stats["socket_polls"] = socketStats.Polls - prevPolls
	stats["packets_captured"] = socketStats.Packets - prevPackets
	stats["decoding_errors"] = atomic.SwapInt64(&s.decodingErrors, 0)

	return stats
}

// Close terminates the DNS traffic snooper as well as the underlying socket and the attached filter
func (s *SocketFilterSnooper) Close() {
	close(s.exit)
	s.wg.Wait()

	if err := bpflib.DetachSocketFilter(s.socketFilter, s.socketFD); err != nil {
		log.Errorf("error detaching socket filter: %s", err)
	}

	s.source.Close()
	s.cache.Close()
}

// processPacket retrieves DNS information from the received packet data and adds it to
// the reverse DNS cache. The underlying packet data can't be referenced after this method
// call since gopacket re-uses it.
func (s *SocketFilterSnooper) processPacket(data []byte) {
	translation := s.parser.Parse(data)
	if translation == nil {
		atomic.AddInt64(&s.decodingErrors, 1)
		return
	}

	s.cache.Add(translation, time.Now())
}

func (s *SocketFilterSnooper) poll() {
	for {
		data, _, err := s.source.ZeroCopyReadPacketData()
		if err == nil {
			s.processPacket(data)
			continue
		}

		// Properly synchronizes termination process
		select {
		case <-s.exit:
			return
		default:
		}

		// Immediately retry for temporary network errors
		if nerr, ok := err.(net.Error); ok && nerr.Temporary() {
			continue
		}

		// Immediately retry for EAGAIN
		if err == syscall.EAGAIN {
			continue
		}

		// Immediately break for known unrecoverable errors
		if err == io.EOF || err == io.ErrUnexpectedEOF || err == io.ErrNoProgress ||
			err == io.ErrClosedPipe || err == io.ErrShortBuffer || err == syscall.EBADF ||
			strings.Contains(err.Error(), "use of closed file") {
			return
		}

		// Sleep briefly and try again
		time.Sleep(5 * time.Millisecond)
	}
}
