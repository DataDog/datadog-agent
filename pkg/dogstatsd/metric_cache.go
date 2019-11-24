// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package dogstatsd

import (
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/hashicorp/golang-lru"
)

type MetricCacheItem struct {
	Name       string
	Mtype      metrics.MetricType
	Tags       []string
	Host       string
	SampleRate float64
}

type metricCache struct {
	cache *lru.Cache
}

func newMetricCache(size int) (*metricCache, error) {
	cache, err := lru.New(size)
	if err != nil {
		return &metricCache{}, err
	}
	return &metricCache{cache: cache}, nil
}

func (m *metricCache) get(metricName string) (*MetricCacheItem) {
	if result, ok := m.cache.Get(metricName); ok {
		return result.(*MetricCacheItem)
	}
	return nil
}

func (m *metricCache) add(metricName string, sample *MetricCacheItem) {
	m.cache.Add(metricName, sample)
}
