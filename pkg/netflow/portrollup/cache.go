package portrollup

import (
	"time"
)

var timeNow = time.Now

// PortCache TODO
type PortCache struct {
	items             map[string]Item
	defaultExpiration uint8 // expiration time in minutes
	LastClean         time.Time
}

// Item storage should be minimized as much as possible since we might have millions of entries
type Item struct {
	Counter                    uint8
	ExpirationMinFromLastCheck uint8
}

// NewCache returns a new instance of evalCache
func NewCache(defaultExpirationMin uint8) *PortCache {
	return &PortCache{
		items:             make(map[string]Item),
		defaultExpiration: defaultExpirationMin,
		LastClean:         time.Now(),
	}
}

func (c *PortCache) Increment(key string) {
	v, ok := c.items[key]
	if !ok {
		c.items[key] = Item{Counter: 1, ExpirationMinFromLastCheck: c.getExpirationMinFromLastCheck()}
	} else {
		v.Counter += 1
		v.ExpirationMinFromLastCheck = c.getExpirationMinFromLastCheck()
		c.items[key] = v
	}
}

func (c *PortCache) getExpirationMinFromLastCheck() uint8 {
	minSinceLastCheck := int(timeNow().Sub(c.LastClean).Minutes())
	//var expMinFromLastCheck uint8
	//if minSinceLastCheck > int(c.defaultExpiration) {
	//	expMinFromLastCheck = 0
	//} else {
	//	expMinFromLastCheck = c.defaultExpiration + uint8(minSinceLastCheck)
	//}
	return c.defaultExpiration + uint8(minSinceLastCheck)
}

func (c *PortCache) Get(key string) uint8 {
	content, found := c.items[key]
	if !found {
		return 0
	}
	return content.Counter
}

func (c *PortCache) ItemCount() int {
	return len(c.items)
}

func (c *PortCache) RefreshExpiration(key string) {
	item, ok := c.items[key]
	if ok {
		item.ExpirationMinFromLastCheck = c.getExpirationMinFromLastCheck()
		c.items[key] = item
	}
}

func (c *PortCache) DeleteAllExpired() {
	//c.mu.Lock()
	minSinceLastClean := int(timeNow().Sub(c.LastClean).Minutes())
	for k, v := range c.items {
		if int(v.ExpirationMinFromLastCheck) <= minSinceLastClean {
			delete(c.items, k)
		}
	}
	//c.mu.Unlock()
}
