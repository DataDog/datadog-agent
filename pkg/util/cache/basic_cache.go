package cache

import (
	"sync"
	"time"
)

// BasicCache is a simple threadsafe cache
type BasicCache struct {
	m        sync.RWMutex
	cache    map[string]interface{}
	modified int64
}

// NewBasicCache Creates new BasicCache
func NewBasicCache() *BasicCache {
	return &BasicCache{
		cache: make(map[string]interface{}),
	}
}

// Add adds value to cache for specified key
// Returns true if the value was added/changed, or false if it was already there.
// If the value was added/changed, it updates the modified timestamp
func (b *BasicCache) Add(k string, v interface{}) bool {
	b.m.Lock()
	defer b.m.Unlock()

	current, found := b.cache[k]
	if !found || current != v {
		b.cache[k] = v
		b.modified = time.Now().Unix()
		return true
	}
	return false
}

// Get gets interface for specified key and a boolean that's false when the key is not found
func (b *BasicCache) Get(k string) (interface{}, bool) {
	b.m.RLock()
	defer b.m.RUnlock()

	v, found := b.cache[k]

	return v, found
}

// Remove removes an entry from the cache if it exists
func (b *BasicCache) Remove(k string) {
	b.m.Lock()
	defer b.m.Unlock()

	delete(b.cache, k)
	b.modified = time.Now().Unix()
}

// Size returns the current size of the cache
func (b *BasicCache) Size() int {
	b.m.Lock()
	defer b.m.Unlock()

	return len(b.cache)
}

// GetModified gets interface for specified key or error
func (b *BasicCache) GetModified() int64 {
	b.m.RLock()
	defer b.m.RUnlock()

	return b.modified
}

// Items returns a map with the elements in the cache
func (b *BasicCache) Items() map[string]interface{} {
	items := map[string]interface{}{}

	b.m.RLock()
	defer b.m.RUnlock()
	for k, v := range b.cache {
		items[k] = v
	}

	return items
}
