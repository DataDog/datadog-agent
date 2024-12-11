// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package rdnsquerierimpl

import (
	"encoding/json"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
)

type cacheEntry struct {
	Hostname         string
	callbacks        []func(string, error)
	ExpirationTime   time.Time
	retriesRemaining int
	queryInProgress  bool
}

type cache interface {
	start()
	stop()
	getHostname(string, func(string), func(string, error)) error
}

// Cache implementation when rdnsquerier cache is enabled
// The implementation is a read-through cache.
// If a requested IP address exists in the cache then its value is used, otherwise a query is initiated and the result is cached and returned asynchronously.
// Only one outstanding query request for an IP address is made at a time.
// Retries for failed lookups are automatically handled.
// The cache is cleaned periodically to remove expired entries.
// A maximum size is also enforced.
type cacheImpl struct {
	config            *rdnsQuerierConfig
	logger            log.Component
	internalTelemetry *rdnsQuerierTelemetry

	mutex sync.Mutex
	data  map[string]*cacheEntry
	exit  chan struct{}

	querier querier
}

func (c *cacheImpl) start() {
	c.querier.start()

	c.loadPersistentCache()
	c.runExpireLoop()
	c.runPersistLoop()
}

func (c *cacheImpl) stop() {
	close(c.exit)
	c.querier.stop()
	c.persist(time.Now())
}

func newCache(config *rdnsQuerierConfig, logger log.Component, internalTelemetry *rdnsQuerierTelemetry, querier querier) cache {
	if !config.cache.enabled {
		return &cacheNone{
			querier: querier,
		}
	}

	cache := &cacheImpl{
		config:            config,
		logger:            logger,
		internalTelemetry: internalTelemetry,

		data: make(map[string]*cacheEntry),
		exit: make(chan struct{}),

		querier: querier,
	}

	return cache
}

// getHostname attempts to resolve the hostname for the given IP address.
// If a cache entry exists the updateHostnameSync callback is called synchronously with the cached hostname,
// otherwise a query is initiated and the result is returned asynchronously via the updateHostnameAsync callback.
func (c *cacheImpl) getHostname(addr string, updateHostnameSync func(string), updateHostnameAsync func(string, error)) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if entry, ok := c.data[addr]; ok {
		if entry.queryInProgress {
			// The cache entry exists but the query is in progress.  Add updateHostnameAsync to list of callbacks to be called when query successfully completes.
			c.internalTelemetry.cacheHitInProgress.Inc()
			entry.callbacks = append(entry.callbacks, updateHostnameAsync)
			return nil
		}

		if entry.ExpirationTime.After(time.Now()) {
			// cache hit (not expired) - invoke the sync callback
			c.internalTelemetry.cacheHit.Inc()
			updateHostnameSync(entry.Hostname)
			return nil
		}

		// cache hit (expired) - remove the cache entry, then fall thru and process the same as a cache miss
		c.internalTelemetry.cacheHitExpired.Inc()
		delete(c.data, addr)
	}

	c.internalTelemetry.cacheMiss.Inc()

	// create an in progress cache entry
	c.data[addr] = &cacheEntry{
		Hostname:         "",
		callbacks:        []func(string, error){updateHostnameAsync},
		retriesRemaining: c.config.cache.maxRetries,
		queryInProgress:  true,
	}

	err := c.sendQuery(addr)
	if err != nil {
		// An error from sendQuery() indicates that the query was dropped due to the channel being full.  Delete the in progress entry
		// since it will never be completed.
		delete(c.data, addr)
		return err
	}

	return nil
}

// sendQuery initiates a query to resolve the hostname for the given IP address, and handles retries of failed lookups
func (c *cacheImpl) sendQuery(addr string) error {
	return c.querier.getHostnameAsync(
		addr,
		func(hostname string, err error) {
			if err != nil {
				c.logger.Debugf("Reverse DNS Enrichment error resolving IP address: %v error: %v", addr, err)

				// attempt retry
				c.mutex.Lock()

				if entry, ok := c.data[addr]; ok {
					if entry.retriesRemaining > 0 {
						c.internalTelemetry.cacheRetry.Inc()
						entry.retriesRemaining--

						// note that the retry is attempted without a delay because the rate limiter/circuit breaker will delay when necessary
						err := c.sendQuery(addr)
						if err != nil {
							// An error from sendQuery() indicates that the query was dropped due to the channel being full.  Delete the in progress entry
							// since it will never be completed.
							delete(c.data, addr)
						}
						c.mutex.Unlock()
						return
					}

					c.mutex.Unlock()

					c.internalTelemetry.cacheRetriesExceeded.Inc()
					// max retries exceeded, fall thru to update the cache and invoke callback(s) with the error
				}
			}

			// add the result to the cache and invoke callback(s)
			c.mutex.Lock()
			if entry, ok := c.data[addr]; ok {
				entry.Hostname = hostname
				entry.ExpirationTime = time.Now().Add(c.config.cache.entryTTL)
				entry.queryInProgress = false

				callbacks := entry.callbacks
				entry.callbacks = nil

				if len(c.data) > c.config.cache.maxSize {
					c.internalTelemetry.cacheMaxSizeExceeded.Inc()
					// cache size exceeds max, delete this entry
					// note that it is not expected to be common to exceed the max size, so this is simply a safeguard to prevent the cache from growing indefinitely
					// without the additional complexity and memory overhead of an LRU cache
					delete(c.data, addr)
				}

				c.mutex.Unlock()

				for _, callback := range callbacks {
					callback(hostname, err)
				}
			} else {
				// Response for an in progress query could not find the in progress cache entry.  This should never occur.
				c.mutex.Unlock()
			}
		},
	)
}

func (c *cacheImpl) runExpireLoop() {
	// call expire() periodically to remove expired entries from the cache
	ticker := time.NewTicker(c.config.cache.cleanInterval)
	go func() {
		for {
			select {
			case now := <-ticker.C:
				c.expire(now)
			case <-c.exit:
				ticker.Stop()
				return
			}
		}
	}()

}

func (c *cacheImpl) expire(startTime time.Time) {
	expired := 0
	c.mutex.Lock()
	for addr, entry := range c.data {
		if entry.queryInProgress {
			continue
		}

		if entry.ExpirationTime.Before(startTime) {
			expired++
			delete(c.data, addr)
		}
	}
	size := len(c.data)
	c.mutex.Unlock()

	c.internalTelemetry.cacheExpired.Add(float64(expired))
	c.internalTelemetry.cacheSize.Set(float64(size))
	c.logger.Debugf("Reverse DNS Enrichment %d cache entries expired, execution time=%s, cache size=%d", expired, time.Since(startTime), size)
}

func (c *cacheImpl) runPersistLoop() {
	// call persist() periodically to save the cache to persistent storage
	ticker := time.NewTicker(c.config.cache.persistInterval)
	go func() {
		for {
			select {
			case now := <-ticker.C:
				c.persist(now)
			case <-c.exit:
				ticker.Stop()
				return
			}
		}
	}()
}

const cachePersistKey = "reverse_dns_enrichment_persistent_cache-1"

func (c *cacheImpl) loadPersistentCache() {
	serializedData, err := persistentcache.Read(cachePersistKey)
	if err != nil {
		c.logger.Debugf("error reading cache for cachePersistKey %s: %v", cachePersistKey, err)
		return
	}
	if serializedData == "" {
		return
	}

	persistedMap := make(map[string]*cacheEntry)
	err = json.Unmarshal([]byte(serializedData), &persistedMap)
	if err != nil {
		_ = c.logger.Warnf("couldn't unmarshal cache for cachePersistKey %s: %v", cachePersistKey, err)
		return
	}

	now := time.Now()
	for ip, entry := range persistedMap {
		// remove expired entries
		if entry.ExpirationTime.Before(now) {
			delete(c.data, ip)
		}

		// Adjust ExpirationTime for entries that are too far in the future, which can occur if entryTTL
		// was decreased since the cache was persisted.
		if entry.ExpirationTime.After(now.Add(c.config.cache.entryTTL)) {
			entry.ExpirationTime = now.Add(c.config.cache.entryTTL)
		}
	}
	size := len(persistedMap)

	c.mutex.Lock()
	c.data = persistedMap
	c.mutex.Unlock()

	c.internalTelemetry.cacheSize.Set(float64(size))
	c.logger.Debugf("Reverse DNS Enrichment cache loaded from persistent storage, cache size=%d", size)
}

func (c *cacheImpl) serializeData(startTime time.Time) (string, int) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	mapCopy := make(map[string]*cacheEntry)
	for k, v := range c.data {
		if v.queryInProgress || v.ExpirationTime.Before(startTime) {
			continue
		}
		mapCopy[k] = v
	}

	size := len(mapCopy)
	if size == 0 {
		return "", 0
	}

	// Note that the lock must be held while calling json.Marshal on mapCopy since the values are pointers
	// to the same cacheEntry objects referenced by c.data.
	serializedData, err := json.Marshal(mapCopy)
	if err != nil {
		_ = c.logger.Warnf("Reverse DNS Enrichment cache persist failed - error marshalling cache: %v", err)
		return "", 0
	}

	return string(serializedData), size
}

func (c *cacheImpl) persist(startTime time.Time) {
	serializedData, size := c.serializeData(startTime)
	if serializedData == "" {
		return
	}

	err := persistentcache.Write(cachePersistKey, serializedData)
	if err != nil {
		_ = c.logger.Warnf("Reverse DNS Enrichment cache persist failed - error writing cache: %v", err)
	}

	c.logger.Debugf("Reverse DNS Enrichment %d cache entries persisted, execution time=%s", size, time.Since(startTime))
}

// Noop cache used when rdnsquerier cache is disabled
type cacheNone struct {
	querier querier
}

func (c *cacheNone) start() {
	c.querier.start()
}

func (c *cacheNone) stop() {
	c.querier.stop()
}

func (c *cacheNone) getHostname(addr string, _ func(string), updateHostnameAsync func(string, error)) error {
	return c.querier.getHostnameAsync(addr, updateHostnameAsync)
}
