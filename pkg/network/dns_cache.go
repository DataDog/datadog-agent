package network

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type reverseDNSCache struct {
	// Telemetry
	// Note: these variables are manipulated with sync/atomic. To ensure
	// that this file can run on a 32 bit system, they must 64-bit aligned.
	// Go will ensure that each struct is 64-bit aligned, so these fields
	// must always be at the beginning of the struct.
	length    int64
	lookups   int64
	resolved  int64
	added     int64
	expired   int64
	oversized int64

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
			atomic.AddInt64(&c.added, 1)
			// flag as in use, so mapping survives until next time connections are queried, in case TTL is shorter
			c.data[addr] = &dnsCacheVal{names: map[string]time.Time{translation.dns: deadline}, inUse: true}
		}
	}

	// Update cache length for telemetry purposes
	atomic.StoreInt64(&c.length, int64(len(c.data)))

	return true
}

func (c *reverseDNSCache) Get(conns []ConnectionStats) map[util.Address][]string {
	c.mux.Lock()
	defer c.mux.Unlock()

	for _, val := range c.data {
		val.inUse = false
	}

	if len(conns) == 0 {
		return nil
	}

	var (
		resolved   = make(map[util.Address][]string)
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

	for _, conn := range conns {
		collectNamesForIP(conn.Source)
		collectNamesForIP(conn.Dest)
	}

	// Update stats for telemetry
	atomic.AddInt64(&c.lookups, int64(len(resolved)+len(unresolved)))
	atomic.AddInt64(&c.resolved, int64(len(resolved)))
	atomic.AddInt64(&c.oversized, int64(len(oversized)))

	return resolved
}

func (c *reverseDNSCache) Len() int {
	return int(atomic.LoadInt64(&c.length))
}

func (c *reverseDNSCache) Stats() map[string]int64 {
	var (
		lookups   = atomic.LoadInt64(&c.lookups)
		resolved  = atomic.LoadInt64(&c.resolved)
		added     = atomic.LoadInt64(&c.added)
		expired   = atomic.LoadInt64(&c.expired)
		oversized = atomic.LoadInt64(&c.oversized)
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

	atomic.StoreInt64(&c.expired, int64(expired))
	atomic.StoreInt64(&c.length, int64(total))
	log.Debugf(
		"dns entries expired. took=%s total=%d expired=%d\n",
		time.Now().Sub(now), total, expired,
	)
}

func (c *reverseDNSCache) getNamesForIP(ip util.Address) []string {
	val, ok := c.data[ip]
	if !ok {
		return nil
	}
	val.inUse = true
	return val.copy()
}

type dnsCacheVal struct {
	names map[string]time.Time
	// inUse keeps track of whether this dns cache record is currently in use by a connection.
	// This flag is reset to false every time reverseDnsCache.Get is called.
	// This flag is only set to true if reverseDNSCache.getNamesForIP returns this struct.
	// If inUse is set, then this record will not be expired out.
	inUse bool
}

func (v *dnsCacheVal) merge(name string, deadline time.Time, maxSize int) (rejected bool) {
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

func (v *dnsCacheVal) copy() []string {
	cpy := make([]string, 0, len(v.names))
	for n := range v.names {
		cpy = append(cpy, n)
	}
	sort.Strings(cpy)
	return cpy
}

type translation struct {
	dns string
	ips map[util.Address]time.Time
}

func (t *translation) add(addr util.Address, ttl time.Duration) {
	if _, ok := t.ips[addr]; ok {
		return
	}
	t.ips[addr] = time.Now().Add(ttl)
}
