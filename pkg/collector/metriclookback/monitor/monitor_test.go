// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package monitor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type inMemoryReader struct {
	points []Point
}

func (r *inMemoryReader) PointsBetween(_ string, from, to time.Time) []Point {
	var out []Point
	for _, point := range r.points {
		if !from.IsZero() && point.Ts.Before(from) {
			continue
		}
		if !to.IsZero() && point.Ts.After(to) {
			continue
		}
		out = append(out, point)
	}
	return out
}

func TestWatcherEmitsHealthyWhenWindowMinimumEqualsBaselineMin(t *testing.T) {
	start := time.Unix(100, 0)
	reader := &inMemoryReader{}
	decisions := make(chan Decision, 1)
	watcher := New(Config{
		MetricName:         "target",
		BaselineMin:        1,
		EvaluationInterval: 15 * time.Second,
		MinPoints:          6,
	}, reader, DecisionSinkFunc(func(decision Decision) {
		decisions <- decision
	}))

	appendWindow(reader, start, 0, 15, 1)
	require.False(t, watcher.Observe("target", start))
	require.False(t, watcher.Observe("target", start.Add(15*time.Second)))

	decision := requireDecision(t, decisions)
	require.Equal(t, Healthy, decision.State)
	require.Equal(t, float64(1), decision.Value)
	require.Equal(t, float64(1), decision.BaselineMin)
	require.Equal(t, 16, decision.PointCount)
	require.Equal(t, uint64(1), watcher.Decisions())
	require.Equal(t, uint64(0), watcher.Breaches())
}

func TestWatcherEmitsBreachWhenWindowMinimumIsBelowBaselineMin(t *testing.T) {
	start := time.Unix(100, 0)
	reader := &inMemoryReader{}
	decisions := make(chan Decision, 1)
	watcher := New(Config{
		MetricName:         "target",
		BaselineMin:        1,
		EvaluationInterval: 15 * time.Second,
		MinPoints:          6,
	}, reader, DecisionSinkFunc(func(decision Decision) {
		decisions <- decision
	}))

	appendWindow(reader, start, 0, 15, 2)
	reader.points = append(reader.points, Point{Ts: start.Add(7 * time.Second), Value: 0.5})
	require.False(t, watcher.Observe("target", start))
	require.True(t, watcher.Observe("target", start.Add(15*time.Second)))

	decision := requireDecision(t, decisions)
	require.Equal(t, Breach, decision.State)
	require.Equal(t, 0.5, decision.Value)
	require.Equal(t, uint64(1), watcher.Decisions())
	require.Equal(t, uint64(1), watcher.Breaches())
}

func TestWatcherEmitsUnknownForSparseWindow(t *testing.T) {
	start := time.Unix(100, 0)
	reader := &inMemoryReader{points: []Point{{Ts: start, Value: 2}}}
	decisions := make(chan Decision, 1)
	watcher := New(Config{
		MetricName:         "target",
		BaselineMin:        1,
		EvaluationInterval: 15 * time.Second,
		MinPoints:          6,
	}, reader, DecisionSinkFunc(func(decision Decision) {
		decisions <- decision
	}))

	require.False(t, watcher.Observe("target", start))
	require.False(t, watcher.Observe("target", start.Add(15*time.Second)))

	decision := requireDecision(t, decisions)
	require.Equal(t, Unknown, decision.State)
	require.Equal(t, 1, decision.PointCount)
	require.Equal(t, uint64(1), watcher.Decisions())
}

func TestWatcherIgnoresNonMatchingMetric(t *testing.T) {
	watcher := New(Config{MetricName: "target"}, PointReaderFunc(func(_ string, _, _ time.Time) []Point {
		require.FailNow(t, "reader should not be called")
		return nil
	}), DecisionSinkFunc(func(Decision) {
		require.FailNow(t, "decision should not be emitted")
	}))

	require.False(t, watcher.Observe("other", time.Now()))
	require.Equal(t, uint64(0), watcher.Decisions())
}

func TestWatcherIgnoresOutOfOrderObservedTime(t *testing.T) {
	start := time.Unix(100, 0)
	reader := &inMemoryReader{}
	watcher := New(Config{MetricName: "target"}, reader, DecisionSinkFunc(func(Decision) {
		require.FailNow(t, "decision should not be emitted")
	}))

	require.False(t, watcher.Observe("target", start))
	require.False(t, watcher.Observe("target", start.Add(-time.Second)))
	require.Equal(t, uint64(0), watcher.Decisions())
}

func appendWindow(reader *inMemoryReader, start time.Time, fromSecond, toSecond int, value float64) {
	for second := fromSecond; second <= toSecond; second++ {
		reader.points = append(reader.points, Point{
			Ts:    start.Add(time.Duration(second) * time.Second),
			Value: value,
		})
	}
}

func requireDecision(t *testing.T, decisions <-chan Decision) Decision {
	t.Helper()
	select {
	case decision := <-decisions:
		return decision
	case <-time.After(time.Second):
		require.FailNow(t, "timed out waiting for monitor decision")
		return Decision{}
	}
}
