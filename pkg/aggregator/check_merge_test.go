// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"cmp"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

func filteredCounter(key ckey.ContextKey, points ...metrics.Point) *metrics.Serie {
	return &metrics.Serie{Name: "my.counter", ContextKey: key, MType: metrics.APIRateType, Points: points}
}

func filteredSketch(key ckey.ContextKey, ts int64, values ...float64) mergedSketch {
	sketch := &quantile.Sketch{}
	sketch.Insert(quantile.Default(), values...)
	return mergedSketch{key: key, sketch: &metrics.SketchSeries{
		DistributionMetadata: metrics.DistributionMetadata{Name: "my.dist"},
		Points:               []metrics.SketchPoint{{Sketch: sketch, Ts: ts}},
	}}
}

// sentPoints returns the points sent for each series, by second.
func sentPoints(series metrics.Series) map[ckey.ContextKey][]metrics.Point {
	byKey := map[ckey.ContextKey][]metrics.Point{}
	for _, s := range series {
		byKey[s.ContextKey] = append(byKey[s.ContextKey], s.Points...)
	}
	for _, points := range byKey {
		slices.SortFunc(points, func(a, b metrics.Point) int { return cmp.Compare(a.Ts, b.Ts) })
	}
	return byKey
}

func TestCheckOutputMergerSumsCountersInTheSameSecond(t *testing.T) {
	m := newCheckOutputMerger()
	var series metrics.Series

	m.mergeSeries([]*metrics.Serie{filteredCounter(1, metrics.Point{Ts: 100.2, Value: 1})})
	m.mergeSeries([]*metrics.Serie{filteredCounter(1, metrics.Point{Ts: 100.7, Value: 2}, metrics.Point{Ts: 101.1, Value: 4})})
	m.mergeSeries([]*metrics.Serie{filteredCounter(2, metrics.Point{Ts: 100.5, Value: 8})})
	m.flush(200, &series, nil)

	require.Len(t, series, 3, "a series per second")
	byKey := sentPoints(series)
	assert.Equal(t, []metrics.Point{{Ts: 100, Value: 3}, {Ts: 101, Value: 4}}, byKey[1])
	assert.Equal(t, []metrics.Point{{Ts: 100, Value: 8}}, byKey[2])
}

func TestCheckOutputMergerMergesSketchesInTheSameSecond(t *testing.T) {
	m := newCheckOutputMerger()
	var sketches metrics.SketchSeriesList

	m.mergeSketches([]mergedSketch{filteredSketch(1, 100, 1, 2)})
	m.mergeSketches([]mergedSketch{filteredSketch(1, 100, 3), filteredSketch(1, 101, 4)})
	m.flush(200, nil, &sketches)

	require.Len(t, sketches, 2, "a sketch series per second")
	counts := map[int64]int64{}
	for _, s := range sketches {
		require.Len(t, s.Points, 1)
		counts[s.Points[0].Ts] += s.Points[0].Sketch.Basic.Cnt
	}
	assert.Equal(t, map[int64]int64{100: 3, 101: 1}, counts)
}

func TestCheckOutputMergerHoldsTheFlushSecond(t *testing.T) {
	m := newCheckOutputMerger()
	var series metrics.Series
	var sketches metrics.SketchSeriesList

	m.mergeSeries([]*metrics.Serie{filteredCounter(1, metrics.Point{Ts: 99.9, Value: 1}, metrics.Point{Ts: 100.1, Value: 2})})
	m.mergeSketches([]mergedSketch{filteredSketch(1, 100, 1)})
	m.flush(100, &series, &sketches)

	require.Len(t, series, 1)
	assert.Equal(t, []metrics.Point{{Ts: 99, Value: 1}}, series[0].Points)
	require.Empty(t, sketches)

	// Another check instance commits into the held second after the flush.
	series, sketches = nil, nil
	m.mergeSeries([]*metrics.Serie{filteredCounter(1, metrics.Point{Ts: 100.8, Value: 4})})
	m.mergeSketches([]mergedSketch{filteredSketch(1, 100, 2)})
	m.flush(115, &series, &sketches)

	require.Len(t, series, 1)
	assert.Equal(t, []metrics.Point{{Ts: 100, Value: 6}}, series[0].Points)
	require.Len(t, sketches, 1)
	assert.Equal(t, int64(2), sketches[0].Points[0].Sketch.Basic.Cnt)
}

func TestCheckOutputMergerDrainSendsHeldPoints(t *testing.T) {
	m := newCheckOutputMerger()
	var series metrics.Series
	var sketches metrics.SketchSeriesList

	m.mergeSeries([]*metrics.Serie{filteredCounter(1, metrics.Point{Ts: 100.1, Value: 2})})
	m.mergeSketches([]mergedSketch{filteredSketch(1, 100, 1)})
	m.flush(100, &series, &sketches)
	require.Empty(t, series)
	require.Empty(t, sketches)

	m.drain(&series, &sketches)
	assert.Len(t, series, 1)
	assert.Len(t, sketches, 1)

	series, sketches = nil, nil
	m.drain(&series, &sketches)
	assert.Empty(t, series)
	assert.Empty(t, sketches)
}

func TestCheckOutputMergerKeepsMetadataPerSecond(t *testing.T) {
	m := newCheckOutputMerger()
	var series metrics.Series

	unindexed := filteredCounter(1, metrics.Point{Ts: 100, Value: 1})
	unindexed.NoIndex = true
	m.mergeSeries([]*metrics.Serie{unindexed, filteredCounter(1, metrics.Point{Ts: 101, Value: 1})})
	m.flush(101, &series, nil)
	require.Len(t, series, 1)
	assert.True(t, series[0].NoIndex)

	series = nil
	m.flush(200, &series, nil)
	require.Len(t, series, 1)
	assert.False(t, series[0].NoIndex, "a second's metadata only comes from its own sources")
}

func TestCheckOutputMergerSendsPointsInTheirOwnSecond(t *testing.T) {
	m := newCheckOutputMerger()
	var series metrics.Series
	var sketches metrics.SketchSeriesList

	m.mergeSeries([]*metrics.Serie{filteredCounter(1, metrics.Point{Ts: 100, Value: 1})})
	m.mergeSketches([]mergedSketch{filteredSketch(1, 100, 1)})
	m.flush(101, &series, &sketches)
	require.Len(t, series, 1)
	require.Len(t, sketches, 1)

	// After a backward clock step, commits are stamped in seconds already sent. Like the rest of the Agent's output,
	// they're sent in their own second again.
	series, sketches = nil, nil
	m.mergeSeries([]*metrics.Serie{filteredCounter(1, metrics.Point{Ts: 100, Value: 4})})
	m.mergeSketches([]mergedSketch{filteredSketch(1, 100, 1)})
	m.flush(102, &series, &sketches)

	require.Len(t, series, 1)
	assert.Equal(t, []metrics.Point{{Ts: 100, Value: 4}}, series[0].Points)
	require.Len(t, sketches, 1)
	assert.Equal(t, int64(100), sketches[0].Points[0].Ts)
}

func TestCheckOutputMergerFollowsALongBackwardClockStep(t *testing.T) {
	m := newCheckOutputMerger()
	var series metrics.Series

	m.mergeSeries([]*metrics.Serie{filteredCounter(1, metrics.Point{Ts: 5000, Value: 1}, metrics.Point{Ts: 5030, Value: 1})})
	m.flush(5001, &series, nil)
	require.Len(t, series, 1)

	// The clock steps back 40 seconds: points keep their own second.
	series = nil
	m.mergeSeries([]*metrics.Serie{filteredCounter(1, metrics.Point{Ts: 4961, Value: 2})})
	m.flush(4962, &series, nil)
	require.Len(t, series, 1)
	assert.Equal(t, []metrics.Point{{Ts: 4961, Value: 2}}, series[0].Points)

	// The point held for a later second merges with the commits that reach it again.
	series = nil
	m.mergeSeries([]*metrics.Serie{filteredCounter(1, metrics.Point{Ts: 5030, Value: 4})})
	m.flush(5031, &series, nil)
	require.Len(t, series, 1)
	assert.Equal(t, []metrics.Point{{Ts: 5030, Value: 5}}, series[0].Points)
}

func TestCheckOutputMergerSendsPointsHeldFarAhead(t *testing.T) {
	m := newCheckOutputMerger()
	var series metrics.Series

	m.mergeSeries([]*metrics.Serie{filteredCounter(1, metrics.Point{Ts: 10000, Value: 1})})
	m.flush(10000, &series, nil)
	require.Empty(t, series)

	// The clock steps back more than maxHold: the held point is sent rather than held until the clock catches up.
	m.mergeSeries([]*metrics.Serie{filteredCounter(1, metrics.Point{Ts: 5000, Value: 2})})
	m.flush(5001, &series, nil)
	assert.Equal(t, []metrics.Point{{Ts: 5000, Value: 2}, {Ts: 10000, Value: 1}}, sentPoints(series)[1])
}

func TestCheckOutputMergerMergesMetadata(t *testing.T) {
	m := newCheckOutputMerger()
	var series metrics.Series
	var sketches metrics.SketchSeriesList

	first := filteredCounter(1, metrics.Point{Ts: 100, Value: 1})
	first.Source = metrics.MetricSourceOpenmetrics
	second := filteredCounter(1, metrics.Point{Ts: 100, Value: 1})
	second.NoIndex = true
	second.Source = metrics.MetricSourceDogstatsd
	firstSketch, secondSketch := filteredSketch(1, 100, 1), filteredSketch(1, 100, 1)
	secondSketch.sketch.NoIndex = true
	m.mergeSeries([]*metrics.Serie{first, second})
	m.mergeSketches([]mergedSketch{firstSketch, secondSketch})
	m.flush(200, &series, &sketches)

	require.Len(t, series, 1)
	assert.True(t, series[0].NoIndex, "a series isn't indexed if any of its sources asks not to")
	assert.Equal(t, min(metrics.MetricSourceOpenmetrics, metrics.MetricSourceDogstatsd), series[0].Source)
	require.Len(t, sketches, 1)
	assert.True(t, sketches[0].NoIndex)
}
