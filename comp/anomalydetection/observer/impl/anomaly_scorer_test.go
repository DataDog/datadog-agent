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

	b := s.ScoreState().Buckets[1] // bucket for t=1005
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

	b14 := s.ScoreState().Buckets[len(s.ScoreState().Buckets)-1]
	if b14.Count != 1 {
		t.Errorf("window expiry: expected count=1 at t=1014, got %d", b14.Count)
	}

	s.Advance(1015)

	b15 := s.ScoreState().Buckets[len(s.ScoreState().Buckets)-1]
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

// TestEmptySeconds verifies that Advance over a gap generates empty buckets.
// WindowSecs=1 so the anomaly at t=1000 expires before t=1001, giving zero
// window count for the gap seconds and allowing pure EWMA decay to be tested.
func TestEmptySeconds(t *testing.T) {
	cfg := DefaultScorerConfig()
	cfg.Alpha = 0.5
	cfg.WindowSecs = 1
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
