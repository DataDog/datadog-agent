package obfuscate

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config/features"
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
	if !features.Has("sql_cache") {
		// a nil *ristretto.Cache is a no-op cache
		return &measuredCache{}
	}
	cfg := &ristretto.Config{
		// We know that the maximum allowed resource length is 5K. This means that
		// in 5MB we can store a minimum of 1000 queries.
		MaxCost: 5_000_000,

		// An appromixated worst-case scenario when the cache is filled with small
		// queries averaged as being of length 11 ("LOCK TABLES"), we would be able
		// to fit 476K of them into 5MB of cost.
		//
		// We average it to 500K and multiply 10x as the documentation recommends.
		NumCounters: 500_000 * 10,

		BufferItems: 64,   // default recommended value
		Metrics:     true, // enable hit/miss counters
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
