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
	ca.Submit(testCheckID, s1, 0, sink)

	require.Len(t, ca.windows, 1, "first submit should open a window")
	require.Empty(t, sink.series, "no series should be emitted yet (deadline not reached)")

	// Second submit for same context at t=1 appends.
	s2 := makeSerie(1, 1, 20)
	ca.Submit(testCheckID, s2, 1, sink)

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
	ca.Submit(testCheckID, makeSerie(1, 0, 1), 0, sink)
	ca.Submit(testCheckID, makeSerie(2, 0, 2), 0, sink)

	assert.Len(t, ca.windows, 2, "different contexts should get different windows")

	// Same (check_id, context) on the same check: no new window.
	ca.Submit(testCheckID, makeSerie(1, 1, 3), 1, sink)
	assert.Len(t, ca.windows, 2, "same (check_id, context) should not open a new window")

	// Same contextKey on a *different* check: separate window (windowKey
	// includes checkID).
	ca.Submit(testCheckID2, makeSerie(1, 0, 4), 0, sink)
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
	key := windowKey{checkID: testCheckID, contextKey: ckey.ContextKey(1)}
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
	ca.Submit(testCheckID, s, 0, sink)

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
	ca.Submit(testCheckID, s1, 0, sink)
	require.Empty(t, sink.series, "first submit, no emission yet")

	// Second commit at t=15: the existing window's deadline (15) has been
	// reached (15 <= 15), so it closes first, then a new window opens
	// for s2.
	s2 := makeSerie(1, 15, 20)
	ca.Submit(testCheckID, s2, 15, sink)

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
	ca.Submit(testCheckID, makeSerie(1, 0, 1), 0, sink)
	ca.Submit(testCheckID, makeSerie(1, 1, 2), 1, sink)
	ca.Submit(testCheckID, makeSerie(1, 2, 3), 2, sink) // exceeds cap

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
	ca.Submit(testCheckID, makeSerie(1, 0, 10), 0, sink)
	// Window B: opened at t=10, deadline t=25. Not yet expired at t=20.
	ca.Submit(testCheckID, makeSerie(2, 10, 20), 10, sink)
	require.Len(t, ca.windows, 2)
	require.Empty(t, sink.series)

	// FlushExpired at t=20 should emit window A only.
	ca.FlushExpired(20, sink)

	require.Len(t, sink.series, 1, "only the expired window should emit")
	assert.Equal(t, float64(10), sink.series[0].Points[0].Value, "value from window A's series")
	assert.Len(t, ca.windows, 1, "window B should still be open")
	_, stillOpen := ca.windows[windowKey{checkID: testCheckID, contextKey: ckey.ContextKey(2)}]
	assert.True(t, stillOpen, "window B (deadline=25) should remain after FlushExpired(20)")
}

// TestDrainForcesAllWindowsToEmit: Drain closes every accepting window
// regardless of deadline. Verifies the Q8 (drain) resolution's basic
// behaviour. (The 5s timeout is a Phase 4 concern.)
func TestDrainForcesAllWindowsToEmit(t *testing.T) {
	ca := newCheckAggregator(15*time.Second, 128)
	sink := &captureSink{}

	// Open two windows for different contexts, both well before their
	// deadlines.
	s1 := makeSerie(1, 0, 1)
	s2 := makeSerie(2, 0, 2)
	ca.Submit(testCheckID, s1, 0, sink)
	ca.Submit(testCheckID, s2, 0, sink)
	require.Len(t, ca.windows, 2)
	require.Empty(t, sink.series, "no flush yet")

	// Drain should close both windows immediately via singleton path.
	ca.Drain(sink)

	assert.Empty(t, ca.windows, "drain should empty the window map")
	assert.Len(t, sink.series, 2, "both singleton windows should have emitted")
}

// Helper: looks up a window by (checkID, ctxSeed) and returns it, or
// fails the test if not found.
func windowForKey(t *testing.T, ca *CheckAggregator, id checkid.ID, ctxSeed uint64) *aggregationWindow {
	t.Helper()
	key := windowKey{checkID: id, contextKey: ckey.ContextKey(ctxSeed)}
	w, ok := ca.windows[key]
	require.True(t, ok, "expected window for (%s, %d)", id, ctxSeed)
	return w
}
