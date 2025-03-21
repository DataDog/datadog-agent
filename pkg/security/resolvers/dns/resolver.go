// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package dns resolves ip addresses to hostnames
package dns

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-go/v5/statsd"
	lru "github.com/hashicorp/golang-lru/v2"
	"go.uber.org/atomic"
	"net/netip"
	"slices"
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
	cache         *lru.Cache[netip.Addr, []string]
	cnameCache    *lru.Cache[string, []string]
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

	cbResolver := func(netip.Addr, []string) {
		ret.resolverStats.cacheEvictions.Inc()
	}

	cbCname := func(string, []string) {
		ret.cnameStats.cacheEvictions.Inc()
	}

	var err error

	ret.cache, err = lru.NewWithEvict[netip.Addr, []string](cfg.DNSResolverCacheSize, cbResolver)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DNS cache: %w", err)
	}

	ret.cnameCache, err = lru.NewWithEvict[string, []string](cfg.DNSResolverCacheSize, cbCname)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DNS cname cache: %w", err)
	}

	return ret, nil
}

// fillWithCnames Recursively fills the set with all the cname aliases for the hostname
func (r *Resolver) fillWithCnames(hostname string, hostnames *[]string, depth int) {
	if depth == 0 {
		return
	}

	c, ok := r.cnameCache.Get(hostname)
	if ok {
		r.cnameStats.cacheHits.Inc()
		for _, hostname := range c {
			if !slices.Contains(*hostnames, hostname) {
				*hostnames = append(*hostnames, hostname)
			}
			r.fillWithCnames(hostname, hostnames, depth-1)
		}
	} else {
		r.cnameStats.cacheMisses.Inc()
	}
}

// HostListFromIP gets a hostname from an IP address if cached
func (r *Resolver) HostListFromIP(addr netip.Addr) []string {
	hostnames, ok := r.cache.Get(addr)
	if ok {
		r.resolverStats.cacheHits.Inc()

		var allHosts []string
		for _, hostname := range hostnames {
			allHosts = append(allHosts, hostname)
			r.fillWithCnames(hostname, &allHosts, 2)
		}

		return allHosts
	}

	r.resolverStats.cacheMisses.Inc()
	return nil
}

// AddNew add new ip address to the resolver cache
func (r *Resolver) AddNew(hostname string, ip netip.Addr) {
	hostnames, ok := r.cache.Get(ip)

	if !ok {
		r.resolverStats.cacheInsertions.Inc()
		hostnames = []string{hostname}
	} else if !slices.Contains(hostnames, hostname) {
		hostnames = append(hostnames, hostname)
	}

	r.cache.Add(ip, hostnames)
}

// AddNewCname add new cname alias to the cache
func (r *Resolver) AddNewCname(cname string, hostname string) {
	hostnames, ok := r.cnameCache.Get(cname)

	if !ok {
		r.cnameStats.cacheInsertions.Inc()
		hostnames = []string{hostname}
	} else if !slices.Contains(hostnames, hostname) {
		hostnames = append(hostnames, hostname)
	}

	r.cnameCache.Add(cname, hostnames)
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
