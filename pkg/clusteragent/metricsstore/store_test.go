// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package metricsstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewMetricsStore(t *testing.T) {
	store := NewMetricsStore(func(_ interface{}) StructuredMetrics { return nil }, nil, nil, nil, nil)

	assert.NotNil(t, store)
	assert.NotNil(t, store.generateMetricsFunc)
}

func TestMetricsStoreAdd(t *testing.T) {
	store := NewMetricsStore(func(_ interface{}) StructuredMetrics {
		return StructuredMetrics{
			{Name: "test.metric", Type: MetricTypeGauge, Value: 1.0, Tags: []string{"test:tag"}},
		}
	}, nil, nil, nil, nil)

	key := "test-ns/test-resource"
	store.Add(key, nil)

	var foundKey string
	store.metrics.Range(func(k, v interface{}) bool {
		foundKey = k.(string)
		e := v.(*keyEntry)
		e.mu.RLock()
		defer e.mu.RUnlock()
		assert.Len(t, e.metrics, 1)
		for _, metric := range e.metrics {
			assert.Equal(t, "test.metric", metric.Name)
		}
		return true
	})

	assert.Equal(t, key, foundKey)
}

func TestMetricsStoreDelete(t *testing.T) {
	store := NewMetricsStore(func(_ interface{}) StructuredMetrics {
		return StructuredMetrics{
			{Name: "test.metric", Type: MetricTypeGauge, Value: 1.0, Tags: []string{"test:tag"}},
		}
	}, nil, nil, nil, nil)

	key := "test-ns/test-resource"

	store.Add(key, nil)

	_, ok := store.metrics.Load(key)
	assert.True(t, ok)

	store.Delete(key)

	_, ok = store.metrics.Load(key)
	assert.False(t, ok)
}

func TestPushGauge_SingleMetric(t *testing.T) {
	store := NewMetricsStore(func(_ interface{}) StructuredMetrics { return nil }, nil, nil, nil, nil)
	key := "test-ns/test-resource"

	store.PushGauge(key, "cpu.limit", 4.0, []string{"container:app"})

	actual, ok := store.metrics.Load(key)
	assert.True(t, ok)
	e := actual.(*keyEntry)
	e.mu.RLock()
	defer e.mu.RUnlock()
	assert.Len(t, e.metrics, 1)
}

func TestPushGauge_SameNameDifferentTags(t *testing.T) {
	store := NewMetricsStore(func(_ interface{}) StructuredMetrics { return nil }, nil, nil, nil, nil)
	key := "test-ns/test-resource"

	store.PushGauge(key, "cpu.limit", 4.0, []string{"container:app"})
	store.PushGauge(key, "cpu.limit", 8.0, []string{"container:sidecar"})

	actual, ok := store.metrics.Load(key)
	assert.True(t, ok)
	e := actual.(*keyEntry)
	e.mu.RLock()
	defer e.mu.RUnlock()
	assert.Len(t, e.metrics, 2)
}

func TestPushGauge_SameNameSameTagsUpdatesValue(t *testing.T) {
	store := NewMetricsStore(func(_ interface{}) StructuredMetrics { return nil }, nil, nil, nil, nil)
	key := "test-ns/test-resource"

	store.PushGauge(key, "cpu.limit", 4.0, []string{"container:app"})
	store.PushGauge(key, "cpu.limit", 8.0, []string{"container:app"})

	actual, ok := store.metrics.Load(key)
	assert.True(t, ok)
	e := actual.(*keyEntry)
	e.mu.RLock()
	defer e.mu.RUnlock()
	assert.Len(t, e.metrics, 1)
	for _, m := range e.metrics {
		assert.Equal(t, 8.0, m.Value)
	}
}

func TestPushGauge_TagOrderNormalized(t *testing.T) {
	store := NewMetricsStore(func(_ interface{}) StructuredMetrics { return nil }, nil, nil, nil, nil)
	key := "test-ns/test-resource"

	store.PushGauge(key, "cpu.limit", 4.0, []string{"b:2", "a:1"})
	store.PushGauge(key, "cpu.limit", 8.0, []string{"a:1", "b:2"})

	actual, ok := store.metrics.Load(key)
	assert.True(t, ok)
	e := actual.(*keyEntry)
	e.mu.RLock()
	defer e.mu.RUnlock()
	// Different tag order resolves to the same identity → single entry, last value wins
	assert.Len(t, e.metrics, 1)
	for _, m := range e.metrics {
		assert.Equal(t, 8.0, m.Value)
	}
}

func TestPushGauge_CoexistsWithGenerated(t *testing.T) {
	store := NewMetricsStore(func(_ interface{}) StructuredMetrics {
		return StructuredMetrics{
			{Name: "generated.metric", Type: MetricTypeGauge, Value: 1.0, Tags: []string{"gen:tag"}},
		}
	}, nil, nil, nil, nil)
	key := "test-ns/test-resource"

	store.PushGauge(key, "pushed.metric", 2.0, []string{"pushed:tag"})
	store.PushGauge(key, "pushed.metric2", 3.0, []string{"pushed:tag"})
	store.Add(key, nil)

	actual, ok := store.metrics.Load(key)
	assert.True(t, ok)
	e := actual.(*keyEntry)
	e.mu.RLock()
	defer e.mu.RUnlock()
	// Add replaces the whole map; only the generated metric remains
	assert.Len(t, e.metrics, 1)
}

func TestPushGauge_TagsAreCloned(t *testing.T) {
	store := NewMetricsStore(func(_ interface{}) StructuredMetrics { return nil }, nil, nil, nil, nil)
	key := "test-ns/test-resource"

	tags := []string{"container:app", "version:1"}
	store.PushGauge(key, "cpu.limit", 4.0, tags)

	// Mutate the original slice after the call
	tags[0] = "container:mutated"

	actual, ok := store.metrics.Load(key)
	assert.True(t, ok)
	e := actual.(*keyEntry)
	e.mu.RLock()
	defer e.mu.RUnlock()
	id := metricIdentity("cpu.limit", []string{"container:app", "version:1"})
	stored, found := e.metrics[id]
	assert.True(t, found, "entry should still be found under the original identity")
	assert.Equal(t, "container:app", stored.Tags[0], "stored tags must not reflect caller mutation")
}

func TestAdd_KeyTagsStoredFromKeyTagsFunc(t *testing.T) {
	store := NewMetricsStore(
		func(_ interface{}) StructuredMetrics { return nil },
		func(_ interface{}) []string { return []string{"env:prod", "team:backend"} },
		nil, nil, nil,
	)
	key := "test-ns/test-resource"

	store.Add(key, nil)

	actual, ok := store.metrics.Load(key)
	assert.True(t, ok)
	e := actual.(*keyEntry)
	e.mu.RLock()
	defer e.mu.RUnlock()
	assert.Equal(t, []string{"env:prod", "team:backend"}, e.keyTags)
}

func TestAdd_KeyTagsUpdatedOnSubsequentAdd(t *testing.T) {
	callCount := 0
	store := NewMetricsStore(
		func(_ interface{}) StructuredMetrics { return nil },
		func(_ interface{}) []string {
			callCount++
			if callCount == 1 {
				return []string{"env:staging"}
			}
			return []string{"env:prod"}
		},
		nil, nil, nil,
	)
	key := "test-ns/test-resource"

	store.Add(key, nil)
	store.Add(key, nil)

	actual, ok := store.metrics.Load(key)
	assert.True(t, ok)
	e := actual.(*keyEntry)
	e.mu.RLock()
	defer e.mu.RUnlock()
	assert.Equal(t, []string{"env:prod"}, e.keyTags)
}

func TestDelete_RemovesAllMetrics(t *testing.T) {
	store := NewMetricsStore(func(_ interface{}) StructuredMetrics {
		return StructuredMetrics{
			{Name: "generated.metric", Type: MetricTypeGauge, Value: 1.0, Tags: []string{"gen:tag"}},
		}
	}, nil, nil, nil, nil)
	key := "test-ns/test-resource"

	store.Add(key, nil)
	store.PushGauge(key, "pushed.metric", 2.0, []string{"pushed:tag"})
	store.Delete(key)

	_, ok := store.metrics.Load(key)
	assert.False(t, ok)
}
