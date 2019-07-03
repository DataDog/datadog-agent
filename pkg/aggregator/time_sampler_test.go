// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package aggregator

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/quantile"
)

type OrderedSeries struct {
	series []*metrics.Serie
}

func (os OrderedSeries) Len() int {
	return len(os.series)
}

func (os OrderedSeries) Less(i, j int) bool {
	return ckey.Compare(os.series[i].ContextKey, os.series[j].ContextKey) == -1
}

func (os OrderedSeries) Swap(i, j int) {
	os.series[j], os.series[i] = os.series[i], os.series[j]
}

// TimeSampler
func TestCalculateBucketStart(t *testing.T) {
	sampler := NewTimeSampler(10)

	assert.Equal(t, int64(123450), sampler.calculateBucketStart(123456.5))
	assert.Equal(t, int64(123460), sampler.calculateBucketStart(123460.5))
}

func TestBucketSampling(t *testing.T) {
	sampler := NewTimeSampler(10)

	mSample := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}
	sampler.addSample(&mSample, 12345.0)
	sampler.addSample(&mSample, 12355.0)
	sampler.addSample(&mSample, 12365.0)

	series, _ := sampler.flush(12360.0)

	expectedSerie := &metrics.Serie{
		Name:       "my.metric.name",
		Tags:       []string{"foo", "bar"},
		Points:     []metrics.Point{{Ts: 12340.0, Value: mSample.Value}, {Ts: 12350.0, Value: mSample.Value}},
		MType:      metrics.APIGaugeType,
		Interval:   10,
		NameSuffix: "",
	}

	assert.Equal(t, 1, len(sampler.metricsByTimestamp))
	if assert.Equal(t, 1, len(series)) {
		metrics.AssertSerieEqual(t, expectedSerie, series[0])
	}
}

func TestContextSampling(t *testing.T) {
	sampler := NewTimeSampler(10)

	mSample1 := metrics.MetricSample{
		Name:       "my.metric.name1",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}
	mSample2 := metrics.MetricSample{
		Name:       "my.metric.name2",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}
	mSample3 := metrics.MetricSample{
		Name:       "my.metric.name3",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		Host:       "metric-hostname",
		SampleRate: 1,
	}

	sampler.addSample(&mSample1, 12346.0)
	sampler.addSample(&mSample2, 12346.0)
	sampler.addSample(&mSample3, 12346.0)

	series, _ := sampler.flush(12360.0)
	orderedSeries := OrderedSeries{series}
	sort.Sort(orderedSeries)

	series = orderedSeries.series

	expectedSerie1 := &metrics.Serie{
		Name:     "my.metric.name1",
		Points:   []metrics.Point{{Ts: 12340.0, Value: float64(1)}},
		Tags:     []string{"bar", "foo"},
		Host:     "",
		MType:    metrics.APIGaugeType,
		Interval: 10,
	}
	expectedSerie2 := &metrics.Serie{
		Name:     "my.metric.name3",
		Points:   []metrics.Point{{Ts: 12340.0, Value: float64(1)}},
		Tags:     []string{"bar", "foo"},
		Host:     "metric-hostname",
		MType:    metrics.APIGaugeType,
		Interval: 10,
	}
	expectedSerie3 := &metrics.Serie{
		Name:     "my.metric.name2",
		Points:   []metrics.Point{{Ts: 12340.0, Value: float64(1)}},
		Tags:     []string{"bar", "foo"},
		Host:     "",
		MType:    metrics.APIGaugeType,
		Interval: 10,
	}

	require.Equal(t, 3, len(series))
	metrics.AssertSerieEqual(t, expectedSerie1, series[0])
	metrics.AssertSerieEqual(t, expectedSerie2, series[1])
	metrics.AssertSerieEqual(t, expectedSerie3, series[2])
}

func TestCounterExpirySeconds(t *testing.T) {
	sampler := NewTimeSampler(10)

	sampleCounter1 := &metrics.MetricSample{
		Name:       "my.counter1",
		Value:      1,
		Mtype:      metrics.CounterType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}
	contextCounter1 := generateContextKey(sampleCounter1)

	sampleCounter2 := &metrics.MetricSample{
		Name:       "my.counter2",
		Value:      2,
		Mtype:      metrics.CounterType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}
	contextCounter2 := generateContextKey(sampleCounter2)

	sampleGauge3 := &metrics.MetricSample{
		Name:       "my.gauge",
		Value:      2,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}

	sampler.addSample(sampleCounter1, 1004.0)
	sampler.addSample(sampleCounter2, 1002.0)
	sampler.addSample(sampleGauge3, 1003.0)
	// counterLastSampledByContext should be populated when a sample is added
	assert.Equal(t, 2, len(sampler.counterLastSampledByContext))

	series, _ := sampler.flush(1010.0)
	orderedSeries := OrderedSeries{series}

	sort.Sort(orderedSeries)

	series = orderedSeries.series

	expectedSerie1 := &metrics.Serie{
		Name:     "my.counter1",
		Points:   []metrics.Point{{Ts: 1000.0, Value: .1}},
		Tags:     []string{"bar", "foo"},
		Host:     "",
		MType:    metrics.APIRateType,
		Interval: 10,
	}

	expectedSerie2 := &metrics.Serie{
		Name:     "my.counter2",
		Points:   []metrics.Point{{Ts: 1000.0, Value: .2}},
		Tags:     []string{"bar", "foo"},
		Host:     "",
		MType:    metrics.APIRateType,
		Interval: 10,
	}

	require.Equal(t, 3, len(series))
	require.Equal(t, 2, len(sampler.counterLastSampledByContext))
	metrics.AssertSerieEqual(t, expectedSerie1, series[0])
	metrics.AssertSerieEqual(t, expectedSerie2, series[1])
	assert.Equal(t, 1004.0, sampler.counterLastSampledByContext[contextCounter1])
	assert.Equal(t, 1002.0, sampler.counterLastSampledByContext[contextCounter2])

	sampleCounter1 = &metrics.MetricSample{
		Name:       "my.counter1",
		Value:      1,
		Mtype:      metrics.CounterType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}

	sampler.addSample(sampleCounter2, 1034.0)
	sampler.addSample(sampleCounter1, 1010.0)
	sampler.addSample(sampleCounter2, 1020.0)

	series, _ = sampler.flush(1040.0)
	orderedSeries = OrderedSeries{series}
	sort.Sort(orderedSeries)

	series = orderedSeries.series

	expectedSerie1 = &metrics.Serie{
		Name:     "my.counter1",
		Points:   []metrics.Point{{Ts: 1010.0, Value: .1}, {Ts: 1020.0, Value: 0.0}, {Ts: 1030.0, Value: 0.0}},
		Tags:     []string{"bar", "foo"},
		Host:     "",
		MType:    metrics.APIRateType,
		Interval: 10,
	}

	expectedSerie2 = &metrics.Serie{
		Name:     "my.counter2",
		Points:   []metrics.Point{{Ts: 1010, Value: 0}, {Ts: 1020.0, Value: .2}, {Ts: 1030.0, Value: .2}},
		Tags:     []string{"bar", "foo"},
		Host:     "",
		MType:    metrics.APIRateType,
		Interval: 10,
	}

	require.Equal(t, 2, len(series))
	metrics.AssertSerieEqual(t, expectedSerie1, series[0])
	metrics.AssertSerieEqual(t, expectedSerie2, series[1])

	// We shouldn't get any empty counter since the last flushSeries was during the same interval
	series, _ = sampler.flush(1045.0)
	assert.Equal(t, 0, len(series))

	// Now we should get the empty counters
	series, _ = sampler.flush(1050.0)
	assert.Equal(t, 2, len(series))

	series, _ = sampler.flush(1329.0)
	// Counter1 should have stopped reporting but the context is not expired yet
	// Counter2 should still report
	assert.Equal(t, 1, len(series))
	assert.Equal(t, 1, len(sampler.counterLastSampledByContext))
	assert.Equal(t, 2, len(sampler.contextResolver.contextsByKey))

	series, _ = sampler.flush(1800.0)
	// Everything stopped reporting and is expired
	assert.Equal(t, 0, len(series))
	assert.Equal(t, 0, len(sampler.counterLastSampledByContext))
	assert.Equal(t, 0, len(sampler.contextResolver.contextsByKey))
}

func TestSketch(t *testing.T) {
	const (
		defaultBucketSize = 10
	)

	var (
		sampler = NewTimeSampler(0)

		insert = func(t *testing.T, ts float64, ctx Context, values ...float64) {
			t.Helper()
			for _, v := range values {
				sampler.addSample(&metrics.MetricSample{
					Name:       ctx.Name,
					Tags:       ctx.Tags,
					Host:       ctx.Host,
					Value:      v,
					Mtype:      metrics.DistributionType,
					SampleRate: 1,
				}, ts)
			}
		}
	)

	assert.EqualValues(t, defaultBucketSize, sampler.interval,
		"interval should default to 10")

	t.Run("empty flush", func(t *testing.T) {
		_, flushed := sampler.flush(timeNowNano())
		require.Len(t, flushed, 0)
	})

	t.Run("single bucket", func(t *testing.T) {
		var (
			now float64
			ctx = Context{Name: "m.0", Tags: []string{"a"}, Host: "host"}
			exp = &quantile.Sketch{}
		)

		for i := 0; i < bucketSize; i++ {
			v := float64(i)
			insert(t, now, ctx, v)
			exp.Insert(quantile.Default(), v)

			now++
		}

		_, flushed := sampler.flush(now)
		metrics.AssertSketchSeriesEqual(t, metrics.SketchSeries{
			Name:     ctx.Name,
			Tags:     ctx.Tags,
			Host:     ctx.Host,
			Interval: 10,
			Points: []metrics.SketchPoint{
				{
					Sketch: exp,
					Ts:     0,
				},
			},
			ContextKey: ckey.Generate(ctx.Name, ctx.Host, ctx.Tags),
		}, flushed[0])

		_, flushed = sampler.flush(now)
		require.Len(t, flushed, 0, "these points have already been flushed")
	})

}

func TestSketchBucketSampling(t *testing.T) {

	sampler := NewTimeSampler(10)

	mSample1 := metrics.MetricSample{
		Name:       "test.metric.name",
		Value:      1,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"a", "b"},
		SampleRate: 1,
	}
	mSample2 := metrics.MetricSample{
		Name:       "test.metric.name",
		Value:      2,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"a", "b"},
		SampleRate: 1,
	}
	sampler.addSample(&mSample1, 10001)
	sampler.addSample(&mSample2, 10002)
	sampler.addSample(&mSample1, 10011)
	sampler.addSample(&mSample2, 10012)
	sampler.addSample(&mSample1, 10021)

	_, flushed := sampler.flush(10020.0)
	expSketch := &quantile.Sketch{}
	expSketch.Insert(quantile.Default(), 1, 2)

	assert.Equal(t, 1, len(flushed))
	metrics.AssertSketchSeriesEqual(t, metrics.SketchSeries{
		Name:     "test.metric.name",
		Tags:     []string{"a", "b"},
		Interval: 10,
		Points: []metrics.SketchPoint{
			{Ts: 10000, Sketch: expSketch},
			{Ts: 10010, Sketch: expSketch},
		},
		ContextKey: generateContextKey(&mSample1),
	}, flushed[0])

	// The samples added after the flush time remains in the dist sampler
	assert.Equal(t, 1, sampler.sketchMap.Len())
}

func TestSketchContextSampling(t *testing.T) {
	sampler := NewTimeSampler(10)

	mSample1 := metrics.MetricSample{
		Name:       "test.metric.name1",
		Value:      1,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"a", "b"},
		SampleRate: 1,
	}
	mSample2 := metrics.MetricSample{
		Name:       "test.metric.name2",
		Value:      1,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"a", "c"},
		SampleRate: 1,
	}
	sampler.addSample(&mSample1, 10011)
	sampler.addSample(&mSample2, 10011)

	_, flushed := sampler.flush(10020)
	expSketch := &quantile.Sketch{}
	expSketch.Insert(quantile.Default(), 1)

	assert.Equal(t, 2, len(flushed))
	sort.Slice(flushed, func(i, j int) bool {
		return flushed[i].Name < flushed[j].Name
	})

	metrics.AssertSketchSeriesEqual(t, metrics.SketchSeries{
		Name:     "test.metric.name1",
		Tags:     []string{"a", "b"},
		Interval: 10,
		Points: []metrics.SketchPoint{
			{Ts: 10010, Sketch: expSketch},
		},
		ContextKey: generateContextKey(&mSample1),
	}, flushed[0])

	metrics.AssertSketchSeriesEqual(t, metrics.SketchSeries{
		Name:     "test.metric.name2",
		Tags:     []string{"a", "c"},
		Interval: 10,
		Points: []metrics.SketchPoint{
			{Ts: 10010, Sketch: expSketch},
		},
		ContextKey: generateContextKey(&mSample2),
	}, flushed[1])
}

func TestBucketSamplingWithSketchAndSeries(t *testing.T) {
	sampler := NewTimeSampler(10)

	dSample1 := metrics.MetricSample{
		Name:       "distribution.metric.name1",
		Value:      1,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"a", "b"},
		SampleRate: 1,
	}
	sampler.addSample(&dSample1, 12345.0)
	sampler.addSample(&dSample1, 12355.0)
	sampler.addSample(&dSample1, 12365.0)

	mSample := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}
	sampler.addSample(&mSample, 12345.0)
	sampler.addSample(&mSample, 12355.0)
	sampler.addSample(&mSample, 12365.0)

	series, sketches := sampler.flush(12360.0)

	expectedSerie := &metrics.Serie{
		Name:       "my.metric.name",
		Tags:       []string{"foo", "bar"},
		Points:     []metrics.Point{{Ts: 12340.0, Value: mSample.Value}, {Ts: 12350.0, Value: mSample.Value}},
		MType:      metrics.APIGaugeType,
		Interval:   10,
		NameSuffix: "",
	}

	assert.Equal(t, 1, len(sampler.metricsByTimestamp))
	if assert.Equal(t, 1, len(series)) {
		metrics.AssertSerieEqual(t, expectedSerie, series[0])
	}

	expSketch := &quantile.Sketch{}
	expSketch.Insert(quantile.Default(), 1)

	metrics.AssertSketchSeriesEqual(t, metrics.SketchSeries{
		Name:     "distribution.metric.name1",
		Tags:     []string{"a", "b"},
		Interval: 10,
		Points: []metrics.SketchPoint{
			{Ts: 12340.0, Sketch: expSketch},
			{Ts: 12350.0, Sketch: expSketch},
		},
		ContextKey: generateContextKey(&dSample1),
	}, sketches[0])
}
