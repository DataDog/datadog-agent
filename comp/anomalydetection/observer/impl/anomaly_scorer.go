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
		WindowSecs:  15,
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
	if key := prefix + "window_secs"; r.IsConfigured(key) {
		cfg.WindowSecs = int64(r.GetInt(key))
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

// windowEntry tracks the last second at which each anomaly level (0–4) was
// observed for a series within the active window. Index = level, value = last
// second observed (0 = never seen or already evicted). Storing per-level
// timestamps (rather than a single max-level + lastSeenSec) ensures that when
// a high-severity peak expires from the window, the series is re-scored at the
// highest level that still has an active timestamp, rather than carrying the
// stale peak forward.
type windowEntry [5]int64

// anomalyScorer is the streaming implementation of observer.AnomalyScorer.
//
// Lifecycle:
//
//	ProcessAnomaly → buffers raw anomaly keyed by its second.
//	Advance(t)     → finalises every second in [lastAdvancedSec+1, t]:
//	                   merge pending anomalies into windowMap,
//	                   evict stale series (older than WindowSecs),
//	                   compute saturation + EWMA from window.
//	ScoreState()   → returns accumulated telemetry snapshot.
//	Reset()        → clears all internal state.
//
// The saturation input is the count of *unique anomalous series* currently
// in the window, not the per-second event count. This means a single noisy
// series caps at 1 regardless of how often it fires, and the score reflects
// "how many distinct series are currently misbehaving" rather than "how many
// anomaly events occurred".
type anomalyScorer struct {
	mu sync.Mutex

	config observer.ScorerConfig

	// pending holds anomalies received since the last Advance, grouped by second.
	// Past-timestamped anomalies are clamped to lastAdvancedSec+1 in ProcessAnomaly,
	// so the past side is always drained on the next Advance. Future-timestamped
	// anomalies (sec > upcoming dataTime) accumulate until Advance reaches that
	// second; no current detector produces future timestamps, but there is no hard
	// cap if one ever does.
	pending map[int64][]observer.Anomaly

	// windowMap tracks the highest anomaly level seen per series within the
	// active window [lastAdvancedSec-WindowSecs+1, lastAdvancedSec].
	// Entries are evicted once lastSeenSec falls outside the window.
	windowMap map[string]windowEntry

	// EWMA state
	ewma float64

	lastAdvancedSec int64

	// buckets accumulates ScoreBucket entries for test-bench and replay
	// inspection via ScoreState(). It is NOT used by the live observer path,
	// which only calls LastScore(). Left unbounded for now — see reviewer note
	// about capping to WindowSecs to avoid unbounded growth in long-running agents.
	buckets []observer.ScoreBucket
}

// NewScorer creates a new anomalyScorer with the given config.
func NewScorer(cfg observer.ScorerConfig) observer.AnomalyScorer {
	return &anomalyScorer{
		config:    cfg,
		pending:   make(map[int64][]observer.Anomaly),
		windowMap: make(map[string]windowEntry),
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
// If the anomaly's timestamp is in the past (already advanced past), it is
// clamped to lastAdvancedSec+1 so it participates in the next Advance call
// rather than leaking into a pending bucket that will never be processed.
// This handles scan detectors (scanmw/scanwelch) that emit changepoints with
// historical timestamps after the scorer has already moved forward.
func (s *anomalyScorer) ProcessAnomaly(a observer.Anomaly) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec := a.Timestamp
	if s.lastAdvancedSec > 0 && sec <= s.lastAdvancedSec {
		// The bucket for sec has already been finalized. Clamp to the next
		// unprocessed second so the anomaly is picked up by the next Advance.
		sec = s.lastAdvancedSec + 1
	}
	s.pending[sec] = append(s.pending[sec], a)
}

// Advance finalises all 1-second buckets from lastAdvancedSec+1 up to dataTime
// (inclusive), running merge → evict → saturate → EWMA for each.
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

// advanceSecond processes a single second. Must be called with mu held.
//
// Steps:
//  1. Merge: update windowMap with anomalies for this second (max level per series).
//  2. Evict: remove series last seen before the window start.
//  3. Bucket: count unique series in the window by level.
//  4. Saturate + EWMA: compute the smoothed score from the window count.
func (s *anomalyScorer) advanceSecond(sec int64) {
	anomalies := s.pending[sec]
	delete(s.pending, sec)

	// Step 1: merge new anomalies into the window.
	// Keyed anomalies record the latest second per level; unkeyed are counted separately.
	var unkeyedCount int
	var unkeyedWeightSum float64

	for _, a := range anomalies {
		sid := seriesID(a)
		l := anomalyLevel(a, s.config)
		if sid == "" {
			unkeyedCount++
			unkeyedWeightSum += levelWeights[l]
			continue
		}
		e := s.windowMap[sid]
		if sec > e[l] {
			e[l] = sec
		}
		s.windowMap[sid] = e
	}

	// Step 2: evict per-level timestamps that have fallen out of the window,
	// and remove the series entirely when no level remains active.
	windowStart := sec - s.config.WindowSecs + 1
	for sid, e := range s.windowMap {
		alive := false
		for lvl := 0; lvl < 5; lvl++ {
			if e[lvl] > 0 && e[lvl] < windowStart {
				e[lvl] = 0
			}
			if e[lvl] > 0 {
				alive = true
			}
		}
		if !alive {
			delete(s.windowMap, sid)
		} else {
			s.windowMap[sid] = e
		}
	}

	// Step 3: bucket from the live window.
	// Each series contributes at the highest level that still has an active timestamp.
	var bins [5]int
	var count int
	var weightSum float64

	for _, e := range s.windowMap {
		maxLevel := -1
		for lvl := 4; lvl >= 0; lvl-- {
			if e[lvl] > 0 {
				maxLevel = lvl
				break
			}
		}
		if maxLevel >= 0 {
			bins[maxLevel]++
			count++
			weightSum += levelWeights[maxLevel]
		}
	}
	// Add unkeyed anomalies from this second on top.
	count += unkeyedCount
	weightSum += unkeyedWeightSum

	// Step 4: saturated input → EWMA.
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
	s.windowMap = make(map[string]windowEntry)
	s.ewma = 0
	s.lastAdvancedSec = 0
	s.buckets = nil
}
