// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metric_history

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSerieSink is a simple mock that records all appended series
type mockSerieSink struct {
	series []*metrics.Serie
}

func (m *mockSerieSink) Append(serie *metrics.Serie) {
	m.series = append(m.series, serie)
}

func TestObservingSink_ForwardsToDelegate(t *testing.T) {
	// Create mock delegate
	delegate := &mockSerieSink{}
	cache := NewMetricHistoryCache()
	sink := NewObservingSink(delegate, cache)

	// Create a test serie
	serie := &metrics.Serie{
		Name:       "test.metric",
		Points:     []metrics.Point{{Ts: 1000.0, Value: 42.0}},
		Tags:       tagset.CompositeTagsFromSlice([]string{"tag1:value1"}),
		Host:       "test-host",
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "test-host", []string{"tag1:value1"}),
	}

	// Append the serie
	sink.Append(serie)

	// Verify it was forwarded to the delegate
	require.Len(t, delegate.series, 1)
	assert.Equal(t, serie, delegate.series[0])
}

func TestObservingSink_CallsCacheObserve(t *testing.T) {
	// Create mock delegate
	delegate := &mockSerieSink{}
	cache := NewMetricHistoryCache()
	sink := NewObservingSink(delegate, cache)

	// Create a test serie
	serie := &metrics.Serie{
		Name:       "test.metric",
		Points:     []metrics.Point{{Ts: 1000.0, Value: 42.0}},
		Tags:       tagset.CompositeTagsFromSlice([]string{"tag1:value1"}),
		Host:       "test-host",
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "test-host", []string{"tag1:value1"}),
	}

	// Cache should be empty initially
	assert.Equal(t, 0, cache.SeriesCount())

	// Append the serie
	sink.Append(serie)

	// Verify cache was updated
	assert.Equal(t, 1, cache.SeriesCount())

	// Verify the history was stored correctly
	history := cache.GetHistory(serie.ContextKey)
	require.NotNil(t, history)
	assert.Equal(t, "test.metric", history.Key.Name)
	assert.Equal(t, metrics.APIGaugeType, history.Type)
	assert.Equal(t, 1, history.Recent.Len())
}

func TestObservingSink_NilCacheDoesNotCrash(t *testing.T) {
	// Create sink with nil cache
	delegate := &mockSerieSink{}
	sink := NewObservingSink(delegate, nil)

	// Create a test serie
	serie := &metrics.Serie{
		Name:       "test.metric",
		Points:     []metrics.Point{{Ts: 1000.0, Value: 42.0}},
		Tags:       tagset.CompositeTagsFromSlice([]string{"tag1:value1"}),
		Host:       "test-host",
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "test-host", []string{"tag1:value1"}),
	}

	// Should not crash even with nil cache
	require.NotPanics(t, func() {
		sink.Append(serie)
	})

	// Verify it was still forwarded to the delegate
	require.Len(t, delegate.series, 1)
	assert.Equal(t, serie, delegate.series[0])
}

func TestObservingSink_MultipleSeries(t *testing.T) {
	// Create mock delegate
	delegate := &mockSerieSink{}
	cache := NewMetricHistoryCache()
	sink := NewObservingSink(delegate, cache)

	// Create multiple test series
	series := []*metrics.Serie{
		{
			Name:       "test.metric1",
			Points:     []metrics.Point{{Ts: 1000.0, Value: 42.0}},
			Tags:       tagset.CompositeTagsFromSlice([]string{"tag1:value1"}),
			Host:       "test-host",
			MType:      metrics.APIGaugeType,
			ContextKey: generateContextKey("test.metric1", "test-host", []string{"tag1:value1"}),
		},
		{
			Name:       "test.metric2",
			Points:     []metrics.Point{{Ts: 2000.0, Value: 100.0}},
			Tags:       tagset.CompositeTagsFromSlice([]string{"tag2:value2"}),
			Host:       "test-host",
			MType:      metrics.APICountType,
			ContextKey: generateContextKey("test.metric2", "test-host", []string{"tag2:value2"}),
		},
		{
			Name:       "test.metric3",
			Points:     []metrics.Point{{Ts: 3000.0, Value: 99.9}},
			Tags:       tagset.CompositeTagsFromSlice([]string{"tag3:value3"}),
			Host:       "test-host",
			MType:      metrics.APIRateType,
			ContextKey: generateContextKey("test.metric3", "test-host", []string{"tag3:value3"}),
		},
	}

	// Append all series
	for _, serie := range series {
		sink.Append(serie)
	}

	// Verify all were forwarded to the delegate
	require.Len(t, delegate.series, 3)
	for i, serie := range series {
		assert.Equal(t, serie, delegate.series[i])
	}

	// Verify all were cached
	assert.Equal(t, 3, cache.SeriesCount())

	// Verify each history
	for _, serie := range series {
		history := cache.GetHistory(serie.ContextKey)
		require.NotNil(t, history)
		assert.Equal(t, serie.Name, history.Key.Name)
		assert.Equal(t, serie.MType, history.Type)
		assert.Equal(t, 1, history.Recent.Len())
	}
}
