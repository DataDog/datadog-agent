// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trigger

import (
	"math"
	"sync"
	"testing"
	"time"

	"go.uber.org/atomic"
)

func TestMovingAverageSeedsAndSmooths(t *testing.T) {
	m := NewMovingAverage(0.5)
	if got := m.Update(10); got != 10 {
		t.Fatalf("first update should seed to the value, got %v", got)
	}
	if got := m.Update(20); got != 15 {
		t.Fatalf("expected 0.5*20 + 0.5*10 = 15, got %v", got)
	}
	if got := m.Update(0); got != 7.5 {
		t.Fatalf("expected 0.5*0 + 0.5*15 = 7.5, got %v", got)
	}
	if m.Count() != 3 {
		t.Fatalf("expected count 3, got %d", m.Count())
	}
}

func TestMovingAverageAlphaClamped(t *testing.T) {
	for _, alpha := range []float64{0, -1, 2} {
		m := NewMovingAverage(alpha)
		m.Update(3)
		if got := m.Update(7); math.Abs(got-7) > 1e-9 {
			t.Fatalf("alpha %v should clamp to 1 (no smoothing), got %v", alpha, got)
		}
	}
}

func TestWatcherDisabledConstruction(t *testing.T) {
	if w := New(Config{MetricName: ""}, func(time.Time, time.Time) (int, error) { return 0, nil }); w != nil {
		t.Fatal("empty metric name should yield a nil (disabled) watcher")
	}
	if w := New(Config{MetricName: "x"}, nil); w != nil {
		t.Fatal("nil callback should yield a nil (disabled) watcher")
	}
	// A nil watcher must be safe to use.
	var w *Watcher
	if w.Observe("x", 1) {
		t.Fatal("nil watcher should not fire")
	}
	if w.MetricName() != "" || w.Fires() != 0 || w.Average() != 0 {
		t.Fatal("nil watcher accessors should be zero-valued")
	}
}

func TestWatcherIgnoresOtherMetrics(t *testing.T) {
	fired := false
	w := New(Config{MetricName: "watch.me", Threshold: 1, Alpha: 1}, func(time.Time, time.Time) (int, error) {
		fired = true
		return 0, nil
	})
	if w.Observe("something.else", 100) {
		t.Fatal("watcher fired on an unrelated metric")
	}
	if fired || w.Fires() != 0 {
		t.Fatal("callback ran for an unrelated metric")
	}
}

func TestWatcherFiresOnThresholdWithCooldown(t *testing.T) {
	fires := atomic.NewInt64(0)
	var wg sync.WaitGroup
	w := New(Config{MetricName: "cpu.load", Threshold: 5, Alpha: 1, Cooldown: 30 * time.Second}, func(time.Time, time.Time) (int, error) {
		fires.Inc()
		wg.Done()
		return 0, nil
	})

	// Drive a deterministic clock.
	clock := time.Unix(0, 0)
	w.now = func() time.Time { return clock }

	// Below threshold: no fire.
	if w.Observe("cpu.load", 4) {
		t.Fatal("should not fire below threshold")
	}

	// Crosses threshold: fires once.
	wg.Add(1)
	if !w.Observe("cpu.load", 9) {
		t.Fatal("should fire at/above threshold")
	}
	waitForWaitGroup(t, &wg)
	waitForSessionIdle(t, w)

	// Still above threshold but within cooldown: suppressed.
	if w.Observe("cpu.load", 10) {
		t.Fatal("should be suppressed during cooldown")
	}

	// Advance past cooldown: fires again.
	clock = clock.Add(31 * time.Second)
	wg.Add(1)
	if !w.Observe("cpu.load", 8) {
		t.Fatal("should fire again after cooldown")
	}
	waitForWaitGroup(t, &wg)
	waitForSessionIdle(t, w)

	if got := fires.Load(); got != 2 {
		t.Fatalf("expected callback to run twice, got %d", got)
	}
	if w.Fires() != 2 {
		t.Fatalf("expected Fires()==2, got %d", w.Fires())
	}
}

func TestWatcherSmoothingDelaysFire(t *testing.T) {
	// With smoothing, seed the average below the threshold so the first sample
	// does not fire, then confirm a spike pushes the EWMA over the threshold.
	w := New(Config{MetricName: "m", Threshold: 5, Alpha: 0.25, Cooldown: 0}, func(time.Time, time.Time) (int, error) {
		return 0, nil
	})
	if w.Observe("m", 0) {
		t.Fatal("seed below threshold should not fire")
	}
	// 0.25*100 + 0.75*0 = 25, which is >= 5 -> crosses.
	if !w.Observe("m", 100) {
		t.Fatal("large spike should push the average over the threshold")
	}
}

func TestDumpSessionDumpsDelayedIncrementalRanges(t *testing.T) {
	triggeredAt := time.Unix(100, 0)
	clock := triggeredAt
	type dumpRange struct {
		from time.Time
		to   time.Time
		at   time.Time
	}
	var ranges []dumpRange
	w := New(Config{
		MetricName:   "signal",
		Threshold:    1,
		Alpha:        1,
		PreWindow:    15 * time.Second,
		PostWindow:   15 * time.Second,
		DumpInterval: 10 * time.Second,
		SendDelay:    17 * time.Second,
	}, func(from, to time.Time) (int, error) {
		ranges = append(ranges, dumpRange{from: from, to: to, at: clock})
		return 0, nil
	})
	w.now = func() time.Time { return clock }
	w.sleep = func(d time.Duration) { clock = clock.Add(d) }

	w.runDumpSession(triggeredAt)

	expected := []dumpRange{
		{from: time.Unix(85, 0), to: time.Unix(93, 0), at: time.Unix(110, 0)},
		{from: time.Unix(93, 1_000), to: time.Unix(103, 0), at: time.Unix(120, 0)},
		{from: time.Unix(103, 1_000), to: time.Unix(113, 0), at: time.Unix(130, 0)},
		{from: time.Unix(113, 1_000), to: time.Unix(115, 0), at: time.Unix(140, 0)},
	}
	if len(ranges) != len(expected) {
		t.Fatalf("expected %d dump ranges, got %d: %#v", len(expected), len(ranges), ranges)
	}
	for i := range expected {
		if !ranges[i].from.Equal(expected[i].from) || !ranges[i].to.Equal(expected[i].to) || !ranges[i].at.Equal(expected[i].at) {
			t.Fatalf("range %d = [%s, %s] at %s, want [%s, %s] at %s", i, ranges[i].from, ranges[i].to, ranges[i].at, expected[i].from, expected[i].to, expected[i].at)
		}
		if ranges[i].at.Sub(ranges[i].to) < 17*time.Second {
			t.Fatalf("range %d was dumped too early: sent at %s for upper timestamp %s", i, ranges[i].at, ranges[i].to)
		}
	}
}

func waitForWaitGroup(t *testing.T, wg *sync.WaitGroup) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dump callback")
	}
}

func waitForSessionIdle(t *testing.T, w *Watcher) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		w.mu.Lock()
		active := w.sessionActive
		w.mu.Unlock()
		if !active {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for dump session to finish")
}
