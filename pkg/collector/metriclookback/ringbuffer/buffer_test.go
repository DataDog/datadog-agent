// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ringbuffer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestAppendSamplesStoresCheckAndDogStatsDNoAggInOneRing(t *testing.T) {
	buf := New(Options{Capacity: 8, ShardCount: 1})

	err := buf.AppendSamples(context.Background(), Source{Kind: SourceCheckShadow, ID: "check:1"}, []metrics.MetricSample{{
		Name:      "check.metric",
		Value:     1,
		Mtype:     metrics.GaugeType,
		Timestamp: 10,
		Host:      "host-a",
		Tags:      []string{"b:2", "a:1", "a:1"},
		NoIndex:   true,
		Unit:      "unit",
	}})
	require.NoError(t, err)

	err = buf.AppendSamples(context.Background(), Source{Kind: SourceDogStatsDNoAggregation}, []metrics.MetricSample{{
		Name:      "dog.metric",
		Value:     2,
		Mtype:     metrics.GaugeType,
		Timestamp: 11,
		Host:      "host-b",
		Tags:      []string{"z:9"},
	}})
	require.NoError(t, err)

	series := buf.Series()
	require.Len(t, series, 2)

	require.Equal(t, "check.metric", series[0].Name)
	require.Equal(t, float64(10), series[0].Points[0].Ts)
	require.Equal(t, float64(1), series[0].Points[0].Value)
	require.Equal(t, []string{"a:1", "b:2"}, series[0].Tags.UnsafeToReadOnlySliceString())
	require.Equal(t, "host-a", series[0].Host)
	require.Equal(t, metrics.APIGaugeType, series[0].MType)
	require.True(t, series[0].NoIndex)
	require.Equal(t, "System", series[0].SourceTypeName)
	require.Equal(t, "unit", series[0].Unit)

	require.Equal(t, "dog.metric", series[1].Name)
	require.Equal(t, float64(11), series[1].Points[0].Ts)
	require.Equal(t, float64(2), series[1].Points[0].Value)
	require.Equal(t, []string{"z:9"}, series[1].Tags.UnsafeToReadOnlySliceString())
	require.Equal(t, "host-b", series[1].Host)
	require.Equal(t, metrics.APIGaugeType, series[1].MType)
	require.Equal(t, int64(dogStatsDNoAggregationInterval), series[1].Interval)
	require.Empty(t, series[1].SourceTypeName)
	require.Empty(t, series[1].Unit)
	require.Equal(t, metrics.MetricSource(0), series[1].Source)

	stats := buf.Stats()
	require.Equal(t, 8, stats.Capacity)
	require.Equal(t, 1, stats.ShardCount)
	require.Equal(t, 2, stats.Records)
	require.Equal(t, 2, stats.ActiveContexts)
	require.Equal(t, uint64(2), stats.AppendedSamples)
	require.Equal(t, uint64(0), stats.OverwrittenSamples)
	require.Equal(t, time.Unix(10, 0).UnixMicro(), stats.OldestUnixMicro)
	require.Equal(t, time.Unix(11, 0).UnixMicro(), stats.NewestUnixMicro)
}

func TestDogStatsDNoAggRateLikeSamplesUseNoAggAPISemantics(t *testing.T) {
	buf := New(Options{Capacity: 4, ShardCount: 1})

	err := buf.AppendSamples(context.Background(), Source{Kind: SourceDogStatsDNoAggregation}, []metrics.MetricSample{{
		Name:      "dog.counter",
		Value:     20,
		Mtype:     metrics.CounterType,
		Timestamp: 10,
	}})
	require.NoError(t, err)

	series := buf.Series()
	require.Len(t, series, 1)
	require.Equal(t, metrics.APIRateType, series[0].MType)
	require.Equal(t, int64(dogStatsDNoAggregationInterval), series[0].Interval)
	require.Equal(t, float64(2), series[0].Points[0].Value)
}

func TestAppendSeriePreservesNormalizedSeriesFields(t *testing.T) {
	buf := New(Options{Capacity: 4, ShardCount: 1})
	serie := &metrics.Serie{
		Name:     "dog.metric",
		Points:   []metrics.Point{{Ts: 10, Value: 2}},
		Tags:     tagset.CompositeTagsFromSlice([]string{"client:tag", "origin:tag"}),
		Host:     "host",
		MType:    metrics.APIRateType,
		Interval: 10,
	}

	require.NoError(t, buf.AppendSerie(context.Background(), Source{Kind: SourceDogStatsDNoAggregation}, serie))

	series := buf.Series()
	require.Len(t, series, 1)
	require.Equal(t, serie.Name, series[0].Name)
	require.Equal(t, serie.Points, series[0].Points)
	require.Equal(t, serie.Tags.UnsafeToReadOnlySliceString(), series[0].Tags.UnsafeToReadOnlySliceString())
	require.Equal(t, serie.Host, series[0].Host)
	require.Equal(t, serie.MType, series[0].MType)
	require.Equal(t, serie.Interval, series[0].Interval)
}

func TestAppendSerieRetainsContextPerPoint(t *testing.T) {
	buf := New(Options{Capacity: 2, ShardCount: 1})
	serie := &metrics.Serie{
		Name:     "dog.metric",
		Points:   []metrics.Point{{Ts: 10, Value: 2}, {Ts: 11, Value: 3}},
		Tags:     tagset.CompositeTagsFromSlice([]string{"client:tag"}),
		Host:     "host",
		MType:    metrics.APIGaugeType,
		Interval: 10,
	}

	require.NoError(t, buf.AppendSerie(context.Background(), Source{Kind: SourceDogStatsDNoAggregation}, serie))
	require.NoError(t, buf.AppendSamples(context.Background(), Source{Kind: SourceCheckShadow, ID: "check:1"}, []metrics.MetricSample{{
		Name:      "check.metric",
		Value:     4,
		Mtype:     metrics.GaugeType,
		Timestamp: 12,
	}}))

	series := buf.Series()
	require.Len(t, series, 2)
	require.Equal(t, "dog.metric", series[0].Name)
	require.Equal(t, []metrics.Point{{Ts: 11, Value: 3}}, series[0].Points)
	require.Equal(t, "check.metric", series[1].Name)
	stats := buf.Stats()
	require.Equal(t, 2, stats.Records)
	require.Equal(t, 2, stats.ActiveContexts)
}

func TestSeriesBetweenFiltersByTimestamp(t *testing.T) {
	buf := New(Options{Capacity: 8, ShardCount: 1})
	err := buf.AppendSamples(context.Background(), Source{Kind: SourceCheckShadow, ID: "check:1"}, []metrics.MetricSample{
		{Name: "metric.1", Value: 1, Mtype: metrics.GaugeType, Timestamp: 10},
		{Name: "metric.2", Value: 2, Mtype: metrics.GaugeType, Timestamp: 20},
		{Name: "metric.3", Value: 3, Mtype: metrics.GaugeType, Timestamp: 30},
	})
	require.NoError(t, err)

	series := buf.SeriesBetween(time.Unix(15, 0), time.Unix(25, 0))
	require.Len(t, series, 1)
	require.Equal(t, "metric.2", series[0].Name)
}

func TestPointsBetweenFiltersBySourceNameAndTimestamp(t *testing.T) {
	buf := New(Options{Capacity: 8, ShardCount: 1})
	dogstatsdSource := Source{Kind: SourceDogStatsDNoAggregation}
	checkSource := Source{Kind: SourceCheckShadow, ID: "check:1"}
	require.NoError(t, buf.AppendSamples(context.Background(), dogstatsdSource, []metrics.MetricSample{
		{Name: "metric", Value: 1, Mtype: metrics.GaugeType, Timestamp: 10},
		{Name: "metric", Value: 2, Mtype: metrics.GaugeType, Timestamp: 20},
		{Name: "other", Value: 3, Mtype: metrics.GaugeType, Timestamp: 20},
	}))
	require.NoError(t, buf.AppendSamples(context.Background(), checkSource, []metrics.MetricSample{
		{Name: "metric", Value: 4, Mtype: metrics.GaugeType, Timestamp: 20},
	}))

	points := buf.PointsBetween(dogstatsdSource, "metric", time.Unix(15, 0), time.Unix(25, 0))
	require.Equal(t, []Point{{Ts: time.Unix(20, 0), Value: 2}}, points)
}

func TestPointsBetweenReturnsRetainedPointTags(t *testing.T) {
	buf := New(Options{Capacity: 8, ShardCount: 1})
	source := Source{Kind: SourceCheckShadow, ID: "check:1"}
	require.NoError(t, buf.AppendSamples(context.Background(), source, []metrics.MetricSample{
		{Name: "metric", Value: 1, Mtype: metrics.GaugeType, Timestamp: 10, Tags: []string{"env:prod", "az:a"}},
	}))

	points := buf.PointsBetween(source, "metric", time.Time{}, time.Time{})
	require.Equal(t, []Point{{Ts: time.Unix(10, 0), Value: 1, Tags: []string{"az:a", "env:prod"}}}, points)
}

func TestPointsBetweenSourcesReadsMultipleDogStatsDSources(t *testing.T) {
	buf := New(Options{Capacity: 8, ShardCount: 1})
	bucketedSource := Source{Kind: SourceDogStatsDBucketed}
	noAggSource := Source{Kind: SourceDogStatsDNoAggregation}
	checkSource := Source{Kind: SourceCheckShadow, ID: "check:1"}
	require.NoError(t, buf.AppendSamples(context.Background(), bucketedSource, []metrics.MetricSample{
		{Name: "metric", Value: 1, Mtype: metrics.GaugeType, Timestamp: 10},
	}))
	require.NoError(t, buf.AppendSamples(context.Background(), noAggSource, []metrics.MetricSample{
		{Name: "metric", Value: 2, Mtype: metrics.GaugeType, Timestamp: 11},
	}))
	require.NoError(t, buf.AppendSamples(context.Background(), checkSource, []metrics.MetricSample{
		{Name: "metric", Value: 3, Mtype: metrics.GaugeType, Timestamp: 12},
	}))

	points := buf.PointsBetweenSources([]Source{bucketedSource, noAggSource}, "metric", time.Time{}, time.Time{})
	require.Equal(t, []Point{
		{Ts: time.Unix(10, 0), Value: 1},
		{Ts: time.Unix(11, 0), Value: 2},
	}, points)
}

func TestPointsBetweenSourcesEmptyIDMatchesAllSourcesWithKind(t *testing.T) {
	buf := New(Options{Capacity: 8, ShardCount: 1})
	require.NoError(t, buf.AppendSamples(context.Background(), Source{Kind: SourceCheckShadow, ID: "check:1"}, []metrics.MetricSample{
		{Name: "metric", Value: 1, Mtype: metrics.GaugeType, Timestamp: 10},
	}))
	require.NoError(t, buf.AppendSamples(context.Background(), Source{Kind: SourceCheckShadow, ID: "check:2"}, []metrics.MetricSample{
		{Name: "metric", Value: 2, Mtype: metrics.GaugeType, Timestamp: 11},
	}))
	require.NoError(t, buf.AppendSamples(context.Background(), Source{Kind: SourceDogStatsDNoAggregation}, []metrics.MetricSample{
		{Name: "metric", Value: 3, Mtype: metrics.GaugeType, Timestamp: 12},
	}))

	points := buf.PointsBetweenSources([]Source{{Kind: SourceCheckShadow}}, "metric", time.Time{}, time.Time{})
	require.Equal(t, []Point{
		{Ts: time.Unix(10, 0), Value: 1},
		{Ts: time.Unix(11, 0), Value: 2},
	}, points)
}

func TestAppendSamplesUsesNowForUntimestampedSamples(t *testing.T) {
	now := time.Unix(123, 456000)
	buf := New(Options{Capacity: 4, ShardCount: 1, Now: func() time.Time { return now }})

	err := buf.AppendSamples(context.Background(), Source{Kind: SourceCheckShadow, ID: "check:1"}, []metrics.MetricSample{{
		Name:  "metric",
		Value: 1,
		Mtype: metrics.GaugeType,
	}})
	require.NoError(t, err)

	series := buf.Series()
	require.Len(t, series, 1)
	require.Equal(t, float64(now.UnixMicro())/1e6, series[0].Points[0].Ts)
}

func TestOverwriteReleasesContexts(t *testing.T) {
	buf := New(Options{Capacity: 2, ShardCount: 1})
	for i := 0; i < 3; i++ {
		err := buf.AppendSamples(context.Background(), Source{Kind: SourceCheckShadow, ID: "check:1"}, []metrics.MetricSample{{
			Name:      "metric",
			Value:     float64(i),
			Mtype:     metrics.GaugeType,
			Timestamp: float64(i),
			Tags:      []string{string(rune('a' + i))},
		}})
		require.NoError(t, err)
	}

	stats := buf.Stats()
	require.Equal(t, 2, stats.Records)
	require.Equal(t, 2, stats.ActiveContexts)
	require.Equal(t, uint64(3), stats.AppendedSamples)
	require.Equal(t, uint64(1), stats.OverwrittenSamples)

	series := buf.Series()
	require.Len(t, series, 2)
	require.Equal(t, float64(1), series[0].Points[0].Value)
	require.Equal(t, float64(2), series[1].Points[0].Value)
}

func TestAppendSamplesReturnsContextCancellation(t *testing.T) {
	buf := New(Options{Capacity: 4, ShardCount: 1})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := buf.AppendSamples(ctx, Source{Kind: SourceCheckShadow, ID: "check:1"}, []metrics.MetricSample{{
		Name:  "metric",
		Value: 1,
		Mtype: metrics.GaugeType,
	}})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 0, buf.Stats().Records)
}
