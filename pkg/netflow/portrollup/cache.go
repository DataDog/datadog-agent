package portrollup

import (
	"github.com/patrickmn/go-cache"
	"time"
)

// PortCache TODO
type PortCache struct {
	cache             *cache.Cache
	defaultExpiration time.Duration
}

// NewCache returns a new instance of evalCache
func NewCache(defaultExpiration, cleanupInterval time.Duration) *PortCache {
	return &PortCache{
		cache:             cache.New(defaultExpiration, cleanupInterval),
		defaultExpiration: defaultExpiration,
	}
}

func (c *PortCache) Increment(key string) {
	if _, ok := c.cache.Get(key); !ok {
		c.cache.Set(key, int8(1), cache.DefaultExpiration)
		return
	}
	_, _ = c.cache.IncrementInt8(key, 1)
}

func (c *PortCache) Get(key string) int8 {
	content, found := c.cache.Get(key)
	if !found {
		return 0
	}
	return content.(int8)
}

func (c *PortCache) ItemCount() int {
	return c.cache.ItemCount()
}

func (c *PortCache) RefreshExpiration(key string) {
	value, ok := c.cache.Get(key)
	if ok {
		c.cache.SetDefault(key, value)
	}
}
