package ebpf

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type reverseDNSCache struct {
	mux  sync.Mutex
	data map[util.Address]*dnsCacheVal
	exit chan struct{}
	wg   sync.WaitGroup
	ttl  time.Duration
}

type dnsCacheVal struct {
	name       string
	expiration int64
}

func newReverseDNSCache(ttl, expirationPeriod time.Duration) *reverseDNSCache {
	cache := &reverseDNSCache{
		data: make(map[util.Address]*dnsCacheVal),
		exit: make(chan struct{}),
		ttl:  ttl,
	}

	ticker := time.NewTicker(expirationPeriod)
	cache.wg.Add(1)
	go func() {
		for {
			select {
			case <-ticker.C:
				cache.expire()
			case <-cache.exit:
				cache.wg.Done()
				return
			}
		}
	}()
	return cache
}

func (c *reverseDNSCache) Add(translations ...*translation) {
	if len(translations) == 0 {
		return
	}

	exp := time.Now().Add(c.ttl).Unix()
	c.mux.Lock()
	for _, t := range translations {
		for addr := range t.ips {
			val, ok := c.data[addr]
			if ok {
				val.expiration = exp
				continue
			}

			c.data[addr] = &dnsCacheVal{name: t.name, expiration: exp}
		}
	}
	c.mux.Unlock()
}

func (c *reverseDNSCache) Get(conns []ConnectionStats) []NamePair {
	if len(conns) == 0 {
		return nil
	}

	names := make([]NamePair, len(conns))
	expiration := time.Now().Add(c.ttl).Unix()

	c.mux.Lock()
	for i, conn := range conns {
		names[i].Source = c.getNameForIP(conn.Source, expiration)
		names[i].Dest = c.getNameForIP(conn.Dest, expiration)
	}
	c.mux.Unlock()
	return names
}

func (c *reverseDNSCache) Close() {
	close(c.exit)
	c.wg.Wait()
}

func (c *reverseDNSCache) getNameForIP(ip util.Address, expiration int64) string {
	val, ok := c.data[ip]
	if !ok {
		return ""
	}

	val.expiration = expiration
	return val.name
}

func (c *reverseDNSCache) expire() {
	expired := 0
	start := time.Now()
	deadline := start.Add(-c.ttl).Unix()

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

	log.Debugf(
		"dns entries expired. took=%s total=%d expired=%d\n",
		time.Now().Sub(start), total, expired,
	)
}
