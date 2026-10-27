// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

func noPendingSketch(ckey.ContextKey, int64) bool { return false }

func filteredCounter(key ckey.ContextKey, points ...metrics.Point) *metrics.Serie {
	return &metrics.Serie{Name: "my.counter", ContextKey: key, MType: metrics.APIRateType, Points: points, TagFiltered: true}
}

func filteredSketch(key ckey.ContextKey, ts int64, values ...float64) *metrics.SketchSeries {
	sketch := &quantile.Sketch{}
	sketch.Insert(quantile.Default(), values...)
	return &metrics.SketchSeries{
		DistributionMetadata: metrics.DistributionMetadata{Name: "my.dist"},
		Points:               []metrics.SketchPoint{{Sketch: sketch, Ts: ts}},
		ContextKey:           key,
		TagFiltered:          true,
	}
}

func TestCheckOutputMergerSumsCountersInTheSameSecond(t *testing.T) {
	m := newCheckOutputMerger(time.Second)
	var series metrics.Series

	m.appendSeries(&series, []*metrics.Serie{filteredCounter(1, metrics.Point{Ts: 100.2, Value: 1})})
	m.appendSeries(&series, []*metrics.Serie{filteredCounter(1, metrics.Point{Ts: 100.7, Value: 2}, metrics.Point{Ts: 101.1, Value: 4})})
	m.appendSeries(&series, []*metrics.Serie{filteredCounter(2, metrics.Point{Ts: 100.5, Value: 8})})
	require.Empty(t, series, "series of filtered metrics are kept to merge")

	m.flush(200, noPendingSketch, &series, nil)

	require.Len(t, series, 2)
	byKey := map[ckey.ContextKey]*metrics.Serie{}
	for _, s := range series {
		byKey[s.ContextKey] = s
	}
	assert.Equal(t, []metrics.Point{{Ts: 100.2, Value: 3}, {Ts: 101.1, Value: 4}}, byKey[1].Points)
	assert.Equal(t, []metrics.Point{{Ts: 100.5, Value: 8}}, byKey[2].Points)
}

func TestCheckOutputMergerMergesSketchesInTheSameSecond(t *testing.T) {
	m := newCheckOutputMerger(time.Second)
	var sketches metrics.SketchSeriesList

	m.appendSketches(&sketches, metrics.SketchSeriesList{filteredSketch(1, 100, 1, 2)})
	m.appendSketches(&sketches, metrics.SketchSeriesList{filteredSketch(1, 100, 3), filteredSketch(1, 101, 4)})
	m.flush(200, noPendingSketch, nil, &sketches)

	require.Len(t, sketches, 1)
	points := sketches[0].Points
	require.Len(t, points, 2)
	assert.Equal(t, int64(100), points[0].Ts)
	assert.Equal(t, int64(3), points[0].Sketch.Basic.Cnt)
	assert.Equal(t, int64(101), points[1].Ts)
	assert.Equal(t, int64(1), points[1].Sketch.Basic.Cnt)
}

func TestCheckOutputMergerHoldsCountersFromTheFlushSecond(t *testing.T) {
	m := newCheckOutputMerger(15 * time.Second)
	var series metrics.Series

	m.appendSeries(&series, []*metrics.Serie{filteredCounter(1, metrics.Point{Ts: 99.9, Value: 1}, metrics.Point{Ts: 100.1, Value: 2})})
	m.flush(100, noPendingSketch, &series, nil)

	require.Len(t, series, 1)
	assert.Equal(t, []metrics.Point{{Ts: 99.9, Value: 1}}, series[0].Points)

	// Another check instance commits into the held second after the flush.
	series = nil
	m.appendSeries(&series, []*metrics.Serie{filteredCounter(1, metrics.Point{Ts: 100.8, Value: 4})})
	m.flush(101, noPendingSketch, &series, nil)

	require.Len(t, series, 1)
	assert.Equal(t, []metrics.Point{{Ts: 100.1, Value: 6}}, series[0].Points)
}

func TestCheckOutputMergerHoldsSketchesForAFlushInterval(t *testing.T) {
	m := newCheckOutputMerger(15 * time.Second)
	var sketches metrics.SketchSeriesList

	m.appendSketches(&sketches, metrics.SketchSeriesList{filteredSketch(1, 84, 1), filteredSketch(1, 85, 1)})
	m.flush(100, noPendingSketch, nil, &sketches)

	require.Len(t, sketches, 1)
	require.Len(t, sketches[0].Points, 1)
	assert.Equal(t, int64(84), sketches[0].Points[0].Ts)

	// A sample submitted before the flush reaches the aggregator after it.
	sketches = nil
	m.appendSketches(&sketches, metrics.SketchSeriesList{filteredSketch(1, 85, 2)})
	m.flush(115, noPendingSketch, nil, &sketches)

	require.Len(t, sketches, 1)
	assert.Equal(t, int64(2), sketches[0].Points[0].Sketch.Basic.Cnt)
}

func TestCheckOutputMergerHoldsSketchesPendingInASampler(t *testing.T) {
	m := newCheckOutputMerger(time.Second)
	var sketches metrics.SketchSeriesList

	// Another check instance has not handed over its sketch for context 1 at second 50 yet.
	pending := true
	isPending := func(ck ckey.ContextKey, ts int64) bool { return pending && ck == 1 && ts == 50 }

	m.appendSketches(&sketches, metrics.SketchSeriesList{filteredSketch(1, 50, 1), filteredSketch(2, 50, 1)})
	m.flush(200, isPending, nil, &sketches)

	require.Len(t, sketches, 1)
	assert.Equal(t, ckey.ContextKey(2), sketches[0].ContextKey)

	// The other instance commits.
	sketches = nil
	pending = false
	m.appendSketches(&sketches, metrics.SketchSeriesList{filteredSketch(1, 50, 2)})
	m.flush(215, isPending, nil, &sketches)

	require.Len(t, sketches, 1)
	assert.Equal(t, ckey.ContextKey(1), sketches[0].ContextKey)
	assert.Equal(t, int64(2), sketches[0].Points[0].Sketch.Basic.Cnt)
}

func TestCheckOutputMergerDrainSendsHeldPoints(t *testing.T) {
	m := newCheckOutputMerger(time.Second)
	var series metrics.Series
	var sketches metrics.SketchSeriesList

	m.appendSeries(&series, []*metrics.Serie{filteredCounter(1, metrics.Point{Ts: 100.1, Value: 2})})
	m.appendSketches(&sketches, metrics.SketchSeriesList{filteredSketch(1, 50, 1)})
	m.flush(100, func(ckey.ContextKey, int64) bool { return true }, &series, &sketches)
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

func TestCheckOutputMergerPassesThroughUnfilteredOutputs(t *testing.T) {
	m := newCheckOutputMerger(time.Second)
	var series metrics.Series
	var sketches metrics.SketchSeriesList

	counter := filteredCounter(1, metrics.Point{Ts: 100, Value: 1})
	counter.TagFiltered = false
	sketch := filteredSketch(1, 100, 1)
	sketch.TagFiltered = false
	m.appendSeries(&series, []*metrics.Serie{counter, counter})
	m.appendSketches(&sketches, metrics.SketchSeriesList{sketch, sketch})

	assert.Len(t, series, 2)
	assert.Len(t, sketches, 2)
}

func TestCheckOutputMergerPassesThroughFilteredSeriesThatCannotBeSummed(t *testing.T) {
	m := newCheckOutputMerger(time.Second)
	var series metrics.Series

	gauge := filteredCounter(1, metrics.Point{Ts: 100, Value: 1})
	gauge.MType = metrics.APIGaugeType
	m.appendSeries(&series, []*metrics.Serie{gauge, gauge})

	assert.Len(t, series, 2)
}

func TestCheckOutputMergerKeepsTheSeriesOfAContextApart(t *testing.T) {
	m := newCheckOutputMerger(time.Second)
	var series metrics.Series

	count := filteredCounter(1, metrics.Point{Ts: 100, Value: 1})
	count.NameSuffix = ".count"
	sum := filteredCounter(1, metrics.Point{Ts: 100, Value: 2})
	sum.NameSuffix = ".sum"
	m.appendSeries(&series, []*metrics.Serie{count, sum})
	m.flush(200, noPendingSketch, &series, nil)

	require.Len(t, series, 2)
	for _, s := range series {
		assert.Len(t, s.Points, 1, s.NameSuffix)
	}
}
