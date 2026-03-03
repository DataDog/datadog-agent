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

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	filterlistdef "github.com/DataDog/datadog-agent/comp/filterlist/def"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
	"github.com/DataDog/datadog-agent/pkg/util/strings"
)

func generateSerieContextKey(serie *metrics.Serie) ckey.ContextKey {
	l := ckey.NewKeyGenerator()
	var tags []string
	serie.Tags.ForEach(func(tag string) {
		tags = append(tags, tag)
	})
	return l.Generate(serie.Name, serie.Host, tagset.NewHashingTagsAccumulatorWithTags(tags))
}

func testTimeSampler(store *tags.Store) *TimeSampler {
	sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")
	return sampler
}

// TimeSampler
func TestCalculateBucketStart(t *testing.T) {
	sampler := testTimeSampler(tags.NewStore(true, "test"))

	assert.Equal(t, int64(123450), sampler.calculateBucketStart(123456.5))
	assert.Equal(t, int64(123460), sampler.calculateBucketStart(123460.5))
}

func testBucketSampling(t *testing.T, store *tags.Store) {
	sampler := testTimeSampler(store)

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

	series, _ := flushSerie(sampler, 12360.0, false)

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
	sampler := testTimeSampler(store)

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

	series, _ := flushSerie(sampler, 12360.0, false)

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
	sampler := testTimeSampler(store)

	math.Abs(1)
	sampleCounter1 := &metrics.MetricSample{
		Name:       "my.counter1",
		Value:      1,
		Mtype:      metrics.CounterType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}

	sampleCounter2 := &metrics.MetricSample{
		Name:       "my.counter2",
		Value:      2,
		Mtype:      metrics.CounterType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
	}

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

	series, _ := flushSerie(sampler, 1010.0, false)

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

	metrics.AssertSeriesEqual(t, expectedSeries, series)

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

	series, _ = flushSerie(sampler, 1040.0, false)

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
	series, _ = flushSerie(sampler, 1045.0, false)
	assert.Equal(t, 0, len(series))

	// Now we should get the empty counters
	series, _ = flushSerie(sampler, 1050.0, false)
	assert.Equal(t, 2, len(series))

	series, _ = flushSerie(sampler, 1329.0, false)
	// Counter1 should have stopped reporting but the context is not expired yet
	// Counter2 should still report
	assert.Equal(t, 1, len(series))
	assert.Equal(t, 2, len(sampler.contextResolver.resolver.contextsByKey))

	series, _ = flushSerie(sampler, 1800.0, false)
	// Everything stopped reporting and is expired
	assert.Equal(t, 0, len(series))
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
		sampler = testTimeSampler(store)

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
		_, flushed := flushSerie(sampler, timeNowNano(), false)
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

		_, flushed := flushSerie(sampler, now, false)
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

		_, flushed = flushSerie(sampler, now, false)
		require.Len(t, flushed, 0, "these points have already been flushed")
	})

}
func TestSketch(t *testing.T) {
	testWithTagsStore(t, testSketch)
}

func testSketchBucketSampling(t *testing.T, store *tags.Store) {
	sampler := testTimeSampler(store)

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

	_, flushed := flushSerie(sampler, 10020.0, false)
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
	sampler := testTimeSampler(store)

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

	_, flushed := flushSerie(sampler, 10020, false)
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
	sampler := testTimeSampler(store)

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

	series, sketches := flushSerie(sampler, 12360.0, false)

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

func testFlushMissingContext(t *testing.T, store *tags.Store) {
	sampler := testTimeSampler(store)

	sampler.sample(&metrics.MetricSample{
		Name:       "test.gauge",
		Value:      1,
		Mtype:      metrics.GaugeType,
		SampleRate: 1,
	}, 1000)
	sampler.sample(&metrics.MetricSample{
		Name:       "test.sketch",
		Value:      1,
		Mtype:      metrics.DistributionType,
		SampleRate: 1,
	}, 1000)

	// Simulate a sutation where contexts are expired prematurely.
	sampler.contextResolver.expireContexts(10000)

	assert.Len(t, sampler.contextResolver.resolver.contextsByKey, 0)

	metrics, sketches := flushSerie(sampler, 1100, false)

	assert.Len(t, metrics, 0)
	assert.Len(t, sketches, 0)
}
func TestFlushMissingContext(t *testing.T) {
	testWithTagsStore(t, testFlushMissingContext)
}
func testFlushFilterList(t *testing.T, store *tags.Store) {
	sampler := testTimeSampler(store)
	matcher := strings.NewMatcher([]string{
		"test.histogram.avg",
		"test.histogram.count",
	}, false)


	sampler.sample(&metrics.MetricSample{
		Name:       "test.gauge",
		Value:      1,
		Mtype:      metrics.GaugeType,
		SampleRate: 1,
	}, 1000)
	sampler.sample(&metrics.MetricSample{
		Name:       "test.histogram",
		Value:      1,
		Mtype:      metrics.HistogramType,
		SampleRate: 1,
	}, 1000)
	sampler.sample(&metrics.MetricSample{
		Name:       "test.sketch",
		Value:      1,
		Mtype:      metrics.DistributionType,
		SampleRate: 1,
	}, 1000)

	metrics, sketches := flushSerieWithFilterList(sampler, 1100, &matcher, false)

	assert.Len(t, metrics, 4)
	assert.Len(t, sketches, 1)

	names := []string{}
	for _, metric := range metrics {
		names = append(names, metric.Name)
	}
	for _, sketch := range sketches {
		names = append(names, sketch.Name)
	}
	assert.ElementsMatch(t, names, []string{
		"test.histogram.max",
		"test.histogram.median",
		"test.histogram.95percentile",
		"test.gauge",
		"test.sketch",
	})
}

func TestFlushFilterList(t *testing.T) {
	testWithTagsStore(t, testFlushFilterList)
}

func TestForcedFlush(t *testing.T) {
	sampler := testTimeSampler(tags.NewStore(false, "test"))

	testMetric1 := &metrics.MetricSample{
		Name:       "test.count1",
		Value:      1,
		Mtype:      metrics.CountType,
		SampleRate: 1,
	}
	testMetric2 := &metrics.MetricSample{
		Name:       "test.count2",
		Value:      1,
		Mtype:      metrics.CountType,
		SampleRate: 1,
	}
	testSketch := &metrics.MetricSample{
		Name:       "test.sketch",
		Value:      1,
		Mtype:      metrics.DistributionType,
		SampleRate: 1,
	}

	sampler.sample(testMetric1, 999)
	sampler.sample(testMetric2, 1010)
	sampler.sample(testMetric2, 1022)

	sampler.sample(testSketch, 999)
	sampler.sample(testSketch, 1010)
	sampler.sample(testSketch, 1021)

	mSerie, sSerie := flushSerie(sampler, 1000, true)

	expMetric1 := &metrics.Serie{
		Name:     testMetric1.Name,
		Points:   []metrics.Point{{Ts: 990.0, Value: float64(1)}},
		Tags:     tagset.CompositeTags{},
		Host:     "",
		MType:    metrics.APICountType,
		Interval: 10,
	}

	expMetric2 := &metrics.Serie{
		Name: testMetric2.Name,
		Points: []metrics.Point{
			{Ts: 1010.0, Value: float64(1)},
			{Ts: 1020.0, Value: float64(1)},
		},
		Tags:     tagset.CompositeTags{},
		Host:     "",
		MType:    metrics.APICountType,
		Interval: 10,
	}

	require.Len(t, mSerie, 2)
	if mSerie[0].Name == testMetric1.Name {
		metrics.AssertSerieEqual(t, expMetric1, mSerie[0])
		metrics.AssertSerieEqual(t, expMetric2, mSerie[1])
	} else {
		metrics.AssertSerieEqual(t, expMetric1, mSerie[1])
		metrics.AssertSerieEqual(t, expMetric2, mSerie[0])
	}

	expSketch := &quantile.Sketch{}
	expSketch.Insert(quantile.Default(), 1)
	metrics.AssertSketchSeriesEqual(t, &metrics.SketchSeries{
		Name:     testSketch.Name,
		Tags:     tagset.CompositeTags{},
		Interval: 10,
		Points: []metrics.SketchPoint{
			{Ts: 990.0, Sketch: expSketch},
			{Ts: 1010.0, Sketch: expSketch},
			{Ts: 1020.0, Sketch: expSketch},
		},
		ContextKey: generateContextKey(testSketch),
	}, sSerie[0])
}

func benchmarkTimeSampler(b *testing.B, store *tags.Store) {
	sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")

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

func flushSerie(sampler *TimeSampler, timestamp float64, forceFlushAll bool) (metrics.Series, metrics.SketchSeriesList) {
	var series metrics.Series
	var sketches metrics.SketchSeriesList

	sampler.flush(timestamp, &series, &sketches, nil, forceFlushAll, filterlist.NewNoopTagMatcher())
	return series, sketches
}

func flushSerieWithFilterList(sampler *TimeSampler, timestamp float64, filter *strings.Matcher, forceFlushAll bool) (metrics.Series, metrics.SketchSeriesList) {
	var series metrics.Series
	var sketches metrics.SketchSeriesList

	sampler.flush(timestamp, &series, &sketches, filter, forceFlushAll, filterlist.NewNoopTagMatcher())
	return series, sketches
}

func flushWithTagFilter(sampler *TimeSampler, timestamp float64, tagFilter filterlistdef.TagMatcher, forceFlushAll bool) (metrics.Series, metrics.SketchSeriesList) {
	var series metrics.Series
	var sketches metrics.SketchSeriesList

	sampler.flush(timestamp, &series, &sketches, nil, forceFlushAll, tagFilter)
	return series, sketches
}

// newDistSample creates a distribution MetricSample with the given name, tags, value and timestamp.
func newDistSample(name string, tags []string, value float64, ts float64) *metrics.MetricSample {
	return &metrics.MetricSample{
		Name:       name,
		Value:      value,
		Mtype:      metrics.DistributionType,
		Tags:       tags,
		SampleRate: 1,
		Timestamp:  ts,
	}
}

// stripTagFilter returns a tag filter that excludes the named tags from the named metric.
func stripTagFilter(metricName string, tagsToStrip []string) filterlistdef.TagMatcher {
	return filterlist.NewTagMatcher(map[string]filterlist.MetricTagList{
		metricName: {Tags: tagsToStrip, Action: "exclude"},
	})
}

// TestFlushSketchesTagStrip groups all tests for the tag-stripping aggregation
// behaviour in flushSketches.
func TestFlushSketchesTagStrip(t *testing.T) {
	testWithTagsStore(t, testFlushSketchesTagStripBasic)
	testWithTagsStore(t, testFlushSketchesTagStripMerge)
	testWithTagsStore(t, testFlushSketchesTagStripNoMerge)
	testWithTagsStore(t, testFlushSketchesTagStripUnmatchedMetric)
	testWithTagsStore(t, testFlushSketchesTagStripMultipleBuckets)
	testWithTagsStore(t, testFlushSketchesTagStripMixedContexts)
}

// testFlushSketchesTagStripBasic verifies that, for a single distribution context,
// flushing with a tag filter produces a SketchSeries whose tags exclude the
// filtered tag.
func testFlushSketchesTagStripBasic(t *testing.T, store *tags.Store) {
	sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")
	tagFilter := stripTagFilter("my.dist", []string{"strip"})

	sampler.sample(newDistSample("my.dist", []string{"keep:yes", "strip:val"}, 1.0, 1001.0), 1001.0)

	_, sketches := flushWithTagFilter(sampler, 1020.0, tagFilter, false)

	require.Len(t, sketches, 1, "should produce exactly one SketchSeries")
	s := sketches[0]
	assert.Equal(t, "my.dist", s.Name)
	metrics.AssertCompositeTagsEqual(t, tagset.CompositeTagsFromSlice([]string{"keep:yes"}), s.Tags)
	require.Len(t, s.Points, 1)

	expSketch := &quantile.Sketch{}
	expSketch.Insert(quantile.Default(), 1.0)
	assert.True(t, expSketch.Equals(s.Points[0].Sketch), "sketch should contain the original value")
}

// testFlushSketchesTagStripMerge verifies that two distribution contexts that
// share the same metric name and differ only in a stripped tag are merged into
// a single SketchSeries whose sketch data contains values from both original
// contexts.
func testFlushSketchesTagStripMerge(t *testing.T, store *tags.Store) {
	sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")
	tagFilter := stripTagFilter("my.dist", []string{"strip"})

	// Two contexts differ only in the stripped "strip" tag.
	sampler.sample(newDistSample("my.dist", []string{"keep:yes", "strip:a"}, 1.0, 1001.0), 1001.0)
	sampler.sample(newDistSample("my.dist", []string{"keep:yes", "strip:b"}, 2.0, 1001.0), 1001.0)

	_, sketches := flushWithTagFilter(sampler, 1020.0, tagFilter, false)

	// The two contexts should be merged into a single SketchSeries.
	require.Len(t, sketches, 1, "two contexts with same stripped key should merge into one SketchSeries")
	s := sketches[0]
	assert.Equal(t, "my.dist", s.Name)
	metrics.AssertCompositeTagsEqual(t, tagset.CompositeTagsFromSlice([]string{"keep:yes"}), s.Tags)
	require.Len(t, s.Points, 1)

	// The merged sketch must contain both values.
	expMerged := &quantile.Sketch{}
	expMerged.Insert(quantile.Default(), 1.0, 2.0)
	assert.True(t, expMerged.Equals(s.Points[0].Sketch), "merged sketch should contain all values from both contexts")
}

// testFlushSketchesTagStripNoMerge verifies that two distribution contexts that
// differ in a non-stripped tag remain distinct after flushing with the filter:
// they should produce two separate SketchSeries.
func testFlushSketchesTagStripNoMerge(t *testing.T, store *tags.Store) {
	sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")
	tagFilter := stripTagFilter("my.dist", []string{"strip"})

	// Two contexts differ in "keep" tag (not stripped) and in "strip" tag (stripped).
	sampler.sample(newDistSample("my.dist", []string{"keep:1", "strip:val"}, 1.0, 1001.0), 1001.0)
	sampler.sample(newDistSample("my.dist", []string{"keep:2", "strip:val"}, 2.0, 1001.0), 1001.0)

	_, sketches := flushWithTagFilter(sampler, 1020.0, tagFilter, false)

	require.Len(t, sketches, 2, "contexts with different non-stripped keys must not be merged")
	sort.Slice(sketches, func(i, j int) bool {
		// Sort by ContextKey for deterministic assertions.
		return sketches[i].ContextKey < sketches[j].ContextKey
	})
	for _, s := range sketches {
		assert.Equal(t, "my.dist", s.Name)
		// Each sketch must NOT contain the "strip" tag.
		s.Tags.ForEach(func(tag string) {
			assert.NotContains(t, tag, "strip:", "stripped tag must not appear in output tags")
		})
		// Each sketch must still have exactly one point.
		require.Len(t, s.Points, 1)
	}
	// Confirm the two contexts have distinct tags (keep:1 vs keep:2).
	allTags := make([]string, 0, 4)
	for _, s := range sketches {
		s.Tags.ForEach(func(tag string) { allTags = append(allTags, tag) })
	}
	assert.ElementsMatch(t, []string{"keep:1", "keep:2"}, allTags)
}

// testFlushSketchesTagStripUnmatchedMetric verifies that metrics not covered by
// the tag filter are passed through unmodified: their tags must not be stripped.
func testFlushSketchesTagStripUnmatchedMetric(t *testing.T, store *tags.Store) {
	sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")
	// The filter only applies to "my.dist", not to "other.metric".
	tagFilter := stripTagFilter("my.dist", []string{"strip"})

	sampler.sample(newDistSample("other.metric", []string{"keep:yes", "strip:val"}, 1.0, 1001.0), 1001.0)

	_, sketches := flushWithTagFilter(sampler, 1020.0, tagFilter, false)

	require.Len(t, sketches, 1)
	s := sketches[0]
	assert.Equal(t, "other.metric", s.Name)
	// Full tag set must be preserved because the filter does not match "other.metric".
	metrics.AssertCompositeTagsEqual(t,
		tagset.CompositeTagsFromSlice([]string{"keep:yes", "strip:val"}),
		s.Tags,
	)
}

// testFlushSketchesTagStripMultipleBuckets verifies that when two contexts are
// merged across multiple time buckets, the per-bucket sketches are merged
// independently.
func testFlushSketchesTagStripMultipleBuckets(t *testing.T, store *tags.Store) {
	sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")
	tagFilter := stripTagFilter("my.dist", []string{"strip"})

	// Bucket 1 (ts 1001-1009): context A value 1.0, context B value 2.0.
	sampler.sample(newDistSample("my.dist", []string{"keep:yes", "strip:a"}, 1.0, 1001.0), 1001.0)
	sampler.sample(newDistSample("my.dist", []string{"keep:yes", "strip:b"}, 2.0, 1001.0), 1001.0)
	// Bucket 2 (ts 1011-1019): context A value 3.0, context B value 4.0.
	sampler.sample(newDistSample("my.dist", []string{"keep:yes", "strip:a"}, 3.0, 1011.0), 1011.0)
	sampler.sample(newDistSample("my.dist", []string{"keep:yes", "strip:b"}, 4.0, 1011.0), 1011.0)

	_, sketches := flushWithTagFilter(sampler, 1030.0, tagFilter, false)

	require.Len(t, sketches, 1, "both buckets should be merged into a single SketchSeries")
	s := sketches[0]
	assert.Equal(t, "my.dist", s.Name)
	metrics.AssertCompositeTagsEqual(t, tagset.CompositeTagsFromSlice([]string{"keep:yes"}), s.Tags)

	// There must be exactly two time points (one per bucket).
	require.Len(t, s.Points, 2)
	sort.Slice(s.Points, func(i, j int) bool { return s.Points[i].Ts < s.Points[j].Ts })

	// Bucket 1: sketch must contain both 1.0 and 2.0.
	expBucket1 := &quantile.Sketch{}
	expBucket1.Insert(quantile.Default(), 1.0, 2.0)
	assert.True(t, expBucket1.Equals(s.Points[0].Sketch),
		"bucket 1 sketch should be the merge of context A and B values")

	// Bucket 2: sketch must contain both 3.0 and 4.0.
	expBucket2 := &quantile.Sketch{}
	expBucket2.Insert(quantile.Default(), 3.0, 4.0)
	assert.True(t, expBucket2.Equals(s.Points[1].Sketch),
		"bucket 2 sketch should be the merge of context A and B values")
}

// testFlushSketchesTagStripMixedContexts verifies that the merge logic operates
// independently for each stripped key: contexts that share a stripped key are
// merged together, while contexts whose stripped keys differ remain separate.
func testFlushSketchesTagStripMixedContexts(t *testing.T, store *tags.Store) {
	sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")
	tagFilter := stripTagFilter("my.dist", []string{"strip"})

	// Group A: same stripped key (keep:1).
	sampler.sample(newDistSample("my.dist", []string{"keep:1", "strip:x"}, 1.0, 1001.0), 1001.0)
	sampler.sample(newDistSample("my.dist", []string{"keep:1", "strip:y"}, 2.0, 1001.0), 1001.0)
	// Group B: different stripped key (keep:2) – stays separate.
	sampler.sample(newDistSample("my.dist", []string{"keep:2", "strip:x"}, 10.0, 1001.0), 1001.0)

	_, sketches := flushWithTagFilter(sampler, 1020.0, tagFilter, false)

	require.Len(t, sketches, 2, "two distinct stripped keys should yield two SketchSeries")

	// Identify which sketch is which by their tags.
	var groupA, groupB *metrics.SketchSeries
	for i := range sketches {
		s := sketches[i]
		s.Tags.ForEach(func(tag string) {
			if tag == "keep:1" {
				groupA = s
			} else if tag == "keep:2" {
				groupB = s
			}
		})
	}
	require.NotNil(t, groupA, "should find a SketchSeries for keep:1")
	require.NotNil(t, groupB, "should find a SketchSeries for keep:2")

	// Group A sketch should contain both 1.0 and 2.0 (merged from two contexts).
	expGroupA := &quantile.Sketch{}
	expGroupA.Insert(quantile.Default(), 1.0, 2.0)
	require.Len(t, groupA.Points, 1)
	assert.True(t, expGroupA.Equals(groupA.Points[0].Sketch),
		"group A sketch should be the merge of the two keep:1 contexts")

	// Group B sketch should contain only 10.0 (no merge).
	expGroupB := &quantile.Sketch{}
	expGroupB.Insert(quantile.Default(), 10.0)
	require.Len(t, groupB.Points, 1)
	assert.True(t, expGroupB.Equals(groupB.Points[0].Sketch),
		"group B sketch should contain only the keep:2 context value")
}

// testFlushSketchesTagStripContextKeyReflectsStrippedTags verifies that the
// ContextKey stored in the output SketchSeries equals the key that would be
// generated from the stripped tag set alone (not the original full tag set).
func testFlushSketchesTagStripContextKeyReflectsStrippedTags(t *testing.T, store *tags.Store) {
	sampler := NewTimeSampler(TimeSamplerID(0), 10, store, nooptagger.NewComponent(), "host")
	tagFilter := stripTagFilter("my.dist", []string{"strip"})

	sampler.sample(newDistSample("my.dist", []string{"keep:yes", "strip:val"}, 1.0, 1001.0), 1001.0)

	_, sketches := flushWithTagFilter(sampler, 1020.0, tagFilter, false)
	require.Len(t, sketches, 1)

	// Compute what the context key should be for the stripped tag set.
	kg := ckey.NewKeyGenerator()
	expectedKey := kg.Generate("my.dist", "", tagset.NewHashingTagsAccumulatorWithTags([]string{"keep:yes"}))
	assert.Equal(t, expectedKey, sketches[0].ContextKey,
		"ContextKey should reflect stripped tags, not original tags")
}

func TestFlushSketchesTagStripContextKeyReflectsStrippedTags(t *testing.T) {
	testWithTagsStore(t, testFlushSketchesTagStripContextKeyReflectsStrippedTags)
}
