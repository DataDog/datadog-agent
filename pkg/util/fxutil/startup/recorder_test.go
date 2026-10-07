// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package startup

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDisabledRecorder(t *testing.T) {
	for _, recorder := range []*Recorder{nil, NewRecorder(false)} {
		recorder.BeginHook(42)
		require.Zero(t, testing.AllocsPerRun(100, func() {
			phase := recorder.Start("operation", "resource")
			phase.SetMetric("count", 1)
			phase.Start("nested", "resource").Finish(nil)
			phase.Finish(nil)
		}))
		recorder.EndHook()
		events, dropped := recorder.Drain()
		require.Empty(t, events)
		require.Zero(t, dropped)
	}
}

func TestNestedPhases(t *testing.T) {
	r := NewRecorder(true)
	now := time.Now()
	r.now = func() time.Time { return now }
	require.Nil(t, r.Start("outside", "hook"))
	r.BeginHook(42)
	parent := r.Start("initialize", "listeners")
	now = now.Add(time.Second)
	child := parent.Start("factory", "kubelet")
	now = now.Add(2 * time.Second)
	child.Finish(errors.New("sensitive error text must not be retained"))
	child.Finish(nil)
	now = now.Add(time.Second)
	parent.Finish(nil)
	r.EndHook()

	events, dropped := r.Drain()
	require.Zero(t, dropped)
	require.Len(t, events, 2)
	require.Equal(t, "factory", events[1].Name)
	require.Equal(t, "kubelet", events[1].Resource)
	require.Equal(t, 2*time.Second, events[1].Duration)
	require.True(t, events[1].Failed)
	require.Equal(t, events[0].SpanID, events[1].ParentID)
	require.EqualValues(t, 42, events[0].ParentID)
	require.Equal(t, 4*time.Second, events[0].Duration)
	require.False(t, events[0].Failed)
	require.False(t, events[0].Incomplete)
	require.False(t, events[1].Incomplete)
}

func TestHookAndStartupBoundaries(t *testing.T) {
	r := NewRecorder(true)
	r.BeginHook(1)
	old := r.Start("old", "hook")
	r.EndHook()
	require.Nil(t, r.Start("outside", "hook"))
	r.BeginHook(2)
	old.Finish(nil)
	require.Nil(t, old.Start("late-child", "old-hook"))
	r.Start("current", "hook").Finish(nil)
	unfinished := r.Start("unfinished", "hook")
	events, _ := r.Drain()
	require.Len(t, events, 3)
	require.EqualValues(t, 1, events[0].ParentID)
	require.True(t, events[0].Incomplete)
	require.EqualValues(t, 2, events[1].ParentID)
	require.False(t, events[1].Incomplete)
	require.True(t, events[2].Incomplete)
	unfinished.Finish(nil)
	r.BeginHook(3)
	require.Nil(t, r.Start("after-startup", "hook"))
	events, dropped := r.Drain()
	require.Empty(t, events)
	require.Zero(t, dropped)
}

func TestPhaseLimit(t *testing.T) {
	r := NewRecorder(true)
	r.BeginHook(42)
	for range maxPhases + 7 {
		r.Start("bounded", "phase").Finish(nil)
	}
	events, dropped := r.Drain()
	require.Len(t, events, maxPhases)
	require.Equal(t, 7, dropped)
}

func TestConcurrentPhasesAndDrain(t *testing.T) {
	r := NewRecorder(true)
	r.BeginHook(42)
	phases := make([]*Phase, 32)
	for i := range phases {
		phases[i] = r.Start("concurrent", "phase")
	}
	var workers sync.WaitGroup
	for _, phase := range phases {
		workers.Go(func() {
			phase.Finish(nil)
			phase.Finish(nil)
		})
	}
	// Race with completion: each phase appears once, complete or incomplete.
	// Late completions must not mutate the snapshot handed to the sender.
	events, _ := r.Drain()
	snapshot := append([]Event(nil), events...)
	workers.Wait()
	require.Equal(t, snapshot, events)
	require.Len(t, events, len(phases))
	seen := make(map[uint64]bool)
	for _, event := range events {
		require.False(t, seen[event.SpanID])
		seen[event.SpanID] = true
		require.EqualValues(t, 42, event.ParentID)
	}
	events, _ = r.Drain()
	require.Empty(t, events)
}

func TestPhaseMetricsAreBoundedAndImmutable(t *testing.T) {
	r := NewRecorder(true)
	r.BeginHook(1)
	phase := r.Start("phase", "resource")
	phase.SetMetric("invalid_nan", math.NaN())
	phase.SetMetric("invalid_inf", math.Inf(1))
	for i := range maxPhaseMetrics + 1 {
		phase.SetMetric(fmt.Sprintf("metric_%d", i), float64(i))
	}
	phase.SetMetric("metric_0", 42) // Existing keys can still be updated at the cap.
	phase.Finish(nil)
	phase.SetMetric("metric_0", 100)
	partial := r.Start("partial", "resource")
	partial.SetMetric("count", 7)
	events, _ := r.Drain()
	partial.SetMetric("count", 99)
	require.Len(t, events[0].Metrics, maxPhaseMetrics)
	require.Equal(t, float64(42), events[0].Metrics["metric_0"])
	require.NotContains(t, events[0].Metrics, "invalid_nan")
	require.NotContains(t, events[0].Metrics, "invalid_inf")
	require.True(t, events[1].Incomplete)
	require.Equal(t, float64(7), events[1].Metrics["count"])
}

func TestConcurrentPhaseMetricsAndDrain(t *testing.T) {
	r := NewRecorder(true)
	r.BeginHook(1)
	phase := r.Start("phase", "resource")
	var workers sync.WaitGroup
	for range 32 {
		workers.Go(func() { phase.SetMetric("count", 1) })
	}
	events, _ := r.Drain()
	before := events[0].Metrics["count"]
	workers.Wait()
	require.Equal(t, before, events[0].Metrics["count"])
}

func TestRecordersAreIndependent(t *testing.T) {
	first, second := NewRecorder(true), NewRecorder(true)
	first.BeginHook(1)
	second.BeginHook(2)
	first.Start("first", "app").Finish(nil)
	first.Drain()
	second.Start("second", "app").Finish(nil)
	events, _ := second.Drain()
	require.Len(t, events, 1)
	require.Equal(t, "second", events[0].Name)
	require.EqualValues(t, 2, events[0].ParentID)
}
