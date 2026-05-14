// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerimpl implements the observer component.
package observerimpl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	compdef "github.com/DataDog/datadog-agent/comp/def"

	hfrunnerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/hfrunner/def"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	reporterdef "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	"github.com/DataDog/datadog-agent/pkg/metrics"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires declares the input types to the observer component constructor.
type Requires struct {
	Lifecycle compdef.Lifecycle
	Config    config.Component
	Log       log.Component
	Telemetry telemetry.Component

	// Recorder is an optional component for transparent metric recording.
	// If provided, all handles will be wrapped to record metrics to parquet files.
	Recorder option.Option[recorderdef.Component]

	// Reporters are provided by reporter/fx, reporter/fx-testbench, etc. via the
	// `anomalydetection_reporters` Fx group. Each reporter gets its own subscription
	// so it receives advance events independently. StorageConsumer reporters receive
	// storage for windowed log-rate annotations.
	Reporters []reporterdef.Reporter `group:"anomalydetection_reporters"`

	// HFRunner runs system and container checks at 1s and routes them into the
	// observer pipeline. The noop variant (hfrunner/fx-noop) is wired for the
	// main agent build; the real implementation lands with the algorithm PRs.
	HFRunner hfrunnerdef.Component
}

// Provides defines the output of the observer component.
type Provides struct {
	Comp observerdef.Component
}

// observation is a message sent from handles to the observer.
type observation struct {
	source string
	metric *metricObs
	log    *logObs
	// flush, when non-nil, is closed by the dispatch loop once this observation
	// is reached, signalling that all prior observations have been processed.
	flush chan struct{}
}

// metricObs contains copied metric data and implements observerdef.MetricView.
type metricObs struct {
	name      string
	value     float64
	tags      []string
	timestamp int64
}

// Ensure metricObs implements observerdef.MetricView
var _ observerdef.MetricView = (*metricObs)(nil)

func (m *metricObs) GetName() string {
	return m.name
}

func (m *metricObs) GetValue() float64 {
	return m.value
}

func (m *metricObs) GetRawTags() []string {
	return m.tags
}

func (m *metricObs) GetTimestampUnix() int64 { return m.timestamp }

// Observer does not store samplerate; just return 1.0
func (m *metricObs) GetSampleRate() float64 {
	return 1.0
}

// logObs contains copied log data and implements observerdef.LogView.
type logObs struct {
	content     string
	status      string
	tags        []string
	hostname    string
	timestampMs int64
}

// Ensure logObs implements observerdef.LogView
var _ observerdef.LogView = (*logObs)(nil)

func (l *logObs) GetContent() string {
	return l.content
}

func (l *logObs) GetStatus() string {
	return l.status
}

func (l *logObs) Tags() []string {
	return l.tags
}

func (l *logObs) GetHostname() string {
	return l.hostname
}

// Optionally, for logs that provide timestamp interface (if needed elsewhere)
func (l *logObs) GetTimestampUnixMilli() int64 {
	return l.timestampMs
}

// settingsFromAgentConfig reads component configuration from the agent config
// system (datadog.yaml). Keys follow the pattern:
//
//	anomaly_detection.detectors.<name>.enabled        (bool)
//	anomaly_detection.detectors.<name>.<field>        (type-specific)
//
// Enabled keys must be registered in pkg/config/setup/config.go.
// Component-specific keys are read via the AgentConfigurable interface —
// config structs that implement it will have their fields populated
// automatically.
func settingsFromAgentConfig(catalog *componentCatalog, cfg config.Component) ComponentSettings {
	var settings ComponentSettings
	if cfg == nil {
		return settings
	}
	settings.Enabled = make(map[string]bool, len(catalog.entries))
	for _, entry := range catalog.Entries() {
		prefix := "anomaly_detection.detectors." + entry.name + "."
		if cfg.IsKnown(prefix + "enabled") {
			settings.Enabled[entry.name] = cfg.GetBool(prefix + "enabled")
		}
		if entry.readConfig != nil {
			if settings.configs == nil {
				settings.configs = make(map[string]any)
			}
			settings.configs[entry.name] = entry.readConfig(cfg, prefix)
		}
	}
	return settings
}

// disabledObserver is the zero-overhead stub returned when config is absent.
// It allocates nothing and starts no goroutines.
type disabledObserver struct{}

func (*disabledObserver) GetHandle(_ string) observerdef.Handle { return &noopObserveHandle{} }
func (*disabledObserver) DumpMetrics(_ string) error            { return nil }

// NewComponent creates an observer.Component.
func NewComponent(deps Requires) Provides {
	cfg := deps.Config
	if cfg == nil {
		return Provides{Comp: &disabledObserver{}}
	}

	catalog := defaultCatalog()
	settings := settingsFromAgentConfig(catalog, cfg)
	detectors, correlators, extractors, _ := catalog.Instantiate(settings)

	storageCfg := defaultStorageConfig()
	if cfg != nil {
		if v := cfg.GetInt("anomaly_detection.storage.max_series"); v >= 0 {
			storageCfg.MaxSeries = v
		}
		if v := cfg.GetFloat64("anomaly_detection.storage.eviction_floor_ratio"); v > 0 {
			storageCfg.EvictionFloorRatio = v
		}
		if v := cfg.GetInt64("anomaly_detection.storage.point_retention_secs"); v >= 0 {
			storageCfg.PointRetentionSecs = v
		}
	}
	eng := newEngine(engineConfig{
		storage:     newTimeSeriesStorageWith(storageCfg),
		extractors:  extractors,
		detectors:   detectors,
		correlators: correlators,
		scheduler:   &currentBehaviorPolicy{},
	})

	// Wire each injected reporter into its own reporterEventSink subscription.
	// StorageConsumer reporters receive engine storage for windowed log-rate annotations.
	for _, r := range deps.Reporters {
		r := r
		if sc, ok := r.(reporterdef.StorageConsumer); ok {
			sc.SetStorage(eng.Storage())
		}
		eng.Subscribe(&reporterEventSink{
			reporters: []reporterdef.Reporter{r},
			state:     eng.StateView(),
		})
	}

	telemetryComp := deps.Telemetry
	if telemetryComp == nil {
		telemetryComp = noopsimpl.GetCompatComponent()
	}

	th := newTelemetryHandler(telemetryComp)

	// Wire direct gauge.Set for processing-time telemetry to avoid per-log
	// ObserverTelemetry struct allocations on the hot path.
	processingTimeGauge := th.telemetryGauges[telemetryDetectorProcessingTimeNs]
	eng.onProcessingTime = func(detectorTag string, nanos float64) {
		processingTimeGauge.Set(nanos, detectorTag)
	}

	obs := &observerImpl{
		engine:               eng,
		catalog:              catalog,
		obsCh:                make(chan observation, 1000),
		telemetryHandler:     th,
		dropCounter:          th.telemetryCounters[telemetryObsChannelDropped],
		hfFilterSources:      make(map[metrics.MetricSource]struct{}),
		ingestMetricsEnabled: cfg.GetBool("anomaly_detection.metrics.enabled"),
	}

	if !obs.ingestMetricsEnabled {
		pkglog.Warn("[observer] anomaly_detection.metrics.enabled=false: externally-ingested metrics will be dropped at the handle factory")
	}

	// Set up handle function based on recording and analysis configuration.
	// Recording (anomaly_detection.recording.enabled) enables parquet writers.
	// Analysis (anomaly_detection.enabled) enables the anomaly detection pipeline.
	analysisEnabled := cfg.GetBool("anomaly_detection.enabled")

	obs.handleFunc = obs.noopHandle
	if analysisEnabled {
		obs.handleFunc = obs.innerHandle
	}

	recorder, recorderEnabled := deps.Recorder.Get()
	if recorderEnabled {
		obs.handleFunc = recorder.GetHandle(obs.handleFunc)

		// Record detect digests and advance log alongside parquet for parity debugging.
		parquetDir := cfg.GetString("anomaly_detection.recording.output_dir")
		if parquetDir != "" {
			digestPath := filepath.Join(parquetDir, detectDigestFileName)
			cleanup, err := enableDetectDigestRecordingToFile(eng, digestPath)
			if err != nil {
				deps.Log.Warnf("[observer] detect digest recording disabled: %v", err)
			} else {
				obs.digestCleanup = cleanup
			}

			advPath := filepath.Join(parquetDir, advanceLogFileName)
			advRec, err := newAdvanceLogRecorder(advPath)
			if err != nil {
				deps.Log.Warnf("[observer] advance log recording disabled: %v", err)
			} else {
				eng.onAdvance = advRec.record
				obs.advanceLogCleanup = func() {
					eng.onAdvance = nil
					_ = advRec.close()
				}
			}
		}
	}

	go obs.run()

	// Start high-frequency check runners. The hfrunner component handles
	// config-based toggling and lifecycle internally; we only collect the
	// MetricSource sets it wants suppressed from the "all-metrics" pipeline.
	for src := range deps.HFRunner.StartSystem(obs.GetHandle(hfrunnerdef.HFSource)) {
		obs.hfFilterSources[src] = struct{}{}
	}
	for src := range deps.HFRunner.StartContainer(obs.GetHandle(hfrunnerdef.HFContainerSource)) {
		obs.hfFilterSources[src] = struct{}{}
	}

	// Wire agent-internal logs into the observer via the pkg/util/log tap.
	// anomaly_detection.logs.enabled is the parent gate; without it,
	// agent_logs are also disabled. anomaly_detection.agent_logs.enabled
	// defaults to true when unset (explicit false disables it).
	logsEnabled := !cfg.IsConfigured("anomaly_detection.logs.enabled") || cfg.GetBool("anomaly_detection.logs.enabled")
	agentLogsEnabled := !cfg.IsConfigured("anomaly_detection.agent_logs.enabled") || cfg.GetBool("anomaly_detection.agent_logs.enabled")
	if (analysisEnabled || recorderEnabled) && logsEnabled && agentLogsEnabled {
		sampleInfo := cfg.GetFloat64("anomaly_detection.agent_logs.sample_rate_info")
		sampleDebug := cfg.GetFloat64("anomaly_detection.agent_logs.sample_rate_debug")
		sampleTrace := cfg.GetFloat64("anomaly_detection.agent_logs.sample_rate_trace")
		agentLogsHandle := obs.GetHandle("agent-internal-logs")
		installAgentLogTap(agentLogsHandle, sampleInfo, sampleDebug, sampleTrace)
		deps.Lifecycle.Append(compdef.Hook{
			OnStop: func(_ context.Context) error {
				pkglog.SetLogObserver(nil)
				return nil
			},
		})
	}

	// Start periodic metric dump if configured
	dumpPath := cfg.GetString("anomaly_detection.debug.dump_path")
	dumpInterval := cfg.GetDuration("anomaly_detection.debug.dump_interval")
	if dumpPath != "" && dumpInterval > 0 {
		go func() {
			ticker := time.NewTicker(dumpInterval)
			defer ticker.Stop()
			for range ticker.C {
				if err := obs.DumpMetrics(dumpPath); err != nil {
					fmt.Fprintf(os.Stderr, "[observer] dump error: %v\n", err)
				} else {
					fmt.Printf("[observer] dumped metrics to %s\n", dumpPath)
				}
			}
		}()
	}

	return Provides{Comp: obs}
}

// observerImpl is the implementation of the observer component.
// It is a thin driver around the engine, which holds storage, extractors,
// detectors, correlators, and raw anomaly tracking.
type observerImpl struct {
	engine     *engine
	catalog    *componentCatalog
	obsCh      chan observation
	handleFunc observerdef.HandleFunc // Handle factory (may wrap with recorder middleware)

	telemetryHandler  *telemetryHandler
	digestCleanup     func() // flushes detect digest recording file
	advanceLogCleanup func() // flushes advance log recording file

	// dropCounter counts observations silently dropped when the channel is full.
	// Tagged by source for Prometheus visibility. Complements engine.droppedObs
	// which tracks drops for live/replay parity analysis.
	dropCounter telemetry.Counter

	// hfFilterSources is the combined set of MetricSource values to suppress from
	// the "all-metrics" pipeline when their HF counterpart is active. Populated at
	// construction time from the MetricSource sets returned by hfrunner.StartSystem
	// and hfrunner.StartContainer.
	hfFilterSources map[metrics.MetricSource]struct{}

	// ingestMetricsEnabled gates externally-ingested metrics at the handle
	// factory. When false, "all-metrics" and HF handles return a wrapper
	// that drops ObserveMetric calls. Logs and profiles still pass through,
	// and log-derived virtual metrics produced inside the engine by
	// LogMetricsExtractors are unaffected because they bypass the handle.
	ingestMetricsEnabled bool

	// replayMu serialises engine access between the run() dispatch loop and
	// the testbench's IngestLogSync/IngestMetricSync direct-ingest path.
	// In production the sync methods are never called so this mutex is always
	// uncontended. In the testbench it prevents a data race between the
	// agent-internal-log observer (which can post to obsCh while run() is
	// processing) and a concurrent IngestLogSync call.
	replayMu sync.Mutex
}

// run is the main dispatch loop, processing all observations sequentially.
func (o *observerImpl) run() {
	for obs := range o.obsCh {
		if obs.flush != nil {
			close(obs.flush)
			continue
		}
		o.replayMu.Lock()
		var requests []advanceRequest
		if obs.metric != nil {
			requests = o.engine.IngestMetric(obs.source, obs.metric)
		}
		if obs.log != nil {
			logRequests, logTelemetry := o.engine.IngestLog(obs.source, obs.log)
			requests = append(requests, logRequests...)
			if len(logTelemetry) > 0 {
				o.telemetryHandler.handleTelemetry(logTelemetry)
			}
		}
		for _, req := range requests {
			result := o.engine.advanceWithReason(req.upToSec, req.reason)
			o.telemetryHandler.handleTelemetry(result.telemetry)
		}
		o.replayMu.Unlock()
	}
}

// defaultDetectorWindowSec is the default window (in seconds) that limits how
// far back seriesDetectorAdapter reads when running detection. 300s = 5 minutes.
const defaultDetectorWindowSec = 300

// defaultAggregations is the standard set of aggregations used when adapting
// a SeriesDetector into a Detector.
var defaultAggregations = []observerdef.Aggregate{
	observerdef.AggregateAverage,
	observerdef.AggregateCount,
}

// seriesDetectorAdapter wraps a stateless SeriesDetector to implement Detector.
// It runs the wrapped detector on all series, handling aggregation suffixes.
//
// The adapter tracks the visible point count per series so it can skip
// re-running the detector when no new data has arrived. This keeps the
// stateless detector path simple and correct even as storage internals evolve.
type seriesDetectorAdapter struct {
	detector     observerdef.SeriesDetector
	aggregations []observerdef.Aggregate

	// windowSec limits how far back GetSeriesRange reads. 0 means unbounded
	// (read from timestamp 0). A positive value reads [dataTime-windowSec, dataTime],
	// bounding per-call cost to O(windowSec) instead of O(totalPoints).
	windowSec int64

	// cachedSeries / cachedGen mirror the pattern used by BOCPDDetector,
	// ScanWelchDetector, and ScanMWDetector: storage.SeriesGeneration() only
	// advances when a brand-new series key is created, so we can avoid the
	// per-Detect full-map ListSeries scan on steady-state cardinality.
	cachedSeries []observerdef.SeriesMeta
	cachedGen    uint64

	// lastVisibleCount is keyed by the storage's compact SeriesRef so we
	// avoid rebuilding a string key per series per Detect call. SeriesRefs
	// are append-only (storage.go:217) so they remain stable for the lifetime
	// of a series.
	lastVisibleCount map[observerdef.SeriesRef]int
}

func newSeriesDetectorAdapter(detector observerdef.SeriesDetector, aggregations []observerdef.Aggregate) *seriesDetectorAdapter {
	return &seriesDetectorAdapter{
		detector:         detector,
		aggregations:     aggregations,
		windowSec:        defaultDetectorWindowSec,
		lastVisibleCount: make(map[observerdef.SeriesRef]int),
	}
}

func (a *seriesDetectorAdapter) Name() string {
	return a.detector.Name()
}

// Reset clears adapter-local caches and resets the wrapped detector when supported.
func (a *seriesDetectorAdapter) Reset() {
	a.lastVisibleCount = make(map[observerdef.SeriesRef]int)
	a.cachedSeries = nil
	a.cachedGen = 0
	if resetter, ok := a.detector.(interface{ Reset() }); ok {
		resetter.Reset()
	}
}

// RemoveSeries drops adapter-local point-count tracking for the given refs
// and forwards the call to the wrapped detector if it also tracks per-series
// state. Without this hook lastVisibleCount grows with the cumulative number
// of series ever observed even after storage frees them.
//
// Concurrency invariant: this method runs on the single observerImpl.run()
// goroutine that drives every other adapter callback (Detect, Reset). The
// engine's fanOutSeriesRemoval is the only caller. Mutating lastVisibleCount
// and cachedSeries without a lock is safe under that invariant only.
func (a *seriesDetectorAdapter) RemoveSeries(refs []observerdef.SeriesRef) {
	if len(refs) == 0 {
		return
	}
	if len(a.lastVisibleCount) > 0 {
		for _, ref := range refs {
			delete(a.lastVisibleCount, ref)
		}
	}
	a.cachedSeries = nil
	a.cachedGen = 0
	if remover, ok := a.detector.(observerdef.SeriesRemover); ok {
		remover.RemoveSeries(refs)
	}
}

func (a *seriesDetectorAdapter) Detect(storage observerdef.StorageReader, dataTime int64) observerdef.DetectionResult {
	gen := storage.SeriesGeneration()
	if a.cachedSeries == nil || gen != a.cachedGen {
		a.cachedSeries = storage.ListSeries(observerdef.WorkloadSeriesFilter())
		a.cachedGen = gen
	}

	var allAnomalies []observerdef.Anomaly
	var allTelemetry []observerdef.ObserverTelemetry

	for _, meta := range a.cachedSeries {
		visibleCount := storage.PointCountUpTo(meta.Ref, dataTime)
		if prev, ok := a.lastVisibleCount[meta.Ref]; ok && prev == visibleCount {
			continue
		}
		a.lastVisibleCount[meta.Ref] = visibleCount

		for _, agg := range a.aggregations {
			start := int64(0)
			if a.windowSec > 0 {
				start = dataTime - a.windowSec
			}
			series := storage.GetSeriesRange(meta.Ref, start, dataTime, agg)
			if series == nil || len(series.Points) == 0 {
				continue
			}

			seriesWithAgg := *series
			seriesWithAgg.Name = series.Name + ":" + aggSuffix(agg)

			result := a.detector.Detect(seriesWithAgg)
			for j := range result.Anomalies {
				result.Anomalies[j].Type = observerdef.AnomalyTypeMetric
				result.Anomalies[j].DetectorName = a.detector.Name()
				result.Anomalies[j].Source = observerdef.SeriesDescriptor{
					Namespace: series.Namespace,
					Name:      series.Name,
					Tags:      series.Tags,
					Aggregate: agg,
				}
				result.Anomalies[j].SourceRef = &observerdef.QueryHandle{
					Ref:       meta.Ref,
					Aggregate: agg,
				}
			}
			allAnomalies = append(allAnomalies, result.Anomalies...)
			allTelemetry = append(allTelemetry, result.Telemetry...)
		}
	}

	return observerdef.DetectionResult{Anomalies: allAnomalies, Telemetry: allTelemetry}
}

// aggSuffix returns a short suffix for the given aggregation type.
func aggSuffix(agg observerdef.Aggregate) string {
	return observerdef.AggregateString(agg)
}

// RawAnomalies returns a copy of currently tracked raw anomalies.
func (o *observerImpl) RawAnomalies() []observerdef.Anomaly {
	return o.engine.RawAnomalies()
}

// TotalAnomalyCount returns the total number of anomalies ever detected (no cap).
func (o *observerImpl) TotalAnomalyCount() int {
	return o.engine.TotalAnomalyCount()
}

// UniqueAnomalySourceCount returns the number of unique sources that had anomalies.
func (o *observerImpl) UniqueAnomalySourceCount() int {
	return o.engine.UniqueAnomalySourceCount()
}

// GetHandle returns a lightweight handle for a named source.
// If a recorder is configured, the handle will be wrapped to record metrics.
func (o *observerImpl) GetHandle(name string) observerdef.Handle {
	pkglog.Infof("[observer] getting handle for %s", name)
	return o.handleFunc(name)
}

// innerHandle creates the base handle without any middleware wrapping.
// When any HF check collection is enabled, the "all-metrics" handle is wrapped
// with hfFilteredHandle to suppress 15s samples for checks that have a 1s HF
// counterpart active — the scorer should only see the higher-resolution stream.
// When anomaly_detection.metrics.enabled=false, the resulting handle is further
// wrapped with metricDropHandle so external metrics are dropped at the edge,
// while ObserveLog/ObserveProfile pass through.
func (o *observerImpl) innerHandle(name string) observerdef.Handle {
	h := &handle{ch: o.obsCh, source: name, dropCounter: o.dropCounter}
	o.engine.registerHandle(h)
	var out observerdef.Handle = h
	if len(o.hfFilterSources) > 0 && name == "all-metrics" {
		out = &hfFilteredHandle{inner: h, sources: o.hfFilterSources}
	}
	if !o.ingestMetricsEnabled {
		out = &metricDropHandle{inner: out}
	}
	return out
}

// sourceProvider is a structural interface satisfied by *metrics.MetricSample,
// which carries a MetricSource enum populated by the standard check sender.
// Using a type assertion (rather than adding GetSource to MetricView) avoids
// importing pkg/metrics into comp/anomalydetection/observer/def.
type sourceProvider interface {
	GetSource() metrics.MetricSource
}

// hfFilteredHandle wraps a Handle and drops metrics whose source is in the
// provided sources set, so that 15s pipeline samples do not compete with their
// 1s HF counterparts in the scorer.
//
// Filtering uses a MetricSource enum map lookup via a type assertion to
// sourceProvider. Samples that do not implement sourceProvider pass through
// unchanged — absence of metadata is not sufficient grounds to drop.
type hfFilteredHandle struct {
	inner   observerdef.Handle
	sources map[metrics.MetricSource]struct{}
}

func (f *hfFilteredHandle) ObserveMetric(sample observerdef.MetricView) {
	_ = f.ObserveMetricAndReportDrop(sample)
}

func (f *hfFilteredHandle) ObserveMetricAndReportDrop(sample observerdef.MetricView) bool {
	if sp, ok := sample.(sourceProvider); ok {
		if _, suppressed := f.sources[sp.GetSource()]; suppressed {
			return false
		}
	}
	if dr, ok := f.inner.(interface {
		ObserveMetricAndReportDrop(observerdef.MetricView) bool
	}); ok {
		return dr.ObserveMetricAndReportDrop(sample)
	}
	f.inner.ObserveMetric(sample)
	return false
}

func (f *hfFilteredHandle) ObserveLog(msg observerdef.LogView) { f.inner.ObserveLog(msg) }

// metricDropHandle drops every ObserveMetric call but lets logs and
// profiles through. Used when anomaly_detection.metrics.enabled=false so
// external metric sources (DogStatsD, check samplers, HF runners) do not
// feed the engine. Virtual metrics produced by LogMetricsExtractors
// during engine.IngestLog are unaffected because they bypass this handle
// path entirely (they are written directly to storage from the engine).
type metricDropHandle struct{ inner observerdef.Handle }

var _ observerdef.Handle = (*metricDropHandle)(nil)

func (m *metricDropHandle) ObserveMetric(_ observerdef.MetricView) {}
func (m *metricDropHandle) ObserveMetricAndReportDrop(_ observerdef.MetricView) bool {
	return true
}
func (m *metricDropHandle) ObserveLog(msg observerdef.LogView) { m.inner.ObserveLog(msg) }

// noopHandle returns a handle that discards all observations.
// Used when analysis is disabled so the analysis pipeline is not started.
func (o *observerImpl) noopHandle(_ string) observerdef.Handle {
	return &noopObserveHandle{}
}

// noopObserveHandle discards all observations.
type noopObserveHandle struct{}

func (h *noopObserveHandle) ObserveMetric(_ observerdef.MetricView) {}
func (h *noopObserveHandle) ObserveMetricAndReportDrop(_ observerdef.MetricView) bool {
	return false
}
func (h *noopObserveHandle) ObserveLog(_ observerdef.LogView) {}

// DumpMetrics writes all stored metrics to the specified file as JSON.
func (o *observerImpl) DumpMetrics(path string) error {
	// For simplicity, just dump directly (storage access is single-threaded from run loop,
	// but this is a debug tool so approximate snapshot is fine)
	return o.engine.Storage().DumpToFile(path)
}

// --- DebugView implementation ---

// StateView returns a read-only window into engine state.
// Implements DebugView.
func (o *observerImpl) StateView() StateView {
	return o.engine.StateView()
}

// CatalogEntries returns the list of all registered components with their metadata.
// Implements DebugView.
func (o *observerImpl) CatalogEntries() []CatalogEntry {
	entries := o.catalog.Entries()
	result := make([]CatalogEntry, len(entries))
	for i, e := range entries {
		result[i] = CatalogEntry{
			Name:           e.name,
			DisplayName:    e.displayName,
			Kind:           kindString(e.kind),
			DefaultEnabled: e.defaultEnabled,
		}
	}
	return result
}

// Flush blocks until all observations currently queued in the dispatch channel
// have been processed by the engine. Implements DebugView.
func (o *observerImpl) Flush() {
	done := make(chan struct{})
	o.obsCh <- observation{flush: done}
	<-done
}

// Reset clears all engine state and reconfigures with new settings. Implements DebugView.
func (o *observerImpl) Reset(settings ComponentSettings) {
	o.Flush()
	detectors, correlators, extractors, _ := o.catalog.Instantiate(settings)
	o.replayMu.Lock()
	o.engine.ResetForReplay(detectors, correlators, extractors)
	o.replayMu.Unlock()
}

// GetReplayProgress returns lock-free replay progress counters. Implements DebugView.
func (o *observerImpl) GetReplayProgress() ReplayProgress {
	return o.engine.GetReplayProgress()
}

// SetReplayPhase updates the replay phase string. Implements DebugView.
func (o *observerImpl) SetReplayPhase(phase string) {
	o.engine.SetReplayPhase(phase)
}

// ExtractorCount returns the number of extractors active in the engine. Implements DebugView.
func (o *observerImpl) ExtractorCount() int {
	return o.engine.ExtractorCount()
}

// AddTelemetry writes a data point into the telemetry namespace. Implements DebugView.
func (o *observerImpl) AddTelemetry(name string, value float64, timestamp int64, tags []string) {
	_ = o.engine.storage.Add(observerdef.TelemetryNamespace, name, value, timestamp, tags)
}

// ReplayStoredData resets analysis state (preserving extractor context) then
// replays all stored data through the scheduler in chronological order.
// Implements DebugView.
func (o *observerImpl) ReplayStoredData() {
	// resetAnalysisState resets detectors/correlators and tracking state but
	// preserves extractor state (contextRefs + provider pattern registry) so
	// enrichAnomaly can still attach log pattern context during replay.
	o.replayMu.Lock()
	o.engine.resetAnalysisState()
	o.engine.ReplayStoredData()
	o.replayMu.Unlock()
}

// StorageReader returns a read-only view of the engine's time-series storage.
// Used by the testbench to compute windowed log rates in change messages.
// Implements DebugView.
func (o *observerImpl) StorageReader() observerdef.StorageReader {
	return o.engine.storage
}

// IngestLogSync feeds a log directly into the engine, bypassing the dispatch
// channel. It replicates what the dispatcher run() loop does for a log
// observation: build logObs, call engine.IngestLog, drive any advance
// requests, and forward telemetry. Implements DebugView.
func (o *observerImpl) IngestLogSync(source string, msg observerdef.LogView) {
	timestampMs := msg.GetTimestampUnixMilli()
	lo := &logObs{
		content:     msg.GetContent(),
		status:      msg.GetStatus(),
		tags:        copyTags(msg.Tags()),
		hostname:    msg.GetHostname(),
		timestampMs: timestampMs,
	}
	o.replayMu.Lock()
	requests, logTelemetry := o.engine.IngestLog(source, lo)
	if len(logTelemetry) > 0 {
		o.telemetryHandler.handleTelemetry(logTelemetry)
	}
	for _, req := range requests {
		result := o.engine.advanceWithReason(req.upToSec, req.reason)
		o.telemetryHandler.handleTelemetry(result.telemetry)
	}
	o.replayMu.Unlock()
}

// IngestMetricSync feeds a metric directly into the engine, bypassing the
// dispatch channel. Mirrors the handle.ObserveMetricAndReportDrop path without
// the non-blocking channel send. Implements DebugView.
func (o *observerImpl) IngestMetricSync(source string, sample observerdef.MetricView) {
	name := sample.GetName()
	if strings.HasPrefix(name, "datadog.") {
		return
	}
	timestamp := sample.GetTimestampUnix()
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}
	mo := &metricObs{
		name:      name,
		value:     sample.GetValue(),
		tags:      copyTags(sample.GetRawTags()),
		timestamp: timestamp,
	}
	o.replayMu.Lock()
	requests := o.engine.IngestMetric(source, mo)
	for _, req := range requests {
		result := o.engine.advanceWithReason(req.upToSec, req.reason)
		o.telemetryHandler.handleTelemetry(result.telemetry)
	}
	o.replayMu.Unlock()
}

// handle is the lightweight observation interface passed to other components.
// It only holds a channel and source name - all processing happens in the observer.
type handle struct {
	ch          chan<- observation
	source      string
	dropCount   atomic.Int64      // per-handle drop counter, collected by engine at advance time
	dropCounter telemetry.Counter // tagged by source for Prometheus visibility; may be nil
}

// ObserveMetric observes a DogStatsD metric sample.
func (h *handle) ObserveMetric(sample observerdef.MetricView) {
	_ = h.ObserveMetricAndReportDrop(sample)
}

// ObserveMetricAndReportDrop observes a metric and reports whether this
// specific call was dropped by the observer channel.
func (h *handle) ObserveMetricAndReportDrop(sample observerdef.MetricView) bool {
	timestamp := sample.GetTimestampUnix()
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}

	name := sample.GetName()

	// filter internal Datadog Agent telemetry
	if strings.HasPrefix(name, "datadog.") {
		return false
	}

	obs := observation{
		source: h.source,
		metric: &metricObs{
			name:      name,
			value:     sample.GetValue(),
			tags:      copyTags(sample.GetRawTags()),
			timestamp: timestamp,
		},
	}

	// Non-blocking send - drop if channel is full.
	select {
	case h.ch <- obs:
		return false
	default:
		h.dropCount.Add(1)
		if h.dropCounter != nil {
			h.dropCounter.Add(1, h.source)
		}
		return true
	}
}

// ObserveLog observes a log message.
func (h *handle) ObserveLog(msg observerdef.LogView) {
	// Use provided timestampMs if available, otherwise use current time
	timestampMs := msg.GetTimestampUnixMilli()

	obs := observation{
		source: h.source,
		log: &logObs{
			content:     msg.GetContent(),
			status:      msg.GetStatus(),
			tags:        copyTags(msg.Tags()),
			hostname:    msg.GetHostname(),
			timestampMs: timestampMs,
		},
	}

	// Non-blocking send - drop if channel is full.
	select {
	case h.ch <- obs:
	default:
		h.dropCount.Add(1)
		if h.dropCounter != nil {
			h.dropCounter.Add(1, h.source)
		}
	}
}

// logView wraps logObs to implement LogView interface.
type logView struct {
	obs *logObs
}

func (v *logView) GetContent() string           { return v.obs.content }
func (v *logView) GetStatus() string            { return v.obs.status }
func (v *logView) Tags() []string               { return v.obs.tags }
func (v *logView) GetHostname() string          { return v.obs.hostname }
func (v *logView) GetTimestampUnixMilli() int64 { return v.obs.timestampMs }

// copyBytes creates a copy of a byte slice.
func copyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	result := make([]byte, len(b))
	copy(result, b)
	return result
}
