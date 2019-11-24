// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package dogstatsd

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMetricCache(t *testing.T) {
	c, err := newMetricCache(3)
	assert.NoError(t, err)
	assert.Equal(t, 0, c.cache.Len())

	// add to cache
	metric1 := &MetricCacheItem{
		Name: "my.metric.1",
	}
	metric2 := &MetricCacheItem{
		Name: "my.metric.2",
	}
	metric3 := &MetricCacheItem{
		Name: "my.metric.3",
	}
	c.add("metric1", metric1)
	c.add("metric2", metric2)
	c.add("metric3", metric3)
	assert.Equal(t, 3, c.cache.Len())

	// adding more metric than max size of the cache will flush the oldest one
	metric4 := &MetricCacheItem{
		Name: "my.metric.4",
	}
	c.add("metric4", metric4)

	assert.Equal(t, 3, c.cache.Len())
	assert.Equal(t, metric2, c.get("metric2"))
	assert.Equal(t, metric3, c.get("metric3"))
	assert.Equal(t, metric4, c.get("metric4"))
	assert.Equal(t, (*MetricCacheItem)(nil), c.get("metric1")) // metric1, the oldest metric has been removed
}
