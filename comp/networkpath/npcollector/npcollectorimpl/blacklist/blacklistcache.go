// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package blacklist stores low-value pathtests in a cache so they don't get tracerouted again.
package blacklist

import (
	"math/rand"
	"sync"
	"time"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/common"
)

const (
	networkPathBlacklistMetricPrefix = "datadog.network_path.blacklist."
)

type cacheEntry struct {
	expiration time.Time
}

func (e cacheEntry) isExpired(now time.Time) bool {
	return now.After(e.expiration)
}

// Config is config for the blacklist cache.
type Config struct {
	// Capacity is the maximum number of entries in the cache.
	Capacity int
	// TTL is how long an entry will linger before expiring.
	TTL time.Duration
	// CleanInterval is how often to run a job that cleans up expired entries.
	CleanInterval time.Duration
}

// Cache contains a blacklist of known low-value pathtests.
type Cache struct {
	mutex     sync.Mutex
	config    Config
	blacklist map[common.PathtestHash]cacheEntry
	timeNow   func() time.Time
}

// NewCache creates a new pathtest blacklist cache.
func NewCache(config Config, timeNow func() time.Time) *Cache {
	return &Cache{
		config:    config,
		blacklist: make(map[common.PathtestHash]cacheEntry),
		timeNow:   timeNow,
	}
}

// Add adds a key to the blacklist cache. Contains will return true for this key until the TTL expires.
func (c *Cache) Add(key common.PathtestHash) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(c.blacklist) >= c.config.Capacity {
		return false
	}

	expiration := c.timeNow().Add(c.config.TTL)
	c.blacklist[key] = cacheEntry{expiration: expiration}

	return true
}

// Contains returns whether the cache has this pathtest key blacklisted.
func (c *Cache) Contains(key common.PathtestHash) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	_, ok := c.blacklist[key]
	return ok
}

// GetBlacklistCount returns the size of the cache
func (c *Cache) GetBlacklistCount() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return len(c.blacklist)
}

func (c *Cache) cleanupExpired() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := c.timeNow()

	for key, entry := range c.blacklist {
		if entry.isExpired(now) {
			delete(c.blacklist, key)
		}
	}
}

func addJitter(duration time.Duration, factor float64) time.Duration {
	randomFactor := rand.Float64() * factor
	jitter := time.Duration(float64(duration) * randomFactor)
	return duration + jitter
}

// CleanupTask runs a periodic task to remove expired blacklist entries from the cache.
func (c *Cache) CleanupTask(exit <-chan struct{}) {
	cleanInterval := addJitter(c.config.CleanInterval, 0.1)
	ticker := time.NewTicker(cleanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-exit:
			return
		case <-ticker.C:
			c.cleanupExpired()
		}
	}
}

func (c *Cache) reportMetrics(statsd ddgostatsd.ClientInterface) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	statsd.Gauge(networkPathBlacklistMetricPrefix+"size", float64(len(c.blacklist)), nil, 1)      //nolint:errcheck
	statsd.Gauge(networkPathBlacklistMetricPrefix+"capacity", float64(c.config.Capacity), nil, 1) //nolint:errcheck
}

// MetricsTask runs a periodic task to report metrics about the blacklist cache (mainly consumption, capacity)
func (c *Cache) MetricsTask(exit <-chan struct{}, statsd ddgostatsd.ClientInterface) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	c.reportMetrics(statsd)

	for {
		select {
		case <-exit:
			return
		case <-ticker.C:
			c.reportMetrics(statsd)
		}
	}
}
