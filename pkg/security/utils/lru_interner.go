// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"sync"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"go.uber.org/atomic"
)

// LRUStringInterner is a best-effort LRU-based string deduplicator
type LRUStringInterner struct {
	sync.Mutex
	store *simplelru.LRU[string, string]
	name  string

	hits   *atomic.Int64
	misses *atomic.Int64
}

// NewLRUStringInterner returns a new LRUStringInterner, with the cache size provided
// if the cache size is negative this function will panic. The name is used to tag
// metrics emitted by SendStats.
func NewLRUStringInterner(size int, name string) *LRUStringInterner {
	store, err := simplelru.NewLRU[string, string](size, nil)
	if err != nil {
		panic(err)
	}

	return &LRUStringInterner{
		store:  store,
		name:   name,
		hits:   atomic.NewInt64(0),
		misses: atomic.NewInt64(0),
	}
}

// Deduplicate returns a possibly de-duplicated string
func (si *LRUStringInterner) Deduplicate(value string) string {
	si.Lock()
	defer si.Unlock()

	return si.deduplicateUnsafe(value)
}

func (si *LRUStringInterner) deduplicateUnsafe(value string) string {
	if res, ok := si.store.Get(value); ok {
		si.hits.Inc()
		return res
	}

	si.misses.Inc()
	si.store.Add(value, value)
	return value
}

// DeduplicateSlice returns a possibly de-duplicated string slice
func (si *LRUStringInterner) DeduplicateSlice(values []string) {
	si.Lock()
	defer si.Unlock()

	for i := range values {
		values[i] = si.deduplicateUnsafe(values[i])
	}
}

// SendStats sends interner metrics (hits, misses, size) tagged with the interner name.
func (si *LRUStringInterner) SendStats(client statsd.ClientInterface, hitsMetric, missesMetric, sizeMetric string) error {
	tags := []string{"interner:" + si.name}

	if hits := si.hits.Swap(0); hits > 0 {
		if err := client.Count(hitsMetric, hits, tags, 1.0); err != nil {
			return err
		}
	}
	if misses := si.misses.Swap(0); misses > 0 {
		if err := client.Count(missesMetric, misses, tags, 1.0); err != nil {
			return err
		}
	}

	si.Lock()
	size := si.store.Len()
	si.Unlock()

	return client.Gauge(sizeMetric, float64(size), tags, 1.0)
}
