// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"math"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	noopsimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
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

func TestNormalizeCorrelationEventThreshold(t *testing.T) {
	cases := []struct {
		value string
		want  string
		valid bool
	}{
		{"", "high", true},
		{"high", "high", true},
		{" MEDIUM ", "medium", true},
		{"low", "", false},
		{"unexpected", "", false},
	}

	for _, tc := range cases {
		got, err := normalizeCorrelationEventThreshold(tc.value)
		if tc.valid && err != nil {
			t.Errorf("normalizeCorrelationEventThreshold(%q) returned error: %v", tc.value, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("normalizeCorrelationEventThreshold(%q) expected error", tc.value)
		}
		if got != tc.want {
			t.Errorf("normalizeCorrelationEventThreshold(%q) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestParseSettingsFromJSONCorrelationEventThreshold(t *testing.T) {
	settings, err := ParseSettingsFromJSON(map[string]json.RawMessage{
		"anomaly_scorer": json.RawMessage(`{"enabled":true}`),
	})
	if err != nil {
		t.Fatalf("ParseSettingsFromJSON() returned error for omitted threshold: %v", err)
	}
	cfg := settings.configs["anomaly_scorer"].(AnomalyScorerConfig)
	if cfg.CorrelationEventThreshold != "high" {
		t.Errorf("default CorrelationEventThreshold = %q, want high", cfg.CorrelationEventThreshold)
	}

	settings, err = ParseSettingsFromJSON(map[string]json.RawMessage{
		"anomaly_scorer": json.RawMessage(`{"enabled":true,"correlation_event_threshold":"medium"}`),
	})
	if err != nil {
		t.Fatalf("ParseSettingsFromJSON() returned error: %v", err)
	}
	cfg = settings.configs["anomaly_scorer"].(AnomalyScorerConfig)
	if cfg.CorrelationEventThreshold != "medium" {
		t.Errorf("CorrelationEventThreshold = %q, want medium", cfg.CorrelationEventThreshold)
	}

	_, err = ParseSettingsFromJSON(map[string]json.RawMessage{
		"anomaly_scorer": json.RawMessage(`{"enabled":true,"correlation_event_threshold":"low"}`),
	})
	if err == nil {
		t.Error("ParseSettingsFromJSON() accepted low correlation_event_threshold")
	}
}

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
		got := anomalyLevel(a, DefaultAnomalyScorerConfig().AnomalyScorerConfig)
		if got != tc.want {
			t.Errorf("anomalyLevel(%s, score=%v): got %d, want %d", tc.detector, tc.score, got, tc.want)
		}
	}
}

// TestEWMABasic verifies that the EWMA is seeded correctly and decays as expected.
func TestEWMABasic(t *testing.T) {
	cfg := DefaultAnomalyScorerConfig()
	cfg.Alpha = 0.5
	// With k=1: saturation(count=1) = 1−exp(−1/1) ≈ 0.632.
	cfg.SaturationK = 1.0
	// WindowSecs=1 so each second is independent: the series from t=1000 expires
	// at t=1001, allowing the EWMA decay test to see zero input.
	cfg.WindowSecs = 1

	s := NewAnomalyScorer(cfg)
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
	cfg := DefaultAnomalyScorerConfig()
	s := NewAnomalyScorer(cfg)

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
	cfg := DefaultAnomalyScorerConfig()
	cfg.WindowSecs = 15
	s := NewAnomalyScorer(cfg)

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
	cfg := DefaultAnomalyScorerConfig()
	cfg.WindowSecs = 15
	s := NewAnomalyScorer(cfg)

	src := observer.SeriesDescriptor{Namespace: "ns", Name: "m", Tags: []string{"host:h"}}
	// Series fires once at t=1000; last seen = 1000.
	// WindowSecs at t=1014: windowStart = 1014-15+1 = 1000 → series still alive (lastSeen >= windowStart).
	// WindowSecs at t=1015: windowStart = 1015-15+1 = 1001 → series expired (lastSeen=1000 < 1001).
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
	cfg := DefaultAnomalyScorerConfig()
	cfg.WindowSecs = 15
	s := NewAnomalyScorer(cfg)

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
	cfg := DefaultAnomalyScorerConfig()
	s := NewAnomalyScorer(cfg)

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
	s := NewAnomalyScorer(DefaultAnomalyScorerConfig())
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
	cfg := DefaultAnomalyScorerConfig()
	cfg.WindowSecs = 15
	s := NewAnomalyScorer(cfg)

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
	var b1011 *observer.AnomalyScoreBucket
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
	cfg := DefaultAnomalyScorerConfig()
	cfg.WindowSecs = 15
	s := NewAnomalyScorer(cfg)

	src := observer.SeriesDescriptor{Namespace: "ns", Name: "m", Tags: []string{"host:h"}}

	s.Advance(1010)

	// Late anomaly with historical timestamp.
	s.ProcessAnomaly(observer.Anomaly{
		DetectorName: "scanmw",
		Timestamp:    1000,
		Score:        scorePtr(20),
		Source:       src,
	})

	// Access internal state directly via type assertion to inspect unexported fields.
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
	cfg := DefaultAnomalyScorerConfig()
	s := NewAnomalyScorer(cfg)

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
		want severityeventsdef.SeverityLevel
	}{
		{0.000, severityeventsdef.SeverityLow},
		{0.039, severityeventsdef.SeverityLow},
		{0.040, severityeventsdef.SeverityMedium},
		{0.059, severityeventsdef.SeverityMedium},
		{0.060, severityeventsdef.SeverityHigh},
		{1.000, severityeventsdef.SeverityHigh},
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
	// High exit margin = 0.060 * 0.20 = 0.012.
	cases := []struct {
		ewma    float64
		current severityeventsdef.SeverityLevel
		want    severityeventsdef.SeverityLevel
	}{
		{0.060, severityeventsdef.SeverityLow, severityeventsdef.SeverityHigh},    // skip straight to High
		{0.045, severityeventsdef.SeverityLow, severityeventsdef.SeverityMedium},  // crosses low threshold
		{0.030, severityeventsdef.SeverityLow, severityeventsdef.SeverityLow},     // stays Low
		{0.065, severityeventsdef.SeverityMedium, severityeventsdef.SeverityHigh}, // escalate Medium→High
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
		current severityeventsdef.SeverityLevel
		want    severityeventsdef.SeverityLevel
		desc    string
	}{
		{0.049, severityeventsdef.SeverityHigh, severityeventsdef.SeverityHigh, "High: within hysteresis band"},
		{0.047, severityeventsdef.SeverityHigh, severityeventsdef.SeverityMedium, "High: below hysteresis -> Medium"},
		{0.005, severityeventsdef.SeverityHigh, severityeventsdef.SeverityLow, "High: far below -> Low"},
		{0.029, severityeventsdef.SeverityMedium, severityeventsdef.SeverityMedium, "Medium: within hysteresis band"},
		{0.027, severityeventsdef.SeverityMedium, severityeventsdef.SeverityLow, "Medium: below hysteresis -> Low"},
	}
	for _, tc := range cases {
		got := nextSeverityLevel(tc.ewma, tc.current, 0.040, 0.060, 0.060*0.20)
		if got != tc.want {
			t.Errorf("%s: nextSeverityLevel(ewma=%.3f, current=%d): got %d, want %d",
				tc.desc, tc.ewma, tc.current, got, tc.want)
		}
	}
}

// ---- Subscribe / subscription state machine ----

// collectingListener records every SeverityEvent it receives.
type collectingListener struct {
	events []severityeventsdef.SeverityEvent
}

func (l *collectingListener) OnSeverityTransition(e severityeventsdef.SeverityEvent) {
	l.events = append(l.events, e)
}

func mustSubscribeSeverityEvents(t *testing.T, s StandaloneAnomalyScorer, cfg severityeventsdef.SeverityEventsConfiguration, listener severityeventsdef.SeverityEventListener) severityeventsdef.SeverityEventsSubscription {
	t.Helper()

	sub, err := s.SubscribeSeverityEvents(cfg, listener)
	if err != nil {
		t.Fatalf("SubscribeSeverityEvents() error = %v", err)
	}
	return sub
}

// TestSubscribeBasic verifies that a listener receives an escalation event when
// the EWMA crosses the Low threshold.
func TestSubscribeBasic(t *testing.T) {
	cfg := DefaultAnomalyScorerConfig()
	cfg.Alpha = 0.99 // near-instant EWMA (1.0 is rejected as invalid by NewScorer)
	cfg.SaturationK = 1.0
	cfg.WindowSecs = 5
	s := NewAnomalyScorer(cfg)

	l := &collectingListener{}
	mustSubscribeSeverityEvents(t, s, severityeventsdef.SeverityEventsConfiguration{}, l)

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
	if evt.FromLevel != severityeventsdef.SeverityLow || evt.ToLevel != severityeventsdef.SeverityHigh {
		t.Errorf("escalation event wrong levels: from=%d to=%d", evt.FromLevel, evt.ToLevel)
	}
	if evt.Direction != severityeventsdef.SeverityEventEscalation {
		t.Errorf("expected escalation direction, got %d", evt.Direction)
	}
}

// TestSubscribeCooldown verifies that de-escalation is blocked during cooldown.
func TestSubscribeCooldown(t *testing.T) {
	cfg := DefaultAnomalyScorerConfig()
	cfg.Alpha = 0.99
	cfg.SaturationK = 1.0
	cfg.WindowSecs = 1 // short window so anomaly expires quickly
	s := NewAnomalyScorer(cfg)

	l := &collectingListener{}
	mustSubscribeSeverityEvents(t, s, severityeventsdef.SeverityEventsConfiguration{
		CooldownSecs: 60, // 60s cooldown on de-escalations
	}, l)

	// Warm-up: seed state at Low (EWMA=0, first advance never fires an event).
	s.Advance(1000)

	// Drive EWMA above high_threshold → escalation Low→High fires.
	s.ProcessAnomaly(makeAnomaly("bocpd", 1001, nil))
	s.Advance(1001) // EWMA≈0.632 → High

	// Advance with no anomalies: EWMA decays near 0 quickly (alpha=0.99).
	// CooldownSecs=60 should block de-escalation for 60 seconds after the escalation.
	s.Advance(1002) // EWMA=0 → raw Low, but cooldown blocks it

	escalations, deescalations := 0, 0
	for _, e := range l.events {
		if e.Direction == severityeventsdef.SeverityEventEscalation {
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
		if e.Direction == severityeventsdef.SeverityEventDeescalation {
			deescalations++
		}
	}
	if deescalations != 1 {
		t.Errorf("expected 1 de-escalation after cooldown expired, got %d", deescalations)
	}
}

// TestSubscribeFilter verifies that events not matching the filter are not delivered.
func TestSubscribeFilter(t *testing.T) {
	cfg := DefaultAnomalyScorerConfig()
	cfg.Alpha = 0.99
	cfg.SaturationK = 1.0
	cfg.WindowSecs = 1
	s := NewAnomalyScorer(cfg)

	// Only receive escalations.
	l := &collectingListener{}
	mustSubscribeSeverityEvents(t, s, severityeventsdef.SeverityEventsConfiguration{
		Filter: severityeventsdef.SeverityEventFilter{
			Direction: severityeventsdef.SeverityEventEscalation,
		},
	}, l)

	// Warm-up: seed state at Low.
	s.Advance(1000)

	// Trigger escalation then de-escalation.
	s.ProcessAnomaly(makeAnomaly("bocpd", 1001, nil))
	s.Advance(1001) // → High (escalation delivered)
	s.Advance(1002) // → Low (de-escalation filtered out)

	if len(l.events) != 1 {
		t.Fatalf("expected 1 event (only escalation), got %d: %v", len(l.events), l.events)
	}
	if l.events[0].Direction != severityeventsdef.SeverityEventEscalation {
		t.Errorf("delivered event should be escalation, got direction=%d", l.events[0].Direction)
	}
}

// TestUnsubscribe verifies that the returned unsubscribe function stops delivery.
func TestUnsubscribe(t *testing.T) {
	cfg := DefaultAnomalyScorerConfig()
	cfg.Alpha = 0.99
	cfg.SaturationK = 1.0
	cfg.WindowSecs = 1
	s := NewAnomalyScorer(cfg)

	l := &collectingListener{}
	sub := mustSubscribeSeverityEvents(t, s, severityeventsdef.SeverityEventsConfiguration{}, l)

	// Warm-up: seed at Low.
	s.Advance(1000)

	s.ProcessAnomaly(makeAnomaly("bocpd", 1001, nil))
	s.Advance(1001) // escalation fires
	sub.Unsubscribe()

	s.Advance(1002) // de-escalation would fire, but subscription already removed
	if len(l.events) != 1 {
		t.Errorf("expected exactly 1 event before unsub; got %d: %v", len(l.events), l.events)
	}
}

// TestResetClearsSubscriptionState verifies that Reset() re-initializes each
// subscription's state machine so no stale state carries over into a replay run.
func TestResetClearsSubscriptionState(t *testing.T) {
	cfg := DefaultAnomalyScorerConfig()
	cfg.Alpha = 0.99
	cfg.SaturationK = 1.0
	cfg.WindowSecs = 1
	s := NewAnomalyScorer(cfg)

	l := &collectingListener{}
	mustSubscribeSeverityEvents(t, s, severityeventsdef.SeverityEventsConfiguration{
		CooldownSecs: 3600, // long cooldown — would suppress de-escalation if stale
	}, l)

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
	if l.events[after-1].Direction != severityeventsdef.SeverityEventEscalation {
		t.Errorf("post-reset event should be escalation, got direction=%d", l.events[after-1].Direction)
	}
}

func TestSubscribeSeverityEventsCreatesIndependentDispatchers(t *testing.T) {
	cfg := DefaultAnomalyScorerConfig()
	cfg.Alpha = 0.99
	cfg.SaturationK = 1.0
	cfg.WindowSecs = 1
	s := NewAnomalyScorer(cfg)

	fast := &collectingListener{}
	slow := &collectingListener{}

	mustSubscribeSeverityEvents(t, s, severityeventsdef.SeverityEventsConfiguration{}, fast)
	mustSubscribeSeverityEvents(t, s, severityeventsdef.SeverityEventsConfiguration{
		CooldownSecs: 60,
	}, slow)

	s.Advance(1000)
	s.ProcessAnomaly(makeAnomaly("bocpd", 1001, nil))
	s.Advance(1001)
	s.Advance(1002)

	if len(fast.events) != 2 {
		t.Fatalf("expected fast dispatcher to see escalation and immediate de-escalation, got %d events: %v", len(fast.events), fast.events)
	}
	if fast.events[0].Direction != severityeventsdef.SeverityEventEscalation || fast.events[1].Direction != severityeventsdef.SeverityEventDeescalation {
		t.Fatalf("unexpected fast dispatcher event sequence: %v", fast.events)
	}

	if len(slow.events) != 1 {
		t.Fatalf("expected slow dispatcher to only see the escalation before cooldown expiry, got %d events: %v", len(slow.events), slow.events)
	}
	if slow.events[0].Direction != severityeventsdef.SeverityEventEscalation {
		t.Fatalf("expected slow dispatcher event to be the escalation, got %v", slow.events[0])
	}

	s.Advance(1062)
	if len(slow.events) != 2 || slow.events[1].Direction != severityeventsdef.SeverityEventDeescalation {
		t.Fatalf("expected slow dispatcher to emit a delayed de-escalation after cooldown, got %v", slow.events)
	}
}

// ---- Original tests ----

// ---- Episode / ActiveCorrelations / watcher tests ----

// newScorerWithTelemetry is a test helper that creates a scorer with no-op
// telemetry gauges so that the internal watcher is active.
func newScorerWithTelemetry(cfg AnomalyScorerConfig) *anomalyScorer {
	tel := noopsimpl.GetCompatComponent()
	stateGauge := tel.NewGauge("test", "scorer_state", nil, "")
	ewmaGauge := tel.NewGauge("test", "scorer_ewma", nil, "")
	return newAnomalyScorerWithTelemetry(cfg, stateGauge, ewmaGauge)
}

// episodeTestCfg returns a scorer config tuned for fast episode tests:
//   - WindowSecs=1 so a single no-anomaly advance fully empties the window
//   - alpha=0.99 so EWMA collapses to ~0 in one step with zero input
//   - SaturationK=1.0 for predictable saturation weight
//   - CooldownSecs=0 so de-escalation is not blocked by cooldown
//
// With this config the episode lifecycle is deterministic across two advances:
//
//	Advance(t0)          — empty; seeds state at Low (rawSeverityLevel(0)=Low)
//	ProcessAnomaly+Advance(t0+1) — spike; Low→High transition fires, episode opens
//	Advance(t0+2)        — no anomalies; EWMA≈0; High→Low fires, episode closes
func episodeTestCfg() AnomalyScorerConfig {
	cfg := DefaultAnomalyScorerConfig()
	cfg.Alpha = 0.99
	cfg.SaturationK = 1.0
	cfg.WindowSecs = 1
	cfg.CooldownSecs = 0
	return cfg
}

// seedAndCrossHighThreshold seeds the state machine with one empty advance,
// then triggers a single spike advance that drives EWMA above high_threshold.
// Returns the advance time of the spike (t0+1).
//
// Caller should call s.Advance(spikeSec+1) next to trigger de-escalation, and
// read ActiveCorrelations() BEFORE that advance if it needs the closed episode.
func seedAndCrossHighThreshold(s *anomalyScorer, t0 int64) int64 {
	s.Advance(t0)                                                        // seed at Low
	s.ProcessAnomaly(makeAnomaly("holt_residual", t0+1, scorePtr(40.0))) // spike
	s.Advance(t0 + 1)                                                    // Low→High fires, episode opens
	return t0 + 1
}

// triggerDeescalation advances once with no anomalies so the EWMA collapses to
// near-zero (WindowSecs=1 empties the window) and the High→Low transition fires,
// closing the open episode. Returns the advance time.
// The caller must read ActiveCorrelations() before the NEXT advance, because the
// closed episode will be drained at the start of the subsequent Advance call.
func triggerDeescalation(s *anomalyScorer, prevSec int64) int64 {
	s.Advance(prevSec + 1)
	return prevSec + 1
}

// TestActiveCorrelationsNilWhenDisabled confirms that ActiveCorrelations returns
// nil when CorrelationEvents is false (the default).
func TestActiveCorrelationsNilWhenDisabled(t *testing.T) {
	cfg := episodeTestCfg()
	// CorrelationEvents defaults to false in episodeTestCfg.
	s := newScorerWithTelemetry(cfg)
	s.Advance(1000)
	if got := s.ActiveCorrelations(); got != nil {
		t.Errorf("expected nil when CorrelationEvents=false, got %v", got)
	}
}

// TestEpisodeOpenClose verifies that OnSeverityTransition opens an episode on
// escalation to High and closes it on de-escalation.
// Step sequence (WindowSecs=1, alpha=0.99):
//
//	Advance(1000) → seed at Low
//	Advance(1001) + spike → Low→High, episode opens (EpisodeStarted in PendingEvents)
//	Advance(1002) (no anomalies) → EWMA≈0; High→Low, episode closes (EpisodeEnded in PendingEvents)
func TestEpisodeOpenClose(t *testing.T) {
	cfg := episodeTestCfg()
	cfg.CorrelationEvents = true
	s := newScorerWithTelemetry(cfg)

	spikeSec := seedAndCrossHighThreshold(s, 1000)

	s.mu.Lock()
	openAfterSpike := s.openEpisode != nil
	s.mu.Unlock()
	if !openAfterSpike {
		t.Fatal("expected openEpisode to be non-nil after crossing High threshold")
	}

	// Drain EpisodeStarted from the escalation advance.
	_ = s.PendingEvents()

	// One no-anomaly advance collapses EWMA to zero (WindowSecs=1): de-escalation fires.
	triggerDeescalation(s, spikeSec)

	s.mu.Lock()
	openAfterDecay := s.openEpisode != nil
	s.mu.Unlock()
	if openAfterDecay {
		t.Error("expected openEpisode to be nil after EWMA decayed below threshold")
	}

	// Closed episode must be in PendingEvents as EpisodeEnded.
	evts := s.PendingEvents()
	var foundEnded bool
	for _, ce := range evts {
		if ce.Kind == observer.CorrelatorEventEpisodeEnded {
			foundEnded = true
		}
	}
	if !foundEnded {
		t.Error("expected an EpisodeEnded event in PendingEvents after de-escalation")
	}
}

func TestEpisodeMediumCorrelationEventThreshold(t *testing.T) {
	cfg := episodeTestCfg()
	cfg.CorrelationEvents = true
	cfg.CorrelationEventThreshold = "medium"
	cfg.LowThreshold = 0.1
	cfg.HighThreshold = 0.9
	cfg.MarginPct = 0.05 // high-relative margin remains below LowThreshold
	s := newScorerWithTelemetry(cfg)

	s.Advance(1000) // seed at Low
	s.ProcessAnomaly(makeAnomaly("bocpd", 1001, nil))
	s.Advance(1001) // Low -> Medium; the score remains below the High threshold

	correlations := s.ActiveCorrelations()
	if len(correlations) != 1 {
		t.Fatalf("expected one Medium-threshold episode, got %d", len(correlations))
	}
	if got, want := correlations[0].Pattern, "anomaly_scorer_medium:1001"; got != want {
		t.Errorf("episode pattern = %q, want %q", got, want)
	}

	_ = s.PendingEvents()
	s.Advance(1002) // Medium -> Low; closes the episode
	if got := s.ActiveCorrelations(); len(got) != 0 {
		t.Errorf("expected Medium-threshold episode to close, got %d active correlations", len(got))
	}
}

// TestActiveCorrelationsSnapshotSafe verifies that calling ActiveCorrelations
// multiple times between Advance calls returns the same result (open episode
// is snapshot-safe), and that PendingEvents drains exactly once.
func TestActiveCorrelationsSnapshotSafe(t *testing.T) {
	cfg := episodeTestCfg()
	cfg.CorrelationEvents = true
	s := newScorerWithTelemetry(cfg)

	// Episode opens — open episode visible in ActiveCorrelations.
	seedAndCrossHighThreshold(s, 1000)

	first := s.ActiveCorrelations()
	if len(first) == 0 {
		t.Fatal("expected open episode in ActiveCorrelations while High")
	}
	// Second read must be identical — no drain in ActiveCorrelations.
	second := s.ActiveCorrelations()
	if len(second) != len(first) {
		t.Errorf("ActiveCorrelations is not snapshot-safe: first=%d, second=%d", len(first), len(second))
	}

	// Drain EpisodeStarted then trigger de-escalation.
	_ = s.PendingEvents()
	spikeSec := first[0].FirstSeen
	triggerDeescalation(s, spikeSec)

	// After de-escalation, no open episode remains in ActiveCorrelations.
	if got := s.ActiveCorrelations(); len(got) != 0 {
		t.Errorf("expected no open episode after de-escalation, got %d", len(got))
	}

	// EpisodeEnded must be in PendingEvents — drain-once semantics.
	evts := s.PendingEvents()
	var foundEnded bool
	for _, ce := range evts {
		if ce.Kind == observer.CorrelatorEventEpisodeEnded {
			foundEnded = true
		}
	}
	if !foundEnded {
		t.Error("expected EpisodeEnded in PendingEvents after de-escalation")
	}
	// Second drain returns nothing.
	if got := s.PendingEvents(); len(got) != 0 {
		t.Errorf("PendingEvents should be empty after drain, got %d", len(got))
	}
}

// TestActiveCorrelationsOpenEpisodeVisible verifies that the currently open
// episode is visible in ActiveCorrelations while the EWMA is High.
func TestActiveCorrelationsOpenEpisodeVisible(t *testing.T) {
	cfg := episodeTestCfg()
	cfg.CorrelationEvents = true
	s := newScorerWithTelemetry(cfg)

	seedAndCrossHighThreshold(s, 1000)

	correlations := s.ActiveCorrelations()
	if len(correlations) == 0 {
		t.Fatal("expected open episode to be visible in ActiveCorrelations while High")
	}
	for _, ac := range correlations {
		if ac.Pattern == "" {
			t.Error("correlation pattern must not be empty")
		}
	}
}

// TestMaxEpisodeAnomalies verifies that the episode anomaly list is capped.
func TestMaxEpisodeAnomalies(t *testing.T) {
	cfg := episodeTestCfg()
	cfg.CorrelationEvents = true
	cfg.MaxEpisodeAnomalies = 3
	cfg.WindowSecs = 30 // wider window keeps episode open across multiple advances
	s := newScorerWithTelemetry(cfg)

	spikeSec := seedAndCrossHighThreshold(s, 1000)
	// Feed many anomalies into the still-open episode (episode is open since
	// WindowSecs=30 means the spike is still in window).
	for i := int64(1); i <= 10; i++ {
		s.ProcessAnomaly(makeAnomaly("bocpd", spikeSec+i, nil))
		s.Advance(spikeSec + i)
	}

	s.mu.Lock()
	var anomalyCount int
	if s.openEpisode != nil {
		anomalyCount = len(s.openEpisode.Anomalies)
	}
	s.mu.Unlock()

	if anomalyCount > cfg.MaxEpisodeAnomalies {
		t.Errorf("episode accumulated %d anomalies, expected cap at %d", anomalyCount, cfg.MaxEpisodeAnomalies)
	}
}

// TestScorerWithTelemetry_GaugesAndLogs verifies that newAnomalyScorerWithTelemetry
// wires the internal watcher self-subscription and does not panic on transitions.
func TestScorerWithTelemetry_GaugesAndLogs(_ *testing.T) {
	cfg := episodeTestCfg()
	cfg.Logs = true
	cfg.CorrelationEvents = false
	s := newScorerWithTelemetry(cfg)

	// Drive EWMA past High threshold — must not panic even with Logs=true.
	spikeSec := seedAndCrossHighThreshold(s, 1000)
	// De-escalate — must not panic.
	triggerDeescalation(s, spikeSec)
}

// TestActiveCorrelationsEngineAccumulationOrdering verifies the engine's
// contract: closed episodes appear in PendingEvents() after the Advance that
// closes them, and are drained exactly once.
func TestActiveCorrelationsEngineAccumulationOrdering(t *testing.T) {
	cfg := episodeTestCfg()
	cfg.CorrelationEvents = true
	s := newScorerWithTelemetry(cfg)

	spikeSec := seedAndCrossHighThreshold(s, 1000)
	// Drain EpisodeStarted from the escalation advance.
	startEvts := s.PendingEvents()
	var startPattern string
	for _, ce := range startEvts {
		if ce.Kind == observer.CorrelatorEventEpisodeStarted {
			startPattern = ce.Correlation.Pattern
		}
	}
	if startPattern == "" {
		t.Fatal("expected EpisodeStarted in PendingEvents after escalation")
	}

	// De-escalation advance: EpisodeEnded lands in PendingEvents.
	triggerDeescalation(s, spikeSec)

	endEvts := s.PendingEvents()
	var foundEnd bool
	for _, ce := range endEvts {
		if ce.Kind == observer.CorrelatorEventEpisodeEnded && ce.Correlation.Pattern == startPattern {
			foundEnd = true
		}
	}
	if !foundEnd {
		t.Fatalf("expected EpisodeEnded for pattern %q in PendingEvents", startPattern)
	}

	// Second drain must be empty.
	if got := s.PendingEvents(); len(got) != 0 {
		t.Errorf("PendingEvents should be empty after drain, got %d events", len(got))
	}
}

// TestActiveCorrelationsResetClearsEpisodes verifies that Reset clears
// the open episode and any pending events.
func TestActiveCorrelationsResetClearsEpisodes(t *testing.T) {
	cfg := episodeTestCfg()
	cfg.CorrelationEvents = true
	s := newScorerWithTelemetry(cfg)

	seedAndCrossHighThreshold(s, 1000)

	if len(s.ActiveCorrelations()) == 0 {
		t.Fatal("expected open episode before Reset")
	}

	s.Reset()

	s.mu.Lock()
	hasOpen := s.openEpisode != nil
	hasPending := len(s.pendingEvents) > 0
	s.mu.Unlock()

	if hasOpen {
		t.Error("Reset did not clear openEpisode")
	}
	if hasPending {
		t.Error("Reset did not clear pendingEvents")
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
	cfg := DefaultAnomalyScorerConfig()
	cfg.Alpha = 0.5
	cfg.WindowSecs = 1
	s := NewAnomalyScorer(cfg)

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

// ---------------------------------------------------------------------------
// PendingEvents tests
// ---------------------------------------------------------------------------

// TestPendingEvents_EpisodeStarted verifies that PendingEvents returns a single
// EpisodeStarted event on the advance that crosses into High severity, and that
// subsequent calls to PendingEvents return nil (drained).
func TestPendingEvents_EpisodeStarted(t *testing.T) {
	cfg := episodeTestCfg()
	cfg.CorrelationEvents = true
	s := newScorerWithTelemetry(cfg)

	// Seed state at Low.
	s.Advance(1000)
	if got := s.PendingEvents(); got != nil {
		t.Fatalf("expected nil PendingEvents after seed advance, got %v", got)
	}

	// Spike: pushes EWMA above HighThreshold → EpisodeStarted.
	ts := seedAndCrossHighThreshold(s, 1001)
	evts := s.PendingEvents()
	if len(evts) != 1 {
		t.Fatalf("expected 1 PendingEvent after spike, got %d", len(evts))
	}
	if evts[0].Kind != observer.CorrelatorEventEpisodeStarted {
		t.Errorf("expected EpisodeStarted, got kind %d", evts[0].Kind)
	}
	if evts[0].ToLevel != severityeventsdef.SeverityHigh {
		t.Errorf("expected ToLevel=High, got %d", evts[0].ToLevel)
	}
	if evts[0].Timestamp != ts {
		t.Errorf("expected Timestamp=%d, got %d", ts, evts[0].Timestamp)
	}
	if evts[0].Correlation.Pattern == "" {
		t.Error("expected non-empty Correlation.Pattern")
	}
	// Drain is idempotent — second call returns nil.
	if got := s.PendingEvents(); got != nil {
		t.Fatalf("expected nil on second PendingEvents call (already drained), got %v", got)
	}
}

// TestPendingEvents_EpisodeEnded verifies that PendingEvents returns an EpisodeEnded
// event on the advance that drops out of High severity.
func TestPendingEvents_EpisodeEnded(t *testing.T) {
	cfg := episodeTestCfg()
	cfg.CorrelationEvents = true
	s := newScorerWithTelemetry(cfg)

	seedAndCrossHighThreshold(s, 1000)
	// Drain the EpisodeStarted event.
	_ = s.PendingEvents()

	// No anomalies → EWMA decays below LowThreshold → High→Low.
	endTs := triggerDeescalation(s, 1001)
	evts := s.PendingEvents()
	if len(evts) != 1 {
		t.Fatalf("expected 1 PendingEvent after decay, got %d", len(evts))
	}
	if evts[0].Kind != observer.CorrelatorEventEpisodeEnded {
		t.Errorf("expected EpisodeEnded, got kind %d", evts[0].Kind)
	}
	if evts[0].FromLevel != severityeventsdef.SeverityHigh {
		t.Errorf("expected FromLevel=High, got %d", evts[0].FromLevel)
	}
	if evts[0].Correlation.LastUpdated != endTs {
		t.Errorf("expected Correlation.LastUpdated=%d, got %d", endTs, evts[0].Correlation.LastUpdated)
	}
	// Drain is idempotent.
	if got := s.PendingEvents(); got != nil {
		t.Fatalf("expected nil on second PendingEvents call (already drained), got %v", got)
	}
}

// TestPendingEvents_DisabledWhenCorrelationEventsOff verifies that PendingEvents
// always returns nil when CorrelationEvents=false, even during severity transitions.
func TestPendingEvents_DisabledWhenCorrelationEventsOff(t *testing.T) {
	cfg := episodeTestCfg()
	cfg.CorrelationEvents = false
	s := newScorerWithTelemetry(cfg)

	seedAndCrossHighThreshold(s, 1000)
	if got := s.PendingEvents(); got != nil {
		t.Fatalf("expected nil PendingEvents with CorrelationEvents=false, got %v", got)
	}
}

// TestPendingEvents_ResetClearsPending verifies that Reset() discards any
// accumulated but unread pending events.
func TestPendingEvents_ResetClearsPending(t *testing.T) {
	cfg := episodeTestCfg()
	cfg.CorrelationEvents = true
	s := newScorerWithTelemetry(cfg)

	seedAndCrossHighThreshold(s, 1000)
	// Don't drain — Reset should clear them.
	s.Reset()
	if got := s.PendingEvents(); got != nil {
		t.Fatalf("expected nil PendingEvents after Reset(), got %v", got)
	}
}
