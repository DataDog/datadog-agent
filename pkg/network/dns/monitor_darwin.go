// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package dns

import (
	"sync"
	"time"

	"github.com/google/gopacket/layers"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/filter"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// darwinDNSMonitor implements ReverseDNS for macOS using libpcap packet capture.
// It maintains two parsers — one for Ethernet frames and one for BSD loopback
// frames — and dispatches each packet to the correct parser based on the
// per-packet LayerType recorded by the capture source.
type darwinDNSMonitor struct {
	ethernetParser  *dnsParser
	loopbackParser  *dnsParser
	cache           *reverseDNSCache
	statKeeper      *dnsStatKeeper
	source          filter.PacketSource
	wg              sync.WaitGroup
	once            sync.Once
	exit            chan struct{}
	collectLocalDNS bool

	// translation is recycled across processPacket calls to avoid per-packet allocations.
	translation *translation
}

// NewReverseDNS starts DNS traffic monitoring on macOS and returns a ReverseDNS
// implementation backed by libpcap packet capture.
func NewReverseDNS(cfg *config.Config, _ telemetry.Component) (ReverseDNS, error) {
	src, err := filter.NewSubSource(cfg, filter.IsDNSPacket)
	if err != nil {
		return nil, err
	}
	return newDarwinDNSMonitorWithSource(cfg, src)
}

// newDarwinDNSMonitorWithSource constructs a darwinDNSMonitor using the
// provided PacketSource. This is the internal constructor used by both
// NewReverseDNS and tests that inject a mock source.
func newDarwinDNSMonitorWithSource(cfg *config.Config, src filter.PacketSource) (*darwinDNSMonitor, error) {
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

	m := &darwinDNSMonitor{
		ethernetParser:  newDNSParser(layers.LayerTypeEthernet, cfg),
		loopbackParser:  newDNSParser(layers.LayerTypeLoopback, cfg),
		cache:           cache,
		statKeeper:      statKeeper,
		source:          src,
		exit:            make(chan struct{}),
		collectLocalDNS: cfg.CollectLocalDNS,
		translation:     new(translation),
	}

	m.wg.Add(1)
	go func() {
		m.pollPackets()
		m.wg.Done()
	}()

	m.wg.Add(1)
	go func() {
		m.logDNSStats()
		m.wg.Done()
	}()

	return m, nil
}

// processPacket handles a single captured packet: selects the appropriate
// parser based on link-layer type, parses the DNS payload, updates the cache
// and telemetry counters, and optionally feeds the stat keeper.
func (m *darwinDNSMonitor) processPacket(data []byte, info filter.PacketInfo, ts time.Time) error {
	pktInfo, _ := info.(*filter.DarwinPacketInfo)
	if pktInfo == nil {
		pktInfo = &filter.DarwinPacketInfo{}
	}

	var parser *dnsParser
	if pktInfo.LayerType == layers.LayerTypeLoopback {
		parser = m.loopbackParser
	} else {
		parser = m.ethernetParser
	}

	t := m.getCachedTranslation()
	dnsInfo := dnsPacketInfo{}

	if err := parser.ParseInto(data, t, &dnsInfo); err != nil {
		switch err {
		case errSkippedPayload: // no-op: valid packet but not a relevant DNS response
		case errTruncated:
			snooperTelemetry.truncatedPkts.Inc()
		default:
			snooperTelemetry.decodingErrors.Inc()
		}
		return nil
	}

	if m.statKeeper != nil && (m.collectLocalDNS || !dnsInfo.key.ServerIP.IsLoopback()) {
		m.statKeeper.ProcessPacketInfo(dnsInfo, ts)
	}

	if dnsInfo.pktType == successfulResponse {
		m.cache.Add(t)
		snooperTelemetry.successes.Inc()
	} else if dnsInfo.pktType == failedResponse {
		snooperTelemetry.errors.Inc()
	} else {
		snooperTelemetry.queries.Inc()
	}

	return nil
}

func (m *darwinDNSMonitor) pollPackets() {
	for {
		err := m.source.VisitPackets(m.processPacket)
		if err != nil {
			log.Warnf("error reading packet: %s", err)
		}

		select {
		case <-m.exit:
			return
		default:
		}

		time.Sleep(5 * time.Millisecond)
	}
}

func (m *darwinDNSMonitor) logDNSStats() {
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
		case <-m.exit:
			return
		}
	}
}

func (m *darwinDNSMonitor) getCachedTranslation() *translation {
	t := m.translation
	if t.ips == nil || len(t.ips) > maxIPBufferSize {
		t.ips = make(map[util.Address]time.Time, 30)
	}
	for k := range t.ips {
		delete(t.ips, k)
	}
	return t
}

// Resolve converts IP addresses to DNS hostnames using the local cache.
func (m *darwinDNSMonitor) Resolve(ips map[util.Address]struct{}) map[util.Address][]Hostname {
	return m.cache.Get(ips)
}

// GetDNSStats returns accumulated DNS statistics and resets internal counters.
// Returns nil if DNS stats collection is disabled.
func (m *darwinDNSMonitor) GetDNSStats() StatsByKeyByNameByType {
	if m.statKeeper == nil {
		return nil
	}
	return m.statKeeper.GetAndResetAllStats()
}

// Start is a no-op because the monitor goroutines are started in the constructor.
func (m *darwinDNSMonitor) Start() error {
	return nil
}

// WaitForDomain blocks until the given domain has been observed by the stat
// keeper, or returns an error if the stat keeper is disabled or times out.
func (m *darwinDNSMonitor) WaitForDomain(domain string) error {
	if m.statKeeper == nil {
		return nil
	}
	return m.statKeeper.WaitForDomain(domain)
}

// Close shuts down the monitor, waits for all goroutines to exit, and releases
// all resources. Safe to call multiple times.
func (m *darwinDNSMonitor) Close() {
	m.once.Do(func() {
		close(m.exit)
		m.source.Close()
		m.wg.Wait()
		m.cache.Close()
		if m.statKeeper != nil {
			m.statKeeper.Close()
		}
	})
}

var _ ReverseDNS = &darwinDNSMonitor{}
