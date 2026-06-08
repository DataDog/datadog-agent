// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"sync"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// levelWeights maps anomaly level (0–4) to its EWMA weight.
// Level 0=VeryLow, 1=Low, 2=Medium, 3=High, 4=XHigh.
var levelWeights = [5]float64{0.2, 0.5, 1.0, 2.0, 3.0}

// scoreThresholds are the boundaries that map a raw detector score to a level.
// score < 6 → 0, 6 ≤ score < 12 → 1, 12 ≤ score < 20 → 2,
// 20 ≤ score < 35 → 3, score ≥ 35 → 4.
var scoreThresholds = [4]float64{6, 12, 20, 35}

// detectorFixedLevel maps detectors that emit no score to a fixed level.
var detectorFixedLevel = map[string]int{
	"bocpd": 2, // Medium — reliable changepoint signal, no score
}

// scoredDetectors are the detectors that emit a numeric score to threshold against.
var scoredDetectors = map[string]bool{
	"holt_residual":  true,
	"tukey_biweight": true,
}

// DefaultScorerConfig returns calibrated defaults (see ANOMALY_SCORING.md §2.8).
// Per-detector thresholds are set based on empirical score distributions across
// kafka-partition-saturation, postmark, and dns-upstream-outage scenarios.
func DefaultScorerConfig() observer.ScorerConfig {
	return observer.ScorerConfig{
		Alpha:         0.014,
		SaturationK:   5.0,
		LowThreshold:  0.040,
		HighThreshold: 0.060,
		MarginPct:     0.20,
		CooldownSecs:  300,
		DetectorThresholds: map[string][4]float64{
			// tukey_biweight scores cap hard at ~50 across all scenarios (natural
			// range is roughly half of holt_residual). Shift thresholds down so
			// the full [0,50] scale is used rather than compressing everything
			// into the bottom half of a [0,35] default range.
			// Calibrated: p25≈6, p50≈9, p75≈15, p90≈27, p99≈45 (3-scenario avg).
			"tukey_biweight": {5, 8, 15, 30},
			// holt_residual can reach 400+ (dns outliers) but 99% stay below ~75.
			// p25≈8, p50≈12, p75≈16, p90≈26, p95≈37 — current defaults [6,12,20,35]
			// already land well; keep them explicit for documentation clarity.
			"holt_residual": {6, 12, 20, 35},
		},
	}
}

// readScorerConfig reads scorer settings from the agent config.
func readScorerConfig(r ConfigReader, prefix string) any {
	cfg := DefaultScorerConfig()
	if key := prefix + "alpha"; r.IsConfigured(key) {
		cfg.Alpha = r.GetFloat64(key)
	}
	if key := prefix + "saturation_k"; r.IsConfigured(key) {
		cfg.SaturationK = r.GetFloat64(key)
	}
	if key := prefix + "low_threshold"; r.IsConfigured(key) {
		cfg.LowThreshold = r.GetFloat64(key)
	}
	if key := prefix + "high_threshold"; r.IsConfigured(key) {
		cfg.HighThreshold = r.GetFloat64(key)
	}
	if key := prefix + "margin_pct"; r.IsConfigured(key) {
		cfg.MarginPct = r.GetFloat64(key)
	}
	if key := prefix + "cooldown_secs"; r.IsConfigured(key) {
		cfg.CooldownSecs = int64(r.GetInt(key))
	}
	return cfg
}

// anomalyLevel assigns a 0–4 level to an anomaly.
// For scored detectors it applies per-detector thresholds from cfg when
// available, falling back to the global defaults scoreThresholds.
func anomalyLevel(a observer.Anomaly, cfg observer.ScorerConfig) int {
	if scoredDetectors[a.DetectorName] {
		if a.Score == nil {
			return 0 // treat nil score from a scored detector as VeryLow
		}
		s := *a.Score
		thresholds := scoreThresholds
		if dt, ok := cfg.DetectorThresholds[a.DetectorName]; ok {
			thresholds = dt
		}
		for i, t := range thresholds {
			if s < t {
				return i
			}
		}
		return 4
	}
	if l, ok := detectorFixedLevel[a.DetectorName]; ok {
		return l
	}
	return 2 // default: Medium
}

// seriesID returns a stable string key for deduplication.
// Returns "" when the anomaly has no identifiable series (never merged then).
func seriesID(a observer.Anomaly) string {
	if a.SourceRef != nil {
		return a.SourceRef.CompactID()
	}
	return a.Source.Key()
}

// dedupKey is the per-second deduplication key.
type dedupKey struct {
	second   int64
	seriesID string
}

// anomalyScorer is the streaming implementation of observer.Scorer.
//
// Lifecycle:
//
//	ProcessAnomaly → buffers raw anomaly keyed by its second.
//	Advance(t)     → finalises every second in [lastAdvancedSec+1, t],
//	                  computing EWMA and state-machine transitions.
//	ScoreState()   → returns accumulated telemetry snapshot.
//	Reset()        → clears all state.
type anomalyScorer struct {
	mu sync.Mutex

	config observer.ScorerConfig

	// pending holds anomalies received since the last Advance, grouped by second.
	pending map[int64][]observer.Anomaly

	// EWMA state
	ewma float64

	// Severity state machine
	state            observer.SeverityLevel
	lastStateEntryTs int64
	stateInitialized bool
	lastAdvancedSec  int64

	// Accumulated telemetry
	buckets []observer.ScoreBucket
	events  []observer.SeverityEvent
}

// NewScorer creates a new anomalyScorer with the given config.
func NewScorer(cfg observer.ScorerConfig) *anomalyScorer {
	return &anomalyScorer{
		config:  cfg,
		pending: make(map[int64][]observer.Anomaly),
		// lastStateEntryTs = 0: sec - 0 >> cooldownSecs for any real unix timestamp,
		// so the first downward transition is never suppressed.
		// (Using math.MinInt64 would overflow the int64 subtraction.)
		lastStateEntryTs: 0,
	}
}

func (s *anomalyScorer) Name() string { return "anomaly_scorer" }

// ProcessAnomaly buffers the anomaly into the pending map keyed by its second.
func (s *anomalyScorer) ProcessAnomaly(a observer.Anomaly) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec := a.Timestamp // already a unix second from the engine
	s.pending[sec] = append(s.pending[sec], a)
}

// Advance finalises all 1-second buckets from lastAdvancedSec+1 up to dataTime
// (inclusive), running dedup → bucketing → EWMA → state machine for each.
func (s *anomalyScorer) Advance(dataTime int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	start := s.lastAdvancedSec + 1
	if s.lastAdvancedSec == 0 {
		// First Advance: start from the earliest pending anomaly (or dataTime if
		// no anomalies), to avoid emitting empty buckets for all seconds since epoch.
		start = dataTime
		for sec := range s.pending {
			if sec < start {
				start = sec
			}
		}
	}

	for sec := start; sec <= dataTime; sec++ {
		s.advanceSecond(sec)
	}
	s.lastAdvancedSec = dataTime
}

// advanceSecond processes a single second, updating all state. Must be called with mu held.
func (s *anomalyScorer) advanceSecond(sec int64) {
	anomalies := s.pending[sec]
	delete(s.pending, sec)

	// Step 0: deduplication — per (second, seriesID) keep highest level.
	bestLevel := map[dedupKey]int{}
	var unkeyed []observer.Anomaly
	for _, a := range anomalies {
		sid := seriesID(a)
		if sid == "" {
			unkeyed = append(unkeyed, a)
			continue
		}
		k := dedupKey{second: sec, seriesID: sid}
		l := anomalyLevel(a, s.config)
		if existing, ok := bestLevel[k]; !ok || l > existing {
			bestLevel[k] = l
		}
	}

	// Step 2: bucketing.
	var bins [5]int
	var count int
	var weightSum float64

	for _, l := range bestLevel {
		bins[l]++
		count++
		weightSum += levelWeights[l]
	}
	for _, a := range unkeyed {
		l := anomalyLevel(a, s.config)
		bins[l]++
		count++
		weightSum += levelWeights[l]
	}

	// Steps 3 & 4: saturated input → EWMA.
	var input float64
	if count > 0 {
		meanWeight := weightSum / float64(count)
		input = meanWeight * (1 - math.Exp(-float64(count)/s.config.SaturationK))
	}

	// Always apply the EWMA formula starting from 0.
	// In the live scorer, empty scenario seconds precede any anomalies, so the
	// EWMA is effectively already near 0 when the first anomaly arrives.
	// In the replay, starting from 0 avoids seeding a spuriously high initial
	// state from a dense first anomaly second.
	s.ewma = s.config.Alpha*input + (1-s.config.Alpha)*s.ewma

	s.buckets = append(s.buckets, observer.ScoreBucket{
		Second:    sec,
		Bins:      bins,
		Count:     count,
		WeightSum: weightSum,
		Ewma:      s.ewma,
	})

	// Step 5: severity state machine.
	margin := s.config.HighThreshold * s.config.MarginPct
	v := s.ewma

	if !s.stateInitialized {
		// Seed the initial state from the raw EWMA value (no hysteresis).
		s.state = rawSeverityLevel(v, s.config.LowThreshold, s.config.HighThreshold)
		s.stateInitialized = true
		return
	}

	next := nextSeverityLevel(v, s.state, s.config.LowThreshold, s.config.HighThreshold, margin)
	if next == s.state {
		return
	}

	// Suppress decrease transitions during cooldown.
	if next < s.state && sec-s.lastStateEntryTs < s.config.CooldownSecs {
		return
	}

	s.events = append(s.events, observer.SeverityEvent{
		Timestamp: sec,
		FromLevel: s.state,
		ToLevel:   next,
	})
	s.state = next
	s.lastStateEntryTs = sec
}

// rawSeverityLevel returns the initial severity level using bare thresholds (no hysteresis).
func rawSeverityLevel(v, low, high float64) observer.SeverityLevel {
	if v >= high {
		return observer.SeverityHigh
	}
	if v >= low {
		return observer.SeverityMedium
	}
	return observer.SeverityLow
}

// nextSeverityLevel applies the full transition logic including hysteresis.
// One-level-down-only from High is enforced: High can only go to Medium, never directly to Low.
func nextSeverityLevel(v float64, cur observer.SeverityLevel, low, high, margin float64) observer.SeverityLevel {
	switch cur {
	case observer.SeverityLow:
		if v >= high+margin {
			return observer.SeverityHigh
		}
		if v >= low+margin {
			return observer.SeverityMedium
		}
		return observer.SeverityLow

	case observer.SeverityMedium:
		if v >= high+margin {
			return observer.SeverityHigh
		}
		if v < low-margin {
			return observer.SeverityLow
		}
		return observer.SeverityMedium

	default: // SeverityHigh
		if v < high-margin {
			return observer.SeverityMedium // one step down only
		}
		return observer.SeverityHigh
	}
}

// ScoreState returns a snapshot of accumulated telemetry. Thread-safe.
func (s *anomalyScorer) ScoreState() observer.ScoreState {
	s.mu.Lock()
	defer s.mu.Unlock()

	buckets := make([]observer.ScoreBucket, len(s.buckets))
	copy(buckets, s.buckets)
	events := make([]observer.SeverityEvent, len(s.events))
	copy(events, s.events)

	return observer.ScoreState{
		Buckets: buckets,
		Events:  events,
		Config:  s.config,
	}
}

// Reset clears all internal state. Implement observer.Scorer.
func (s *anomalyScorer) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pending = make(map[int64][]observer.Anomaly)
	s.ewma = 0
	s.state = observer.SeverityLow
	s.lastStateEntryTs = 0
	s.stateInitialized = false
	s.lastAdvancedSec = 0
	s.buckets = nil
	s.events = nil
}
