// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

func TestTimeSeriesStorage_Add(t *testing.T) {
	s := newTimeSeriesStorage()

	// Add first point
	stats := s.Add("test", "my.metric", 10.0, 1000, []string{"env:prod"})

	require.NotNil(t, stats)
	assert.Equal(t, "test", stats.Namespace)
	assert.Equal(t, "my.metric", stats.Name)
	assert.Equal(t, []string{"env:prod"}, stats.Tags)
	require.Len(t, stats.Points, 1)
	assert.Equal(t, int64(1000), stats.Points[0].Timestamp)
	assert.Equal(t, 10.0, stats.Points[0].Sum)
	assert.Equal(t, int64(1), stats.Points[0].Count)
	assert.Equal(t, 10.0, stats.Points[0].Min)
	assert.Equal(t, 10.0, stats.Points[0].Max)
}

func TestTimeSeriesStorage_AddSameBucket(t *testing.T) {
	s := newTimeSeriesStorage()

	// Add multiple points to same bucket
	s.Add("test", "my.metric", 10.0, 1000, []string{"env:prod"})
	s.Add("test", "my.metric", 20.0, 1000, []string{"env:prod"})
	stats := s.Add("test", "my.metric", 5.0, 1000, []string{"env:prod"})

	require.Len(t, stats.Points, 1)
	assert.Equal(t, 35.0, stats.Points[0].Sum)
	assert.Equal(t, int64(3), stats.Points[0].Count)
	assert.Equal(t, 5.0, stats.Points[0].Min)
	assert.Equal(t, 20.0, stats.Points[0].Max)
	assert.InDelta(t, 11.67, stats.Points[0].Value(), 0.01)
}

func TestTimeSeriesStorage_AddDifferentBuckets(t *testing.T) {
	s := newTimeSeriesStorage()

	// Add points to different buckets
	s.Add("test", "my.metric", 10.0, 1000, []string{"env:prod"})
	s.Add("test", "my.metric", 20.0, 1001, []string{"env:prod"})
	stats := s.Add("test", "my.metric", 30.0, 1002, []string{"env:prod"})

	require.Len(t, stats.Points, 3)
	// Points should be sorted by timestamp
	assert.Equal(t, int64(1000), stats.Points[0].Timestamp)
	assert.Equal(t, int64(1001), stats.Points[1].Timestamp)
	assert.Equal(t, int64(1002), stats.Points[2].Timestamp)
}

func TestTimeSeriesStorage_DifferentTags(t *testing.T) {
	s := newTimeSeriesStorage()

	// Different tags = different series
	s.Add("test", "my.metric", 10.0, 1000, []string{"env:prod"})
	s.Add("test", "my.metric", 20.0, 1000, []string{"env:staging"})

	prodStats := s.GetSeries("test", "my.metric", []string{"env:prod"})
	stagingStats := s.GetSeries("test", "my.metric", []string{"env:staging"})

	require.NotNil(t, prodStats)
	require.NotNil(t, stagingStats)
	assert.Equal(t, 10.0, prodStats.Points[0].Sum)
	assert.Equal(t, 20.0, stagingStats.Points[0].Sum)
}

func TestTimeSeriesStorage_TagOrderDoesNotMatter(t *testing.T) {
	s := newTimeSeriesStorage()

	// Tags in different order should be same series
	s.Add("test", "my.metric", 10.0, 1000, []string{"a:1", "b:2"})
	s.Add("test", "my.metric", 20.0, 1000, []string{"b:2", "a:1"})

	stats := s.GetSeries("test", "my.metric", []string{"a:1", "b:2"})
	require.NotNil(t, stats)
	assert.Equal(t, 30.0, stats.Points[0].Sum)
	assert.Equal(t, int64(2), stats.Points[0].Count)
}

func TestTimeSeriesStorage_GetSeries_NotFound(t *testing.T) {
	s := newTimeSeriesStorage()

	stats := s.GetSeries("test", "nonexistent", nil)
	assert.Nil(t, stats)
}

func TestTimeSeriesStorage_AllSeries(t *testing.T) {
	s := newTimeSeriesStorage()

	// Add series to different namespaces
	s.Add("ns1", "metric1", 10.0, 1000, nil)
	s.Add("ns1", "metric2", 20.0, 1000, nil)
	s.Add("ns2", "metric3", 30.0, 1000, nil)

	ns1Series := s.AllSeries("ns1")
	ns2Series := s.AllSeries("ns2")

	assert.Len(t, ns1Series, 2)
	assert.Len(t, ns2Series, 1)
}

func TestStatPoint_Value(t *testing.T) {
	// Zero count
	p := &observer.StatPoint{Count: 0, Sum: 10.0}
	assert.Equal(t, 0.0, p.Value())

	// Normal case
	p = &observer.StatPoint{Count: 4, Sum: 100.0}
	assert.Equal(t, 25.0, p.Value())
}
