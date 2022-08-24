// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows || linux_bpf
// +build windows linux_bpf

package dns

import (
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type reverseDNSCache struct {
	// Telemetry
	length    *atomic.Int64
	lookups   *atomic.Int64
	resolved  *atomic.Int64
	added     *atomic.Int64
	expired   *atomic.Int64
	oversized *atomic.Int64

	mux  sync.Mutex
	data map[util.Address]*dnsCacheVal
	exit chan struct{}
	size int

	// maxDomainsPerIP is the maximum number of domains mapped to a single IP
	maxDomainsPerIP   int
	oversizedLogLimit *util.LogLimit
}

func newReverseDNSCache(size int, expirationPeriod time.Duration) *reverseDNSCache {
	cache := &reverseDNSCache{
		length:            atomic.NewInt64(0),
		lookups:           atomic.NewInt64(0),
		resolved:          atomic.NewInt64(0),
		added:             atomic.NewInt64(0),
		expired:           atomic.NewInt64(0),
		oversized:         atomic.NewInt64(0),
		data:              make(map[util.Address]*dnsCacheVal),
		exit:              make(chan struct{}),
		size:              size,
		oversizedLogLimit: util.NewLogLimit(10, time.Minute*10),
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
	if len(c.data) >= c.size {
		return false
	}

	for addr, deadline := range translation.ips {
		val, ok := c.data[addr]
		if ok {
			if rejected := val.merge(translation.dns, deadline, c.maxDomainsPerIP); rejected && c.oversizedLogLimit.ShouldLog() {
				log.Warnf("%s mapped to too many domains, DNS information will be dropped (this will be logged the first 10 times, and then at most every 10 minutes)", addr)
			}
		} else {
			c.added.Inc()
			// flag as in use, so mapping survives until next time connections are queried, in case TTL is shorter
			c.data[addr] = &dnsCacheVal{names: map[Hostname]time.Time{translation.dns: deadline}, inUse: true}
		}
	}

	// Update cache length for telemetry purposes
	c.length.Store(int64(len(c.data)))

	return true
}

func (c *reverseDNSCache) Get(ips []util.Address) map[util.Address][]Hostname {
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

	for _, ip := range ips {
		collectNamesForIP(ip)
	}

	// Update stats for telemetry
	c.lookups.Add(int64(len(resolved) + len(unresolved)))
	c.resolved.Add(int64(len(resolved)))
	c.oversized.Add(int64(len(oversized)))

	return resolved
}

func (c *reverseDNSCache) Len() int {
	return int(c.length.Load())
}

func (c *reverseDNSCache) Stats() map[string]int64 {
	var (
		lookups   = c.lookups.Load()
		resolved  = c.resolved.Load()
		added     = c.added.Load()
		expired   = c.expired.Load()
		oversized = c.oversized.Load()
		ips       = int64(c.Len())
	)

	return map[string]int64{
		"lookups":   lookups,
		"resolved":  resolved,
		"added":     added,
		"expired":   expired,
		"oversized": oversized,
		"ips":       ips,
	}
}

func (c *reverseDNSCache) Close() {
	c.exit <- struct{}{}
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
	total := len(c.data)
	c.mux.Unlock()

	c.expired.Store(int64(expired))
	c.length.Store(int64(total))
	log.Debugf(
		"dns entries expired. took=%s total=%d expired=%d\n",
		time.Now().Sub(now), total, expired,
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
	dns Hostname
	ips map[util.Address]time.Time
}

func (t *translation) add(addr util.Address, ttl time.Duration) {
	if _, ok := t.ips[addr]; ok {
		return
	}
	t.ips[addr] = time.Now().Add(ttl)
}
