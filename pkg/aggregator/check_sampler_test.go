// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"math"
	"testing"
	"testing/synctest"
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
	"github.com/DataDog/datadog-agent/pkg/util/metricname"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
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
	matcher := metricname.NewMatcher([]string{}, false)
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

	matcher := metricname.NewMatcher([]string{}, false)
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
	matcher := metricname.NewMatcher([]string{}, false)

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
	matcher := metricname.NewMatcher([]string{}, false)

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
		DistributionMetadata: metrics.DistributionMetadata{
			Name: "my.histogram",
			Tags: tagset.CompositeTagsFromSlice([]string{"foo", "bar"}),
		},
		Points: []metrics.SketchPoint{
			{Ts: 12345.0, Sketch: expSketch},
		},
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
		DistributionMetadata: metrics.DistributionMetadata{
			Name: "my.histogram",
			Tags: tagset.CompositeTagsFromSlice([]string{"foo", "bar"}),
		},
		Points: []metrics.SketchPoint{
			{Ts: 12400.0, Sketch: expSketch},
		},
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

	matcher := metricname.NewMatcher([]string{}, false)
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
		DistributionMetadata: metrics.DistributionMetadata{
			Name: "my.histogram",
			Tags: tagset.CompositeTagsFromSlice([]string{"foo", "bar"}),
		},
		Points: []metrics.SketchPoint{
			{Ts: 12400.0, Sketch: expSketch},
		},
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
		DistributionMetadata: metrics.DistributionMetadata{
			Name: "my.histogram",
		},
		Points: []metrics.SketchPoint{
			{Ts: 12410, Sketch: sketchOf(10, 20, 3)},
		},
	}, flushed[0], 0.01)

	metrics.AssertSketchSeriesApproxEqual(t, &metrics.SketchSeries{
		DistributionMetadata: metrics.DistributionMetadata{
			Name: "my.histogram",
		},
		Points: []metrics.SketchPoint{
			{Ts: 12420, Sketch: sketchOf(10, 20, 2)},
		},
	}, flushed[1], 0.01)
}

func TestCheckHistogramBucketReset(t *testing.T) {
	testWithTagsStore(t, testCheckHistogramBucketReset)
}

func testCheckHistogramBucketMultipleBucketsSampling(t *testing.T, store *tags.Store) {
	taggerComponent := nooptagger.NewComponent()
	checkSampler := newCheckSampler(1, true, true, 1*time.Second, true, store, checkid.ID("hello:world:1234"), taggerComponent)

	tagmatcher := filterlistimpl.NewNoopTagMatcher()
	matcher := metricname.NewMatcher([]string{}, false)

	// Two buckets with the same name and tags (so they share a context key)
	// but different bounds. MultipleBuckets: true forces addBucket to track
	// last-seen monotonic values per (contextKey, bounds) pair.
	bucket1 := &metrics.HistogramBucket{
		Name:            "my.histogram",
		Value:           4,
		LowerBound:      10.0,
		UpperBound:      20.0,
		Tags:            []string{"foo", "bar"},
		Timestamp:       12345.0,
		Monotonic:       true,
		FlushFirstValue: true,
		MultipleBuckets: true,
	}
	bucket2 := &metrics.HistogramBucket{
		Name:            "my.histogram",
		Value:           6,
		LowerBound:      30.0,
		UpperBound:      40.0,
		Tags:            []string{"foo", "bar"},
		Timestamp:       12345.0,
		Monotonic:       true,
		FlushFirstValue: true,
		MultipleBuckets: true,
	}

	checkSampler.addBucket(bucket1, tagmatcher)
	checkSampler.addBucket(bucket2, tagmatcher)

	ctx := generateContextKey(bucket1)
	require.Equal(t, ctx, generateContextKey(bucket2))

	// MultipleBuckets path must populate lastBucketValueByBound and leave
	// the simple lastBucketValue map untouched.
	assert.Len(t, checkSampler.lastBucketValue, 0)
	require.Len(t, checkSampler.lastBucketValueByBound, 1)
	assert.Len(t, checkSampler.lastBucketValueByBound[ctx], 2)

	checkSampler.commit(12349.0, &matcher)
	_, flushed := checkSampler.flush()
	require.Len(t, flushed, 1)

	expSketch := &quantile.Agent{}
	expSketch.InsertInterpolate(10.0, 20.0, 4)
	expSketch.InsertInterpolate(30.0, 40.0, 6)
	metrics.AssertSketchSeriesApproxEqual(t, &metrics.SketchSeries{
		DistributionMetadata: metrics.DistributionMetadata{
			Name: "my.histogram",
			Tags: tagset.CompositeTagsFromSlice([]string{"foo", "bar"}),
		},
		Points: []metrics.SketchPoint{
			{Ts: 12345.0, Sketch: expSketch.Finish()},
		},
	}, flushed[0], .03)

	// Second round: same bounds, larger raw values. Per-bound de-cumulation
	// should produce delta sketches (3 and 5), not the raw new values.
	bucket3 := &metrics.HistogramBucket{
		Name:            "my.histogram",
		Value:           7,
		LowerBound:      10.0,
		UpperBound:      20.0,
		Tags:            []string{"foo", "bar"},
		Timestamp:       12400.0,
		Monotonic:       true,
		MultipleBuckets: true,
	}
	bucket4 := &metrics.HistogramBucket{
		Name:            "my.histogram",
		Value:           11,
		LowerBound:      30.0,
		UpperBound:      40.0,
		Tags:            []string{"foo", "bar"},
		Timestamp:       12400.0,
		Monotonic:       true,
		MultipleBuckets: true,
	}

	checkSampler.addBucket(bucket3, tagmatcher)
	checkSampler.addBucket(bucket4, tagmatcher)

	checkSampler.commit(12401.0, &matcher)
	_, flushed = checkSampler.flush()
	require.Len(t, flushed, 1)

	expSketch = &quantile.Agent{}
	expSketch.InsertInterpolate(10.0, 20.0, 3)
	expSketch.InsertInterpolate(30.0, 40.0, 5)
	metrics.AssertSketchSeriesApproxEqual(t, &metrics.SketchSeries{
		DistributionMetadata: metrics.DistributionMetadata{
			Name: "my.histogram",
			Tags: tagset.CompositeTagsFromSlice([]string{"foo", "bar"}),
		},
		Points: []metrics.SketchPoint{
			{Ts: 12400.0, Sketch: expSketch.Finish()},
		},
	}, flushed[0], .03)

	// One more commit without further adds expires the context (sampler was
	// constructed with expirationCount=1) and triggers GC of the new map.
	checkSampler.commit(12402.0, &matcher)
	assert.Len(t, checkSampler.lastBucketValueByBound, 0)
}

func TestCheckHistogramBucketMultipleBucketsSampling(t *testing.T) {
	testWithTagsStore(t, testCheckHistogramBucketMultipleBucketsSampling)
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
	matcher := metricname.NewMatcher([]string{}, false)

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
		DistributionMetadata: metrics.DistributionMetadata{
			Name: "my.histogram",
			Tags: tagset.CompositeTagsFromSlice([]string{"foo", "bar"}),
		},
		Points: []metrics.SketchPoint{
			{Ts: 12345.0, Sketch: expSketch},
		},
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
	matcher := metricname.NewMatcher([]string{}, false)
	checkSampler.commit(12349.0, &matcher)

	_, sketches := checkSampler.flush()

	expSketch := &quantile.Sketch{}
	expSketch.Insert(quantile.Default(), 1)

	metrics.AssertSketchSeriesEqual(t, &metrics.SketchSeries{
		DistributionMetadata: metrics.DistributionMetadata{
			Name: "my.metric.name",
			Tags: tagset.CompositeTagsFromSlice([]string{"foo", "bar"}),
		},
		Points: []metrics.SketchPoint{
			{Ts: 12345.0, Sketch: expSketch},
		},
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
	matcher := metricname.NewMatcher([]string{"custom.metric.two", "custom.metric.four"}, false)
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
	matcher := metricname.NewMatcher([]string{"custom.distribution.two", "custom.distribution.four"}, false)
	checkSampler.commit(12346.0, &matcher)
	_, sketches := checkSampler.flush()

	// Check that only non-filtered sketches are present
	require.Equal(t, 3, len(sketches))
	sketchNames := make(map[string]bool)
	for _, sketch := range sketches {
		sketchNames[sketch.GetName()] = true
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

func testNewSketchSeriesWithMissingContext(t *testing.T, store *tags.Store) {
	taggerComponent := nooptagger.NewComponent()
	checkSampler := newCheckSampler(1, true, true, 1*time.Second, true, store, checkid.ID("hello:world:1234"), taggerComponent)

	// Generate a context key that is NOT registered in the context resolver.
	k := ckey.NewKeyGenerator()
	tb := tagset.NewHashingTagsAccumulator()
	orphanKey := k.Generate("orphan.metric", "orphanhost", tb)

	// Insert a sketch point directly into sketchMap, bypassing trackContext.
	// This simulates an inconsistent state where the sketch map references a
	// context key not present in the context resolver.
	checkSampler.sketchMap.insert(12345, orphanKey, 1.0, 1.0)

	// Before the fix, commitSketches would call newSketchSeries which would
	// dereference a nil *Context, causing a panic. After the fix, it logs
	// an error and skips the sketch.
	matcher := metricname.NewMatcher([]string{}, false)
	require.NotPanics(t, func() {
		checkSampler.commit(12349.0, &matcher)
	})

	// The orphan sketch should be silently dropped.
	_, sketches := checkSampler.flush()
	assert.Empty(t, sketches)
}

func TestNewSketchSeriesWithMissingContext(t *testing.T) {
	testWithTagsStore(t, testNewSketchSeriesWithMissingContext)
}

func testSketchSeriesSourcePreserved(t *testing.T, store *tags.Store) {
	taggerComponent := nooptagger.NewComponent()
	checkSampler := newCheckSampler(1, true, true, 1*time.Second, true, store, checkid.ID("hello:world:1234"), taggerComponent)

	tagmatcher := filterlistimpl.NewNoopTagMatcher()

	mSample := metrics.MetricSample{
		Name:       "my.distribution",
		Value:      1,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12345.0,
		Source:     metrics.MetricSourceJmxCustom,
	}

	checkSampler.addSample(&mSample, tagmatcher)
	matcher := metricname.NewMatcher([]string{}, false)
	checkSampler.commit(12349.0, &matcher)
	_, sketches := checkSampler.flush()

	require.Len(t, sketches, 1)
	assert.Equal(t, metrics.MetricSourceJmxCustom, sketches[0].Source,
		"SketchSeries must carry the MetricSource set on the original sample")
}

func TestSketchSeriesSourcePreserved(t *testing.T) {
	testWithTagsStore(t, testSketchSeriesSourcePreserved)
}

// Shared contract tests for the frozen PR and literal periodic prototype.
// Only compareIntermediate differs: it invokes each revision's actual sampler boundary.
func comparisonSampler(t *testing.T) *CheckSampler {
	t.Helper()
	cs := newCheckSampler(2, true, true, time.Hour, false, tags.NewStore(true, "comparison"), checkid.ID("comparison"), nooptagger.NewComponent())
	comparisonConfigure(cs)
	t.Cleanup(cs.release)
	return cs
}
func comparisonAdd(cs *CheckSampler, kind metrics.MetricType, value, ts float64, first bool) {
	cs.addSample(&metrics.MetricSample{Name: "comparison.metric", Mtype: kind, Value: value, Timestamp: ts, SampleRate: 1, FlushFirstValue: first}, filterlistimpl.NewNoopTagMatcher())
}
func comparisonPoints(cs *CheckSampler, suffix string) []float64 {
	series, _ := cs.flush()
	var values []float64
	for _, s := range series {
		if s.Name == "comparison.metric"+suffix {
			for _, p := range s.Points {
				values = append(values, p.Value)
			}
		}
	}
	return values
}
func TestComparisonHistorateContinuity(t *testing.T) {
	cs := comparisonSampler(t)
	comparisonAdd(cs, metrics.HistorateType, 100, 1, false)
	comparisonAdd(cs, metrics.HistorateType, 120, 2, false)
	comparisonIntermediate(cs, 100)
	comparisonAdd(cs, metrics.HistorateType, 150, 3, false)
	cs.commit(101, nil)
	require.Equal(t, []float64{25}, comparisonPoints(cs, ".avg"), "historate must retain its complete check-run distribution")
}
func TestComparisonCountConservation(t *testing.T) {
	cs := comparisonSampler(t)
	comparisonAdd(cs, metrics.CountType, 3, 1, false)
	comparisonAdd(cs, metrics.CountType, 7, 2, false)
	comparisonIntermediate(cs, 100)
	comparisonAdd(cs, metrics.CountType, 5, 3, false)
	cs.commit(101, nil)
	require.Equal(t, []float64{10, 5}, comparisonPoints(cs, ""))
}
func TestComparisonRateContinuity(t *testing.T) {
	cs := comparisonSampler(t)
	comparisonAdd(cs, metrics.RateType, 100, 1, false)
	comparisonAdd(cs, metrics.RateType, 120, 2, false)
	comparisonIntermediate(cs, 100)
	comparisonAdd(cs, metrics.RateType, 150, 3, false)
	cs.commit(101, nil)
	require.Equal(t, []float64{30}, comparisonPoints(cs, ""), "rate keeps the original last-two-samples check-run contract")
}
func TestComparisonMonotonicResetConservation(t *testing.T) {
	cs := comparisonSampler(t)
	comparisonAdd(cs, metrics.MonotonicCountType, 100, 1, false)
	comparisonIntermediate(cs, 100)
	comparisonAdd(cs, metrics.MonotonicCountType, 110, 2, true)
	comparisonAdd(cs, metrics.MonotonicCountType, 5, 3, true)
	comparisonAdd(cs, metrics.MonotonicCountType, 15, 4, true)
	cs.commit(101, nil)
	require.Equal(t, []float64{25}, comparisonPoints(cs, ""), "10 before reset + 5 reset value + 10 after reset")
}
func TestComparisonGaugeFinalValue(t *testing.T) {
	cs := comparisonSampler(t)
	comparisonAdd(cs, metrics.GaugeType, 1, 1, false)
	comparisonAdd(cs, metrics.GaugeType, 2, 2, false)
	comparisonIntermediate(cs, 100)
	comparisonAdd(cs, metrics.GaugeType, 3, 3, false)
	cs.commit(101, nil)
	require.Equal(t, []float64{2, 3}, comparisonPoints(cs, ""))
}

func comparisonConfigure(_ *CheckSampler)                 {}
func comparisonIntermediate(cs *CheckSampler, ts float64) { cs.commitIntermediate(ts, nil) }

func TestComparisonHistogramConservation(t *testing.T) {
	cs := comparisonSampler(t)
	comparisonAdd(cs, metrics.HistogramType, 10, 1, false)
	comparisonAdd(cs, metrics.HistogramType, 20, 2, false)
	comparisonIntermediate(cs, 100)
	comparisonAdd(cs, metrics.HistogramType, 60, 3, false)
	cs.commit(101, nil)
	series, _ := cs.flush()
	counts := map[float64]float64{}
	averages := map[float64]float64{}
	for _, s := range series {
		for _, p := range s.Points {
			if s.Name == "comparison.metric.count" {
				counts[p.Ts] = p.Value
			}
			if s.Name == "comparison.metric.avg" {
				averages[p.Ts] = p.Value
			}
		}
	}
	var count, sum float64
	for ts, n := range counts {
		count += n
		sum += n * averages[ts]
	}
	require.Equal(t, float64(3), count)
	require.Equal(t, float64(90), sum)
}
func TestComparisonDistributionConservation(t *testing.T) {
	cs := comparisonSampler(t)
	cs.addBucket(&metrics.HistogramBucket{Name: "comparison.bucket", Value: 7, LowerBound: 1, UpperBound: 1, Timestamp: 1}, filterlistimpl.NewNoopTagMatcher())
	comparisonIntermediate(cs, 100)
	cs.addBucket(&metrics.HistogramBucket{Name: "comparison.bucket", Value: 11, LowerBound: 1, UpperBound: 1, Timestamp: 2}, filterlistimpl.NewNoopTagMatcher())
	cs.commit(101, nil)
	_, sketches := cs.flush()
	var count int64
	for _, s := range sketches {
		for _, p := range s.Points {
			count += p.Sketch.Basic.Cnt
		}
	}
	require.Equal(t, int64(18), count)
}
func TestComparisonBucketBaselineAcrossIntermediateExpiry(t *testing.T) {
	for _, multiple := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "multiple"}[multiple], func(t *testing.T) {
			cs := comparisonSampler(t)
			bucket := func(v int64, ts float64) {
				cs.addBucket(&metrics.HistogramBucket{Name: "comparison.bucket", Value: v, LowerBound: 1, UpperBound: 2, Timestamp: ts, Monotonic: true, MultipleBuckets: multiple}, filterlistimpl.NewNoopTagMatcher())
			}
			bucket(100, 1)
			for ts := 100.; ts < 104; ts++ {
				comparisonIntermediate(cs, ts)
			}
			bucket(125, 104)
			cs.commit(105, nil)
			_, sketches := cs.flush()
			var count int64
			for _, s := range sketches {
				for _, p := range s.Points {
					count += p.Sketch.Basic.Cnt
				}
			}
			require.Equal(t, int64(25), count, "periodic context expiry within one run must not forget a bucket baseline")
		})
	}
}

func TestCorrectedPeriodicFinalTimestampCollision(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cs := comparisonSampler(t)
		comparisonAdd(cs, metrics.CountType, 7, 1, false)
		cs.commitNow(true, nil)
		first := cs.lastCommitSecond
		comparisonAdd(cs, metrics.CountType, 11, 2, false)
		cs.commitNow(false, nil)
		require.Equal(t, first, cs.lastCommitSecond)
		cs.commitAt(float64(first+15), true, false, nil)
		series, _ := cs.flush()
		points := map[int64]float64{}
		var total float64
		for _, s := range series {
			for _, p := range s.Points {
				require.NotContains(t, points, int64(p.Ts))
				points[int64(p.Ts)] = p.Value
				total += p.Value
			}
		}
		require.Equal(t, float64(18), total)
		require.Greater(t, cs.lastCommitSecond, first)
	})
}

func TestPeriodicReclaimsFlushedGaugeContexts(t *testing.T) {
	cs := comparisonSampler(t)
	add := func(value float64) {
		cs.addSample(&metrics.MetricSample{Name: "reclaimed.gauge", Mtype: metrics.GaugeType, Value: value, Timestamp: 1, SampleRate: 1, Tags: []string{"stable:tag"}}, filterlistimpl.NewNoopTagMatcher())
	}
	add(7)
	cs.commitIntermediate(100, nil)
	require.Zero(t, cs.contextResolver.length())
	series, _ := cs.flush()
	require.Len(t, series, 1)
	require.Equal(t, "reclaimed.gauge", series[0].Name)
	require.Equal(t, float64(7), series[0].Points[0].Value)
	require.Equal(t, "stable:tag", series[0].Tags.Join(","))
	add(11)
	cs.commitIntermediate(101, nil)
	series, _ = cs.flush()
	require.Len(t, series, 1)
	require.Equal(t, float64(11), series[0].Points[0].Value)
	require.Equal(t, "stable:tag", series[0].Tags.Join(","))
}

func TestPeriodicClockAndDeferredCommit(t *testing.T) {
	for _, kind := range []metrics.MetricType{metrics.CountType, metrics.MonotonicCountType, metrics.GaugeType} {
		t.Run(kind.String(), func(t *testing.T) {
			cs := comparisonSampler(t)
			comparisonAdd(cs, kind, 7, 1, true)
			cs.commitAt(100, true, false, nil)
			series, _ := cs.flush()
			require.Len(t, series, 1)
			require.Equal(t, float64(7), series[0].Points[0].Value)
			comparisonAdd(cs, kind, 11, 2, true)
			// Many same-second check completions must neither wait nor expire
			// accepted contributions. Histogram windows still close each run.
			for i := 0; i < 10; i++ {
				cs.commitAt(100.5, false, false, nil)
			}
			require.Empty(t, cs.series)
			require.Equal(t, int64(100), cs.lastCommitSecond)
			cs.commitAt(115, true, false, nil)
			series, _ = cs.flush()
			require.Len(t, series, 1)
			want := 11.0
			if kind == metrics.MonotonicCountType {
				want = 4
			}
			require.Equal(t, want, series[0].Points[0].Value)
			require.Equal(t, float64(115), series[0].Points[0].Ts)
			comparisonAdd(cs, kind, 13, 3, true)
			// Rollback does not block the input loop or retain unbounded output.
			cs.commitAt(1, true, false, nil)
			series, _ = cs.flush()
			require.Len(t, series, 1)
			require.Equal(t, float64(116), series[0].Points[0].Ts)
			comparisonAdd(cs, kind, 17, 4, true)
			cs.commitAt(1, true, true, nil)
			series, _ = cs.flush()
			require.Len(t, series, 1)
			require.Equal(t, float64(117), series[0].Points[0].Ts)
		})
	}
}

func TestPeriodicPreservesHistogramQueries(t *testing.T) {
	cs := comparisonSampler(t)
	for i := 0; i < 99; i++ {
		comparisonAdd(cs, metrics.HistogramType, 0, float64(i+1), false)
	}
	cs.commitAt(100, true, false, nil)
	require.Empty(t, cs.series)
	comparisonAdd(cs, metrics.HistogramType, 100, 101, false)
	// Even a colliding final commit must close the full histogram window.
	cs.commitAt(100.5, false, false, nil)
	series, _ := cs.flush()
	values := map[string]float64{}
	for _, s := range series {
		values[s.Name] = s.Points[0].Value
	}
	require.Equal(t, float64(1), values["comparison.metric.avg"])
	require.Equal(t, float64(100), values["comparison.metric.count"])
	require.Contains(t, values, "comparison.metric.95percentile")
	require.Zero(t, values["comparison.metric.95percentile"])
	comparisonAdd(cs, metrics.HistogramType, 7, 102, false)
	cs.commitAt(100.6, false, false, nil)
	require.Equal(t, []float64{7}, comparisonPoints(cs, ".avg"), "next run must have a fresh window")
}

func TestPeriodicReclaimsMonotonicMetadataWithoutAgingBaseline(t *testing.T) {
	cs := comparisonSampler(t)
	comparisonAdd(cs, metrics.MonotonicCountType, 100, 1, false)
	cs.commitIntermediate(100, nil)
	require.Zero(t, cs.contextResolver.length())
	require.Len(t, cs.retiredBaselines, 1)
	// Several hours of one run must not start the stateful expiry timeout.
	cs.commitIntermediate(10000, nil)
	comparisonAdd(cs, metrics.MonotonicCountType, 125, 2, false)
	require.Empty(t, cs.retiredBaselines)
	cs.commitIntermediate(10001, nil)
	require.Zero(t, cs.contextResolver.length())
	require.Equal(t, []float64{25}, comparisonPoints(cs, ""))
	// The usual two completed-run context grace period still applies.
	cs.commit(10002, nil)
	cs.commit(10003, nil)
	require.Len(t, cs.retiredBaselines, 1)
	cs.commit(10004, nil)
	require.Empty(t, cs.retiredBaselines)
	// Once the additional stateful timeout passes, a new baseline is needed.
	cs.commit(14000, nil)
	comparisonAdd(cs, metrics.MonotonicCountType, 150, 3, false)
	cs.commit(14001, nil)
	require.Empty(t, comparisonPoints(cs, ""))
}

func TestPeriodicExplicitTimestampsSurviveClockPolicy(t *testing.T) {
	cs := comparisonSampler(t)
	comparisonAdd(cs, metrics.CountWithTimestampType, 7, 42, false)
	cs.commitAt(100, true, false, nil)
	comparisonAdd(cs, metrics.CountWithTimestampType, 11, 43, false)
	cs.commitAt(1, true, true, nil)
	series, _ := cs.flush()
	require.Len(t, series, 2)
	require.Equal(t, float64(42), series[0].Points[0].Ts)
	require.Equal(t, float64(43), series[1].Points[0].Ts)
}

func TestPeriodicPendingSketchPinsMetadataUntilFlushed(t *testing.T) {
	cs := comparisonSampler(t)
	comparisonAdd(cs, metrics.DistributionType, 7, 200, false)
	// Model a wall-clock rollback: several runs finish while the old bucket
	// remains ahead of wall time. Run-based expiry must not orphan its sketch.
	for ts := 100.0; ts < 105; ts++ {
		cs.commitAt(ts, false, false, nil)
		require.Equal(t, 1, cs.contextResolver.length())
	}
	cs.commitAt(201, false, false, nil)
	_, sketches := cs.flush()
	require.Len(t, sketches, 1)
	require.Equal(t, int64(1), sketches[0].Points[0].Sketch.Basic.Cnt)
	require.Equal(t, int64(200), sketches[0].Points[0].Ts)
	// Pinning metadata must not reset its original completed-run age.
	require.Zero(t, cs.contextResolver.length())
}
