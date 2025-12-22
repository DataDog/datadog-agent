// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeSeriesStorage_Add(t *testing.T) {
	s := newTimeSeriesStorage()

	// Add first point
	s.Add("test", "my.metric", 10.0, 1000, []string{"env:prod"})
	series := s.GetSeries("test", "my.metric", []string{"env:prod"}, AggregateAverage)

	require.NotNil(t, series)
	assert.Equal(t, "test", series.Namespace)
	assert.Equal(t, "my.metric", series.Name)
	assert.Equal(t, []string{"env:prod"}, series.Tags)
	require.Len(t, series.Points, 1)
	assert.Equal(t, int64(1000), series.Points[0].Timestamp)
	assert.Equal(t, 10.0, series.Points[0].Value)
}

func TestTimeSeriesStorage_AddSameBucket_Average(t *testing.T) {
	s := newTimeSeriesStorage()

	// Add multiple points to same bucket
	s.Add("test", "my.metric", 10.0, 1000, []string{"env:prod"})
	s.Add("test", "my.metric", 20.0, 1000, []string{"env:prod"})
	s.Add("test", "my.metric", 5.0, 1000, []string{"env:prod"})
	series := s.GetSeries("test", "my.metric", []string{"env:prod"}, AggregateAverage)

	require.NotNil(t, series)
	require.Len(t, series.Points, 1)
	// Average of 10, 20, 5 = 35/3 = 11.67
	assert.InDelta(t, 11.67, series.Points[0].Value, 0.01)
}

func TestTimeSeriesStorage_AddSameBucket_Sum(t *testing.T) {
	s := newTimeSeriesStorage()

	s.Add("test", "my.metric", 10.0, 1000, nil)
	s.Add("test", "my.metric", 20.0, 1000, nil)
	s.Add("test", "my.metric", 5.0, 1000, nil)
	series := s.GetSeries("test", "my.metric", nil, AggregateSum)

	require.NotNil(t, series)
	require.Len(t, series.Points, 1)
	assert.Equal(t, 35.0, series.Points[0].Value)
}

func TestTimeSeriesStorage_AddSameBucket_Count(t *testing.T) {
	s := newTimeSeriesStorage()

	s.Add("test", "my.metric", 10.0, 1000, nil)
	s.Add("test", "my.metric", 20.0, 1000, nil)
	s.Add("test", "my.metric", 5.0, 1000, nil)
	series := s.GetSeries("test", "my.metric", nil, AggregateCount)

	require.NotNil(t, series)
	require.Len(t, series.Points, 1)
	assert.Equal(t, 3.0, series.Points[0].Value)
}

func TestTimeSeriesStorage_AddSameBucket_MinMax(t *testing.T) {
	s := newTimeSeriesStorage()

	s.Add("test", "my.metric", 10.0, 1000, nil)
	s.Add("test", "my.metric", 20.0, 1000, nil)
	s.Add("test", "my.metric", 5.0, 1000, nil)

	minSeries := s.GetSeries("test", "my.metric", nil, AggregateMin)
	maxSeries := s.GetSeries("test", "my.metric", nil, AggregateMax)

	require.NotNil(t, minSeries)
	require.NotNil(t, maxSeries)
	assert.Equal(t, 5.0, minSeries.Points[0].Value)
	assert.Equal(t, 20.0, maxSeries.Points[0].Value)
}

func TestTimeSeriesStorage_AddDifferentBuckets(t *testing.T) {
	s := newTimeSeriesStorage()

	// Add points to different buckets
	s.Add("test", "my.metric", 10.0, 1000, []string{"env:prod"})
	s.Add("test", "my.metric", 20.0, 1001, []string{"env:prod"})
	s.Add("test", "my.metric", 30.0, 1002, []string{"env:prod"})
	series := s.GetSeries("test", "my.metric", []string{"env:prod"}, AggregateAverage)

	require.NotNil(t, series)
	require.Len(t, series.Points, 3)
	// Points should be sorted by timestamp
	assert.Equal(t, int64(1000), series.Points[0].Timestamp)
	assert.Equal(t, int64(1001), series.Points[1].Timestamp)
	assert.Equal(t, int64(1002), series.Points[2].Timestamp)
	assert.Equal(t, 10.0, series.Points[0].Value)
	assert.Equal(t, 20.0, series.Points[1].Value)
	assert.Equal(t, 30.0, series.Points[2].Value)
}

func TestTimeSeriesStorage_DifferentTags(t *testing.T) {
	s := newTimeSeriesStorage()

	// Different tags = different series
	s.Add("test", "my.metric", 10.0, 1000, []string{"env:prod"})
	s.Add("test", "my.metric", 20.0, 1000, []string{"env:staging"})

	prodSeries := s.GetSeries("test", "my.metric", []string{"env:prod"}, AggregateAverage)
	stagingSeries := s.GetSeries("test", "my.metric", []string{"env:staging"}, AggregateAverage)

	require.NotNil(t, prodSeries)
	require.NotNil(t, stagingSeries)
	assert.Equal(t, 10.0, prodSeries.Points[0].Value)
	assert.Equal(t, 20.0, stagingSeries.Points[0].Value)
}

func TestTimeSeriesStorage_TagOrderDoesNotMatter(t *testing.T) {
	s := newTimeSeriesStorage()

	// Tags in different order should be same series
	s.Add("test", "my.metric", 10.0, 1000, []string{"a:1", "b:2"})
	s.Add("test", "my.metric", 20.0, 1000, []string{"b:2", "a:1"})

	series := s.GetSeries("test", "my.metric", []string{"a:1", "b:2"}, AggregateAverage)
	require.NotNil(t, series)
	// Average of 10 and 20 = 15
	assert.Equal(t, 15.0, series.Points[0].Value)
}

func TestTimeSeriesStorage_GetSeries_NotFound(t *testing.T) {
	s := newTimeSeriesStorage()

	series := s.GetSeries("test", "nonexistent", nil, AggregateAverage)
	assert.Nil(t, series)
}

func TestTimeSeriesStorage_AllSeries(t *testing.T) {
	s := newTimeSeriesStorage()

	// Add series to different namespaces
	s.Add("ns1", "metric1", 10.0, 1000, nil)
	s.Add("ns1", "metric2", 20.0, 1000, nil)
	s.Add("ns2", "metric3", 30.0, 1000, nil)

	ns1Series := s.AllSeries("ns1", AggregateAverage)
	ns2Series := s.AllSeries("ns2", AggregateAverage)

	assert.Len(t, ns1Series, 2)
	assert.Len(t, ns2Series, 1)
}

func TestStatPoint_aggregate(t *testing.T) {
	p := &statPoint{
		Sum:   100.0,
		Count: 4,
		Min:   10.0,
		Max:   40.0,
	}

	assert.Equal(t, 25.0, p.aggregate(AggregateAverage))
	assert.Equal(t, 100.0, p.aggregate(AggregateSum))
	assert.Equal(t, 4.0, p.aggregate(AggregateCount))
	assert.Equal(t, 10.0, p.aggregate(AggregateMin))
	assert.Equal(t, 40.0, p.aggregate(AggregateMax))

	// Zero count returns 0 for average
	p2 := &statPoint{Count: 0, Sum: 10.0}
	assert.Equal(t, 0.0, p2.aggregate(AggregateAverage))
}
