// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metriclookback"
	"github.com/DataDog/datadog-agent/pkg/metriclookback/monitor"
	"github.com/DataDog/datadog-agent/pkg/metriclookback/ringbuffer"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestDogStatsDAdapterDoesNotAppendUntilFirstScalarMatch(t *testing.T) {
	retention := metriclookback.NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	adapter := NewDogStatsDAdapter(retention, DogStatsDOptions{MetricNames: []string{"target.metric"}})
	require.NotNil(t, adapter)

	adapter.AppendDogStatsDNoAggSerie(noAggSerie("other.metric", 1, 10, nil))
	require.Zero(t, retention.Stats().Records)

	adapter.AppendDogStatsDNoAggSerie(noAggSerie("target.metric", 2, 11, nil))
	require.Equal(t, 1, retention.Stats().Records)
}

func TestDogStatsDAdapterAdmitsOnlySelectedNames(t *testing.T) {
	retention := metriclookback.NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
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
	retention := metriclookback.NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
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
	retention := metriclookback.NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	watcher := newNoopWatcher(t, "monitor.metric")
	adapter := NewDogStatsDAdapter(retention, DogStatsDOptions{MetricNames: []string{"stored.metric"}, Monitor: watcher})
	require.NotNil(t, adapter)

	adapter.AppendDogStatsDNoAggSerie(noAggSerie("other.metric", 100, 11, nil))

	require.Empty(t, retention.Series())
	require.Equal(t, uint64(0), watcher.Breaches())
}

func TestDogStatsDAdapterIgnoresNilAndEmptySeries(t *testing.T) {
	retention := metriclookback.NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
	adapter := NewDogStatsDAdapter(retention, DogStatsDOptions{MetricNames: []string{"target.metric"}})
	require.NotNil(t, adapter)

	adapter.AppendDogStatsDNoAggSerie(nil)
	adapter.AppendDogStatsDNoAggSerie(&metrics.Serie{Name: "target.metric"})

	require.Empty(t, retention.Series())
}

func TestDogStatsDAdapterRoutesSelectedNormalSamplesToBucketMaterializer(t *testing.T) {
	retention := metriclookback.NewRetention(ringbuffer.Options{Capacity: 8, ShardCount: 1})
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
	watcher, err := monitor.New(monitor.Config{MetricName: metricName}, monitor.PointReaderFunc(func(_ string, _, _ time.Time) []monitor.Point {
		return nil
	}), monitor.DecisionSinkFunc(func(monitor.Decision) {
		require.FailNow(t, "decision should not be emitted")
	}))
	require.NoError(t, err)
	return watcher
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
