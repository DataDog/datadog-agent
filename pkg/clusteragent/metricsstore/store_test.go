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
	store := NewMetricsStore(func(_ interface{}) StructuredMetrics { return nil }, nil, nil, nil)

	assert.NotNil(t, store)
	assert.NotNil(t, store.generateMetricsFunc)
}

func TestMetricsStoreAdd(t *testing.T) {
	store := NewMetricsStore(func(_ interface{}) StructuredMetrics {
		return StructuredMetrics{
			{Name: "test.metric", Type: MetricTypeGauge, Value: 1.0, Tags: []string{"test:tag"}},
		}
	}, nil, nil, nil)

	key := "test-ns/test-resource"

	store.Add(key, nil)

	var foundKey string
	store.metrics.Range(func(k, v interface{}) bool {
		foundKey = k.(string)
		metrics := v.(StructuredMetrics)
		assert.Len(t, metrics, 1)
		assert.Equal(t, "test.metric", metrics[0].Name)
		return true
	})

	assert.Equal(t, key, foundKey)
}

func TestMetricsStoreDelete(t *testing.T) {
	store := NewMetricsStore(func(_ interface{}) StructuredMetrics {
		return StructuredMetrics{
			{Name: "test.metric", Type: MetricTypeGauge, Value: 1.0, Tags: []string{"test:tag"}},
		}
	}, nil, nil, nil)

	key := "test-ns/test-resource"

	store.Add(key, nil)

	_, ok := store.metrics.Load(key)
	assert.True(t, ok)

	store.Delete(key)

	_, ok = store.metrics.Load(key)
	assert.False(t, ok)
}
