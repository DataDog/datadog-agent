// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"sync"

	"github.com/hashicorp/golang-lru/v2/simplelru"
)

type LRUStringInterner struct {
	sync.RWMutex
	store *simplelru.LRU[string, string]
}

func NewLRUStringInterner(size int) *LRUStringInterner {
	store, err := simplelru.NewLRU[string, string](size, nil)
	if err != nil {
		panic(err)
	}

	return &LRUStringInterner{
		store: store,
	}
}

func (si *LRUStringInterner) Deduplicate(value string) string {
	if res, ok := si.store.Get(value); ok {
		return res
	}

	si.store.Add(value, value)
	return value
}

func (si *LRUStringInterner) DeduplicateSlice(values []string) {
	for i := range values {
		values[i] = si.Deduplicate(values[i])
	}
}
