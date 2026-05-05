// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discovery

import (
	"sync"
	"time"
)

type cacheEntry struct {
	result    ProbeResult
	success   bool
	expiresAt time.Time // zero = never
}

type probeCache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
	now     func() time.Time
}

func newProbeCache(now func() time.Time) *probeCache {
	if now == nil {
		now = time.Now
	}
	return &probeCache{entries: make(map[string]cacheEntry), now: now}
}

func cacheKey(svcID, cfgHash string) string {
	return svcID + "|" + cfgHash
}

func (c *probeCache) get(svcID, cfgHash string) (ProbeResult, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[cacheKey(svcID, cfgHash)]
	if !ok {
		return ProbeResult{}, false, false
	}
	if !e.expiresAt.IsZero() && c.now().After(e.expiresAt) {
		delete(c.entries, cacheKey(svcID, cfgHash))
		return ProbeResult{}, false, false
	}
	return e.result, e.success, true
}

func (c *probeCache) putSuccess(svcID, cfgHash string, r ProbeResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[cacheKey(svcID, cfgHash)] = cacheEntry{result: r, success: true}
}

func (c *probeCache) putFailure(svcID, cfgHash string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[cacheKey(svcID, cfgHash)] = cacheEntry{success: false, expiresAt: c.now().Add(ttl)}
}
