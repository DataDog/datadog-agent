// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf || darwin

package dns

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/process/util"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const dnsCacheModuleName = "network_tracer__dns_cache"

// Telemetry
var cacheTelemetry = struct {
	length    *telemetry.StatGaugeWrapper
	lookups   *telemetry.StatCounterWrapper
	resolved  *telemetry.StatCounterWrapper
	added     *telemetry.StatCounterWrapper
	expired   *telemetry.StatCounterWrapper
	oversized *telemetry.StatCounterWrapper
}{
	telemetry.NewStatGaugeWrapper(telemetryimpl.GetCompatComponent(), dnsCacheModuleName, "size", []string{}, "Gauge measuring the current size of the DNS cache"),
	telemetry.NewStatCounterWrapper(telemetryimpl.GetCompatComponent(), dnsCacheModuleName, "lookups", []string{}, "Counter measuring the number of lookups to the DNS cache"),
	telemetry.NewStatCounterWrapper(telemetryimpl.GetCompatComponent(), dnsCacheModuleName, "hits", []string{}, "Counter measuring the number of successful lookups to the DNS cache"),
	telemetry.NewStatCounterWrapper(telemetryimpl.GetCompatComponent(), dnsCacheModuleName, "added", []string{}, "Counter measuring the number of additions to the DNS cache"),
	telemetry.NewStatCounterWrapper(telemetryimpl.GetCompatComponent(), dnsCacheModuleName, "expired", []string{}, "Counter measuring the number of failed lookups to the DNS cache"),
	telemetry.NewStatCounterWrapper(telemetryimpl.GetCompatComponent(), dnsCacheModuleName, "oversized", []string{}, "Counter measuring the number of lookups to the DNS cache that reached the max domains per IP limit"),
}

type reverseDNSCache struct {
	mux  sync.Mutex
	data map[util.Address]*dnsCacheVal
	exit chan struct{}
	size int

	// cnames maps a CNAME target back to the owner names that point at it, each
	// with its own expiry (the CNAME record's TTL). CNAME records typically carry
	// a longer TTL than the terminal name's A records, so when only the terminal
	// name is re-resolved we can still re-associate the destination IP with the
	// originally queried names (whose shorter-lived reverse mappings would
	// otherwise expire).
	cnames map[Hostname]map[Hostname]time.Time

	// maxDomainsPerIP is the maximum number of domains mapped to a single IP
	maxDomainsPerIP   int
	oversizedLogLimit *log.Limit
}

func newReverseDNSCache(size int, expirationPeriod time.Duration) *reverseDNSCache {
	cache := &reverseDNSCache{
		data:              make(map[util.Address]*dnsCacheVal),
		cnames:            make(map[Hostname]map[Hostname]time.Time),
		exit:              make(chan struct{}),
		size:              size,
		oversizedLogLimit: log.NewLogLimit(10, time.Minute*10),
		maxDomainsPerIP:   1000,
	}

	ticker := time.NewTicker(expirationPeriod)
	go func() {
		for {
			select {
			case now := <-ticker.C:
				cache.Expire(now)
			case <-cache.exit:
				ticker.Stop()
				return
			}
		}
	}()
	return cache
}

func (c *reverseDNSCache) Add(translation *translation) bool {
	if translation == nil {
		return false
	}

	c.mux.Lock()
	defer c.mux.Unlock()

	// Learn CNAME owner->target edges even if the data map is full, so a later
	// terminal-name refresh can still be mapped back to the queried names.
	c.addCNAMEs(translation)

	if len(c.data) >= c.size {
		return false
	}

	// Any owner name that resolves (directly or transitively) to translation.dns
	// via a still-valid CNAME edge. For a terminal ELB name this recovers the
	// original *.datadoghq.com name; for a full response the terminal name is the
	// queried name and this is empty.
	owners := c.cnameOwners(translation.dns)
	for addr, deadline := range translation.ips {
		c.addName(addr, translation.dns, deadline)
		for _, owner := range owners {
			c.addName(addr, owner, deadline)
		}
	}

	// Update cache length for telemetry purposes
	cacheTelemetry.length.Set(int64(len(c.data)))

	return true
}

// addName maps addr to name with the given deadline, creating or merging the
// cache entry. The caller must hold c.mux.
func (c *reverseDNSCache) addName(addr util.Address, name Hostname, deadline time.Time) {
	val, ok := c.data[addr]
	if ok {
		if rejected := val.merge(name, deadline, c.maxDomainsPerIP); rejected && c.oversizedLogLimit.ShouldLog() {
			log.Warnf("%s mapped to too many domains, DNS information will be dropped (this will be logged the first 10 times, and then at most every 10 minutes)", addr)
		}
		return
	}
	cacheTelemetry.added.Inc()
	// flag as in use, so mapping survives until next time connections are queried, in case TTL is shorter
	c.data[addr] = &dnsCacheVal{names: map[Hostname]time.Time{name: deadline}, inUse: true}
}

// addCNAMEs records the translation's CNAME edges in the owner graph. The caller
// must hold c.mux.
func (c *reverseDNSCache) addCNAMEs(translation *translation) {
	now := time.Now()
	for _, edge := range translation.cnames {
		if edge.owner == edge.target {
			continue
		}
		deadline := now.Add(edge.ttl)
		owners, ok := c.cnames[edge.target]
		if !ok {
			if len(c.cnames) >= c.size {
				continue
			}
			owners = make(map[Hostname]time.Time)
			c.cnames[edge.target] = owners
		}
		if exp, ok := owners[edge.owner]; !ok || deadline.After(exp) {
			owners[edge.owner] = deadline
		}
	}
}

// cnameOwners walks the CNAME graph upward from target and returns every owner
// name that reaches it, following chains (e.g. datadoghq.com -> cloudfront ->
// ELB). The caller must hold c.mux.
func (c *reverseDNSCache) cnameOwners(target Hostname) []Hostname {
	if len(c.cnames) == 0 {
		return nil
	}
	var owners []Hostname
	visited := map[Hostname]struct{}{target: {}}
	queue := []Hostname{target}
	for len(queue) > 0 && len(owners) < c.maxDomainsPerIP {
		cur := queue[0]
		queue = queue[1:]
		for owner := range c.cnames[cur] {
			if _, seen := visited[owner]; seen {
				continue
			}
			visited[owner] = struct{}{}
			owners = append(owners, owner)
			queue = append(queue, owner)
		}
	}
	return owners
}

func (c *reverseDNSCache) Get(ips map[util.Address]struct{}) map[util.Address][]Hostname {
	c.mux.Lock()
	defer c.mux.Unlock()

	for _, val := range c.data {
		val.inUse = false
	}

	if len(ips) == 0 {
		return nil
	}

	var (
		resolved   = make(map[util.Address][]Hostname)
		unresolved = make(map[util.Address]struct{})
		oversized  = make(map[util.Address]struct{})
	)

	collectNamesForIP := func(addr util.Address) {
		if _, ok := resolved[addr]; ok {
			return
		}

		if _, ok := unresolved[addr]; ok {
			return
		}

		if _, ok := oversized[addr]; ok {
			return
		}

		names := c.getNamesForIP(addr)
		if len(names) == 0 {
			unresolved[addr] = struct{}{}
		} else if len(names) == c.maxDomainsPerIP {
			oversized[addr] = struct{}{}
		} else {
			resolved[addr] = names
		}
	}

	for ip := range ips {
		collectNamesForIP(ip)
	}

	// Update stats for telemetry
	cacheTelemetry.lookups.Add(int64(len(resolved) + len(unresolved)))
	cacheTelemetry.resolved.Add(int64(len(resolved)))
	cacheTelemetry.oversized.Add(int64(len(oversized)))

	return resolved
}

func (c *reverseDNSCache) Len() int {
	c.mux.Lock()
	defer c.mux.Unlock()
	return len(c.data)
}

func (c *reverseDNSCache) Close() {
	close(c.exit)
}

func (c *reverseDNSCache) Expire(now time.Time) {
	expired := 0
	c.mux.Lock()
	for addr, val := range c.data {
		if val.inUse {
			continue
		}

		for ip, deadline := range val.names {
			if deadline.Before(now) {
				delete(val.names, ip)
			}
		}

		if len(val.names) != 0 {
			continue
		}
		expired++
		delete(c.data, addr)
	}

	// Prune expired CNAME edges. These are never held "in use"; they are cheap to
	// relearn from the next full response and must not outlive their TTL.
	for target, owners := range c.cnames {
		for owner, deadline := range owners {
			if deadline.Before(now) {
				delete(owners, owner)
			}
		}
		if len(owners) == 0 {
			delete(c.cnames, target)
		}
	}

	total := len(c.data)
	c.mux.Unlock()

	cacheTelemetry.expired.Add(int64(expired))
	cacheTelemetry.length.Set(int64(total))
	log.Debugf(
		"dns entries expired. took=%s total=%d expired=%d\n",
		time.Since(now), total, expired,
	)
}

func (c *reverseDNSCache) getNamesForIP(ip util.Address) []Hostname {
	val, ok := c.data[ip]
	if !ok {
		return nil
	}
	val.inUse = true
	return val.copy()
}

type dnsCacheVal struct {
	names map[Hostname]time.Time
	// inUse keeps track of whether this dns cache record is currently in use by a connection.
	// This flag is reset to false every time reverseDnsCache.Get is called.
	// This flag is only set to true if reverseDNSCache.getNamesForIP returns this struct.
	// If inUse is set, then this record will not be expired out.
	inUse bool
}

func (v *dnsCacheVal) merge(name Hostname, deadline time.Time, maxSize int) (rejected bool) {
	if exp, ok := v.names[name]; ok {
		if deadline.After(exp) {
			v.names[name] = deadline
			v.inUse = true
		}
		return false
	}
	if len(v.names) == maxSize {
		return true
	}

	v.names[name] = deadline
	v.inUse = true
	return false
}

func (v *dnsCacheVal) copy() []Hostname {
	cpy := make([]Hostname, 0, len(v.names))
	for n := range v.names {
		cpy = append(cpy, n)
	}
	return cpy
}

type translation struct {
	dns    Hostname
	ips    map[util.Address]time.Time
	cnames []cnameEdge
}

// cnameEdge is a single owner->target CNAME record observed in a response.
type cnameEdge struct {
	owner  Hostname
	target Hostname
	ttl    time.Duration
}

func (t *translation) add(addr util.Address, ttl time.Duration) {
	if _, ok := t.ips[addr]; ok {
		return
	}
	t.ips[addr] = time.Now().Add(ttl)
}

func (t *translation) addCNAME(owner, target Hostname, ttl time.Duration) {
	t.cnames = append(t.cnames, cnameEdge{owner: owner, target: target, ttl: ttl})
}
