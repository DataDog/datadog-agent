// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package envvars holds envvars related files
package dns

import (
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	lru "github.com/hashicorp/golang-lru/v2"
	"net/netip"
)

// Resolver defines a resolver
type Resolver struct {
	cache      *lru.Cache[netip.Addr, map[string]bool]
	cnameCache *lru.Cache[string, map[string]bool]
}

// NewDnsResolver returns a new resolver
func NewDnsResolver(cfg *config.Config) *Resolver {
	c, _ := lru.New[netip.Addr, map[string]bool](cfg.DNSResolverCacheSize)
	cc, _ := lru.New[string, map[string]bool](cfg.DNSResolverCacheSize)
	return &Resolver{
		cache:      c,
		cnameCache: cc,
	}
}

// fillWithCnames Recursively fills the set with all the cname aliases for the hostname
func (r *Resolver) fillWithCnames(hostname string, set *map[string]bool, depth int) {
	if depth == 0 {
		return
	}

	c, ok := r.cnameCache.Get(hostname)
	if ok {
		for hostname, _ := range c {
			(*set)[hostname] = true
			r.fillWithCnames(hostname, set, depth-1)
		}
	}
}

// HostListFromIp gets a hostname from an IP address if cached
func (r *Resolver) HostListFromIp(addr netip.Addr) []string {
	hostname, ok := r.cache.Get(addr)
	if ok {
		// Create a set wil all the hostnames to be returned
		allHosts := make(map[string]bool)
		for k, _ := range hostname {
			allHosts[k] = true
			r.fillWithCnames(k, &allHosts, 2)
		}

		ret := make([]string, 0)
		for hostname, _ := range allHosts {
			ret = append(ret, hostname)
		}
		return ret
	}
	return nil
}

// AddNew add new ip address to the resolver cache
func (r *Resolver) AddNew(hostname string, ip netip.Addr) {
	hostnames, ok := r.cache.Get(ip)

	if !ok {
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
