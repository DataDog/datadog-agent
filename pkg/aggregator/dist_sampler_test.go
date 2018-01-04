// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package aggregator

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/percentile"
)

// OrderedSketchSeries used to sort []*SketchSeries
type OrderedSketchSeries struct {
	sketchSeries []*percentile.SketchSeries
}

func (oss OrderedSketchSeries) Len() int {
	return len(oss.sketchSeries)
}

func (oss OrderedSketchSeries) Less(i, j int) bool {
	return ckey.Compare(oss.sketchSeries[i].ContextKey, oss.sketchSeries[j].ContextKey) == -1
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

	expectedSketch := percentile.NewGKArray()
	expectedSketch = expectedSketch.Add(1).(percentile.GKArray)
	expectedSketch = expectedSketch.Add(2).(percentile.GKArray)
	expectedSeries := &percentile.SketchSeries{
		Name:     "test.metric.name",
		Tags:     []string{"a", "b"},
		Interval: 10,
		Sketches: []percentile.Sketch{
			{Timestamp: 10000, Sketch: expectedSketch},
			{Timestamp: 10010, Sketch: expectedSketch},
		},
		ContextKey: generateContextKey(&mSample1),
	}
	assert.Equal(t, 1, len(sketchSeries))
	metrics.AssertSketchSeriesEqual(t, expectedSeries, sketchSeries[0])

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

	expectedSketch := percentile.NewGKArray()
	expectedSketch = expectedSketch.Add(1).(percentile.GKArray)
	expectedSeries1 := &percentile.SketchSeries{
		Name:     "test.metric.name1",
		Tags:     []string{"a", "b"},
		Interval: 10,
		Sketches: []percentile.Sketch{
			{Timestamp: 10010, Sketch: expectedSketch},
		},
		ContextKey: generateContextKey(&mSample1),
	}
	expectedSeries2 := &percentile.SketchSeries{
		Name:     "test.metric.name2",
		Tags:     []string{"a", "c"},
		Interval: 10,
		Sketches: []percentile.Sketch{
			{Timestamp: 10010, Sketch: expectedSketch},
		},
		ContextKey: generateContextKey(&mSample2),
	}

	assert.Equal(t, 2, len(sketchSeries))
	metrics.AssertSketchSeriesEqual(t, expectedSeries1, sketchSeries[1])
	metrics.AssertSketchSeriesEqual(t, expectedSeries2, sketchSeries[0])
}

func TestDistSamplerMultiSketchContextSampling(t *testing.T) {
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
		Mtype:      metrics.DistributionKType,
		Tags:       []string{"a", "c"},
		SampleRate: 1,
	}
	distSampler.addSample(&mSample1, 10011)
	distSampler.addSample(&mSample2, 10011)

	orderedSketchSeries := OrderedSketchSeries{distSampler.flush(10020.0)}
	sort.Sort(orderedSketchSeries)
	sketchSeries := orderedSketchSeries.sketchSeries

	expectedSketch1 := percentile.NewGKArray()
	expectedSketch1 = expectedSketch1.Add(1).(percentile.GKArray)
	expectedSeries1 := &percentile.SketchSeries{
		Name:     "test.metric.name1",
		Tags:     []string{"a", "b"},
		Interval: 10,
		Sketches: []percentile.Sketch{
			{Timestamp: 10010, Sketch: expectedSketch1},
		},
		ContextKey: generateContextKey(&mSample1),
		SketchType: percentile.SketchGK,
	}
	expectedSketch2 := percentile.NewKLL()
	expectedSketch2 = expectedSketch2.Add(1).(percentile.KLL)
	expectedSeries2 := &percentile.SketchSeries{
		Name:     "test.metric.name2",
		Tags:     []string{"a", "c"},
		Interval: 10,
		Sketches: []percentile.Sketch{
			{Timestamp: 10010, Sketch: expectedSketch2},
		},
		ContextKey: generateContextKey(&mSample2),
		SketchType: percentile.SketchKLL,
	}

	assert.Equal(t, 2, len(sketchSeries))
	metrics.AssertSketchSeriesEqual(t, expectedSeries1, sketchSeries[1])
	metrics.AssertSketchSeriesEqual(t, expectedSeries2, sketchSeries[0])
}

func TestDistSamplerWrongSketchType(t *testing.T) {
	distSampler := NewDistSampler(10, "")

	mSample1 := metrics.MetricSample{
		Name:       "test.metric.name1",
		Value:      1,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"a", "b"},
		SampleRate: 1,
	}
	mSample2 := metrics.MetricSample{
		Name:       "test.metric.name1",
		Value:      1,
		Mtype:      metrics.DistributionKType,
		Tags:       []string{"a", "b"},
		SampleRate: 1,
	}
	distSampler.addSample(&mSample1, 10011)
	distSampler.addSample(&mSample2, 10011)
	// Only the fist sketch is returned
	sketchSeries := distSampler.flush(10020)

	expectedSketch := percentile.NewGKArray()
	expectedSketch = expectedSketch.Add(1).(percentile.GKArray)
	expectedSeries := &percentile.SketchSeries{
		Name:     "test.metric.name1",
		Tags:     []string{"a", "b"},
		Interval: 10,
		Sketches: []percentile.Sketch{
			{Timestamp: 10010, Sketch: expectedSketch},
		},
		ContextKey: generateContextKey(&mSample1),
		SketchType: percentile.SketchGK,
	}
	assert.Equal(t, 1, len(sketchSeries))
	metrics.AssertSketchSeriesEqual(t, expectedSeries, sketchSeries[0])

}
