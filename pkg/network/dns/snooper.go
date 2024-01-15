// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package dns

import (
	"sync"
	"time"

	"github.com/google/gopacket"

	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	dnsCacheExpirationPeriod = 1 * time.Minute
	dnsCacheSize             = 100000
	dnsModuleName            = "network_tracer__dns"
)

// Telemetry
var snooperTelemetry = struct {
	decodingErrors *ebpftelemetry.StatCounterWrapper
	truncatedPkts  *ebpftelemetry.StatCounterWrapper
	queries        *ebpftelemetry.StatCounterWrapper
	successes      *ebpftelemetry.StatCounterWrapper
	errors         *ebpftelemetry.StatCounterWrapper
}{
	ebpftelemetry.NewStatCounterWrapper(dnsModuleName, "decoding_errors", []string{}, "Counter measuring the number of decoding errors while processing packets"),
	ebpftelemetry.NewStatCounterWrapper(dnsModuleName, "truncated_pkts", []string{}, "Counter measuring the number of truncated packets while processing"),
	// DNS telemetry, values calculated *till* the last tick in pollStats
	ebpftelemetry.NewStatCounterWrapper(dnsModuleName, "queries", []string{}, "Counter measuring the number of packets that are DNS queries in processed packets"),
	ebpftelemetry.NewStatCounterWrapper(dnsModuleName, "successes", []string{}, "Counter measuring the number of successful DNS responses in processed packets"),
	ebpftelemetry.NewStatCounterWrapper(dnsModuleName, "errors", []string{}, "Counter measuring the number of failed DNS responses in processed packets"),
}

var _ ReverseDNS = &socketFilterSnooper{}

// socketFilterSnooper is a DNS traffic snooper built on top of an eBPF SOCKET_FILTER
type socketFilterSnooper struct {
	source          packetSource
	parser          *dnsParser
	cache           *reverseDNSCache
	statKeeper      *dnsStatKeeper
	exit            chan struct{}
	wg              sync.WaitGroup
	collectLocalDNS bool
	once            sync.Once

	// cache translation object to avoid allocations
	translation *translation
}

func (s *socketFilterSnooper) WaitForDomain(domain string) error {
	return s.statKeeper.WaitForDomain(domain)
}

// packetSource reads raw packet data
type packetSource interface {
	// VisitPackets reads all new raw packets that are available, invoking the given callback for each packet.
	// If no packet is available, VisitPacket returns immediately.
	// The format of the packet is dependent on the implementation of packetSource -- i.e. it may be an ethernet frame, or a IP frame.
	// The data buffer is reused between invocations of VisitPacket and thus should not be pointed to.
	// If the cancel channel is closed, VisitPackets will stop reading.
	VisitPackets(cancel <-chan struct{}, visitor func(data []byte, timestamp time.Time) error) error

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
		s.wg.Wait()
		s.source.Close()
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
func (s *socketFilterSnooper) processPacket(data []byte, ts time.Time) error {
	t := s.getCachedTranslation()
	pktInfo := dnsPacketInfo{}

	if err := s.parser.ParseInto(data, t, &pktInfo); err != nil {
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
