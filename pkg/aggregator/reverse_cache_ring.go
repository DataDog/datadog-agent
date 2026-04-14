// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import "github.com/DataDog/datadog-agent/pkg/aggregator/ckey"

const reverseCacheCapacity = 100

// reverseCacheRing is a fixed-capacity ring buffer of pre-strip context keys.
// Once full, the oldest entry is overwritten on each add. Zero allocations in steady state.
// Use a ring buffer rather than a continuously growing array since otherwise we only evict
// when the post-strip key expires, if a high churn tag is stripped (eg. rotating pod
// identifiers) but the post-strip remains continuously active, these pre-strip keys
// would continue to accumulate.
type reverseCacheRing struct {
	keys  [reverseCacheCapacity]ckey.ContextKey
	pos   int // next write position (oldest element when full)
	count int
}

// add inserts a key into the ring. If full, returns the evicted key.
func (r *reverseCacheRing) add(key ckey.ContextKey) (ckey.ContextKey, bool) {
	var evicted ckey.ContextKey
	didEvict := r.count == reverseCacheCapacity
	if didEvict {
		evicted = r.keys[r.pos]
	}
	r.keys[r.pos] = key
	r.pos = (r.pos + 1) % reverseCacheCapacity
	if !didEvict {
		r.count++
	}
	return evicted, didEvict
}

// forEach calls fn for every key in the ring.
func (r *reverseCacheRing) forEach(fn func(ckey.ContextKey)) {
	for i := 0; i < r.count; i++ {
		fn(r.keys[i])
	}
}
