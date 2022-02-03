// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package provider

import (
	"sync"
	"time"
)

const (
	contStatsCachePrefix    = "cs-"
	contNetStatsCachePrefix = "cns-"
	gcInterval              = 30 * time.Second
)

type cacheEntry struct {
	value     interface{}
	err       error
	timestamp time.Time
}

// collectorCache is a wrapper handling cache for collectors.
// this cache is fully synchronized to minimize locking. If a method is called multiple times in parallel for the same cachey key,
// it may result in multiple calls to the underlying collector
type collectorCache struct {
	collector   Collector
	cache       map[string]cacheEntry
	cacheLock   sync.RWMutex
	gcTimestamp time.Time
}

// NewCollectorCache returns a new cache dedicated to a collector
func NewCollectorCache(collector Collector) Collector {
	return &collectorCache{
		collector: collector,
		cache:     make(map[string]cacheEntry),
	}
}

// ID returns the actual collector ID, no cache
func (cc *collectorCache) ID() string {
	return cc.collector.ID()
}

// GetContainerStats returns the stats if in cache and within cacheValidity
// errors are cached as well to avoid hammering underlying collector
func (cc *collectorCache) GetContainerStats(containerID string, cacheValidity time.Duration) (*ContainerStats, error) {
	currentTime := time.Now()
	cacheKey := contStatsCachePrefix + containerID

	if cacheValidity > 0 {
		entry, found, err := cc.getCacheEntry(currentTime, cacheKey, cacheValidity)
		if found {
			if err != nil {
				return nil, err
			}

			return entry.(*ContainerStats), nil
		}
	}

	// No cache, cacheValidity is 0 or too old value
	cstats, err := cc.collector.GetContainerStats(containerID, cacheValidity)
	if err != nil {
		cc.storeCacheEntry(currentTime, cacheKey, nil, err)
		return nil, err
	}

	cc.storeCacheEntry(currentTime, cacheKey, cstats, nil)
	return cstats, nil
}

// GetContainerNetworkStats returns the stats if in cache and within cacheValidity
// errors are cached as well to avoid hammering underlying collector
func (cc *collectorCache) GetContainerNetworkStats(containerID string, cacheValidity time.Duration) (*ContainerNetworkStats, error) {
	// Generics could be useful. Meanwhile copy-paste.
	currentTime := time.Now()
	cacheKey := contNetStatsCachePrefix + containerID

	if cacheValidity > 0 {
		entry, found, err := cc.getCacheEntry(currentTime, cacheKey, cacheValidity)
		if found {
			if err != nil {
				return nil, err
			}

			return entry.(*ContainerNetworkStats), nil
		}
	}

	// No cache, cacheValidity is 0 or too old value
	val, err := cc.collector.GetContainerNetworkStats(containerID, cacheValidity)
	if err != nil {
		cc.storeCacheEntry(currentTime, cacheKey, nil, err)
		return nil, err
	}

	cc.storeCacheEntry(currentTime, cacheKey, val, nil)
	return val, nil
}

func (cc *collectorCache) getCacheEntry(currentTime time.Time, key string, cacheValidity time.Duration) (interface{}, bool, error) {
	cc.cacheLock.RLock()
	entry, found := cc.cache[key]
	cc.cacheLock.RUnlock()

	if !found {
		return nil, false, nil
	}

	if currentTime.Sub(entry.timestamp) > cacheValidity {
		return nil, false, nil
	}

	if entry.err != nil {
		return nil, true, entry.err
	}

	return entry.value, true, nil
}

func (cc *collectorCache) storeCacheEntry(currentTime time.Time, key string, value interface{}, err error) {
	cc.cacheLock.Lock()
	defer cc.cacheLock.Unlock()

	if currentTime.Sub(cc.gcTimestamp) > gcInterval {
		cc.cache = make(map[string]cacheEntry, len(cc.cache))
		cc.gcTimestamp = currentTime
	}

	cc.cache[key] = cacheEntry{value: value, timestamp: currentTime, err: err}
}
