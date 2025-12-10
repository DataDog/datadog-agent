// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metric_history

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshotRoundTrip(t *testing.T) {
	// Create a cache with some data
	cache := NewMetricHistoryCache()

	// Add a series with data in all tiers
	tags1 := []string{"env:test", "host:local"}
	serie := &metrics.Serie{
		Name:       "test.metric",
		Tags:       tagset.CompositeTagsFromSlice(tags1),
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("test.metric", "", tags1),
		Points: []metrics.Point{
			{Ts: 100, Value: 10.0},
			{Ts: 115, Value: 15.0},
			{Ts: 130, Value: 12.0},
		},
	}
	cache.Observe(serie)

	// Add another series
	tags2 := []string{"env:prod"}
	serie2 := &metrics.Serie{
		Name:       "test.counter",
		Tags:       tagset.CompositeTagsFromSlice(tags2),
		MType:      metrics.APICountType,
		ContextKey: generateContextKey("test.counter", "", tags2),
		Points: []metrics.Point{
			{Ts: 100, Value: 1.0},
			{Ts: 115, Value: 2.0},
		},
	}
	cache.Observe(serie2)

	// Save snapshot
	tmpDir := t.TempDir()
	snapshotPath := filepath.Join(tmpDir, "test_snapshot.json")
	err := SaveSnapshot(cache, snapshotPath)
	require.NoError(t, err)

	// Verify file exists and is valid JSON
	data, err := os.ReadFile(snapshotPath)
	require.NoError(t, err)
	assert.True(t, len(data) > 0)

	// Load snapshot into new cache
	loadedCache, err := LoadSnapshot(snapshotPath)
	require.NoError(t, err)

	// Verify series count
	assert.Equal(t, cache.SeriesCount(), loadedCache.SeriesCount())

	// Verify data for first series
	originalKeys := cache.ListSeries()
	for _, key := range originalKeys {
		originalRecent := cache.GetRecent(key)
		loadedRecent := loadedCache.GetRecent(key)

		require.NotNil(t, loadedRecent, "series %s should exist in loaded cache", key.Name)
		assert.Equal(t, len(originalRecent), len(loadedRecent), "series %s should have same point count", key.Name)

		for i := range originalRecent {
			assert.Equal(t, originalRecent[i].Timestamp, loadedRecent[i].Timestamp)
			assert.Equal(t, originalRecent[i].Stats.Sum, loadedRecent[i].Stats.Sum)
			assert.Equal(t, originalRecent[i].Stats.Count, loadedRecent[i].Stats.Count)
		}
	}
}

func TestCaptureSnapshot(t *testing.T) {
	cache := NewMetricHistoryCache()

	tags := []string{"a:1"}
	serie := &metrics.Serie{
		Name:       "snapshot.test",
		Tags:       tagset.CompositeTagsFromSlice(tags),
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey("snapshot.test", "", tags),
		Points: []metrics.Point{
			{Ts: 1000, Value: 42.0},
		},
	}
	cache.Observe(serie)

	snapshot := CaptureSnapshot(cache)

	assert.Len(t, snapshot.Series, 1)
	assert.Equal(t, "snapshot.test", snapshot.Series[0].Name)
	assert.Equal(t, []string{"a:1"}, snapshot.Series[0].Tags)
	assert.Len(t, snapshot.Series[0].Recent, 1)
	assert.Equal(t, int64(1000), snapshot.Series[0].Recent[0].Timestamp)
	assert.Equal(t, 42.0, snapshot.Series[0].Recent[0].Sum)
}

func TestRestoreSnapshot(t *testing.T) {
	snapshot := &Snapshot{
		Series: []SeriesSnapshot{
			{
				Name:     "restored.metric",
				Tags:     []string{"env:test"},
				Type:     int(metrics.APIGaugeType),
				LastSeen: 500,
				Recent: []DataPointSnapshot{
					{Timestamp: 100, Count: 1, Sum: 10.0, Min: 10.0, Max: 10.0},
					{Timestamp: 115, Count: 1, Sum: 20.0, Min: 20.0, Max: 20.0},
				},
				Medium: []DataPointSnapshot{},
				Long:   []DataPointSnapshot{},
			},
		},
	}

	cache := RestoreSnapshot(snapshot)

	assert.Equal(t, 1, cache.SeriesCount())

	keys := cache.ListSeries()
	require.Len(t, keys, 1)
	assert.Equal(t, "restored.metric", keys[0].Name)

	recent := cache.GetRecent(keys[0])
	assert.Len(t, recent, 2)
	assert.Equal(t, int64(100), recent[0].Timestamp)
	assert.Equal(t, 10.0, recent[0].Stats.Sum)
	assert.Equal(t, int64(115), recent[1].Timestamp)
	assert.Equal(t, 20.0, recent[1].Stats.Sum)
}
