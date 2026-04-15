// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import "github.com/DataDog/datadog-agent/pkg/aggregator/ckey"

const reverseCacheCapacity = 1000

// tagFilterCacheEntry holds the post-strip context and tag keys for a given pre-strip context key.
type tagFilterCacheEntry struct {
	contextKey  ckey.ContextKey
	taggerKey   ckey.TagsKey
	metricKey   ckey.TagsKey
	removedTags int // total number of tags removed by RetainFunc (tagger + metric)
}

// tagFilterCache maintains a bidirectional mapping between pre-filter and post-strip context keys.
// The forward map (cache) allows O(1) lookup of previously computed post-tagFilter keys.
// The reverse map uses a ring buffer per post-tagFilter key to bound memory when high-churn
// tags are tagFilterped but the post-filter key remains active.
type tagFilterCache struct {
	// cache maps a pre-tagFilter contextKey to the post-filter (contextKey, taggerKey, metricKey).
	// This avoids repeated RetainFunc calls for metrics we have already processed.
	cache map[ckey.ContextKey]tagFilterCacheEntry
	// reverseCache maps a post-tagFilter contextKey back to the pre-filter contextKeys that produced it.
	// This allows O(1) cleanup of tagFilterCache entries when a context is removed from contextsByKey.
	reverseCache map[ckey.ContextKey]*reverseCacheRing
}

// newTagFilterCache returns an initialized empty filterCache.
func newTagFilterCache() *tagFilterCache {
	return &tagFilterCache{
		cache:        make(map[ckey.ContextKey]tagFilterCacheEntry),
		reverseCache: make(map[ckey.ContextKey]*reverseCacheRing),
	}
}

// get returns the cached post-tagFilter entry for a pre-filter context key, if present.
func (sc *tagFilterCache) get(key ckey.ContextKey) (tagFilterCacheEntry, bool) {
	cached, ok := sc.cache[key]
	return cached, ok
}

// add stores a pre-tagFilter → post-filter mapping and updates the reverse ring buffer.
// If the ring buffer for the post-tagFilter key is full, the oldest pre-filter key is
// evicted from the forward cache.
//
// Note the cache can get in an inconsistent state if you add a key that already
// exists in the cache as the reverse entries will still exist pointing to the
// old entry.
func (sc *tagFilterCache) add(key ckey.ContextKey, entry tagFilterCacheEntry) {
	sc.cache[key] = entry

	ring := sc.reverseCache[entry.contextKey]
	if ring == nil {
		ring = &reverseCacheRing{}
		sc.reverseCache[entry.contextKey] = ring
	}
	if evicted, ok := ring.add(key); ok {
		delete(sc.cache, evicted)
		tlmFilteredTagsCacheEvict.Inc()
	}
}

// delete removes a post-tagFilter context key and all pre-filter keys that mapped to it.
func (sc *tagFilterCache) delete(key ckey.ContextKey) {
	if ring := sc.reverseCache[key]; ring != nil {
		ring.forEach(func(k ckey.ContextKey) {
			delete(sc.cache, k)
		})
	}
	delete(sc.reverseCache, key)
}

// clear removes all entries from both forward and reverse caches.
func (sc *tagFilterCache) clear() {
	clear(sc.cache)
	clear(sc.reverseCache)
}

// reverseCacheRing is a ring buffer of pre-tagFilter context keys that grows on demand
// up to reverseCacheCapacity. Once full, the oldest entry is overwritten on each add.
// Use a ring buffer rather than a continuously growing array since otherwise we only evict
// when the post-tagFilter key expires, if a high churn tag is filterped (eg. rotating pod
// identifiers) but the post-tagFilter remains continuously active, these pre-filter keys
// would continue to accumulate.
type reverseCacheRing struct {
	keys  []ckey.ContextKey
	pos   int // next write position (oldest element when full)
	count int
}

// add inserts a key into the ring. If full, returns the evicted key.
func (r *reverseCacheRing) add(key ckey.ContextKey) (ckey.ContextKey, bool) {
	if r.count < reverseCacheCapacity {
		r.keys = append(r.keys, key)
		r.count++
		return 0, false
	}
	evicted := r.keys[r.pos]
	r.keys[r.pos] = key
	r.pos = (r.pos + 1) % reverseCacheCapacity
	return evicted, true
}

// forEach calls fn for every key in the ring.
func (r *reverseCacheRing) forEach(fn func(ckey.ContextKey)) {
	for i := 0; i < r.count; i++ {
		fn(r.keys[i])
	}
}
