package cache

import (
	"fmt"
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
func (b *BasicCache) Add(k string, v interface{}) {
	b.m.Lock()
	defer b.m.Unlock()

	b.cache[k] = v
	b.modified = time.Now().Unix()
}

// Get gets interface for specified key or error
func (b *BasicCache) Get(k string) (interface{}, error) {
	b.m.RLock()
	defer b.m.RUnlock()

	if v, ok := b.cache[k]; ok {
		return v, nil
	}

	return nil, fmt.Errorf("item not in cache")
}

// Remove removes an entry from the cache if it exists
func (b *BasicCache) Remove(k string) {
	b.m.Lock()
	defer b.m.Unlock()

	delete(b.cache, k)
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

// Iterator returns key and value channels to iterate on
// Not the most performant implementation - but clean and should be fine
// given that basic caches should be small
func (b *BasicCache) Iterator() (<-chan string, <-chan interface{}) {
	b.m.RLock()
	defer b.m.RUnlock()

	keyChan := make(chan string, len(b.cache))
	valueChan := make(chan interface{}, len(b.cache))
	go func() {
		b.m.RLock()
		defer b.m.RUnlock()
		for k, v := range b.cache {
			keyChan <- k
			valueChan <- v
		}
		close(keyChan)
		close(valueChan)
	}()

	return keyChan, valueChan
}
