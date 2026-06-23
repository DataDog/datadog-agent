// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"sync"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// rawSeverityLevel returns the initial severity level using bare thresholds
// (no hysteresis). Used only for the first Advance call to seed the state.
func rawSeverityLevel(ewma float64, low, high float64) observer.SeverityLevel {
	if ewma >= high {
		return observer.SeverityHigh
	}
	if ewma >= low {
		return observer.SeverityMedium
	}
	return observer.SeverityLow
}

// nextSeverityLevel returns the next severity level given the current EWMA,
// the current state, and the thresholds with hysteresis margin applied to
// downward transitions.
func nextSeverityLevel(ewma float64, current observer.SeverityLevel, low, high, margin float64) observer.SeverityLevel {
	switch current {
	case observer.SeverityLow:
		if ewma >= high {
			return observer.SeverityHigh
		}
		if ewma >= low {
			return observer.SeverityMedium
		}
		return observer.SeverityLow
	case observer.SeverityMedium:
		if ewma >= high {
			return observer.SeverityHigh
		}
		// De-escalate only when EWMA drops below low - margin.
		if ewma < low-margin {
			return observer.SeverityLow
		}
		return observer.SeverityMedium
	case observer.SeverityHigh:
		// De-escalate only when EWMA drops below high - margin.
		if ewma < high-margin {
			if ewma >= low {
				return observer.SeverityMedium
			}
			return observer.SeverityLow
		}
		return observer.SeverityHigh
	}
	return observer.SeverityLow
}

// severityDirection returns the direction of a state-machine transition.
func severityDirection(from, to observer.SeverityLevel) observer.ScorerEventDirection {
	if to > from {
		return observer.ScorerEventEscalation
	}
	return observer.ScorerEventDeescalation
}

// scorerEventFilterMatches reports whether evt satisfies all conditions of f.
// A nil or empty slice in FromLevels / ToLevels means "any value".
// The zero-value ScorerEventFilter matches every transition.
func scorerEventFilterMatches(f observer.ScorerEventFilter, evt observer.SeverityEvent) bool {
	if len(f.FromLevels) > 0 {
		found := false
		for _, l := range f.FromLevels {
			if evt.FromLevel == l {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(f.ToLevels) > 0 {
		found := false
		for _, l := range f.ToLevels {
			if evt.ToLevel == l {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	switch f.Direction {
	case observer.ScorerEventEscalation:
		if evt.ToLevel <= evt.FromLevel {
			return false
		}
	case observer.ScorerEventDeescalation:
		if evt.ToLevel >= evt.FromLevel {
			return false
		}
	}
	return true
}

// scorerSubscription is a registered listener with its own per-subscription
// state machine. Stored as a pointer so state mutations persist across the
// snapshot copy taken inside Advance without holding subsMu during callbacks.
type scorerSubscription struct {
	cfg observer.AnomalyScorerConfiguration

	// Per-subscription severity state machine — uses cfg.CooldownSecs as its cooldown parameter.
	state            observer.SeverityLevel
	lastStateEntryTs int64
	stateInitialized bool
}

// advance runs the subscription's own severity state machine for one second.
// Returns the transition event and true if a qualifying transition occurred.
func (sub *scorerSubscription) advance(sec int64, ewma float64, cfg observer.ScorerConfig) (observer.SeverityEvent, bool) {
	margin := cfg.HighThreshold * cfg.MarginPct

	if !sub.stateInitialized {
		sub.state = rawSeverityLevel(ewma, cfg.LowThreshold, cfg.HighThreshold)
		sub.stateInitialized = true
		return observer.SeverityEvent{}, false
	}

	next := nextSeverityLevel(ewma, sub.state, cfg.LowThreshold, cfg.HighThreshold, margin)
	if next == sub.state {
		return observer.SeverityEvent{}, false
	}

	// Apply per-subscription cooldown on de-escalations.
	cooldown := sub.cfg.CooldownSecs
	if next < sub.state && cooldown > 0 && sec-sub.lastStateEntryTs < cooldown {
		return observer.SeverityEvent{}, false
	}

	evt := observer.SeverityEvent{
		Timestamp: sec,
		FromLevel: sub.state,
		ToLevel:   next,
		Direction: severityDirection(sub.state, next),
	}
	sub.state = next
	sub.lastStateEntryTs = sec
	return evt, true
}

// levelWeights maps anomaly level (0–4) to its EWMA weight.
// Level 0=VeryLow, 1=Low, 2=Medium, 3=High, 4=XHigh.
var levelWeights = [5]float64{0.2, 0.5, 1.0, 2.0, 3.0}

// DefaultScorerConfig returns calibrated defaults.
// Per-detector thresholds are set based on empirical score distributions across
// kafka-partition-saturation, postmark, and dns-upstream-outage scenarios.
func DefaultScorerConfig() observer.ScorerConfig {
	return observer.ScorerConfig{
		Alpha:         0.014,
		SaturationK:   5.0,
		WindowSecs:    15,
		LowThreshold:  0.040,
		HighThreshold: 0.060,
		MarginPct:     0.20,
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
	defaults := DefaultScorerConfig()
	cfg := defaults
	if key := prefix + "alpha"; r.IsConfigured(key) {
		v := r.GetFloat64(key)
		if v <= 0 || v >= 1 {
			pkglog.Warnf("anomaly_scorer: %s must be in (0, 1), got %g — using default %g", key, v, defaults.Alpha)
			v = defaults.Alpha
		}
		cfg.Alpha = v
	}
	if key := prefix + "saturation_k"; r.IsConfigured(key) {
		v := r.GetFloat64(key)
		if v <= 0 {
			pkglog.Warnf("anomaly_scorer: %s must be > 0, got %g — using default %g", key, v, defaults.SaturationK)
			v = defaults.SaturationK
		}
		cfg.SaturationK = v
	}
	if key := prefix + "window_secs"; r.IsConfigured(key) {
		v := r.GetInt(key)
		if v < 1 {
			pkglog.Warnf("anomaly_scorer: %s must be >= 1, got %d — using default %d", key, v, defaults.WindowSecs)
			v = int(defaults.WindowSecs)
		}
		cfg.WindowSecs = int64(v)
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
	return cfg
}

// anomalyLevel assigns a 0–4 level to an anomaly.
// If the detector has an entry in cfg.DetectorThresholds, the numeric Score is
// compared against its four boundaries. Detectors without an entry (including
// unscored detectors such as bocpd) default to Medium (level 2).
func anomalyLevel(a observer.Anomaly, cfg observer.ScorerConfig) int {
	if thresholds, ok := cfg.DetectorThresholds[a.DetectorName]; ok {
		if a.Score == nil {
			return 0 // treat nil score from a scored detector as VeryLow
		}
		s := *a.Score
		for i, t := range thresholds {
			if s < t {
				return i
			}
		}
		return 4
	}
	return 2 // detectors without explicit thresholds default to Medium
}

// seriesID returns a stable string key for deduplication.
// Prefers SourceRef.CompactID() when available (set by the metrics pipeline);
// falls back to Source.Key() otherwise. SeriesDescriptor.Key() always returns
// a non-empty string, so the result is never "".
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

// secEWMA is a (timestamp, ewma) pair collected during Advance and used to
// drive per-subscription state machines outside the scorer's mu lock.
type secEWMA struct {
	sec  int64
	ewma float64
}

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

	// buckets retains the most recent WindowSecs ScoreBucket entries for debug
	// and replay inspection via ScoreState(). Capped at WindowSecs to prevent
	// unbounded growth in long-running agents; older entries are discarded.
	// Not used by the live observer path, which only calls LastScore().
	buckets []observer.ScoreBucket

	// Subscriptions — guarded by subsMu, independent of mu so that listeners
	// can be registered or removed while Advance is running.
	// The slice holds pointers so per-subscription state (lastStateEntryTs)
	// survives the snapshot copy taken inside Advance.
	subsMu sync.RWMutex
	subs   []*scorerSubscription
}

// NewScorer creates a new anomalyScorer with the given config.
// Invalid parameter values are clamped to safe defaults to prevent panics
// (e.g. a non-positive WindowSecs would cause make() to panic in the trim path).
func NewScorer(cfg observer.ScorerConfig) observer.AnomalyScorer {
	defaults := DefaultScorerConfig()
	if cfg.WindowSecs < 1 {
		cfg.WindowSecs = defaults.WindowSecs
	}
	if cfg.Alpha <= 0 || cfg.Alpha >= 1 {
		cfg.Alpha = defaults.Alpha
	}
	if cfg.SaturationK <= 0 {
		cfg.SaturationK = defaults.SaturationK
	}
	return &anomalyScorer{
		config:    cfg,
		pending:   make(map[int64][]observer.Anomaly),
		windowMap: make(map[string]windowEntry),
	}
}

// Subscribe registers cfg.Listener to receive severity transitions matching
// cfg.Filter. Each subscription runs its own state machine using cfg.CooldownSecs.
// Returns an unsubscribe function. Safe to call concurrently.
// Panics if cfg.Listener is nil.
func (s *anomalyScorer) Subscribe(cfg observer.AnomalyScorerConfiguration) func() {
	if cfg.Listener == nil {
		panic("anomalyScorer.Subscribe: Listener must not be nil")
	}
	sub := &scorerSubscription{cfg: cfg}

	s.subsMu.Lock()
	s.subs = append(s.subs, sub)
	s.subsMu.Unlock()

	return func() {
		s.subsMu.Lock()
		defer s.subsMu.Unlock()
		for i, existing := range s.subs {
			if existing == sub {
				s.subs = append(s.subs[:i], s.subs[i+1:]...)
				return
			}
		}
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
// After releasing mu, each subscription's own state machine is advanced with
// the same EWMA values and calls its listener on any resulting transition.
func (s *anomalyScorer) Advance(dataTime int64) {
	s.mu.Lock()

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

	// Collect per-second EWMA values to feed subscription state machines later.
	ewmas := make([]secEWMA, 0, int(dataTime-start+1))
	for sec := start; sec <= dataTime; sec++ {
		ewma := s.advanceSecond(sec)
		ewmas = append(ewmas, secEWMA{sec: sec, ewma: ewma})
	}
	s.lastAdvancedSec = dataTime
	cfg := s.config // snapshot for subscription state machines

	s.mu.Unlock()

	// Drive each subscription's independent state machine outside the lock so
	// listeners can safely call back into the scorer without deadlocking.
	s.subsMu.RLock()
	subs := make([]*scorerSubscription, len(s.subs))
	copy(subs, s.subs)
	s.subsMu.RUnlock()

	for _, sub := range subs {
		for _, se := range ewmas {
			if evt, ok := sub.advance(se.sec, se.ewma, cfg); ok {
				if scorerEventFilterMatches(sub.cfg.Filter, evt) {
					sub.cfg.Listener.OnSeverityTransition(evt)
				}
			}
		}
	}
}

// advanceSecond processes a single second, updating all state. Must be called
// with mu held. Returns the resulting EWMA value for the second.
//
// Steps:
//  1. Merge: record the latest second per level for each series in windowMap.
//  2. Evict: remove per-level timestamps that have fallen outside the window.
//  3. Bucket: count unique live series by their highest active level.
//  4. Saturate + EWMA: compute the smoothed score from the window count.
func (s *anomalyScorer) advanceSecond(sec int64) float64 {
	anomalies := s.pending[sec]
	delete(s.pending, sec)

	// Step 1: merge new anomalies into the window.
	// seriesID always returns a non-empty key, so every anomaly is keyed.
	for _, a := range anomalies {
		sid := seriesID(a)
		l := anomalyLevel(a, s.config)
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
	if int64(len(s.buckets)) > s.config.WindowSecs {
		trimmed := make([]observer.ScoreBucket, s.config.WindowSecs)
		copy(trimmed, s.buckets[int64(len(s.buckets))-s.config.WindowSecs:])
		s.buckets = trimmed
	}

	return s.ewma
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

// Reset clears all internal EWMA/window state and resets every subscription's
// state machine so they re-seed on the next Advance call. Implements observer.AnomalyScorer.
func (s *anomalyScorer) Reset() {
	s.mu.Lock()
	s.pending = make(map[int64][]observer.Anomaly)
	s.windowMap = make(map[string]windowEntry)
	s.ewma = 0
	s.lastAdvancedSec = 0
	s.buckets = nil
	s.mu.Unlock()

	// Reset each subscription's state machine so stale state from before
	// the reset cannot produce spurious transitions or block cooldowns.
	s.subsMu.RLock()
	for _, sub := range s.subs {
		sub.stateInitialized = false
		sub.lastStateEntryTs = 0
	}
	s.subsMu.RUnlock()
}
