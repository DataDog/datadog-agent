// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf
// +build windows,npm linux_bpf

package dns

import (
	"sync"
	"time"

	"github.com/google/gopacket"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	dnsCacheExpirationPeriod = 1 * time.Minute
	dnsCacheSize             = 100000
)

var _ ReverseDNS = &socketFilterSnooper{}

// socketFilterSnooper is a DNS traffic snooper built on top of an eBPF SOCKET_FILTER
type socketFilterSnooper struct {
	// Telemetry
	decodingErrors *atomic.Int64
	truncatedPkts  *atomic.Int64

	// DNS telemetry, values calculated *till* the last tick in pollStats
	queries   *atomic.Int64
	successes *atomic.Int64
	errors    *atomic.Int64

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
		decodingErrors: atomic.NewInt64(0),
		truncatedPkts:  atomic.NewInt64(0),
		queries:        atomic.NewInt64(0),
		successes:      atomic.NewInt64(0),
		errors:         atomic.NewInt64(0),

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
func (s *socketFilterSnooper) Resolve(ips []util.Address) map[util.Address][]Hostname {
	return s.cache.Get(ips)
}

// GetDNSStats gets the latest Stats keyed by unique Key, and domain
func (s *socketFilterSnooper) GetDNSStats() StatsByKeyByNameByType {
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

	stats["decoding_errors"] = s.decodingErrors.Load()
	stats["truncated_packets"] = s.truncatedPkts.Load()
	stats["timestamp_micro_secs"] = time.Now().UnixNano() / 1000
	stats["queries"] = s.queries.Load()
	stats["successes"] = s.successes.Load()
	stats["errors"] = s.errors.Load()
	if s.statKeeper != nil {
		numStats, droppedStats := s.statKeeper.GetNumStats()
		stats["num_stats"] = int64(numStats)
		stats["dropped_stats"] = int64(droppedStats)
	}
	return stats
}

// Start starts the snooper (no-op currently)
func (s *socketFilterSnooper) Start() error {
	return nil // no-op as this is done in newSocketFilterSnooper above
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
			s.truncatedPkts.Inc()
		default:
			s.decodingErrors.Inc()
		}
		return nil
	}

	if s.statKeeper != nil && (s.collectLocalDNS || !pktInfo.key.ServerIP.IsLoopback()) {
		s.statKeeper.ProcessPacketInfo(pktInfo, ts)
	}

	if pktInfo.pktType == successfulResponse {
		s.cache.Add(t)
		s.successes.Inc()
	} else if pktInfo.pktType == failedResponse {
		s.errors.Inc()
	} else {
		s.queries.Inc()
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
		queries, lastQueries     int64
		successes, lastSuccesses int64
		errors, lastErrors       int64
	)
	for {
		select {
		case <-ticker.C:
			queries = s.queries.Load()
			successes = s.successes.Load()
			errors = s.errors.Load()
			log.Infof("DNS Stats. Queries :%d, Successes :%d, Errors: %d", queries-lastQueries, successes-lastSuccesses, errors-lastErrors)
			lastQueries = queries
			lastSuccesses = successes
			lastErrors = errors
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
