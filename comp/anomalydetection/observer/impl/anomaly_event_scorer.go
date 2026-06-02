// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"crypto/sha256"
	"fmt"
	"math"
	"sort"
	"strings"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// Scoring constants for the PoC anomaly event scorer.
const (
	// defaultMissingAnomalyScore is used when a detector did not attach a score.
	defaultMissingAnomalyScore = 0.5

	// Log-count cap parameters.
	// The cap grows logarithmically with the total number of anomalies in the window:
	//   cap(N) = logCapBase + (logCapMax - logCapBase) * log(1+N) / log(1+logCapRef)
	//
	// Calibration (1-minute window):
	//   N=1  → ~0.57   N=3  → ~0.72   N=5  → ~0.81   N=10 → 0.95 (ref)
	logCapBase = 0.45
	logCapMax  = 0.95
	logCapRef  = 10.0

	// Severity thresholds (applied to the EWMA score).
	mediumSeverityThreshold = 0.40
	highSeverityThreshold   = 0.75
	// severityHysteresis prevents rapid flapping around a threshold.
	// Up-crossing uses the raw threshold; down-crossing requires score to fall
	// below threshold - hysteresis.
	severityHysteresis = 0.05

	// EWMA parameters.
	// alpha = 0.30 gives a half-life of ~2 events, smoothing burst patterns.
	defaultAnomalyEventEWMAAlpha = 0.30
	// trendEpsilon prevents treating float noise as a meaningful trend change.
	defaultTrendEpsilon = 0.03

	// Sliding window.
	defaultEventWindowSeconds  = 1 * 60
	defaultEventWindowMaxItems = 100
)

// scopeScoreState holds per-scope EWMA and severity state between events.
type scopeScoreState struct {
	EWMA     float64
	Severity observerdef.AnomalyEventSeverity
}

// anomalyEventScorerConfig holds construction parameters.
type anomalyEventScorerConfig struct {
	windowSeconds int64
	maxItems      int
	ewmaAlpha     float64
	trendEpsilon  float64
}

// anomalyEventScorer scores every new detector anomaly and emits one
// ScoredAnomalyEvent candidate. It maintains:
//   - a bounded sliding window of recent anomalies for instant scoring;
//   - per-scope EWMA state for trend and hysteresis-based severity.
//
// Not goroutine-safe — must be driven from the single engine goroutine.
type anomalyEventScorer struct {
	cfg anomalyEventScorerConfig

	// window holds the current sliding window of anomalies (newest last).
	window []observerdef.Anomaly

	// scopeState maps scope key → EWMA + severity state.
	scopeState map[string]scopeScoreState

	// events holds all ScoredAnomalyEvents emitted so far.
	events []observerdef.ScoredAnomalyEvent
}

// newAnomalyEventScorer creates a scorer with the given configuration.
// Zero values in cfg use the package-level defaults.
func newAnomalyEventScorer(cfg anomalyEventScorerConfig) *anomalyEventScorer {
	if cfg.windowSeconds == 0 {
		cfg.windowSeconds = defaultEventWindowSeconds
	}
	if cfg.maxItems == 0 {
		cfg.maxItems = defaultEventWindowMaxItems
	}
	if cfg.ewmaAlpha == 0 {
		cfg.ewmaAlpha = defaultAnomalyEventEWMAAlpha
	}
	if cfg.trendEpsilon == 0 {
		cfg.trendEpsilon = defaultTrendEpsilon
	}
	return &anomalyEventScorer{
		cfg:        cfg,
		scopeState: make(map[string]scopeScoreState),
	}
}

// ProcessAnomaly adds the anomaly to the sliding window, computes the instant
// and EWMA scores, applies hysteresis-based severity, and returns the event.
func (s *anomalyEventScorer) ProcessAnomaly(a observerdef.Anomaly) observerdef.ScoredAnomalyEvent {
	// 1. Add trigger to the window.
	s.window = append(s.window, a)

	// 2. Evict anomalies older than the configured window.
	if len(s.window) > 0 {
		cutoff := a.Timestamp - s.cfg.windowSeconds
		start := 0
		for start < len(s.window) && s.window[start].Timestamp < cutoff {
			start++
		}
		if start > 0 {
			s.window = s.window[start:]
		}
	}

	// 3. Trim to maxItems (keep newest).
	if len(s.window) > s.cfg.maxItems {
		s.window = s.window[len(s.window)-s.cfg.maxItems:]
	}

	windowStart := int64(0)
	windowEnd := a.Timestamp
	if len(s.window) > 0 {
		windowStart = s.window[0].Timestamp
	}

	// 4. Group recent anomalies by signal key.
	bySignal := make(map[string][]observerdef.Anomaly)
	for _, ra := range s.window {
		key := ra.Source.Key()
		bySignal[key] = append(bySignal[key], ra)
	}

	signalKeys := make([]string, 0, len(bySignal))
	for k := range bySignal {
		signalKeys = append(signalKeys, k)
	}
	sort.Strings(signalKeys)

	// 5. Per-signal noisy-OR: signal_score = 1 - prod(1 - a_score_i).
	signals := make([]observerdef.SignalEvidence, 0, len(signalKeys))
	perSignalScores := make(map[string]float64, len(signalKeys))
	missingScoreCount := 0
	detectorAnomalyCount := 0

	for _, key := range signalKeys {
		anomalies := bySignal[key]
		detectorAnomalyCount += len(anomalies)
		complement := 1.0
		for _, ra := range anomalies {
			var sc float64
			if ra.Score != nil {
				sc = *ra.Score
			} else {
				sc = defaultMissingAnomalyScore
				missingScoreCount++
			}
			complement *= (1.0 - sc)
		}
		signalScore := 1.0 - complement
		perSignalScores[key] = signalScore
		signals = append(signals, observerdef.SignalEvidence{
			Key:       key,
			Score:     signalScore,
			Severity:  rawSeverityFromScore(signalScore),
			Anomalies: anomalies,
		})
	}

	// 6. Cross-signal noisy-OR over all distinct signals.
	sort.Slice(signals, func(i, j int) bool { return signals[i].Score > signals[j].Score })
	effectiveSignalCount := len(signals)

	eventComplement := 1.0
	for _, sig := range signals {
		eventComplement *= (1.0 - sig.Score)
	}
	rawEventScore := 1.0 - eventComplement

	// 7. Logarithmic count cap: more anomalies → higher instant score.
	windowCount := float64(len(s.window))
	logCountCap := logCapBase + (logCapMax-logCapBase)*math.Log1p(windowCount)/math.Log1p(logCapRef)
	if logCountCap > logCapMax {
		logCountCap = logCapMax
	}
	logCapped := false
	instantScore := rawEventScore
	if instantScore > logCountCap {
		instantScore = logCountCap
		logCapped = true
	}
	instantScore = math.Max(0, math.Min(1, instantScore))

	// 8. EWMA update per scope.
	scope := scopeKey(a)
	prev := s.scopeState[scope]
	prevEWMA := prev.EWMA
	prevSeverity := prev.Severity

	ewma := s.cfg.ewmaAlpha*instantScore + (1-s.cfg.ewmaAlpha)*prevEWMA

	// 9. Trend from EWMA delta.
	delta := ewma - prevEWMA
	var trend observerdef.AnomalyEventTrend
	switch {
	case delta > s.cfg.trendEpsilon:
		trend = observerdef.AnomalyEventTrendIncreased
	case delta < -s.cfg.trendEpsilon:
		trend = observerdef.AnomalyEventTrendDecreased
	default:
		trend = observerdef.AnomalyEventTrendStable
	}

	// 10. Severity from EWMA with hysteresis.
	severity := eventSeverityWithHysteresis(ewma, prevSeverity)
	severityChanged := prevSeverity != "" && prevSeverity != severity

	// Update scope state.
	s.scopeState[scope] = scopeScoreState{EWMA: ewma, Severity: severity}

	// 11. Emit the ScoredAnomalyEvent.
	evt := observerdef.ScoredAnomalyEvent{
		ID:      eventID(a),
		Scope:   scope,
		Anomaly: a,
		Score: observerdef.AnomalyEventScore{
			Instant:          instantScore,
			EWMA:             ewma,
			PreviousEWMA:     prevEWMA,
			Severity:         severity,
			PreviousSeverity: prevSeverity,
			SeverityChanged:  severityChanged,
			Trend:            trend,
		},
		Window: observerdef.AnomalyEventWindow{
			StartSec: windowStart,
			EndSec:   windowEnd,
			Size:     len(s.window),
			MaxSize:  s.cfg.maxItems,
		},
		Signals: signals,
		Breakdown: observerdef.AnomalyEventScoreBreakdown{
			SignalCount:           len(signals),
			EffectiveSignalCount:  effectiveSignalCount,
			DetectorAnomalyCount:  detectorAnomalyCount,
			MissingScoreCount:     missingScoreCount,
			PerSignalScores:       perSignalScores,
			CombinedEvidenceScore: rawEventScore,
			LogCountCapApplied:    logCapped,
			LogCountCap:           logCountCap,
			WindowAnomalyCount:    len(s.window),
		},
	}
	s.events = append(s.events, evt)
	return evt
}

// Events returns a copy of all events emitted so far.
func (s *anomalyEventScorer) Events() []observerdef.ScoredAnomalyEvent {
	result := make([]observerdef.ScoredAnomalyEvent, len(s.events))
	copy(result, s.events)
	return result
}

// Reset clears all state.
func (s *anomalyEventScorer) Reset() {
	s.window = nil
	s.scopeState = make(map[string]scopeScoreState)
	s.events = nil
}

// eventSeverityWithHysteresis maps an EWMA score to a severity using hysteresis
// to prevent rapid flapping around a threshold.
//
//	low    → medium: requires ewma >= mediumSeverityThreshold
//	medium → low:    requires ewma <  mediumSeverityThreshold - severityHysteresis
//	medium → high:   requires ewma >= highSeverityThreshold
//	high   → medium: requires ewma <  highSeverityThreshold   - severityHysteresis
func eventSeverityWithHysteresis(ewma float64, prev observerdef.AnomalyEventSeverity) observerdef.AnomalyEventSeverity {
	switch prev {
	case observerdef.AnomalyEventSeverityHigh:
		if ewma >= highSeverityThreshold-severityHysteresis {
			return observerdef.AnomalyEventSeverityHigh
		}
		if ewma >= mediumSeverityThreshold-severityHysteresis {
			return observerdef.AnomalyEventSeverityMedium
		}
		return observerdef.AnomalyEventSeverityLow
	case observerdef.AnomalyEventSeverityMedium:
		if ewma >= highSeverityThreshold {
			return observerdef.AnomalyEventSeverityHigh
		}
		if ewma >= mediumSeverityThreshold-severityHysteresis {
			return observerdef.AnomalyEventSeverityMedium
		}
		return observerdef.AnomalyEventSeverityLow
	default: // low or unset
		if ewma >= highSeverityThreshold {
			return observerdef.AnomalyEventSeverityHigh
		}
		if ewma >= mediumSeverityThreshold {
			return observerdef.AnomalyEventSeverityMedium
		}
		return observerdef.AnomalyEventSeverityLow
	}
}

// rawSeverityFromScore maps a raw signal score to an AnomalySeverity (no hysteresis).
// Used for per-signal display only; events use eventSeverityWithHysteresis.
func rawSeverityFromScore(score float64) observerdef.AnomalySeverity {
	switch {
	case score >= highSeverityThreshold:
		return observerdef.AnomalySeverityHigh
	case score >= mediumSeverityThreshold:
		return observerdef.AnomalySeverityMedium
	default:
		return observerdef.AnomalySeverityLow
	}
}

// scopeKey derives a stable scope string from the anomaly tags.
func scopeKey(a observerdef.Anomaly) string {
	if a.Context != nil && len(a.Context.SplitTags) > 0 {
		var parts []string
		for k, v := range a.Context.SplitTags {
			parts = append(parts, k+":"+v)
		}
		sort.Strings(parts)
		return strings.Join(parts, "|")
	}

	tags := a.Source.Tags
	service, env := "", ""
	for _, t := range tags {
		if strings.HasPrefix(t, "service:") {
			service = t
		} else if strings.HasPrefix(t, "env:") {
			env = t
		}
	}
	if service != "" && env != "" {
		return service + "|" + env
	}
	if service != "" {
		return service
	}
	if len(tags) > 0 {
		sorted := make([]string, len(tags))
		copy(sorted, tags)
		sort.Strings(sorted)
		return strings.Join(sorted, "|")
	}
	return "global"
}

// eventID returns a stable 16-character hex ID for an anomaly event.
func eventID(a observerdef.Anomaly) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%d|%s", a.Source.Key(), a.DetectorName, a.Timestamp, a.Title)
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}
