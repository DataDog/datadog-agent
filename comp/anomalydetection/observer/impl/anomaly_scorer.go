// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"container/heap"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logging"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	severityeventsimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/impl"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
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

// ---------------------------------------------------------------------------
// EWMA helpers
// ---------------------------------------------------------------------------

// levelWeights maps anomaly level (0–4) to its EWMA weight.
// Level 0=VeryLow, 1=Low, 2=Medium, 3=High, 4=XHigh.
var levelWeights = [5]float64{0.2, 0.5, 1.0, 2.0, 3.0}

var anomalySeverityLabels = [5]string{"xlow", "low", "medium", "high", "xhigh"}

func anomalySeverityLabel(level int) string {
	if level < 0 || level >= len(anomalySeverityLabels) {
		return "unknown"
	}
	return anomalySeverityLabels[level]
}

// detectorConfidenceLevel assigns a 0–4 confidence level from the detector
// score. Detectors without calibrated thresholds default to Medium.
func detectorConfidenceLevel(a observerdef.Anomaly, cfg observerdef.AnomalyScorerConfig) int {
	if thresholds, ok := cfg.DetectorThresholds[a.DetectorAlgorithm()]; ok {
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

// relativeChangeLevel assigns a 0–4 severity level from the gate-relative
// magnitude between the before and after medians. Bounds are inclusive.
func relativeChangeLevel(change float64) int {
	switch {
	case change <= 0.06:
		return 0
	case change <= 0.10:
		return 1
	case change <= 0.20:
		return 2
	case change <= 0.50:
		return 3
	default:
		return 4
	}
}

// relativeChangeMagnitude measures the gate-relative difference between two
// finite levels. It saturates at MaxFloat64 when the ratio overflows.
func relativeChangeMagnitude(before, after float64) float64 {
	if before == 0 {
		return math.MaxFloat64
	}
	change := math.Abs(after - before)
	if math.IsInf(change, 0) {
		return math.MaxFloat64
	}
	change /= math.Abs(before)
	if math.IsInf(change, 0) {
		return math.MaxFloat64
	}
	return change
}

// anomalyLevel assigns a 0–4 level to an anomaly. Stable gate evidence uses
// relative change for the base level, with one level of confidence bonus.
func anomalyLevel(a observerdef.Anomaly, cfg observerdef.AnomalyScorerConfig) int {
	if a.LevelChange == nil {
		return detectorConfidenceLevel(a, cfg)
	}
	level := relativeChangeLevel(relativeChangeMagnitude(
		a.LevelChange.BeforeMedian,
		a.LevelChange.AfterMedian,
	))
	if detectorConfidenceLevel(a, cfg) >= 3 {
		level++
	}
	return min(level, len(levelWeights)-1)
}

// contributorWeight interpolates an anomaly score between the adjacent
// calibrated levelWeights. It is used only to rank retained top anomalies; the
// scorer's EWMA continues to use the calibrated discrete levelWeights.
func contributorWeight(a observerdef.Anomaly, cfg observerdef.AnomalyScorerConfig) float64 {
	if a.LevelChange != nil {
		return levelWeights[anomalyLevel(a, cfg)]
	}

	thresholds, ok := cfg.DetectorThresholds[a.DetectorAlgorithm()]
	if !ok {
		return levelWeights[2] // uncalibrated detectors retain Medium's weight
	}
	if a.Score == nil {
		return levelWeights[0]
	}

	score := *a.Score
	if score <= thresholds[0] {
		return levelWeights[0]
	}

	lowerThreshold := thresholds[0]
	for level, upperThreshold := range thresholds {
		if score < upperThreshold {
			span := upperThreshold - lowerThreshold
			if span <= 0 {
				return levelWeights[level]
			}
			fraction := (score - lowerThreshold) / span
			return levelWeights[level] + fraction*(levelWeights[level+1]-levelWeights[level])
		}
		lowerThreshold = upperThreshold
	}
	return levelWeights[len(levelWeights)-1]
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

// scoringState carries the score and severity before subscriber cooldown.
type scoringState struct {
	ewma        float64
	level       severityeventsdef.SeverityLevel
	initialized bool
}

const (
	topAnomalyWindowSecs        = int64(5 * 60)
	contributorBufferMultiplier = 10
	minMaxReportedItems         = 10
	maxMaxReportedItems         = 1000
)

// topAnomaly is one storage-backed anomaly occurrence retained for scorer
// episode attribution. It deliberately stores no source descriptor, tags, or
// detector payload; those are resolved only for the final reported series.
type topAnomaly struct {
	handle    observerdef.QueryHandle
	timestamp int64
	weight    float64
}

// topAnomalyBuffer is a bounded approximation of the strongest anomalies over
// the previous five minutes. Entries form a min-heap, with the weakest retained
// anomaly at the root.
type topAnomalyBuffer struct {
	entries  []topAnomaly
	capacity int
}

func newTopAnomalyBuffer(displayCount int) *topAnomalyBuffer {
	capacity := displayCount * contributorBufferMultiplier
	return &topAnomalyBuffer{
		entries:  make([]topAnomaly, 0, capacity),
		capacity: capacity,
	}
}

// update expires stale entries then considers each anomaly from a finalized
// scorer second. sec is data time, never wall-clock time.
func (b *topAnomalyBuffer) update(sec int64, anomalies []observerdef.Anomaly, cfg observerdef.AnomalyScorerConfig) {
	b.expire(sec)
	for _, anomaly := range anomalies {
		b.consider(sec, anomaly, cfg)
	}
}

func (b *topAnomalyBuffer) consider(sec int64, anomaly observerdef.Anomaly, cfg observerdef.AnomalyScorerConfig) {
	if anomaly.SourceRef == nil {
		return
	}
	b.insert(topAnomaly{
		handle:    *anomaly.SourceRef,
		timestamp: sec,
		weight:    contributorWeight(anomaly, cfg),
	})
}

func (b *topAnomalyBuffer) expire(sec int64) {
	cutoff := sec - topAnomalyWindowSecs
	kept := b.entries[:0]
	for _, entry := range b.entries {
		if entry.timestamp > cutoff {
			kept = append(kept, entry)
		}
	}
	b.entries = kept
	heap.Init(b)
}

func (b *topAnomalyBuffer) insert(candidate topAnomaly) {
	if b.capacity == 0 {
		return
	}
	if len(b.entries) < b.capacity {
		heap.Push(b, candidate)
		return
	}
	if !topAnomalyBefore(candidate, b.entries[0]) {
		return
	}
	b.entries[0] = candidate
	heap.Fix(b, 0)
}

func (b *topAnomalyBuffer) Len() int {
	return len(b.entries)
}

func (b *topAnomalyBuffer) Less(i, j int) bool {
	return topAnomalyBefore(b.entries[j], b.entries[i])
}

func (b *topAnomalyBuffer) Swap(i, j int) {
	b.entries[i], b.entries[j] = b.entries[j], b.entries[i]
}

func (b *topAnomalyBuffer) Push(value any) {
	b.entries = append(b.entries, value.(topAnomaly))
}

func (b *topAnomalyBuffer) Pop() any {
	last := len(b.entries) - 1
	value := b.entries[last]
	b.entries = b.entries[:last]
	return value
}

// topAnomalyBefore defines the stable rank order for retained occurrences.
func topAnomalyBefore(a, b topAnomaly) bool {
	if a.weight != b.weight {
		return a.weight > b.weight
	}
	if a.timestamp != b.timestamp {
		return a.timestamp > b.timestamp
	}
	if a.handle.Ref != b.handle.Ref {
		return a.handle.Ref < b.handle.Ref
	}
	return a.handle.Aggregate < b.handle.Aggregate
}

func (b *topAnomalyBuffer) reset() {
	b.entries = b.entries[:0]
}

// contributors returns the highest-weight metrics represented in the retained
// anomaly occurrences. Shares are normalized across the returned top items.
func (b *topAnomalyBuffer) contributors(maxItems int) []observerdef.ScorerContributor {
	if maxItems <= 0 || len(b.entries) == 0 {
		return nil
	}

	totals := make(map[observerdef.QueryHandle]float64, len(b.entries))
	var total float64
	for _, entry := range b.entries {
		totals[entry.handle] += entry.weight
		total += entry.weight
	}
	if total == 0 {
		return nil
	}

	contributors := make([]observerdef.ScorerContributor, 0, len(totals))
	for handle, weight := range totals {
		contributors = append(contributors, observerdef.ScorerContributor{
			Handle: handle,
			Weight: weight,
		})
	}
	sort.Slice(contributors, func(i, j int) bool {
		if contributors[i].Weight != contributors[j].Weight {
			return contributors[i].Weight > contributors[j].Weight
		}
		if contributors[i].Handle.Ref != contributors[j].Handle.Ref {
			return contributors[i].Handle.Ref < contributors[j].Handle.Ref
		}
		return contributors[i].Handle.Aggregate < contributors[j].Handle.Aggregate
	})
	if len(contributors) > maxItems {
		contributors = contributors[:maxItems]
	}
	var selectedTotal float64
	for _, contributor := range contributors {
		selectedTotal += contributor.Weight
	}
	for i := range contributors {
		contributors[i].Share = contributors[i].Weight / selectedTotal
	}
	return contributors
}

// secState is the per-second scorer state emitted after each Advance step.
type secState struct {
	sec          int64
	ewma         float64
	level        severityeventsdef.SeverityLevel
	highCrossing int64
}

// ---------------------------------------------------------------------------
// AnomalyScorerConfig (impl-level)
// ---------------------------------------------------------------------------

// AnomalyScorerConfig is the impl-level config for the unified anomaly scorer.
// It embeds the def-level EWMA parameters and adds output toggles for the
// internal watcher (logs, correlation events, cooldown, episode size cap).
type AnomalyScorerConfig struct {
	observerdef.AnomalyScorerConfig
	// Logs controls whether severity transitions are logged through the anomaly-detection logger.
	Logs bool `json:"logs"`
	// CorrelationEvents controls whether scorer severity episodes are tracked
	// and returned by ActiveCorrelations() for the reporter pipeline.
	CorrelationEvents bool `json:"correlation_events"`
	// CorrelationEventThreshold is the lowest severity that opens a correlation
	// episode. Supported values are "medium" and "high"; "low" is invalid
	// because Low is the scorer's no-evidence baseline.
	CorrelationEventThreshold string `json:"correlation_event_threshold"`
	// CooldownSecs is the minimum interval between de-escalation callbacks
	// from the internal watcher subscription.
	CooldownSecs int64 `json:"cooldown_secs"`
	// MaxEpisodeAnomalies caps the number of anomalies stored per episode.
	// 0 means no cap.
	MaxEpisodeAnomalies int `json:"max_episode_anomalies"`
	// MaxReportedItems is the number of metrics shown in a scorer episode event.
	// The top-anomaly buffer retains ten times this count.
	MaxReportedItems int `json:"max_reported_items"`
}

// DefaultAnomalyScorerConfig returns calibrated defaults.
// Per-detector thresholds are set based on empirical score distributions across
// kafka-partition-saturation, postmark, and dns-upstream-outage scenarios.
func DefaultAnomalyScorerConfig() AnomalyScorerConfig {
	const window = 15
	return AnomalyScorerConfig{
		AnomalyScorerConfig: observerdef.AnomalyScorerConfig{
			Alpha:               0.014,
			SaturationK:         5.0,
			WindowSecs:          window,
			LateAnomalyBehavior: lateAnomalyClamp,
			LowThreshold:        0.15,
			HighThreshold:       0.40,
			MarginPct:           0.20,
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
		Logs:                      false,
		CorrelationEvents:         false,
		CorrelationEventThreshold: "high",
		CooldownSecs:              300,
		MaxEpisodeAnomalies:       50,
		MaxReportedItems:          16,
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
		logging.Warnf("anomaly scorer: %s must be in (0, 1), got %g — using default %g", key, v, defaults.Alpha)
		v = defaults.Alpha
	}
	ewma.Alpha = v

	key = prefix + "saturation_k"
	v = r.GetFloat64(key)
	if v <= 0 {
		logging.Warnf("anomaly scorer: %s must be > 0, got %g — using default %g", key, v, defaults.SaturationK)
		v = defaults.SaturationK
	}
	ewma.SaturationK = v

	key = prefix + "window"
	d := r.GetDuration(key)
	if d < time.Second {
		logging.Warnf("anomaly scorer: %s must be >= 1s, got %s — using default %ds", key, d, defaults.WindowSecs)
		d = time.Duration(defaults.WindowSecs) * time.Second
	}
	ewma.WindowSecs = int64(d.Seconds())

	key = prefix + "anomaly_reordering_window"
	d = r.GetDuration(key)
	if seconds := int64(d / time.Second); d%time.Second != 0 || !validReorderingWindow(seconds) {
		logging.Warnf("anomaly scorer: %s must be a whole number of seconds from 0s to 1h, got %s; using %s",
			key, d, time.Duration(defaults.ReorderingWindowSecs)*time.Second)
	} else {
		ewma.ReorderingWindowSecs = seconds
	}

	key = prefix + "late_anomaly_behavior"
	if behavior := r.GetString(key); validLateAnomalyBehavior(behavior) {
		ewma.LateAnomalyBehavior = behavior
	} else if behavior != "" {
		logging.Warnf("anomaly scorer: %s must be ignore or clamp, got %q; using %q", key, behavior, defaults.LateAnomalyBehavior)
	}

	ewma.LowThreshold = r.GetFloat64(prefix + "low_threshold")
	ewma.HighThreshold = r.GetFloat64(prefix + "high_threshold")
	ewma.MarginPct = r.GetFloat64(prefix + "margin_pct")

	outPrefix := prefix + "output."
	cfg.Logs = r.GetBool(outPrefix + "logs")
	cfg.CorrelationEvents = r.GetBool(outPrefix + "correlation_events")
	thresholdKey := outPrefix + "correlation_event_threshold"
	threshold, err := normalizeCorrelationEventThreshold(r.GetString(thresholdKey))
	if err != nil {
		logging.Warnf("anomaly scorer: %s %v; using default %q", thresholdKey, err, defaults.CorrelationEventThreshold)
	} else {
		cfg.CorrelationEventThreshold = threshold
	}
	key = outPrefix + "cooldown"
	d = r.GetDuration(key)
	if d < 0 {
		logging.Warnf("anomaly scorer: %s must be >= 0, got %s — using default %ds", key, d, defaults.CooldownSecs)
		d = time.Duration(defaults.CooldownSecs) * time.Second
	}
	cfg.CooldownSecs = int64(d.Seconds())
	cfg.MaxEpisodeAnomalies = r.GetInt(outPrefix + "max_anomalies")
	key = outPrefix + "max_reported_items"
	cfg.MaxReportedItems = r.GetInt(key)
	if cfg.MaxReportedItems < minMaxReportedItems {
		logging.Warnf("anomaly scorer: %s must be at least %d, got %d — using %d", key, minMaxReportedItems, cfg.MaxReportedItems, minMaxReportedItems)
		cfg.MaxReportedItems = minMaxReportedItems
	} else if cfg.MaxReportedItems > maxMaxReportedItems {
		logging.Warnf("anomaly scorer: %s must be at most %d, got %d — using %d", key, maxMaxReportedItems, cfg.MaxReportedItems, maxMaxReportedItems)
		cfg.MaxReportedItems = maxMaxReportedItems
	}

	return cfg
}

func normalizeCorrelationEventThreshold(value string) (string, error) {
	switch normalized := strings.ToLower(strings.TrimSpace(value)); normalized {
	case "", "high":
		return "high", nil
	case "medium":
		return "medium", nil
	case "low":
		return "", errors.New("must be \"medium\" or \"high\"; \"low\" is the baseline state")
	default:
		return "", fmt.Errorf("must be \"medium\" or \"high\", got %q", value)
	}
}

func correlationEventSeverity(threshold string) severityeventsdef.SeverityLevel {
	if threshold == "medium" {
		return severityeventsdef.SeverityMedium
	}
	return severityeventsdef.SeverityHigh
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
//  3. Internal watcher (optional): self-subscribes to the event manager; on each
//     transition it sets configured gauges, optionally logs the event, and
//     optionally tracks configured-severity episodes for ActiveCorrelations().
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
//	ActiveCorrelations → returns the open configured-threshold episode when enabled.
//	ScoreState()   → returns accumulated telemetry snapshot.
//	Reset()        → clears all internal state for reanalysis.
type anomalyScorer struct {
	mu sync.Mutex

	dispatchersMu sync.RWMutex

	config AnomalyScorerConfig

	// history is populated only for interactive replay introspection.
	history *scorerHistory

	// pending holds anomalies received since the last Advance, grouped by second.
	// Future-timestamped anomalies accumulate until Advance reaches their second.
	pending map[int64][]observerdef.Anomaly
	// late holds arrivals for already-scored seconds until the next forward Advance.
	late []observerdef.Anomaly

	// windowMap tracks the highest anomaly level seen per series within the
	// active window [lastAdvancedSec-WindowSecs+1, lastAdvancedSec].
	// Entries are evicted once lastSeenSec falls outside the window.
	windowMap map[string]windowEntry
	// topAnomalies retains bounded five-minute attribution only when
	// correlation events are enabled.
	topAnomalies *topAnomalyBuffer

	score scoringState

	replay *scorerReplayHistory

	lastAdvancedSec int64

	// buckets retains the most recent WindowSecs AnomalyScoreBucket entries for debug
	// and replay inspection via ScoreState(). Capped at WindowSecs to prevent
	// unbounded growth in long-running agents; older entries are discarded.
	buckets []observerdef.AnomalyScoreBucket

	// dispatchers fan the raw severity stream out to push-based subscribers and
	// helper adapters like SeverityReader. Each dispatcher owns its own
	// filter/cooldown state machine.
	dispatchers []*severityeventsimpl.Dispatcher

	// Internal watcher fields (gauges may be nil when replay only needs episodes).
	ewmaGauge     telemetry.Gauge // may be nil; set on every Advance tick
	severityGauge telemetry.Gauge // may be nil; set to the current severity on every Advance tick

	// Episode tracking (guarded by mu; only active when correlationEvents is true).
	// openEpisode is the currently open configured-threshold severity period.
	// Closed episodes are no longer buffered here — they are emitted as EpisodeEnded
	// CorrelatorEvents via PendingEvents() and accumulated by the engine from there.
	openEpisode *observerdef.ActiveCorrelation

	// pendingEvents holds lifecycle events (EpisodeStarted/EpisodeEnded) produced
	// during the last Advance cycle. Drained once by the engine via PendingEvents().
	pendingEvents []observerdef.CorrelatorEvent
}

type scorerHistory struct {
	inputs  []ScorerHistoryInput
	events  []severityeventsdef.SeverityEvent
	reports []ScorerHistoryReport
}

// StandaloneAnomalyScorer is the public interface for a scorer that is used
// independently of the observer engine (e.g. testbench replay path).
type StandaloneAnomalyScorer interface {
	SubscribeSeverityEvents(cfg severityeventsdef.SeverityEventsConfiguration, listener severityeventsdef.SeverityEventListener) (severityeventsdef.SeverityEventsSubscription, error)
	SubscribeSeverityEventsReader(cfg severityeventsdef.SeverityEventsConfiguration) (severityeventsdef.SeverityEventsReaderSubscription, error)
	ProcessAnomaly(a observerdef.Anomaly)
	Advance(dataTime int64)
	PendingEvents() []observerdef.CorrelatorEvent
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
	if !validReorderingWindow(cfg.ReorderingWindowSecs) {
		cfg.ReorderingWindowSecs = defaults.ReorderingWindowSecs
	}
	if !validLateAnomalyBehavior(cfg.LateAnomalyBehavior) {
		cfg.LateAnomalyBehavior = defaults.LateAnomalyBehavior
	}
	if cfg.Alpha <= 0 || cfg.Alpha >= 1 {
		cfg.Alpha = defaults.Alpha
	}
	if cfg.SaturationK <= 0 {
		cfg.SaturationK = defaults.SaturationK
	}
	if cfg.MaxReportedItems < minMaxReportedItems {
		cfg.MaxReportedItems = minMaxReportedItems
	} else if cfg.MaxReportedItems > maxMaxReportedItems {
		cfg.MaxReportedItems = maxMaxReportedItems
	}
	if threshold, err := normalizeCorrelationEventThreshold(cfg.CorrelationEventThreshold); err != nil {
		logging.Warnf("anomaly scorer: correlation_event_threshold %v; using default %q", err, defaults.CorrelationEventThreshold)
		cfg.CorrelationEventThreshold = defaults.CorrelationEventThreshold
	} else {
		cfg.CorrelationEventThreshold = threshold
	}
	scorer := &anomalyScorer{
		config:    cfg,
		pending:   make(map[int64][]observerdef.Anomaly),
		windowMap: make(map[string]windowEntry),
	}
	if cfg.CorrelationEvents {
		scorer.topAnomalies = newTopAnomalyBuffer(cfg.MaxReportedItems)
	}
	return scorer
}

// NewAnomalyScorer creates a new anomalyScorer with the given config.
// When correlation events are enabled, it also installs the internal watcher
// so standalone consumers such as the testbench can inspect episode events.
// Invalid EWMA parameter values are clamped to safe defaults.
func NewAnomalyScorer(cfg AnomalyScorerConfig) StandaloneAnomalyScorer {
	if cfg.CorrelationEvents {
		return newAnomalyScorerWithTelemetry(cfg, nil, nil)
	}
	return newAnomalyScorerBase(cfg)
}

// newAnomalyScorerWithTelemetry creates a scorer with the watcher enabled.
// Non-nil severityGauge and ewmaGauge values are written with the current
// severity and EWMA on every advance tick. The watcher self-subscribes using
// cfg.CooldownSecs.
func newAnomalyScorerWithTelemetry(cfg AnomalyScorerConfig, severityGauge, ewmaGauge telemetry.Gauge) *anomalyScorer {
	s := newAnomalyScorerBase(cfg)
	s.ewmaGauge = ewmaGauge
	s.severityGauge = severityGauge

	// Self-subscribe as the internal watcher.
	if _, err := s.SubscribeSeverityEvents(severityeventsdef.SeverityEventsConfiguration{
		CooldownSecs: cfg.CooldownSecs,
	}, s); err != nil {
		logging.Errorf("anomaly scorer self-subscription failed: %v", err)
	}

	return s
}

func (s *anomalyScorer) enableHistory() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.history == nil {
		s.history = &scorerHistory{}
	}
}

func (s *anomalyScorer) historyState() ScorerHistory {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.history == nil {
		return ScorerHistory{}
	}

	history := ScorerHistory{
		Available:    true,
		Buckets:      append([]observerdef.AnomalyScoreBucket(nil), s.buckets...),
		Events:       append([]severityeventsdef.SeverityEvent(nil), s.history.events...),
		Inputs:       append([]ScorerHistoryInput(nil), s.history.inputs...),
		Config:       s.config.AnomalyScorerConfig,
		CooldownSecs: s.config.CooldownSecs,
	}
	// Replay owns retention; keep MaxInt64 out of browser round trips.
	history.Config.MaxBuckets = 0
	history.Reports = make([]ScorerHistoryReport, len(s.history.reports))
	for i, report := range s.history.reports {
		history.Reports[i] = ScorerHistoryReport{
			Timestamp:    report.Timestamp,
			Contributors: append([]observerdef.ScorerContributor(nil), report.Contributors...),
		}
	}
	return history
}

// ---------------------------------------------------------------------------
// severityevents.SeverityEventListener — internal watcher callback
// ---------------------------------------------------------------------------

// OnSeverityTransition is called by the self-subscription on each severity
// transition. It optionally logs the event and manages episodes while severity
// is at or above the configured event threshold.
func (s *anomalyScorer) OnSeverityTransition(evt severityeventsdef.SeverityEvent) {
	direction := "escalation"
	if evt.Direction == severityeventsdef.SeverityEventDeescalation {
		direction = "deescalation"
	}

	if s.config.Logs {
		logging.Infof("anomaly scorer %s severity %s to %s (was %s, t=%d)",
			s.Name(),
			direction,
			evt.ToLevel.String(),
			evt.FromLevel.String(),
			evt.Timestamp,
		)
	}

	if s.config.CorrelationEvents || s.history != nil {
		s.mu.Lock()
		if s.history != nil {
			s.history.events = append(s.history.events, evt)
		}
		if !s.config.CorrelationEvents {
			s.mu.Unlock()
			return
		}
		threshold := correlationEventSeverity(s.config.CorrelationEventThreshold)
		if evt.FromLevel < threshold && evt.ToLevel >= threshold {
			thresholdName := s.config.CorrelationEventThreshold
			s.openEpisode = &observerdef.ActiveCorrelation{
				Pattern:     fmt.Sprintf("anomaly_scorer_%s:%d", thresholdName, evt.Timestamp),
				Title:       fmt.Sprintf("Anomaly scorer: %s-or-higher severity period", thresholdName),
				FirstSeen:   evt.Timestamp,
				LastUpdated: evt.Timestamp,
			}
			contributors := s.topAnomalies.contributors(s.config.MaxReportedItems)
			s.pendingEvents = append(s.pendingEvents, observerdef.CorrelatorEvent{
				Kind:                observerdef.CorrelatorEventEpisodeStarted,
				CorrelatorName:      s.Name(),
				Timestamp:           evt.Timestamp,
				HistoricalTimestamp: evt.HistoricalTimestamp,
				Correlation:         *s.openEpisode,
				FromLevel:           evt.FromLevel,
				ToLevel:             evt.ToLevel,
				// Live scheduling advances one data second at a time, so this
				// snapshot represents the same second as the severity transition.
				Contributors: contributors,
			})
			if s.history != nil {
				s.history.reports = append(s.history.reports, ScorerHistoryReport{
					Timestamp:    evt.Timestamp,
					Contributors: contributors,
				})
			}
		} else if evt.FromLevel >= threshold && evt.ToLevel < threshold && s.openEpisode != nil {
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

// ProcessAnomaly buffers evidence at its original or clamped scoring second.
func (s *anomalyScorer) ProcessAnomaly(a observerdef.Anomaly) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec := a.Timestamp
	if s.score.initialized && sec <= s.lastAdvancedSec {
		if s.replay != nil {
			if sec > 0 && (s.config.LateAnomalyBehavior == lateAnomalyClamp ||
				sec > s.lastAdvancedSec-s.replay.windowSecs()) {
				s.late = append(s.late, a)
			}
			return
		}
		if s.config.LateAnomalyBehavior == lateAnomalyIgnore {
			return
		}
		sec = s.lastAdvancedSec + 1
	}
	s.pending[sec] = append(s.pending[sec], a)
	s.recordAnomaly(a, sec)
}

// recordAnomaly tracks accepted evidence for history and an open episode.
func (s *anomalyScorer) recordAnomaly(a observerdef.Anomaly, scoringSecond int64) {
	if s.history != nil {
		s.history.inputs = append(s.history.inputs, ScorerHistoryInput{
			ScoringSecond: scoringSecond,
			Anomaly:       a,
		})
	}

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
	if s.score.initialized && dataTime <= s.lastAdvancedSec {
		s.mu.Unlock()
		return
	}

	start := s.lastAdvancedSec + 1
	if !s.score.initialized {
		// First Advance: start from the earliest pending anomaly (or dataTime if
		// no anomalies), to avoid emitting empty buckets for all seconds since epoch.
		start = dataTime
		for sec := range s.pending {
			if sec < start {
				start = sec
			}
		}
	}

	late, replayStart := s.takeLateInputs(dataTime)

	// Score forward without the late batch to identify ordinary High crossings.
	// From replayStart on, these provisional states must not mark High coverage.
	states := make([]secState, 0, int(dataTime-start+1))
	for sec := start; sec <= dataTime; sec++ {
		states = append(states, s.advanceSecond(sec, replayStart == 0 || sec < replayStart))
	}
	s.lastAdvancedSec = dataTime
	var missedHigh int64
	if len(late) > 0 {
		missedHigh = s.replayLateInputs(dataTime, replayStart, late, states)
	}
	s.mu.Unlock()

	// Push EWMA gauge for the last second of this advance tick.
	if s.ewmaGauge != nil && len(states) > 0 {
		last := states[len(states)-1]
		s.ewmaGauge.Set(last.ewma, s.Name())
	}
	if s.severityGauge != nil && len(states) > 0 {
		last := states[len(states)-1]
		s.severityGauge.Set(float64(last.level), s.Name())
	}

	// Drive the dispatchers outside the scorer lock so listeners can safely call
	// back into the scorer without deadlocking.
	s.dispatchersMu.RLock()
	dispatchers := append([]*severityeventsimpl.Dispatcher(nil), s.dispatchers...)
	s.dispatchersMu.RUnlock()
	for _, st := range states {
		for _, dispatcher := range dispatchers {
			switch {
			case st.sec == dataTime && missedHigh != 0:
				dispatcher.DiscoverHigh(dataTime, missedHigh)
			case st.highCrossing != 0:
				dispatcher.DiscoverHigh(st.sec, st.highCrossing)
			case st.level != severityeventsdef.SeverityHigh:
				dispatcher.Advance(st.sec, st.level)
			default:
				// A continuous High cannot open or renew a hold.
			}
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

// ActiveCorrelations returns the currently open scorer episode (if any).
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
	s.late = nil
	s.windowMap = make(map[string]windowEntry)
	if s.topAnomalies != nil {
		s.topAnomalies.reset()
	}
	s.score = scoringState{}
	s.replay = nil
	s.lastAdvancedSec = 0
	s.buckets = nil
	s.openEpisode = nil
	s.pendingEvents = nil
	if s.history != nil {
		s.history = &scorerHistory{}
	}
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
	score := s.score
	lastSec := s.lastAdvancedSec
	s.mu.Unlock()

	if score.initialized {
		dispatcher.DeliverInitial(lastSec, score.level)
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
	return s.score.ewma
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

// advanceSecond consumes pending anomalies and records attribution and debug buckets.
// Must be called with mu held.
func (s *anomalyScorer) advanceSecond(sec int64, recognizeHigh bool) secState {
	anomalies := s.pending[sec]
	delete(s.pending, sec)

	if s.topAnomalies != nil {
		s.topAnomalies.update(sec, anomalies, s.config.AnomalyScorerConfig)
	}

	cfg := s.config.AnomalyScorerConfig
	var inputs []scoringInput
	if len(anomalies) > 0 {
		inputs = make([]scoringInput, len(anomalies))
		for i, a := range anomalies {
			inputs[i] = scoringEvidence(a, cfg)
		}
	}
	if cfg.ReorderingWindowSecs > 0 && s.replay == nil {
		s.replay = newScorerReplayHistory(cfg.ReorderingWindowSecs, cfg.WindowSecs)
		s.replay.firstSecond = sec
		s.replay.saveCheckpoint(sec-1, s.score, true)
	}
	bucket, next := scoreSecond(sec, inputs, cfg, s.windowMap, s.score)
	var highCrossing int64
	if next.level == severityeventsdef.SeverityHigh && s.score.level != next.level {
		highCrossing = sec
	}
	if s.replay != nil {
		s.replay.recordSecond(sec, inputs, next, recognizeHigh)
	}
	s.score = next
	s.buckets = append(s.buckets, bucket)

	// Default cap is WindowSecs; MaxBuckets overrides this when set to a positive value.
	bucketCap := s.bucketCap()
	if int64(len(s.buckets)) > bucketCap {
		trimmed := make([]observerdef.AnomalyScoreBucket, bucketCap)
		copy(trimmed, s.buckets[int64(len(s.buckets))-bucketCap:])
		s.buckets = trimmed
	}

	return secState{sec: sec, ewma: next.ewma, level: next.level, highCrossing: highCrossing}
}

func (s *anomalyScorer) bucketCap() int64 {
	if s.config.MaxBuckets > 0 {
		return s.config.MaxBuckets
	}
	return s.config.WindowSecs
}

func mergeScoringInputs(sec int64, inputs []scoringInput, window map[string]windowEntry) {
	for _, input := range inputs {
		entry := window[input.series]
		if sec > entry[input.level] {
			entry[input.level] = sec
		}
		window[input.series] = entry
	}
}

// scoreSecond updates the evidence window in place and computes the next score
// and raw severity from the previous second.
func scoreSecond(sec int64, inputs []scoringInput, cfg observerdef.AnomalyScorerConfig, window map[string]windowEntry, previous scoringState) (observerdef.AnomalyScoreBucket, scoringState) {
	mergeScoringInputs(sec, inputs, window)

	// A newer, weaker anomaly must not extend the lifetime of an older peak.
	windowStart := sec - cfg.WindowSecs + 1
	for sid, entry := range window {
		alive := false
		for level := 0; level < 5; level++ {
			if entry[level] > 0 && entry[level] < windowStart {
				entry[level] = 0
			}
			if entry[level] > 0 {
				alive = true
			}
		}
		if !alive {
			delete(window, sid)
			continue
		}
		window[sid] = entry
	}

	// Each series contributes at the highest level that still has an active timestamp.
	var bins [5]int
	var count int
	var weightSum float64
	for _, entry := range window {
		for level := 4; level >= 0; level-- {
			if entry[level] == 0 {
				continue
			}
			bins[level]++
			count++
			weightSum += levelWeights[level]
			break
		}
	}

	var input float64
	if count > 0 {
		meanWeight := weightSum / float64(count)
		input = meanWeight * (1 - math.Exp(-float64(count)/cfg.SaturationK))
	}

	ewma := cfg.Alpha*input + (1-cfg.Alpha)*previous.ewma
	var level severityeventsdef.SeverityLevel
	if previous.initialized {
		margin := cfg.HighThreshold * cfg.MarginPct
		level = nextSeverityLevel(ewma, previous.level, cfg.LowThreshold, cfg.HighThreshold, margin)
	} else {
		level = rawSeverityLevel(ewma, cfg.LowThreshold, cfg.HighThreshold)
	}

	return observerdef.AnomalyScoreBucket{
		Second:    sec,
		Bins:      bins,
		Count:     count,
		WeightSum: weightSum,
		Ewma:      ewma,
	}, scoringState{ewma: ewma, level: level, initialized: true}
}

const (
	maxAnomalyReorderingWindowSecs = 60 * 60
	lateAnomalyIgnore              = "ignore"
	lateAnomalyClamp               = "clamp"
)

func validLateAnomalyBehavior(behavior string) bool {
	return behavior == lateAnomalyIgnore || behavior == lateAnomalyClamp
}

func validReorderingWindow(seconds int64) bool {
	return seconds >= 0 && seconds <= maxAnomalyReorderingWindowSecs
}

// scoringInput is the evidence needed to score an anomaly. It retains no payload or attribution.
type scoringInput struct {
	series string
	level  int
}

func scoringEvidence(a observerdef.Anomaly, cfg observerdef.AnomalyScorerConfig) scoringInput {
	return scoringInput{series: seriesID(a), level: anomalyLevel(a, cfg)}
}

type scoringCheckpoint struct {
	second  int64
	state   scoringState
	present bool
	// Keep recognized coverage even if a later replay lowers the raw level.
	recognizedHigh bool
}

// scorerReplayHistory retains scoring state independently of debug buckets.
// At T, the oldest replayable second is T-window+1. Its seed is the
// checkpoint at T-window, and it needs evidence back to T-window-evidence+2.
type scorerReplayHistory struct {
	checkpoints    []scoringCheckpoint
	journal        map[int64][]scoringInput
	evidenceWindow int64
	firstSecond    int64
}

func newScorerReplayHistory(window, evidence int64) *scorerReplayHistory {
	return &scorerReplayHistory{
		checkpoints:    make([]scoringCheckpoint, window+1),
		evidenceWindow: evidence,
	}
}

func replaySlot(second int64, size int) int {
	slot := int(second % int64(size))
	if slot < 0 {
		slot += size
	}
	return slot
}

func (h *scorerReplayHistory) saveCheckpoint(second int64, state scoringState, recognizeHigh bool) {
	slot := &h.checkpoints[replaySlot(second, len(h.checkpoints))]
	if !slot.present || slot.second != second {
		*slot = scoringCheckpoint{second: second, present: true}
	}
	slot.state = state
	slot.recognizedHigh = slot.recognizedHigh || (recognizeHigh && state.level == severityeventsdef.SeverityHigh)
}

func (h *scorerReplayHistory) recognizedHighAt(second int64) bool {
	slot := h.checkpoints[replaySlot(second, len(h.checkpoints))]
	return slot.present && slot.second == second && slot.recognizedHigh
}

func (h *scorerReplayHistory) windowSecs() int64 {
	return int64(len(h.checkpoints) - 1)
}

func (h *scorerReplayHistory) ensureJournal() {
	if h.journal == nil {
		h.journal = make(map[int64][]scoringInput)
	}
}

func (h *scorerReplayHistory) recordSecond(second int64, inputs []scoringInput, state scoringState, recognizeHigh bool) {
	h.saveCheckpoint(second, state, recognizeHigh)
	if second <= 0 {
		return // Nonpositive timestamps cannot enter the evidence window.
	}
	if len(inputs) > 0 {
		h.ensureJournal()
		h.journal[second] = inputs
	}

	// Forward scoring visits every second, including across catch-up advances.
	cutoff := second - h.windowSecs()
	if cutoff >= h.evidenceWindow {
		delete(h.journal, cutoff-h.evidenceWindow+1)
	}
}

// takeLateInputs drains the arrivals and finds the first second they can change.
func (s *anomalyScorer) takeLateInputs(dataTime int64) ([]observerdef.Anomaly, int64) {
	if len(s.late) == 0 {
		return nil, 0
	}
	h := s.replay
	oldest := dataTime - h.windowSecs() + 1
	var start int64
	accepted := s.late[:0]
	for _, a := range s.late {
		if a.Timestamp < oldest && s.config.LateAnomalyBehavior != lateAnomalyClamp {
			continue
		}
		if sec := max(a.Timestamp, oldest); start == 0 || sec < start {
			start = sec
		}
		accepted = append(accepted, a)
	}
	s.late = nil
	return accepted, start
}

func (s *anomalyScorer) replayLateInputs(dataTime, start int64, late []observerdef.Anomaly, forward []secState) int64 {
	h := s.replay
	oldest := dataTime - h.windowSecs() + 1
	cfg := s.config.AnomalyScorerConfig
	h.ensureJournal()
	for _, a := range late {
		sec := max(a.Timestamp, oldest)
		h.journal[sec] = append(h.journal[sec], scoringEvidence(a, cfg))
		s.recordAnomaly(a, sec)
		// Forward scoring already expired the contributor buffer through dataTime.
		if s.topAnomalies != nil && sec > dataTime-topAnomalyWindowSecs {
			s.topAnomalies.consider(sec, a, cfg)
		}
	}
	return s.replayScoredSeconds(start, forward)
}

// replayScoredSeconds restores the scoring state without replaying side effects.
// A High without recognized coverage or an ordinary entry is discovered now.
func (s *anomalyScorer) replayScoredSeconds(start int64, forward []secState) int64 {
	h := s.replay
	cfg := s.config.AnomalyScorerConfig
	var previous scoringState
	if start <= h.firstSecond {
		// A late input can precede the first advance. There is no earlier score.
		h.firstSecond = start
		h.saveCheckpoint(start-1, previous, true)
	} else {
		// Admission guarantees that start-1 is still in the checkpoint ring.
		previous = h.checkpoints[replaySlot(start-1, len(h.checkpoints))].state
	}
	clear(s.windowMap)
	for sec := max(h.firstSecond, start-cfg.WindowSecs+1); sec < start; sec++ {
		mergeScoringInputs(sec, h.journal[sec], s.windowMap)
	}

	debugStart := s.buckets[0].Second
	if from := max(start, s.lastAdvancedSec-s.bucketCap()+1); from < debugStart {
		// Startup backfill can add seconds before the first stored bucket.
		s.buckets = append(make([]observerdef.AnomalyScoreBucket, debugStart-from), s.buckets...)
		debugStart = from
	}
	var firstMissed, highStart int64
	knownHigh := previous.level == severityeventsdef.SeverityHigh
	forwardStart := forward[0].sec
	for sec := start; sec <= s.lastAdvancedSec; sec++ {
		bucket, next := scoreSecond(sec, h.journal[sec], cfg, s.windowMap, previous)
		var ordinaryEntry bool
		if sec >= forwardStart {
			st := &forward[sec-forwardStart]
			ordinaryEntry = st.highCrossing != 0
			*st = secState{sec: sec, level: next.level, ewma: next.ewma}
		}
		if next.level == severityeventsdef.SeverityHigh {
			if previous.level != next.level {
				highStart = sec
				knownHigh = false
			}
			knownHigh = knownHigh || h.recognizedHighAt(sec)
			// Only retain an ordinary forward entry if it still falls inside a
			// corrected High period that began in the forward part of this advance.
			if !knownHigh && highStart >= forwardStart && ordinaryEntry {
				forward[sec-forwardStart].highCrossing = highStart
				knownHigh = true
			}
		} else if highStart != 0 {
			if firstMissed == 0 && !knownHigh {
				firstMissed = highStart
			}
			highStart = 0
		}
		h.saveCheckpoint(sec, next, true)
		if sec >= debugStart {
			s.buckets[sec-debugStart] = bucket
		}
		previous = next
	}
	s.score = previous
	if firstMissed == 0 && !knownHigh {
		return highStart
	}
	return firstMissed
}
