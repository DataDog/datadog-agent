// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

func makeAC(pattern string) observer.ActiveCorrelation {
	return observer.ActiveCorrelation{
		Pattern:     pattern,
		Title:       "test: " + pattern,
		FirstSeen:   100,
		LastUpdated: 100,
	}
}

// TestCorrelationEmitter_FirstSeen verifies that observe emits a
// CorrelationDetected event the first time a pattern is seen.
func TestCorrelationEmitter_FirstSeen(t *testing.T) {
	e := newCorrelationEmitter("test_correlator")
	active := []observer.ActiveCorrelation{makeAC("pattern_a")}
	e.observe(active, 200)

	evts := e.drain()
	if len(evts) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evts))
	}
	ev := evts[0]
	if ev.Kind != observer.CorrelatorEventCorrelationDetected {
		t.Errorf("expected CorrelationDetected, got kind %d", ev.Kind)
	}
	if ev.Correlation.Pattern != "pattern_a" {
		t.Errorf("expected pattern_a, got %q", ev.Correlation.Pattern)
	}
	if ev.CorrelatorName != "test_correlator" {
		t.Errorf("unexpected correlator name %q", ev.CorrelatorName)
	}
	if ev.Timestamp != 200 {
		t.Errorf("expected timestamp 200, got %d", ev.Timestamp)
	}
}

// TestCorrelationEmitter_NoReEmitWhileActive verifies no duplicate events while
// the pattern stays in the active set.
func TestCorrelationEmitter_NoReEmitWhileActive(t *testing.T) {
	e := newCorrelationEmitter("test_correlator")
	active := []observer.ActiveCorrelation{makeAC("pattern_a")}

	e.observe(active, 100)
	_ = e.drain()

	// Same pattern still active — no new event.
	e.observe(active, 101)
	evts := e.drain()
	if len(evts) != 0 {
		t.Errorf("expected 0 events while pattern still active, got %d", len(evts))
	}

	e.observe(active, 102)
	evts = e.drain()
	if len(evts) != 0 {
		t.Errorf("expected 0 events while pattern still active, got %d", len(evts))
	}
}

// TestCorrelationEmitter_ReFireAfterInactive verifies that a pattern re-fires
// after it leaves the active set and then comes back.
func TestCorrelationEmitter_ReFireAfterInactive(t *testing.T) {
	e := newCorrelationEmitter("test_correlator")
	active := []observer.ActiveCorrelation{makeAC("pattern_a")}

	// First activation.
	e.observe(active, 100)
	evts := e.drain()
	if len(evts) != 1 {
		t.Fatalf("expected 1 event on first activation, got %d", len(evts))
	}

	// Pattern goes inactive.
	e.observe(nil, 101)
	evts = e.drain()
	if len(evts) != 0 {
		t.Errorf("expected 0 events when pattern goes inactive, got %d", len(evts))
	}

	// Pattern comes back — should re-fire.
	e.observe(active, 102)
	evts = e.drain()
	if len(evts) != 1 {
		t.Fatalf("expected 1 event on recurrence, got %d", len(evts))
	}
	if evts[0].Kind != observer.CorrelatorEventCorrelationDetected {
		t.Errorf("expected CorrelationDetected on recurrence, got kind %d", evts[0].Kind)
	}
}

// TestCorrelationEmitter_DrainIsIdempotent verifies drain returns nil when called twice.
func TestCorrelationEmitter_DrainIsIdempotent(t *testing.T) {
	e := newCorrelationEmitter("test_correlator")
	e.observe([]observer.ActiveCorrelation{makeAC("p")}, 1)
	_ = e.drain()
	if got := e.drain(); got != nil {
		t.Errorf("expected nil on second drain, got %v", got)
	}
}

// TestCorrelationEmitter_Reset clears all state including seen/pending.
func TestCorrelationEmitter_Reset(t *testing.T) {
	e := newCorrelationEmitter("test_correlator")
	active := []observer.ActiveCorrelation{makeAC("pattern_a")}
	e.observe(active, 100)

	e.reset()

	// Pending should be cleared.
	if evts := e.drain(); len(evts) != 0 {
		t.Errorf("expected no pending events after reset, got %d", len(evts))
	}

	// Seen state should be cleared — pattern re-fires after reset.
	e.observe(active, 101)
	evts := e.drain()
	if len(evts) != 1 {
		t.Fatalf("expected 1 event after reset (pattern treated as new), got %d", len(evts))
	}
}

// TestTimeClusterCorrelator_PendingEvents verifies that a cluster triggers a
// CorrelationDetected event from PendingEvents after Advance.
func TestTimeClusterCorrelator_PendingEvents(t *testing.T) {
	cfg := DefaultTimeClusterConfig()
	c := NewTimeClusterCorrelator(cfg)

	// Add an anomaly to create a cluster.
	c.ProcessAnomaly(makeTestAnomaly("src_a", 1000))
	c.Advance(1000)

	evts := c.PendingEvents()
	if len(evts) != 1 {
		t.Fatalf("expected 1 PendingEvent after cluster formed, got %d", len(evts))
	}
	ev := evts[0]
	if ev.Kind != observer.CorrelatorEventCorrelationDetected {
		t.Errorf("expected CorrelationDetected, got kind %d", ev.Kind)
	}
	if ev.Correlation.Pattern == "" {
		t.Error("expected non-empty Correlation.Pattern")
	}
	if ev.CorrelatorName != "time_cluster_correlator" {
		t.Errorf("unexpected CorrelatorName %q", ev.CorrelatorName)
	}
}

// TestTimeClusterCorrelator_PendingEvents_NoDuplicate verifies that the same
// cluster does not produce duplicate CorrelationDetected events across advances.
func TestTimeClusterCorrelator_PendingEvents_NoDuplicate(t *testing.T) {
	cfg := DefaultTimeClusterConfig()
	c := NewTimeClusterCorrelator(cfg)

	c.ProcessAnomaly(makeTestAnomaly("src_a", 1000))
	c.Advance(1000)
	_ = c.PendingEvents() // drain first event

	// Second advance, same cluster still active — should produce no new event.
	c.Advance(1001)
	evts := c.PendingEvents()
	if len(evts) != 0 {
		t.Errorf("expected 0 events on second advance (no new pattern), got %d", len(evts))
	}
}

// TestTimeClusterCorrelator_PendingEvents_BatchEviction verifies that a cluster
// evicted on the same advance still emits a CorrelationDetected event (emitter
// observes BEFORE evictOldClustersLocked).
func TestTimeClusterCorrelator_PendingEvents_BatchEviction(t *testing.T) {
	cfg := TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    60,
	}
	c := NewTimeClusterCorrelator(cfg)

	// Anomaly at t=0 forms a cluster, then we advance far enough to evict it.
	c.ProcessAnomaly(makeTestAnomaly("src_a", 0))
	// Advance to t=61: cluster (maxTimestamp=0) is outside the 60s window.
	c.Advance(61)

	evts := c.PendingEvents()
	if len(evts) != 1 {
		t.Fatalf("expected 1 CorrelationDetected even though cluster was evicted, got %d", len(evts))
	}
	if evts[0].Kind != observer.CorrelatorEventCorrelationDetected {
		t.Errorf("expected CorrelationDetected, got kind %d", evts[0].Kind)
	}
}

// TestTrackCorrelationHistory_Gating verifies that CorrelationHistory is empty
// in live mode (trackCorrelationHistory=false) and populated in testbench mode.
func TestTrackCorrelationHistory_Gating(t *testing.T) {
	cfg := TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    120,
	}
	tc := NewTimeClusterCorrelator(cfg)
	e := newEngine(engineConfig{storage: newTimeSeriesStorage()})
	e.SetCorrelators([]observer.Correlator{tc})

	// Without TrackCorrelationHistory, AccumulatedCorrelations stays empty.
	tc.ProcessAnomaly(makeTestAnomaly("src_a", 1000))
	_ = e.Advance(1000)

	hist := e.AccumulatedCorrelations()
	if len(hist) != 0 {
		t.Errorf("expected empty CorrelationHistory in live mode, got %d entries", len(hist))
	}

	// Enable via ResetForReplay with TrackCorrelationHistory=true.
	storageCfg := DefaultStorageConfig()
	storageCfg.TrackCorrelationHistory = true
	e.ResetForReplay(nil, []observer.Correlator{tc}, nil, nil, storageCfg, BaselineConfig{})

	tc.ProcessAnomaly(makeTestAnomaly("src_a", 2000))
	_ = e.Advance(2000)

	hist = e.AccumulatedCorrelations()
	if len(hist) == 0 {
		t.Error("expected non-empty CorrelationHistory in testbench mode")
	}
}
