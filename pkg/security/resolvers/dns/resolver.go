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
)

// Resolver defines a resolver
type Resolver struct {
	cache           *lru.Cache[netip.Addr, map[string]bool]
	cnameCache      *lru.Cache[string, map[string]bool]
	statsdClient    statsd.ClientInterface
	cacheHits       atomic.Int64
	cacheMisses     atomic.Int64
	cacheInsertions atomic.Int64
	cacheEvictions  atomic.Int64
}

// NewDNSResolver returns a new resolver
func NewDNSResolver(cfg *config.Config, statsdClient statsd.ClientInterface) *Resolver {
	ret := &Resolver{
		statsdClient: statsdClient,
	}

	cb := func(netip.Addr, map[string]bool) {
		ret.cacheEvictions.Inc()
	}

	ret.cache, _ = lru.NewWithEvict[netip.Addr, map[string]bool](cfg.DNSResolverCacheSize, cb)
	ret.cnameCache, _ = lru.New[string, map[string]bool](cfg.DNSResolverCacheSize)

	return ret
}

// fillWithCnames Recursively fills the set with all the cname aliases for the hostname
func (r *Resolver) fillWithCnames(hostname string, set *map[string]bool, depth int) {
	if depth == 0 {
		return
	}

	c, ok := r.cnameCache.Get(hostname)
	if ok {
		for hostname := range c {
			(*set)[hostname] = true
			r.fillWithCnames(hostname, set, depth-1)
		}
	}
}

// HostListFromIP gets a hostname from an IP address if cached
func (r *Resolver) HostListFromIP(addr netip.Addr) []string {
	hostname, ok := r.cache.Get(addr)
	if ok {
		r.cacheHits.Inc()
		fmt.Printf("DNS cache hit for %s. count = %d\n", addr, r.cacheHits.Load())
		// Create a set wil all the hostnames to be returned
		allHosts := make(map[string]bool)
		for k := range hostname {
			allHosts[k] = true
			r.fillWithCnames(k, &allHosts, 2)
		}

		ret := make([]string, 0)
		for hostname := range allHosts {
			ret = append(ret, hostname)
		}
		return ret
	}

	r.cacheMisses.Inc()
	fmt.Printf("DNS cache miss for %s. count = %d\n", addr, r.cacheMisses.Load())
	return nil
}

// AddNew add new ip address to the resolver cache
func (r *Resolver) AddNew(hostname string, ip netip.Addr) {
	fmt.Printf("Adding new IP to resolver %v, %v\n", hostname, ip)

	hostnames, ok := r.cache.Get(ip)

	if !ok {
		r.cacheInsertions.Inc()
		hostnames = map[string]bool{}
	}

	hostnames[hostname] = true
	r.cache.Add(ip, hostnames)
}

// AddNewCname add new cname alias to the cache
func (r *Resolver) AddNewCname(cname string, hostname string) {
	hostnames, ok := r.cnameCache.Get(cname)

	if !ok {
		hostnames = map[string]bool{}
	}

	hostnames[hostname] = true
	r.cnameCache.Add(cname, hostnames)
}

// SendStats sends the DNS resolver metrics
func (r *Resolver) SendStats() error {
	entry := []string{
		metrics.CacheTag,
		metrics.SegmentResolutionTag,
	}

	hits := r.cacheHits.Swap(0)
	_ = r.statsdClient.Count(metrics.MetricDNSResolverHits, hits, entry, 1.0)

	misses := r.cacheMisses.Swap(0)
	_ = r.statsdClient.Count(metrics.MetricDNSResolverMiss, misses, entry, 1.0)

	evictions := r.cacheEvictions.Swap(0)
	_ = r.statsdClient.Count(metrics.MetricDNSResolverEvictions, evictions, entry, 1.0)

	insertions := r.cacheInsertions.Swap(0)
	_ = r.statsdClient.Count(metrics.MetricDNSResolverInsertions, insertions, entry, 1.0)

	return nil
}
