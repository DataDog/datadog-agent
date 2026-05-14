// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// Note: stateView is defined in stateview.go and provides read-only access
// to engine state for consumers like the testbench UI.

// anomalyDedupKey is a map key for O(1) anomaly deduplication.
type anomalyDedupKey struct {
	sourceKey    string // SeriesDescriptor.Key()
	detectorName string
	timestamp    int64
	title        string
}

type seriesContextRef struct {
	namespace  string
	contextKey string
}

// engine is the shared orchestration core for the observer pipeline.
// It encapsulates storage, log extraction, detection, and correlation,
// providing a single execution path used by both the live observer and testbench.
//
// The engine does not own reporters or scheduling policy. It accepts explicit
// Advance calls and returns results that callers route to their own outputs.
type engine struct {
	// mu protects detectors, correlators, extractors, logObservers,
	// lastAnalyzedDataTime, and latestDataTime from concurrent access.
	// Writers (Advance, Reset, SetDetectors, SetCorrelators, SetExtractors)
	// take a write lock; readers (stateView methods) take a read lock.
	mu sync.RWMutex

	storage          *timeSeriesStorage
	extractors       []observerdef.LogMetricsExtractor
	detectors        []observerdef.Detector
	correlators      []observerdef.Correlator
	contextProviders map[string]observerdef.ContextProvider // namespace → provider
	contextRefs      map[observerdef.SeriesRef]seriesContextRef

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
	rawAnomalyWindow     int64           // seconds to keep (0 = unlimited)
	maxRawAnomalies      int             // max count to keep (0 = unlimited)
	currentDataTime      int64           // latest anomaly timestamp seen
	totalAnomalyCount    int             // total count ever (no cap)
	uniqueAnomalySources map[string]bool // unique sources that had anomalies (keyed by SeriesDescriptor.Key())

	// Accumulated correlations from all advance cycles.
	// Correlators maintain sliding windows that evict old state, but for
	// testbench/replay we want the full history. This map accumulates
	// every correlation ever seen, keyed by Pattern string, updating
	// existing entries when the correlator reports a newer version.
	accumulatedCorrelations map[string]observerdef.ActiveCorrelation
	correlationMu           sync.RWMutex

	// onProcessingTime is an optional callback for reporting per-component
	// processing time directly (gauge.Set) instead of constructing ObserverTelemetry
	// objects. Live mode sets this; nil means skip timing telemetry entirely.
	onProcessingTime func(detectorTag string, nanos float64)

	// detectorTags caches "detector:<name>" strings for each extractor,
	// detector, logObserver, and correlator to avoid per-log concatenation.
	detectorTags map[string]string

	// Event subscription management.
	sinks   []eventSink
	sinksMu sync.RWMutex

	// Replay progress counters (atomic, lock-free reads).
	replayTimestampsDone  atomic.Int64
	replayTimestampsTotal atomic.Int64
	replayAdvances        atomic.Int64
	replayAnomalies       atomic.Int64
	replayPhase           atomic.Value // string: "", "loading", "detecting", "done"

	// Optional instrumentation for live/replay parity debugging.
	onDetectDigest func(detectDigest)
	instrStorage   *instrumentedStorage
	onAdvance      func(advanceEntry) // scheduler trace

	// Counters for data ingestion anomalies, reset after each advance.
	latePoints         atomic.Int64     // points ingested after their timestamp was already analyzed
	latePointsBySource map[string]int64 // per-source breakdown (single-goroutine access from run loop)
	handles            []*handle        // registered handles for per-source drop collection
	handlesMu          sync.Mutex       // protects handles slice

	// sourceTagCache memoises the "observer_source:<source>" string used in
	// IngestLog/IngestMetric. Without this we allocate a fresh string per
	// log/metric ingest. Sources are a small bounded set (e.g. "logs",
	// "profiles", "traces") so a single-goroutine map is plenty; access is
	// confined to the engine run loop. Lock-free via atomic.Pointer to a
	// copy-on-write map so we don't add a mutex to the hot path.
	sourceTagCache atomic.Pointer[map[string]string]
}

// engineConfig holds the parameters for constructing an engine.
type engineConfig struct {
	storage          *timeSeriesStorage
	extractors       []observerdef.LogMetricsExtractor
	detectors        []observerdef.Detector
	correlators      []observerdef.Correlator
	contextProviders map[string]observerdef.ContextProvider // namespace → provider

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
	validateUniqueExtractorNames(cfg.extractors)

	e := &engine{
		storage:          cfg.storage,
		extractors:       cfg.extractors,
		detectors:        cfg.detectors,
		correlators:      cfg.correlators,
		contextProviders: cfg.contextProviders,
		contextRefs:      make(map[observerdef.SeriesRef]seriesContextRef),
		scheduler:        sched,

		rawAnomalyWindow: cfg.rawAnomalyWindow,
		maxRawAnomalies:  cfg.maxRawAnomalies,
		rawAnomalyIndex:  make(map[anomalyDedupKey]int),
	}

	// Cache log observers from detectors.
	for _, d := range e.detectors {
		if lo, ok := d.(observerdef.LogObserver); ok {
			e.logObservers = append(e.logObservers, lo)
		}
	}

	e.rebuildDetectorTags()
	return e
}

// rebuildDetectorTags rebuilds the cached "detector:<name>" tag strings from
// the current extractors, detectors, logObservers, and correlators.
// Called on construction and whenever the component sets change.
func (e *engine) rebuildDetectorTags() {
	tags := make(map[string]string)
	for _, ext := range e.extractors {
		tags[ext.Name()] = "detector:" + ext.Name()
	}
	for _, d := range e.detectors {
		tags[d.Name()] = "detector:" + d.Name()
	}
	for _, lo := range e.logObservers {
		tags[lo.Name()] = "detector:" + lo.Name()
	}
	for _, c := range e.correlators {
		tags[c.Name()] = "detector:" + c.Name()
	}
	e.detectorTags = tags
}

// detectorTag returns the cached "detector:<name>" tag string. Falls back to
// concatenation if the name is not cached (should not happen in practice).
func (e *engine) detectorTag(name string) string {
	if tag, ok := e.detectorTags[name]; ok {
		return tag
	}
	return "detector:" + name
}

// enableDetectDigestRecording sets a callback invoked after each Detect() call
// with a digest of the detection output and input hash. Pass nil to disable.
func (e *engine) enableDetectDigestRecording(fn func(detectDigest)) {
	e.onDetectDigest = fn
	if fn != nil {
		e.instrStorage = newInstrumentedStorage(e.storage)
	} else {
		e.instrStorage = nil
	}
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

// registerHandle adds a handle to the engine's handle list so that per-source
// drop counts can be collected at advance time.
func (e *engine) registerHandle(h *handle) {
	e.handlesMu.Lock()
	e.handles = append(e.handles, h)
	e.handlesMu.Unlock()
}

// sourceTagForIngest returns "observer_source:<source>" with memoisation so
// IngestLog / IngestMetric don't allocate a fresh string per ingest. The
// source set is small and bounded; a copy-on-write map indexed via an
// atomic.Pointer keeps reads lock-free on the hot path.
//
// The bounded-source assumption: every production caller of obs.GetHandle()
// passes a statically-defined string constant. As of this writing the full
// set is:
//   - "all-metrics"          (pkg/aggregator/demultiplexer_agent.go)
//   - "dogstatsd"            (comp/dogstatsd/server/server.go)
//   - "logs"                 (comp/observer/logssource/impl/component.go)
//   - "agent-internal-logs"  (comp/observer/impl/observer.go)
//   - "profile-agent"        (comp/observer/impl/observer.go)
//   - hfrunnerdef.HFSource          (comp/anomalydetection/hfrunner/def/component.go)
//   - hfrunnerdef.HFContainerSource (comp/anomalydetection/hfrunner/def/component.go)
//
// If a future caller ever passes a user-controlled or per-container source
// string, the COW map becomes unbounded and this memoisation strategy is
// the wrong shape (use sync.Map or a bounded LRU). Adding an entry to that
// list above means revisiting this function.
func (e *engine) sourceTagForIngest(source string) string {
	if m := e.sourceTagCache.Load(); m != nil {
		if tag, ok := (*m)[source]; ok {
			return tag
		}
	}
	tag := "observer_source:" + source
	for {
		old := e.sourceTagCache.Load()
		newMap := make(map[string]string, 4)
		if old != nil {
			for k, v := range *old {
				newMap[k] = v
			}
		}
		newMap[source] = tag
		if e.sourceTagCache.CompareAndSwap(old, &newMap) {
			break
		}
	}
	return tag
}

// IngestMetric stores a metric observation and consults the scheduler policy
// to determine whether detectors should advance. Returns advance requests
// that the caller should execute via Advance.
func (e *engine) IngestMetric(source string, m *metricObs) []advanceRequest {
	e.storage.Add(source, m.name, m.value, m.timestamp, m.tags)
	// Track points that arrive after their timestamp was already analyzed.
	// These points are in storage but were invisible to detectors at analysis time.
	if m.timestamp <= e.lastAnalyzedDataTime {
		e.latePoints.Add(1)
		if e.latePointsBySource == nil {
			e.latePointsBySource = make(map[string]int64)
		}
		e.latePointsBySource[source]++
	}
	e.trackLatestDataTime(m.timestamp)
	return e.scheduler.onObservation(m.timestamp, e.schedulerState())
}

// IngestLog processes a log observation: runs extractors to produce virtual metrics,
// notifies log observers, and consults the scheduler policy to determine whether
// detectors should advance. Returns advance requests that the caller should execute.
func (e *engine) IngestLog(source string, l *logObs) ([]advanceRequest, []observerdef.ObserverTelemetry) {
	sourceTag := e.sourceTagForIngest(source)
	view := &logView{obs: l}
	var logTelemetry []observerdef.ObserverTelemetry
	for _, extractor := range e.extractors {
		processingStartTime := time.Now()
		out := extractor.ProcessLog(view)
		e.removeContextRefsForEvictedKeys(extractor.Name(), out.EvictedContextKeys)
		if e.onProcessingTime != nil {
			e.onProcessingTime(e.detectorTag(extractor.Name()), float64(time.Since(processingStartTime).Nanoseconds()))
		}
		for _, m := range out.Metrics {
			// Avoid copying m.Tags when sourceTag is already present: storage.Add
			// performs its own deep copy on first-write of a series via
			// canonicalizeTags, and seriesKey sorts a copy internally — neither
			// mutates the input. The copy is only required when we need to
			// append sourceTag without disturbing the extractor's slice.
			tags := m.Tags
			if !sliceContains(tags, sourceTag) {
				newTags := make([]string, len(tags), len(tags)+1)
				copy(newTags, tags)
				tags = append(newTags, sourceTag)
			}
			res := e.storage.Add(extractor.Name(), m.Name, m.Value, l.timestampMs/1000, tags)
			if m.ContextKey != "" && res.Ref >= 0 {
				e.contextRefs[res.Ref] = seriesContextRef{
					namespace:  extractor.Name(),
					contextKey: m.ContextKey,
				}
			}
		}
		if len(out.Telemetry) > 0 {
			logTelemetry = append(logTelemetry, out.Telemetry...)
		}
	}
	for _, lo := range e.logObservers {
		processingStartTime := time.Now()
		lo.ProcessLog(view)
		if e.onProcessingTime != nil {
			e.onProcessingTime(e.detectorTag(lo.Name()), float64(time.Since(processingStartTime).Nanoseconds()))
		}
	}
	dataTimeSec := l.timestampMs / 1000
	e.storage.RecordObservationTime(dataTimeSec)
	e.trackLatestDataTime(dataTimeSec)
	return e.scheduler.onObservation(dataTimeSec, e.schedulerState()), logTelemetry
}

func sliceContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// removeContextRefsForEvictedKeys drops engine contextRefs whose extractor
// namespace and context key match an eviction from extractor GC, and frees
// the corresponding storage series. Without the storage cleanup, evicted
// patterns leak their tags + columnar arrays indefinitely (the contextRefs
// map is just metadata; the heavy data lives in storage.series).
func (e *engine) removeContextRefsForEvictedKeys(namespace string, evictedKeys []string) {
	if len(evictedKeys) == 0 {
		return
	}
	want := make(map[string]struct{}, len(evictedKeys))
	for _, k := range evictedKeys {
		if k != "" {
			want[k] = struct{}{}
		}
	}
	if len(want) == 0 {
		return
	}
	var freedRefs []observerdef.SeriesRef
	for ref, contextRef := range e.contextRefs {
		if contextRef.namespace != namespace {
			continue
		}
		if _, ok := want[contextRef.contextKey]; ok {
			delete(e.contextRefs, ref)
			freedRefs = append(freedRefs, ref)
		}
	}
	if len(freedRefs) > 0 {
		freed := e.storage.RemoveSeriesByRefs(freedRefs)
		e.fanOutSeriesRemoval(freed)
	}
}

// fanOutSeriesRemoval notifies every detector that implements the optional
// SeriesRemover interface that the listed SeriesRefs have been freed by
// storage. This keeps detector-side per-series state (BOCPD posterior maps,
// ScanMW/ScanWelch segment trackers, seriesDetectorAdapter visible-count
// maps) symmetric with storage so the LRU caps placed on extractorsâ
// contexts actually translate into bounded heap usage end-to-end.
//
// The caller (removeContextRefsForEvictedKeys / Reset / future GC paths)
// is responsible for invoking this with whatever refs storage actually
// freed. Detectors are expected to ignore unknown refs, so itâs safe to
// broadcast the same ref list to all of them.
//
// Concurrency invariant: this method, like every method on engine and
// every detector RemoveSeries / Detect callback, runs only on the single
// goroutine driving observerImpl.run() (observer.go). Ingest, advance,
// detection, and these eviction fan-outs are all serialised through that
// loop, so detector implementations may mutate per-series state without
// taking their own locks. Adding a new caller of this function from a
// different goroutine would break that invariant for every detector.
func (e *engine) fanOutSeriesRemoval(refs []observerdef.SeriesRef) {
	if len(refs) == 0 || len(e.detectors) == 0 {
		return
	}
	for _, d := range e.detectors {
		if remover, ok := d.(observerdef.SeriesRemover); ok {
			remover.RemoveSeries(refs)
		}
	}
}

// trackLatestDataTime updates latestDataTime if the given timestamp is newer.
func (e *engine) trackLatestDataTime(dataTimeSec int64) {
	e.mu.Lock()
	if dataTimeSec > e.latestDataTime {
		e.latestDataTime = dataTimeSec
	}
	e.mu.Unlock()
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
	// Snapshot mutable fields under the lock. We cannot hold mu during
	// runDetectorsAndCorrelators because emit() callbacks may re-enter
	// stateView methods that take mu.RLock, causing a deadlock.
	e.mu.Lock()
	if upToSec <= e.lastAnalyzedDataTime {
		e.mu.Unlock()
		return advanceResult{}
	}
	detectors := e.detectors
	correlators := e.correlators
	e.lastAnalyzedDataTime = upToSec
	e.mu.Unlock()

	if e.onAdvance != nil {
		var lateBySource map[string]int64
		if len(e.latePointsBySource) > 0 {
			lateBySource = e.latePointsBySource
			e.latePointsBySource = nil
		}
		var totalDrops int64
		var dropsBySource map[string]int64
		e.handlesMu.Lock()
		for _, h := range e.handles {
			n := h.dropCount.Swap(0)
			if n > 0 {
				totalDrops += n
				if dropsBySource == nil {
					dropsBySource = make(map[string]int64)
				}
				dropsBySource[h.source] += n
			}
		}
		e.handlesMu.Unlock()
		e.onAdvance(advanceEntry{
			DataTime:           upToSec,
			Reason:             advanceReasonString(reason),
			LatePoints:         e.latePoints.Swap(0),
			LatePointsBySource: lateBySource,
			DroppedObs:         totalDrops,
			DroppedBySource:    dropsBySource,
		})
	}

	result := e.runDetectorsAndCorrelatorsSnapshot(upToSec, detectors, correlators)

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

// runDetectorsAndCorrelatorsSnapshot runs the given detectors and correlators.
// Uses explicit slices so the caller can snapshot them under a lock.
//
// Scan detectors (ScanMW, ScanWelch) emit anomalies with historical changepoint
// timestamps that may be hundreds of seconds behind upTo. The correlator's
// currentDataTime persists across calls at the previous upTo, so advancing
// correlators to upTo after processing would evict just-formed clusters before
// they can be accumulated. We accumulate correlations BEFORE advancing so
// clusters are captured while still alive.
func (e *engine) runDetectorsAndCorrelatorsSnapshot(upTo int64, detectors []observerdef.Detector, correlators []observerdef.Correlator) advanceResult {
	var allAnomalies []observerdef.Anomaly
	var allTelemetry []observerdef.ObserverTelemetry

	// Detect, deduplicate, and feed anomalies to correlators.
	for _, detector := range detectors {
		// Use instrumented storage when digest recording is active.
		storageForDetect := observerdef.StorageReader(e.storage)
		if e.instrStorage != nil {
			e.instrStorage.inner = e.storage // rebind in case storage was swapped
			e.instrStorage.reset()
			storageForDetect = e.instrStorage
		}

		processingStartTime := time.Now()
		result := detector.Detect(storageForDetect, upTo)
		if e.onProcessingTime != nil {
			e.onProcessingTime(e.detectorTag(detector.Name()), float64(time.Since(processingStartTime).Nanoseconds()))
		}

		// Emit detect digest (captures raw result BEFORE dedup).
		if e.onDetectDigest != nil {
			fps := make([]string, len(result.Anomalies))
			for i, a := range result.Anomalies {
				fps[i] = anomalyFingerprint(a)
			}
			sort.Strings(fps)
			dd := detectDigest{
				DetectorName:        detector.Name(),
				DataTime:            upTo,
				AnomalyCount:        len(result.Anomalies),
				AnomalyFingerprints: fps,
			}
			if e.instrStorage != nil {
				rd := e.instrStorage.digest(detector.Name(), upTo)
				dd.InputHash = rd.Hash
				dd.ReadCount = rd.ReadCount
				dd.PointCount = rd.PointCount
			}
			e.onDetectDigest(dd)
		}

		for _, anomaly := range result.Anomalies {
			e.enrichAnomaly(&anomaly)
			if !e.captureRawAnomaly(anomaly) {
				continue // duplicate
			}
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

	// Accumulate correlations before advancing — captures clusters formed from
	// historical-timestamp anomalies before Advance(upTo) evicts them.
	for _, correlator := range correlators {
		e.accumulateCorrelations(correlator.ActiveCorrelations())
		advanceStart := time.Now()
		correlator.Advance(upTo)
		if e.onProcessingTime != nil {
			e.onProcessingTime(e.detectorTag(correlator.Name()), float64(time.Since(advanceStart).Nanoseconds()))
		}
		e.emit(engineEvent{
			kind:      eventCorrelationUpdated,
			timestamp: upTo,
			correlationUpdated: &correlationUpdatedEvent{
				correlatorName: correlator.Name(),
			},
		})
	}

	return advanceResult{
		anomalies: allAnomalies,
		telemetry: allTelemetry,
	}
}

// enrichAnomaly decorates an anomaly with context from the originating
// extractor, if available. This runs automatically on every anomaly so
// detectors don't need to be aware of context providers.
// Lookup uses the anomaly's SourceRef (set by all detectors that write to storage)
// for an O(1) contextRefs lookup — no seriesKey recomputation needed.
func (e *engine) enrichAnomaly(a *observerdef.Anomaly) {
	if a.SourceRef == nil {
		return
	}
	ref, ok := e.contextRefs[a.SourceRef.Ref]
	if !ok {
		return
	}
	provider, ok := e.contextProviders[ref.namespace]
	if !ok {
		return
	}
	ctx, ok := provider.GetContextByKey(ref.contextKey)
	if !ok {
		return
	}
	a.Context = &observerdef.MetricContext{
		Pattern:   ctx.Pattern,
		Example:   truncate(ctx.Example, 160),
		Source:    ctx.Source,
		SplitTags: ctx.SplitTags,
	}
}

// processAnomaly sends an anomaly to all registered correlators.
func (e *engine) processAnomaly(anomaly observerdef.Anomaly) {
	for _, correlator := range e.correlators {
		processingStartTime := time.Now()
		correlator.ProcessAnomaly(anomaly)
		if e.onProcessingTime != nil {
			e.onProcessingTime(e.detectorTag(correlator.Name()), float64(time.Since(processingStartTime).Nanoseconds()))
		}
	}
}

// captureRawAnomaly stores a raw anomaly for telemetry and testbench display.
// Deduplicates by Source+DetectorName+Timestamp+Title.
// Returns true if the anomaly was new, false if it was a duplicate.
func (e *engine) captureRawAnomaly(anomaly observerdef.Anomaly) bool {
	e.rawAnomalyMu.Lock()
	defer e.rawAnomalyMu.Unlock()

	e.totalAnomalyCount++

	if e.uniqueAnomalySources == nil {
		e.uniqueAnomalySources = make(map[string]bool)
	}
	const maxUniqueSources = 500
	if len(e.uniqueAnomalySources) < maxUniqueSources {
		e.uniqueAnomalySources[anomaly.Source.Key()] = true
	}

	if anomaly.Timestamp > e.currentDataTime {
		e.currentDataTime = anomaly.Timestamp
	}

	// Deduplicate by Source+DetectorName+Timestamp+Title
	key := anomalyDedupKey{
		sourceKey:    anomaly.Source.Key(),
		detectorName: anomaly.DetectorName,
		timestamp:    anomaly.Timestamp,
		title:        anomaly.Title,
	}
	if _, ok := e.rawAnomalyIndex[key]; ok {
		return false // exact duplicate
	}
	e.rawAnomalyIndex[key] = len(e.rawAnomalies)
	e.rawAnomalies = append(e.rawAnomalies, anomaly)

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
				sourceKey:    a.Source.Key(),
				detectorName: a.DetectorName,
				timestamp:    a.Timestamp,
				title:        a.Title,
			}] = i
		}
	}
	return true
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
const maxAccumulatedCorrelations = 500

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

	// Evict oldest entries if over cap.
	for len(e.accumulatedCorrelations) > maxAccumulatedCorrelations {
		var oldestKey string
		var oldestTime int64 = math.MaxInt64
		for k, ac := range e.accumulatedCorrelations {
			if ac.LastUpdated < oldestTime {
				oldestTime = ac.LastUpdated
				oldestKey = k
			}
		}
		delete(e.accumulatedCorrelations, oldestKey)
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
	e.mu.Lock()
	defer e.mu.Unlock()

	e.detectors = detectors
	e.logObservers = nil
	for _, d := range e.detectors {
		if lo, ok := d.(observerdef.LogObserver); ok {
			e.logObservers = append(e.logObservers, lo)
		}
	}
	e.rebuildDetectorTags()
}

// SetCorrelators replaces the engine's correlators.
func (e *engine) SetCorrelators(correlators []observerdef.Correlator) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.correlators = correlators
	e.rebuildDetectorTags()
}

// SetExtractors replaces the engine's log-metrics extractors. Used when
// testbench components are toggled so that replayed log ingestion uses
// only the currently-enabled extractors.
func (e *engine) SetExtractors(extractors []observerdef.LogMetricsExtractor) {
	e.mu.Lock()
	defer e.mu.Unlock()

	validateUniqueExtractorNames(extractors)
	e.extractors = extractors
	e.contextProviders = collectContextProviders(extractors)
	e.contextRefs = make(map[observerdef.SeriesRef]seriesContextRef)
	e.rebuildDetectorTags()
}

// Reset clears analysis state so detectors will re-analyze from scratch.
// This does NOT clear storage or raw anomalies — use resetFull for that.
func (e *engine) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()

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

	for _, extractor := range e.extractors {
		if resetter, ok := extractor.(interface{ Reset() }); ok {
			resetter.Reset()
		}
	}

	e.contextRefs = make(map[observerdef.SeriesRef]seriesContextRef)
}

// resetRawAnomalies clears the raw anomaly tracking state.
func (e *engine) resetRawAnomalies() {
	e.rawAnomalyMu.Lock()
	defer e.rawAnomalyMu.Unlock()

	e.rawAnomalies = nil
	e.rawAnomalyIndex = make(map[anomalyDedupKey]int)
	e.totalAnomalyCount = 0
	e.uniqueAnomalySources = nil
	e.currentDataTime = 0
}

// resetCorrelations clears accumulated correlation history.
func (e *engine) resetCorrelations() {
	e.correlationMu.Lock()
	defer e.correlationMu.Unlock()
	e.accumulatedCorrelations = nil
}

// resetFull resets all engine state: analysis progress, raw anomalies, and correlations.
// Storage is NOT cleared — the caller manages storage lifecycle.
func (e *engine) resetFull() {
	e.Reset()
	e.resetRawAnomalies()
	e.resetCorrelations()
}

// resetAnalysisState resets detector and correlator state, anomaly tracking,
// telemetry, and correlations — but does NOT reset extractors and does NOT
// clear contextRefs. Used before batch replay so that:
//   - enrichAnomaly can still call provider.GetContextByKey (extractor context intact)
//   - contextRefs still maps series storage keys to their context keys
//
// Detectors and correlators ARE reset so they start from a clean slate and
// produce correct anomaly/correlation results during the replay.
func (e *engine) resetAnalysisState() {
	e.mu.Lock()
	e.lastAnalyzedDataTime = 0
	e.latestDataTime = 0
	e.mu.Unlock()

	for _, detector := range e.detectors {
		if resetter, ok := detector.(interface{ Reset() }); ok {
			resetter.Reset()
		}
	}
	for _, correlator := range e.correlators {
		correlator.Reset()
	}
	// Extractors and contextRefs are intentionally NOT reset: their state was
	// built during log ingestion and is needed by enrichAnomaly during replay.

	e.resetRawAnomalies()
	e.resetCorrelations()
}

// ResetForReplay reconfigures with new components, clears all state, and replaces storage.
func (e *engine) ResetForReplay(detectors []observerdef.Detector, correlators []observerdef.Correlator, extractors []observerdef.LogMetricsExtractor) {
	e.SetDetectors(detectors)
	e.SetCorrelators(correlators)
	e.SetExtractors(extractors)
	e.resetFull()
	e.mu.Lock()
	e.storage = newTimeSeriesStorage()
	e.mu.Unlock()
}

// ExtractorCount returns the number of extractors currently registered.
func (e *engine) ExtractorCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.extractors)
}

// SetReplayPhase stores the current replay phase for progress reporting.
func (e *engine) SetReplayPhase(phase string) {
	e.replayPhase.Store(phase)
}

// ReplayProgress holds lock-free replay progress counters.
type ReplayProgress struct {
	Phase           string `json:"phase"` // "", "loading", "detecting", "done"
	TimestampsDone  int64  `json:"timestampsDone"`
	TimestampsTotal int64  `json:"timestampsTotal"`
	Advances        int64  `json:"advances"`
	Anomalies       int64  `json:"anomalies"`
}

// GetReplayProgress returns the current replay progress (lock-free).
func (e *engine) GetReplayProgress() ReplayProgress {
	phase, _ := e.replayPhase.Load().(string)
	return ReplayProgress{
		Phase:           phase,
		TimestampsDone:  e.replayTimestampsDone.Load(),
		TimestampsTotal: e.replayTimestampsTotal.Load(),
		Advances:        e.replayAdvances.Load(),
		Anomalies:       e.replayAnomalies.Load(),
	}
}

// ReplayStoredData replays all data in storage through the scheduler policy,
// using the same timing semantics as live ingestion. For each unique data
// timestamp, it consults the scheduler to decide when to advance analysis.
// After all timestamps are processed, calls onReplayEnd to flush remaining data.
func (e *engine) ReplayStoredData() advanceResult {
	var allAnomalies []observerdef.Anomaly
	var allTelemetry []observerdef.ObserverTelemetry

	timestamps := e.storage.DataTimestamps()

	e.replayPhase.Store("detecting")
	e.replayTimestampsTotal.Store(int64(len(timestamps)))
	e.replayTimestampsDone.Store(0)
	e.replayAdvances.Store(0)
	e.replayAnomalies.Store(0)

	advances := 0
	for i, ts := range timestamps {
		e.trackLatestDataTime(ts)
		requests := e.scheduler.onObservation(ts, e.schedulerState())
		for _, req := range requests {
			result := e.advanceWithReason(req.upToSec, req.reason)
			allAnomalies = append(allAnomalies, result.anomalies...)
			allTelemetry = append(allTelemetry, result.telemetry...)
			advances++
		}
		e.replayTimestampsDone.Store(int64(i + 1))
		e.replayAdvances.Store(int64(advances))
		e.replayAnomalies.Store(int64(len(allAnomalies)))
	}

	// Final flush for any remaining data not yet analyzed.
	endRequests := e.scheduler.onReplayEnd(e.schedulerState())
	for _, req := range endRequests {
		result := e.advanceWithReason(req.upToSec, req.reason)
		allAnomalies = append(allAnomalies, result.anomalies...)
		allTelemetry = append(allTelemetry, result.telemetry...)
		advances++
	}

	e.replayAdvances.Store(int64(advances))
	e.replayAnomalies.Store(int64(len(allAnomalies)))
	e.replayPhase.Store("done")

	return advanceResult{
		anomalies: allAnomalies,
		telemetry: allTelemetry,
	}
}

// ReplayWithLiveSchedule replays stored data but only advances at the timestamps
// recorded in the live advance log. The live advance log records upToSec values
// (typically dataTimeSec-1 from the scheduler), which may not match data timestamps
// exactly. We advance at each live time once the data stream has reached or passed it.
func (e *engine) ReplayWithLiveSchedule(liveAdvanceTimes []int64) advanceResult {
	var allAnomalies []observerdef.Anomaly
	var allTelemetry []observerdef.ObserverTelemetry

	timestamps := e.storage.DataTimestamps()

	e.replayPhase.Store("detecting")
	e.replayTimestampsTotal.Store(int64(len(timestamps)))
	e.replayTimestampsDone.Store(0)
	e.replayAdvances.Store(0)
	e.replayAnomalies.Store(0)

	// liveAdvanceTimes must be sorted (guaranteed by liveAdvanceTimes()).
	liveIdx := 0
	advances := 0
	for i, ts := range timestamps {
		e.trackLatestDataTime(ts)

		// Advance at all live advance times that the data stream has reached.
		// Live advance times are upToSec values (often dataTimeSec-1), so they
		// may not appear in DataTimestamps(). We trigger when ts >= advanceTime.
		for liveIdx < len(liveAdvanceTimes) && liveAdvanceTimes[liveIdx] <= ts {
			result := e.advanceWithReason(liveAdvanceTimes[liveIdx], advanceReasonInputDriven)
			allAnomalies = append(allAnomalies, result.anomalies...)
			allTelemetry = append(allTelemetry, result.telemetry...)
			advances++
			liveIdx++
		}

		e.replayTimestampsDone.Store(int64(i + 1))
		e.replayAdvances.Store(int64(advances))
		e.replayAnomalies.Store(int64(len(allAnomalies)))
	}

	// Final flush for any remaining data not yet analyzed.
	endRequests := e.scheduler.onReplayEnd(e.schedulerState())
	for _, req := range endRequests {
		result := e.advanceWithReason(req.upToSec, req.reason)
		allAnomalies = append(allAnomalies, result.anomalies...)
		allTelemetry = append(allTelemetry, result.telemetry...)
		advances++
	}

	e.replayAdvances.Store(int64(advances))
	e.replayAnomalies.Store(int64(len(allAnomalies)))
	e.replayPhase.Store("done")

	return advanceResult{
		anomalies: allAnomalies,
		telemetry: allTelemetry,
	}
}
