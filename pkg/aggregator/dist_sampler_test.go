// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package aggregator

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/percentile"
)

func AssertSketchSeriesEqual(t *testing.T, expected, actual *percentile.SketchSeries) {
	assert.Equal(t, expected.Name, actual.Name)
	if expected.Tags != nil {
		assert.NotNil(t, actual.Tags)
		metrics.AssertTagsEqual(t, expected.Tags, actual.Tags)
	}
	assert.Equal(t, expected.Host, actual.Host)
	assert.Equal(t, expected.Interval, actual.Interval)
	if expected.ContextKey != "" {
		assert.Equal(t, expected.ContextKey, actual.ContextKey)
	}
	if expected.Sketches != nil {
		assert.NotNil(t, actual.Sketches)
		AssertSketchesEqual(t, expected.Sketches, actual.Sketches)
	}
}

func AssertSketchesEqual(t *testing.T, expected, actual []percentile.Sketch) {
	if assert.Equal(t, len(expected), len(actual)) {
		actualOrdered := OrderedSketches{actual}
		sort.Sort(actualOrdered)
		for i, sketch := range expected {
			assert.Equal(t, sketch, actualOrdered.sketches[i])
		}
	}
}

// OrderedSketches used to sort []Sketch
type OrderedSketches struct {
	sketches []percentile.Sketch
}

func (os OrderedSketches) Len() int {
	return len(os.sketches)
}

func (os OrderedSketches) Less(i, j int) bool {
	return os.sketches[i].Timestamp < os.sketches[j].Timestamp
}

func (os OrderedSketches) Swap(i, j int) {
	os.sketches[i], os.sketches[j] = os.sketches[j], os.sketches[i]
}

// OrderedSketchSeries used to sort []*SketchSeries
type OrderedSketchSeries struct {
	sketchSeries []*percentile.SketchSeries
}

func (oss OrderedSketchSeries) Len() int {
	return len(oss.sketchSeries)
}

func (oss OrderedSketchSeries) Less(i, j int) bool {
	return oss.sketchSeries[i].ContextKey < oss.sketchSeries[j].ContextKey
}

func (oss OrderedSketchSeries) Swap(i, j int) {
	oss.sketchSeries[i], oss.sketchSeries[j] = oss.sketchSeries[j], oss.sketchSeries[i]
}

func TestDistSamplerBucketSampling(t *testing.T) {
	distSampler := NewDistSampler(10, "")

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
	distSampler.addSample(&mSample1, 10001)
	distSampler.addSample(&mSample2, 10002)
	distSampler.addSample(&mSample1, 10011)
	distSampler.addSample(&mSample2, 10012)
	distSampler.addSample(&mSample1, 10021)

	sketchSeries := distSampler.flush(10020.0)

	expectedSketch := percentile.NewQSketch()
	expectedSketch = expectedSketch.Add(1)
	expectedSketch = expectedSketch.Add(2)
	expectedSeries := &percentile.SketchSeries{
		Name:     "test.metric.name",
		Tags:     []string{"a", "b"},
		Interval: 10,
		Sketches: []percentile.Sketch{
			percentile.Sketch{Timestamp: int64(10000), Sketch: expectedSketch},
			percentile.Sketch{Timestamp: int64(10010), Sketch: expectedSketch},
		},
		ContextKey: "test.metric.name,a,b,",
	}
	assert.Equal(t, 1, len(sketchSeries))
	AssertSketchSeriesEqual(t, expectedSeries, sketchSeries[0])

	// The samples added after the flush time remains in the dist sampler
	assert.Equal(t, 1, len(distSampler.sketchesByTimestamp))
}

func TestDistSamplerContextSampling(t *testing.T) {
	distSampler := NewDistSampler(10, "")

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
	distSampler.addSample(&mSample1, 10011)
	distSampler.addSample(&mSample2, 10011)

	orderedSketchSeries := OrderedSketchSeries{distSampler.flush(10020.0)}
	sort.Sort(orderedSketchSeries)
	sketchSeries := orderedSketchSeries.sketchSeries

	expectedSketch := percentile.NewQSketch()
	expectedSketch = expectedSketch.Add(1)
	expectedSeries1 := &percentile.SketchSeries{
		Name:     "test.metric.name1",
		Tags:     []string{"a", "b"},
		Interval: 10,
		Sketches: []percentile.Sketch{
			percentile.Sketch{Timestamp: int64(10010), Sketch: expectedSketch},
		},
		ContextKey: "test.metric.name1,a,b,",
	}
	expectedSeries2 := &percentile.SketchSeries{
		Name:     "test.metric.name2",
		Tags:     []string{"a", "c"},
		Interval: 10,
		Sketches: []percentile.Sketch{
			percentile.Sketch{Timestamp: int64(10010), Sketch: expectedSketch},
		},
		ContextKey: "test.metric.name2,a,c,",
	}

	assert.Equal(t, 2, len(sketchSeries))
	AssertSketchSeriesEqual(t, expectedSeries1, sketchSeries[0])
	AssertSketchSeriesEqual(t, expectedSeries2, sketchSeries[1])
}
