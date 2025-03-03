// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package discard stores low-value pathtests in a cache so they don't get tracerouted again.
package discard

import (
	"net"
	"time"

	"github.com/jellydator/ttlcache/v3"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
)

const (
	networkPathDiscardScannerMetricPrefix = "datadog.network_path.discard_scanner."
)

// ScannerConfig is the configuration for the discard scanner
type ScannerConfig struct {
	// Enabled is whether the discard scanner is enabled.
	// If this is false, nothing ever gets discarded
	Enabled       bool
	CacheCapacity int64
	CacheTTL      time.Duration
}

// Scanner is a discard scanner, which identifies and remembers low-value traceroutes
type Scanner struct {
	config ScannerConfig
	cache  *ttlcache.Cache[common.PathtestHash, struct{}]
}

// NewScanner creates a new discard scanner
func NewScanner(config ScannerConfig) *Scanner {
	capacity := uint64(config.CacheCapacity)
	if config.CacheCapacity < 0 {
		capacity = 0
	}
	return &Scanner{
		config: config,
		cache: ttlcache.New[common.PathtestHash, struct{}](
			ttlcache.WithCapacity[common.PathtestHash, struct{}](capacity),
			ttlcache.WithTTL[common.PathtestHash, struct{}](config.CacheTTL),
		),
	}
}

// ShouldDiscard returns true if a network path result indicates it is low value and should be discarded.
// It looks for short traceroutes that only have a single reachable hop.
func (s *Scanner) ShouldDiscard(path *payload.NetworkPath) bool {
	if !s.config.Enabled {
		return false
	}
	// shouldn't happen, but avoid crashing
	if len(path.Hops) == 0 {
		return false
	}
	// we only discard short traceroutes
	if len(path.Hops) > 2 {
		return false
	}
	// only the last hop should be reachable
	for i, hop := range path.Hops {
		isLast := i == len(path.Hops)-1
		if hop.Reachable != isLast {
			return false
		}
	}
	// we only want to discard private IPs because those tend to
	// have packet encapsulation which results in those single-hop traceroutes
	destIP := net.ParseIP(path.Destination.IPAddress)
	return destIP.IsPrivate()
}

// MarkDiscardableHash stores in the cache that this pathtest is discardable (AKA, a low value pathtest)
func (s *Scanner) MarkDiscardableHash(hash common.PathtestHash) {
	s.cache.Set(hash, struct{}{}, ttlcache.DefaultTTL)
}

// IsKnownDiscardable checks whether this pathtest has been marked discardable by MarkDiscardableHash
func (s *Scanner) IsKnownDiscardable(hash common.PathtestHash) bool {
	return s.cache.Has(hash)
}

func (s *Scanner) reportMetrics(statsd ddgostatsd.ClientInterface) {
	statsd.Gauge(networkPathDiscardScannerMetricPrefix+"size", float64(s.cache.Len()), nil, 1)              //nolint:errcheck
	statsd.Gauge(networkPathDiscardScannerMetricPrefix+"capacity", float64(s.config.CacheCapacity), nil, 1) //nolint:errcheck
}

// CacheExpirationTask is a task that cleans up stale entries from the cache.
func (s *Scanner) CacheExpirationTask(exit <-chan struct{}) {
	go s.cache.Start()
	<-exit
	s.cache.Stop()
}

// MetricsTask runs a periodic task to report metrics about the discard cache (mainly consumption, capacity)
func (s *Scanner) MetricsTask(exit <-chan struct{}, statsd ddgostatsd.ClientInterface) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	s.reportMetrics(statsd)

	for {
		select {
		case <-exit:
			return
		case <-ticker.C:
			s.reportMetrics(statsd)
		}
	}
}
