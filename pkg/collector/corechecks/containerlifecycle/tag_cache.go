// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"time"

	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

// tagCache caches tagger lookups for a short TTL to reduce query pressure on
// the tagger. Entries expire after the TTL; expired entries are swept on each
// put, keeping the map bounded by entities seen per TTL window.
//
// Only the processQueues goroutine accesses the cache (via flush -> enrichTags),
// so no locking is required.
type tagCache struct {
	ttl time.Duration
	now func() time.Time
	// entries maps a tagger entity ID to its tags and expiry time. Empty tag
	// slices are cached like any other result (negative caching).
	entries map[taggertypes.EntityID]tagCacheEntry
}

type tagCacheEntry struct {
	tags    []string
	expires time.Time
}

func newTagCache(ttl time.Duration) *tagCache {
	return &tagCache{
		ttl:     ttl,
		now:     time.Now,
		entries: map[taggertypes.EntityID]tagCacheEntry{},
	}
}

// get returns the cached tags for the entity and whether a valid entry exists.
// A nil cache behaves like an always-empty cache.
func (c *tagCache) get(entityID taggertypes.EntityID) ([]string, bool) {
	if c == nil {
		return nil, false
	}

	entry, found := c.entries[entityID]
	if !found || c.now().After(entry.expires) {
		return nil, false
	}
	return entry.tags, true
}

// put stores the tags for the entity and sweeps expired entries.
// put is a no-op on a nil cache or when the TTL is zero or negative (caching disabled).
func (c *tagCache) put(entityID taggertypes.EntityID, tags []string) {
	if c == nil || c.ttl <= 0 {
		return
	}

	c.sweep()
	c.entries[entityID] = tagCacheEntry{
		tags:    tags,
		expires: c.now().Add(c.ttl),
	}
}

// sweep removes expired entries from the map.
func (c *tagCache) sweep() {
	now := c.now()
	for entityID, entry := range c.entries {
		if now.After(entry.expires) {
			delete(c.entries, entityID)
		}
	}
}
