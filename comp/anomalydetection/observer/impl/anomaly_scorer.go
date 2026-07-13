// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	severityeventsimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/impl"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// ---------------------------------------------------------------------------
// Severity direction helpers.
// ---------------------------------------------------------------------------

// rawSeverityLevel returns the initial severity level using bare thresholds
// (no hysteresis). Used only for the first Advance call to seed the state.
func rawSeverityLevel(ewma float64, low, high float64) severityeventsdef.SeverityLevel {
	if ewma >= high {
		return severityeventsdef.SeverityHigh
	}
	if ewma >= low {
		return severityeventsdef.SeverityMedium
	}
	return severityeventsdef.SeverityLow
}

// nextSeverityLevel returns the next severity level given the current EWMA,
// the current state, and the thresholds with hysteresis margin applied to
// downward transitions.
func nextSeverityLevel(ewma float64, current severityeventsdef.SeverityLevel, low, high, margin float64) severityeventsdef.SeverityLevel {
	switch current {
	case severityeventsdef.SeverityLow:
		if ewma >= high {
			return severityeventsdef.SeverityHigh
		}
		if ewma >= low {
			return severityeventsdef.SeverityMedium
		}
		return severityeventsdef.SeverityLow
	case severityeventsdef.SeverityMedium:
		if ewma >= high {
			return severityeventsdef.SeverityHigh
		}
		// De-escalate only when EWMA drops below low - margin.
		if ewma < low-margin {
			return severityeventsdef.SeverityLow
		}
		return severityeventsdef.SeverityMedium
	case severityeventsdef.SeverityHigh:
		// De-escalate only when EWMA drops below high - margin.
		if ewma < high-margin {
			if ewma >= low {
				return severityeventsdef.SeverityMedium
			}
			return severityeventsdef.SeverityLow
		}
		return severityeventsdef.SeverityHigh
	}
	return severityeventsdef.SeverityLow
}

// severityLevelName returns a human-readable label for a SeverityLevel.
func severityLevelName(l severityeventsdef.SeverityLevel) string {
	switch l {
	case severityeventsdef.SeverityLow:
		return "Low"
	case severityeventsdef.SeverityMedium:
		return "Medium"
	case severityeventsdef.SeverityHigh:
		return "High"
	default:
		return fmt.Sprintf("SeverityLevel(%d)", int(l))
	}
}

// ---------------------------------------------------------------------------
// EWMA helpers
// ---------------------------------------------------------------------------

// levelWeights maps anomaly level (0–4) to its EWMA weight.
// Level 0=VeryLow, 1=Low, 2=Medium, 3=High, 4=XHigh.
var levelWeights = [5]float64{0.2, 0.5, 1.0, 2.0, 3.0}

// anomalyLevel assigns a 0–4 level to an anomaly.
// If the detector has an entry in cfg.DetectorThresholds, the numeric Score is
// compared against its four boundaries. Detectors without an entry (including
// unscored detectors such as bocpd) default to Medium (level 2).
func anomalyLevel(a observerdef.Anomaly, cfg observerdef.AnomalyScorerConfig) int {
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
func seriesID(a observerdef.Anomaly) string {
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

// secState is the per-second scorer state emitted after each Advance step.
type secState struct {
	sec   int64
	ewma  float64
	level severityeventsdef.SeverityLevel
}

// ---------------------------------------------------------------------------
// AnomalyScorerConfig (impl-level)
// ---------------------------------------------------------------------------

// AnomalyScorerConfig is the impl-level config for the unified anomaly scorer.
// It embeds the def-level EWMA parameters and adds output toggles for the
// internal watcher (logs, correlation events, cooldown, episode size cap).
type AnomalyScorerConfig struct {
	observerdef.AnomalyScorerConfig
	// Logs controls whether severity transitions are logged via pkglog.
	Logs bool `json:"logs"`
	// CorrelationEvents controls whether High-severity episodes are tracked
	// and returned by ActiveCorrelations() for the reporter pipeline.
	CorrelationEvents bool `json:"correlation_events"`
	// CooldownSecs is the minimum interval between de-escalation callbacks
	// from the internal watcher subscription.
	CooldownSecs int64 `json:"cooldown_secs"`
	// MaxEpisodeAnomalies caps the number of anomalies stored per episode.
	// 0 means no cap.
	MaxEpisodeAnomalies int `json:"max_episode_anomalies"`
}

// DefaultAnomalyScorerConfig returns calibrated defaults.
// Per-detector thresholds are set based on empirical score distributions across
// kafka-partition-saturation, postmark, and dns-upstream-outage scenarios.
func DefaultAnomalyScorerConfig() AnomalyScorerConfig {
	const window = 15
	return AnomalyScorerConfig{
		AnomalyScorerConfig: observerdef.AnomalyScorerConfig{
			Alpha:         0.014,
			SaturationK:   5.0,
			WindowSecs:    window,
			LowThreshold:  0.15,
			HighThreshold: 0.40,
			MarginPct:     0.20,
			// MaxBuckets intentionally left at zero: the trim logic defaults to WindowSecs.
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
		},
		Logs:                false,
		CorrelationEvents:   false,
		CooldownSecs:        300,
		MaxEpisodeAnomalies: 50,
	}
}

// readAnomalyScorerConfig reads scorer settings from the agent config.
// prefix is the key prefix, e.g. "anomaly_detection.anomaly_scorer.".
func readAnomalyScorerConfig(r ConfigReader, prefix string) AnomalyScorerConfig {
	cfg := DefaultAnomalyScorerConfig()
	ewma := &cfg.AnomalyScorerConfig
	defaults := DefaultAnomalyScorerConfig()

	key := prefix + "alpha"
	v := r.GetFloat64(key)
	if v <= 0 || v >= 1 {
		pkglog.Warnf("anomaly_scorer: %s must be in (0, 1), got %g — using default %g", key, v, defaults.Alpha)
		v = defaults.Alpha
	}
	ewma.Alpha = v

	key = prefix + "saturation_k"
	v = r.GetFloat64(key)
	if v <= 0 {
		pkglog.Warnf("anomaly_scorer: %s must be > 0, got %g — using default %g", key, v, defaults.SaturationK)
		v = defaults.SaturationK
	}
	ewma.SaturationK = v

	key = prefix + "window"
	d := r.GetDuration(key)
	if d < time.Second {
		pkglog.Warnf("anomaly_scorer: %s must be >= 1s, got %s — using default %ds", key, d, defaults.WindowSecs)
		d = time.Duration(defaults.WindowSecs) * time.Second
	}
	ewma.WindowSecs = int64(d.Seconds())

	ewma.LowThreshold = r.GetFloat64(prefix + "low_threshold")
	ewma.HighThreshold = r.GetFloat64(prefix + "high_threshold")
	ewma.MarginPct = r.GetFloat64(prefix + "margin_pct")

	outPrefix := prefix + "output."
	cfg.Logs = r.GetBool(outPrefix + "logs")
	cfg.CorrelationEvents = r.GetBool(outPrefix + "correlation_events")
	key = outPrefix + "cooldown"
	d = r.GetDuration(key)
	if d < 0 {
		pkglog.Warnf("anomaly_scorer: %s must be >= 0, got %s — using default %ds", key, d, defaults.CooldownSecs)
		d = time.Duration(defaults.CooldownSecs) * time.Second
	}
	cfg.CooldownSecs = int64(d.Seconds())
	cfg.MaxEpisodeAnomalies = r.GetInt(outPrefix + "max_anomalies")

	return cfg
}

// ---------------------------------------------------------------------------
// anomalyScorer — unified struct
// ---------------------------------------------------------------------------

// anomalyScorer is the unified implementation of the anomaly scoring pipeline.
//
// It has three concerns:
//  1. EWMA core: buffers and processes anomalies, maintains the deduplication
//     window, computes saturation + EWMA per second tick.
//  2. Event manager: a severityevents dispatcher that receives the scorer's
//     derived per-second severity levels and manages push subscriptions.
//  3. Internal watcher (optional): self-subscribes to the event manager when
//     telemetry gauges are provided; on each transition it sets gauges, optionally
//     logs the event, and optionally tracks High-severity episodes for
//     ActiveCorrelations() output.
//
// Implements observerdef.Correlator so the engine treats it like any other
// correlator. Also exposes Subscribe/LastScore/ScoreState for testbench replay.
//
// Lifecycle:
//
//	ProcessAnomaly → buffers raw anomaly keyed by its second.
//	Advance(t)     → finalises every second in [lastAdvancedSec+1, t]:
//	                   merge pending anomalies into windowMap,
//	                   evict stale series (older than WindowSecs),
//	                   compute saturation + EWMA from window,
//	                   derive the raw severity level, push ewmaGauge, and
//	                   feed the severityevents dispatcher.
//	ActiveCorrelations → returns closed High-severity episodes when enabled.
//	ScoreState()   → returns accumulated telemetry snapshot.
//	Reset()        → clears all internal state for reanalysis.
type anomalyScorer struct {
	mu sync.Mutex

	dispatchersMu sync.RWMutex

	config AnomalyScorerConfig

	// pending holds anomalies received since the last Advance, grouped by second.
	// Past-timestamped anomalies are clamped to lastAdvancedSec+1 in ProcessAnomaly,
	// so the past side is always drained on the next Advance. Future-timestamped
	// anomalies (sec > upcoming dataTime) accumulate until Advance reaches that
	// second; no current detector produces future timestamps, but there is no hard
	// cap if one ever does.
	pending map[int64][]observerdef.Anomaly

	// windowMap tracks the highest anomaly level seen per series within the
	// active window [lastAdvancedSec-WindowSecs+1, lastAdvancedSec].
	// Entries are evicted once lastSeenSec falls outside the window.
	windowMap map[string]windowEntry

	// EWMA state
	ewma float64

	lastAdvancedSec int64

	// buckets retains the most recent WindowSecs AnomalyScoreBucket entries for debug
	// and replay inspection via ScoreState(). Capped at WindowSecs to prevent
	// unbounded growth in long-running agents; older entries are discarded.
	buckets []observerdef.AnomalyScoreBucket

	// rawSeverityLevel is the scorer's uncooldowned per-second severity state,
	// derived directly from the EWMA stream.
	rawLevel            severityeventsdef.SeverityLevel
	rawLevelInitialized bool

	// dispatchers fan the raw severity stream out to push-based subscribers and
	// helper adapters like SeverityReader. Each dispatcher owns its own
	// filter/cooldown state machine.
	dispatchers []*severityeventsimpl.Dispatcher

	// Internal watcher fields (non-nil only when constructed with telemetry).
	ewmaGauge  telemetry.Gauge // may be nil; set on every Advance tick
	stateGauge telemetry.Gauge // may be nil; set on severity transitions

	// Episode tracking (guarded by mu; only active when correlationEvents is true).
	// openEpisode is the currently open High-severity period; nil when Low/Medium.
	// Closed episodes are no longer buffered here — they are emitted as EpisodeEnded
	// CorrelatorEvents via PendingEvents() and accumulated by the engine from there.
	openEpisode *observerdef.ActiveCorrelation

	// pendingEvents holds lifecycle events (EpisodeStarted/EpisodeEnded) produced
	// during the last Advance cycle. Drained once by the engine via PendingEvents().
	pendingEvents []observerdef.CorrelatorEvent
}

// StandaloneAnomalyScorer is the public interface for a scorer that is used
// independently of the observer engine (e.g. testbench replay path).
type StandaloneAnomalyScorer interface {
	SubscribeSeverityEvents(cfg severityeventsdef.SeverityEventsConfiguration, listener severityeventsdef.SeverityEventListener) (severityeventsdef.SeverityEventsSubscription, error)
	SubscribeSeverityEventsReader(cfg severityeventsdef.SeverityEventsConfiguration) (severityeventsdef.SeverityEventsReaderSubscription, error)
	ProcessAnomaly(a observerdef.Anomaly)
	Advance(dataTime int64)
	LastScore() float64
	ScoreState() observerdef.AnomalyScoreState
	Reset()
}

// newAnomalyScorerBase allocates and validates an anomalyScorer without wiring
// the internal watcher. Shared by both public constructors.
func newAnomalyScorerBase(cfg AnomalyScorerConfig) *anomalyScorer {
	defaults := DefaultAnomalyScorerConfig()
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
		pending:   make(map[int64][]observerdef.Anomaly),
		windowMap: make(map[string]windowEntry),
	}
}

// NewAnomalyScorer creates a new anomalyScorer with the given config.
// The watcher (telemetry gauges, logs, episodes) is not active.
// Used by the testbench replay path.
// Invalid EWMA parameter values are clamped to safe defaults.
func NewAnomalyScorer(cfg AnomalyScorerConfig) StandaloneAnomalyScorer {
	return newAnomalyScorerBase(cfg)
}

// advanceRawLevel updates the scorer's uncooldowned severity state from one
// EWMA tick and returns the resulting level. Must be called with mu held.
func (s *anomalyScorer) advanceRawLevel(ewma float64) severityeventsdef.SeverityLevel {
	if !s.rawLevelInitialized {
		s.rawLevel = rawSeverityLevel(ewma, s.config.LowThreshold, s.config.HighThreshold)
		s.rawLevelInitialized = true
		return s.rawLevel
	}
	margin := s.config.HighThreshold * s.config.MarginPct
	s.rawLevel = nextSeverityLevel(ewma, s.rawLevel, s.config.LowThreshold, s.config.HighThreshold, margin)
	return s.rawLevel
}

// newAnomalyScorerWithTelemetry creates a scorer with the watcher enabled.
// stateGauge and ewmaGauge are written on each severity transition and EWMA tick
// respectively. The watcher self-subscribes using cfg.CooldownSecs.
func newAnomalyScorerWithTelemetry(cfg AnomalyScorerConfig, stateGauge, ewmaGauge telemetry.Gauge) *anomalyScorer {
	s := newAnomalyScorerBase(cfg)
	s.ewmaGauge = ewmaGauge
	s.stateGauge = stateGauge

	// Self-subscribe as the internal watcher.
	if _, err := s.SubscribeSeverityEvents(severityeventsdef.SeverityEventsConfiguration{
		CooldownSecs: cfg.CooldownSecs,
	}, s); err != nil {
		pkglog.Errorf("[observer] anomaly scorer self-subscription failed: %v", err)
	}

	return s
}

// ---------------------------------------------------------------------------
// severityevents.SeverityEventListener — internal watcher callback
// ---------------------------------------------------------------------------

// OnSeverityTransition is called by the self-subscription on each severity
// transition. It sets the state gauge, optionally logs the event, manages
// High-severity episodes, and optionally sends a v2 change event.
func (s *anomalyScorer) OnSeverityTransition(evt severityeventsdef.SeverityEvent) {
	direction := "escalation"
	if evt.Direction == severityeventsdef.SeverityEventDeescalation {
		direction = "deescalation"
	}

	if s.config.Logs {
		pkglog.Infof("[observer] anomaly scorer %s severity %s to %s (was %s, t=%d)",
			s.Name(),
			direction,
			severityLevelName(evt.ToLevel),
			severityLevelName(evt.FromLevel),
			evt.Timestamp,
		)
	}

	if s.stateGauge != nil {
		s.stateGauge.Set(float64(evt.ToLevel), "scorer:"+s.Name(), direction)
	}

	if s.config.CorrelationEvents {
		s.mu.Lock()
		if evt.ToLevel == severityeventsdef.SeverityHigh {
			s.openEpisode = &observerdef.ActiveCorrelation{
				Pattern:     fmt.Sprintf("anomaly_scorer_high:%d", evt.Timestamp),
				Title:       "Anomaly scorer: high severity period",
				FirstSeen:   evt.Timestamp,
				LastUpdated: evt.Timestamp,
			}
			s.pendingEvents = append(s.pendingEvents, observerdef.CorrelatorEvent{
				Kind:           observerdef.CorrelatorEventEpisodeStarted,
				CorrelatorName: s.Name(),
				Timestamp:      evt.Timestamp,
				Correlation:    *s.openEpisode,
				FromLevel:      evt.FromLevel,
				ToLevel:        evt.ToLevel,
			})
		} else if evt.FromLevel == severityeventsdef.SeverityHigh && s.openEpisode != nil {
			ep := *s.openEpisode
			ep.LastUpdated = evt.Timestamp
			s.openEpisode = nil
			s.pendingEvents = append(s.pendingEvents, observerdef.CorrelatorEvent{
				Kind:           observerdef.CorrelatorEventEpisodeEnded,
				CorrelatorName: s.Name(),
				Timestamp:      evt.Timestamp,
				Correlation:    ep,
				FromLevel:      evt.FromLevel,
				ToLevel:        evt.ToLevel,
			})
		}
		s.mu.Unlock()
	}
}

// ---------------------------------------------------------------------------
// observerdef.Correlator implementation
// ---------------------------------------------------------------------------

func (s *anomalyScorer) Name() string { return "anomaly_scorer" }

// ProcessAnomaly buffers the anomaly into the pending map keyed by its second.
// If the anomaly's timestamp is in the past (already advanced past), it is
// clamped to lastAdvancedSec+1 so it participates in the next Advance call.
// Also appends to the open High-severity episode if one is active.
func (s *anomalyScorer) ProcessAnomaly(a observerdef.Anomaly) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec := a.Timestamp
	if s.lastAdvancedSec > 0 && sec <= s.lastAdvancedSec {
		sec = s.lastAdvancedSec + 1
	}
	s.pending[sec] = append(s.pending[sec], a)

	if s.openEpisode != nil {
		if s.config.MaxEpisodeAnomalies <= 0 || len(s.openEpisode.Anomalies) < s.config.MaxEpisodeAnomalies {
			s.openEpisode.Anomalies = append(s.openEpisode.Anomalies, a)
			if a.Timestamp > s.openEpisode.LastUpdated {
				s.openEpisode.LastUpdated = a.Timestamp
			}
		}
	}
}

// Advance finalises all 1-second buckets from lastAdvancedSec+1 up to dataTime
// (inclusive). After releasing mu, the derived raw severity stream is fed to
// the dispatcher, which may call listeners on resulting transitions.
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

	// Collect per-second EWMA values and derived raw severity levels to feed
	// the dispatcher after releasing the scorer lock.
	states := make([]secState, 0, int(dataTime-start+1))
	for sec := start; sec <= dataTime; sec++ {
		ewma := s.advanceSecond(sec)
		level := s.advanceRawLevel(ewma)
		states = append(states, secState{sec: sec, ewma: ewma, level: level})
	}
	s.lastAdvancedSec = dataTime

	s.mu.Unlock()

	// Push EWMA gauge for the last second of this advance tick.
	if s.ewmaGauge != nil && len(states) > 0 {
		last := states[len(states)-1]
		s.ewmaGauge.Set(last.ewma, s.Name())
	}

	// Drive the dispatchers outside the scorer lock so listeners can safely call
	// back into the scorer without deadlocking.
	s.dispatchersMu.RLock()
	dispatchers := append([]*severityeventsimpl.Dispatcher(nil), s.dispatchers...)
	s.dispatchersMu.RUnlock()
	for _, st := range states {
		for _, dispatcher := range dispatchers {
			dispatcher.Advance(st.sec, st.level)
		}
	}
}

// PendingEvents returns and drains typed lifecycle events accumulated during
// the last Advance call. The engine calls this once per advance cycle.
// Returns nil when no events are pending or CorrelationEvents is disabled.
// Implements observerdef.Correlator.
func (s *anomalyScorer) PendingEvents() []observerdef.CorrelatorEvent {
	if !s.config.CorrelationEvents {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pendingEvents) == 0 {
		return nil
	}
	evts := s.pendingEvents
	s.pendingEvents = nil
	return evts
}

// ActiveCorrelations returns the currently open High-severity episode (if any).
// Closed episodes are no longer buffered here; they are emitted as EpisodeEnded
// events via PendingEvents() and accumulated by the engine from there.
// Returns nil when correlationEvents is false or no episode is open.
func (s *anomalyScorer) ActiveCorrelations() []observerdef.ActiveCorrelation {
	if !s.config.CorrelationEvents {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.openEpisode == nil {
		return nil
	}
	return []observerdef.ActiveCorrelation{*s.openEpisode}
}

// Reset clears all internal EWMA/window state and resets every dispatcher so
// subscriptions re-seed on the next Advance call.
// Implements observerdef.Correlator.
func (s *anomalyScorer) Reset() {
	s.mu.Lock()
	s.pending = make(map[int64][]observerdef.Anomaly)
	s.windowMap = make(map[string]windowEntry)
	s.ewma = 0
	s.lastAdvancedSec = 0
	s.buckets = nil
	s.rawLevel = severityeventsdef.SeverityLow
	s.rawLevelInitialized = false
	s.openEpisode = nil
	s.pendingEvents = nil
	s.mu.Unlock()

	s.dispatchersMu.RLock()
	dispatchers := append([]*severityeventsimpl.Dispatcher(nil), s.dispatchers...)
	s.dispatchersMu.RUnlock()
	for _, dispatcher := range dispatchers {
		dispatcher.Reset()
	}
}

// ---------------------------------------------------------------------------
// Standalone scorer methods (retained for testbench replay)
// ---------------------------------------------------------------------------

// SubscribeSeverityEvents creates a dispatcher for cfg/listener, seeds it
// from the current raw severity when available, and returns it with an
// unsubscribe function.
func (s *anomalyScorer) SubscribeSeverityEvents(cfg severityeventsdef.SeverityEventsConfiguration, listener severityeventsdef.SeverityEventListener) (severityeventsdef.SeverityEventsSubscription, error) {
	if listener == nil {
		return severityeventsdef.SeverityEventsSubscription{}, errors.New("nil severity event listener")
	}

	dispatcher := severityeventsimpl.NewDispatcher(cfg, listener)

	s.mu.Lock()
	knownLevel := s.rawLevelInitialized
	lastSec := s.lastAdvancedSec
	level := s.rawLevel
	s.mu.Unlock()

	if knownLevel {
		dispatcher.DeliverInitial(lastSec, level)
	}

	s.dispatchersMu.Lock()
	s.dispatchers = append(s.dispatchers, dispatcher)
	s.dispatchersMu.Unlock()

	return severityeventsdef.SeverityEventsSubscription{
		Dispatcher: dispatcher,
		Unsubscribe: func() {
			s.dispatchersMu.Lock()
			defer s.dispatchersMu.Unlock()
			for i, existing := range s.dispatchers {
				if existing == dispatcher {
					s.dispatchers = append(s.dispatchers[:i], s.dispatchers[i+1:]...)
					return
				}
			}
		},
	}, nil
}

// SubscribeSeverityEventsReader is a convenience for pull-only consumers: it
// registers its own internal listener via SubscribeSeverityEvents and returns
// a Reader whose GetSeverity() reflects the latest delivered level.
func (s *anomalyScorer) SubscribeSeverityEventsReader(cfg severityeventsdef.SeverityEventsConfiguration) (severityeventsdef.SeverityEventsReaderSubscription, error) {
	return severityeventsimpl.NewSeverityReader(s, cfg)
}

// LastScore returns the most recently computed EWMA score. Thread-safe.
func (s *anomalyScorer) LastScore() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ewma
}

// ScoreState returns a snapshot of accumulated telemetry. Thread-safe.
func (s *anomalyScorer) ScoreState() observerdef.AnomalyScoreState {
	s.mu.Lock()
	defer s.mu.Unlock()

	buckets := make([]observerdef.AnomalyScoreBucket, len(s.buckets))
	copy(buckets, s.buckets)

	return observerdef.AnomalyScoreState{
		Buckets: buckets,
		Config:  s.config.AnomalyScorerConfig,
	}
}

// ---------------------------------------------------------------------------
// advanceSecond — EWMA core (called with mu held)
// ---------------------------------------------------------------------------

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
	for _, a := range anomalies {
		sid := seriesID(a)
		l := anomalyLevel(a, s.config.AnomalyScorerConfig)
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

	s.buckets = append(s.buckets, observerdef.AnomalyScoreBucket{
		Second:    sec,
		Bins:      bins,
		Count:     count,
		WeightSum: weightSum,
		Ewma:      s.ewma,
	})
	// Default cap is WindowSecs; MaxBuckets overrides this when set to a positive value.
	bucketCap := s.config.MaxBuckets
	if bucketCap <= 0 {
		bucketCap = s.config.WindowSecs
	}
	if int64(len(s.buckets)) > bucketCap {
		trimmed := make([]observerdef.AnomalyScoreBucket, bucketCap)
		copy(trimmed, s.buckets[int64(len(s.buckets))-bucketCap:])
		s.buckets = trimmed
	}

	return s.ewma
}
