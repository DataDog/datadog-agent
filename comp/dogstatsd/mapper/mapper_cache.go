// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mapper

import (
	lru "github.com/hashicorp/golang-lru/v2"
)

type mapperCache struct {
	cache *lru.Cache[string, *MapResult]
}

// newMapperCache creates a new mapperCache
func newMapperCache(size int) (*mapperCache, error) {
	panic("not called")
}

// get returns:
// - a MapResult if found, otherwise nil
// - a boolean indicating if a match has been found
func (m *mapperCache) get(metricName string) (*MapResult, bool) {
	panic("not called")
}

// add adds MapResult to cache with metric name as key
func (m *mapperCache) add(metricName string, mapResult *MapResult) {
	panic("not called")
}
