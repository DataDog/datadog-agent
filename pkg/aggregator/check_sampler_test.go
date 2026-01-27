// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
	"github.com/DataDog/datadog-agent/pkg/util/strings"
)

func generateContextKey(sample metrics.MetricSampleContext) ckey.ContextKey {
	k := ckey.NewKeyGenerator()
	tb := tagset.NewHashingTagsAccumulator()
	taggerComponent := nooptagger.NewComponent()
	sample.GetTags(tb, tb, taggerComponent)
	return k.Generate(sample.GetName(), sample.GetHost(), tb)
}

func testCheckGaugeSampling(t *testing.T, store *tags.Store) {
	taggerComponent := nooptagger.NewComponent()
	checkSampler := newCheckSampler(1, true, true, 1*time.Second, true, store, checkid.ID("hello:world:1234"), taggerComponent)

	tagmatcher := filterlistimpl.NewNoopTagMatcher()
	mSample1 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}
	mSample2 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      2,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12347.0,
	}
	mSample3 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"foo", "bar", "baz"},
		SampleRate: 1,
		Timestamp:  12348.0,
	}

	checkSampler.addSample(&mSample1, tagmatcher)
	checkSampler.addSample(&mSample2, tagmatcher)
	checkSampler.addSample(&mSample3, tagmatcher)
	matcher := strings.NewMatcher([]string{}, false)
	checkSampler.commit(12349.0, &matcher)
	series, _ := checkSampler.flush()

	expectedSerie1 := &metrics.Serie{
		Name:           "my.metric.name",
		Tags:           tagset.CompositeTagsFromSlice([]string{"bar", "foo"}),
		Points:         []metrics.Point{{Ts: 12349.0, Value: mSample2.Value}},
		MType:          metrics.APIGaugeType,
		SourceTypeName: checksSourceTypeName,
		ContextKey:     generateContextKey(&mSample2),
		NameSuffix:     "",
	}

	expectedSerie2 := &metrics.Serie{
		Name:           "my.metric.name",
		Tags:           tagset.CompositeTagsFromSlice([]string{"bar", "baz", "foo"}),
		Points:         []metrics.Point{{Ts: 12349.0, Value: mSample3.Value}},
		MType:          metrics.APIGaugeType,
		SourceTypeName: checksSourceTypeName,
		ContextKey:     generateContextKey(&mSample3),
		NameSuffix:     "",
	}

	expectedSeries := []*metrics.Serie{expectedSerie1, expectedSerie2}
	metrics.AssertSeriesEqual(t, expectedSeries, series)
}

func TestCheckGaugeSampling(t *testing.T) {
	testWithTagsStore(t, testCheckGaugeSampling)
}

func testCheckRateSampling(t *testing.T, store *tags.Store) {
	taggerComponent := nooptagger.NewComponent()
	checkSampler := newCheckSampler(1, true, true, 1*time.Second, true, store, checkid.ID("hello:world:1234"), taggerComponent)

	tagmatcher := filterlistimpl.NewNoopTagMatcher()

	mSample1 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.RateType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}
	mSample2 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      10,
		Mtype:      metrics.RateType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12347.5,
	}
	mSample3 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.RateType,
		Tags:       []string{"foo", "bar", "baz"},
		SampleRate: 1,
		Timestamp:  12348.0,
	}

	checkSampler.addSample(&mSample1, tagmatcher)
	checkSampler.addSample(&mSample2, tagmatcher)
	checkSampler.addSample(&mSample3, tagmatcher)

	matcher := strings.NewMatcher([]string{}, false)
	checkSampler.commit(12349.0, &matcher)
	series, _ := checkSampler.flush()

	expectedSerie := &metrics.Serie{
		Name:           "my.metric.name",
		Tags:           tagset.CompositeTagsFromSlice([]string{"foo", "bar"}),
		Points:         []metrics.Point{{Ts: 12347.5, Value: 3.6}},
		MType:          metrics.APIGaugeType,
		SourceTypeName: checksSourceTypeName,
		NameSuffix:     "",
	}

	if assert.Equal(t, 1, len(series)) {
		metrics.AssertSerieEqual(t, expectedSerie, series[0])
	}
}

func TestCheckRateSampling(t *testing.T) {
	testWithTagsStore(t, testCheckRateSampling)
}

func testHistogramCountSampling(t *testing.T, store *tags.Store) {
	taggerComponent := nooptagger.NewComponent()
	checkSampler := newCheckSampler(1, true, true, 1*time.Second, true, store, checkid.ID("hello:world:1234"), taggerComponent)

	tagmatcher := filterlistimpl.NewNoopTagMatcher()
	matcher := strings.NewMatcher([]string{}, false)

	mSample1 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.HistogramType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}
	mSample2 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      10,
		Mtype:      metrics.HistogramType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12347.5,
	}
	mSample3 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.HistogramType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12348.0,
	}

	checkSampler.addSample(&mSample1, tagmatcher)
	checkSampler.addSample(&mSample2, tagmatcher)
	checkSampler.addSample(&mSample3, tagmatcher)

	checkSampler.commit(12349.0, &matcher)
	require.Equal(t, 1, checkSampler.contextResolver.length())
	series, _ := checkSampler.flush()

	// Check that the `.count` metric returns a raw count of the samples, with no interval normalization
	expectedCountSerie := &metrics.Serie{
		Name:           "my.metric.name.count",
		Tags:           tagset.CompositeTagsFromSlice([]string{"foo", "bar"}),
		Points:         []metrics.Point{{Ts: 12349.0, Value: 3.}},
		MType:          metrics.APIRateType,
		SourceTypeName: checksSourceTypeName,
		NameSuffix:     ".count",
	}

	require.Len(t, series, 5)

	foundCount := false
	for _, serie := range series {
		if serie.Name == expectedCountSerie.Name {
			metrics.AssertSerieEqual(t, expectedCountSerie, serie)
			foundCount = true
		}
	}

	assert.True(t, foundCount)

	checkSampler.commit(12349.0, &matcher)
	require.Equal(t, 0, checkSampler.contextResolver.length())
}

func TestHistogramCountSampling(t *testing.T) {
	testWithTagsStore(t, testHistogramCountSampling)
}

func testCheckHistogramBucketSampling(t *testing.T, store *tags.Store) {
	taggerComponent := nooptagger.NewComponent()
	checkSampler := newCheckSampler(1, true, true, 1*time.Second, true, store, checkid.ID("hello:world:1234"), taggerComponent)

	tagmatcher := filterlistimpl.NewNoopTagMatcher()
	matcher := strings.NewMatcher([]string{}, false)

	bucket1 := &metrics.HistogramBucket{
		Name:            "my.histogram",
		Value:           4.0,
		LowerBound:      10.0,
		UpperBound:      20.0,
		Tags:            []string{"foo", "bar"},
		Timestamp:       12345.0,
		Monotonic:       true,
		FlushFirstValue: true,
	}

	checkSampler.addBucket(bucket1, tagmatcher)
	assert.Equal(t, len(checkSampler.lastBucketValue), 1)

	checkSampler.commit(12349.0, &matcher)
	_, flushed := checkSampler.flush()
	assert.Equal(t, 1, len(flushed))

	expSketch := &quantile.Sketch{}
	// linear interpolated values
	expSketch.Insert(quantile.Default(), 10.0, 12.5, 15.0, 17.5)

	// ~3% error seen in this test case for sums (sum error is additive so it's always the worst)
	metrics.AssertSketchSeriesApproxEqual(t, &metrics.SketchSeries{
		Name: "my.histogram",
		Tags: tagset.CompositeTagsFromSlice([]string{"foo", "bar"}),
		Points: []metrics.SketchPoint{
			{Ts: 12345.0, Sketch: expSketch},
		},
		ContextKey: generateContextKey(bucket1),
	}, flushed[0], .03)

	bucket2 := &metrics.HistogramBucket{
		Name:       "my.histogram",
		Value:      6.0,
		LowerBound: 10.0,
		UpperBound: 20.0,
		Tags:       []string{"foo", "bar"},
		Timestamp:  12400.0,
		Monotonic:  true,
	}
	checkSampler.addBucket(bucket2, tagmatcher)
	assert.Equal(t, len(checkSampler.lastBucketValue), 1)

	checkSampler.commit(12401.0, &matcher)
	assert.Len(t, checkSampler.lastBucketValue, 1)
	checkSampler.commit(12401.0, &matcher)
	assert.Len(t, checkSampler.lastBucketValue, 0)
	_, flushed = checkSampler.flush()

	expSketch = &quantile.Sketch{}
	// linear interpolated values (only 2 since we stored the delta)
	expSketch.Insert(quantile.Default(), 10.0, 15.0)

	assert.Equal(t, 1, len(flushed))
	// ~3% error seen in this test case for sums (sum error is additive so it's always the worst)
	metrics.AssertSketchSeriesApproxEqual(t, &metrics.SketchSeries{
		Name: "my.histogram",
		Tags: tagset.CompositeTagsFromSlice([]string{"foo", "bar"}),
		Points: []metrics.SketchPoint{
			{Ts: 12400.0, Sketch: expSketch},
		},
		ContextKey: generateContextKey(bucket1),
	}, flushed[0], .03)

	// garbage collection
	time.Sleep(11 * time.Millisecond)
	checkSampler.flush()
	assert.Equal(t, len(checkSampler.lastBucketValue), 0)
}

func TestCheckHistogramBucketSampling(t *testing.T) {
	testWithTagsStore(t, testCheckHistogramBucketSampling)
}

func testCheckHistogramBucketDontFlushFirstValue(t *testing.T, store *tags.Store) {
	taggerComponent := nooptagger.NewComponent()
	checkSampler := newCheckSampler(1, true, true, 1*time.Second, true, store, checkid.ID("hello:world:1234"), taggerComponent)

	tagmatcher := filterlistimpl.NewNoopTagMatcher()

	bucket1 := &metrics.HistogramBucket{
		Name:            "my.histogram",
		Value:           4.0,
		LowerBound:      10.0,
		UpperBound:      20.0,
		Tags:            []string{"foo", "bar"},
		Timestamp:       12345.0,
		Monotonic:       true,
		FlushFirstValue: false,
	}
	checkSampler.addBucket(bucket1, tagmatcher)
	assert.Equal(t, len(checkSampler.lastBucketValue), 1)

	matcher := strings.NewMatcher([]string{}, false)
	checkSampler.commit(12349.0, &matcher)
	_, flushed := checkSampler.flush()
	assert.Equal(t, 0, len(flushed))

	bucket2 := &metrics.HistogramBucket{
		Name:       "my.histogram",
		Value:      6.0,
		LowerBound: 10.0,
		UpperBound: 20.0,
		Tags:       []string{"foo", "bar"},
		Timestamp:  12400.0,
		Monotonic:  true,
	}
	checkSampler.addBucket(bucket2, tagmatcher)
	assert.Equal(t, len(checkSampler.lastBucketValue), 1)

	checkSampler.commit(12401.0, &matcher)
	_, flushed = checkSampler.flush()

	expSketch := &quantile.Sketch{}
	// linear interpolated values (only 2 since we stored the delta)
	expSketch.Insert(quantile.Default(), 10.0, 15.0)

	assert.Equal(t, 1, len(flushed))
	// ~3% error seen in this test case for sums (sum error is additive so it's always the worst)
	metrics.AssertSketchSeriesApproxEqual(t, &metrics.SketchSeries{
		Name: "my.histogram",
		Tags: tagset.CompositeTagsFromSlice([]string{"foo", "bar"}),
		Points: []metrics.SketchPoint{
			{Ts: 12400.0, Sketch: expSketch},
		},
		ContextKey: generateContextKey(bucket1),
	}, flushed[0], .03)
}

func TestCheckHistogramBucketDontFlushFirstValue(t *testing.T) {
	testWithTagsStore(t, testCheckHistogramBucketDontFlushFirstValue)
}

func testCheckHistogramBucketReset(t *testing.T, store *tags.Store) {
	taggerComponent := nooptagger.NewComponent()
	checkSampler := newCheckSampler(1, true, true, 1*time.Second, true, store, checkid.ID("hello:world:1234"), taggerComponent)

	tagmatcher := filterlistimpl.NewNoopTagMatcher()

	checkSampler.addBucket(&metrics.HistogramBucket{
		Name:            "my.histogram",
		Value:           6.0,
		LowerBound:      10.0,
		UpperBound:      20.0,
		Timestamp:       12400.0,
		Monotonic:       true,
		FlushFirstValue: false,
	}, tagmatcher)
	checkSampler.commit(12401, nil)

	checkSampler.addBucket(&metrics.HistogramBucket{
		Name:            "my.histogram",
		Value:           9.0,
		LowerBound:      10.0,
		UpperBound:      20.0,
		Timestamp:       12410.0,
		Monotonic:       true,
		FlushFirstValue: true,
	}, tagmatcher)

	checkSampler.commit(12411, nil)

	checkSampler.addBucket(&metrics.HistogramBucket{
		Name:            "my.histogram",
		Value:           2.0,
		LowerBound:      10.0,
		UpperBound:      20.0,
		Timestamp:       12420.0,
		Monotonic:       true,
		FlushFirstValue: true,
	}, tagmatcher)

	checkSampler.commit(12421, nil)

	checkSampler.addBucket(&metrics.HistogramBucket{
		Name:            "my.histogram",
		Value:           1.0,
		LowerBound:      10.0,
		UpperBound:      20.0,
		Timestamp:       12440.0,
		Monotonic:       true,
		FlushFirstValue: false,
	}, tagmatcher)

	checkSampler.commit(12441, nil)

	_, flushed := checkSampler.flush()

	require.Len(t, flushed, 2)
	metrics.AssertSketchSeriesApproxEqual(t, &metrics.SketchSeries{
		Name:       "my.histogram",
		ContextKey: generateContextKey(&metrics.HistogramBucket{Name: "my.histogram"}),
		Points: []metrics.SketchPoint{
			{Ts: 12410, Sketch: sketchOf(10, 20, 3)},
		},
	}, flushed[0], 0.01)

	metrics.AssertSketchSeriesApproxEqual(t, &metrics.SketchSeries{
		Name:       "my.histogram",
		ContextKey: generateContextKey(&metrics.HistogramBucket{Name: "my.histogram"}),
		Points: []metrics.SketchPoint{
			{Ts: 12420, Sketch: sketchOf(10, 20, 2)},
		},
	}, flushed[1], 0.01)
}

func TestCheckHistogramBucketReset(t *testing.T) {
	testWithTagsStore(t, testCheckHistogramBucketReset)
}

func sketchOf(lower, upper float64, count uint) *quantile.Sketch {
	s := quantile.Agent{}
	s.InsertInterpolate(lower, upper, count)
	return s.Finish()
}

func testCheckHistogramBucketInfinityBucket(t *testing.T, store *tags.Store) {
	taggerComponent := nooptagger.NewComponent()
	checkSampler := newCheckSampler(1, true, true, 1*time.Second, true, store, checkid.ID("hello:world:1234"), taggerComponent)

	tagmatcher := filterlistimpl.NewNoopTagMatcher()
	matcher := strings.NewMatcher([]string{}, false)

	bucket1 := &metrics.HistogramBucket{
		Name:       "my.histogram",
		Value:      4.0,
		LowerBound: 9000.0,
		UpperBound: math.Inf(1),
		Tags:       []string{"foo", "bar"},
		Timestamp:  12345.0,
	}
	checkSampler.addBucket(bucket1, tagmatcher)

	checkSampler.commit(12349.0, &matcher)
	_, flushed := checkSampler.flush()
	assert.Equal(t, 1, len(flushed))

	expSketch := &quantile.Sketch{}
	expSketch.InsertMany(quantile.Default(), []float64{9000.0, 9000.0, 9000.0, 9000.0})

	// ~3% error seen in this test case for sums (sum error is additive so it's always the worst)
	metrics.AssertSketchSeriesApproxEqual(t, &metrics.SketchSeries{
		Name: "my.histogram",
		Tags: tagset.CompositeTagsFromSlice([]string{"foo", "bar"}),
		Points: []metrics.SketchPoint{
			{Ts: 12345.0, Sketch: expSketch},
		},
		ContextKey: generateContextKey(bucket1),
	}, flushed[0], .03)
}

func TestCheckHistogramBucketInfinityBucket(t *testing.T) {
	testWithTagsStore(t, testCheckHistogramBucketInfinityBucket)
}

func testCheckDistribution(t *testing.T, store *tags.Store) {
	taggerComponent := nooptagger.NewComponent()
	checkSampler := newCheckSampler(1, true, true, 1*time.Second, true, store, checkid.ID("hello:world:1234"), taggerComponent)

	tagmatcher := filterlistimpl.NewNoopTagMatcher()

	mSample1 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}

	checkSampler.addSample(&mSample1, tagmatcher)
	matcher := strings.NewMatcher([]string{}, false)
	checkSampler.commit(12349.0, &matcher)

	_, sketches := checkSampler.flush()

	expSketch := &quantile.Sketch{}
	expSketch.Insert(quantile.Default(), 1)

	metrics.AssertSketchSeriesEqual(t, &metrics.SketchSeries{
		Name: "my.metric.name",
		Tags: tagset.CompositeTagsFromSlice([]string{"foo", "bar"}),
		Points: []metrics.SketchPoint{
			{Ts: 12345.0, Sketch: expSketch},
		},
		ContextKey: generateContextKey(&mSample1),
	}, sketches[0])
}

func TestCheckDistribution(t *testing.T) {
	testWithTagsStore(t, testCheckDistribution)
}

func testFilteredMetrics(t *testing.T, store *tags.Store) {
	taggerComponent := nooptagger.NewComponent()
	checkSampler := newCheckSampler(1, true, true, 1*time.Second, true, store, checkid.ID("hello:world:1234"), taggerComponent)

	tagmatcher := filterlistimpl.NewNoopTagMatcher()

	mSample1 := metrics.MetricSample{
		Name:       "custom.metric.one",
		Value:      50.0,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"host:server1"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}
	mSample2 := metrics.MetricSample{
		Name:       "custom.metric.two",
		Value:      75.0,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"host:server1"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}
	mSample3 := metrics.MetricSample{
		Name:       "custom.metric.three",
		Value:      100.0,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"host:server1"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}
	mSample4 := metrics.MetricSample{
		Name:       "custom.metric.four",
		Value:      5.0,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"host:server1"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}
	mSample5 := metrics.MetricSample{
		Name:       "custom.metric.five",
		Value:      25.0,
		Mtype:      metrics.GaugeType,
		Tags:       []string{"host:server1"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}

	checkSampler.addSample(&mSample1, tagmatcher)
	checkSampler.addSample(&mSample2, tagmatcher)
	checkSampler.addSample(&mSample3, tagmatcher)
	checkSampler.addSample(&mSample4, tagmatcher)
	checkSampler.addSample(&mSample5, tagmatcher)

	// Filter out two and four
	matcher := strings.NewMatcher([]string{"custom.metric.two", "custom.metric.four"}, false)
	checkSampler.commit(12346.0, &matcher)
	series, _ := checkSampler.flush()

	require.Equal(t, 3, len(series))

	// Check that only non-filtered metrics are present
	metricNames := make(map[string]bool)
	for _, serie := range series {
		metricNames[serie.Name] = true
	}

	assert.True(t, metricNames["custom.metric.one"])
	assert.True(t, metricNames["custom.metric.three"])
	assert.True(t, metricNames["custom.metric.five"])

	assert.False(t, metricNames["custom.metric.two"])
	assert.False(t, metricNames["custom.metric.four"])
}

func TestFilteredMetrics(t *testing.T) {
	testWithTagsStore(t, testFilteredMetrics)
}

func testFilteredSketches(t *testing.T, store *tags.Store) {
	taggerComponent := nooptagger.NewComponent()
	checkSampler := newCheckSampler(1, true, true, 1*time.Second, true, store, checkid.ID("hello:world:1234"), taggerComponent)

	tagmatcher := filterlistimpl.NewNoopTagMatcher()

	mSample1 := metrics.MetricSample{
		Name:       "custom.distribution.one",
		Value:      10.0,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"host:server1"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}
	mSample2 := metrics.MetricSample{
		Name:       "custom.distribution.two",
		Value:      20.0,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"host:server1"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}
	mSample3 := metrics.MetricSample{
		Name:       "custom.distribution.three",
		Value:      30.0,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"host:server1"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}
	mSample4 := metrics.MetricSample{
		Name:       "custom.distribution.four",
		Value:      40.0,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"host:server1"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}
	mSample5 := metrics.MetricSample{
		Name:       "custom.distribution.five",
		Value:      50.0,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"host:server1"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}

	checkSampler.addSample(&mSample1, tagmatcher)
	checkSampler.addSample(&mSample2, tagmatcher)
	checkSampler.addSample(&mSample3, tagmatcher)
	checkSampler.addSample(&mSample4, tagmatcher)
	checkSampler.addSample(&mSample5, tagmatcher)

	// Filter out two and four
	matcher := strings.NewMatcher([]string{"custom.distribution.two", "custom.distribution.four"}, false)
	checkSampler.commit(12346.0, &matcher)
	_, sketches := checkSampler.flush()

	// Check that only non-filtered sketches are present
	require.Equal(t, 3, len(sketches))
	sketchNames := make(map[string]bool)
	for _, sketch := range sketches {
		sketchNames[sketch.Name] = true
	}

	assert.True(t, sketchNames["custom.distribution.one"])
	assert.True(t, sketchNames["custom.distribution.three"])
	assert.True(t, sketchNames["custom.distribution.five"])

	assert.False(t, sketchNames["custom.distribution.two"])
	assert.False(t, sketchNames["custom.distribution.four"])
}

func TestFilteredSketches(t *testing.T) {
	testWithTagsStore(t, testFilteredSketches)
}
