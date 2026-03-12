// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sync"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// Note: stateView is defined in stateview.go and provides read-only access
// to engine state for consumers like the testbench UI.

// anomalyDedupKey is a map key for O(1) anomaly deduplication.
type anomalyDedupKey struct {
	seriesID     observerdef.SeriesID
	detectorName string
	timestamp    int64
}

// engine is the shared orchestration core for the observer pipeline.
// It encapsulates storage, log extraction, detection, and correlation,
// providing a single execution path used by both the live observer and testbench.
//
// The engine does not own reporters or scheduling policy. It accepts explicit
// Advance calls and returns results that callers route to their own outputs.
type engine struct {
	storage     *timeSeriesStorage
	extractors  []observerdef.LogMetricsExtractor
	detectors   []observerdef.Detector
	correlators []observerdef.Correlator

	// scheduler decides when the engine should advance analysis.
	scheduler schedulerPolicy

	// logObservers are detectors that also implement LogObserver.
	// Cached at construction time to avoid repeated type assertions.
	logObservers []observerdef.LogObserver

	// lastAnalyzedDataTime is the data timestamp up to which detection has run.
	lastAnalyzedDataTime int64

	// latestDataTime is the latest data timestamp seen across all ingested observations.
	latestDataTime int64

	// Raw anomaly tracking (for telemetry and testbench display).
	rawAnomalies         []observerdef.Anomaly
	rawAnomalyIndex      map[anomalyDedupKey]int // O(1) dedup lookup
	rawAnomalyMu         sync.RWMutex
	rawAnomalyWindow     int64                           // seconds to keep (0 = unlimited)
	maxRawAnomalies      int                             // max count to keep (0 = unlimited)
	currentDataTime      int64                           // latest anomaly timestamp seen
	totalAnomalyCount    int                             // total count ever (no cap)
	uniqueAnomalySources map[observerdef.MetricName]bool // unique sources that had anomalies

	// Accumulated correlations from all advance cycles.
	// Correlators maintain sliding windows that evict old state, but for
	// testbench/replay we want the full history. This map accumulates
	// every correlation ever seen, keyed by Pattern string, updating
	// existing entries when the correlator reports a newer version.
	accumulatedCorrelations map[string]observerdef.ActiveCorrelation
	correlationMu           sync.RWMutex

	// Accumulated telemetry from detection runs (for StateView access).
	accumulatedTelemetry []observerdef.ObserverTelemetry
	telemetryMu          sync.RWMutex

	// Event subscription management.
	sinks   []eventSink
	sinksMu sync.RWMutex
}

// engineConfig holds the parameters for constructing an engine.
type engineConfig struct {
	storage     *timeSeriesStorage
	extractors  []observerdef.LogMetricsExtractor
	detectors   []observerdef.Detector
	correlators []observerdef.Correlator

	// scheduler is the scheduling policy. If nil, defaults to currentBehaviorPolicy.
	scheduler schedulerPolicy

	rawAnomalyWindow int64
	maxRawAnomalies  int
}

// newEngine creates an engine with the given configuration.
func newEngine(cfg engineConfig) *engine {
	sched := cfg.scheduler
	if sched == nil {
		sched = &currentBehaviorPolicy{}
	}

	e := &engine{
		storage:     cfg.storage,
		extractors:  cfg.extractors,
		detectors:   cfg.detectors,
		correlators: cfg.correlators,
		scheduler:   sched,

		rawAnomalyWindow: cfg.rawAnomalyWindow,
		maxRawAnomalies:  cfg.maxRawAnomalies,
	}

	// Cache log observers from detectors.
	for _, d := range e.detectors {
		if lo, ok := d.(observerdef.LogObserver); ok {
			e.logObservers = append(e.logObservers, lo)
		}
	}

	return e
}

// Subscribe registers an event sink to receive engine events.
// Returns an unsubscribe function that removes the sink.
func (e *engine) Subscribe(sink eventSink) func() {
	e.sinksMu.Lock()
	e.sinks = append(e.sinks, sink)
	// Capture the sink pointer for removal.
	registered := sink
	e.sinksMu.Unlock()

	return func() {
		e.sinksMu.Lock()
		defer e.sinksMu.Unlock()
		for i, s := range e.sinks {
			if s == registered {
				e.sinks = append(e.sinks[:i], e.sinks[i+1:]...)
				return
			}
		}
	}
}

// emit sends an event to all registered sinks.
func (e *engine) emit(evt engineEvent) {
	e.sinksMu.RLock()
	sinks := make([]eventSink, len(e.sinks))
	copy(sinks, e.sinks)
	e.sinksMu.RUnlock()

	for _, sink := range sinks {
		sink.onEngineEvent(evt)
	}
}

// IngestMetric stores a metric observation and consults the scheduler policy
// to determine whether detectors should advance. Returns advance requests
// that the caller should execute via Advance.
func (e *engine) IngestMetric(source string, m *metricObs) []advanceRequest {
	e.storage.Add(source, m.name, m.value, m.timestamp, m.tags)
	e.trackLatestDataTime(m.timestamp)
	return e.scheduler.onObservation(m.timestamp, e.schedulerState())
}

// IngestLog processes a log observation: runs extractors to produce virtual metrics,
// notifies log observers, and consults the scheduler policy to determine whether
// detectors should advance. Returns advance requests that the caller should execute.
func (e *engine) IngestLog(source string, l *logObs) []advanceRequest {
	view := &logView{obs: l}
	for _, extractor := range e.extractors {
		metrics := extractor.ProcessLog(view)
		for _, m := range metrics {
			e.storage.Add(source, "_virtual."+m.Name, m.Value, l.timestampMs/1000, m.Tags)
		}
	}
	for _, lo := range e.logObservers {
		lo.ProcessLog(view)
	}
	dataTimeSec := l.timestampMs / 1000
	e.trackLatestDataTime(dataTimeSec)
	return e.scheduler.onObservation(dataTimeSec, e.schedulerState())
}

// trackLatestDataTime updates latestDataTime if the given timestamp is newer.
func (e *engine) trackLatestDataTime(dataTimeSec int64) {
	if dataTimeSec > e.latestDataTime {
		e.latestDataTime = dataTimeSec
	}
}

// schedulerState returns the current scheduler-relevant state.
func (e *engine) schedulerState() schedulerState {
	return schedulerState{
		lastAnalyzedDataTime: e.lastAnalyzedDataTime,
		latestDataTime:       e.latestDataTime,
	}
}

// advanceResult holds the outputs from an Advance call.
type advanceResult struct {
	anomalies []observerdef.Anomaly
	telemetry []observerdef.ObserverTelemetry
}

// Advance runs detectors and correlators up to the given event time.
// It returns all anomalies produced and updates the lastAnalyzedDataTime.
// The caller is responsible for routing anomalies to reporters or UI.
func (e *engine) Advance(upToSec int64) advanceResult {
	return e.advanceWithReason(upToSec, advanceReasonManual)
}

// advanceWithReason runs detectors and correlators up to the given event time,
// recording the reason for the advance in the emitted event.
func (e *engine) advanceWithReason(upToSec int64, reason advanceReason) advanceResult {
	if upToSec <= e.lastAnalyzedDataTime {
		return advanceResult{}
	}

	result := e.runDetectorsAndCorrelators(upToSec)
	e.lastAnalyzedDataTime = upToSec

	e.emit(engineEvent{
		kind:      eventAdvanceCompleted,
		timestamp: upToSec,
		advanceCompleted: &advanceCompletedEvent{
			advancedToSec:  upToSec,
			reason:         reason,
			anomalyCount:   len(result.anomalies),
			telemetryCount: len(result.telemetry),
			anomalies:      result.anomalies,
		},
	})

	return result
}

// runDetectorsAndCorrelators runs all detectors and feeds results through correlators.
func (e *engine) runDetectorsAndCorrelators(upTo int64) advanceResult {
	var allAnomalies []observerdef.Anomaly
	var allTelemetry []observerdef.ObserverTelemetry

	for _, detector := range e.detectors {
		result := detector.Detect(e.storage, upTo)
		for _, anomaly := range result.Anomalies {
			pkglog.Infof("[observer] anomaly detected: detector=%s source=%s timestamp=%d description=%q",
				anomaly.DetectorName, anomaly.Source, anomaly.Timestamp, anomaly.Description)
			e.captureRawAnomaly(anomaly)
			e.processAnomaly(anomaly)
			allAnomalies = append(allAnomalies, anomaly)

			e.emit(engineEvent{
				kind:      eventAnomalyCreated,
				timestamp: anomaly.Timestamp,
				anomalyCreated: &anomalyCreatedEvent{
					anomaly: anomaly,
				},
			})
		}
		allTelemetry = append(allTelemetry, result.Telemetry...)
	}

	// Advance correlators so they can update their internal state.
	for _, correlator := range e.correlators {
		correlator.Advance(upTo)
		e.accumulateCorrelations(correlator.ActiveCorrelations())
		e.emit(engineEvent{
			kind:      eventCorrelationUpdated,
			timestamp: upTo,
			correlationUpdated: &correlationUpdatedEvent{
				correlatorName: correlator.Name(),
			},
		})
	}

	// Accumulate telemetry so StateView can expose it.
	if len(allTelemetry) > 0 {
		e.telemetryMu.Lock()
		e.accumulatedTelemetry = append(e.accumulatedTelemetry, allTelemetry...)
		e.telemetryMu.Unlock()
	}

	return advanceResult{
		anomalies: allAnomalies,
		telemetry: allTelemetry,
	}
}

// processAnomaly sends an anomaly to all registered correlators.
func (e *engine) processAnomaly(anomaly observerdef.Anomaly) {
	for _, correlator := range e.correlators {
		correlator.ProcessAnomaly(anomaly)
	}
}

// captureRawAnomaly stores a raw anomaly for telemetry and testbench display.
// Deduplicates by SourceSeriesID+DetectorName+Timestamp.
func (e *engine) captureRawAnomaly(anomaly observerdef.Anomaly) {
	e.rawAnomalyMu.Lock()
	defer e.rawAnomalyMu.Unlock()

	e.totalAnomalyCount++

	if e.uniqueAnomalySources == nil {
		e.uniqueAnomalySources = make(map[observerdef.MetricName]bool)
	}
	e.uniqueAnomalySources[anomaly.Source] = true

	if anomaly.Timestamp > e.currentDataTime {
		e.currentDataTime = anomaly.Timestamp
	}

	// Deduplicate by SourceSeriesID+DetectorName+Timestamp
	key := anomalyDedupKey{
		seriesID:     anomaly.SourceSeriesID,
		detectorName: anomaly.DetectorName,
		timestamp:    anomaly.Timestamp,
	}
	if _, ok := e.rawAnomalyIndex[key]; ok {
		return // exact duplicate (same series + detector + timestamp)
	} else {
		if e.rawAnomalyIndex == nil {
			e.rawAnomalyIndex = make(map[anomalyDedupKey]int)
		}
		e.rawAnomalyIndex[key] = len(e.rawAnomalies)
		e.rawAnomalies = append(e.rawAnomalies, anomaly)
	}

	// Evict old anomalies if window is set
	needsReindex := false
	if e.rawAnomalyWindow > 0 {
		cutoff := e.currentDataTime - e.rawAnomalyWindow
		newBuffer := e.rawAnomalies[:0]
		for _, a := range e.rawAnomalies {
			if a.Timestamp >= cutoff {
				newBuffer = append(newBuffer, a)
			}
		}
		if len(newBuffer) != len(e.rawAnomalies) {
			needsReindex = true
		}
		e.rawAnomalies = newBuffer
	}

	// Cap at maxRawAnomalies if set
	if e.maxRawAnomalies > 0 && len(e.rawAnomalies) > e.maxRawAnomalies {
		e.rawAnomalies = e.rawAnomalies[len(e.rawAnomalies)-e.maxRawAnomalies:]
		needsReindex = true
	}

	// Rebuild index after eviction changes indices.
	if needsReindex {
		e.rawAnomalyIndex = make(map[anomalyDedupKey]int, len(e.rawAnomalies))
		for i, a := range e.rawAnomalies {
			e.rawAnomalyIndex[anomalyDedupKey{
				seriesID:     a.SourceSeriesID,
				detectorName: a.DetectorName,
				timestamp:    a.Timestamp,
			}] = i
		}
	}
}

// RawAnomalies returns a copy of currently tracked raw anomalies.
func (e *engine) RawAnomalies() []observerdef.Anomaly {
	e.rawAnomalyMu.RLock()
	defer e.rawAnomalyMu.RUnlock()

	result := make([]observerdef.Anomaly, len(e.rawAnomalies))
	copy(result, e.rawAnomalies)
	return result
}

// TotalAnomalyCount returns the total number of anomalies ever detected.
func (e *engine) TotalAnomalyCount() int {
	e.rawAnomalyMu.RLock()
	defer e.rawAnomalyMu.RUnlock()
	return e.totalAnomalyCount
}

// UniqueAnomalySourceCount returns the number of unique sources that had anomalies.
func (e *engine) UniqueAnomalySourceCount() int {
	e.rawAnomalyMu.RLock()
	defer e.rawAnomalyMu.RUnlock()
	return len(e.uniqueAnomalySources)
}

// accumulateCorrelations merges active correlations into the engine's historical set.
// Existing entries are updated if the new version has more anomalies or a later timestamp.
func (e *engine) accumulateCorrelations(active []observerdef.ActiveCorrelation) {
	e.correlationMu.Lock()
	defer e.correlationMu.Unlock()

	if e.accumulatedCorrelations == nil {
		e.accumulatedCorrelations = make(map[string]observerdef.ActiveCorrelation)
	}
	for _, ac := range active {
		existing, ok := e.accumulatedCorrelations[ac.Pattern]
		if !ok || len(ac.Anomalies) > len(existing.Anomalies) || ac.LastUpdated > existing.LastUpdated {
			e.accumulatedCorrelations[ac.Pattern] = ac
		}
	}
}

// AccumulatedCorrelations returns all correlations ever detected across the run.
func (e *engine) AccumulatedCorrelations() []observerdef.ActiveCorrelation {
	e.correlationMu.RLock()
	defer e.correlationMu.RUnlock()

	result := make([]observerdef.ActiveCorrelation, 0, len(e.accumulatedCorrelations))
	for _, ac := range e.accumulatedCorrelations {
		result = append(result, ac)
	}
	return result
}

// Storage returns the engine's storage.
func (e *engine) Storage() *timeSeriesStorage {
	return e.storage
}

// SetDetectors replaces the engine's detectors. Used when testbench components
// are toggled. Also refreshes the cached log observers list.
func (e *engine) SetDetectors(detectors []observerdef.Detector) {
	e.detectors = detectors
	e.logObservers = nil
	for _, d := range e.detectors {
		if lo, ok := d.(observerdef.LogObserver); ok {
			e.logObservers = append(e.logObservers, lo)
		}
	}
}

// SetCorrelators replaces the engine's correlators.
func (e *engine) SetCorrelators(correlators []observerdef.Correlator) {
	e.correlators = correlators
}

// Reset clears analysis state so detectors will re-analyze from scratch.
// This does NOT clear storage or raw anomalies — use resetFull for that.
func (e *engine) Reset() {
	e.lastAnalyzedDataTime = 0
	e.latestDataTime = 0

	for _, detector := range e.detectors {
		if resetter, ok := detector.(interface{ Reset() }); ok {
			resetter.Reset()
		}
	}

	for _, correlator := range e.correlators {
		correlator.Reset()
	}
}

// resetRawAnomalies clears the raw anomaly tracking state.
func (e *engine) resetRawAnomalies() {
	e.rawAnomalyMu.Lock()
	defer e.rawAnomalyMu.Unlock()

	e.rawAnomalies = nil
	e.rawAnomalyIndex = nil
	e.totalAnomalyCount = 0
	e.uniqueAnomalySources = nil
	e.currentDataTime = 0
}

// resetTelemetry clears accumulated telemetry.
func (e *engine) resetTelemetry() {
	e.telemetryMu.Lock()
	defer e.telemetryMu.Unlock()
	e.accumulatedTelemetry = nil
}

// resetCorrelations clears accumulated correlation history.
func (e *engine) resetCorrelations() {
	e.correlationMu.Lock()
	defer e.correlationMu.Unlock()
	e.accumulatedCorrelations = nil
}

// resetFull resets all engine state: analysis progress, raw anomalies, telemetry, and correlations.
// Storage is NOT cleared — the caller manages storage lifecycle.
func (e *engine) resetFull() {
	e.Reset()
	e.resetRawAnomalies()
	e.resetTelemetry()
	e.resetCorrelations()
}

// ReplayStoredData replays all data in storage through the scheduler policy,
// using the same timing semantics as live ingestion. For each unique data
// timestamp, it consults the scheduler to decide when to advance analysis.
// After all timestamps are processed, calls onReplayEnd to flush remaining data.
func (e *engine) ReplayStoredData() advanceResult {
	var allAnomalies []observerdef.Anomaly
	var allTelemetry []observerdef.ObserverTelemetry

	timestamps := e.storage.DataTimestamps()
	for _, ts := range timestamps {
		e.trackLatestDataTime(ts)
		requests := e.scheduler.onObservation(ts, e.schedulerState())
		for _, req := range requests {
			result := e.advanceWithReason(req.upToSec, req.reason)
			allAnomalies = append(allAnomalies, result.anomalies...)
			allTelemetry = append(allTelemetry, result.telemetry...)
		}
	}

	// Final flush for any remaining data not yet analyzed.
	endRequests := e.scheduler.onReplayEnd(e.schedulerState())
	for _, req := range endRequests {
		result := e.advanceWithReason(req.upToSec, req.reason)
		allAnomalies = append(allAnomalies, result.anomalies...)
		allTelemetry = append(allTelemetry, result.telemetry...)
	}

	return advanceResult{
		anomalies: allAnomalies,
		telemetry: allTelemetry,
	}
}
