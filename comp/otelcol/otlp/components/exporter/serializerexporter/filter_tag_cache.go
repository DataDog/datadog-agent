// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"sync"

	"github.com/twmb/murmur3"
)

// defaultFilterTagCacheCapacity bounds the number of distinct (metric name, tag
// set) combinations remembered by filterTagCache before the oldest entry is
// evicted.
const defaultFilterTagCacheCapacity = 10000

// filterTagCacheKey identifies a metric name and tag set combination. hash is
// an order-independent combination of the tag set, so the same set of tags
// produces the same key regardless of the order dimensions.Tags() returns them in.
type filterTagCacheKey struct {
	name string
	hash uint64
}

// hashTags combines the tags into a single order-independent hash for use as a
// filterTagCache key. It is not related to the hashing filterlist itself uses
// to match tag names; it only needs to detect a recurring identical tag set.
func hashTags(tags []string) uint64 {
	var h uint64
	for _, tag := range tags {
		h += murmur3.StringSum64(tag)
	}
	return h
}

// filterTagCache remembers the result of filtering a metric's tags against the
// metric_tag_filterlist, so that OTel series/sketches that recur across export
// batches (e.g. the same host or pod emitting the same series every collection
// interval) don't re-scan their tag set every time. It must be cleared whenever
// the underlying filterlist is updated (e.g. via remote config), otherwise it
// would keep serving results computed against a stale filter.
type filterTagCache struct {
	mu       sync.Mutex
	entries  map[filterTagCacheKey][]string
	order    []filterTagCacheKey
	capacity int
}

func newFilterTagCache(capacity int) *filterTagCache {
	return &filterTagCache{
		entries:  make(map[filterTagCacheKey][]string),
		capacity: capacity,
	}
}

func (c *filterTagCache) get(key filterTagCacheKey) ([]string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	tags, ok := c.entries[key]
	return tags, ok
}

// add stores the filtered tags for key, evicting the oldest entry if the cache
// is at capacity.
func (c *filterTagCache) add(key filterTagCacheKey, tags []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[key]; !exists {
		if len(c.order) >= c.capacity {
			oldest := c.order[0]
			c.order = c.order[1:]
			delete(c.entries, oldest)
		}
		c.order = append(c.order, key)
	}
	c.entries[key] = tags
}

// clear removes all entries. Called whenever the filterlist is updated so
// stale filtered results are never served after a config/remote-config change.
func (c *filterTagCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[filterTagCacheKey][]string)
	c.order = nil
}
