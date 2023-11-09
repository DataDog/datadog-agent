package cache

import "unsafe"

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	// There are multiple instances of the interner, one per worker. Counters are normally fine,
	// gauges require special care to make sense. We don't need to clean up when an instance is
	// dropped, because it only happens on agent shutdown.
	tlmSIResets = telemetry.NewSimpleCounter("dogstatsd", "string_interner_resets",
		"Amount of resets of the string interner used in dogstatsd")
	tlmSIRSize = telemetry.NewSimpleGauge("dogstatsd", "string_interner_entries",
		"Number of entries in the string interner")
	tlmSIRBytes = telemetry.NewSimpleGauge("dogstatsd", "string_interner_bytes",
		"Number of bytes stored in the string interner")
	tlmSIRHits = telemetry.NewSimpleCounter("dogstatsd", "string_interner_hits",
		"Number of times string interner returned an existing string")
	tlmSIRMiss = telemetry.NewSimpleCounter("dogstatsd", "string_interner_miss",
		"Number of times string interner created a new string object")
	tlmSIRNew = telemetry.NewSimpleCounter("dogstatsd", "string_interner_new",
		"Number of times string interner was created")
	tlmSIRStrBytes = telemetry.NewSimpleHistogram("dogstatsd", "string_interner_str_bytes",
		"Number of times string with specific length were added",
		[]float64{1, 2, 4, 8, 16, 32, 64, 128})
)

// stringInterner is a string cache providing a longer life for strings,
// helping to avoid GC runs because they're re-used many times instead of
// created every time.
type stringCacheItem struct {
	s string // points into the mmap region
	// LRU doubly-linked list.  Head is most recently used, tail is
	// next candidate for eviction.
	prev, next *stringCacheItem
}

type lruStringCache struct {
	// LRU cache in strings.
	strings map[string]*stringCacheItem
	// LRU linked list head/tail.
	head, tail *stringCacheItem
	maxSize    int
	curBytes   int
	tlmEnabled bool
}

func newLruStringCache(maxSize int, tlmEnabled bool) lruStringCache {
	if tlmEnabled {
		tlmSIRNew.Inc()
	}
	return lruStringCache{
		strings:    make(map[string]*stringCacheItem),
		maxSize:    maxSize,
		tlmEnabled: tlmEnabled,
	}
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
		if c.tlmEnabled {
			tlmSIRHits.Inc()
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

		return s.s
	}

	for len(c.strings) >= c.maxSize {
		if c.tail == nil {
			panic("Empty list and still over size!")
		}
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

		if c.tlmEnabled {
			tlmSIResets.Inc()
			tlmSIRBytes.Sub(float64(c.curBytes))
			tlmSIRSize.Sub(float64(len(c.strings)))
			c.curBytes -= lastLen
		}

		//log.Debug("Removing element from interner LRU cache")
	}

	str := allocator(key)
	// Don't insert blanks.
	if str == "" {
		return str
	}

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

	if c.tlmEnabled {
		length := len(s.s)
		tlmSIRMiss.Inc()
		tlmSIRSize.Inc()
		tlmSIRBytes.Add(float64(length))
		tlmSIRStrBytes.Observe(float64(length))
		c.curBytes += length
	}

	return s.s
}
