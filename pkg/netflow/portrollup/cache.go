package portrollup

import (
	"sync"
	"time"
)

var timeNow = time.Now

// PortCache TODO
type PortCache struct {
	items             map[string]Item
	defaultExpiration time.Duration
	mu                sync.RWMutex
}

// Item storage should be minimized as much as possible since we might have millions of entries
type Item struct {
	Counter    uint8
	Expiration int64
}

// NewCache returns a new instance of evalCache
func NewCache(defaultExpirationMin int) *PortCache {
	return &PortCache{
		items:             make(map[string]Item),
		defaultExpiration: time.Duration(defaultExpirationMin) * time.Second,
	}
}

func (c *PortCache) Increment(key string) {
	c.mu.Lock()
	v, ok := c.items[key]
	if !ok {
		c.items[key] = Item{Counter: 1, Expiration: c.getExpiration()}
	} else {
		v.Counter += 1
		v.Expiration = c.getExpiration()
		c.items[key] = v
	}
	c.mu.Unlock()
}

func (c *PortCache) getExpiration() int64 {
	return timeNow().Add(c.defaultExpiration).UnixNano()
}

func (c *PortCache) Get(key string) uint8 {
	c.mu.RLock()
	content, found := c.items[key]
	c.mu.RUnlock()
	if !found {
		return 0
	}
	return content.Counter
}

func (c *PortCache) GetExpiration(key string) int64 {
	c.mu.RLock()
	content, found := c.items[key]
	c.mu.RUnlock()
	if !found {
		return 0
	}
	return content.Expiration
}

func (c *PortCache) ItemCount() int {
	c.mu.RLock()
	count := len(c.items)
	c.mu.RUnlock()
	return count
}

func (c *PortCache) RefreshExpiration(key string) {
	c.mu.Lock()
	item, ok := c.items[key]
	if ok {
		item.Expiration = c.getExpiration()
		c.items[key] = item
	}
	c.mu.Unlock()
}

func (c *PortCache) DeleteAllExpired() {
	now := timeNow().UnixNano()
	c.mu.Lock()
	for k, v := range c.items {
		if v.Expiration <= now {
			delete(c.items, k)
		}
	}
	c.mu.Unlock()
}
