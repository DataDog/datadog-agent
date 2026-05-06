// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"strings"
	"sync"
	"time"
)

// cacheState is the state of a (svcID, integration) entry in the cache.
type cacheState int

const (
	stateMiss    cacheState = iota // no entry
	stateHit                       // success entry — return cached configs
	statePending                   // failure entry — may probe again at nextRetryAt
	stateGivenUp                   // failure entry — schedule exhausted, no more probes
)

type cacheEntry struct {
	success bool
	result  Result // valid when success

	// failure-only:
	attemptsMade int       // count of failures so far
	nextRetryAt  time.Time // zero when givenUp
	givenUp      bool
}

type cacheLookupResult struct {
	state       cacheState
	result      Result    // valid when state == stateHit
	nextRetryAt time.Time // valid when state == statePending
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

func (c *cache) lookup(svcID, integrationName string) cacheLookupResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[cacheKey(svcID, integrationName)]
	if !ok {
		return cacheLookupResult{state: stateMiss}
	}
	if e.success {
		return cacheLookupResult{state: stateHit, result: e.result}
	}
	if e.givenUp {
		return cacheLookupResult{state: stateGivenUp}
	}
	return cacheLookupResult{state: statePending, nextRetryAt: e.nextRetryAt}
}

func (c *cache) putSuccess(svcID, integrationName string, r Result) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[cacheKey(svcID, integrationName)] = cacheEntry{success: true, result: r}
}

// putFailure records a probe failure and advances the retry schedule.
// `schedule[attemptsMade-1]` is the wait time before the next probe attempt;
// once attemptsMade > len(schedule), the entry is marked givenUp.
func (c *cache) putFailure(svcID, integrationName string, schedule []time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	k := cacheKey(svcID, integrationName)
	e := c.entries[k]
	e.success = false
	e.result = Result{}
	e.attemptsMade++
	if e.attemptsMade > len(schedule) {
		e.givenUp = true
		e.nextRetryAt = time.Time{}
	} else {
		e.nextRetryAt = c.now().Add(schedule[e.attemptsMade-1])
	}
	c.entries[k] = e
}

// forget drops all entries for a given svcID. Called from configmgr on
// service removal so a stopped-and-restarted container starts fresh.
func (c *cache) forget(svcID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := svcID + "|"
	for k := range c.entries {
		if strings.HasPrefix(k, prefix) {
			delete(c.entries, k)
		}
	}
}
