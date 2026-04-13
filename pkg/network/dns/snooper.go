// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf || darwin

package dns

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	dnsCacheExpirationPeriod = 1 * time.Minute
	dnsCacheSize             = 100000
	dnsModuleName            = "network_tracer__dns"
)

// snooperTelemetry holds DNS packet processing counters shared across all DNS monitor implementations.
var snooperTelemetry = struct {
	decodingErrors *telemetry.StatCounterWrapper
	truncatedPkts  *telemetry.StatCounterWrapper
	queries        *telemetry.StatCounterWrapper
	successes      *telemetry.StatCounterWrapper
	errors         *telemetry.StatCounterWrapper
}{
	telemetry.NewStatCounterWrapper(dnsModuleName, "decoding_errors", []string{}, "Counter measuring the number of decoding errors while processing packets"),
	telemetry.NewStatCounterWrapper(dnsModuleName, "truncated_pkts", []string{}, "Counter measuring the number of truncated packets while processing"),
	// DNS telemetry, values calculated *till* the last tick in pollStats
	telemetry.NewStatCounterWrapper(dnsModuleName, "queries", []string{}, "Counter measuring the number of packets that are DNS queries in processed packets"),
	telemetry.NewStatCounterWrapper(dnsModuleName, "successes", []string{}, "Counter measuring the number of successful DNS responses in processed packets"),
	telemetry.NewStatCounterWrapper(dnsModuleName, "errors", []string{}, "Counter measuring the number of failed DNS responses in processed packets"),
}

var _ ReverseDNS = &socketFilterSnooper{}

// socketFilterSnooper is a DNS traffic snooper built on top of an eBPF SOCKET_FILTER
type socketFilterSnooper struct {
	source          filter.PacketSource
	parser          *dnsParser
	cache           *reverseDNSCache
	statKeeper      *dnsStatKeeper
	exit            chan struct{}
	wg              sync.WaitGroup
	collectLocalDNS bool
	once            sync.Once

	// cache translation object to avoid allocations
	translation *translation

	// parserFor returns the DNS parser to use for a given packet. Defaults to
	// returning s.parser (ignoring the PacketInfo). On darwin this is overridden
	// to select between ethernet and loopback parsers based on the packet's
	// link-layer type. It is always invoked from the single pollPackets
	// goroutine, so it may safely access shared state without synchronization.
	parserFor func(info filter.PacketInfo) *dnsParser
}

func (s *socketFilterSnooper) WaitForDomain(domain string) error {
	if s.statKeeper == nil {
		return nil
	}
	return s.statKeeper.WaitForDomain(domain)
}

// newSocketFilterSnooper returns a new socketFilterSnooper. If parserSelector
// is nil, the snooper uses its default parser for all packets.
func newSocketFilterSnooper(cfg *config.Config, source filter.PacketSource, parserSelector func(filter.PacketInfo) *dnsParser) (*socketFilterSnooper, error) {
	cache := newReverseDNSCache(dnsCacheSize, dnsCacheExpirationPeriod)
	var statKeeper *dnsStatKeeper
	if cfg.CollectDNSStats {
		statKeeper = newDNSStatkeeper(cfg.DNSTimeout, int64(cfg.MaxDNSStats))
		log.Infof("DNS Stats Collection has been enabled. Maximum number of stats objects: %d", cfg.MaxDNSStats)
		if cfg.CollectDNSDomains {
			log.Infof("DNS domain collection has been enabled")
		}
	} else {
		log.Infof("DNS Stats Collection has been disabled.")
	}
	snooper := &socketFilterSnooper{
		source:          source,
		parser:          newDNSParser(source.LayerType(), cfg),
		cache:           cache,
		statKeeper:      statKeeper,
		translation:     new(translation),
		exit:            make(chan struct{}),
		collectLocalDNS: cfg.CollectLocalDNS,
	}
	if parserSelector != nil {
		snooper.parserFor = parserSelector
	} else {
		snooper.parserFor = func(_ filter.PacketInfo) *dnsParser { return snooper.parser }
	}

	return snooper, nil
}

// startPolling starts the background goroutines that read packets and log stats.
// It must be called by the caller of newSocketFilterSnooper once the snooper is
// fully initialised.
func (s *socketFilterSnooper) startPolling() {
	s.wg.Add(1)
	go func() {
		s.pollPackets()
		s.wg.Done()
	}()

	s.wg.Add(1)
	go func() {
		s.logDNSStats()
		s.wg.Done()
	}()
}

// Resolve IPs to DNS addresses
func (s *socketFilterSnooper) Resolve(ips map[util.Address]struct{}) map[util.Address][]Hostname {
	return s.cache.Get(ips)
}

// GetDNSStats gets the latest Stats keyed by unique Key, and domain
func (s *socketFilterSnooper) GetDNSStats() StatsByKeyByNameByType {
	if s.statKeeper == nil {
		return nil
	}
	return s.statKeeper.GetAndResetAllStats()
}

// Start starts the snooper (no-op currently)
func (s *socketFilterSnooper) Start() error {
	return nil // no-op as this is done in newSocketFilterSnooper above
}

// Close terminates the DNS traffic snooper as well as the underlying socket and the attached filter
func (s *socketFilterSnooper) Close() {
	s.once.Do(func() {
		close(s.exit)
		// close the packet capture loop and wait for it to finish
		s.source.Close()
		s.wg.Wait()
		s.cache.Close()
		if s.statKeeper != nil {
			s.statKeeper.Close()
		}
	})
}

// processPacket retrieves DNS information from the received packet data and adds it to
// the reverse DNS cache. The underlying packet data can't be referenced after this method
// call since the underlying memory content gets invalidated by `afpacket`.
// The *translation is recycled and re-used in subsequent calls and it should not be accessed concurrently.
// The second parameter `ts` is the time when the packet was captured off the wire. This is used for latency calculation
// and much more reliable than calling time.Now() at the user layer.
func (s *socketFilterSnooper) processPacket(data []byte, info filter.PacketInfo, ts time.Time) error {
	t := s.getCachedTranslation()
	pktInfo := dnsPacketInfo{}

	if err := s.parserFor(info).ParseInto(data, t, &pktInfo); err != nil {
		switch err {
		case errSkippedPayload: // no need to count or log cases where the packet is valid but has no relevant content
		case errTruncated:
			snooperTelemetry.truncatedPkts.Inc()
		default:
			snooperTelemetry.decodingErrors.Inc()
		}
		return nil
	}

	if s.statKeeper != nil && (s.collectLocalDNS || !pktInfo.key.ServerIP.IsLoopback()) {
		s.statKeeper.ProcessPacketInfo(pktInfo, ts)
	}

	if pktInfo.pktType == successfulResponse {
		s.cache.Add(t)
		snooperTelemetry.successes.Inc()
	} else if pktInfo.pktType == failedResponse {
		snooperTelemetry.errors.Inc()
	} else {
		snooperTelemetry.queries.Inc()
	}

	return nil
}

func (s *socketFilterSnooper) pollPackets() {
	for {
		err := s.source.VisitPackets(s.processPacket)

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
			queries = snooperTelemetry.queries.Load()
			successes = snooperTelemetry.successes.Load()
			errors = snooperTelemetry.errors.Load()
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
