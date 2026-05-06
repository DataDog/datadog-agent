// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"sync"
	"time"
)

type cacheEntry struct {
	result    Result
	success   bool
	expiresAt time.Time // zero = never
}

type cache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
	now     func() time.Time
}

func newCache(now func() time.Time) *cache {
	if now == nil {
		now = time.Now
	}
	return &cache{entries: make(map[string]cacheEntry), now: now}
}

func cacheKey(svcID, integrationName string) string {
	return svcID + "|" + integrationName
}

func (c *cache) get(svcID, integrationName string) (Result, bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[cacheKey(svcID, integrationName)]
	if !ok {
		return Result{}, false, false
	}
	if !e.expiresAt.IsZero() && c.now().After(e.expiresAt) {
		delete(c.entries, cacheKey(svcID, integrationName))
		return Result{}, false, false
	}
	return e.result, e.success, true
}

func (c *cache) putSuccess(svcID, integrationName string, r Result) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[cacheKey(svcID, integrationName)] = cacheEntry{result: r, success: true}
}

func (c *cache) putFailure(svcID, integrationName string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[cacheKey(svcID, integrationName)] = cacheEntry{success: false, expiresAt: c.now().Add(ttl)}
}
