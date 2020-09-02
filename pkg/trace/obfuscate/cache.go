package obfuscate

import (
	"container/list"
	"sync"
	"sync/atomic"
)

// defaultCacheSize specifies the maximum string cache size in bytes.
//
// The cache keys and values are both tag values, which have the maximum
// length of 5K, meaning that a 5MB cache will be able to hold approximately
// 1000 queries.
const defaultCacheSize = (5000 + 5000) * 1000

type queryCache struct {
	hits, misses uint64 // metrics
	size         uint64 // current cache size
	maxSize      uint64 // maximum cache size

	mu    sync.RWMutex // guards below
	items map[string]*list.Element
	list  *list.List
}

type cacheItem struct {
	key string
	val *ObfuscatedQuery
}

func (i *cacheItem) size() uint64 {
	return uint64(len(i.key) + len(i.val.Query) + len(i.val.TablesCSV))
}

func newQueryCache(size uint64) *queryCache {
	return &queryCache{
		maxSize: size,
		items:   make(map[string]*list.Element),
		list:    list.New(),
	}
}

func (c *queryCache) Get(key string) (oq *ObfuscatedQuery, ok bool) {
	c.mu.RLock()
	ele, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		atomic.AddUint64(&c.misses, 1)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	atomic.AddUint64(&c.hits, 1)
	c.list.MoveToFront(ele)
	return ele.Value.(*cacheItem).val, true
}

func (c *queryCache) Add(key string, val *ObfuscatedQuery) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ele, ok := c.items[key]; ok {
		// key already in cache
		c.list.MoveToFront(ele)
		return
	}
	item := &cacheItem{key, val}
	c.items[key] = c.list.PushFront(item)
	c.size += item.size()
	for c.size > c.maxSize {
		c.removeOldest()
	}
}

func (c *queryCache) removeOldest() {
	ele := c.list.Back()
	if ele == nil {
		return
	}
	item := c.list.Remove(ele).(*cacheItem)
	delete(c.items, item.key)
	c.size -= item.size()
}
