package obfuscate

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"

	"github.com/dgraph-io/ristretto"
)

// measuredCache is a wrapper on top of *ristretto.Cache which additionally
// sends metrics (hits and misses) every 10 seconds.
type measuredCache struct {
	*ristretto.Cache

	// close allows sending shutdown notification.
	close chan struct{}
}

// Close gracefully closes the cache when active.
func (c *measuredCache) Close() {
	if c.Cache == nil {
		return
	}
	c.close <- struct{}{}
	<-c.close
}

func (c *measuredCache) statsLoop() {
	defer func() {
		c.close <- struct{}{}
	}()
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	mx := c.Cache.Metrics
	for {
		select {
		case <-tick.C:
			metrics.Gauge("datadog.trace_agent.ofuscation.sql_cache.hits", float64(mx.Hits()), nil, 1)
			metrics.Gauge("datadog.trace_agent.ofuscation.sql_cache.misses", float64(mx.Misses()), nil, 1)
		case <-c.close:
			c.Cache.Close()
			return
		}
	}
}

// newMeasuredCache returns a new measuredCache.
func newMeasuredCache() *measuredCache {
	if !config.HasFeature("sql_cache") {
		// a nil *ristretto.Cache is a no-op cache
		return &measuredCache{}
	}
	cfg := &ristretto.Config{
		Metrics: true,
		// We know that both cache keys and values will have a maximum
		// length of 5K, so one entry (key + value) will be 10K maximum.
		// At worst case scenario, a 5M cache should fit at least 500 queries.
		MaxCost: 5 * 1024 * 1024,
		// An appromixation worst-case scenario when the cache is filled of small
		// queries averaged as being of length 19 (SELECT * FROM users), we would
		// be able to fit 263K of them into 5MB of cost.
		// We multiply the value by x10 as advised in the ristretto.Config documentation.
		NumCounters: 3 * 1000 * 1000,
		// 64 is the recommended default value
		BufferItems: 64,
	}
	cache, err := ristretto.NewCache(cfg)
	if err != nil {
		panic(fmt.Errorf("Error starting obfuscator query cache: %v", err))
	}
	c := measuredCache{
		close: make(chan struct{}),
		Cache: cache,
	}
	go c.statsLoop()
	return &c
}
