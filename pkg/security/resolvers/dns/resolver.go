// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dns resolves ip addresses to hostnames
package dns

import (
	"fmt"
	"net/netip"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-go/v5/statsd"
	lru "github.com/hashicorp/golang-lru/v2"
	"go.uber.org/atomic"
)

// CacheStats defines metrics for the LRU
type CacheStats struct {
	cacheHits       atomic.Int64
	cacheMisses     atomic.Int64
	cacheInsertions atomic.Int64
	cacheEvictions  atomic.Int64
}

// Resolver defines a DNS resolver
type Resolver struct {
	cache  *lru.Cache[netip.Addr, []string]
	direct *lru.Cache[string, []netip.Addr]
	cnames *lru.Cache[string, string]

	inFlightHostnames []string

	statsdClient  statsd.ClientInterface
	resolverStats *CacheStats
	cnameStats    *CacheStats
}

// NewDNSResolver returns a new resolver
func NewDNSResolver(cfg *config.Config, statsdClient statsd.ClientInterface) (*Resolver, error) {
	ret := &Resolver{
		statsdClient:  statsdClient,
		resolverStats: &CacheStats{},
		cnameStats:    &CacheStats{},
	}

	cbCacheResolver := func(netip.Addr, []string) {
		ret.resolverStats.cacheEvictions.Inc()
	}

	cbDirectResolver := func(string, []netip.Addr) {
		ret.cnameStats.cacheEvictions.Inc()
	}

	cbCnamesResolver := func(string, string) {
		ret.cnameStats.cacheEvictions.Inc()
	}

	var err error
	ret.cache, err = lru.NewWithEvict[netip.Addr, []string](cfg.DNSResolverCacheSize, cbCacheResolver)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DNS cache: %w", err)
	}

	ret.direct, err = lru.NewWithEvict[string, []netip.Addr](cfg.DNSResolverCacheSize, cbDirectResolver)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DNS cache: %w", err)
	}

	ret.cnames, err = lru.NewWithEvict[string, string](cfg.DNSResolverCacheSize, cbCnamesResolver)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DNS cache: %w", err)
	}

	return ret, nil
}

func (r *Resolver) clear() {
	r.cache.Purge()
	r.direct.Purge()
	r.cnames.Purge()
	r.inFlightHostnames = nil
}

// HostListFromIP gets a hostname from an IP address if cached
func (r *Resolver) HostListFromIP(addr netip.Addr) []string {
	if hostnames, ok := r.cache.Get(addr); ok {
		r.resolverStats.cacheHits.Inc()
		return hostnames
	}

	r.resolverStats.cacheMisses.Inc()
	return nil
}

// AddNew add new ip address to the resolver cache
func (r *Resolver) AddNew(hostname string, ip netip.Addr) {
	appendLRUValue(r.direct, hostname, ip)
	r.inFlightHostnames = append(r.inFlightHostnames, hostname)
	r.cnameStats.cacheInsertions.Inc()
}

// AddNewCname add new cname alias to the cache
func (r *Resolver) AddNewCname(cname string, hostname string) {
	r.cnames.Add(hostname, cname)
	r.inFlightHostnames = append(r.inFlightHostnames, cname, hostname)
	r.cnameStats.cacheInsertions.Inc()
}

const indirectCnameLimit = 5

// CommitInFlights commits all in-flight hostnames to the reverse cache
func (r *Resolver) CommitInFlights() {
	for _, inFlight := range r.inFlightHostnames {
		ips, ok := r.queryDirect(inFlight, indirectCnameLimit)
		if !ok {
			continue
		}

		for _, ip := range ips {
			appendLRUValue(r.cache, ip, inFlight)
			r.resolverStats.cacheInsertions.Inc()
		}
	}

	r.inFlightHostnames = nil
}

func appendLRUValue[K comparable, V any](list *lru.Cache[K, []V], key K, value V) {
	var next []V
	if old, ok := list.Get(key); ok {
		next = append(old, value)
	} else {
		next = []V{value}
	}
	list.Add(key, next)
}

func (r *Resolver) queryDirect(hostname string, iterations int) ([]netip.Addr, bool) {
	ips, ok := r.direct.Get(hostname)
	if ok {
		return ips, true
	}

	if iterations > 0 {
		if next, ok := r.cnames.Get(hostname); ok {
			return r.queryDirect(next, iterations-1)
		}
	}

	return nil, false
}

// SendStats sends the DNS resolver metrics
func (r *Resolver) SendStats() error {
	tags := []string{
		"hit",
	}

	hits := r.resolverStats.cacheHits.Swap(0)
	_ = r.statsdClient.Count(metrics.MetricDNSResolverCnameResolverCache, hits, tags, 1.0)

	cnameHits := r.cnameStats.cacheHits.Swap(0)
	_ = r.statsdClient.Count(metrics.MetricDNSResolverIPResolverCache, cnameHits, tags, 1.0)

	tags = []string{
		"miss",
	}

	misses := r.resolverStats.cacheMisses.Swap(0)
	_ = r.statsdClient.Count(metrics.MetricDNSResolverCnameResolverCache, misses, tags, 1.0)

	cnameMisses := r.cnameStats.cacheMisses.Swap(0)
	_ = r.statsdClient.Count(metrics.MetricDNSResolverIPResolverCache, cnameMisses, tags, 1.0)

	tags = []string{
		"insertion",
	}

	insertions := r.resolverStats.cacheInsertions.Swap(0)
	_ = r.statsdClient.Count(metrics.MetricDNSResolverCnameResolverCache, insertions, tags, 1.0)

	cnameInsertions := r.cnameStats.cacheInsertions.Swap(0)
	_ = r.statsdClient.Count(metrics.MetricDNSResolverIPResolverCache, cnameInsertions, tags, 1.0)

	tags = []string{
		"eviction",
	}

	evictions := r.resolverStats.cacheEvictions.Swap(0)
	_ = r.statsdClient.Count(metrics.MetricDNSResolverCnameResolverCache, evictions, tags, 1.0)

	cnameEvictions := r.cnameStats.cacheEvictions.Swap(0)
	_ = r.statsdClient.Count(metrics.MetricDNSResolverIPResolverCache, cnameEvictions, tags, 1.0)

	return nil
}
