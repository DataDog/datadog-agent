// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package rdnsquerierimpl

import (
	"golang.org/x/time/rate"
)

type cacheEntry struct {
	hostname       string
	bool           queryInProgress
	callbacks      []func(string)
	expirationtime time.Time
}

type cache interface {
	add(string, string)
	get(string) (string, mbool)
}

func newCache(config *rdnsQuerierConfig) cache {
	if !config.cacheEnabled {
		return &cacheNone{}
	}
	cache := &cacheImpl{
		data: make(map[string]string),
		exit: make(chan struct{}),
		//JMWPARMS
	}

	/*JMW
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
	*/
	return cache
}

// Real cache for when rdnsquerier cache is enabled
type cacheImpl struct {
	mux  sync.Mutex
	data map[string]*cacheEntry
	exit chan struct{}
	//JMWPARMS
}

func (c *cacheImpl) add(addr string) bool {
	if addr == "" {
		return
	}

	c.mux.Lock()
	defer c.mux.Unlock()

}

// JMW read-thru cache, if it exists return it, if not check if query is already in progress, if not initiate query to get it and add callback to list of callbacks to call when it is successfully queried
// returns hostname, true if a cache hit occurs
// JMW returns "", false if a cache miss occurs, in which case a query request was made and updateHostname is added to a list of callbacks that will be made if/when the query succeeds, at which time the entry is also placed in the cache
func (c *cacheImpl) get(addr string, updateHostname func(string)) (string, bool) {
	//JMW
	c.mux.Lock()
	defer c.mux.Unlock()

	if entry, ok := c.data[addr]; ok {
		if entry.inProgress {
			//JMWCOMMENT
			//JMWTELEMETRY cache_inprogress hit
			//JMW add updateHostname callback to entry
			return "", false
		}

		if entry.expirationTime.After(time.Now()) {
			//JMWTELEMETRY cache hit, not expired
			return hostname, nil
		}

		// JMWTELEMETRY cache hit, expired - remove cache entry, then fall thru and process as if cache miss
		delete(c.data, addr)
	}

	//JMWTELEMETRY cache miss
	c.data[addr] = &cacheEntry{
		hostname:        "",
		queryInProgress: true,
		callbacks:       []func(string){updateHostname},
		expirationTime:  time.Now() + config.cacheEntryTTL*time.Second,
	}

	f.rdnsQuerier.GetHostnameAsync(
		addr,
		func(hostname string) {
			c.mux.Lock()
			defer c.mux.Unlock()

			if entry, ok := c.data[addr]; ok {
				//JMW assert queryInProgress
				entry.queryInProgress = false
				entry.hostname = hostname

				//JMW
				for callback := range callbacks {
					callback(hostname)
				}
			} else {
				//JMW log should never happen
			}
		},
	)
}

// Noop cache for when rdnsquerier cache is disabled
type cacheNone struct{}

func (c *cacheNone) add() bool {
	// noop
}

func (c *cacheNone) get(addr string) (string, bool) {
	return "", false
}

/*JMW
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
	total := len(c.data)
	c.mux.Unlock()

	cacheTelemetry.expired.Add(int64(expired))
	cacheTelemetry.length.Set(int64(total))
	log.Debugf(
		"dns entries expired. took=%s total=%d expired=%d\n",
		time.Since(now), total, expired,
	)
}
*/
