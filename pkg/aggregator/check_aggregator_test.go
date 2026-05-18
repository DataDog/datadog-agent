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
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

// Spec phase-1 acceptance tests for the CheckAggregator wrapper.
// One test per spec rule / invariant called out in Plan B Phase 1.

// makeSerie builds a *Serie with a single Point at `ts`. ContextKey is
// derived from `ctxSeed` so callers can simulate same/different contexts.
func makeSerie(ctxSeed uint64, ts float64, value float64) *metrics.Serie {
	return &metrics.Serie{
		Name:       "test.metric",
		Points:     []metrics.Point{{Ts: ts, Value: value}},
		MType:      metrics.APIGaugeType,
		ContextKey: ckey.ContextKey(ctxSeed),
	}
}

// captureSink collects every Serie passed to Append for inspection.
type captureSink struct {
	series []*metrics.Serie
}

func (c *captureSink) Append(s *metrics.Serie) {
	c.series = append(c.series, s)
}

const (
	testCheckID  = checkid.ID("test-check")
	testCheckID2 = checkid.ID("test-check-2")
)

// TestOpenWindowForNewContext: a Submit for a context with no open window
// opens a new one. Submit for the same context within the deadline
// appends.
func TestOpenWindowForNewContext_FirstAndSecondSubmit(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSink{}

	// First submit at t=0 opens a window with deadline=15.
	s1 := makeSerie(1, 0, 10)
	ca.Submit(testCheckID, s1, sink)

	require.Len(t, ca.windows, 1, "first submit should open a window")
	require.Empty(t, sink.series, "no series should be emitted yet (deadline not reached)")

	// Second submit for same context at t=1 appends.
	s2 := makeSerie(1, 1, 20)
	ca.Submit(testCheckID, s2, sink)

	require.Len(t, ca.windows, 1, "second submit on same context should not open a new window")
	window := windowForKey(t, ca, testCheckID, 1)
	require.Len(t, window.series, 2, "both series should be buffered in the open window")
}

// TestSingleAcceptingWindowPerContext: distinct contexts get distinct
// windows; same (check_id, contextKey) collapses to one.
func TestSingleAcceptingWindowPerContext(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSink{}

	// Two different contexts on the same check.
	ca.Submit(testCheckID, makeSerie(1, 0, 1), sink)
	ca.Submit(testCheckID, makeSerie(2, 0, 2), sink)

	assert.Len(t, ca.windows, 2, "different contexts should get different windows")

	// Same (check_id, context) on the same check: no new window.
	ca.Submit(testCheckID, makeSerie(1, 1, 3), sink)
	assert.Len(t, ca.windows, 2, "same (check_id, context) should not open a new window")

	// Same contextKey on a *different* check: separate window (windowKey
	// includes checkID).
	ca.Submit(testCheckID2, makeSerie(1, 0, 4), sink)
	assert.Len(t, ca.windows, 3, "same contextKey on different check is a different window")
}

// TestDropEmptyWindowOnDeadline: a window that has reached its deadline
// with zero series produces no emission. (In practice this shouldn't
// happen because windows are lazy-open, but the close path defends
// against it.)
func TestDropEmptyWindowOnDeadline(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSink{}

	// Manually insert an empty window past its deadline (simulates the
	// case where every buffered series was somehow removed).
	key := windowKey{checkID: testCheckID, contextKey: ckey.ContextKey(1), name: "test.metric", mType: metrics.APIGaugeType}
	ca.windows[key] = &aggregationWindow{
		series:   nil,
		openedAt: 0,
		deadline: 15,
	}

	ca.FlushExpired(20, sink)

	assert.Empty(t, sink.series, "empty window must not synthesize an emission")
	assert.Empty(t, ca.windows, "expired empty window should be removed")
}

// TestSingletonWindowPassThrough: a window with exactly one series at
// its deadline emits that series unchanged (preserving its original
// commit_timestamp via the Point's Ts field). This is the byte-identical
// path for slow checks.
func TestSingletonWindowPassThrough(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSink{}

	// Submit one series at t=0 (deadline = 15).
	s := makeSerie(1, 0, 42)
	ca.Submit(testCheckID, s, sink)

	// Flush at t=15 — deadline reached, count = 1 → SingletonPassThrough.
	ca.FlushExpired(15, sink)

	require.Len(t, sink.series, 1, "singleton window should emit exactly one series")
	emitted := sink.series[0]
	assert.Same(t, s, emitted, "series should be emitted unchanged (same pointer)")
	require.Len(t, emitted.Points, 1)
	assert.Equal(t, float64(0), emitted.Points[0].Ts, "commit_timestamp preserved")
	assert.Equal(t, float64(42), emitted.Points[0].Value, "value preserved")
	assert.Empty(t, ca.windows, "emitted window should be removed")
}

// TestSubmitClosesExpiredWindowBeforeAppending: when a new series arrives
// for a context whose existing window has already expired (e.g. a slow
// check at 15s cadence), the existing window is closed first and a new
// window is opened with the incoming series.
//
// This is the critical byte-identicality path for slow checks across
// multiple flush cycles.
func TestSubmitClosesExpiredWindowBeforeAppending(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSink{}

	// First commit at t=0; deadline = 15.
	s1 := makeSerie(1, 0, 10)
	ca.Submit(testCheckID, s1, sink)
	require.Empty(t, sink.series, "first submit, no emission yet")

	// Second commit at t=15: the existing window's deadline (15) has been
	// reached (15 <= 15), so it closes first, then a new window opens
	// for s2.
	s2 := makeSerie(1, 15, 20)
	ca.Submit(testCheckID, s2, sink)

	require.Len(t, sink.series, 1, "expired window should have emitted s1 before s2 opened")
	assert.Same(t, s1, sink.series[0], "s1 was the singleton emission")

	// s2 should now be in a fresh window with deadline=30.
	require.Len(t, ca.windows, 1)
	window := windowForKey(t, ca, testCheckID, 1)
	require.Len(t, window.series, 1)
	assert.Same(t, s2, window.series[0])
	assert.Equal(t, float64(30), window.deadline)
}

// TestIncrementDropCountWhenWindowFull: a Submit for a full window
// increments the drop counter and discards the series.
func TestIncrementDropCountWhenWindowFull(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 2) // cap = 2 for easy testing
	sink := &captureSink{}

	// Three submits at t=0..2 for the same context, all before deadline (15).
	ca.Submit(testCheckID, makeSerie(1, 0, 1), sink)
	ca.Submit(testCheckID, makeSerie(1, 1, 2), sink)
	ca.Submit(testCheckID, makeSerie(1, 2, 3), sink) // exceeds cap

	window := windowForKey(t, ca, testCheckID, 1)
	assert.Len(t, window.series, 2, "buffer should be capped at 2 series")
	assert.Equal(t, 1, window.droppedCount, "third submit should increment drop counter")
}

// TestFlushExpiredMixedDeadlines: FlushExpired should only close windows
// whose deadline has passed; windows still within their deadline must
// remain open and accumulate further series. Regression guard for the
// per-window deadline check inside the FlushExpired loop.
func TestFlushExpiredMixedDeadlines(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSink{}

	// Window A: opened at t=0, deadline t=15. Expired by t=20.
	ca.Submit(testCheckID, makeSerie(1, 0, 10), sink)
	// Window B: opened at t=10, deadline t=25. Not yet expired at t=20.
	ca.Submit(testCheckID, makeSerie(2, 10, 20), sink)
	require.Len(t, ca.windows, 2)
	require.Empty(t, sink.series)

	// FlushExpired at t=20 should emit window A only.
	ca.FlushExpired(20, sink)

	require.Len(t, sink.series, 1, "only the expired window should emit")
	assert.Equal(t, float64(10), sink.series[0].Points[0].Value, "value from window A's series")
	assert.Len(t, ca.windows, 1, "window B should still be open")
	_, stillOpen := ca.windows[windowKey{checkID: testCheckID, contextKey: ckey.ContextKey(2), name: "test.metric", mType: metrics.APIGaugeType}]
	assert.True(t, stillOpen, "window B (deadline=25) should remain after FlushExpired(20)")
}

// TestDrainForcesAllWindowsToEmit: Drain closes every accepting window
// regardless of deadline. Verifies the Q8 (drain) resolution's basic
// behaviour. (The 5s timeout is a Phase 4 concern.)
func TestDrainForcesAllWindowsToEmit(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	serieSink := &captureSink{}
	sketchSink := &captureSketchSink{}

	// Open two series windows for different contexts, both well before
	// their deadlines.
	s1 := makeSerie(1, 0, 1)
	s2 := makeSerie(2, 0, 2)
	ca.Submit(testCheckID, s1, serieSink)
	ca.Submit(testCheckID, s2, serieSink)
	require.Len(t, ca.windows, 2)
	require.Empty(t, serieSink.series, "no flush yet")

	// Drain should close both windows immediately via singleton path.
	ca.Drain(serieSink, sketchSink)

	assert.Empty(t, ca.windows, "drain should empty the window map")
	assert.Len(t, serieSink.series, 2, "both singleton windows should have emitted")
}

// Helper: looks up a window by (checkID, ctxSeed) and returns it, or
// fails the test if not found.
func windowForKey(t *testing.T, ca *CheckAggregator, id checkid.ID, ctxSeed uint64) *aggregationWindow {
	t.Helper()
	key := windowKey{checkID: id, contextKey: ckey.ContextKey(ctxSeed), name: "test.metric", mType: metrics.APIGaugeType}
	w, ok := ca.windows[key]
	require.True(t, ok, "expected window for (%s, %d)", id, ctxSeed)
	return w
}

// captureSketchSink collects every SketchSeries passed to Append.
type captureSketchSink struct {
	sketches []*metrics.SketchSeries
}

func (c *captureSketchSink) Append(ss *metrics.SketchSeries) {
	c.sketches = append(c.sketches, ss)
}

// makeSketchSeries builds a SketchSeries carrying one SketchPoint whose
// underlying sketch contains the given values. ContextKey is derived
// from ctxSeed.
func makeSketchSeries(ctxSeed uint64, ts int64, values ...float64) *metrics.SketchSeries {
	s := &quantile.Sketch{}
	s.Insert(quantile.Default(), values...)
	return &metrics.SketchSeries{
		Name:       "test.distribution",
		ContextKey: ckey.ContextKey(ctxSeed),
		Points:     []metrics.SketchPoint{{Ts: ts, Sketch: s}},
	}
}

// TestSketchSubmit_OpensAndAppendsWindow: the sketch path's windowing
// mirrors the series path — first SketchSeries opens a window, second
// for the same context appends before deadline.
func TestSketchSubmit_OpensAndAppendsWindow(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSketchSink{}

	ca.SubmitSketch(testCheckID, makeSketchSeries(1, 0, 10), sink)
	require.Len(t, ca.sketchWindows, 1, "first sketch submit should open a window")
	require.Empty(t, sink.sketches, "no emission yet (deadline not reached)")

	ca.SubmitSketch(testCheckID, makeSketchSeries(1, 1, 20), sink)
	assert.Len(t, ca.sketchWindows, 1, "second submit on same context should append, not open a new window")
	key := windowKey{checkID: testCheckID, contextKey: ckey.ContextKey(1), name: "test.distribution"}
	assert.Len(t, ca.sketchWindows[key].sketches, 2, "both sketches buffered")
}

// TestSketchSingletonPassThrough: a sketch window with exactly one
// SketchSeries at deadline emits unchanged. Slow checks see byte-
// identical sketch output.
func TestSketchSingletonPassThrough(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSketchSink{}

	original := makeSketchSeries(1, 0, 10, 20, 30)
	ca.SubmitSketch(testCheckID, original, sink)
	ca.FlushExpiredSketches(15, sink)

	require.Len(t, sink.sketches, 1)
	assert.Same(t, original, sink.sketches[0], "singleton sketch window emits unchanged")
}

// TestSketchMergeQuantiles: when multiple sketches in the same window close
// together, mergeSketches produces one merged sketch with quantiles matching
// a reference sketch built from the same inputs.
func TestSketchMergeQuantiles(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSketchSink{}

	// Submit three sketches across three "commits", each containing a
	// portion of the underlying samples. Total: 10..100.
	values := []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	ca.SubmitSketch(testCheckID, makeSketchSeries(1, 1, values[0:3]...), sink)
	ca.SubmitSketch(testCheckID, makeSketchSeries(1, 2, values[3:7]...), sink)
	ca.SubmitSketch(testCheckID, makeSketchSeries(1, 3, values[7:]...), sink)

	ca.FlushExpiredSketches(20, sink)

	require.Len(t, sink.sketches, 1, "three sketches must merge to one output")
	emitted := sink.sketches[0]
	require.Len(t, emitted.Points, 1, "merged output has one SketchPoint")
	assert.Equal(t, int64(3), emitted.Points[0].Ts, "stamped at latest input timestamp")

	mergedSketch, ok := emitted.Points[0].Sketch.(*quantile.Sketch)
	require.True(t, ok, "emitted sketch should be *quantile.Sketch")

	// Reference sketch built from all values directly. Quantiles should match
	// for the already-built DDSketch inputs used in this test.
	ref := &quantile.Sketch{}
	ref.Insert(quantile.Default(), values...)

	for _, q := range []float64{0.5, 0.95, 0.99} {
		assert.InDelta(t,
			ref.Quantile(quantile.Default(), q),
			mergedSketch.Quantile(quantile.Default(), q),
			1e-9,
			"quantile q=%f should match the direct-build reference", q)
	}
}

// TestSketchDropEmptyWindow: an empty sketch window at deadline emits
// nothing. (Defensive; lazy-open means this rarely occurs in practice.)
func TestSketchDropEmptyWindow(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSketchSink{}

	key := windowKey{checkID: testCheckID, contextKey: ckey.ContextKey(1), name: "test.distribution"}
	ca.sketchWindows[key] = &sketchAggregationWindow{
		sketches: nil,
		openedAt: 0,
		deadline: 15,
	}

	ca.FlushExpiredSketches(20, sink)
	assert.Empty(t, sink.sketches, "empty sketch window must not emit")
	assert.Empty(t, ca.sketchWindows, "expired empty window should be removed")
}

// TestSketchSubmitClosesExpiredWindowBeforeAppending: deadline-passed
// case for the sketch path. Mirrors the series-side test.
func TestSketchSubmitClosesExpiredWindowBeforeAppending(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSketchSink{}

	first := makeSketchSeries(1, 0, 10)
	ca.SubmitSketch(testCheckID, first, sink)
	require.Empty(t, sink.sketches)

	// Next submit at t=15: deadline (15) <= now (15) → close singleton,
	// then open a new window.
	second := makeSketchSeries(1, 15, 20)
	ca.SubmitSketch(testCheckID, second, sink)

	require.Len(t, sink.sketches, 1, "expired singleton window emits before new one opens")
	assert.Same(t, first, sink.sketches[0])
}

// TestSketchDropCountWhenWindowFull: cap on sketches per window.
func TestSketchDropCountWhenWindowFull(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 2)
	sink := &captureSketchSink{}

	ca.SubmitSketch(testCheckID, makeSketchSeries(1, 0, 1), sink)
	ca.SubmitSketch(testCheckID, makeSketchSeries(1, 1, 2), sink)
	ca.SubmitSketch(testCheckID, makeSketchSeries(1, 2, 3), sink) // exceeds cap

	key := windowKey{checkID: testCheckID, contextKey: ckey.ContextKey(1), name: "test.distribution"}
	window := ca.sketchWindows[key]
	assert.Len(t, window.sketches, 2, "buffer capped at 2 sketches")
	assert.Equal(t, 1, window.droppedCount, "third submit increments drop counter")
}

// TestDrainEmitsBothSeriesAndSketches: Drain must close every accepting
// window in both maps.
func TestDrainEmitsBothSeriesAndSketches(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	serieSink := &captureSink{}
	sketchSink := &captureSketchSink{}

	ca.Submit(testCheckID, makeSerie(1, 0, 10), serieSink)
	ca.SubmitSketch(testCheckID, makeSketchSeries(2, 0, 99), sketchSink)
	require.Empty(t, serieSink.series)
	require.Empty(t, sketchSink.sketches)

	ca.Drain(serieSink, sketchSink)

	assert.Empty(t, ca.windows, "Drain empties the series window map")
	assert.Empty(t, ca.sketchWindows, "Drain empties the sketch window map")
	assert.Len(t, serieSink.series, 1, "series emitted via singleton path")
	assert.Len(t, sketchSink.sketches, 1, "sketches emitted via singleton path")
}

// makeSerieMType builds a *Serie with one Point and the given API metric
// type, for testing the per-mtype roll-up strategies.
func makeSerieMType(ctxSeed uint64, ts float64, value float64, mtype metrics.APIMetricType) *metrics.Serie {
	return &metrics.Serie{
		Name:       "test.metric",
		Points:     []metrics.Point{{Ts: ts, Value: value}},
		MType:      mtype,
		ContextKey: ckey.ContextKey(ctxSeed),
	}
}

// TestTimestampedSerieSlowCheckPreserved: a multi-point Serie (the shape
// of MetricWithTimestamp.flush() output) arriving in a slow-check window
// is emitted unchanged via SingletonWindowPassThrough — preserving all
// points and timestamps. Slow checks see no behaviour change.
func TestTimestampedSerieSlowCheckPreserved(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSink{}

	multiPoint := &metrics.Serie{
		Name:       "test.timestamped",
		Points:     []metrics.Point{{Ts: 1, Value: 10}, {Ts: 2, Value: 20}, {Ts: 3, Value: 30}},
		MType:      metrics.APIGaugeType,
		ContextKey: ckey.ContextKey(99),
	}

	ca.Submit(testCheckID, multiPoint, sink)
	// One Serie in the window → singleton passthrough at deadline.
	ca.FlushExpired(20, sink)

	require.Len(t, sink.series, 1, "single multi-point Serie should emit once")
	assert.Same(t, multiPoint, sink.series[0], "singleton path preserves Serie unchanged")
	require.Len(t, sink.series[0].Points, 3, "all three timestamped points preserved")
}

// TestTimestampedSerieFastCheckAggregated: when multiple commits each
// produce timestamped Series in the same window (fast-cadence
// timestamped emission), the wrapper aggregates per the API metric type
// and emits a single Serie. Per-timestamp fidelity at backend is lost
// (recoverable via the recorder); cost goal met.
func TestTimestampedSerieFastCheckAggregated(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSink{}

	// Three commits, each producing a Serie carrying one timestamped
	// point. All share a context. APIGaugeType → `last` strategy.
	for i, ts := range []float64{1, 2, 3} {
		s := &metrics.Serie{
			Name:       "test.timestamped",
			Points:     []metrics.Point{{Ts: ts, Value: float64(10 * (i + 1))}},
			MType:      metrics.APIGaugeType,
			ContextKey: ckey.ContextKey(99),
		}
		ca.Submit(testCheckID, s, sink)
	}
	ca.FlushExpired(20, sink)

	require.Len(t, sink.series, 1, "fast-cadence timestamped aggregates to one Serie")
	emitted := sink.series[0]
	require.Len(t, emitted.Points, 1, "aggregated output is single-point")
	assert.Equal(t, float64(30), emitted.Points[0].Value, "last value retained (10*3=30)")
	assert.Equal(t, float64(3), emitted.Points[0].Ts, "stamped at latest commit's timestamp")
}

// TestAggregateGauge_LastStrategy: count > 1 window of gauge series
// emits one Serie carrying the latest value, stamped at the latest
// commit's timestamp.
func TestAggregateGauge_LastStrategy(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSink{}

	// Three gauge commits at t=1, 2, 3 with values 10, 20, 30.
	ca.Submit(testCheckID, makeSerieMType(1, 1, 10, metrics.APIGaugeType), sink)
	ca.Submit(testCheckID, makeSerieMType(1, 2, 20, metrics.APIGaugeType), sink)
	ca.Submit(testCheckID, makeSerieMType(1, 3, 30, metrics.APIGaugeType), sink)

	// Force window close via FlushExpired at t=20 (well past deadline 16).
	ca.FlushExpired(20, sink)

	require.Len(t, sink.series, 1, "should emit one aggregated series")
	emitted := sink.series[0]
	require.Len(t, emitted.Points, 1)
	assert.Equal(t, float64(30), emitted.Points[0].Value, "last value wins")
	assert.Equal(t, float64(3), emitted.Points[0].Ts, "stamped at latest commit's timestamp")
	assert.Equal(t, metrics.APIGaugeType, emitted.MType, "MType preserved")
}

// TestAggregateCount_SumStrategy: count > 1 window of count series emits
// one Serie carrying the sum of all values across the window.
func TestAggregateCount_SumStrategy(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSink{}

	// Three count commits at t=1, 2, 3 with values 5, 7, 8 (sum = 20).
	ca.Submit(testCheckID, makeSerieMType(1, 1, 5, metrics.APICountType), sink)
	ca.Submit(testCheckID, makeSerieMType(1, 2, 7, metrics.APICountType), sink)
	ca.Submit(testCheckID, makeSerieMType(1, 3, 8, metrics.APICountType), sink)

	ca.FlushExpired(20, sink)

	require.Len(t, sink.series, 1)
	emitted := sink.series[0]
	require.Len(t, emitted.Points, 1)
	assert.Equal(t, float64(20), emitted.Points[0].Value, "sum across window")
	assert.Equal(t, float64(3), emitted.Points[0].Ts, "stamped at latest commit's timestamp")
	assert.Equal(t, metrics.APICountType, emitted.MType)
}

// TestAggregateRate_AvgStrategy: count > 1 window of APIRateType series
// emits one Serie carrying the arithmetic mean of values across the
// window. At uniform commit cadence (which CheckSampler produces), this
// equals the true window rate.
func TestAggregateRate_AvgStrategy(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSink{}

	// Three commits with rate values 0.5, 0.7, 0.9. Avg = 0.7.
	ca.Submit(testCheckID, makeSerieMType(1, 1, 0.5, metrics.APIRateType), sink)
	ca.Submit(testCheckID, makeSerieMType(1, 2, 0.7, metrics.APIRateType), sink)
	ca.Submit(testCheckID, makeSerieMType(1, 3, 0.9, metrics.APIRateType), sink)

	ca.FlushExpired(20, sink)

	require.Len(t, sink.series, 1, "should emit one aggregated series")
	emitted := sink.series[0]
	require.Len(t, emitted.Points, 1)
	assert.InDelta(t, 0.7, emitted.Points[0].Value, 1e-9, "average of 0.5/0.7/0.9")
	assert.Equal(t, float64(3), emitted.Points[0].Ts, "stamped at latest commit's timestamp")
	assert.Equal(t, metrics.APIRateType, emitted.MType)
}

// TestAggregateUnknownAPIMetricType_FallsBackToLast: a Serie with a
// future / unknown APIMetricType still emits a single aggregated Serie
// (the cost constraint is non-negotiable), using the conservative
// `last` strategy.
func TestAggregateUnknownAPIMetricType_FallsBackToLast(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSink{}

	// Synthetic API metric type beyond the current enum range. Numeric
	// value chosen to avoid colliding with existing constants.
	unknownAPIType := metrics.APIMetricType(99)
	ca.Submit(testCheckID, makeSerieMType(1, 1, 10, unknownAPIType), sink)
	ca.Submit(testCheckID, makeSerieMType(1, 2, 20, unknownAPIType), sink)
	ca.Submit(testCheckID, makeSerieMType(1, 3, 30, unknownAPIType), sink)

	ca.FlushExpired(20, sink)

	require.Len(t, sink.series, 1, "default branch must still aggregate to 1 series")
	assert.Equal(t, float64(30), sink.series[0].Points[0].Value, "default falls back to last")
}

// TestSingleSampleMetricWithTimestamp_FallsThroughHeuristic: a
// MetricWithTimestamp that received exactly one sample produces a
// Serie with len(Points) == 1, which the bypass heuristic (len > 1)
// does not catch. For slow checks (singleton windows) it passes through
// unchanged via SingletonWindowPassThrough. For fast checks it is
// windowed like a regular same-API-type metric (documented
// approximation; see package comment).
func TestSingleSampleMetricWithTimestamp_FallsThroughHeuristic(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSink{}

	// Slow-check path: one Serie, deadline reached → singleton bypass
	// emits unchanged. This is the safe case.
	single := &metrics.Serie{
		Name:       "test.timestamped.single",
		Points:     []metrics.Point{{Ts: 5, Value: 42}},
		MType:      metrics.APIGaugeType,
		ContextKey: ckey.ContextKey(7),
	}
	ca.Submit(testCheckID, single, sink)
	ca.FlushExpired(20, sink)

	require.Len(t, sink.series, 1, "single-point Serie should still emit")
	assert.Same(t, single, sink.series[0], "singleton path preserves it unchanged")
	require.Len(t, sink.series[0].Points, 1)
	assert.Equal(t, float64(5), sink.series[0].Points[0].Ts, "caller-provided timestamp preserved")
}

// TestAggregateDoesNotMutateBufferedSeries: cloneSerieWithSinglePoint
// guarantees the original buffered series are not mutated by aggregation.
// Important because callers (CheckSampler.flush, tests) may still hold
// references to the original series.
func TestAggregateDoesNotMutateBufferedSeries(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSink{}

	s1 := makeSerieMType(1, 1, 10, metrics.APIGaugeType)
	s2 := makeSerieMType(1, 2, 20, metrics.APIGaugeType)
	ca.Submit(testCheckID, s1, sink)
	ca.Submit(testCheckID, s2, sink)

	ca.FlushExpired(20, sink)

	// Originals untouched.
	require.Len(t, s1.Points, 1)
	assert.Equal(t, float64(10), s1.Points[0].Value)
	require.Len(t, s2.Points, 1)
	assert.Equal(t, float64(20), s2.Points[0].Value)

	// Emitted is a fresh Serie, not one of the inputs.
	require.Len(t, sink.series, 1)
	assert.NotSame(t, s1, sink.series[0])
	assert.NotSame(t, s2, sink.series[0])
}

func TestAggregateGaugeLastUsesLatestTimestamp(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSink{}

	ca.Submit(testCheckID, makeSerieMType(1, 3, 30, metrics.APIGaugeType), sink)
	ca.Submit(testCheckID, makeSerieMType(1, 1, 10, metrics.APIGaugeType), sink)
	ca.Submit(testCheckID, makeSerieMType(1, 2, 20, metrics.APIGaugeType), sink)

	ca.FlushExpired(20, sink)

	require.Len(t, sink.series, 1)
	assert.Equal(t, float64(30), sink.series[0].Points[0].Value)
	assert.Equal(t, float64(3), sink.series[0].Points[0].Ts)
}

func TestAggregateHistogramGaugeSuffixStrategies(t *testing.T) {
	tests := []struct {
		name       string
		metricName string
		expected   float64
	}{
		{name: "max suffix keeps max of maxes", metricName: "request.duration.max", expected: 30},
		{name: "min suffix keeps min of mins", metricName: "request.duration.min", expected: 10},
		{name: "sum suffix sums sums", metricName: "request.duration.sum", expected: 60},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ca := newCheckAggregator(15*time.Second, 128)
			sink := &captureSink{}

			for i, value := range []float64{20, 10, 30} {
				ca.Submit(testCheckID, &metrics.Serie{
					Name:       tt.metricName,
					Points:     []metrics.Point{{Ts: float64(i + 1), Value: value}},
					MType:      metrics.APIGaugeType,
					ContextKey: ckey.ContextKey(1),
				}, sink)
			}
			ca.FlushExpired(20, sink)

			require.Len(t, sink.series, 1)
			assert.Equal(t, tt.expected, sink.series[0].Points[0].Value)
			assert.Equal(t, float64(3), sink.series[0].Points[0].Ts)
		})
	}
}

func TestSubmitUsesSeriesTimestampForRollover(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSink{}

	for ts := float64(0); ts < 15; ts++ {
		ca.Submit(testCheckID, makeSerie(1, ts, ts+1), sink)
	}

	require.Empty(t, sink.series, "series within the same sample-time window must not close against the flush trigger time")
	ca.FlushExpired(15, sink)

	require.Len(t, sink.series, 1)
	require.Len(t, sink.series[0].Points, 1)
	assert.Equal(t, float64(15), sink.series[0].Points[0].Value, "gauge window keeps the latest sample")
	assert.Equal(t, float64(14), sink.series[0].Points[0].Ts)
}

func TestSameContextDifferentSeriesIdentitiesDoNotMerge(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSink{}

	ca.Submit(testCheckID, &metrics.Serie{
		Name:       "request.duration.avg",
		Points:     []metrics.Point{{Ts: 1, Value: 10}},
		MType:      metrics.APIGaugeType,
		ContextKey: ckey.ContextKey(1),
	}, sink)
	ca.Submit(testCheckID, &metrics.Serie{
		Name:       "request.duration.count",
		Points:     []metrics.Point{{Ts: 1, Value: 3}},
		MType:      metrics.APIRateType,
		ContextKey: ckey.ContextKey(1),
	}, sink)
	ca.Submit(testCheckID, &metrics.Serie{
		Name:       "request.duration.avg",
		Points:     []metrics.Point{{Ts: 2, Value: 20}},
		MType:      metrics.APIGaugeType,
		ContextKey: ckey.ContextKey(1),
	}, sink)
	ca.Submit(testCheckID, &metrics.Serie{
		Name:       "request.duration.count",
		Points:     []metrics.Point{{Ts: 2, Value: 5}},
		MType:      metrics.APIRateType,
		ContextKey: ckey.ContextKey(1),
	}, sink)

	ca.FlushExpired(20, sink)

	require.Len(t, sink.series, 2, "same context key with different final series identity must produce separate outputs")
	byName := map[string]*metrics.Serie{}
	for _, s := range sink.series {
		byName[s.Name] = s
	}
	require.Contains(t, byName, "request.duration.avg")
	require.Contains(t, byName, "request.duration.count")
	assert.Equal(t, float64(20), byName["request.duration.avg"].Points[0].Value)
	assert.Equal(t, metrics.APIGaugeType, byName["request.duration.avg"].MType)
	assert.Equal(t, float64(4), byName["request.duration.count"].Points[0].Value)
	assert.Equal(t, metrics.APIRateType, byName["request.duration.count"].MType)
}

func TestSubmitSketchUsesSketchTimestampForRollover(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSketchSink{}

	for ts := int64(0); ts < 15; ts++ {
		ca.SubmitSketch(testCheckID, makeSketchSeries(1, ts, float64(ts+1)), sink)
	}

	require.Empty(t, sink.sketches, "sketches within the same sample-time window must not close against the flush trigger time")
	ca.FlushExpiredSketches(15, sink)

	require.Len(t, sink.sketches, 1)
	require.Len(t, sink.sketches[0].Points, 1)
	assert.Equal(t, int64(14), sink.sketches[0].Points[0].Ts)
}
