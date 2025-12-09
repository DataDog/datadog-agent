// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metric_history

import (
	"testing"
	"time"

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

func TestExpireRemovesStaleSeriesOnly(t *testing.T) {
	cache := NewMetricHistoryCache()

	// Create series with different timestamps
	tags1 := []string{"host:web01"}
	oldSerie := &metrics.Serie{
		Name:       "old.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags1),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 10.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("old.metric", "", tags1),
	}

	tags2 := []string{"host:web02"}
	recentSerie := &metrics.Serie{
		Name:       "recent.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags2),
		Points:     []metrics.Point{{Ts: 2000.0, Value: 20.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("recent.metric", "", tags2),
	}

	cache.Observe(oldSerie)
	cache.Observe(recentSerie)

	// Verify both series exist
	assert.Equal(t, 2, cache.SeriesCount())

	// Run expiration with a timestamp that should expire the old serie
	// oldSerie was last seen at 1000, so with expiry duration of 25 minutes (1500 seconds),
	// it should be expired when nowTimestamp > 1000 + 1500 = 2500
	nowTimestamp := int64(2600)
	expired := cache.Expire(nowTimestamp)

	// Should have expired 1 series
	assert.Equal(t, 1, expired)
	assert.Equal(t, 1, cache.SeriesCount())

	// Old series should be gone
	oldHistory := cache.GetHistory(oldSerie.ContextKey)
	assert.Nil(t, oldHistory)

	// Recent series should still exist
	recentHistory := cache.GetHistory(recentSerie.ContextKey)
	assert.NotNil(t, recentHistory)
}

func TestExpireWithActivelyUpdatedSeries(t *testing.T) {
	cache := NewMetricHistoryCache()

	tags := []string{"env:prod"}
	contextKey := generateContextKey("active.metric", "", tags)

	// Create initial observation
	serie1 := &metrics.Serie{
		Name:       "active.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 10.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: contextKey,
	}

	cache.Observe(serie1)

	// Update with recent data
	serie2 := &metrics.Serie{
		Name:       "active.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 2000.0, Value: 20.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: contextKey,
	}

	cache.Observe(serie2)

	// Verify series exists and LastSeen is updated
	assert.Equal(t, 1, cache.SeriesCount())
	history := cache.GetHistory(contextKey)
	require.NotNil(t, history)
	assert.Equal(t, int64(2000), history.LastSeen)

	// Run expiration with a timestamp that would have expired the original point,
	// but not the updated one
	nowTimestamp := int64(2500)
	expired := cache.Expire(nowTimestamp)

	// Should not have expired any series
	assert.Equal(t, 0, expired)
	assert.Equal(t, 1, cache.SeriesCount())

	// Series should still exist
	history = cache.GetHistory(contextKey)
	assert.NotNil(t, history)
}

func TestExpireWithNoStaleMetrics(t *testing.T) {
	cache := NewMetricHistoryCache()

	// Create recent series
	tags := []string{"env:prod"}
	serie := &metrics.Serie{
		Name:       "recent.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 2000.0, Value: 10.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("recent.metric", "", tags),
	}

	cache.Observe(serie)
	assert.Equal(t, 1, cache.SeriesCount())

	// Run expiration with a timestamp that shouldn't expire anything
	nowTimestamp := int64(2100)
	expired := cache.Expire(nowTimestamp)

	// Should not have expired any series
	assert.Equal(t, 0, expired)
	assert.Equal(t, 1, cache.SeriesCount())
}

func TestExpireWithEmptyCache(t *testing.T) {
	cache := NewMetricHistoryCache()

	// Run expiration on empty cache
	nowTimestamp := int64(1000)
	expired := cache.Expire(nowTimestamp)

	// Should not have expired any series
	assert.Equal(t, 0, expired)
	assert.Equal(t, 0, cache.SeriesCount())
}

func TestExpireWithCustomExpiryDuration(t *testing.T) {
	cache := NewMetricHistoryCache()

	// Set a shorter expiry duration
	cfg := DefaultConfig()
	cfg.ExpiryDuration = 5 * time.Minute // 300 seconds
	cache.Configure(cfg)

	// Create series
	tags := []string{"env:prod"}
	serie := &metrics.Serie{
		Name:       "test.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		Points:     []metrics.Point{{Ts: 1000.0, Value: 10.0}},
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "", tags),
	}

	cache.Observe(serie)
	assert.Equal(t, 1, cache.SeriesCount())

	// Run expiration with a timestamp that exceeds the custom expiry duration
	// LastSeen = 1000, expiry = 300, so should expire at nowTimestamp > 1300
	nowTimestamp := int64(1400)
	expired := cache.Expire(nowTimestamp)

	// Should have expired the series
	assert.Equal(t, 1, expired)
	assert.Equal(t, 0, cache.SeriesCount())
}

func TestExpireWithMultipleSeriesAtDifferentAges(t *testing.T) {
	cache := NewMetricHistoryCache()

	// Create series at different timestamps
	series := []struct {
		name      string
		timestamp float64
		tags      []string
	}{
		{"very.old.metric", 1000.0, []string{"age:very_old"}},
		{"old.metric", 2000.0, []string{"age:old"}},
		{"recent.metric", 3000.0, []string{"age:recent"}},
		{"very.recent.metric", 4000.0, []string{"age:very_recent"}},
	}

	for _, s := range series {
		serie := &metrics.Serie{
			Name:       s.name,
			Tags:       tagset.CompositeTagsFromSlice(s.tags),
			Points:     []metrics.Point{{Ts: s.timestamp, Value: 10.0}},
			MType:      metrics.APIGaugeType,
			ContextKey: generateContextKey(s.name, "", s.tags),
		}
		cache.Observe(serie)
	}

	assert.Equal(t, 4, cache.SeriesCount())

	// Expire with timestamp that should remove the two oldest
	// Expiry duration is 25 minutes (1500 seconds)
	// nowTimestamp = 3600, so threshold = 3600 - 1500 = 2100
	// Should expire: very.old.metric (1000) and old.metric (2000)
	// Should keep: recent.metric (3000) and very.recent.metric (4000)
	nowTimestamp := int64(3600)
	expired := cache.Expire(nowTimestamp)

	assert.Equal(t, 2, expired)
	assert.Equal(t, 2, cache.SeriesCount())

	// Verify the correct series remain
	assert.Nil(t, cache.GetHistory(generateContextKey("very.old.metric", "", []string{"age:very_old"})))
	assert.Nil(t, cache.GetHistory(generateContextKey("old.metric", "", []string{"age:old"})))
	assert.NotNil(t, cache.GetHistory(generateContextKey("recent.metric", "", []string{"age:recent"})))
	assert.NotNil(t, cache.GetHistory(generateContextKey("very.recent.metric", "", []string{"age:very_recent"})))
}

func TestConfigureUpdatesExpiryDuration(t *testing.T) {
	cache := NewMetricHistoryCache()

	// Verify default expiry duration
	assert.Equal(t, 25*time.Minute, cache.expiryDuration)

	// Configure with a different expiry duration
	cfg := DefaultConfig()
	cfg.ExpiryDuration = 10 * time.Minute
	cache.Configure(cfg)

	assert.Equal(t, 10*time.Minute, cache.expiryDuration)
}
