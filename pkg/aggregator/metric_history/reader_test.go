// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metric_history

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestListSeriesReturnsAllTrackedSeries(t *testing.T) {
	cache := NewMetricHistoryCache()

	// Add multiple series
	tags1 := []string{"env:prod"}
	serie1 := &metrics.Serie{
		Name:       "cpu.usage",
		Tags:       tagset.CompositeTagsFromSlice(tags1),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 50.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("cpu.usage", "", tags1),
	}

	tags2 := []string{"env:dev"}
	serie2 := &metrics.Serie{
		Name:       "memory.usage",
		Tags:       tagset.CompositeTagsFromSlice(tags2),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 80.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("memory.usage", "", tags2),
	}

	tags3 := []string{"env:staging"}
	serie3 := &metrics.Serie{
		Name:       "disk.usage",
		Tags:       tagset.CompositeTagsFromSlice(tags3),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 60.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("disk.usage", "", tags3),
	}

	cache.Observe(serie1)
	cache.Observe(serie2)
	cache.Observe(serie3)

	// Get all series keys
	keys := cache.ListSeries()

	assert.Equal(t, 3, len(keys))

	// Verify all series are present
	metricNames := make(map[string]bool)
	for _, key := range keys {
		metricNames[key.Name] = true
	}

	assert.True(t, metricNames["cpu.usage"])
	assert.True(t, metricNames["memory.usage"])
	assert.True(t, metricNames["disk.usage"])
}

func TestListSeriesEmptyCache(t *testing.T) {
	cache := NewMetricHistoryCache()

	keys := cache.ListSeries()
	assert.Empty(t, keys)
}

func TestGetRecentReturnsCorrectData(t *testing.T) {
	cache := NewMetricHistoryCache()

	tags := []string{"env:prod"}
	serie := &metrics.Serie{
		Name:       "test.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 10.0}, {Ts: 2000.0, Value: 20.0}, {Ts: 3000.0, Value: 30.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "", tags),
	}

	cache.Observe(serie)

	// Create SeriesKey for lookup
	key := SeriesKey{
		ContextKey: serie.ContextKey,
		Name:       serie.Name,
		Tags:       tags,
	}

	points := cache.GetRecent(key)
	require.NotNil(t, points)
	assert.Equal(t, 3, len(points))

	// Verify points are in correct order
	assert.Equal(t, int64(1000), points[0].Timestamp)
	assert.Equal(t, 10.0, points[0].Stats.Sum)

	assert.Equal(t, int64(2000), points[1].Timestamp)
	assert.Equal(t, 20.0, points[1].Stats.Sum)

	assert.Equal(t, int64(3000), points[2].Timestamp)
	assert.Equal(t, 30.0, points[2].Stats.Sum)
}

func TestGetRecentNonExistentSeries(t *testing.T) {
	cache := NewMetricHistoryCache()

	key := SeriesKey{
		ContextKey: generateContextKey("nonexistent.metric", "", []string{"tag:value"}),
		Name:       "nonexistent.metric",
		Tags:       []string{"tag:value"},
	}

	points := cache.GetRecent(key)
	assert.Nil(t, points)
}

func TestGetMediumReturnsCorrectData(t *testing.T) {
	cache := NewMetricHistoryCache()

	tags := []string{"env:prod"}
	serie := &metrics.Serie{
		Name:       "test.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 10.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "", tags),
	}

	cache.Observe(serie)

	// Get the history and manually add a point to Medium tier for testing
	history := cache.GetHistory(serie.ContextKey)
	require.NotNil(t, history)

	mediumPoint := DataPoint{
		Timestamp: 60000,
		Stats: SummaryStats{
			Count: 4,
			Sum:   100.0,
			Min:   20.0,
			Max:   30.0,
		},
	}
	history.Medium.Push(mediumPoint)

	// Create SeriesKey for lookup
	key := SeriesKey{
		ContextKey: serie.ContextKey,
		Name:       serie.Name,
		Tags:       tags,
	}

	points := cache.GetMedium(key)
	require.NotNil(t, points)
	assert.Equal(t, 1, len(points))

	assert.Equal(t, int64(60000), points[0].Timestamp)
	assert.Equal(t, int64(4), points[0].Stats.Count)
	assert.Equal(t, 100.0, points[0].Stats.Sum)
	assert.Equal(t, 25.0, points[0].Stats.Mean())
}

func TestGetLongReturnsCorrectData(t *testing.T) {
	cache := NewMetricHistoryCache()

	tags := []string{"env:prod"}
	serie := &metrics.Serie{
		Name:       "test.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 10.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "", tags),
	}

	cache.Observe(serie)

	// Get the history and manually add a point to Long tier for testing
	history := cache.GetHistory(serie.ContextKey)
	require.NotNil(t, history)

	longPoint := DataPoint{
		Timestamp: 3600000,
		Stats: SummaryStats{
			Count: 240,
			Sum:   12000.0,
			Min:   40.0,
			Max:   60.0,
		},
	}
	history.Long.Push(longPoint)

	// Create SeriesKey for lookup
	key := SeriesKey{
		ContextKey: serie.ContextKey,
		Name:       serie.Name,
		Tags:       tags,
	}

	points := cache.GetLong(key)
	require.NotNil(t, points)
	assert.Equal(t, 1, len(points))

	assert.Equal(t, int64(3600000), points[0].Timestamp)
	assert.Equal(t, int64(240), points[0].Stats.Count)
	assert.Equal(t, 12000.0, points[0].Stats.Sum)
	assert.Equal(t, 50.0, points[0].Stats.Mean())
}

func TestGetScalarSeriesExtractsMean(t *testing.T) {
	cache := NewMetricHistoryCache()

	tags := []string{"env:prod"}
	serie := &metrics.Serie{
		Name:       "test.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 10.0}, {Ts: 2000.0, Value: 20.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "", tags),
	}

	cache.Observe(serie)

	key := SeriesKey{
		ContextKey: serie.ContextKey,
		Name:       serie.Name,
		Tags:       tags,
	}

	values := cache.GetScalarSeries(key, TierRecent, "mean")
	require.NotNil(t, values)
	assert.Equal(t, 2, len(values))

	assert.Equal(t, int64(1000), values[0].Timestamp)
	assert.Equal(t, 10.0, values[0].Value)

	assert.Equal(t, int64(2000), values[1].Timestamp)
	assert.Equal(t, 20.0, values[1].Value)
}

func TestGetScalarSeriesExtractsMin(t *testing.T) {
	cache := NewMetricHistoryCache()

	tags := []string{"env:prod"}
	serie := &metrics.Serie{
		Name:       "test.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 10.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "", tags),
	}

	cache.Observe(serie)

	// Get the history and manually add a point with different min/max for testing
	history := cache.GetHistory(serie.ContextKey)
	require.NotNil(t, history)

	testPoint := DataPoint{
		Timestamp: 2000,
		Stats: SummaryStats{
			Count: 5,
			Sum:   100.0,
			Min:   15.0,
			Max:   25.0,
		},
	}
	history.Recent.Push(testPoint)

	key := SeriesKey{
		ContextKey: serie.ContextKey,
		Name:       serie.Name,
		Tags:       tags,
	}

	values := cache.GetScalarSeries(key, TierRecent, "min")
	require.NotNil(t, values)
	assert.Equal(t, 2, len(values))

	assert.Equal(t, int64(1000), values[0].Timestamp)
	assert.Equal(t, 10.0, values[0].Value)

	assert.Equal(t, int64(2000), values[1].Timestamp)
	assert.Equal(t, 15.0, values[1].Value)
}

func TestGetScalarSeriesExtractsMax(t *testing.T) {
	cache := NewMetricHistoryCache()

	tags := []string{"env:prod"}
	serie := &metrics.Serie{
		Name:       "test.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 10.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "", tags),
	}

	cache.Observe(serie)

	// Get the history and manually add a point with different min/max for testing
	history := cache.GetHistory(serie.ContextKey)
	require.NotNil(t, history)

	testPoint := DataPoint{
		Timestamp: 2000,
		Stats: SummaryStats{
			Count: 5,
			Sum:   100.0,
			Min:   15.0,
			Max:   25.0,
		},
	}
	history.Recent.Push(testPoint)

	key := SeriesKey{
		ContextKey: serie.ContextKey,
		Name:       serie.Name,
		Tags:       tags,
	}

	values := cache.GetScalarSeries(key, TierRecent, "max")
	require.NotNil(t, values)
	assert.Equal(t, 2, len(values))

	assert.Equal(t, int64(1000), values[0].Timestamp)
	assert.Equal(t, 10.0, values[0].Value)

	assert.Equal(t, int64(2000), values[1].Timestamp)
	assert.Equal(t, 25.0, values[1].Value)
}

func TestGetScalarSeriesExtractsCount(t *testing.T) {
	cache := NewMetricHistoryCache()

	tags := []string{"env:prod"}
	serie := &metrics.Serie{
		Name:       "test.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 10.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "", tags),
	}

	cache.Observe(serie)

	// Get the history and manually add a point with different count
	history := cache.GetHistory(serie.ContextKey)
	require.NotNil(t, history)

	testPoint := DataPoint{
		Timestamp: 2000,
		Stats: SummaryStats{
			Count: 5,
			Sum:   100.0,
			Min:   15.0,
			Max:   25.0,
		},
	}
	history.Recent.Push(testPoint)

	key := SeriesKey{
		ContextKey: serie.ContextKey,
		Name:       serie.Name,
		Tags:       tags,
	}

	values := cache.GetScalarSeries(key, TierRecent, "count")
	require.NotNil(t, values)
	assert.Equal(t, 2, len(values))

	assert.Equal(t, int64(1000), values[0].Timestamp)
	assert.Equal(t, 1.0, values[0].Value)

	assert.Equal(t, int64(2000), values[1].Timestamp)
	assert.Equal(t, 5.0, values[1].Value)
}

func TestGetScalarSeriesExtractsSum(t *testing.T) {
	cache := NewMetricHistoryCache()

	tags := []string{"env:prod"}
	serie := &metrics.Serie{
		Name:       "test.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 10.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "", tags),
	}

	cache.Observe(serie)

	// Get the history and manually add a point with different sum
	history := cache.GetHistory(serie.ContextKey)
	require.NotNil(t, history)

	testPoint := DataPoint{
		Timestamp: 2000,
		Stats: SummaryStats{
			Count: 5,
			Sum:   100.0,
			Min:   15.0,
			Max:   25.0,
		},
	}
	history.Recent.Push(testPoint)

	key := SeriesKey{
		ContextKey: serie.ContextKey,
		Name:       serie.Name,
		Tags:       tags,
	}

	values := cache.GetScalarSeries(key, TierRecent, "sum")
	require.NotNil(t, values)
	assert.Equal(t, 2, len(values))

	assert.Equal(t, int64(1000), values[0].Timestamp)
	assert.Equal(t, 10.0, values[0].Value)

	assert.Equal(t, int64(2000), values[1].Timestamp)
	assert.Equal(t, 100.0, values[1].Value)
}

func TestGetScalarSeriesFromDifferentTiers(t *testing.T) {
	cache := NewMetricHistoryCache()

	tags := []string{"env:prod"}
	serie := &metrics.Serie{
		Name:       "test.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 10.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "", tags),
	}

	cache.Observe(serie)

	// Add points to different tiers
	history := cache.GetHistory(serie.ContextKey)
	require.NotNil(t, history)

	mediumPoint := DataPoint{
		Timestamp: 60000,
		Stats:     SummaryStats{Count: 4, Sum: 100.0, Min: 20.0, Max: 30.0},
	}
	history.Medium.Push(mediumPoint)

	longPoint := DataPoint{
		Timestamp: 3600000,
		Stats:     SummaryStats{Count: 240, Sum: 12000.0, Min: 40.0, Max: 60.0},
	}
	history.Long.Push(longPoint)

	key := SeriesKey{
		ContextKey: serie.ContextKey,
		Name:       serie.Name,
		Tags:       tags,
	}

	// Test Recent tier
	recentValues := cache.GetScalarSeries(key, TierRecent, "mean")
	require.NotNil(t, recentValues)
	assert.Equal(t, 1, len(recentValues))
	assert.Equal(t, int64(1000), recentValues[0].Timestamp)

	// Test Medium tier
	mediumValues := cache.GetScalarSeries(key, TierMedium, "mean")
	require.NotNil(t, mediumValues)
	assert.Equal(t, 1, len(mediumValues))
	assert.Equal(t, int64(60000), mediumValues[0].Timestamp)
	assert.Equal(t, 25.0, mediumValues[0].Value)

	// Test Long tier
	longValues := cache.GetScalarSeries(key, TierLong, "mean")
	require.NotNil(t, longValues)
	assert.Equal(t, 1, len(longValues))
	assert.Equal(t, int64(3600000), longValues[0].Timestamp)
	assert.Equal(t, 50.0, longValues[0].Value)
}

func TestGetScalarSeriesNonExistentSeries(t *testing.T) {
	cache := NewMetricHistoryCache()

	key := SeriesKey{
		ContextKey: generateContextKey("nonexistent.metric", "", []string{"tag:value"}),
		Name:       "nonexistent.metric",
		Tags:       []string{"tag:value"},
	}

	values := cache.GetScalarSeries(key, TierRecent, "mean")
	assert.Nil(t, values)
}

func TestGetScalarSeriesInvalidAspect(t *testing.T) {
	cache := NewMetricHistoryCache()

	tags := []string{"env:prod"}
	serie := &metrics.Serie{
		Name:       "test.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 10.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "", tags),
	}

	cache.Observe(serie)

	key := SeriesKey{
		ContextKey: serie.ContextKey,
		Name:       serie.Name,
		Tags:       tags,
	}

	// Use an invalid aspect - should default to mean
	values := cache.GetScalarSeries(key, TierRecent, "invalid_aspect")
	require.NotNil(t, values)
	assert.Equal(t, 1, len(values))
	assert.Equal(t, 10.0, values[0].Value) // Should get mean value
}

func TestScanIteratesAllSeries(t *testing.T) {
	cache := NewMetricHistoryCache()

	// Add multiple series
	series := []struct {
		name  string
		tags  []string
		value float64
	}{
		{"cpu.usage", []string{"host:web01"}, 50.0},
		{"memory.usage", []string{"host:web02"}, 80.0},
		{"disk.usage", []string{"host:web03"}, 60.0},
	}

	for _, s := range series {
		serie := &metrics.Serie{
			Name:       s.name,
			Tags:       tagset.CompositeTagsFromSlice(s.tags),
			Points:     []metrics.Point{{Ts: 1000.0, Value: s.value}},
			MType:      metrics.APIGaugeType,
			ContextKey: generateContextKey(s.name, "", s.tags),
		}
		cache.Observe(serie)
	}

	// Scan and collect all series names
	var scannedNames []string
	cache.Scan(func(key SeriesKey, history *MetricHistory) bool {
		scannedNames = append(scannedNames, key.Name)
		return true // continue iteration
	})

	assert.Equal(t, 3, len(scannedNames))

	// Verify all series were scanned
	nameMap := make(map[string]bool)
	for _, name := range scannedNames {
		nameMap[name] = true
	}

	assert.True(t, nameMap["cpu.usage"])
	assert.True(t, nameMap["memory.usage"])
	assert.True(t, nameMap["disk.usage"])
}

func TestScanStopsWhenFunctionReturnsFalse(t *testing.T) {
	cache := NewMetricHistoryCache()

	// Add multiple series
	series := []struct {
		name  string
		tags  []string
		value float64
	}{
		{"cpu.usage", []string{"host:web01"}, 50.0},
		{"memory.usage", []string{"host:web02"}, 80.0},
		{"disk.usage", []string{"host:web03"}, 60.0},
		{"network.usage", []string{"host:web04"}, 40.0},
	}

	for _, s := range series {
		serie := &metrics.Serie{
			Name:       s.name,
			Tags:       tagset.CompositeTagsFromSlice(s.tags),
			Points:     []metrics.Point{{Ts: 1000.0, Value: s.value}},
			MType:      metrics.APIGaugeType,
			ContextKey: generateContextKey(s.name, "", s.tags),
		}
		cache.Observe(serie)
	}

	// Scan and stop after 2 series
	var scannedCount int
	cache.Scan(func(key SeriesKey, history *MetricHistory) bool {
		scannedCount++
		return scannedCount < 2 // stop after 2 iterations
	})

	assert.Equal(t, 2, scannedCount)
}

func TestScanEmptyCache(t *testing.T) {
	cache := NewMetricHistoryCache()

	scannedCount := 0
	cache.Scan(func(key SeriesKey, history *MetricHistory) bool {
		scannedCount++
		return true
	})

	assert.Equal(t, 0, scannedCount)
}

func TestScanProvidesCorrectHistoryData(t *testing.T) {
	cache := NewMetricHistoryCache()

	tags := []string{"env:prod"}
	serie := &metrics.Serie{
		Name:       "test.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 42.5}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "", tags),
	}

	cache.Observe(serie)

	// Scan and verify the history data
	var foundHistory *MetricHistory
	cache.Scan(func(key SeriesKey, history *MetricHistory) bool {
		if key.Name == "test.metric" {
			foundHistory = history
			return false // stop after finding
		}
		return true
	})

	require.NotNil(t, foundHistory)
	assert.Equal(t, "test.metric", foundHistory.Key.Name)
	assert.Equal(t, metrics.APIGaugeType, foundHistory.Type)
	assert.Equal(t, 1, foundHistory.Recent.Len())

	point := foundHistory.Recent.Get(0)
	assert.Equal(t, int64(1000), point.Timestamp)
	assert.Equal(t, 42.5, point.Stats.Sum)
}
