// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"sync"
	"time"
)

// rolloutCacheEntry represents a cached rollout duration result
type rolloutCacheEntry struct {
	duration  float64
	timestamp time.Time
}

// rolloutCache provides caching for rollout duration calculations to reduce API calls.
// 
// The cache stores duration calculations with a configurable TTL to avoid repeated
// expensive API calls to ControllerRevisions and ReplicaSets. Cache keys include
// resource identity and generation/revision information to ensure cache invalidation
// when rollouts change state.
//
// Key format examples:
//   - StatefulSet: "statefulset:namespace/name:revisionName"
//   - Deployment:  "deployment:namespace/name:generation"
//
// This reduces API calls from O(resources*scrapes) to O(resources*rollout_changes).
type rolloutCache struct {
	entries map[string]*rolloutCacheEntry
	mutex   sync.RWMutex
	ttl     time.Duration
}

// newRolloutCache creates a new rollout cache with the specified TTL
func newRolloutCache(ttl time.Duration) *rolloutCache {
	return &rolloutCache{
		entries: make(map[string]*rolloutCacheEntry),
		ttl:     ttl,
	}
}

// get retrieves a cached duration if it exists and is still valid
func (c *rolloutCache) get(key string) (float64, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return 0, false
	}

	// Check if entry has expired
	if time.Since(entry.timestamp) > c.ttl {
		return 0, false
	}

	return entry.duration, true
}

// set stores a duration in the cache
func (c *rolloutCache) set(key string, duration float64) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.entries[key] = &rolloutCacheEntry{
		duration:  duration,
		timestamp: time.Now(),
	}
}

// cleanup removes expired entries from the cache
func (c *rolloutCache) cleanup() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.Sub(entry.timestamp) > c.ttl {
			delete(c.entries, key)
		}
	}
}

// size returns the current number of entries in the cache
func (c *rolloutCache) size() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return len(c.entries)
}

// clear removes all entries from the cache
func (c *rolloutCache) clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.entries = make(map[string]*rolloutCacheEntry)
}