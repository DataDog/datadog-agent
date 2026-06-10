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
	if w := New(Config{MetricName: ""}, func() {}); w != nil {
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
	w := New(Config{MetricName: "watch.me", Threshold: 1, Alpha: 1}, func() { fired = true })
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
	w := New(Config{MetricName: "cpu.load", Threshold: 5, Alpha: 1, Cooldown: 30 * time.Second}, func() {
		fires.Inc()
		wg.Done()
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

	wg.Wait() // ensure both callbacks ran
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
	w := New(Config{MetricName: "m", Threshold: 5, Alpha: 0.25, Cooldown: 0}, func() {})
	if w.Observe("m", 0) {
		t.Fatal("seed below threshold should not fire")
	}
	// 0.25*100 + 0.75*0 = 25, which is >= 5 -> crosses.
	if !w.Observe("m", 100) {
		t.Fatal("large spike should push the average over the threshold")
	}
}
