// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"math"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/quantile"
)

func generateSerieContextKey(serie *metrics.Serie) ckey.ContextKey {
	l := ckey.NewKeyGenerator()
	var tags []string
	serie.Tags.ForEach(func(tag string) {
		tags = append(tags, tag)
	})
	return l.Generate(serie.Name, serie.Host, tagset.NewHashingTagsAccumulatorWithTags(tags))
}

func testTimeSampler() *TimeSampler {
	sampler := NewTimeSampler(TimeSamplerID(0), 10, tags.NewStore(false, "test"), "host")
	return sampler
}

// TimeSampler
func TestCalculateBucketStart(t *testing.T) {
	sampler := testTimeSampler()

	assert.Equal(t, int64(123450), sampler.calculateBucketStart(123456.5))
	assert.Equal(t, int64(123460), sampler.calculateBucketStart(123460.5))
}

func testBucketSampling(t *testing.T, store *tags.Store) {
	sampler := testTimeSampler()

	mSample := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}
	sampler.sample(&mSample, 12345.0)
	sampler.sample(&mSample, 12355.0)
	sampler.sample(&mSample, 12365.0)

	series, _ := flushSerie(sampler, 12360.0)

	expectedSerie := &metrics.Serie{
		Name:       "my.metric.name",
		Tags:       tagset.CompositeTagsFromSlice([]string{"foo", "bar"}),
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
func TestBucketSampling(t *testing.T) {
	testWithTagsStore(t, testBucketSampling)
}

func testContextSampling(t *testing.T, store *tags.Store) {
	sampler := testTimeSampler()

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

	sampler.sample(&mSample1, 12346.0)
	sampler.sample(&mSample2, 12346.0)
	sampler.sample(&mSample3, 12346.0)

	series, _ := flushSerie(sampler, 12360.0)

	expectedSerie1 := &metrics.Serie{
		Name:     "my.metric.name1",
		Points:   []metrics.Point{{Ts: 12340.0, Value: float64(1)}},
		Tags:     tagset.CompositeTagsFromSlice([]string{"bar", "foo"}),
		Host:     "",
		MType:    metrics.APIGaugeType,
		Interval: 10,
	}
	expectedSerie1.ContextKey = generateSerieContextKey(expectedSerie1)
	expectedSerie2 := &metrics.Serie{
		Name:     "my.metric.name3",
		Points:   []metrics.Point{{Ts: 12340.0, Value: float64(1)}},
		Tags:     tagset.CompositeTagsFromSlice([]string{"bar", "foo"}),
		Host:     "metric-hostname",
		MType:    metrics.APIGaugeType,
		Interval: 10,
	}
	expectedSerie2.ContextKey = generateSerieContextKey(expectedSerie2)
	expectedSerie3 := &metrics.Serie{
		Name:     "my.metric.name2",
		Points:   []metrics.Point{{Ts: 12340.0, Value: float64(1)}},
		Tags:     tagset.CompositeTagsFromSlice([]string{"bar", "foo"}),
		Host:     "",
		MType:    metrics.APIGaugeType,
		Interval: 10,
	}
	expectedSerie3.ContextKey = generateSerieContextKey(expectedSerie3)

	expectedSeries := metrics.Series{expectedSerie1, expectedSerie2, expectedSerie3}
	metrics.AssertSeriesEqual(t, expectedSeries, series)
}
func TestContextSampling(t *testing.T) {
	testWithTagsStore(t, testContextSampling)
}

func testCounterExpirySeconds(t *testing.T, store *tags.Store) {
	sampler := testTimeSampler()

	math.Abs(1)
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

	sampler.sample(sampleCounter1, 1004.0)
	sampler.sample(sampleCounter2, 1002.0)
	sampler.sample(sampleGauge3, 1003.0)
	// counterLastSampledByContext should be populated when a sample is added
	assert.Equal(t, 2, len(sampler.counterLastSampledByContext))

	series, _ := flushSerie(sampler, 1010.0)

	expectedSerie1 := &metrics.Serie{
		Name:       "my.counter1",
		Points:     []metrics.Point{{Ts: 1000.0, Value: .1}},
		Tags:       tagset.CompositeTagsFromSlice([]string{"bar", "foo"}),
		Host:       "",
		MType:      metrics.APIRateType,
		ContextKey: generateContextKey(sampleCounter1),
		Interval:   10,
	}

	expectedSerie2 := &metrics.Serie{
		Name:       "my.counter2",
		Points:     []metrics.Point{{Ts: 1000.0, Value: .2}},
		Tags:       tagset.CompositeTagsFromSlice([]string{"bar", "foo"}),
		Host:       "",
		MType:      metrics.APIRateType,
		ContextKey: generateContextKey(sampleCounter2),
		Interval:   10,
	}

	expectedSerie3 := &metrics.Serie{
		Name:       "my.gauge",
		Points:     []metrics.Point{{Ts: 1000.0, Value: 2}},
		Tags:       tagset.CompositeTagsFromSlice([]string{"bar", "foo"}),
		Host:       "",
		MType:      metrics.APIGaugeType,
		ContextKey: generateContextKey(sampleGauge3),
		Interval:   10,
	}
	expectedSeries := metrics.Series{expectedSerie1, expectedSerie2, expectedSerie3}

	require.Equal(t, 2, len(sampler.counterLastSampledByContext))
	metrics.AssertSeriesEqual(t, expectedSeries, series)
	assert.Equal(t, 1004.0, sampler.counterLastSampledByContext[contextCounter1])
	assert.Equal(t, 1002.0, sampler.counterLastSampledByContext[contextCounter2])

	sampleCounter1 = &metrics.MetricSample{
		Name:       "my.counter1",
		Value:      1,
		Mtype:      metrics.CounterType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}

	sampler.sample(sampleCounter2, 1034.0)
	sampler.sample(sampleCounter1, 1010.0)
	sampler.sample(sampleCounter2, 1020.0)

	series, _ = flushSerie(sampler, 1040.0)

	expectedSerie1 = &metrics.Serie{
		Name:       "my.counter1",
		Points:     []metrics.Point{{Ts: 1010.0, Value: .1}, {Ts: 1020.0, Value: 0.0}, {Ts: 1030.0, Value: 0.0}},
		Tags:       tagset.CompositeTagsFromSlice([]string{"bar", "foo"}),
		Host:       "",
		MType:      metrics.APIRateType,
		ContextKey: generateContextKey(sampleCounter1),
		Interval:   10,
	}

	expectedSerie2 = &metrics.Serie{
		Name:       "my.counter2",
		Points:     []metrics.Point{{Ts: 1010, Value: 0}, {Ts: 1020.0, Value: .2}, {Ts: 1030.0, Value: .2}},
		Tags:       tagset.CompositeTagsFromSlice([]string{"bar", "foo"}),
		Host:       "",
		MType:      metrics.APIRateType,
		ContextKey: generateContextKey(sampleCounter2),
		Interval:   10,
	}
	expectedSeries = metrics.Series{expectedSerie1, expectedSerie2}

	metrics.AssertSeriesEqual(t, expectedSeries, series)

	// We shouldn't get any empty counter since the last flushSeries was during the same interval
	series, _ = flushSerie(sampler, 1045.0)
	assert.Equal(t, 0, len(series))

	// Now we should get the empty counters
	series, _ = flushSerie(sampler, 1050.0)
	assert.Equal(t, 2, len(series))

	series, _ = flushSerie(sampler, 1329.0)
	// Counter1 should have stopped reporting but the context is not expired yet
	// Counter2 should still report
	assert.Equal(t, 1, len(series))
	assert.Equal(t, 1, len(sampler.counterLastSampledByContext))
	assert.Equal(t, 1, len(sampler.contextResolver.resolver.contextsByKey))

	series, _ = flushSerie(sampler, 1800.0)
	// Everything stopped reporting and is expired
	assert.Equal(t, 0, len(series))
	assert.Equal(t, 0, len(sampler.counterLastSampledByContext))
	assert.Equal(t, 0, len(sampler.contextResolver.resolver.contextsByKey))
}
func TestCounterExpirySeconds(t *testing.T) {
	testWithTagsStore(t, testCounterExpirySeconds)
}

func testSketch(t *testing.T, store *tags.Store) {
	const (
		defaultBucketSize = 10
	)

	var (
		sampler = testTimeSampler()

		insert = func(t *testing.T, ts float64, name string, tags []string, host string, values ...float64) {
			t.Helper()
			for _, v := range values {
				sampler.sample(&metrics.MetricSample{
					Name:       name,
					Tags:       tags,
					Host:       host,
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
		_, flushed := flushSerie(sampler, timeNowNano())
		require.Len(t, flushed, 0)
	})

	t.Run("single bucket", func(t *testing.T) {
		var (
			now    float64
			name   = "m.0"
			tags   = []string{"a"}
			host   = "host"
			exp    = &quantile.Sketch{}
			keyGen = ckey.NewKeyGenerator()
		)

		for i := 0; i < bucketSize; i++ {
			v := float64(i)
			insert(t, now, name, tags, host, v)
			exp.Insert(quantile.Default(), v)

			now++
		}

		_, flushed := flushSerie(sampler, now)
		metrics.AssertSketchSeriesEqual(t, &metrics.SketchSeries{
			Name:     name,
			Tags:     tagset.CompositeTagsFromSlice(tags),
			Host:     host,
			Interval: 10,
			Points: []metrics.SketchPoint{
				{
					Sketch: exp,
					Ts:     0,
				},
			},
			ContextKey: keyGen.Generate(name, host, tagset.NewHashingTagsAccumulatorWithTags(tags)),
		}, flushed[0])

		_, flushed = flushSerie(sampler, now)
		require.Len(t, flushed, 0, "these points have already been flushed")
	})

}
func TestSketch(t *testing.T) {
	testWithTagsStore(t, testSketch)
}

func testSketchBucketSampling(t *testing.T, store *tags.Store) {
	sampler := testTimeSampler()

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
	sampler.sample(&mSample1, 10001)
	sampler.sample(&mSample2, 10002)
	sampler.sample(&mSample1, 10011)
	sampler.sample(&mSample2, 10012)
	sampler.sample(&mSample1, 10021)

	_, flushed := flushSerie(sampler, 10020.0)
	expSketch := &quantile.Sketch{}
	expSketch.Insert(quantile.Default(), 1, 2)

	assert.Equal(t, 1, len(flushed))
	metrics.AssertSketchSeriesEqual(t, &metrics.SketchSeries{
		Name:     "test.metric.name",
		Tags:     tagset.CompositeTagsFromSlice([]string{"a", "b"}),
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
func TestSketchBucketSampling(t *testing.T) {
	testWithTagsStore(t, testSketchBucketSampling)
}

func testSketchContextSampling(t *testing.T, store *tags.Store) {
	sampler := testTimeSampler()

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
	sampler.sample(&mSample1, 10011)
	sampler.sample(&mSample2, 10011)

	_, flushed := flushSerie(sampler, 10020)
	expSketch := &quantile.Sketch{}
	expSketch.Insert(quantile.Default(), 1)

	assert.Equal(t, 2, len(flushed))
	sort.Slice(flushed, func(i, j int) bool {
		return flushed[i].Name < flushed[j].Name
	})

	metrics.AssertSketchSeriesEqual(t, &metrics.SketchSeries{
		Name:     "test.metric.name1",
		Tags:     tagset.CompositeTagsFromSlice([]string{"a", "b"}),
		Interval: 10,
		Points: []metrics.SketchPoint{
			{Ts: 10010, Sketch: expSketch},
		},
		ContextKey: generateContextKey(&mSample1),
	}, flushed[0])

	metrics.AssertSketchSeriesEqual(t, &metrics.SketchSeries{
		Name:     "test.metric.name2",
		Tags:     tagset.CompositeTagsFromSlice([]string{"a", "c"}),
		Interval: 10,
		Points: []metrics.SketchPoint{
			{Ts: 10010, Sketch: expSketch},
		},
		ContextKey: generateContextKey(&mSample2),
	}, flushed[1])
}
func TestSketchContextSampling(t *testing.T) {
	testWithTagsStore(t, testSketchContextSampling)
}

func testBucketSamplingWithSketchAndSeries(t *testing.T, store *tags.Store) {
	sampler := testTimeSampler()

	dSample1 := metrics.MetricSample{
		Name:       "distribution.metric.name1",
		Value:      1,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"a", "b"},
		SampleRate: 1,
	}
	sampler.sample(&dSample1, 12345.0)
	sampler.sample(&dSample1, 12355.0)
	sampler.sample(&dSample1, 12365.0)

	mSample := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}
	sampler.sample(&mSample, 12345.0)
	sampler.sample(&mSample, 12355.0)
	sampler.sample(&mSample, 12365.0)

	series, sketches := flushSerie(sampler, 12360.0)

	expectedSerie := &metrics.Serie{
		Name:       "my.metric.name",
		Tags:       tagset.CompositeTagsFromSlice([]string{"foo", "bar"}),
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

	metrics.AssertSketchSeriesEqual(t, &metrics.SketchSeries{
		Name:     "distribution.metric.name1",
		Tags:     tagset.CompositeTagsFromSlice([]string{"a", "b"}),
		Interval: 10,
		Points: []metrics.SketchPoint{
			{Ts: 12340.0, Sketch: expSketch},
			{Ts: 12350.0, Sketch: expSketch},
		},
		ContextKey: generateContextKey(&dSample1),
	}, sketches[0])
}
func TestBucketSamplingWithSketchAndSeries(t *testing.T) {
	testWithTagsStore(t, testBucketSamplingWithSketchAndSeries)
}

func benchmarkTimeSampler(b *testing.B, store *tags.Store) {
	sampler := testTimeSampler()

	sample := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}
	for n := 0; n < b.N; n++ {
		sampler.sample(&sample, 12345.0)
	}
}
func BenchmarkTimeSampler(b *testing.B) {
	benchWithTagsStore(b, benchmarkTimeSampler)
}

func flushSerie(sampler *TimeSampler, timestamp float64) (metrics.Series, metrics.SketchSeriesList) {
	var series metrics.Series
	var sketches metrics.SketchSeriesList

	sampler.flush(timestamp, &series, &sketches)
	return series, sketches
}
