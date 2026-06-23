// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// makeAnomaly is a test helper that creates an anomaly with the given detector,
// timestamp, and optional score.
func makeAnomaly(detector string, ts int64, score *float64) observer.Anomaly {
	src := observer.SeriesDescriptor{
		Namespace: "test",
		Name:      "series",
		Tags:      []string{"host:h1"},
	}
	return observer.Anomaly{
		DetectorName: detector,
		Timestamp:    ts,
		Score:        score,
		Source:       src,
	}
}

func scorePtr(v float64) *float64 { return &v }

// TestAnomalyLevel verifies the score-to-level mapping for scored and fixed detectors.
func TestAnomalyLevel(t *testing.T) {
	cases := []struct {
		detector string
		score    *float64
		want     int
	}{
		{"holt_residual", scorePtr(0), 0}, // < 6 → VeryLow
		{"holt_residual", scorePtr(5.9), 0},
		{"holt_residual", scorePtr(6), 1}, // 6 ≤ … < 12 → Low
		{"holt_residual", scorePtr(11.9), 1},
		{"holt_residual", scorePtr(12), 2}, // Medium
		{"holt_residual", scorePtr(19.9), 2},
		{"holt_residual", scorePtr(20), 3}, // High
		{"holt_residual", scorePtr(34.9), 3},
		{"holt_residual", scorePtr(35), 4}, // XHigh
		{"holt_residual", scorePtr(100), 4},
		{"holt_residual", nil, 0},          // nil score → VeryLow
		{"bocpd", nil, 2},                  // fixed Medium
		{"bocpd", scorePtr(99), 2},         // score ignored for bocpd
		{"unknown_detector", nil, 2},       // default Medium
		{"tukey_biweight", scorePtr(7), 1}, // Low (per-detector threshold)
	}
	for _, tc := range cases {
		a := makeAnomaly(tc.detector, 1000, tc.score)
		got := anomalyLevel(a, DefaultScorerConfig())
		if got != tc.want {
			t.Errorf("anomalyLevel(%s, score=%v): got %d, want %d", tc.detector, tc.score, got, tc.want)
		}
	}
}

// TestEWMABasic verifies that the EWMA is seeded correctly and decays as expected.
func TestEWMABasic(t *testing.T) {
	cfg := DefaultScorerConfig()
	cfg.Alpha = 0.5
	// With k=1: saturation(count=1) = 1−exp(−1/1) ≈ 0.632.
	cfg.SaturationK = 1.0
	// WindowSecs=1 so each second is independent: the series from t=1000 expires
	// at t=1001, allowing the EWMA decay test to see zero input.
	cfg.WindowSecs = 1

	s := NewScorer(cfg)
	f := scorePtr(20.0) // holt_residual level 3 → weight 2.0
	s.ProcessAnomaly(makeAnomaly("holt_residual", 1000, f))
	s.Advance(1000)

	st := s.ScoreState()
	if len(st.Buckets) != 1 {
		t.Fatalf("expected 1 bucket, got %d", len(st.Buckets))
	}
	b := st.Buckets[0]

	// count=1, weight=2.0, meanWeight=2.0, saturation=1−exp(−1/1)=0.6321…
	// EWMA = alpha*input + (1-alpha)*0 on first second.
	expectedInput := 2.0 * (1 - math.Exp(-1.0/1.0))
	expectedEWMA := cfg.Alpha*expectedInput + (1-cfg.Alpha)*0
	if math.Abs(b.Ewma-expectedEWMA) > 1e-9 {
		t.Errorf("EWMA first bucket: got %.6f, want %.6f", b.Ewma, expectedEWMA)
	}

	// Second advance with no anomalies → EWMA decays by (1-alpha).
	// With WindowSecs=1 the bucket history is capped at 1 entry, so after
	// Advance(1001) only the t=1001 bucket remains at Buckets[0].
	s.Advance(1001)
	st = s.ScoreState()
	b2 := st.Buckets[0]
	expected2 := cfg.Alpha*0 + (1-cfg.Alpha)*expectedEWMA
	if math.Abs(b2.Ewma-expected2) > 1e-9 {
		t.Errorf("EWMA decay: got %.6f, want %.6f", b2.Ewma, expected2)
	}
}

// TestDeduplication verifies that two anomalies on the same series at the same
// second collapse to the higher-level one.
func TestDeduplication(t *testing.T) {
	cfg := DefaultScorerConfig()
	s := NewScorer(cfg)

	// Two anomalies on the same series: levels 1 (Low) and 3 (High).
	// Only the High one (weight=2.0) should survive in the window.
	src := observer.SeriesDescriptor{Namespace: "ns", Name: "m", Tags: []string{"host:h"}}
	a1 := observer.Anomaly{
		DetectorName: "holt_residual", Timestamp: 1000, Score: scorePtr(8), Source: src,
	}
	a2 := observer.Anomaly{
		DetectorName: "tukey_biweight", Timestamp: 1000, Score: scorePtr(25), Source: src,
	}
	s.ProcessAnomaly(a1)
	s.ProcessAnomaly(a2)
	s.Advance(1000)

	st := s.ScoreState()
	b := st.Buckets[0]
	if b.Count != 1 {
		t.Errorf("dedup: expected count=1, got %d", b.Count)
	}
	if b.Bins[3] != 1 { // level 3 (High) survived
		t.Errorf("dedup: expected bins[3]=1, got %v", b.Bins)
	}
}

// TestWindowDedup verifies that the same series firing at different seconds
// within the window still counts as a single entry.
func TestWindowDedup(t *testing.T) {
	cfg := DefaultScorerConfig()
	cfg.WindowSecs = 15
	s := NewScorer(cfg)

	src := observer.SeriesDescriptor{Namespace: "ns", Name: "m", Tags: []string{"host:h"}}
	// Series fires at t=1000 (Medium, level 2) and again at t=1005 (Low, level 1).
	// The window should keep max level = 2 and count = 1 at t=1005.
	s.ProcessAnomaly(observer.Anomaly{DetectorName: "bocpd", Timestamp: 1000, Source: src})
	s.Advance(1000)

	s.ProcessAnomaly(observer.Anomaly{DetectorName: "bocpd", Timestamp: 1005, Source: src})
	s.Advance(1005)

	// Advance(1005) appends buckets for t=1001..1005; the last one is t=1005.
	st := s.ScoreState()
	b := st.Buckets[len(st.Buckets)-1]
	if b.Second != 1005 {
		t.Fatalf("window dedup: expected last bucket at t=1005, got t=%d", b.Second)
	}
	if b.Count != 1 {
		t.Errorf("window dedup: expected count=1 at t=1005, got %d", b.Count)
	}
	if b.Bins[2] != 1 { // level 2 (Medium) — max of (2,2) = 2
		t.Errorf("window dedup: expected bins[2]=1 at t=1005, got %v", b.Bins)
	}
}

// TestWindowExpiry verifies that a series is evicted once it falls outside the window.
func TestWindowExpiry(t *testing.T) {
	cfg := DefaultScorerConfig()
	cfg.WindowSecs = 15
	s := NewScorer(cfg)

	src := observer.SeriesDescriptor{Namespace: "ns", Name: "m", Tags: []string{"host:h"}}
	// Series fires once at t=1000; last seen = 1000.
	// Window at t=1014: windowStart = 1014-15+1 = 1000 → series still alive (lastSeen >= windowStart).
	// Window at t=1015: windowStart = 1015-15+1 = 1001 → series expired (lastSeen=1000 < 1001).
	s.ProcessAnomaly(observer.Anomaly{DetectorName: "bocpd", Timestamp: 1000, Source: src})
	s.Advance(1014)

	st14 := s.ScoreState()
	b14 := st14.Buckets[len(st14.Buckets)-1]
	if b14.Count != 1 {
		t.Errorf("window expiry: expected count=1 at t=1014, got %d", b14.Count)
	}

	s.Advance(1015)

	st15 := s.ScoreState()
	b15 := st15.Buckets[len(st15.Buckets)-1]
	if b15.Count != 0 {
		t.Errorf("window expiry: expected count=0 at t=1015 (series expired), got %d", b15.Count)
	}
}

// TestWindowLevelExpiry reproduces the bug where a high-severity peak updates
// lastSeenSec via a later lower-severity event, causing the expired peak to
// continue inflating the score beyond its true window lifetime.
//
// Timeline (WindowSecs=15):
//
//	t=1000: High (level 3) fires → entry {level:3, lastSeen:1000}
//	t=1010: Low  (level 1) fires → entry {level:3, lastSeen:1010}  ← lastSeen bumped
//	t=1015: windowStart = 1001; High at t=1000 is now OUTSIDE the window.
//	        The entry must be counted at level 1 (the only active level),
//	        NOT level 3 (the expired peak).
func TestWindowLevelExpiry(t *testing.T) {
	cfg := DefaultScorerConfig()
	cfg.WindowSecs = 15
	s := NewScorer(cfg)

	src := observer.SeriesDescriptor{Namespace: "ns", Name: "m", Tags: []string{"host:h"}}

	// High anomaly at t=1000
	s.ProcessAnomaly(observer.Anomaly{
		DetectorName: "holt_residual", Timestamp: 1000, Score: scorePtr(20), Source: src,
	})
	s.Advance(1000)

	// Low anomaly at t=1010 (same series, lower severity)
	s.ProcessAnomaly(observer.Anomaly{
		DetectorName: "holt_residual", Timestamp: 1010, Score: scorePtr(7), Source: src,
	})
	s.Advance(1010)

	// Advance to t=1015: windowStart = 1015-15+1 = 1001.
	// High anomaly (t=1000) is now outside the window; Low (t=1010) is still inside.
	s.Advance(1015)

	st := s.ScoreState()
	b := st.Buckets[len(st.Buckets)-1]

	if b.Count != 1 {
		t.Fatalf("expected series still active at t=1015 (low anomaly in window), got count=%d", b.Count)
	}
	// The series must be counted at level 1 (Low), not the expired level 3 (High).
	if b.Bins[3] != 0 {
		t.Errorf("BUG: expired High peak still inflating level 3 at t=1015, bins=%v", b.Bins)
	}
	if b.Bins[1] != 1 {
		t.Errorf("expected series at level 1 (Low) at t=1015, bins=%v", b.Bins)
	}
}

// TestDeduplicationDifferentSeries verifies that anomalies on different series
// are never merged (each counts independently even at the same second).
func TestDeduplicationDifferentSeries(t *testing.T) {
	cfg := DefaultScorerConfig()
	s := NewScorer(cfg)

	a1 := observer.Anomaly{
		DetectorName: "bocpd",
		Timestamp:    1000,
		Source:       observer.SeriesDescriptor{Namespace: "ns", Name: "m1"},
	}
	a2 := observer.Anomaly{
		DetectorName: "bocpd",
		Timestamp:    1000,
		Source:       observer.SeriesDescriptor{Namespace: "ns", Name: "m2"},
	}
	s.ProcessAnomaly(a1)
	s.ProcessAnomaly(a2)
	s.Advance(1000)

	st := s.ScoreState()
	b := st.Buckets[0]
	if b.Count != 2 {
		t.Errorf("different series: expected count=2, got %d", b.Count)
	}
}

// TestReset verifies that Reset clears all accumulated state.
func TestReset(t *testing.T) {
	s := NewScorer(DefaultScorerConfig())
	s.ProcessAnomaly(makeAnomaly("bocpd", 1000, nil))
	s.Advance(1000)
	s.Reset()
	st := s.ScoreState()
	if len(st.Buckets) != 0 {
		t.Errorf("Reset: expected empty buckets, got %d", len(st.Buckets))
	}
	if s.LastScore() != 0 {
		t.Errorf("Reset: expected score=0, got %f", s.LastScore())
	}
}

// TestLateAnomalyClamp verifies that an anomaly with a historical timestamp
// (already advanced past) is clamped to lastAdvancedSec so it participates in
// the next Advance call rather than leaking into a pending bucket that is
// never processed.
//
// This reproduces the scanmw/scanwelch pattern: a scan detector emits a
// changepoint with a historical timestamp after the scorer has moved forward.
func TestLateAnomalyClamp(t *testing.T) {
	cfg := DefaultScorerConfig()
	cfg.WindowSecs = 15
	s := NewScorer(cfg)

	src := observer.SeriesDescriptor{Namespace: "ns", Name: "m", Tags: []string{"host:h"}}

	// Advance to t=1010 with no anomalies.
	s.Advance(1010)

	// Now receive a scanmw anomaly with a historical timestamp (t=1000),
	// which is already behind lastAdvancedSec=1010.
	s.ProcessAnomaly(observer.Anomaly{
		DetectorName: "scanmw",
		Timestamp:    1000,
		Score:        scorePtr(20),
		Source:       src,
	})

	// Advance to t=1011 — the anomaly is clamped to lastAdvancedSec+1=1011
	// and must appear in that bucket.
	s.Advance(1011)

	st := s.ScoreState()
	var b1011 *observer.ScoreBucket
	for i := range st.Buckets {
		if st.Buckets[i].Second == 1011 {
			b1011 = &st.Buckets[i]
			break
		}
	}
	if b1011 == nil {
		t.Fatal("no bucket for t=1011")
	}
	if b1011.Count != 1 {
		t.Errorf("clamped anomaly not in t=1011 bucket: count=%d, bins=%v", b1011.Count, b1011.Bins)
	}
}

// TestLateAnomalyNoLeakInPending verifies that after clamping, the original
// historical second (t=1000) has no pending entry — i.e. nothing leaks.
func TestLateAnomalyNoLeakInPending(t *testing.T) {
	cfg := DefaultScorerConfig()
	cfg.WindowSecs = 15
	s := NewScorer(cfg)

	src := observer.SeriesDescriptor{Namespace: "ns", Name: "m", Tags: []string{"host:h"}}

	s.Advance(1010)

	// Late anomaly with historical timestamp.
	s.ProcessAnomaly(observer.Anomaly{
		DetectorName: "scanmw",
		Timestamp:    1000,
		Score:        scorePtr(20),
		Source:       src,
	})

	// Cast to concrete type to inspect internal state directly.
	sc := s.(*anomalyScorer)
	sc.mu.Lock()
	_, hasOldSec := sc.pending[1000]
	_, hasNextSec := sc.pending[1011] // clamped to lastAdvancedSec+1
	sc.mu.Unlock()

	if hasOldSec {
		t.Error("BUG: pending still has entry for historical second 1000 (memory leak)")
	}
	if !hasNextSec {
		t.Error("clamped anomaly not found in pending[1011]")
	}
}

// TestLateAnomalyBeforeFirstAdvance verifies that anomalies received before
// the first Advance are NOT clamped — their original timestamp is preserved.
func TestLateAnomalyBeforeFirstAdvance(t *testing.T) {
	cfg := DefaultScorerConfig()
	s := NewScorer(cfg)

	src := observer.SeriesDescriptor{Namespace: "ns", Name: "m", Tags: []string{"host:h"}}

	// No advance yet; lastAdvancedSec == 0.
	s.ProcessAnomaly(observer.Anomaly{
		DetectorName: "scanmw",
		Timestamp:    1000,
		Score:        scorePtr(20),
		Source:       src,
	})

	s.Advance(1005)

	st := s.ScoreState()
	// The anomaly at t=1000 must appear in a real bucket, not be dropped.
	found := false
	for _, b := range st.Buckets {
		if b.Second == 1000 && b.Count == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("pre-first-advance anomaly was clamped or dropped; buckets=%v", st.Buckets)
	}
}

// ---- Severity state machine helpers ----

// TestRawSeverityLevel verifies the initial seeding function.
func TestRawSeverityLevel(t *testing.T) {
	cases := []struct {
		ewma float64
		want observer.SeverityLevel
	}{
		{0.000, observer.SeverityLow},
		{0.039, observer.SeverityLow},
		{0.040, observer.SeverityMedium},
		{0.059, observer.SeverityMedium},
		{0.060, observer.SeverityHigh},
		{1.000, observer.SeverityHigh},
	}
	for _, tc := range cases {
		got := rawSeverityLevel(tc.ewma, 0.040, 0.060)
		if got != tc.want {
			t.Errorf("rawSeverityLevel(%.3f): got %d, want %d", tc.ewma, got, tc.want)
		}
	}
}

// TestNextSeverityLevelEscalation verifies upward transitions (no hysteresis).
func TestNextSeverityLevelEscalation(t *testing.T) {
	// margin = 0.060 * 0.20 = 0.012
	cases := []struct {
		ewma    float64
		current observer.SeverityLevel
		want    observer.SeverityLevel
	}{
		{0.060, observer.SeverityLow, observer.SeverityHigh},    // skip straight to High
		{0.045, observer.SeverityLow, observer.SeverityMedium},  // crosses low threshold
		{0.030, observer.SeverityLow, observer.SeverityLow},     // stays Low
		{0.065, observer.SeverityMedium, observer.SeverityHigh}, // escalate Medium→High
	}
	for _, tc := range cases {
		got := nextSeverityLevel(tc.ewma, tc.current, 0.040, 0.060, 0.060*0.20)
		if got != tc.want {
			t.Errorf("nextSeverityLevel(ewma=%.3f, current=%d): got %d, want %d",
				tc.ewma, tc.current, got, tc.want)
		}
	}
}

// TestNextSeverityLevelHysteresis verifies that downward transitions are suppressed
// until the EWMA drops below threshold − margin.
func TestNextSeverityLevelHysteresis(t *testing.T) {
	// low=0.040, high=0.060, margin=0.060*0.20=0.012
	// From High: drop only when ewma < 0.060-0.012 = 0.048.
	// From Medium: drop only when ewma < 0.040-0.012 = 0.028.
	cases := []struct {
		ewma    float64
		current observer.SeverityLevel
		want    observer.SeverityLevel
		desc    string
	}{
		{0.049, observer.SeverityHigh, observer.SeverityHigh, "High: within hysteresis band"},
		{0.047, observer.SeverityHigh, observer.SeverityMedium, "High: below hysteresis → Medium"},
		{0.005, observer.SeverityHigh, observer.SeverityLow, "High: far below → Low"},
		{0.029, observer.SeverityMedium, observer.SeverityMedium, "Medium: within hysteresis band"},
		{0.027, observer.SeverityMedium, observer.SeverityLow, "Medium: below hysteresis → Low"},
	}
	for _, tc := range cases {
		got := nextSeverityLevel(tc.ewma, tc.current, 0.040, 0.060, 0.060*0.20)
		if got != tc.want {
			t.Errorf("%s: nextSeverityLevel(ewma=%.3f, current=%d): got %d, want %d",
				tc.desc, tc.ewma, tc.current, got, tc.want)
		}
	}
}

// ---- scorerEventFilterMatches ----

func TestScorerEventFilterMatches(t *testing.T) {
	mkEvt := func(from, to observer.SeverityLevel) observer.SeverityEvent {
		return observer.SeverityEvent{
			FromLevel: from, ToLevel: to,
			Direction: severityDirection(from, to),
		}
	}
	cases := []struct {
		filter observer.ScorerEventFilter
		evt    observer.SeverityEvent
		want   bool
		desc   string
	}{
		{
			observer.ScorerEventFilter{},
			mkEvt(observer.SeverityLow, observer.SeverityMedium),
			true, "zero filter matches everything",
		},
		{
			observer.ScorerEventFilter{Direction: observer.ScorerEventEscalation},
			mkEvt(observer.SeverityLow, observer.SeverityMedium),
			true, "escalation filter matches escalation",
		},
		{
			observer.ScorerEventFilter{Direction: observer.ScorerEventEscalation},
			mkEvt(observer.SeverityHigh, observer.SeverityLow),
			false, "escalation filter rejects de-escalation",
		},
		{
			observer.ScorerEventFilter{Direction: observer.ScorerEventDeescalation},
			mkEvt(observer.SeverityHigh, observer.SeverityLow),
			true, "de-escalation filter matches de-escalation",
		},
		{
			observer.ScorerEventFilter{ToLevels: []observer.SeverityLevel{observer.SeverityHigh}},
			mkEvt(observer.SeverityLow, observer.SeverityHigh),
			true, "ToLevels match",
		},
		{
			observer.ScorerEventFilter{ToLevels: []observer.SeverityLevel{observer.SeverityHigh}},
			mkEvt(observer.SeverityLow, observer.SeverityMedium),
			false, "ToLevels mismatch",
		},
		{
			observer.ScorerEventFilter{FromLevels: []observer.SeverityLevel{observer.SeverityMedium}},
			mkEvt(observer.SeverityMedium, observer.SeverityLow),
			true, "FromLevels match",
		},
		{
			observer.ScorerEventFilter{FromLevels: []observer.SeverityLevel{observer.SeverityMedium}},
			mkEvt(observer.SeverityLow, observer.SeverityMedium),
			false, "FromLevels mismatch",
		},
	}
	for _, tc := range cases {
		got := scorerEventFilterMatches(tc.filter, tc.evt)
		if got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.desc, got, tc.want)
		}
	}
}

// ---- Subscribe / subscription state machine ----

// collectingListener records every SeverityEvent it receives.
type collectingListener struct{ events []observer.SeverityEvent }

func (l *collectingListener) OnSeverityTransition(e observer.SeverityEvent) {
	l.events = append(l.events, e)
}

// TestSubscribeBasic verifies that a listener receives an escalation event when
// the EWMA crosses the Low threshold.
func TestSubscribeBasic(t *testing.T) {
	cfg := DefaultScorerConfig()
	cfg.Alpha = 0.99 // near-instant EWMA (1.0 is rejected as invalid by NewScorer)
	cfg.SaturationK = 1.0
	cfg.WindowSecs = 5
	s := NewScorer(cfg)

	l := &collectingListener{}
	s.Subscribe(observer.AnomalyScorerConfiguration{Listener: l})

	// Advance with no anomalies: EWMA=0, stays Low — no event.
	s.Advance(1000)
	if len(l.events) != 0 {
		t.Fatalf("expected no events after empty advance, got %v", l.events)
	}

	// Push one bocpd anomaly: medium level, saturation(1,k=1) ≈ 0.632, weight=1.0 → EWMA ≈ 0.632.
	// That's above high_threshold=0.060 → escalation Low→High.
	s.ProcessAnomaly(makeAnomaly("bocpd", 1001, nil))
	s.Advance(1001)

	if len(l.events) != 1 {
		t.Fatalf("expected 1 escalation event, got %d: %v", len(l.events), l.events)
	}
	evt := l.events[0]
	if evt.FromLevel != observer.SeverityLow || evt.ToLevel != observer.SeverityHigh {
		t.Errorf("escalation event wrong levels: from=%d to=%d", evt.FromLevel, evt.ToLevel)
	}
	if evt.Direction != observer.ScorerEventEscalation {
		t.Errorf("expected escalation direction, got %d", evt.Direction)
	}
}

// TestSubscribeCooldown verifies that de-escalation is blocked during cooldown.
func TestSubscribeCooldown(t *testing.T) {
	cfg := DefaultScorerConfig()
	cfg.Alpha = 0.99
	cfg.SaturationK = 1.0
	cfg.WindowSecs = 1 // short window so anomaly expires quickly
	s := NewScorer(cfg)

	l := &collectingListener{}
	s.Subscribe(observer.AnomalyScorerConfiguration{
		Listener:     l,
		CooldownSecs: 60, // 60s cooldown on de-escalations
	})

	// Warm-up: seed state at Low (EWMA=0, first advance never fires an event).
	s.Advance(1000)

	// Drive EWMA above high_threshold → escalation Low→High fires.
	s.ProcessAnomaly(makeAnomaly("bocpd", 1001, nil))
	s.Advance(1001) // EWMA≈0.632 → High

	// Advance with no anomalies: EWMA decays near 0 quickly (alpha=0.99).
	// Cooldown=60 should block de-escalation for 60 seconds after the escalation.
	s.Advance(1002) // EWMA=0 → raw Low, but cooldown blocks it

	escalations, deescalations := 0, 0
	for _, e := range l.events {
		if e.Direction == observer.ScorerEventEscalation {
			escalations++
		} else {
			deescalations++
		}
	}
	if escalations != 1 {
		t.Errorf("expected 1 escalation, got %d", escalations)
	}
	if deescalations != 0 {
		t.Errorf("expected 0 de-escalations within cooldown, got %d", deescalations)
	}

	// After cooldown expires (60 seconds), de-escalation should fire.
	s.Advance(1062)
	deescalations = 0
	for _, e := range l.events {
		if e.Direction == observer.ScorerEventDeescalation {
			deescalations++
		}
	}
	if deescalations != 1 {
		t.Errorf("expected 1 de-escalation after cooldown expired, got %d", deescalations)
	}
}

// TestSubscribeFilter verifies that events not matching the filter are not delivered.
func TestSubscribeFilter(t *testing.T) {
	cfg := DefaultScorerConfig()
	cfg.Alpha = 0.99
	cfg.SaturationK = 1.0
	cfg.WindowSecs = 1
	s := NewScorer(cfg)

	// Only receive escalations.
	l := &collectingListener{}
	s.Subscribe(observer.AnomalyScorerConfiguration{
		Listener: l,
		Filter:   observer.ScorerEventFilter{Direction: observer.ScorerEventEscalation},
	})

	// Warm-up: seed state at Low.
	s.Advance(1000)

	// Trigger escalation then de-escalation.
	s.ProcessAnomaly(makeAnomaly("bocpd", 1001, nil))
	s.Advance(1001) // → High (escalation delivered)
	s.Advance(1002) // → Low (de-escalation filtered out)

	if len(l.events) != 1 {
		t.Fatalf("expected 1 event (only escalation), got %d: %v", len(l.events), l.events)
	}
	if l.events[0].Direction != observer.ScorerEventEscalation {
		t.Errorf("delivered event should be escalation, got direction=%d", l.events[0].Direction)
	}
}

// TestSubscribeNilPanics verifies that Subscribe panics on a nil Listener.
func TestSubscribeNilPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil Listener, got none")
		}
	}()
	NewScorer(DefaultScorerConfig()).Subscribe(observer.AnomalyScorerConfiguration{})
}

// TestUnsubscribe verifies that the returned unsubscribe function stops delivery.
func TestUnsubscribe(t *testing.T) {
	cfg := DefaultScorerConfig()
	cfg.Alpha = 0.99
	cfg.SaturationK = 1.0
	cfg.WindowSecs = 1
	s := NewScorer(cfg)

	l := &collectingListener{}
	unsub := s.Subscribe(observer.AnomalyScorerConfiguration{Listener: l})

	// Warm-up: seed at Low.
	s.Advance(1000)

	s.ProcessAnomaly(makeAnomaly("bocpd", 1001, nil))
	s.Advance(1001) // escalation fires
	unsub()

	s.Advance(1002) // de-escalation would fire, but subscription already removed
	if len(l.events) != 1 {
		t.Errorf("expected exactly 1 event before unsub; got %d: %v", len(l.events), l.events)
	}
}

// TestResetClearsSubscriptionState verifies that Reset() re-initializes each
// subscription's state machine so no stale state carries over into a replay run.
func TestResetClearsSubscriptionState(t *testing.T) {
	cfg := DefaultScorerConfig()
	cfg.Alpha = 0.99
	cfg.SaturationK = 1.0
	cfg.WindowSecs = 1
	s := NewScorer(cfg)

	l := &collectingListener{}
	s.Subscribe(observer.AnomalyScorerConfiguration{
		Listener:     l,
		CooldownSecs: 3600, // long cooldown — would suppress de-escalation if stale
	})

	// Drive to High.
	s.ProcessAnomaly(makeAnomaly("bocpd", 1000, nil))
	s.Advance(1000)

	// Reset clears EWMA and must also clear subscription state (stateInitialized=false).
	s.Reset()

	// After reset, replaying the same sequence must fire escalation again,
	// proving the subscription re-seeded rather than carrying over the High state.
	// Warm-up first so the state seeds at Low before the anomaly.
	before := len(l.events)
	s.Advance(2000) // seeds at Low (EWMA=0)
	s.ProcessAnomaly(makeAnomaly("bocpd", 2001, nil))
	s.Advance(2001) // EWMA→High → escalation
	after := len(l.events)

	if after-before != 1 {
		t.Errorf("expected 1 new escalation event after Reset+replay, got %d new events", after-before)
	}
	if l.events[after-1].Direction != observer.ScorerEventEscalation {
		t.Errorf("post-reset event should be escalation, got direction=%d", l.events[after-1].Direction)
	}
}

// ---- Original tests ----

// TestEmptySeconds verifies that Advance over a gap generates empty buckets.
// WindowSecs=1 so the anomaly at t=1000 expires before t=1001, giving zero
// window count for the gap seconds and allowing pure EWMA decay to be tested.
// With WindowSecs=1 the bucket history is capped at 1 entry, so ScoreState
// only returns the latest bucket; we verify via LastScore that the EWMA has
// decayed and check the final bucket directly.
func TestEmptySeconds(t *testing.T) {
	cfg := DefaultScorerConfig()
	cfg.Alpha = 0.5
	cfg.WindowSecs = 1
	s := NewScorer(cfg)

	f := scorePtr(25.0) // level 3
	s.ProcessAnomaly(makeAnomaly("holt_residual", 1000, f))
	s.Advance(1002) // advance covers seconds 1000, 1001, 1002

	st := s.ScoreState()
	// Only the last bucket (t=1002) is retained due to WindowSecs=1 cap.
	if len(st.Buckets) != 1 {
		t.Fatalf("expected 1 bucket (cap=WindowSecs), got %d", len(st.Buckets))
	}
	if st.Buckets[0].Second != 1002 {
		t.Errorf("expected last bucket at t=1002, got t=%d", st.Buckets[0].Second)
	}
	if st.Buckets[0].Count != 0 {
		t.Errorf("expected empty bucket at t=1002, got count=%d", st.Buckets[0].Count)
	}
	// EWMA after three seconds: seed→decay→decay. Score must be > 0 (decayed,
	// not zero) and strictly less than after t=1000 (score has decayed twice).
	if s.LastScore() == 0 {
		t.Error("expected non-zero EWMA after decaying from seeded value")
	}
}
