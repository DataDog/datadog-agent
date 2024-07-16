// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package rdnsquerierimpl

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
)

type cacheEntry struct {
	hostname         string
	callbacks        []func(string, error)
	expirationTime   time.Time
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

	ticker := time.NewTicker(c.config.cacheCleanInterval)
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

func (c *cacheImpl) stop() {
	close(c.exit)
	c.querier.stop()
}

func newCache(config *rdnsQuerierConfig, logger log.Component, internalTelemetry *rdnsQuerierTelemetry, querier querier) cache {
	if !config.cacheEnabled {
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

		if entry.expirationTime.After(time.Now()) {
			// cache hit (not expired) - invoke the sync callback
			c.internalTelemetry.cacheHit.Inc()
			updateHostnameSync(entry.hostname)
			return nil
		}

		// cache hit (expired) - remove the cache entry, then fall thru and process the same as a cache miss
		c.internalTelemetry.cacheHitExpired.Inc()
		delete(c.data, addr)
	}

	c.internalTelemetry.cacheMiss.Inc()

	// create an in progress cache entry
	c.data[addr] = &cacheEntry{
		hostname:         "",
		callbacks:        []func(string, error){updateHostnameAsync},
		retriesRemaining: c.config.cacheMaxRetries,
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
				entry.hostname = hostname
				entry.expirationTime = time.Now().Add(c.config.cacheEntryTTL)
				entry.queryInProgress = false

				callbacks := entry.callbacks
				entry.callbacks = nil

				if len(c.data) > c.config.cacheMaxSize {
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

func (c *cacheImpl) expire(startTime time.Time) {
	expired := 0
	c.mutex.Lock()
	for addr, entry := range c.data {
		if entry.queryInProgress {
			continue
		}

		if entry.expirationTime.Before(startTime) {
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
