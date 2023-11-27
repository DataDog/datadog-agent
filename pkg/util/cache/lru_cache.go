// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cache

import (
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	// There are multiple instances of the interner, one per worker. Counters are normally fine,
	// gauges require special care to make sense. We don't need to clean up when an instance is
	// dropped, because it only happens on agent shutdown.

	// TlmSIDrops counts strings dropped for being the least recently used
	TlmSIDrops = telemetry.NewCounter("dogstatsd", "string_interner_drops", []string{"interner_id"},
		"Amount of drops of the string interner used in dogstatsd")
	// TlmSIRSize counts the number of entries in the string interner
	TlmSIRSize = telemetry.NewGauge("dogstatsd", "string_interner_entries", []string{"interner_id"},
		"Number of entries in the string interner")
	// TlmSIRBytes counts the number of bytes stored in the string interner
	TlmSIRBytes = telemetry.NewGauge("dogstatsd", "string_interner_bytes", []string{"interner_id"},
		"Number of bytes stored in the string interner")
	// TlmSIRHits counts the number of times string interner returned an existing string
	TlmSIRHits = telemetry.NewCounter("dogstatsd", "string_interner_hits", []string{"interner_id"},
		"Number of times string interner returned an existing string")
	// TlmSIRMiss counts the number of times string interner created a new string object
	TlmSIRMiss = telemetry.NewCounter("dogstatsd", "string_interner_miss", []string{"interner_id"},
		"Number of times string interner created a new string object")
	// TlmSIRNew counts the number of times string interner was created
	TlmSIRNew = telemetry.NewSimpleCounter("dogstatsd", "string_interner_new",
		"Number of times string interner was created")
	// TlmSIRStrBytes is a histogram for interned string lengths.
	TlmSIRStrBytes = telemetry.NewSimpleHistogram("dogstatsd", "string_interner_str_bytes",
		"Number of times string with specific length were added",
		[]float64{1, 2, 4, 8, 16, 32, 64, 128})
)

// stringInterner is a string cache providing a longer life for strings,
// helping to avoid GC runs because they're re-used many times instead of
// created every time.
type stringCacheItem struct {
	s string // points into the mmap region
	// predecessors are more recently used in the cache.
	prev, next *stringCacheItem
}

type lruStringCache struct {
	// LRU cache in strings.
	strings map[string]*stringCacheItem
	// LRU doubly-linked list.  Head is most recently used, tail is
	// next candidate for eviction.
	head, tail *stringCacheItem
	maxSize    int
	origin     string
	telemetry  siTelemetry
}

type siTelemetry struct {
	enabled  bool
	curBytes int

	drops, hits, miss telemetry.SimpleCounter
	size, bytes       telemetry.SimpleGauge
}

func newLruStringCache(maxSize int, origin string, enableTelemetry bool) lruStringCache {
	i := &lruStringCache{
		strings: make(map[string]*stringCacheItem),
		maxSize: maxSize,
		origin:  origin,
		telemetry: siTelemetry{
			enabled: enableTelemetry,
		},
	}

	if i.telemetry.enabled {
		i.prepareTelemetry()
	}

	return *i
}

func (c *lruStringCache) prepareTelemetry() {
	c.telemetry.drops = TlmSIDrops.WithValues(c.origin)
	c.telemetry.size = TlmSIRSize.WithValues(c.origin)
	c.telemetry.bytes = TlmSIRBytes.WithValues(c.origin)
	c.telemetry.hits = TlmSIRHits.WithValues(c.origin)
	c.telemetry.miss = TlmSIRMiss.WithValues(c.origin)
}

func (c *lruStringCache) deleteOldestNode() {
	last := c.tail
	if last.prev != nil {
		last.prev.next = nil
	}
	c.tail = last.prev
	if c.maxSize == 1 {
		c.head = c.tail
	}
	lastLen := len(last.s)
	delete(c.strings, last.s)

	if c.telemetry.enabled {
		c.telemetry.drops.Inc()
		c.telemetry.bytes.Sub(float64(c.telemetry.curBytes))
		c.telemetry.size.Sub(float64(len(c.strings)))
		c.telemetry.curBytes -= lastLen
	}
}

// promoteNode marks the given node as the most-recently-used.
func (c *lruStringCache) promoteNode(s *stringCacheItem) {
	if c.telemetry.enabled {
		c.telemetry.hits.Inc()
	}
	// If we found it, it's now the least recently used item, so rearrange it
	// in the LRU linked list.
	if c.tail == s && s.prev != nil {
		c.tail = s.prev
	}
	if c.head != s {
		if s.prev != nil {
			s.prev.next = s.next
		}
		if s.next != nil {
			s.next.prev = s.prev
		}
		s.prev = nil
		s.next = c.head
		c.head.prev = s
		c.head = s
	}
}

func (c *lruStringCache) addItemToCacheHead(str string) *stringCacheItem {
	// Insert into the cache, and allocate to the backing store.
	s := &stringCacheItem{
		s:    str,
		prev: nil,
		next: c.head,
	}

	if c.head != nil {
		c.head.prev = s
	}
	c.head = s
	if c.tail == nil {
		c.tail = s
	}

	if str == "" || unsafe.StringData(str) == nil || c == nil || c.strings == nil {
		panic("Dead string going to LRU")
	}
	c.strings[str] = s

	if c.telemetry.enabled {
		length := len(s.s)
		c.telemetry.miss.Inc()
		c.telemetry.size.Inc()
		c.telemetry.bytes.Add(float64(length))
		TlmSIRStrBytes.Observe(float64(length))
		c.telemetry.curBytes += length
	}

	return s
}

// lookupOrInsert looks inside the cache for this key.  If found, it makes it the head of the LRU cache.
// if not, it may evict the least recently used entry to make space.  It uses the allocator arg function
// to create the string, in case there's a separate backing store for it.
func (c *lruStringCache) lookupOrInsert(key []byte, allocator func(key []byte) string) string {
	if len(key) < 1 {
		return ""
	}

	// here is the string interner trick: the map lookup using
	// string(key) doesn't actually allocate a string, but is
	// returning the string value -> no new heap allocation
	// for this string.
	// See https://github.com/golang/go/commit/f5f5a8b6209f84961687d993b93ea0d397f5d5bf
	if s, found := c.strings[string(key)]; found {
		c.promoteNode(s)
		return s.s
	}

	for len(c.strings) >= c.maxSize {
		if c.tail == nil {
			panic("Empty list and still over size!")
		}
		c.deleteOldestNode()
	}

	str := allocator(key)
	// Allocator can fail, return the failure instead of saving blanks
	// into the LRU.
	if str == "" {
		return str
	}

	s := c.addItemToCacheHead(str)
	return s.s
}
