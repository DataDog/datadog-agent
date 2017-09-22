// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package aggregator

import (
	"math"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/percentile"
)

func TestContextSketchSampling(t *testing.T) {
	ctxSketch := makeContextSketch()
	contextKey := "context_key"

	ctxSketch.addSample(contextKey, &metrics.MetricSample{Value: 1}, 1, 10)
	ctxSketch.addSample(contextKey, &metrics.MetricSample{Value: 5}, 3, 10)
	resultSeries := ctxSketch.flush(12345.0)

	expectedSketch := percentile.NewQSketch()
	expectedSketch = expectedSketch.Add(1)
	expectedSketch = expectedSketch.Add(5)
	expectedSeries := &percentile.SketchSeries{
		ContextKey: contextKey,
		Sketches:   []percentile.Sketch{{Timestamp: int64(12345), Sketch: expectedSketch}}}

	assert.Equal(t, 1, len(resultSeries))
	AssertSketchSeriesEqual(t, expectedSeries, resultSeries[0])

	// No sketches should be flushed when there's no new sample since
	// last flush
	resultSeries = ctxSketch.flush(12355.0)
	assert.Equal(t, 0, len(resultSeries))
}

// The sketches ignore sample values of +Inf/-Inf
func TestContextSketchSamplingInfinity(t *testing.T) {
	ctxSketch := makeContextSketch()
	contextKey := "context_key"

	ctxSketch.addSample(contextKey, &metrics.MetricSample{Value: math.Inf(1)}, 1, 10)
	ctxSketch.addSample(contextKey, &metrics.MetricSample{Value: math.Inf(-1)}, 2, 10)
	resultSeries := ctxSketch.flush(12345.0)

	assert.Equal(t, 0, len(resultSeries))
}

func TestContextSketchSamplingMultiContexts(t *testing.T) {
	ctxSketch := makeContextSketch()
	contextKey1 := "context_key1"
	contextKey2 := "context_key2"
	ctxSketch.addSample(contextKey1, &metrics.MetricSample{Value: 1}, 1, 10)
	ctxSketch.addSample(contextKey2, &metrics.MetricSample{Value: 1}, 1, 10)
	ctxSketch.addSample(contextKey1, &metrics.MetricSample{Value: 3}, 5, 10)
	orderedSketchSeries := OrderedSketchSeries{ctxSketch.flush(12345.0)}
	sort.Sort(orderedSketchSeries)

	expectedSketch1 := percentile.NewQSketch()
	expectedSketch1 = expectedSketch1.Add(1)
	expectedSketch1 = expectedSketch1.Add(3)
	expectedSeries1 := &percentile.SketchSeries{
		ContextKey: contextKey1,
		Sketches:   []percentile.Sketch{{Timestamp: int64(12345), Sketch: expectedSketch1}}}
	expectedSketch2 := percentile.NewQSketch()
	expectedSketch2 = expectedSketch2.Add(1)
	expectedSeries2 := &percentile.SketchSeries{
		ContextKey: contextKey2,
		Sketches:   []percentile.Sketch{{Timestamp: int64(12345), Sketch: expectedSketch2}}}

	assert.Equal(t, 2, orderedSketchSeries.Len())

	AssertSketchSeriesEqual(t, expectedSeries1, orderedSketchSeries.sketchSeries[0])
	AssertSketchSeriesEqual(t, expectedSeries2, orderedSketchSeries.sketchSeries[1])
}
