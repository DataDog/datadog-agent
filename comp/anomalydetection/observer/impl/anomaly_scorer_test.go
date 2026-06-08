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
		{"tukey_biweight", scorePtr(7), 1}, // Low
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
	cfg.SaturationK = 1e9 // very large k → saturation ≈ 0 for count=1 → input ≈ 0
	// With k→∞, saturation factor = 1−exp(−1/1e9) ≈ 1e-9 ≈ 0.
	// Instead use k=1 so saturation(1) = 1−exp(−1) ≈ 0.632.
	cfg.SaturationK = 1.0

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
	// EWMA always uses the formula: alpha*input + (1-alpha)*0 = alpha*input on first second.
	expectedInput := 2.0 * (1 - math.Exp(-1.0/1.0))
	expectedEWMA := cfg.Alpha*expectedInput + (1-cfg.Alpha)*0
	if math.Abs(b.Ewma-expectedEWMA) > 1e-9 {
		t.Errorf("EWMA first bucket: got %.6f, want %.6f", b.Ewma, expectedEWMA)
	}

	// Second advance with no anomalies → EWMA decays by (1-alpha)
	s.Advance(1001)
	st = s.ScoreState()
	b2 := st.Buckets[1]
	expected2 := cfg.Alpha*0 + (1-cfg.Alpha)*expectedEWMA
	if math.Abs(b2.Ewma-expected2) > 1e-9 {
		t.Errorf("EWMA decay: got %.6f, want %.6f", b2.Ewma, expected2)
	}
}

// TestDeduplication verifies that two anomalies on the same series at the same
// second collapse to the higher-level one.
func TestDeduplication(t *testing.T) {
	cfg := DefaultScorerConfig()
	cfg.SaturationK = 1e9 // effectively no saturation factor noise
	s := NewScorer(cfg)

	// Two anomalies on the same series at the same second: levels 1 (Low) and 3 (High).
	// Only the High one (weight=2.0) should survive.
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

// TestDeduplicationDifferentSeries verifies that anomalies on different series
// are never merged (each counts independently even at the same second).
func TestDeduplicationDifferentSeries(t *testing.T) {
	cfg := DefaultScorerConfig()
	s := NewScorer(cfg)

	// Two bocpd anomalies on different series at the same second → both counted.
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

// TestSeverityStateTransitions exercises all state-machine paths.
func TestSeverityStateTransitions(t *testing.T) {
	cfg := DefaultScorerConfig()
	cfg.Alpha = 1.0 // instant EWMA (= saturated input)
	cfg.SaturationK = 5.0
	// With k=5, count=1: saturation = 1−exp(−1/5) ≈ 0.181.
	// With α=1: ewma = meanWeight × 0.181.
	// Level 0 weight=0.2: ewma ≈ 0.036 < low=0.040 → initial state Low ✓
	// Level 3 weight=2.0: ewma ≈ 0.362 > high+margin=0.072 → High ✓

	scoreHigh := scorePtr(25.0) // → level 3, weight 2.0
	scoreLow := scorePtr(0.5)   // → level 0, weight 0.2

	s := NewScorer(cfg)
	var ts int64 = 1000

	// Confirm initial Low state.
	s.ProcessAnomaly(makeAnomaly("holt_residual", ts, scoreLow))
	s.Advance(ts)
	st := s.ScoreState()
	if len(st.Events) != 0 {
		t.Errorf("expected no events on init (low input), got %v", st.Events)
	}
	ts++

	// Drive EWMA above high+margin → expect Low→High.
	s.ProcessAnomaly(makeAnomaly("holt_residual", ts, scoreHigh))
	s.Advance(ts)
	st = s.ScoreState()
	if len(st.Events) < 1 || st.Events[len(st.Events)-1].ToLevel != observer.SeverityHigh {
		t.Errorf("expected transition to High, events: %v", st.Events)
	}
	ts++

	// With no anomalies, EWMA = α*0 + (1-α)*ewma.  With α=1: ewma = 0 immediately.
	// But cooldown is 300s, so decrease should be suppressed.
	s.Advance(ts)
	st = s.ScoreState()
	lastEvent := st.Events[len(st.Events)-1]
	if lastEvent.ToLevel != observer.SeverityHigh {
		t.Errorf("expected cooldown to suppress decrease, last event: %v", lastEvent)
	}
	ts++

	// Jump 300 seconds past the transition to clear cooldown.
	highEntryTs := int64(1001)
	ts = highEntryTs + cfg.CooldownSecs + 1
	// Drive EWMA below high-margin (no anomalies → ewma decays to 0) → High→Medium.
	s.Advance(ts)
	st = s.ScoreState()
	lastEvent = st.Events[len(st.Events)-1]
	if lastEvent.ToLevel != observer.SeverityMedium {
		t.Errorf("expected High→Medium after cooldown, got ToLevel=%d; events: %v", lastEvent.ToLevel, st.Events)
	}

	// Verify High→Low is never allowed in one step.
	for _, ev := range st.Events {
		if ev.FromLevel == observer.SeverityHigh && ev.ToLevel == observer.SeverityLow {
			t.Errorf("illegal High→Low transition at ts=%d", ev.Timestamp)
		}
	}
}

// TestReset verifies that Reset clears all accumulated state.
func TestReset(t *testing.T) {
	s := NewScorer(DefaultScorerConfig())
	s.ProcessAnomaly(makeAnomaly("bocpd", 1000, nil))
	s.Advance(1000)
	s.Reset()
	st := s.ScoreState()
	if len(st.Buckets) != 0 || len(st.Events) != 0 {
		t.Errorf("Reset: expected empty state, got %d buckets %d events", len(st.Buckets), len(st.Events))
	}
}

// TestEmptySeconds verifies that Advance over a gap generates empty buckets.
func TestEmptySeconds(t *testing.T) {
	cfg := DefaultScorerConfig()
	cfg.Alpha = 0.5
	s := NewScorer(cfg)

	f := scorePtr(25.0) // level 3
	s.ProcessAnomaly(makeAnomaly("holt_residual", 1000, f))
	s.Advance(1002) // advance covers seconds 1000, 1001, 1002

	st := s.ScoreState()
	if len(st.Buckets) != 3 {
		t.Fatalf("expected 3 buckets, got %d", len(st.Buckets))
	}
	if st.Buckets[0].Second != 1000 || st.Buckets[1].Second != 1001 || st.Buckets[2].Second != 1002 {
		t.Errorf("unexpected seconds: %v %v %v", st.Buckets[0].Second, st.Buckets[1].Second, st.Buckets[2].Second)
	}
	if st.Buckets[1].Count != 0 || st.Buckets[2].Count != 0 {
		t.Errorf("expected empty gap buckets, got %v %v", st.Buckets[1], st.Buckets[2])
	}
}
