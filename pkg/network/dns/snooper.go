//+build windows linux_bpf

package dns

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/gopacket"
)

const (
	dnsCacheExpirationPeriod = 1 * time.Minute
	dnsCacheSize             = 100000
)

var _ ReverseDNS = &socketFilterSnooper{}

// socketFilterSnooper is a DNS traffic snooper built on top of an eBPF SOCKET_FILTER
type socketFilterSnooper struct {
	// Telemetry is at the beginning of the struct to keep all fields 64-bit aligned.
	// see https://staticcheck.io/docs/checks#SA1027
	decodingErrors int64
	truncatedPkts  int64

	// DNS telemetry, values calculated *till* the last tick in pollStats
	queries   int64
	successes int64
	errors    int64

	source          packetSource
	parser          *dnsParser
	cache           *reverseDNSCache
	statKeeper      *dnsStatKeeper
	exit            chan struct{}
	wg              sync.WaitGroup
	collectLocalDNS bool

	// cache translation object to avoid allocations
	translation *translation
}

// packetSource reads raw packet data
type packetSource interface {
	// VisitPackets reads all new raw packets that are available, invoking the given callback for each packet.
	// If no packet is available, VisitPacket returns immediately.
	// The format of the packet is dependent on the implementation of packetSource -- i.e. it may be an ethernet frame, or a IP frame.
	// The data buffer is reused between invocations of VisitPacket and thus should not be pointed to.
	// If the cancel channel is closed, VisitPackets will stop reading.
	VisitPackets(cancel <-chan struct{}, visitor func(data []byte, timestamp time.Time) error) error

	// Stats returns a map of counters, meant to be reported as telemetry
	Stats() map[string]int64

	// PacketType returns the type of packet this source reads
	PacketType() gopacket.LayerType

	// Close closes the packet source
	Close()
}

// newSocketFilterSnooper returns a new socketFilterSnooper
func newSocketFilterSnooper(cfg *config.Config, source packetSource) (*socketFilterSnooper, error) {
	cache := newReverseDNSCache(dnsCacheSize, dnsCacheExpirationPeriod)
	var statKeeper *dnsStatKeeper
	if cfg.CollectDNSStats {
		statKeeper = newDNSStatkeeper(cfg.DNSTimeout, cfg.MaxDNSStats)
		log.Infof("DNS Stats Collection has been enabled. Maximum number of stats objects: %d", cfg.MaxDNSStats)
		if cfg.CollectDNSDomains {
			log.Infof("DNS domain collection has been enabled")
		}
	} else {
		log.Infof("DNS Stats Collection has been disabled.")
	}
	snooper := &socketFilterSnooper{
		source:          source,
		parser:          newDNSParser(source.PacketType(), cfg),
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

	// Start logging DNS stats
	snooper.wg.Add(1)
	go func() {
		snooper.logDNSStats()
		snooper.wg.Done()
	}()
	return snooper, nil
}

// Resolve IPs to DNS addresses
func (s *socketFilterSnooper) Resolve(ips []util.Address) map[util.Address][]string {
	return s.cache.Get(ips)
}

// GetDNSStats gets the latest Stats keyed by unique Key, and domain
func (s *socketFilterSnooper) GetDNSStats() map[Key]map[string]map[QueryType]Stats {
	if s.statKeeper == nil {
		return nil
	}
	return s.statKeeper.GetAndResetAllStats()
}

// GetStats returns stats for use with telemetry
func (s *socketFilterSnooper) GetStats() map[string]int64 {
	stats := s.cache.Stats()

	for key, value := range s.source.Stats() {
		stats[key] = value
	}

	stats["decoding_errors"] = atomic.LoadInt64(&s.decodingErrors)
	stats["truncated_packets"] = atomic.LoadInt64(&s.truncatedPkts)
	stats["timestamp_micro_secs"] = time.Now().UnixNano() / 1000
	stats["queries"] = atomic.LoadInt64(&s.queries)
	stats["successes"] = atomic.LoadInt64(&s.successes)
	stats["errors"] = atomic.LoadInt64(&s.errors)
	if s.statKeeper != nil {
		numStats, droppedStats := s.statKeeper.GetNumStats()
		stats["num_stats"] = int64(numStats)
		stats["dropped_stats"] = int64(droppedStats)
	}
	return stats
}

// Close terminates the DNS traffic snooper as well as the underlying socket and the attached filter
func (s *socketFilterSnooper) Close() {
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
func (s *socketFilterSnooper) processPacket(data []byte, ts time.Time) error {
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
		return nil
	}

	if s.statKeeper != nil && (s.collectLocalDNS || !pktInfo.key.ServerIP.IsLoopback()) {
		s.statKeeper.ProcessPacketInfo(pktInfo, ts)
	}

	if pktInfo.pktType == successfulResponse {
		s.cache.Add(t)
		atomic.AddInt64(&s.successes, 1)
	} else if pktInfo.pktType == failedResponse {
		atomic.AddInt64(&s.errors, 1)
	} else {
		atomic.AddInt64(&s.queries, 1)
	}

	return nil
}

func (s *socketFilterSnooper) pollPackets() {
	for {
		err := s.source.VisitPackets(s.exit, s.processPacket)

		if err != nil {
			log.Warnf("error reading packet: %s", err)
		}

		// Properly synchronizes termination process
		select {
		case <-s.exit:
			return
		default:
		}

		// Sleep briefly and try again
		time.Sleep(5 * time.Millisecond)
	}
}

func (s *socketFilterSnooper) logDNSStats() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	var (
		queries   int64
		successes int64
		errors    int64
	)
	for {
		select {
		case <-ticker.C:
			queries = atomic.SwapInt64(&s.queries, 0)
			successes = atomic.SwapInt64(&s.successes, 0)
			errors = atomic.SwapInt64(&s.errors, 0)
			log.Infof("DNS Stats. Queries :%d, Successes :%d, Errors: %d", queries, successes, errors)
		case <-s.exit:
			return
		}
	}
}

func (s *socketFilterSnooper) getCachedTranslation() *translation {
	t := s.translation

	// Recycle buffer if necessary
	if t.ips == nil || len(t.ips) > maxIPBufferSize {
		t.ips = make(map[util.Address]time.Time, 30)
	}
	for k := range t.ips {
		delete(t.ips, k)
	}
	return t
}
