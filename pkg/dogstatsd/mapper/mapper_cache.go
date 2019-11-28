// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package mapper

import (
	"github.com/hashicorp/golang-lru"
)

type mapperCache struct {
	cache *lru.Cache
}

// newMapperCache creates a new mapperCache
func newMapperCache(size int) (*mapperCache, error) {
	cache, err := lru.New(size)
	if err != nil {
		return &mapperCache{}, err
	}
	return &mapperCache{cache: cache}, nil
}

// get returns:
// - a MapResult if found, otherwise nil
// - a boolean indicating if a match has been found
func (m *mapperCache) get(metricName string) (*MapResult, bool) {
	if result, ok := m.cache.Get(metricName); ok {
		return result.(*MapResult), true
	}
	return nil, false
}

// add adds MapResult to cache with metric name as key
func (m *mapperCache) add(metricName string, mapResult *MapResult) {
	m.cache.Add(metricName, mapResult)
}
