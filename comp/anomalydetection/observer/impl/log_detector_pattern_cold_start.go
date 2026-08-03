// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"sort"
	"strings"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

const LogPatternColdStartDetectorName = "log_pattern_cold_start"

// LogPatternColdStartConfig controls one-shot detection for a previously
// unseen error-log pattern. Durations are expressed in seconds so the same
// configuration can be used by the live Agent and deterministic replay.
type LogPatternColdStartConfig struct {
	HealthyHistorySeconds     int64 `json:"healthy_history_seconds,omitempty"`
	MinOccurrences            int   `json:"min_occurrences,omitempty"`
	OccurrenceWindowSeconds   int64 `json:"occurrence_window_seconds,omitempty"`
	SourceHealthMaxGapSeconds int64 `json:"source_health_max_gap_seconds,omitempty"`
	PatternTimeToLiveSeconds  int64 `json:"pattern_time_to_live_seconds,omitempty"`
	MaxPatternsPerSource      int   `json:"max_patterns_per_source,omitempty"`
	MaxSources                int   `json:"max_sources,omitempty"`
}

// DefaultLogPatternColdStartConfig returns the settings validated by the
// Observer scenario experiment: five occurrences within thirty seconds after
// five minutes of continuous source health.
func DefaultLogPatternColdStartConfig() LogPatternColdStartConfig {
	return LogPatternColdStartConfig{
		HealthyHistorySeconds:     5 * 60,
		MinOccurrences:            5,
		OccurrenceWindowSeconds:   30,
		SourceHealthMaxGapSeconds: 15,
		PatternTimeToLiveSeconds:  4 * 60 * 60,
		MaxPatternsPerSource:      1024,
		MaxSources:                256,
	}
}

type logPatternObservationConsumer interface {
	ObserveLogPattern(observerdef.LogPatternObservation)
}

type logSourceHealthConsumer interface {
	ObserveLogSourceHealth(observerdef.LogSourceHealthObservation)
}

type logPatternObservationProducer interface {
	SetPatternObservationEnabled(bool)
}

type coldStartPatternKey struct {
	extractor  string
	metricName string
}

type coldStartCandidateKey struct {
	sourceID string
	pattern  coldStartPatternKey
}

type coldStartPatternState struct {
	firstSeen   int64
	lastSeen    int64
	occurrences []int64
	pattern     string
	example     string
	tags        []string
	closed      bool
}

type coldStartSourceState struct {
	healthy          bool
	healthySince     int64
	lastHealthSample int64
	lastSeen         int64
	patterns         map[coldStartPatternKey]*coldStartPatternState
}

// LogPatternColdStartDetector detects the onset of a new error-log pattern
// before its normal count series is warm enough for a statistical detector.
// It consumes extractor pattern observations directly and never backfills
// synthetic points into storage.
type LogPatternColdStartDetector struct {
	config  LogPatternColdStartConfig
	sources map[string]*coldStartSourceState
	pending map[coldStartCandidateKey]*coldStartPatternState
}

var _ observerdef.Detector = (*LogPatternColdStartDetector)(nil)
var _ observerdef.SeriesRemover = (*LogPatternColdStartDetector)(nil)
var _ logPatternObservationConsumer = (*LogPatternColdStartDetector)(nil)
var _ logSourceHealthConsumer = (*LogPatternColdStartDetector)(nil)

func NewLogPatternColdStartDetector(cfg LogPatternColdStartConfig) *LogPatternColdStartDetector {
	defaults := DefaultLogPatternColdStartConfig()
	if cfg.HealthyHistorySeconds <= 0 {
		cfg.HealthyHistorySeconds = defaults.HealthyHistorySeconds
	}
	if cfg.MinOccurrences <= 0 {
		cfg.MinOccurrences = defaults.MinOccurrences
	}
	if cfg.OccurrenceWindowSeconds <= 0 {
		cfg.OccurrenceWindowSeconds = defaults.OccurrenceWindowSeconds
	}
	if cfg.SourceHealthMaxGapSeconds <= 0 {
		cfg.SourceHealthMaxGapSeconds = defaults.SourceHealthMaxGapSeconds
	}
	if cfg.PatternTimeToLiveSeconds <= 0 {
		cfg.PatternTimeToLiveSeconds = defaults.PatternTimeToLiveSeconds
	}
	if cfg.MaxPatternsPerSource == 0 {
		cfg.MaxPatternsPerSource = defaults.MaxPatternsPerSource
	}
	if cfg.MaxSources == 0 {
		cfg.MaxSources = defaults.MaxSources
	}
	return &LogPatternColdStartDetector{
		config:  cfg,
		sources: make(map[string]*coldStartSourceState),
		pending: make(map[coldStartCandidateKey]*coldStartPatternState),
	}
}

func (*LogPatternColdStartDetector) Name() string { return LogPatternColdStartDetectorName }

// ObserveLogSourceHealth records explicit tailer-health evidence. A gap larger
// than the configured allowance starts a new healthy interval even if both
// samples say healthy; missing samples are never assumed healthy.
func (d *LogPatternColdStartDetector) ObserveLogSourceHealth(observation observerdef.LogSourceHealthObservation) {
	if observation.SourceID == "" || observation.Timestamp < 0 {
		return
	}
	state := d.ensureSource(observation.SourceID, observation.Timestamp)
	if observation.Timestamp < state.lastHealthSample {
		return
	}
	if observation.Healthy {
		if !state.healthy || observation.Timestamp-state.lastHealthSample > d.config.SourceHealthMaxGapSeconds {
			state.healthySince = observation.Timestamp
		}
		state.healthy = true
	} else {
		state.healthy = false
		state.healthySince = 0
	}
	state.lastHealthSample = observation.Timestamp
	state.lastSeen = max(state.lastSeen, observation.Timestamp)
}

// ObserveLogPattern records only error patterns. The first observation starts
// a bounded decision window; later occurrences cannot turn an old pattern into
// a new one.
func (d *LogPatternColdStartDetector) ObserveLogPattern(observation observerdef.LogPatternObservation) {
	if !observation.IsError || observation.SourceID == "" || observation.Extractor == "" || observation.MetricName == "" || observation.Timestamp < 0 {
		return
	}
	state := d.ensureSource(observation.SourceID, observation.Timestamp)
	state.lastSeen = max(state.lastSeen, observation.Timestamp)
	key := coldStartPatternKey{extractor: observation.Extractor, metricName: observation.MetricName}
	pattern := state.patterns[key]
	if pattern == nil {
		d.evictPatternIfFull(observation.SourceID, state)
		pattern = &coldStartPatternState{
			firstSeen: observation.Timestamp,
			lastSeen:  observation.Timestamp,
			pattern:   observation.Pattern,
			example:   observation.Example,
			tags:      canonicalizeTags(observation.Tags),
		}
		state.patterns[key] = pattern
		d.pending[coldStartCandidateKey{sourceID: observation.SourceID, pattern: key}] = pattern
	}
	if pattern.closed {
		return
	}
	pattern.lastSeen = max(pattern.lastSeen, observation.Timestamp)
	insertColdStartTimestamp(&pattern.occurrences, observation.Timestamp)
}

// Detect evaluates completed onset windows. Storage is intentionally unused:
// this detector operates on pre-threshold pattern observations and explicit
// source-health evidence.
func (d *LogPatternColdStartDetector) Detect(_ observerdef.StorageReader, dataTime int64) observerdef.DetectionResult {
	if dataTime < 0 {
		return observerdef.DetectionResult{}
	}
	keys := make([]coldStartCandidateKey, 0, len(d.pending))
	for key := range d.pending {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].sourceID != keys[j].sourceID {
			return keys[i].sourceID < keys[j].sourceID
		}
		if keys[i].pattern.extractor != keys[j].pattern.extractor {
			return keys[i].pattern.extractor < keys[j].pattern.extractor
		}
		return keys[i].pattern.metricName < keys[j].pattern.metricName
	})

	var anomalies []observerdef.Anomaly
	for _, key := range keys {
		pattern := d.pending[key]
		deadline := pattern.firstSeen + d.config.OccurrenceWindowSeconds
		occurrences := coldStartTimestampsAtOrBefore(pattern.occurrences, min(dataTime, deadline))
		if len(occurrences) >= d.config.MinOccurrences {
			triggerAt := occurrences[d.config.MinOccurrences-1]
			if d.sourceHealthyBefore(key.sourceID, pattern.firstSeen) {
				anomalies = append(anomalies, d.newAnomaly(key.sourceID, key.pattern, pattern, triggerAt))
			}
			pattern.closed = true
			pattern.occurrences = nil
			delete(d.pending, key)
			continue
		}
		if dataTime >= deadline {
			pattern.closed = true
			pattern.occurrences = nil
			delete(d.pending, key)
		}
	}
	d.garbageCollect(dataTime)
	return observerdef.DetectionResult{Anomalies: anomalies}
}

func (d *LogPatternColdStartDetector) Reset() {
	clear(d.sources)
	clear(d.pending)
}

// RemoveSeries is intentionally a no-op. Cold-start state is keyed by bounded
// source/pattern identities, not storage SeriesRefs, and is reclaimed by its
// own per-source caps and TTL.
func (*LogPatternColdStartDetector) RemoveSeries(_ []observerdef.SeriesRef) {}

func (d *LogPatternColdStartDetector) ensureSource(sourceID string, timestamp int64) *coldStartSourceState {
	if source := d.sources[sourceID]; source != nil {
		return source
	}
	if d.config.MaxSources > 0 && len(d.sources) >= d.config.MaxSources {
		victimID := ""
		var victim *coldStartSourceState
		for id, source := range d.sources {
			if victim == nil || source.lastSeen < victim.lastSeen || (source.lastSeen == victim.lastSeen && id < victimID) {
				victimID, victim = id, source
			}
		}
		for key := range victim.patterns {
			delete(d.pending, coldStartCandidateKey{sourceID: victimID, pattern: key})
		}
		delete(d.sources, victimID)
	}
	state := &coldStartSourceState{
		lastSeen: timestamp,
		patterns: make(map[coldStartPatternKey]*coldStartPatternState),
	}
	d.sources[sourceID] = state
	return state
}

func (d *LogPatternColdStartDetector) evictPatternIfFull(sourceID string, source *coldStartSourceState) {
	if d.config.MaxPatternsPerSource <= 0 || len(source.patterns) < d.config.MaxPatternsPerSource {
		return
	}
	var victimKey coldStartPatternKey
	var victim *coldStartPatternState
	for key, pattern := range source.patterns {
		if victim == nil || pattern.lastSeen < victim.lastSeen ||
			(pattern.lastSeen == victim.lastSeen && (key.extractor < victimKey.extractor ||
				(key.extractor == victimKey.extractor && key.metricName < victimKey.metricName))) {
			victimKey, victim = key, pattern
		}
	}
	delete(d.pending, coldStartCandidateKey{sourceID: sourceID, pattern: victimKey})
	delete(source.patterns, victimKey)
}

func (d *LogPatternColdStartDetector) sourceHealthyBefore(sourceID string, timestamp int64) bool {
	state := d.sources[sourceID]
	if state == nil || !state.healthy {
		return false
	}
	if state.healthySince > timestamp-d.config.HealthyHistorySeconds {
		return false
	}
	return state.lastHealthSample >= timestamp-d.config.SourceHealthMaxGapSeconds
}

func (d *LogPatternColdStartDetector) garbageCollect(dataTime int64) {
	cutoff := dataTime - d.config.PatternTimeToLiveSeconds
	for sourceID, source := range d.sources {
		for key, pattern := range source.patterns {
			if pattern.lastSeen < cutoff {
				delete(d.pending, coldStartCandidateKey{sourceID: sourceID, pattern: key})
				delete(source.patterns, key)
			}
		}
		if !source.healthy && len(source.patterns) == 0 && source.lastSeen < cutoff {
			delete(d.sources, sourceID)
		}
	}
}

func (d *LogPatternColdStartDetector) newAnomaly(sourceID string, key coldStartPatternKey, pattern *coldStartPatternState, triggerAt int64) observerdef.Anomaly {
	source := observerdef.SeriesDescriptor{
		Namespace: key.extractor,
		Name:      key.metricName,
		Tags:      copyTags(pattern.tags),
		Aggregate: observerdef.AggregateAverage,
	}
	return observerdef.Anomaly{
		Type:         observerdef.AnomalyTypeLog,
		Source:       source,
		DetectorName: d.Name(),
		Title:        "New error log pattern onset: " + source.String(),
		Description: fmt.Sprintf(
			"source %q was continuously healthy for %ds before a previously unseen error pattern appeared %d times within %ds",
			sourceID,
			d.config.HealthyHistorySeconds,
			d.config.MinOccurrences,
			d.config.OccurrenceWindowSeconds,
		),
		Context: &observerdef.MetricContext{
			Pattern: pattern.pattern,
			Example: pattern.example,
			Source:  key.extractor,
		},
		Timestamp: triggerAt,
	}
}

func insertColdStartTimestamp(timestamps *[]int64, timestamp int64) {
	values := *timestamps
	if len(values) == 0 || values[len(values)-1] <= timestamp {
		*timestamps = append(values, timestamp)
		return
	}
	index := sort.Search(len(values), func(i int) bool { return values[i] > timestamp })
	values = append(values, 0)
	copy(values[index+1:], values[index:])
	values[index] = timestamp
	*timestamps = values
}

func coldStartTimestampsAtOrBefore(timestamps []int64, end int64) []int64 {
	index := sort.Search(len(timestamps), func(i int) bool { return timestamps[i] > end })
	return timestamps[:index]
}

func isErrorLogStatus(status string) bool {
	switch strings.ToLower(status) {
	case "emergency", "alert", "critical", "error":
		return true
	default:
		return false
	}
}

func logSourceIDForView(log observerdef.LogView, fallback string) string {
	if identified, ok := log.(observerdef.LogSourceIDView); ok {
		if sourceID := identified.GetLogSourceID(); sourceID != "" {
			return sourceID
		}
	}
	return fallback
}
