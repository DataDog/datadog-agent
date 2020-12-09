// +build linux_bpf

package network

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf/manager"
	"github.com/google/gopacket/afpacket"
)

const (
	dnsCacheTTL              = 3 * time.Minute
	dnsCacheExpirationPeriod = 1 * time.Minute
	dnsCacheSize             = 100000
)

var _ ReverseDNS = &SocketFilterSnooper{}

// SocketFilterSnooper is a DNS traffic snooper built on top of an eBPF SOCKET_FILTER
type SocketFilterSnooper struct {
	source          *packetSource
	parser          *dnsParser
	cache           *reverseDNSCache
	statKeeper      *dnsStatKeeper
	exit            chan struct{}
	wg              sync.WaitGroup
	collectLocalDNS bool

	// cache translation object to avoid allocations
	translation *translation

	// packet telemetry
	captured       int64
	processed      int64
	dropped        int64
	polls          int64
	decodingErrors int64
	truncatedPkts  int64

	// DNS telemetry, values calculated *till* the last tick in pollStats
	queries   int64
	successes int64
	errors    int64
}

// NewSocketFilterSnooper returns a new SocketFilterSnooper
func NewSocketFilterSnooper(cfg *config.Config, filter *manager.Probe) (*SocketFilterSnooper, error) {
	var (
		packetSrc *packetSource
		srcErr    error
	)

	// Create the RAW_SOCKET inside the root network namespace
	nsErr := util.WithRootNS(cfg.ProcRoot, func() {
		packetSrc, srcErr = newPacketSource(filter)
	})
	if nsErr != nil {
		return nil, nsErr
	}
	if srcErr != nil {
		return nil, srcErr
	}

	cache := newReverseDNSCache(dnsCacheSize, dnsCacheTTL, dnsCacheExpirationPeriod)
	var statKeeper *dnsStatKeeper
	if cfg.CollectDNSStats {
		statKeeper = newDNSStatkeeper(cfg.DNSTimeout)
	}
	snooper := &SocketFilterSnooper{
		source:          packetSrc,
		parser:          newDNSParser(cfg.CollectDNSStats),
		cache:           cache,
		statKeeper:      statKeeper,
		translation:     new(translation),
		exit:            make(chan struct{}),
		collectLocalDNS: cfg.CollectLocalDNS,
	}

	// Start consuming packets
	snooper.wg.Add(1)
	go func() {
		snooper.pollPackets()
		snooper.wg.Done()
	}()

	// Start polling socket stats
	snooper.wg.Add(1)
	go func() {
		snooper.pollStats()
		snooper.wg.Done()
	}()

	return snooper, nil
}

// Resolve IPs to DNS addresses
func (s *SocketFilterSnooper) Resolve(connections []ConnectionStats) map[util.Address][]string {
	return s.cache.Get(connections, time.Now())
}

func (s *SocketFilterSnooper) GetDNSStats() map[dnsKey]dnsStats {
	if s.statKeeper == nil {
		return nil
	}
	return s.statKeeper.GetAndResetAllStats()
}

func (s *SocketFilterSnooper) GetStats() map[string]int64 {
	stats := s.cache.Stats()
	stats["socket_polls"] = atomic.LoadInt64(&s.polls)
	stats["packets_processed"] = atomic.LoadInt64(&s.processed)
	stats["packets_captured"] = atomic.LoadInt64(&s.captured)
	stats["packets_dropped"] = atomic.LoadInt64(&s.dropped)
	stats["decoding_errors"] = atomic.LoadInt64(&s.decodingErrors)
	stats["truncated_packets"] = atomic.LoadInt64(&s.truncatedPkts)
	stats["queries"] = atomic.LoadInt64(&s.queries)
	stats["successes"] = atomic.LoadInt64(&s.successes)
	stats["errors"] = atomic.LoadInt64(&s.errors)
	stats["timestamp_micro_secs"] = time.Now().UnixNano() / 1000
	return stats
}

// Close terminates the DNS traffic snooper as well as the underlying socket and the attached filter
func (s *SocketFilterSnooper) Close() {
	close(s.exit)
	s.wg.Wait()
	s.source.Close()
	s.cache.Close()
	if s.statKeeper != nil {
		s.statKeeper.Close()
	}
}

// processPacket retrieves DNS information from the received packet data and adds it to
// the reverse DNS cache. The underlying packet data can't be referenced after this method
// call since the underlying memory content gets invalidated by `afpacket`.
// The *translation is recycled and re-used in subsequent calls and it should not be accessed concurrently.
// The second parameter `ts` is the time when the packet was captured off the wire. This is used for latency calculation
// and much more reliable than calling time.Now() at the user layer.
func (s *SocketFilterSnooper) processPacket(data []byte, ts time.Time) {
	t := s.getCachedTranslation()
	pktInfo := dnsPacketInfo{}

	if err := s.parser.ParseInto(data, t, &pktInfo); err != nil {
		switch err {
		case errSkippedPayload: // no need to count or log cases where the packet is valid but has no relevant content
		case errTruncated:
			atomic.AddInt64(&s.truncatedPkts, 1)
		default:
			atomic.AddInt64(&s.decodingErrors, 1)
			log.Tracef("error decoding DNS payload: %v", err)
		}
		return
	}

	if s.statKeeper != nil && (s.collectLocalDNS || !pktInfo.key.serverIP.IsLoopback()) {
		s.statKeeper.ProcessPacketInfo(pktInfo, ts)
	}

	if pktInfo.pktType == SuccessfulResponse {
		s.cache.Add(t, time.Now())
		atomic.AddInt64(&s.successes, 1)
	} else if pktInfo.pktType == FailedResponse {
		atomic.AddInt64(&s.errors, 1)
	} else {
		atomic.AddInt64(&s.queries, 1)
	}
}

func (s *SocketFilterSnooper) pollPackets() {
	for {
		data, captureInfo, err := s.source.ZeroCopyReadPacketData()

		// Properly synchronizes termination process
		select {
		case <-s.exit:
			return
		default:
		}

		if err == nil {
			s.processPacket(data, captureInfo.Timestamp)
			continue
		}

		// Immediately retry for EAGAIN
		if err == syscall.EAGAIN {
			continue
		}

		// Sleep briefly and try again
		time.Sleep(5 * time.Millisecond)
	}
}

func (s *SocketFilterSnooper) pollStats() {
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
			sourceStats, _ := s.source.Stats()
			_, socketStats, err := s.source.SocketStats()
			if err != nil {
				log.Errorf("error polling socket stats: %s", err)
				continue
			}

			atomic.AddInt64(&s.polls, sourceStats.Polls-prevPolls)
			atomic.AddInt64(&s.processed, sourceStats.Packets-prevProcessed)
			atomic.AddInt64(&s.captured, int64(socketStats.Packets())-prevCaptured)
			atomic.AddInt64(&s.dropped, int64(socketStats.Drops())-prevDropped)

			prevPolls = sourceStats.Polls
			prevProcessed = sourceStats.Packets
			prevCaptured = int64(socketStats.Packets())
			prevDropped = int64(socketStats.Drops())
		case <-s.exit:
			return
		}
	}
}

func (s *SocketFilterSnooper) getCachedTranslation() *translation {
	t := s.translation

	// Recycle buffer if necessary
	if t.ips == nil || len(t.ips) > maxIPBufferSize {
		t.ips = make([]util.Address, 30)
	}
	t.ips = t.ips[:0]

	return t
}

// packetSource provides a RAW_SOCKET attached to an eBPF SOCKET_FILTER
type packetSource struct {
	*afpacket.TPacket
	socketFilter *manager.Probe
	socketFD     int
}

func newPacketSource(filter *manager.Probe) (*packetSource, error) {
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

	return &packetSource{
		TPacket:      rawSocket,
		socketFilter: filter,
		socketFD:     socketFD,
	}, nil
}

func (p *packetSource) Close() {
	if err := p.socketFilter.Detach(); err != nil {
		log.Errorf("error detaching socket filter: %s", err)
	}

	p.TPacket.Close()
}
