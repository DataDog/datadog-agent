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

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/monitor"
	"github.com/DataDog/datadog-agent/pkg/collector/metriclookback/ringbuffer"
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

func TestRetentionAppendSketchSeriesAndForwardRange(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	err := retention.AppendSketchSeries(context.Background(), ringbuffer.Source{Kind: ringbuffer.SourceDogStatsDBucketed}, &metrics.SketchSeries{
		Name:    "dist.metric",
		Tags:    tagset.CompositeTagsFromSlice([]string{"env:test"}),
		Host:    "host",
		NoIndex: true,
		Source:  metrics.MetricSourceDogstatsd,
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
		series := source.Current()
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

func TestRetentionProjectsSketchPointsWithPlaceholderAverage(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	err := retention.AppendSketchSeries(context.Background(), ringbuffer.Source{Kind: ringbuffer.SourceDogStatsDBucketed}, &metrics.SketchSeries{
		Name: "dist.metric",
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

func TestDogStatsDAdapterAdmitsOnlySelectedNames(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	adapter := NewDogStatsDAdapter(retention, DogStatsDOptions{MetricNames: []string{"target.metric"}})
	require.NotNil(t, adapter)

	adapter.AppendDogStatsDNoAggSerie(noAggSerie("other.metric", 1, 10, nil))
	adapter.AppendDogStatsDNoAggSerie(noAggSerie("target.metric", 2, 11, []string{"client:tag", "origin:tag"}))

	series := retention.Series()
	require.Len(t, series, 1)
	require.Equal(t, "target.metric", series[0].Name)
	require.Equal(t, float64(2), series[0].Points[0].Value)
	require.Equal(t, int64(10), series[0].Interval)
	require.Equal(t, metrics.APIGaugeType, series[0].MType)
	require.Equal(t, []string{"client:tag", "origin:tag"}, series[0].Tags.UnsafeToReadOnlySliceString())

	stats := retention.Stats()
	require.Equal(t, 1, stats.Records)
	require.Equal(t, uint64(1), stats.AppendedSamples)
}

func TestDogStatsDAdapterUsesMonitorMetricAsAdmissionName(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	watcher := newNoopWatcher(t, "monitor.metric")
	adapter := NewDogStatsDAdapter(retention, DogStatsDOptions{Monitor: watcher})
	require.NotNil(t, adapter)

	adapter.AppendDogStatsDNoAggSerie(noAggSerie("monitor.metric", 11, 11, nil))

	series := retention.Series()
	require.Len(t, series, 1)
	require.Equal(t, "monitor.metric", series[0].Name)
	require.Equal(t, uint64(0), watcher.Breaches())
}

func TestDogStatsDAdapterDoesNotMonitorNonAdmittedSamples(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	watcher := newNoopWatcher(t, "monitor.metric")
	adapter := NewDogStatsDAdapter(retention, DogStatsDOptions{MetricNames: []string{"stored.metric"}, Monitor: watcher})
	require.NotNil(t, adapter)

	adapter.AppendDogStatsDNoAggSerie(noAggSerie("other.metric", 100, 11, nil))

	require.Empty(t, retention.Series())
	require.Equal(t, uint64(0), watcher.Breaches())
}

func TestDogStatsDAdapterIgnoresNilAndEmptySeries(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	adapter := NewDogStatsDAdapter(retention, DogStatsDOptions{MetricNames: []string{"target.metric"}})
	require.NotNil(t, adapter)

	adapter.AppendDogStatsDNoAggSerie(nil)
	adapter.AppendDogStatsDNoAggSerie(&metrics.Serie{Name: "target.metric"})

	require.Empty(t, retention.Series())
}

func TestDogStatsDAdapterRoutesSelectedNormalSamplesToBucketMaterializer(t *testing.T) {
	retention := NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	materializer := NewDogStatsDBucketMaterializer(retention, DogStatsDBucketMaterializerOptions{SealDelay: -1, ShardCount: 1})
	adapter := NewDogStatsDAdapter(retention, DogStatsDOptions{
		MetricNames:        []string{"target.metric"},
		BucketMaterializer: materializer,
	})
	require.NotNil(t, adapter)
	require.True(t, adapter.WantsDogStatsDMetric("target.metric"))
	require.False(t, adapter.WantsDogStatsDMetric("other.metric"))

	adapter.ObserveDogStatsDSample(&metrics.MetricSample{Name: "other.metric", Value: 10, Mtype: metrics.GaugeType, SampleRate: 1}, 10.1, aggregator.DogStatsDLookbackContext{ContextKey: ckey.ContextKey(1), Name: "other.metric"})
	adapter.ObserveDogStatsDSample(&metrics.MetricSample{Name: "target.metric", Value: 2, Mtype: metrics.GaugeType, SampleRate: 1}, 10.1, aggregator.DogStatsDLookbackContext{
		ContextKey: ckey.ContextKey(2),
		Name:       "target.metric",
		Host:       "host",
		Tags:       []string{"env:test"},
	})
	adapter.FlushDogStatsDBuckets(11, false)

	series := retention.Series()
	require.Len(t, series, 1)
	require.Equal(t, "target.metric", series[0].Name)
	require.Equal(t, []metrics.Point{{Ts: 10, Value: 2}}, series[0].Points)
	require.Equal(t, []string{"env:test"}, series[0].Tags.UnsafeToReadOnlySliceString())
}

func newNoopWatcher(t *testing.T, metricName string) *monitor.Watcher {
	t.Helper()
	return monitor.New(monitor.Config{MetricName: metricName}, monitor.PointReaderFunc(func(_ string, _, _ time.Time) []monitor.Point {
		return nil
	}), monitor.DecisionSinkFunc(func(monitor.Decision) {
		require.FailNow(t, "decision should not be emitted")
	}))
}

func noAggSerie(name string, value float64, ts float64, tags []string) *metrics.Serie {
	return &metrics.Serie{
		Name:     name,
		Points:   []metrics.Point{{Ts: ts, Value: value}},
		Tags:     tagset.CompositeTagsFromSlice(tags),
		Host:     "host",
		MType:    metrics.APIGaugeType,
		Interval: 10,
	}
}

func testSketchData(values ...float64) metrics.SketchData {
	var agent quantile.Agent
	for _, value := range values {
		agent.Insert(value, 1)
	}
	return agent.Finish()
}
