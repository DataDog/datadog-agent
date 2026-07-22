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
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metriclookback"
	"github.com/DataDog/datadog-agent/pkg/metriclookback/monitor"
	"github.com/DataDog/datadog-agent/pkg/metriclookback/ringbuffer"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestDogStatsDBucketMaterializerSealsGaugeWithResolvedContext(t *testing.T) {
	retention := metriclookback.NewRetention(ringbuffer.Options{Capacity: 16, ShardCount: 1})
	materializer := NewDogStatsDBucketMaterializer(retention, DogStatsDBucketMaterializerOptions{
		BucketWidth: time.Second,
		SealDelay:   -1,
		ShardCount:  1,
	})

	ctx := aggregator.DogStatsDLookbackContext{
		ContextKey: ckey.ContextKey(42),
		Name:       "mapped.metric",
		Host:       "host-a",
		Tags:       []string{"tagger:tag", "metric:tag"},
		NoIndex:    true,
		Source:     metrics.MetricSource(7),
	}
	materializer.Observe(&metrics.MetricSample{Name: "mapped.metric", Value: 1, Mtype: metrics.GaugeType, SampleRate: 1}, 10.1, ctx)
	materializer.Observe(&metrics.MetricSample{Name: "mapped.metric", Value: 2, Mtype: metrics.GaugeType, SampleRate: 1}, 10.9, ctx)
	materializer.Flush(11)

	series := retention.Series()
	require.Len(t, series, 1)
	require.Equal(t, "mapped.metric", series[0].Name)
	require.Equal(t, "host-a", series[0].Host)
	require.Equal(t, []string{"tagger:tag", "metric:tag"}, series[0].Tags.UnsafeToReadOnlySliceString())
	require.Equal(t, metrics.APIGaugeType, series[0].MType)
	require.Equal(t, int64(1), series[0].Interval)
	require.True(t, series[0].NoIndex)
	require.Equal(t, metrics.MetricSource(7), series[0].Source)
	require.Equal(t, []metrics.Point{{Ts: 10, Value: 2}}, series[0].Points)
}

func TestDogStatsDBucketMaterializerSealsCounterUsingDogStatsDSemantics(t *testing.T) {
	retention := metriclookback.NewRetention(ringbuffer.Options{Capacity: 16, ShardCount: 1})
	materializer := NewDogStatsDBucketMaterializer(retention, DogStatsDBucketMaterializerOptions{
		BucketWidth:   time.Second,
		SealDelay:     -1,
		ShardCount:    1,
		CounterExpiry: time.Second,
	})
	ctx := dogstatsdBucketTestContext("counter.metric", 1)

	materializer.Observe(&metrics.MetricSample{Name: "counter.metric", Value: 2, Mtype: metrics.CounterType, SampleRate: 0.5}, 10.1, ctx)
	materializer.Observe(&metrics.MetricSample{Name: "counter.metric", Value: 3, Mtype: metrics.CounterType, SampleRate: 1}, 10.8, ctx)
	materializer.Flush(11)

	series := retention.Series()
	require.Len(t, series, 1)
	require.Equal(t, "counter.metric", series[0].Name)
	require.Equal(t, metrics.APIRateType, series[0].MType)
	require.Equal(t, int64(1), series[0].Interval)
	require.Equal(t, []metrics.Point{{Ts: 10, Value: 7}}, series[0].Points)
}

func TestDogStatsDBucketMaterializerSealsDistributionSketch(t *testing.T) {
	retention := metriclookback.NewRetention(ringbuffer.Options{Capacity: 16, ShardCount: 1})
	materializer := NewDogStatsDBucketMaterializer(retention, DogStatsDBucketMaterializerOptions{
		BucketWidth: time.Second,
		SealDelay:   -1,
		ShardCount:  1,
	})
	ctx := aggregator.DogStatsDLookbackContext{
		ContextKey: ckey.ContextKey(2),
		Name:       "dist.metric",
		Host:       "host-a",
		Tags:       []string{"env:test", "role:web"},
		NoIndex:    true,
		Source:     metrics.MetricSourceDogstatsd,
	}

	materializer.Observe(&metrics.MetricSample{Name: "dist.metric", Value: 1, Mtype: metrics.DistributionType, SampleRate: 1}, 10.1, ctx)
	materializer.Observe(&metrics.MetricSample{Name: "dist.metric", Value: 3, Mtype: metrics.DistributionType, SampleRate: 1}, 10.2, ctx)
	materializer.Observe(&metrics.MetricSample{Name: "dist.metric", Value: 5, Mtype: metrics.DistributionType, SampleRate: 0.5}, 10.3, ctx)
	materializer.Flush(11)

	require.Empty(t, retention.Series())
	sketches := retention.SketchSeries()
	require.Len(t, sketches, 1)
	require.Equal(t, "dist.metric", sketches[0].Name)
	require.Equal(t, "host-a", sketches[0].Host)
	require.Equal(t, []string{"env:test", "role:web"}, sketches[0].Tags.UnsafeToReadOnlySliceString())
	require.Equal(t, int64(1), sketches[0].Interval)
	require.True(t, sketches[0].NoIndex)
	require.Equal(t, metrics.MetricSourceDogstatsd, sketches[0].Source)
	require.Len(t, sketches[0].Points, 1)
	require.Equal(t, int64(10), sketches[0].Points[0].Ts)
	cnt, min, max, sum, avg := sketches[0].Points[0].Sketch.BasicStats()
	require.Equal(t, int64(4), cnt)
	require.Equal(t, float64(1), min)
	require.Equal(t, float64(5), max)
	require.Equal(t, float64(14), sum)
	require.Equal(t, 3.5, avg)

	points := retention.ProjectedSketchPointsBetweenSources(
		[]ringbuffer.Source{{Kind: ringbuffer.SourceDogStatsDBucketed}},
		"dist.metric",
		time.Unix(9, 0),
		time.Unix(11, 0),
		metriclookback.PlaceholderAverageSketchProjection{},
	)
	require.Equal(t, []ringbuffer.Point{{Ts: time.Unix(10, 0), Value: 3.5, Tags: []string{"env:test", "role:web"}}}, points)
}

func TestDogStatsDBucketMaterializerFlushAllFlushesDistributionOnlyBuckets(t *testing.T) {
	retention := metriclookback.NewRetention(ringbuffer.Options{Capacity: 16, ShardCount: 1})
	materializer := NewDogStatsDBucketMaterializer(retention, DogStatsDBucketMaterializerOptions{
		BucketWidth: time.Second,
		SealDelay:   30 * time.Second,
		ShardCount:  1,
	})
	ctx := dogstatsdBucketTestContext("dist.metric", 1)

	materializer.Observe(&metrics.MetricSample{Name: "dist.metric", Value: 1, Mtype: metrics.DistributionType, SampleRate: 1}, 10.1, ctx)
	materializer.Flush(11)
	require.Empty(t, retention.SketchSeries())

	materializer.FlushAll(11)
	sketches := retention.SketchSeries()
	require.Len(t, sketches, 1)
	require.Equal(t, int64(10), sketches[0].Points[0].Ts)
}

func TestDogStatsDBucketMaterializerZeroFillsActiveCounters(t *testing.T) {
	retention := metriclookback.NewRetention(ringbuffer.Options{Capacity: 16, ShardCount: 1})
	materializer := NewDogStatsDBucketMaterializer(retention, DogStatsDBucketMaterializerOptions{
		BucketWidth:   time.Second,
		SealDelay:     -1,
		ShardCount:    1,
		CounterExpiry: 2 * time.Second,
	})
	ctx := dogstatsdBucketTestContext("counter.metric", 1)

	materializer.Observe(&metrics.MetricSample{Name: "counter.metric", Value: 5, Mtype: metrics.CounterType, SampleRate: 1}, 10.1, ctx)
	materializer.Flush(14)

	series := retention.Series()
	require.Len(t, series, 2)
	require.Equal(t, []metrics.Point{{Ts: 10, Value: 5}}, series[0].Points)
	require.Equal(t, []metrics.Point{{Ts: 11, Value: 0}}, series[1].Points)
}

func TestDogStatsDBucketMaterializerDropsUnsupportedAndLateSamples(t *testing.T) {
	retention := metriclookback.NewRetention(ringbuffer.Options{Capacity: 16, ShardCount: 1})
	materializer := NewDogStatsDBucketMaterializer(retention, DogStatsDBucketMaterializerOptions{
		BucketWidth: time.Second,
		SealDelay:   -1,
		ShardCount:  1,
	})
	ctx := dogstatsdBucketTestContext("metric", 1)

	materializer.Observe(&metrics.MetricSample{Name: "metric", Value: 100, Mtype: metrics.HistogramType, SampleRate: 1}, 10.1, ctx)
	materializer.Observe(&metrics.MetricSample{Name: "metric", Value: 1, Mtype: metrics.GaugeType, SampleRate: 1}, 10.1, ctx)
	materializer.Flush(11)
	materializer.Observe(&metrics.MetricSample{Name: "metric", Value: 2, Mtype: metrics.GaugeType, SampleRate: 1}, 10.5, ctx)
	materializer.Flush(12)

	series := retention.Series()
	require.Len(t, series, 1)
	require.Equal(t, []metrics.Point{{Ts: 10, Value: 1}}, series[0].Points)
}

func TestDogStatsDBucketMaterializerHonorsWiderBucketWidth(t *testing.T) {
	retention := metriclookback.NewRetention(ringbuffer.Options{Capacity: 16, ShardCount: 1})
	materializer := NewDogStatsDBucketMaterializer(retention, DogStatsDBucketMaterializerOptions{
		BucketWidth: 5 * time.Second,
		SealDelay:   -1,
		ShardCount:  1,
	})
	ctx := dogstatsdBucketTestContext("gauge.metric", 1)

	materializer.Observe(&metrics.MetricSample{Name: "gauge.metric", Value: 1, Mtype: metrics.GaugeType, SampleRate: 1}, 10.1, ctx)
	materializer.Observe(&metrics.MetricSample{Name: "gauge.metric", Value: 2, Mtype: metrics.GaugeType, SampleRate: 1}, 14.9, ctx)
	materializer.Flush(15)

	series := retention.Series()
	require.Len(t, series, 1)
	require.Equal(t, int64(5), series[0].Interval)
	require.Equal(t, []metrics.Point{{Ts: 10, Value: 2}}, series[0].Points)
}

func TestDogStatsDBucketMaterializerFlushAllIgnoresSealDelay(t *testing.T) {
	retention := metriclookback.NewRetention(ringbuffer.Options{Capacity: 16, ShardCount: 1})
	materializer := NewDogStatsDBucketMaterializer(retention, DogStatsDBucketMaterializerOptions{
		BucketWidth: time.Second,
		SealDelay:   30 * time.Second,
		ShardCount:  1,
	})
	ctx := dogstatsdBucketTestContext("gauge.metric", 1)

	materializer.Observe(&metrics.MetricSample{Name: "gauge.metric", Value: 1, Mtype: metrics.GaugeType, SampleRate: 1}, 10.1, ctx)
	materializer.Flush(11)
	require.Empty(t, retention.Series())

	materializer.FlushAll(11)
	series := retention.Series()
	require.Len(t, series, 1)
	require.Equal(t, []metrics.Point{{Ts: 10, Value: 1}}, series[0].Points)
}

func TestDogStatsDBucketMaterializerExpiresDescriptors(t *testing.T) {
	retention := metriclookback.NewRetention(ringbuffer.Options{Capacity: 16, ShardCount: 1})
	materializer := NewDogStatsDBucketMaterializer(retention, DogStatsDBucketMaterializerOptions{
		BucketWidth:   time.Second,
		SealDelay:     -1,
		ShardCount:    1,
		ContextExpiry: time.Second,
		CounterExpiry: time.Second,
	})
	ctx := dogstatsdBucketTestContext("gauge.metric", 1)

	materializer.Observe(&metrics.MetricSample{Name: "gauge.metric", Value: 1, Mtype: metrics.GaugeType, SampleRate: 1}, 10.1, ctx)
	materializer.Flush(14)

	require.Len(t, materializer.shards[0].descriptors, 0)
}

func TestDogStatsDBucketMaterializerMatchesContextMetricsForSupportedTypes(t *testing.T) {
	tests := []struct {
		name    string
		samples []metrics.MetricSample
	}{
		{
			name: "gauge last value",
			samples: []metrics.MetricSample{
				{Name: "metric", Value: 1, Mtype: metrics.GaugeType, SampleRate: 1},
				{Name: "metric", Value: 5, Mtype: metrics.GaugeType, SampleRate: 1},
				{Name: "metric", Value: 3, Mtype: metrics.GaugeType, SampleRate: 1},
			},
		},
		{
			name: "counter sample rate adjusted rate",
			samples: []metrics.MetricSample{
				{Name: "metric", Value: 2, Mtype: metrics.CounterType, SampleRate: 0.5},
				{Name: "metric", Value: 3, Mtype: metrics.CounterType, SampleRate: 1},
				{Name: "metric", Value: 4, Mtype: metrics.CounterType, SampleRate: 0.25},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contextKey := ckey.ContextKey(99)
			expectedMetrics := metrics.MakeContextMetrics()
			retention := metriclookback.NewRetention(ringbuffer.Options{Capacity: 16, ShardCount: 1})
			materializer := NewDogStatsDBucketMaterializer(retention, DogStatsDBucketMaterializerOptions{
				BucketWidth:   time.Second,
				SealDelay:     -1,
				ShardCount:    1,
				CounterExpiry: time.Second,
			})
			ctx := dogstatsdBucketTestContext("metric", contextKey)

			for i := range tt.samples {
				sample := tt.samples[i]
				timestamp := 10.1 + float64(i)/10
				require.NoError(t, expectedMetrics.AddSample(contextKey, &sample, timestamp, 1, nil, pkgconfigsetup.Datadog()))
				materializer.Observe(&sample, timestamp, ctx)
			}
			materializer.Flush(11)

			expected, errs := expectedMetrics.Flush(10)
			require.Empty(t, errs)
			require.Len(t, expected, 1)
			actual := retention.Series()
			require.Len(t, actual, 1)
			require.Equal(t, expected[0].MType, actual[0].MType)
			require.Equal(t, expected[0].Points, actual[0].Points)
		})
	}
}

func TestDogStatsDBucketMaterializerObservesMonitorAfterAppendingAllShards(t *testing.T) {
	retention := metriclookback.NewRetention(ringbuffer.Options{Capacity: 16, ShardCount: 2})
	reader := monitor.PointReaderFunc(func(metricName string, from, to time.Time) []monitor.Point {
		points := retention.PointsBetweenSources([]ringbuffer.Source{{Kind: ringbuffer.SourceDogStatsDBucketed}}, metricName, from, to)
		out := make([]monitor.Point, 0, len(points))
		for _, point := range points {
			out = append(out, monitor.Point{Ts: point.Ts, Value: point.Value, Tags: point.Tags})
		}
		return out
	})
	var decisions []monitor.Decision
	watcher, err := monitor.New(monitor.Config{
		MetricName:         "target.metric",
		RangeEpsilon:       5,
		EvaluationInterval: 2 * time.Second,
	}, reader, monitor.DecisionSinkFunc(func(decision monitor.Decision) {
		decisions = append(decisions, decision)
	}))
	require.NoError(t, err)

	materializer := NewDogStatsDBucketMaterializer(retention, DogStatsDBucketMaterializerOptions{
		BucketWidth: time.Second,
		SealDelay:   -1,
		ShardCount:  2,
		Monitor:     watcher,
	})

	ctxShard0 := dogstatsdBucketTestContext("target.metric", 2)
	ctxShard1 := dogstatsdBucketTestContext("target.metric", 3)
	materializer.Observe(&metrics.MetricSample{Name: "target.metric", Value: 1, Mtype: metrics.GaugeType, SampleRate: 1}, 10.1, ctxShard0)
	materializer.Observe(&metrics.MetricSample{Name: "target.metric", Value: 10, Mtype: metrics.GaugeType, SampleRate: 1}, 10.1, ctxShard1)
	materializer.Observe(&metrics.MetricSample{Name: "target.metric", Value: 1, Mtype: metrics.GaugeType, SampleRate: 1}, 12.1, ctxShard0)
	materializer.Observe(&metrics.MetricSample{Name: "target.metric", Value: 10, Mtype: metrics.GaugeType, SampleRate: 1}, 12.1, ctxShard1)

	materializer.Flush(13)

	require.NotEmpty(t, decisions)
	require.Equal(t, monitor.Breach, decisions[0].State)
	require.Equal(t, 4, decisions[0].PointCount)
	require.Equal(t, float64(9), decisions[0].Range)
}

func dogstatsdBucketTestContext(name string, key ckey.ContextKey) aggregator.DogStatsDLookbackContext {
	return aggregator.DogStatsDLookbackContext{
		ContextKey: key,
		Name:       name,
		Host:       "host",
		Tags:       []string{"env:test"},
	}
}
