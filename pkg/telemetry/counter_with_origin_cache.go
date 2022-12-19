// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"sync"
)

// While we try to add the origin tag in the given telemetry metric, we want to
// avoid having it growing indefinitely, hence this safeguard to limit the
// size of this cache for long-running agent or environment with a lot of
// different container IDs.
const maxOriginCounters = 200

// CounterWithOriginCache is a cache of specialized telemetry counter counting contexts
// or metrics per origin.
type CounterWithOriginCache struct {
	// mu must be held when accessing cachedCountersWithOrigin and cachedOrder
	mu sync.Mutex

	// cachedOriginCounts caches telemetry counter per origin
	// (when dogstatsd origin telemetry is enabled)
	cachedCountersWithOrigin map[string]cachedCounterWithOrigin
	cachedOrder              []cachedCounterWithOrigin // for cache eviction

	context bool

	baseCounter Counter
}

// cachedCounterWithOrigin is an entry in the cache.
type cachedCounterWithOrigin struct {
	origin string
	ok     map[string]string
	err    map[string]string
	okCnt  SimpleCounter
	errCnt SimpleCounter
}

// NewMetricCounterWithOriginCache creates a cache for metrics counter telemetry metrics.
// The baseCounter MUST have three tags: `message_type`, `state` and `origin`.
func NewMetricCounterWithOriginCache(baseCounter Counter) CounterWithOriginCache {
	return CounterWithOriginCache{
		cachedCountersWithOrigin: make(map[string]cachedCounterWithOrigin),
		cachedOrder:              make([]cachedCounterWithOrigin, 0, maxOriginCounters/2),
		baseCounter:              baseCounter,
		context:                  false,
	}
}

// NewContextCounterWithOriginCache creates a cache for contexts counter telemetry metrics.
// The baseCounter MUST have one tag: `origin`.
func NewContextCounterWithOriginCache(baseCounter Counter) CounterWithOriginCache {
	return CounterWithOriginCache{
		cachedCountersWithOrigin: make(map[string]cachedCounterWithOrigin),
		cachedOrder:              make([]cachedCounterWithOrigin, 0, maxOriginCounters/2),
		baseCounter:              baseCounter,
		context:                  true,
	}
}

// Get returns a telemetry counter for processed metrics or contetxs using the given origin as a tag.
// The cache makes sure that only `maxOriginCounters` are stored to avoid an infinite expansion.
// Counters returned by `get` are thread safe.
func (p *CounterWithOriginCache) Get(origin string) (SimpleCounter, SimpleCounter) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// return from cache if available

	if maps, ok := p.cachedCountersWithOrigin[origin]; ok {
		if p.context {
			return maps.okCnt, maps.okCnt
		} else {
			return maps.okCnt, maps.errCnt
		}
	}

	// create the entry in the cache

	var first, second SimpleCounter
	if p.context {
		first = p.createAndReturnContext(origin)
		second = first
	} else {
		first, second = p.createAndReturnMetric(origin)
	}

	// expire cache entries if necessary

	p.expire()

	return first, second
}

func (p *CounterWithOriginCache) createAndReturnMetric(origin string) (SimpleCounter, SimpleCounter) {
	okMap := map[string]string{"message_type": "metrics", "state": "ok", "origin": origin}
	errorMap := map[string]string{"message_type": "metrics", "state": "error", "origin": origin}

	maps := cachedCounterWithOrigin{
		origin: origin,
		ok:     okMap,
		err:    errorMap,
		okCnt:  p.baseCounter.WithTags(okMap),
		errCnt: p.baseCounter.WithTags(errorMap),
	}

	p.cachedCountersWithOrigin[origin] = maps
	p.cachedOrder = append(p.cachedOrder, maps)

	return maps.okCnt, maps.errCnt
}

func (s *CounterWithOriginCache) createAndReturnContext(origin string) SimpleCounter {
	okMap := map[string]string{"origin": origin}

	maps := cachedCounterWithOrigin{
		origin: origin,
		ok:     okMap,
		err:    okMap,
		okCnt:  s.baseCounter.WithTags(okMap),
	}
	maps.errCnt = maps.okCnt

	s.cachedCountersWithOrigin[origin] = maps
	s.cachedOrder = append(s.cachedOrder, maps)

	return maps.okCnt
}

func (p *CounterWithOriginCache) expire() {
	if len(p.cachedOrder) > maxOriginCounters {
		// remove the oldest one from the cache
		pop := p.cachedOrder[0]
		delete(p.cachedCountersWithOrigin, pop.origin)
		p.cachedOrder = p.cachedOrder[1:]
		// remove it from the telemetry metrics as well
		p.baseCounter.DeleteWithTags(pop.ok)
		if !p.context {
			p.baseCounter.DeleteWithTags(pop.err)
		}
	}
}
