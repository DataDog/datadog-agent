// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package obfuscate

import (
	"fmt"
	"time"

	"github.com/outcaste-io/ristretto"
)

// measuredCache is a wrapper on top of *ristretto.Cache which additionally
// sends metrics (hits and misses) every 10 seconds.
type measuredCache struct {
	*ristretto.Cache

	// close allows sending shutdown notification.
	close  chan struct{}
	statsd StatsClient
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
			_ = c.statsd.Gauge("datadog.trace_agent.obfuscation.sql_cache.hits", float64(mx.Hits()), nil, 1)     //nolint:errcheck
			_ = c.statsd.Gauge("datadog.trace_agent.obfuscation.sql_cache.misses", float64(mx.Misses()), nil, 1) //nolint:errcheck
		case <-c.close:
			c.Cache.Close()
			return
		}
	}
}

type cacheOptions struct {
	On      bool
	Statsd  StatsClient
	MaxSize int64
}

// newMeasuredCache returns a new measuredCache.
func newMeasuredCache(opts cacheOptions) *measuredCache {
	if !opts.On {
		// a nil *ristretto.Cache is a no-op cache
		return &measuredCache{}
	}
	cfg := &ristretto.Config{
		MaxCost:     opts.MaxSize,
		NumCounters: opts.MaxSize * 10, // Multiplied by 10 as per ristretto recommendation
		BufferItems: 64,                // default recommended value
		Metrics:     true,              // enable hit/miss counters
	}
	cache, err := ristretto.NewCache(cfg)
	if err != nil {
		panic(fmt.Errorf("Error starting obfuscator query cache: %v", err))
	}
	c := measuredCache{
		close:  make(chan struct{}),
		statsd: opts.Statsd,
		Cache:  cache,
	}
	go c.statsLoop()
	return &c
}
