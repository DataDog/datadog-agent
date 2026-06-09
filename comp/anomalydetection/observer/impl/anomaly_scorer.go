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

// scoredDetectors are the detectors that emit a numeric score to threshold against.
var scoredDetectors = map[string]bool{
	"holt_residual":  true,
	"tukey_biweight": true,
	"scanmw":         true,
	"scanwelch":      true,
}

// DefaultScorerConfig returns calibrated defaults.
// Per-detector thresholds are set based on empirical score distributions across
// kafka-partition-saturation, postmark, and dns-upstream-outage scenarios.
func DefaultScorerConfig() observer.ScorerConfig {
	return observer.ScorerConfig{
		Alpha:       0.014,
		SaturationK: 5.0,
		DetectorThresholds: map[string][4]float64{
			// tukey_biweight scores cap hard at ~50 across all scenarios.
			// Calibrated: p25≈6, p50≈9, p75≈15, p90≈27, p99≈45 (3-scenario avg).
			"tukey_biweight": {5, 8, 15, 30},
			// holt_residual can reach 400+ (dns outliers) but 99% stay below ~75.
			"holt_residual": {6, 12, 20, 35},
			// scanmw / scanwelch scores are -log10(p-value), floored at 8.0.
			// Calibrated: p25≈8.5, p50≈8.5–9.7, p90≈10–16, p99≈19–35.
			"scanmw":    {8, 10, 15, 25},
			"scanwelch": {8, 10, 15, 25},
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
	return 2 // non-scored detectors (e.g. bocpd) default to Medium
}

// seriesID returns a stable string key for deduplication.
// Returns "" when the anomaly has no identifiable series (never deduplicated then).
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

// anomalyScorer is the streaming implementation of observer.AnomalyScorer.
//
// Lifecycle:
//
//	ProcessAnomaly → buffers raw anomaly keyed by its second.
//	Advance(t)     → finalises every second in [lastAdvancedSec+1, t],
//	                  computing dedup → bucketing → EWMA.
//	ScoreState()   → returns accumulated telemetry snapshot.
//	Reset()        → clears all internal state.
type anomalyScorer struct {
	mu sync.Mutex

	config observer.ScorerConfig

	// pending holds anomalies received since the last Advance, grouped by second.
	pending map[int64][]observer.Anomaly

	// EWMA state
	ewma float64

	lastAdvancedSec int64

	// Accumulated telemetry
	buckets []observer.ScoreBucket
}

// NewScorer creates a new anomalyScorer with the given config.
func NewScorer(cfg observer.ScorerConfig) *anomalyScorer {
	return &anomalyScorer{
		config:  cfg,
		pending: make(map[int64][]observer.Anomaly),
	}
}

func (s *anomalyScorer) Name() string { return "anomaly_scorer" }

// LastScore returns the most recently computed EWMA score. Thread-safe.
func (s *anomalyScorer) LastScore() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ewma
}

// ProcessAnomaly buffers the anomaly into the pending map keyed by its second.
func (s *anomalyScorer) ProcessAnomaly(a observer.Anomaly) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec := a.Timestamp
	s.pending[sec] = append(s.pending[sec], a)
}

// Advance finalises all 1-second buckets from lastAdvancedSec+1 up to dataTime
// (inclusive), running dedup → bucketing → EWMA for each.
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

// advanceSecond processes a single second, updating EWMA state. Must be called
// with mu held.
func (s *anomalyScorer) advanceSecond(sec int64) {
	anomalies := s.pending[sec]
	delete(s.pending, sec)

	// Deduplication — per (second, seriesID) keep the highest level.
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

	// Bucketing.
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

	// Saturated input → EWMA.
	var input float64
	if count > 0 {
		meanWeight := weightSum / float64(count)
		input = meanWeight * (1 - math.Exp(-float64(count)/s.config.SaturationK))
	}

	s.ewma = s.config.Alpha*input + (1-s.config.Alpha)*s.ewma

	s.buckets = append(s.buckets, observer.ScoreBucket{
		Second:    sec,
		Bins:      bins,
		Count:     count,
		WeightSum: weightSum,
		Ewma:      s.ewma,
	})
}

// ScoreState returns a snapshot of accumulated telemetry. Thread-safe.
func (s *anomalyScorer) ScoreState() observer.ScoreState {
	s.mu.Lock()
	defer s.mu.Unlock()

	buckets := make([]observer.ScoreBucket, len(s.buckets))
	copy(buckets, s.buckets)

	return observer.ScoreState{
		Buckets: buckets,
		Config:  s.config,
	}
}

// Reset clears all internal state. Implements observer.AnomalyScorer.
func (s *anomalyScorer) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pending = make(map[int64][]observer.Anomaly)
	s.ewma = 0
	s.lastAdvancedSec = 0
	s.buckets = nil
}
