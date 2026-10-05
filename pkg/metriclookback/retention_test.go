// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metriclookback/ringbuffer"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	serializermocks "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

func TestRetentionAppendSamplesAndForwardRange(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	err := retention.AppendSamples(context.Background(), ringbuffer.Source{Kind: ringbuffer.SourceCheckShadow, ID: "check:1"}, []metrics.MetricSample{
		{Name: "metric.1", Value: 1, Mtype: metrics.GaugeType, Timestamp: 10},
		{Name: "metric.2", Value: 2, Mtype: metrics.GaugeType, Timestamp: 20},
	})
	require.NoError(t, err)

	serializer := serializermocks.NewMetricSerializer(t)
	serializer.On("SendIterableSeries", mock.Anything).Run(func(args mock.Arguments) {
		source := args.Get(0).(metrics.SerieSource)
		require.Equal(t, uint64(1), source.Count())
		require.True(t, source.MoveNext())
		require.Equal(t, "metric.2", source.Current().Name)
		require.False(t, source.MoveNext())
	}).Return(nil).Once()

	count, err := retention.ForwardRange(serializer, time.Unix(15, 0), time.Unix(25, 0))
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestRetentionForwardRangeIsHalfOpen(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	err := retention.AppendSamples(context.Background(), ringbuffer.Source{Kind: ringbuffer.SourceCheckShadow, ID: "check:1"}, []metrics.MetricSample{
		{Name: "metric.1", Value: 1, Mtype: metrics.GaugeType, Timestamp: 10},
		{Name: "metric.2", Value: 2, Mtype: metrics.GaugeType, Timestamp: 20},
	})
	require.NoError(t, err)

	serializer := serializermocks.NewMetricSerializer(t)
	serializer.On("SendIterableSeries", mock.Anything).Run(func(args mock.Arguments) {
		source := args.Get(0).(metrics.SerieSource)
		require.Equal(t, uint64(1), source.Count())
		require.True(t, source.MoveNext())
		require.Equal(t, "metric.1", source.Current().Name)
	}).Return(nil).Once()

	count, err := retention.ForwardRange(serializer, time.Unix(10, 0), time.Unix(20, 0))
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestRetentionForwardRangeIncludesFinalEligibleMicrosecond(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	err := retention.AppendSamples(context.Background(), ringbuffer.Source{Kind: ringbuffer.SourceCheckShadow, ID: "check:1"}, []metrics.MetricSample{
		{Name: "metric.edge", Value: 1, Mtype: metrics.GaugeType, Timestamp: 20},
		{Name: "metric.after", Value: 2, Mtype: metrics.GaugeType, Timestamp: 21},
	})
	require.NoError(t, err)

	serializer := serializermocks.NewMetricSerializer(t)
	serializer.On("SendIterableSeries", mock.Anything).Run(func(args mock.Arguments) {
		source := args.Get(0).(metrics.SerieSource)
		require.Equal(t, uint64(1), source.Count())
		require.True(t, source.MoveNext())
		require.Equal(t, "metric.edge", source.Current().Name)
		require.False(t, source.MoveNext())
	}).Return(nil).Once()

	count, err := retention.ForwardRange(serializer, time.Unix(19, 0), time.Unix(20, 500))
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestRetentionForwardRangeExcludesMicrosecondBeforeSubMicrosecondLowerBound(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	err := retention.AppendSamples(context.Background(), ringbuffer.Source{Kind: ringbuffer.SourceCheckShadow, ID: "check:1"}, []metrics.MetricSample{
		{Name: "metric.before", Value: 1, Mtype: metrics.GaugeType, Timestamp: 20},
		{Name: "metric.after", Value: 2, Mtype: metrics.GaugeType, Timestamp: 20.001},
	})
	require.NoError(t, err)

	serializer := serializermocks.NewMetricSerializer(t)
	serializer.On("SendIterableSeries", mock.Anything).Run(func(args mock.Arguments) {
		source := args.Get(0).(metrics.SerieSource)
		require.Equal(t, uint64(1), source.Count())
		require.True(t, source.MoveNext())
		require.Equal(t, "metric.after", source.Current().Name)
		require.False(t, source.MoveNext())
	}).Return(nil).Once()

	count, err := retention.ForwardRange(serializer, time.Unix(20, 500), time.Unix(21, 0))
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestRetentionAppendSketchSeriesAndForwardRange(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	err := retention.AppendSketchSeries(context.Background(), ringbuffer.Source{Kind: ringbuffer.SourceDogStatsDBucketed}, &metrics.SketchSeries{
		DistributionMetadata: metrics.DistributionMetadata{
			Name:    "dist.metric",
			Tags:    tagset.CompositeTagsFromSlice([]string{"env:test"}),
			Host:    "host",
			NoIndex: true,
			Source:  metrics.MetricSourceDogstatsd,
		},
		Points: []metrics.SketchPoint{
			{Ts: 10, Sketch: testSketchData(1, 3)},
			{Ts: 20, Sketch: testSketchData(2, 4)},
		},
	})
	require.NoError(t, err)

	serializer := serializermocks.NewMetricSerializer(t)
	serializer.On("SendSketch", mock.Anything).Run(func(args mock.Arguments) {
		source := args.Get(0).(metrics.SketchesSource)
		require.Equal(t, uint64(1), source.Count())
		require.True(t, source.WaitForValue())
		require.True(t, source.MoveNext())
		series, ok := source.Current().(*metrics.SketchSeries)
		require.True(t, ok)
		require.Equal(t, "dist.metric", series.Name)
		require.Equal(t, "host", series.Host)
		require.Equal(t, []string{"env:test"}, series.Tags.UnsafeToReadOnlySliceString())
		require.True(t, series.NoIndex)
		require.Equal(t, metrics.MetricSourceDogstatsd, series.Source)
		require.Len(t, series.Points, 1)
		require.Equal(t, int64(20), series.Points[0].Ts)
		cnt, min, max, sum, avg := series.Points[0].Sketch.BasicStats()
		require.Equal(t, int64(2), cnt)
		require.Equal(t, float64(2), min)
		require.Equal(t, float64(4), max)
		require.Equal(t, float64(6), sum)
		require.Equal(t, float64(3), avg)
		require.False(t, source.MoveNext())
	}).Return(nil).Once()

	count, err := retention.ForwardRange(serializer, time.Unix(15, 0), time.Unix(25, 0))
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestRetentionForwardRangeSketchOnlyDoesNotRequireScalarRing(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	require.Nil(t, retention.buffer)

	err := retention.AppendSketchSeries(context.Background(), ringbuffer.Source{Kind: ringbuffer.SourceDogStatsDBucketed}, &metrics.SketchSeries{
		DistributionMetadata: metrics.DistributionMetadata{Name: "dist.metric"},
		Points: []metrics.SketchPoint{
			{Ts: 20, Sketch: testSketchData(2, 4)},
		},
	})
	require.NoError(t, err)
	require.Nil(t, retention.buffer)
	require.Equal(t, 1, retention.SketchStats().Records)

	serializer := serializermocks.NewMetricSerializer(t)
	serializer.On("SendSketch", mock.Anything).Return(nil).Once()

	count, err := retention.ForwardRange(serializer, time.Unix(15, 0), time.Unix(25, 0))
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestRetentionProjectsSketchPointsWithPlaceholderAverage(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	err := retention.AppendSketchSeries(context.Background(), ringbuffer.Source{Kind: ringbuffer.SourceDogStatsDBucketed}, &metrics.SketchSeries{
		DistributionMetadata: metrics.DistributionMetadata{Name: "dist.metric"},
		Points: []metrics.SketchPoint{
			{Ts: 10, Sketch: testSketchData(1, 5)},
			{Ts: 20, Sketch: testSketchData(2, 6)},
		},
	})
	require.NoError(t, err)

	points := retention.ProjectedSketchPointsBetweenSources(
		[]ringbuffer.Source{{Kind: ringbuffer.SourceDogStatsDBucketed}},
		"dist.metric",
		time.Unix(15, 0),
		time.Unix(25, 0),
		PlaceholderAverageSketchProjection{},
	)
	require.Equal(t, []ringbuffer.Point{{Ts: time.Unix(20, 0), Value: 4}}, points)
}

func testSketchData(values ...float64) *quantile.Sketch {
	var agent quantile.Agent
	for _, value := range values {
		agent.Insert(value, 1)
	}
	return agent.Finish()
}
