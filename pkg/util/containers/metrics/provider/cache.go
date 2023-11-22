// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package provider

import (
	"sync"
	"time"
)

const cacheGCInterval = 30 * time.Second

type cacheEntry struct {
	value     interface{}
	err       error
	timestamp time.Time
}

// Cache provides a caching mechanism based on staleness toleration provided by requestor
type Cache struct {
	cache       map[string]cacheEntry
	cacheLock   sync.RWMutex
	gcInterval  time.Duration
	gcTimestamp time.Time
}

// NewCache returns a new cache dedicated to a collector
func NewCache(gcInterval time.Duration) *Cache {
	return &Cache{
		cache:      make(map[string]cacheEntry),
		gcInterval: gcInterval,
	}
}

// Get retrieves data from cache, returns not found if cacheValidity == 0
func (c *Cache) Get(currentTime time.Time, key string, cacheValidity time.Duration) (interface{}, bool, error) {
	if cacheValidity <= 0 {
		return nil, false, nil
	}

	c.cacheLock.RLock()
	entry, found := c.cache[key]
	c.cacheLock.RUnlock()

	if !found || currentTime.Sub(entry.timestamp) > cacheValidity {
		return nil, false, nil
	}

	if entry.err != nil {
		return nil, true, entry.err
	}

	return entry.value, true, nil
}

// Store retrieves data from cache
func (c *Cache) Store(currentTime time.Time, key string, value interface{}, err error) {
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()

	if currentTime.Sub(c.gcTimestamp) > c.gcInterval {
		c.cache = make(map[string]cacheEntry, len(c.cache))
		c.gcTimestamp = currentTime
	}

	c.cache[key] = cacheEntry{value: value, timestamp: currentTime, err: err}
}
