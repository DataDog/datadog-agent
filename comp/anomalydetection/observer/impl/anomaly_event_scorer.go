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
	//   N=1  → ~0.57   (low-ish)
	//   N=3  → ~0.72   (medium)
	//   N=5  → ~0.81   (enters high)
	//   N=10 → 0.95    (ref point — strong high)
	//   N>10 → clamped to 0.95
	logCapBase = 0.45 // floor cap (single weak signal)
	logCapMax  = 0.95 // ceiling
	logCapRef  = 10.0 // anomaly count at which cap reaches logCapMax

	// mediumSeverityThreshold is the minimum score for medium severity.
	mediumSeverityThreshold = 0.40
	// highSeverityThreshold is the minimum score for high severity.
	highSeverityThreshold = 0.80

	// defaultEventWindowSeconds is the sliding window width in seconds.
	defaultEventWindowSeconds = 1 * 60
	// defaultEventWindowMaxItems is the maximum number of anomalies in the window.
	defaultEventWindowMaxItems = 100
)

// anomalyEventScorerConfig holds construction parameters.
type anomalyEventScorerConfig struct {
	windowSeconds int64
	maxItems      int
}

// anomalyEventScorer scores every new detector anomaly and emits one AnomalyEvent candidate.
// It keeps a bounded sliding window of recent anomalies and computes a contextual event score
// using a noisy-OR combination across distinct signals.
//
// Not goroutine-safe — must be driven from the single engine goroutine.
type anomalyEventScorer struct {
	cfg anomalyEventScorerConfig

	// window holds the current sliding window of anomalies (newest last).
	window []observerdef.Anomaly

	// previousSeverity maps scope key -> last severity seen for that scope.
	previousSeverity map[string]observerdef.AnomalySeverity

	// events holds all AnomalyEvents emitted so far.
	events []observerdef.AnomalyEvent
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
	return &anomalyEventScorer{
		cfg:              cfg,
		previousSeverity: make(map[string]observerdef.AnomalySeverity),
	}
}

// ProcessAnomaly adds the anomaly to the sliding window, scores the event, and returns the event.
func (s *anomalyEventScorer) ProcessAnomaly(a observerdef.Anomaly) observerdef.AnomalyEvent {
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

	// Stable signal key order for determinism.
	signalKeys := make([]string, 0, len(bySignal))
	for k := range bySignal {
		signalKeys = append(signalKeys, k)
	}
	sort.Strings(signalKeys)

	// 5. Compute per-signal evidence using noisy-OR: signal_score = 1 - prod(1 - a_score_i).
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
			Severity:  severityFromScore(signalScore),
			Anomalies: anomalies,
		})
	}

	// 6. Combine all signals using noisy-OR: event_score = 1 - prod(1 - signal_score_j).
	// All distinct signals contribute; the log-count cap (step 7) prevents saturation.
	sort.Slice(signals, func(i, j int) bool { return signals[i].Score > signals[j].Score })
	effectiveSignalCount := len(signals)

	eventComplement := 1.0
	for _, sig := range signals {
		eventComplement *= (1.0 - sig.Score)
	}
	rawEventScore := 1.0 - eventComplement

	// 7. Apply a logarithmic count cap so that more anomalies yield a higher score,
	// but with diminishing returns.  The cap grows with total window anomaly count N:
	//   cap(N) = logCapBase + (logCapMax - logCapBase) * log(1+N) / log(1+logCapRef)
	// This means 10 anomalies in the window always scores higher than 5 anomalies.
	windowCount := float64(len(s.window))
	logCountCap := logCapBase + (logCapMax-logCapBase)*math.Log1p(windowCount)/math.Log1p(logCapRef)
	if logCountCap > logCapMax {
		logCountCap = logCapMax
	}
	logCapped := false
	eventScore := rawEventScore
	if eventScore > logCountCap {
		eventScore = logCountCap
		logCapped = true
	}
	// Clamp to [0,1] for safety (noisy-OR is always in range, but float math can drift).
	eventScore = math.Max(0, math.Min(1, eventScore))

	// 8. Convert final score to severity.
	severity := severityFromScore(eventScore)

	// 9. Compare against previous severity for the same scope.
	scope := scopeKey(a)
	prevSeverity := s.previousSeverity[scope]
	severityChanged := prevSeverity != "" && prevSeverity != severity
	var severityDirection string
	switch {
	case prevSeverity == "":
		severityDirection = "same"
	case prevSeverity == severity:
		severityDirection = "same"
	case severityRank(severity) > severityRank(prevSeverity):
		severityDirection = "up"
		severityChanged = true
	default:
		severityDirection = "down"
		severityChanged = true
	}
	s.previousSeverity[scope] = severity

	// 10. Emit the AnomalyEvent.
	windowCopy := make([]observerdef.Anomaly, len(s.window))
	copy(windowCopy, s.window)

	evt := observerdef.AnomalyEvent{
		ID:                eventID(a),
		Trigger:           a,
		WindowStart:       windowStart,
		WindowEnd:         windowEnd,
		RecentAnomalies:   windowCopy,
		Signals:           signals, // all signals (sorted by score desc), for display
		Score:             eventScore,
		Severity:          severity,
		PreviousSeverity:  prevSeverity,
		SeverityChanged:   severityChanged,
		SeverityDirection: severityDirection,
		Breakdown: observerdef.CorrelationScoreBreakdown{
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

// Events returns all events emitted so far.
func (s *anomalyEventScorer) Events() []observerdef.AnomalyEvent {
	result := make([]observerdef.AnomalyEvent, len(s.events))
	copy(result, s.events)
	return result
}

// Reset clears all state.
func (s *anomalyEventScorer) Reset() {
	s.window = nil
	s.previousSeverity = make(map[string]observerdef.AnomalySeverity)
	s.events = nil
}

// severityFromScore maps a score in [0,1] to an AnomalySeverity.
func severityFromScore(score float64) observerdef.AnomalySeverity {
	switch {
	case score >= highSeverityThreshold:
		return observerdef.AnomalySeverityHigh
	case score >= mediumSeverityThreshold:
		return observerdef.AnomalySeverityMedium
	default:
		return observerdef.AnomalySeverityLow
	}
}

// severityRank returns a numeric rank for severity comparisons.
func severityRank(s observerdef.AnomalySeverity) int {
	switch s {
	case observerdef.AnomalySeverityHigh:
		return 2
	case observerdef.AnomalySeverityMedium:
		return 1
	default:
		return 0
	}
}

// scopeKey derives a scope string from the anomaly tags (service+env > service > source tags > global).
func scopeKey(a observerdef.Anomaly) string {
	tags := a.Source.Tags
	if a.Context != nil && len(a.Context.SplitTags) > 0 {
		// Prefer context split tags (service, env, host, source).
		var parts []string
		for k, v := range a.Context.SplitTags {
			parts = append(parts, k+":"+v)
		}
		sort.Strings(parts)
		return strings.Join(parts, "|")
	}

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

// eventID returns a stable ID for an anomaly event.
func eventID(a observerdef.Anomaly) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%d|%s", a.Source.Key(), a.DetectorName, a.Timestamp, a.Title)
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}
