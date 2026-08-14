// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package monitor

import (
	"math"
	"sync"
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

func TestWatcherEmitsHealthyWhenWindowRangeEqualsRangeEpsilon(t *testing.T) {
	start := time.Unix(100, 0)
	reader := &inMemoryReader{}
	decisions := make(chan Decision, 1)
	watcher := requireWatcher(t, Config{
		MetricName:         "target",
		RangeEpsilon:       0.25,
		EvaluationInterval: 30 * time.Second,
		MinPoints:          6,
	}, reader, DecisionSinkFunc(func(decision Decision) {
		decisions <- decision
	}))

	appendWindow(reader, start, 0, 30, 1)
	reader.points = append(reader.points, Point{Ts: start.Add(15 * time.Second), Value: 1.25})
	assertObserveSequence(t, watcher, start, false)

	decision := requireDecision(t, decisions)
	require.Equal(t, Healthy, decision.State)
	require.Equal(t, float64(1), decision.Min)
	require.Equal(t, 1.25, decision.Max)
	require.Equal(t, 0.25, decision.Range)
	require.Equal(t, 0.25, decision.RangeEpsilon)
	require.Equal(t, 32, decision.PointCount)
	require.Equal(t, uint64(1), watcher.Decisions())
	require.Equal(t, uint64(0), watcher.Breaches())
}

func TestWatcherEmitsBreachWhenWindowRangeExceedsRangeEpsilon(t *testing.T) {
	start := time.Unix(100, 0)
	reader := &inMemoryReader{}
	decisions := make(chan Decision, 1)
	watcher := requireWatcher(t, Config{
		MetricName:         "target",
		RangeEpsilon:       0.05,
		EvaluationInterval: 30 * time.Second,
		MinPoints:          6,
	}, reader, DecisionSinkFunc(func(decision Decision) {
		decisions <- decision
	}))

	appendWindow(reader, start, 0, 30, 1)
	reader.points = append(reader.points, Point{Ts: start.Add(15 * time.Second), Value: 1.06})
	assertObserveSequence(t, watcher, start, true)

	decision := requireDecision(t, decisions)
	require.Equal(t, Breach, decision.State)
	require.Equal(t, float64(1), decision.Min)
	require.Equal(t, 1.06, decision.Max)
	require.InDelta(t, 0.06, decision.Range, 1e-12)
	require.Equal(t, 0.05, decision.RangeEpsilon)
	require.Equal(t, uint64(1), watcher.Decisions())
	require.Equal(t, uint64(1), watcher.Breaches())
}

func TestWatcherEmitsHealthyForStableLowAbsoluteValue(t *testing.T) {
	start := time.Unix(100, 0)
	reader := &inMemoryReader{}
	decisions := make(chan Decision, 1)
	watcher := requireWatcher(t, Config{
		MetricName:         "target",
		RangeEpsilon:       0.05,
		EvaluationInterval: 30 * time.Second,
		MinPoints:          6,
	}, reader, DecisionSinkFunc(func(decision Decision) {
		decisions <- decision
	}))

	appendWindow(reader, start, 0, 30, -100)
	assertObserveSequence(t, watcher, start, false)

	decision := requireDecision(t, decisions)
	require.Equal(t, Healthy, decision.State)
	require.Equal(t, float64(-100), decision.Min)
	require.Equal(t, float64(-100), decision.Max)
	require.Equal(t, float64(0), decision.Range)
}

func TestWatcherPartitionsWindowRangeByConfiguredTags(t *testing.T) {
	start := time.Unix(100, 0)
	reader := &inMemoryReader{points: []Point{
		{Ts: start, Value: 1.00, Tags: []string{"az:a"}},
		{Ts: start.Add(time.Second), Value: 1.01, Tags: []string{"az:a"}},
		{Ts: start.Add(2 * time.Second), Value: 2.00, Tags: []string{"az:b"}},
		{Ts: start.Add(3 * time.Second), Value: 2.01, Tags: []string{"az:b"}},
	}}
	decisions := make(chan Decision, 1)
	watcher := requireWatcher(t, Config{
		MetricName:         "target",
		RangeEpsilon:       0.05,
		PartitionTags:      []string{"az"},
		EvaluationInterval: 30 * time.Second,
		MinPoints:          2,
	}, reader, DecisionSinkFunc(func(decision Decision) {
		decisions <- decision
	}))

	require.False(t, watcher.Observe("target", start))
	require.False(t, watcher.Observe("target", start.Add(30*time.Second)))

	decision := requireDecision(t, decisions)
	require.Equal(t, Healthy, decision.State)
	require.Equal(t, []string{"az"}, decision.PartitionTags)
	require.Equal(t, 2, decision.PartitionCount)
	require.Contains(t, []string{"az:a", "az:b"}, decision.PartitionKey)
	require.InDelta(t, 0.01, decision.Range, 1e-12)
}

func TestWatcherBreachesWhenAnyPartitionExceedsRangeEpsilon(t *testing.T) {
	start := time.Unix(100, 0)
	reader := &inMemoryReader{points: []Point{
		{Ts: start, Value: 1.00, Tags: []string{"az:a"}},
		{Ts: start.Add(time.Second), Value: 1.01, Tags: []string{"az:a"}},
		{Ts: start.Add(2 * time.Second), Value: 2.00, Tags: []string{"az:b"}},
		{Ts: start.Add(3 * time.Second), Value: 2.20, Tags: []string{"az:b"}},
	}}
	decisions := make(chan Decision, 1)
	watcher := requireWatcher(t, Config{
		MetricName:         "target",
		RangeEpsilon:       0.05,
		PartitionTags:      []string{"az"},
		EvaluationInterval: 30 * time.Second,
		MinPoints:          2,
	}, reader, DecisionSinkFunc(func(decision Decision) {
		decisions <- decision
	}))

	require.False(t, watcher.Observe("target", start))
	require.True(t, watcher.Observe("target", start.Add(30*time.Second)))

	decision := requireDecision(t, decisions)
	require.Equal(t, Breach, decision.State)
	require.Equal(t, "az:b", decision.PartitionKey)
	require.Equal(t, 2, decision.PartitionCount)
	require.Equal(t, 2.00, decision.Min)
	require.Equal(t, 2.20, decision.Max)
	require.InDelta(t, 0.20, decision.Range, 1e-12)
}

func TestWatcherIgnoresSparsePartitionsWhenClassifyingHealthy(t *testing.T) {
	start := time.Unix(100, 0)
	reader := &inMemoryReader{points: []Point{
		{Ts: start, Value: 1.00, Tags: []string{"az:a"}},
		{Ts: start.Add(time.Second), Value: 1.01, Tags: []string{"az:a"}},
		{Ts: start.Add(2 * time.Second), Value: 100.00, Tags: []string{"az:b"}},
	}}
	decisions := make(chan Decision, 1)
	watcher := requireWatcher(t, Config{
		MetricName:         "target",
		RangeEpsilon:       0.05,
		PartitionTags:      []string{"az"},
		EvaluationInterval: 30 * time.Second,
		MinPoints:          2,
	}, reader, DecisionSinkFunc(func(decision Decision) {
		decisions <- decision
	}))

	require.False(t, watcher.Observe("target", start))
	require.False(t, watcher.Observe("target", start.Add(30*time.Second)))

	decision := requireDecision(t, decisions)
	require.Equal(t, Healthy, decision.State)
	require.Equal(t, "az:a", decision.PartitionKey)
	require.Equal(t, 1, decision.PartitionCount)
}

func TestWatcherUsesMissingTagPartition(t *testing.T) {
	start := time.Unix(100, 0)
	reader := &inMemoryReader{points: []Point{
		{Ts: start, Value: 1.00, Tags: []string{"env:prod"}},
		{Ts: start.Add(time.Second), Value: 1.20, Tags: []string{"env:prod"}},
	}}
	decisions := make(chan Decision, 1)
	watcher := requireWatcher(t, Config{
		MetricName:         "target",
		RangeEpsilon:       0.05,
		PartitionTags:      []string{"az"},
		EvaluationInterval: 30 * time.Second,
		MinPoints:          2,
	}, reader, DecisionSinkFunc(func(decision Decision) {
		decisions <- decision
	}))

	require.False(t, watcher.Observe("target", start))
	require.True(t, watcher.Observe("target", start.Add(30*time.Second)))

	decision := requireDecision(t, decisions)
	require.Equal(t, Breach, decision.State)
	require.Equal(t, "az:<missing>", decision.PartitionKey)
}

func TestWatcherEmitsUnknownWhenNoPartitionHasEnoughPoints(t *testing.T) {
	start := time.Unix(100, 0)
	reader := &inMemoryReader{points: []Point{
		{Ts: start, Value: 1.00, Tags: []string{"az:a"}},
		{Ts: start.Add(time.Second), Value: 2.00, Tags: []string{"az:b"}},
	}}
	decisions := make(chan Decision, 1)
	watcher := requireWatcher(t, Config{
		MetricName:         "target",
		RangeEpsilon:       0.05,
		PartitionTags:      []string{"az"},
		EvaluationInterval: 30 * time.Second,
		MinPoints:          2,
	}, reader, DecisionSinkFunc(func(decision Decision) {
		decisions <- decision
	}))

	require.False(t, watcher.Observe("target", start))
	require.False(t, watcher.Observe("target", start.Add(30*time.Second)))

	decision := requireDecision(t, decisions)
	require.Equal(t, Unknown, decision.State)
	require.Equal(t, 2, decision.PointCount)
	require.Equal(t, 2, decision.PartitionCount)
}

func TestWatcherEmitsUnknownForSparseWindow(t *testing.T) {
	start := time.Unix(100, 0)
	reader := &inMemoryReader{points: []Point{{Ts: start, Value: 2}}}
	decisions := make(chan Decision, 1)
	watcher := requireWatcher(t, Config{
		MetricName:         "target",
		RangeEpsilon:       0.05,
		EvaluationInterval: 30 * time.Second,
		MinPoints:          6,
	}, reader, DecisionSinkFunc(func(decision Decision) {
		decisions <- decision
	}))

	require.False(t, watcher.Observe("target", start))
	require.False(t, watcher.Observe("target", start.Add(30*time.Second)))

	decision := requireDecision(t, decisions)
	require.Equal(t, Unknown, decision.State)
	require.Equal(t, 1, decision.PointCount)
	require.Equal(t, 0.05, decision.RangeEpsilon)
	require.Equal(t, uint64(1), watcher.Decisions())
}

func TestWatcherIgnoresNaNAndInfinityInWindowRange(t *testing.T) {
	start := time.Unix(100, 0)
	reader := &inMemoryReader{points: []Point{
		{Ts: start, Value: math.NaN()},
		{Ts: start.Add(time.Second), Value: math.Inf(1)},
		{Ts: start.Add(2 * time.Second), Value: 10},
		{Ts: start.Add(3 * time.Second), Value: 10.04},
	}}
	decisions := make(chan Decision, 1)
	watcher := requireWatcher(t, Config{
		MetricName:         "target",
		RangeEpsilon:       0.05,
		EvaluationInterval: 30 * time.Second,
		MinPoints:          2,
	}, reader, DecisionSinkFunc(func(decision Decision) {
		decisions <- decision
	}))

	require.False(t, watcher.Observe("target", start))
	require.False(t, watcher.Observe("target", start.Add(30*time.Second)))

	decision := requireDecision(t, decisions)
	require.Equal(t, Healthy, decision.State)
	require.Equal(t, 2, decision.PointCount)
	require.Equal(t, float64(10), decision.Min)
	require.Equal(t, 10.04, decision.Max)
	require.InDelta(t, 0.04, decision.Range, 1e-12)
}

func TestWatcherRejectsNegativeRangeEpsilon(t *testing.T) {
	watcher, err := New(Config{
		MetricName:         "target",
		RangeEpsilon:       -1,
		EvaluationInterval: 30 * time.Second,
		MinPoints:          2,
	}, &inMemoryReader{}, DecisionSinkFunc(func(Decision) {
		require.FailNow(t, "decision should not be emitted")
	}))

	require.Nil(t, watcher)
	require.ErrorContains(t, err, "range epsilon")
}

func TestWatcherIgnoresNonMatchingMetric(t *testing.T) {
	watcher := requireWatcher(t, Config{MetricName: "target"}, PointReaderFunc(func(_ string, _, _ time.Time) []Point {
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
	watcher := requireWatcher(t, Config{MetricName: "target"}, reader, DecisionSinkFunc(func(Decision) {
		require.FailNow(t, "decision should not be emitted")
	}))

	require.False(t, watcher.Observe("target", start))
	require.False(t, watcher.Observe("target", start.Add(-time.Second)))
	require.Equal(t, uint64(0), watcher.Decisions())
}

func TestWatcherSerializesConcurrentDecisionsInWindowOrder(t *testing.T) {
	start := time.Unix(100, 0)
	firstWindowTo := start.Add(30 * time.Second)
	secondWindowTo := start.Add(60 * time.Second)

	firstReadStarted := make(chan struct{})
	releaseFirstRead := make(chan struct{})
	secondReadStarted := make(chan struct{})
	var firstOnce sync.Once
	var secondOnce sync.Once
	defer func() {
		select {
		case <-releaseFirstRead:
		default:
			close(releaseFirstRead)
		}
	}()

	reader := PointReaderFunc(func(_ string, from, to time.Time) []Point {
		switch {
		case from.Equal(start) && to.Equal(firstWindowTo):
			firstOnce.Do(func() { close(firstReadStarted) })
			<-releaseFirstRead
			return []Point{
				{Ts: start, Value: 1},
				{Ts: firstWindowTo, Value: 2},
			}
		case from.Equal(firstWindowTo) && to.Equal(secondWindowTo):
			secondOnce.Do(func() { close(secondReadStarted) })
			return []Point{
				{Ts: firstWindowTo, Value: 3},
				{Ts: secondWindowTo, Value: 3.1},
			}
		default:
			return nil
		}
	})
	decisions := make(chan Decision, 2)
	watcher := requireWatcher(t, Config{
		MetricName:         "target",
		RangeEpsilon:       0.5,
		EvaluationInterval: 30 * time.Second,
		MinPoints:          2,
	}, reader, DecisionSinkFunc(func(decision Decision) {
		decisions <- decision
	}))

	require.False(t, watcher.Observe("target", start))

	firstResult := make(chan bool, 1)
	go func() {
		firstResult <- watcher.Observe("target", firstWindowTo)
	}()
	select {
	case <-firstReadStarted:
	case <-time.After(time.Second):
		require.FailNow(t, "timed out waiting for first evaluation to start")
	}

	secondResult := make(chan bool, 1)
	go func() {
		secondResult <- watcher.Observe("target", secondWindowTo)
	}()

	select {
	case <-secondReadStarted:
		require.FailNow(t, "second evaluation started before the first decision was delivered")
	case decision := <-decisions:
		require.FailNow(t, "decision emitted before first evaluation was released", "decision=%+v", decision)
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseFirstRead)

	firstDecision := requireDecision(t, decisions)
	require.Equal(t, firstWindowTo, firstDecision.WindowTo)
	require.Equal(t, Breach, firstDecision.State)

	secondDecision := requireDecision(t, decisions)
	require.Equal(t, secondWindowTo, secondDecision.WindowTo)
	require.Equal(t, Healthy, secondDecision.State)

	require.True(t, <-firstResult)
	require.False(t, <-secondResult)
}

func requireWatcher(t *testing.T, cfg Config, reader PointReader, sink DecisionSink) *Watcher {
	t.Helper()
	watcher, err := New(cfg, reader, sink)
	require.NoError(t, err)
	require.NotNil(t, watcher)
	return watcher
}

func appendWindow(reader *inMemoryReader, start time.Time, fromSecond, toSecond int, value float64) {
	for second := fromSecond; second <= toSecond; second++ {
		reader.points = append(reader.points, Point{
			Ts:    start.Add(time.Duration(second) * time.Second),
			Value: value,
		})
	}
}

func assertObserveSequence(t *testing.T, watcher *Watcher, start time.Time, wantBreach bool) {
	t.Helper()
	require.False(t, watcher.Observe("target", start))
	require.Equal(t, wantBreach, watcher.Observe("target", start.Add(30*time.Second)))
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
