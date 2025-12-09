// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metric_history

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// generateContextKey creates a ContextKey for testing purposes
func generateContextKey(name, hostname string, tags []string) ckey.ContextKey {
	keygen := ckey.NewKeyGenerator()
	return keygen.Generate(name, hostname, tagset.NewHashingTagsAccumulatorWithTags(tags))
}

func TestNewMetricHistoryCache(t *testing.T) {
	cache := NewMetricHistoryCache()

	assert.NotNil(t, cache)
	assert.Equal(t, 0, cache.SeriesCount())
	assert.Equal(t, 20, cache.recentCapacity)
	assert.Equal(t, 60, cache.mediumCapacity)
	assert.Equal(t, 24, cache.longCapacity)
	assert.Equal(t, 0, len(cache.includePrefixes))
}

func TestObserveStoresPointsInRecentBuffer(t *testing.T) {
	cache := NewMetricHistoryCache()

	// Create a test serie
	tags := []string{"env:prod", "host:web01"}
	serie := &metrics.Serie{
		Name:       "test.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 42.5}, {Ts: 2000.0, Value: 43.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "", tags),
	}

	cache.Observe(serie)

	// Verify series was stored
	assert.Equal(t, 1, cache.SeriesCount())

	// Get the history
	history := cache.GetHistory(serie.ContextKey)
	require.NotNil(t, history)

	// Verify the series key
	assert.Equal(t, serie.ContextKey, history.Key.ContextKey)
	assert.Equal(t, "test.metric", history.Key.Name)
	assert.Equal(t, []string{"env:prod", "host:web01"}, history.Key.Tags)
	assert.Equal(t, metrics.APIGaugeType, history.Type)

	// Verify the points were stored in Recent buffer
	assert.Equal(t, 2, history.Recent.Len())

	// Verify first point
	point1 := history.Recent.Get(0)
	assert.Equal(t, int64(1000), point1.Timestamp)
	assert.Equal(t, int64(1), point1.Stats.Count)
	assert.Equal(t, 42.5, point1.Stats.Sum)
	assert.Equal(t, 42.5, point1.Stats.Min)
	assert.Equal(t, 42.5, point1.Stats.Max)

	// Verify second point
	point2 := history.Recent.Get(1)
	assert.Equal(t, int64(2000), point2.Timestamp)
	assert.Equal(t, int64(1), point2.Stats.Count)
	assert.Equal(t, 43.0, point2.Stats.Sum)
	assert.Equal(t, 43.0, point2.Stats.Min)
	assert.Equal(t, 43.0, point2.Stats.Max)

	// Verify LastSeen was updated to the latest timestamp
	assert.Equal(t, int64(2000), history.LastSeen)
}

func TestObserveMultipleSeries(t *testing.T) {
	cache := NewMetricHistoryCache()

	// Create two different series
	tags1 := []string{"host:web01"}
	serie1 := &metrics.Serie{
		Name:       "cpu.usage",
		Tags:       tagset.CompositeTagsFromSlice(tags1),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 50.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("cpu.usage", "", tags1),
	}

	tags2 := []string{"host:web01"}
	serie2 := &metrics.Serie{
		Name:       "memory.usage",
		Tags:       tagset.CompositeTagsFromSlice(tags2),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 80.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("memory.usage", "", tags2),
	}

	cache.Observe(serie1)
	cache.Observe(serie2)

	// Verify both series were stored
	assert.Equal(t, 2, cache.SeriesCount())

	// Verify each series has its own history
	history1 := cache.GetHistory(serie1.ContextKey)
	require.NotNil(t, history1)
	assert.Equal(t, "cpu.usage", history1.Key.Name)
	assert.Equal(t, 1, history1.Recent.Len())

	history2 := cache.GetHistory(serie2.ContextKey)
	require.NotNil(t, history2)
	assert.Equal(t, "memory.usage", history2.Key.Name)
	assert.Equal(t, 1, history2.Recent.Len())
}

func TestObserveUpdatesExistingSeries(t *testing.T) {
	cache := NewMetricHistoryCache()

	tags := []string{"env:prod"}
	contextKey := generateContextKey("test.metric", "", tags)

	// Create first observation
	serie1 := &metrics.Serie{
		Name:       "test.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 10.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: contextKey,
	}

	cache.Observe(serie1)

	// Create second observation with the same context key
	serie2 := &metrics.Serie{
		Name:       "test.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 2000.0, Value: 20.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: contextKey,
	}

	cache.Observe(serie2)

	// Verify only one series is stored
	assert.Equal(t, 1, cache.SeriesCount())

	// Verify both points are in the history
	history := cache.GetHistory(contextKey)
	require.NotNil(t, history)
	assert.Equal(t, 2, history.Recent.Len())

	point1 := history.Recent.Get(0)
	assert.Equal(t, int64(1000), point1.Timestamp)
	assert.Equal(t, 10.0, point1.Stats.Sum)

	point2 := history.Recent.Get(1)
	assert.Equal(t, int64(2000), point2.Timestamp)
	assert.Equal(t, 20.0, point2.Stats.Sum)
}

func TestPrefixFilteringMatching(t *testing.T) {
	cache := NewMetricHistoryCache()
	cache.SetIncludePrefixes([]string{"system.", "app."})

	// Create series with matching and non-matching prefixes
	tags := []string{"host:web01"}
	matchingSerie1 := &metrics.Serie{
		Name:       "system.cpu.usage",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 50.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("system.cpu.usage", "", tags),
	}

	matchingSerie2 := &metrics.Serie{
		Name:       "app.requests",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 100.0}},
		MType:      metrics.APICountType,
		ContextKey: generateContextKey("app.requests", "", tags),
	}

	nonMatchingSerie := &metrics.Serie{
		Name:       "custom.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 25.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("custom.metric", "", tags),
	}

	cache.Observe(matchingSerie1)
	cache.Observe(matchingSerie2)
	cache.Observe(nonMatchingSerie)

	// Verify only matching series were stored
	assert.Equal(t, 2, cache.SeriesCount())

	history1 := cache.GetHistory(matchingSerie1.ContextKey)
	assert.NotNil(t, history1)

	history2 := cache.GetHistory(matchingSerie2.ContextKey)
	assert.NotNil(t, history2)

	history3 := cache.GetHistory(nonMatchingSerie.ContextKey)
	assert.Nil(t, history3)
}

func TestPrefixFilteringEmptyIncludesAll(t *testing.T) {
	cache := NewMetricHistoryCache()
	// No prefixes set, should include all metrics

	tags1 := []string{"host:web01"}
	serie1 := &metrics.Serie{
		Name:       "any.metric.name",
		Tags:       tagset.CompositeTagsFromSlice(tags1),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 10.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("any.metric.name", "", tags1),
	}

	tags2 := []string{"host:web02"}
	serie2 := &metrics.Serie{
		Name:       "another.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags2),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 20.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("another.metric", "", tags2),
	}

	cache.Observe(serie1)
	cache.Observe(serie2)

	// Verify all series were stored
	assert.Equal(t, 2, cache.SeriesCount())

	history1 := cache.GetHistory(serie1.ContextKey)
	assert.NotNil(t, history1)

	history2 := cache.GetHistory(serie2.ContextKey)
	assert.NotNil(t, history2)
}

func TestGetHistoryNotFound(t *testing.T) {
	cache := NewMetricHistoryCache()

	// Create a context key that doesn't exist
	nonExistentKey := generateContextKey("nonexistent.metric", "", []string{"tag:value"})

	history := cache.GetHistory(nonExistentKey)
	assert.Nil(t, history)
}

func TestBufferCapacities(t *testing.T) {
	cache := NewMetricHistoryCache()

	tags := []string{"env:prod"}
	serie := &metrics.Serie{
		Name:       "test.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 1.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "", tags),
	}

	cache.Observe(serie)

	history := cache.GetHistory(serie.ContextKey)
	require.NotNil(t, history)

	// Verify buffer capacities match cache configuration
	assert.Equal(t, 20, history.Recent.Cap())
	assert.Equal(t, 60, history.Medium.Cap())
	assert.Equal(t, 24, history.Long.Cap())
}

func TestObserveWithMultiplePoints(t *testing.T) {
	cache := NewMetricHistoryCache()

	// Create a serie with multiple points
	points := make([]metrics.Point, 10)
	for i := 0; i < 10; i++ {
		points[i] = metrics.Point{Ts: float64(1000 + i*100), Value: float64(i * 10)}
	}

	tags := []string{"env:prod"}
	serie := &metrics.Serie{
		Name:       "test.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     points,
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "", tags),
	}

	cache.Observe(serie)

	history := cache.GetHistory(serie.ContextKey)
	require.NotNil(t, history)

	// Verify all points were stored
	assert.Equal(t, 10, history.Recent.Len())

	// Verify points are in the correct order
	for i := 0; i < 10; i++ {
		point := history.Recent.Get(i)
		assert.Equal(t, int64(1000+i*100), point.Timestamp)
		assert.Equal(t, float64(i*10), point.Stats.Sum)
	}

	// Verify LastSeen is the latest timestamp
	assert.Equal(t, int64(1900), history.LastSeen)
}

func TestMatchesPrefix(t *testing.T) {
	cache := NewMetricHistoryCache()

	// Test with no prefixes (should match all)
	assert.True(t, cache.matchesPrefix("any.metric"))
	assert.True(t, cache.matchesPrefix("another.metric"))

	// Set prefixes
	cache.SetIncludePrefixes([]string{"system.", "app."})

	// Test matching prefixes
	assert.True(t, cache.matchesPrefix("system.cpu"))
	assert.True(t, cache.matchesPrefix("system.memory.usage"))
	assert.True(t, cache.matchesPrefix("app.requests"))
	assert.True(t, cache.matchesPrefix("app.errors.total"))

	// Test non-matching prefixes
	assert.False(t, cache.matchesPrefix("custom.metric"))
	assert.False(t, cache.matchesPrefix("other.metric"))
	assert.False(t, cache.matchesPrefix("sys.metric")) // partial match doesn't count
}

func TestObserveWithEmptyPoints(t *testing.T) {
	cache := NewMetricHistoryCache()

	tags := []string{"env:prod"}
	serie := &metrics.Serie{
		Name:       "test.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{}, // Empty points
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "", tags),
	}

	cache.Observe(serie)

	// Series should still be created even with no points
	assert.Equal(t, 1, cache.SeriesCount())

	history := cache.GetHistory(serie.ContextKey)
	require.NotNil(t, history)
	assert.Equal(t, 0, history.Recent.Len())
	assert.Equal(t, int64(0), history.LastSeen)
}
