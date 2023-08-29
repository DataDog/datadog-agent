// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/aggregator/internal/tags"
	"github.com/DataDog/datadog-agent/pkg/tagset"

	// stdlib
	"math"
	"testing"
	"time"

	// 3p
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/quantile"
)

func generateContextKey(sample metrics.MetricSampleContext) ckey.ContextKey {
	k := ckey.NewKeyGenerator()
	tb := tagset.NewHashingTagsAccumulator()
	sample.GetTags(tb, tb)
	return k.Generate(sample.GetName(), sample.GetHost(), tb)
}

func testCheckGaugeSampling(t *testing.T, store *tags.Store) {
	checkSampler := newCheckSampler(1, true, 1*time.Second, store)

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

	checkSampler.addSample(&mSample1)
	checkSampler.addSample(&mSample2)
	checkSampler.addSample(&mSample3)

	checkSampler.commit(12349.0)
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
	checkSampler := newCheckSampler(1, true, 1*time.Second, store)

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

	checkSampler.addSample(&mSample1)
	checkSampler.addSample(&mSample2)
	checkSampler.addSample(&mSample3)

	checkSampler.commit(12349.0)
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
	checkSampler := newCheckSampler(1, true, 1*time.Second, store)

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

	checkSampler.addSample(&mSample1)
	checkSampler.addSample(&mSample2)
	checkSampler.addSample(&mSample3)

	checkSampler.commit(12349.0)
	require.Len(t, checkSampler.contextResolver.expireCountByKey, 1)
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
	checkSampler.commit(12349.0)
	require.Len(t, checkSampler.contextResolver.expireCountByKey, 0)
}
func TestHistogramCountSampling(t *testing.T) {
	testWithTagsStore(t, testHistogramCountSampling)
}

func testCheckHistogramBucketSampling(t *testing.T, store *tags.Store) {
	checkSampler := newCheckSampler(1, true, 1*time.Second, store)

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
	checkSampler.addBucket(bucket1)
	assert.Equal(t, len(checkSampler.lastBucketValue), 1)

	checkSampler.commit(12349.0)
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
	checkSampler.addBucket(bucket2)
	assert.Equal(t, len(checkSampler.lastBucketValue), 1)

	checkSampler.commit(12401.0)
	assert.Len(t, checkSampler.lastBucketValue, 1)
	checkSampler.commit(12401.0)
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
	checkSampler := newCheckSampler(1, true, 1*time.Second, store)

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
	checkSampler.addBucket(bucket1)
	assert.Equal(t, len(checkSampler.lastBucketValue), 1)

	checkSampler.commit(12349.0)
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
	checkSampler.addBucket(bucket2)
	assert.Equal(t, len(checkSampler.lastBucketValue), 1)

	checkSampler.commit(12401.0)
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

func testCheckHistogramBucketInfinityBucket(t *testing.T, store *tags.Store) {
	checkSampler := newCheckSampler(1, true, 1*time.Second, store)

	bucket1 := &metrics.HistogramBucket{
		Name:       "my.histogram",
		Value:      4.0,
		LowerBound: 9000.0,
		UpperBound: math.Inf(1),
		Tags:       []string{"foo", "bar"},
		Timestamp:  12345.0,
	}
	checkSampler.addBucket(bucket1)

	checkSampler.commit(12349.0)
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
	checkSampler := newCheckSampler(1, true, 1*time.Second, store)

	mSample1 := metrics.MetricSample{
		Name:       "my.metric.name",
		Value:      1,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"foo", "bar"},
		SampleRate: 1,
		Timestamp:  12345.0,
	}

	checkSampler.addSample(&mSample1)
	checkSampler.commit(12349.0)

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
