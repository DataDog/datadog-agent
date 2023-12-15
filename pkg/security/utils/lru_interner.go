// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"sync"

	"github.com/hashicorp/golang-lru/v2/simplelru"
)

// LRUStringInterner is a best-effort LRU-based string deduplicator
type LRUStringInterner struct {
	sync.Mutex
	store *simplelru.LRU[string, string]
}

// NewLRUStringInterner returns a new LRUStringInterner, with the cache size provided
// if the cache size is negative this function will panic
func NewLRUStringInterner(size int) *LRUStringInterner {
	store, err := simplelru.NewLRU[string, string](size, nil)
	if err != nil {
		panic(err)
	}

	return &LRUStringInterner{
		store: store,
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
		return res
	}

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
