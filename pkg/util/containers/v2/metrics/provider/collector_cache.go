// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package provider

import (
	"strconv"
	"time"
)

const (
	contCoreStatsCachePrefix = "cs-"
	contOpenFilesCachePrefix = "of-"
	contNetStatsCachePrefix  = "cns-"
	contPidToCidCachePrefix  = "pid-"

	cacheGCInterval = 30 * time.Second
)

// collectorCache is a wrapper handling cache for collectors.
// this cache is fully synchronized to minimize locking. If a method is called multiple times in parallel for the same cachey key,
// it may result in multiple calls to the underlying collector
type collectorCache struct {
	collector            Collector
	cache                *Cache
	selfContainerIDCache string
}

// NewCollectorCache returns a new cache dedicated to a collector
func NewCollectorCache(collector Collector) Collector {
	return &collectorCache{
		collector: collector,
		cache:     NewCache(cacheGCInterval),
	}
}

// ID returns the actual collector ID, no cache
func (cc *collectorCache) ID() string {
	return cc.collector.ID()
}

// GetContainerStats returns the stats if in cache and within cacheValidity
// errors are cached as well to avoid hammering underlying collector
func (cc *collectorCache) GetContainerStats(containerNS, containerID string, cacheValidity time.Duration) (*ContainerStats, error) {
	currentTime := time.Now()
	cacheKey := contCoreStatsCachePrefix + containerNS + containerID

	entry, found, err := cc.cache.Get(currentTime, cacheKey, cacheValidity)
	if found {
		if err != nil {
			return nil, err
		}

		return entry.(*ContainerStats), nil
	}

	// No cache, cacheValidity is 0 or too old value
	cstats, err := cc.collector.GetContainerStats(containerNS, containerID, cacheValidity)
	if err != nil {
		cc.cache.Store(currentTime, cacheKey, nil, err)
		return nil, err
	}

	cc.cache.Store(currentTime, cacheKey, cstats, nil)
	return cstats, nil
}

// GetContainerOpenFilesCount returns the count of open files if in cache and within cacheValidity
// errors are cached as well to avoid hammering underlying collector
func (cc *collectorCache) GetContainerOpenFilesCount(containerNS, containerID string, cacheValidity time.Duration) (*uint64, error) {
	// Generics could be useful. Meanwhile copy-paste.
	currentTime := time.Now()
	cacheKey := contOpenFilesCachePrefix + containerNS + containerID

	entry, found, err := cc.cache.Get(currentTime, cacheKey, cacheValidity)
	if found {
		if err != nil {
			return nil, err
		}

		return entry.(*uint64), nil
	}

	// No cache, cacheValidity is 0 or too old value
	openFilesCount, err := cc.collector.GetContainerOpenFilesCount(containerNS, containerID, cacheValidity)
	if err != nil {
		cc.cache.Store(currentTime, cacheKey, nil, err)
		return nil, err
	}

	cc.cache.Store(currentTime, cacheKey, openFilesCount, nil)
	return openFilesCount, nil
}

// GetContainerNetworkStats returns the stats if in cache and within cacheValidity
// errors are cached as well to avoid hammering underlying collector
func (cc *collectorCache) GetContainerNetworkStats(containerNS, containerID string, cacheValidity time.Duration) (*ContainerNetworkStats, error) {
	// Generics could be useful. Meanwhile copy-paste.
	currentTime := time.Now()
	cacheKey := contNetStatsCachePrefix + containerNS + containerID

	entry, found, err := cc.cache.Get(currentTime, cacheKey, cacheValidity)
	if found {
		if err != nil {
			return nil, err
		}

		return entry.(*ContainerNetworkStats), nil
	}

	// No cache, cacheValidity is 0 or too old value
	val, err := cc.collector.GetContainerNetworkStats(containerNS, containerID, cacheValidity)
	if err != nil {
		cc.cache.Store(currentTime, cacheKey, nil, err)
		return nil, err
	}

	cc.cache.Store(currentTime, cacheKey, val, nil)
	return val, nil
}

// GetContainerIDForPID returns the container ID for given PID
// errors are cached as well to avoid hammering underlying collector
func (cc *collectorCache) GetContainerIDForPID(pid int, cacheValidity time.Duration) (string, error) {
	// Generics could be useful. Meanwhile copy-paste.
	currentTime := time.Now()
	cacheKey := contPidToCidCachePrefix + strconv.FormatInt(int64(pid), 10)

	entry, found, err := cc.cache.Get(currentTime, cacheKey, cacheValidity)
	if found {
		if err != nil {
			return "", err
		}

		return entry.(string), nil
	}

	// No cache, cacheValidity is 0 or too old value
	val, err := cc.collector.GetContainerIDForPID(pid, cacheValidity)
	if err != nil {
		cc.cache.Store(currentTime, cacheKey, nil, err)
		return "", err
	}

	cc.cache.Store(currentTime, cacheKey, val, nil)
	return val, nil
}

// GetSelfContainerID returns current process container ID
// No caching as it's not supposed to change
func (cc *collectorCache) GetSelfContainerID() (string, error) {
	if cc.selfContainerIDCache != "" {
		return cc.selfContainerIDCache, nil
	}

	selfID, err := cc.collector.GetSelfContainerID()
	if err == nil {
		cc.selfContainerIDCache = selfID
	}

	return selfID, err
}
