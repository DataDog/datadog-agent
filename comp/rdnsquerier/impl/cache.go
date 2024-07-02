// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package rdnsquerierimpl

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
)

type cacheEntry struct {
	hostname        string
	queryInProgress bool
	callbacks       []func(string)
	expirationTime  time.Time
}

type cache interface {
	get(string, func(string)) (string, bool)
}

// Cache implementation for when rdnsquerier cache is enabled
type cacheImpl struct {
	config         *rdnsQuerierConfig
	logger         log.Component
	cacheTelemetry *cacheTelemetry

	mutex sync.Mutex
	data  map[string]*cacheEntry
	//JMWexit chan struct{}
	//JMWPARMS

	rdnsQueryChan chan *rdnsQuery
}

type cacheTelemetry = struct {
	hit             telemetry.Counter
	hitExpired      telemetry.Counter
	hitInProgress   telemetry.Counter
	miss            telemetry.Counter
	chanAdded       telemetry.Counter
	droppedChanFull telemetry.Counter
}

const cacheModuleName = "reverse_dns_enrichment_cache" //JMWNAME JMWMOVE

func newCache(config *rdnsQuerierConfig, logger log.Component, telemetry telemetry.Component, rdnsQueryChan chan *rdnsQuery) cache {
	if !config.cacheEnabled {
		logger.Debugf("JMW Cache disabled - returning cacheNone")
		return &cacheNone{
			rdnsQueryChan: rdnsQueryChan,
		}
	}

	cacheTelemetry := &cacheTelemetry{
		telemetry.NewCounter(cacheModuleName, "hit", []string{}, "Counter measuring the number of successful rDNS cache hits"),
		telemetry.NewCounter(cacheModuleName, "hit_expired", []string{}, "Counter measuring the number of expired rDNS cache hits"),
		telemetry.NewCounter(cacheModuleName, "hit_inprogress", []string{}, "Counter measuring the number of in progress rDNS cache hits"),
		telemetry.NewCounter(cacheModuleName, "miss", []string{}, "Counter measuring the number of rDNS cache misses"),
		telemetry.NewCounter(cacheModuleName, "chan_added", []string{}, "Counter measuring the number of rDNS requests added to the channel"),
		telemetry.NewCounter(cacheModuleName, "dropped_chan_full", []string{}, "Counter measuring the number of rDNS requests dropped because the channel was full"),
	}

	cache := &cacheImpl{
		config:         config,
		logger:         logger,
		cacheTelemetry: cacheTelemetry,

		data: make(map[string]*cacheEntry),
		//JMWexit: make(chan struct{}), // JMW or pass ctx like ratelimiter?

		rdnsQueryChan: rdnsQueryChan,
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
	logger.Debugf("JMW Cache enabled - returning cacheImpl")
	return cache
}

// JMW read-thru cache, if it exists return it, if not check if query is already in progress, if not initiate query to get it and add callback to list of callbacks to call when it is successfully queried
// returns hostname, true if a cache hit occurs
// JMW returns "", false if a cache miss occurs, in which case a query request was made and updateHostname is added to a list of callbacks that will be made if/when the query succeeds, at which time the entry is also placed in the cache
func (c *cacheImpl) get(addr string, updateHostname func(string)) (string, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if entry, ok := c.data[addr]; ok {
		if entry.queryInProgress {
			//JMWCOMMENT
			//JMWTELEMETRY cache_inprogress hit
			c.cacheTelemetry.hitInProgress.Inc()
			//JMW add updateHostname callback to entry
			entry.callbacks = append(entry.callbacks, updateHostname)
			c.logger.Debugf("JMW Cache hit (in progress) for %s - added callback - callbacks slice size %d", addr, len(entry.callbacks))
			return "", false
		}

		if entry.expirationTime.After(time.Now()) {
			//JMWTELEMETRY cache hit, not expired
			c.cacheTelemetry.hit.Inc()
			c.logger.Debugf("JMW Cache hit (not expired) for addr %s hostname %s", addr, entry.hostname)
			return entry.hostname, true
		}

		// JMWTELEMETRY cache hit, expired - remove cache entry, then fall thru and process as if cache miss
		//JMW assert !entry.queryInProgress
		c.cacheTelemetry.hitExpired.Inc()
		c.logger.Debugf("JMW Cache hit (expired) for addr %s - falling thru to cache miss path", addr)
		delete(c.data, addr)
	}

	//JMWTELEMETRY cache miss
	c.cacheTelemetry.miss.Inc()
	c.data[addr] = &cacheEntry{
		hostname:        "",
		queryInProgress: true,
		callbacks:       []func(string){updateHostname},
	}
	c.logger.Debugf("JMW Cache miss for addr %s - created cacheEntry %+v - cache size %d", addr, c.data[addr], len(c.data))

	//JMWDUP
	query := &rdnsQuery{
		addr,
		func(hostname string) {
			c.mutex.Lock()
			defer c.mutex.Unlock()

			if entry, ok := c.data[addr]; ok {
				//JMW assert queryInProgress
				entry.queryInProgress = false
				entry.hostname = hostname
				entry.expirationTime = time.Now().Add(time.Duration(c.config.cacheEntryTTL) * time.Second)

				//JMW
				c.logger.Debugf("JMW lookup successful - Cache entry updated for addr %s hostname %s - calling %d callbacks", addr, hostname, len(entry.callbacks))
				for _, callback := range entry.callbacks {
					callback(hostname)
				}
			} else {
				//JMW log should never happen
				c.logger.Debugf("JMW lookup successful - Cache entry not found for addr %s hostname %s - shouldn't happen", addr, hostname)
			}
		},
	}

	select {
	case c.rdnsQueryChan <- query:
		c.cacheTelemetry.chanAdded.Inc()
	default:
		c.cacheTelemetry.droppedChanFull.Inc()
		c.logger.Debugf("Reverse DNS Enrichment channel is full, dropping query for IP address %s - removing cache entry", addr)
		delete(c.data, addr)
	}

	return "", false
}

// Noop cache for when rdnsquerier cache is disabled
type cacheNone struct {
	rdnsQueryChan chan *rdnsQuery
}

func (c *cacheNone) get(addr string, updateHostname func(string)) (string, bool) {
	//JMWDUP
	query := &rdnsQuery{
		addr,
		updateHostname,
	}

	select {
	case c.rdnsQueryChan <- query:
		//JMWJMWc.cacheTelemetry.chanAdded.Inc()
	default:
		//JMWJMWc.cacheTelemetry.droppedChanFull.Inc()
		//JMWJMWc.logger.Debugf("Reverse DNS Enrichment channel is full, dropping query for IP address %s", query.addr)
	}

	return "", false
}

/*JMW
func (c *reverseDNSCache) Close() {
	close(c.exit)
}

func (c *reverseDNSCache) Expire(now time.Time) {
	expired := 0
	c.mutex.Lock()
	for addr, entry := range c.data {
		if entry.queryInProgress {
			continue
		}

		if entry.inUse {
			continue
		}

		for ip, deadline := range entry.names {
			if deadline.Before(now) {
				delete(entry.names, ip)
			}
		}

		if len(entry.names) != 0 {
			continue
		}
		expired++
		delete(c.data, addr)
	}
	total := len(c.data)
	c.mutex.Unlock()

	cacheTelemetry.expired.Add(int64(expired))
	cacheTelemetry.length.Set(int64(total))
	log.Debugf(
		"dns entries expired. took=%s total=%d expired=%d\n",
		time.Since(now), total, expired,
	)
    //JMWTELEMETRY set cache size gauge?
}
*/
