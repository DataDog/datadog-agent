package ebpf

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type reverseDNSCache struct {
	mux  sync.Mutex
	data map[util.Address]*dnsCacheVal
	exit chan struct{}
	ttl  time.Duration
	size int

	// Telemetry
	length   int64
	lookups  int64
	resolved int64
}

func newReverseDNSCache(size int, ttl, expirationPeriod time.Duration) *reverseDNSCache {
	cache := &reverseDNSCache{
		data: make(map[util.Address]*dnsCacheVal),
		exit: make(chan struct{}),
		ttl:  ttl,
		size: size,
	}

	ticker := time.NewTicker(expirationPeriod)
	go func() {
		for {
			select {
			case now := <-ticker.C:
				cache.Expire(now)
			case <-cache.exit:
				return
			}
		}
	}()
	return cache
}

func (c *reverseDNSCache) Add(translation *translation, now time.Time) bool {
	if translation == nil {
		return false
	}

	c.mux.Lock()
	defer c.mux.Unlock()
	if len(c.data) >= c.size {
		return false
	}

	exp := now.Add(c.ttl).UnixNano()
	for addr := range translation.ips {
		val, ok := c.data[addr]
		if ok {
			val.expiration = exp
			val.merge(translation.name)
			continue
		}

		c.data[addr] = &dnsCacheVal{names: []string{translation.name}, expiration: exp}
	}

	// Update cache length for telemetry purposes
	atomic.StoreInt64(&c.length, int64(len(c.data)))

	return true
}

func (c *reverseDNSCache) Get(conns []ConnectionStats, now time.Time) map[util.Address][]string {
	if len(conns) == 0 {
		return nil
	}

	var (
		ipToNames  = make(map[util.Address][]string)
		visited    = make(map[util.Address]struct{})
		expiration = now.Add(c.ttl).UnixNano()
	)

	collectNamesForIP := func(addr util.Address) {
		if _, ok := visited[addr]; ok {
			return
		}
		visited[addr] = struct{}{}

		if names := c.getNamesForIP(addr, expiration); names != nil {
			ipToNames[addr] = names
		}
	}

	c.mux.Lock()
	for _, conn := range conns {
		collectNamesForIP(conn.Source)
		collectNamesForIP(conn.Dest)
	}
	c.mux.Unlock()

	// Update stats for telemetry
	atomic.AddInt64(&c.lookups, int64(len(visited)))
	atomic.AddInt64(&c.resolved, int64(len(ipToNames)))

	return ipToNames
}

func (c *reverseDNSCache) Len() int {
	return int(atomic.LoadInt64(&c.length))
}

func (c *reverseDNSCache) Stats() map[string]int64 {
	var (
		lookups  = atomic.SwapInt64(&c.lookups, 0)
		resolved = atomic.SwapInt64(&c.resolved, 0)
		ips      = int64(c.Len())
	)

	return map[string]int64{
		"lookups":  lookups,
		"resolved": resolved,
		"ips":      ips,
	}
}

func (c *reverseDNSCache) Close() {
	c.exit <- struct{}{}
}

func (c *reverseDNSCache) Expire(now time.Time) {
	deadline := now.UnixNano()
	expired := 0
	c.mux.Lock()
	for addr, val := range c.data {
		if val.expiration > deadline {
			continue
		}

		expired++
		delete(c.data, addr)
	}
	total := len(c.data)
	c.mux.Unlock()

	atomic.StoreInt64(&c.length, int64(total))
	log.Debugf(
		"dns entries expired. took=%s total=%d expired=%d\n",
		time.Now().Sub(now), total, expired,
	)
}

func (c *reverseDNSCache) getNamesForIP(ip util.Address, updatedTTL int64) []string {
	val, ok := c.data[ip]
	if !ok {
		return nil
	}

	val.expiration = updatedTTL
	return val.copy()
}

type dnsCacheVal struct {
	// opting for a []string instead of map[string]struct{} since common case is len(names) == 1
	names      []string
	expiration int64
}

func (v *dnsCacheVal) merge(name string) {
	if sort.SearchStrings(v.names, name) < len(v.names) {
		return
	}

	v.names = append(v.names, name)
	sort.Strings(v.names)
}

func (v *dnsCacheVal) copy() []string {
	cpy := make([]string, len(v.names))
	copy(cpy, v.names)
	return cpy
}

type translation struct {
	name string
	ips  map[util.Address]struct{}
}

func newTranslation(domain []byte) *translation {
	return &translation{
		name: string(domain),
		ips:  make(map[util.Address]struct{}),
	}
}

func (t *translation) add(addr util.Address) {
	t.ips[addr] = struct{}{}
}
