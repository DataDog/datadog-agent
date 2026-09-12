// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerimpl implements the observer component.
package observerimpl

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	compdef "github.com/DataDog/datadog-agent/comp/def"

	anomalydetectionconfig "github.com/DataDog/datadog-agent/comp/anomalydetection/config"
	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logging"
	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logsfilter"
	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	reporterdef "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/def"
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires declares the input types to the observer component constructor.
type Requires struct {
	Lifecycle compdef.Lifecycle
	Config    config.Component
	Telemetry telemetry.Component

	// Recorder is an optional component for transparent metric recording.
	// If provided, all handles will be wrapped to record metrics to parquet files.
	Recorder option.Option[recorderdef.Component]

	// Reporters are provided by reporter/fx, reporter/fx-testbench, etc. via the
	// `anomalydetection_reporters` Fx group. Each reporter gets its own subscription
	// so it receives advance events independently. StorageConsumer reporters receive
	// storage for windowed log-rate annotations.
	Reporters []reporterdef.Reporter `group:"anomalydetection_reporters"`
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

// GetTimestampUnixMilli implements observerdef.LogView.
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
		// The anomaly_scorer has its own dedicated config prefix outside detectors.*.
		if entry.name == "anomaly_scorer" {
			continue
		}
		prefix := "anomaly_detection.detectors." + entry.name + "."
		if cfg.IsConfigured(prefix + "enabled") {
			settings.Enabled[entry.name] = cfg.GetBool(prefix + "enabled")
		}
		if entry.readConfig != nil {
			if settings.configs == nil {
				settings.configs = make(map[string]any)
			}
			settings.configs[entry.name] = entry.readConfig(cfg, prefix)
		}
	}

	// Dedicated scorer read path under anomaly_detection.anomaly_scorer.*
	const scorerPrefix = "anomaly_detection.anomaly_scorer."
	if anomalydetectionconfig.ScorerRequired(cfg) {
		settings.Enabled["anomaly_scorer"] = true
		if settings.configs == nil {
			settings.configs = make(map[string]any)
		}
		scorerCfg := readAnomalyScorerConfig(cfg, scorerPrefix)
		if anomalydetectionconfig.AnomalyScorerDryRunEnabled(cfg) {
			scorerCfg.CorrelationEvents = false
		}
		settings.configs["anomaly_scorer"] = scorerCfg
	}

	settings.Baseline = DefaultBaselineConfig()
	const basePrefix = "anomaly_detection.baseline_analysis."
	if cfg.IsConfigured(basePrefix + "enabled") {
		settings.Baseline.Enabled = cfg.GetBool(basePrefix + "enabled")
	}
	if cfg.IsConfigured(basePrefix + "mute_noisy_metrics") {
		settings.Baseline.MuteNoisyMetrics = cfg.GetBool(basePrefix + "mute_noisy_metrics")
	}
	if cfg.IsConfigured(basePrefix + "duration") {
		settings.Baseline.DurationSec = int64(cfg.GetDuration(basePrefix + "duration").Seconds())
	}
	if cfg.IsConfigured(basePrefix + "verbose") {
		settings.Baseline.Verbose = cfg.GetBool(basePrefix + "verbose")
	}

	return settings
}

func logCountBucketConfigFromAgent(cfg config.Component) LogCountBucketConfig {
	config := DefaultLogCountBucketConfig()
	if cfg == nil {
		return config
	}
	config.Enabled = cfg.GetBool("anomaly_detection.logs.time_buckets.enabled")
	if width := cfg.GetDuration("anomaly_detection.logs.time_buckets.bucket_width"); width > 0 {
		config.BucketSeconds = int64(width.Seconds())
	} else if config.Enabled {
		logging.Warnf("anomaly_detection.logs.time_buckets.bucket_width must be > 0, got %s; using 5s", width)
	}
	if ttl := cfg.GetDuration("anomaly_detection.logs.time_buckets.idle_ttl"); ttl >= 0 {
		config.IdleTTLSeconds = int64(ttl.Seconds())
	} else if config.Enabled {
		logging.Warnf("anomaly_detection.logs.time_buckets.idle_ttl must be >= 0, got %s; using 5m", ttl)
	}
	if retention := cfg.GetDuration("anomaly_detection.logs.time_buckets.retention"); retention >= 0 {
		config.RetentionSeconds = int64(retention.Seconds())
	} else if config.Enabled {
		logging.Warnf("anomaly_detection.logs.time_buckets.retention must be >= 0, got %s; using 10m", retention)
	}
	return config
}

// disabledObserver is the zero-overhead stub returned when config is absent.
// It allocates nothing and starts no goroutines.
type disabledObserver struct{}

func (*disabledObserver) GetHandle(_ string) observerdef.Handle { return &noopObserveHandle{} }
func (*disabledObserver) RecordSamplerDropped(_, _ string)      {}
func (*disabledObserver) DumpMetrics(_ string) error            { return nil }

func (*disabledObserver) SubscribeSeverityEvents(_ severityeventsdef.SeverityEventsConfiguration, _ severityeventsdef.SeverityEventListener) (severityeventsdef.SeverityEventsSubscription, error) {
	return severityeventsdef.SeverityEventsSubscription{}, errors.New("no active anomaly scorer")
}

func (*disabledObserver) SubscribeSeverityEventsReader(_ severityeventsdef.SeverityEventsConfiguration) (severityeventsdef.SeverityEventsReaderSubscription, error) {
	return severityeventsdef.SeverityEventsReaderSubscription{}, errors.New("no active anomaly scorer")
}

// NewComponent creates an observer.Component.
func NewComponent(deps Requires) (Provides, error) {
	cfg := deps.Config
	if cfg == nil {
		return Provides{Comp: &disabledObserver{}}, nil
	}
	// Off-by-default fast path: when neither analysis nor recording is active the
	// live observer noops every handle (see handleFunc below) and installs no log
	// tap, so skip building the catalog, engine, storage, 1000-cap channel, and
	// dispatch goroutine — return the zero-allocation stub instead. The predicate
	// mirrors the observerRequired/recorderEnabled gates used further down.
	if !anomalydetectionconfig.ObserverRequired(cfg) {
		if _, recorderEnabled := deps.Recorder.Get(); !recorderEnabled {
			return Provides{Comp: &disabledObserver{}}, nil
		}
	}

	catalog := defaultCatalog()
	settings := settingsFromAgentConfig(catalog, cfg)
	detectors, correlators, rawScorer, extractors, _ := catalog.Instantiate(settings)

	storageCfg := storageConfigFromAgentConfig(cfg, detectors)

	compiledMetricFilter, err := loadMetricFilter(cfg)
	if err != nil {
		return Provides{}, fmt.Errorf("%s: %w", metricProcessingRulesConfigKey, err)
	}

	obsTelemetry := newObserverTelemetry(deps.Telemetry)

	// Upgrade the raw scorer (no telemetry) to one with gauges. The catalog
	// returns a plain *anomalyScorer; here we reconstruct it with the watcher
	// enabled so the live observer gets full telemetry. Replay enables the same
	// watcher with nil gauges when settings are reset below.
	var scorer *anomalyScorer
	if rawScorer != nil {
		scorer = newAnomalyScorerWithTelemetry(rawScorer.config, obsTelemetry.scorerSeverity, obsTelemetry.scorerEwma)
		logging.Infof("anomaly scorer registered (logs=%v, correlation_events=%v, cooldown=%ds)",
			scorer.config.Logs, scorer.config.CorrelationEvents, scorer.config.CooldownSecs)
	}

	eng := newEngine(engineConfig{
		storage:         newTimeSeriesStorageWith(storageCfg),
		extractors:      extractors,
		detectors:       detectors,
		correlators:     correlators,
		scorer:          scorer,
		scheduler:       &currentBehaviorPolicy{},
		baseline:        settings.Baseline,
		logCountBuckets: logCountBucketConfigFromAgent(cfg),
	})

	eng.onStorageSeriesEvicted = obsTelemetry.recordStorageSeriesEvicted
	eng.onStorageCapacityHit = obsTelemetry.recordStorageCapacityHit
	eng.onAnomalyDedupEvicted = obsTelemetry.recordAnomalyDedupEvicted
	eng.onAdvanceSkipped = obsTelemetry.recordAdvanceSkipped
	eng.onProcessingTime = obsTelemetry.recordProcessingTime
	eng.onDetectorEmission = obsTelemetry.recordDetectorEmission
	for _, extractor := range extractors {
		if sinkAware, ok := extractor.(interface{ SetObserverTelemetry(*observerTelemetry) }); ok {
			sinkAware.SetObserverTelemetry(obsTelemetry)
		}
	}
	for _, detector := range detectors {
		if sinkAware, ok := detector.(interface{ SetObserverTelemetry(*observerTelemetry) }); ok {
			sinkAware.SetObserverTelemetry(obsTelemetry)
		}
	}

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

	obs := &observerImpl{
		engine:               eng,
		catalog:              catalog,
		obsCh:                make(chan observation, 1000),
		telemetry:            obsTelemetry,
		ingestMetricsEnabled: !cfg.IsConfigured("anomaly_detection.metrics.enabled") || cfg.GetBool("anomaly_detection.metrics.enabled"),
		metricFilter:         compiledMetricFilter,
	}

	// When baseline muting is enabled, subscribe a sink that publishes the mute
	// hash set to the shared filter on window end. Handles on all goroutines share
	// the same *metricsFilterRules pointer, so the atomic Store is visible to all.
	if settings.Baseline.Enabled && settings.Baseline.MuteNoisyMetrics {
		eng.Subscribe(&baselineEventSink{filter: compiledMetricFilter})
	}

	if !obs.ingestMetricsEnabled {
		logging.Warn("anomaly_detection.metrics.enabled=false: externally-ingested metrics will be dropped at the handle factory")
	}

	// Set up handle function based on recording and analysis configuration.
	// Recording enables parquet writers. ObserverRequired enables the live
	// anomaly-detection pipeline and its default metric/log ingestion paths.
	observerRequired := anomalydetectionconfig.ObserverRequired(cfg)
	if observerRequired {
		obsTelemetry.initLogsInFlight()
		obsTelemetry.setSeriesCount(0)
	}

	obs.handleFunc = obs.noopHandle
	if observerRequired {
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
				logging.Warnf("detect digest recording disabled: %v", err)
			} else {
				obs.digestCleanup = cleanup
			}

			advPath := filepath.Join(parquetDir, advanceLogFileName)
			advRec, err := newAdvanceLogRecorder(advPath)
			if err != nil {
				logging.Warnf("advance log recording disabled: %v", err)
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

	// Wire agent-internal logs into the observer via the pkg/util/log tap.
	// anomaly_detection.logs.enabled is the parent gate; without it,
	// internal logs are also disabled. anomaly_detection.logs.internal.enabled
	// defaults to true when unset (explicit false disables it).
	logsEnabled := !cfg.IsConfigured("anomaly_detection.logs.enabled") || cfg.GetBool("anomaly_detection.logs.enabled")
	agentLogsEnabled := !cfg.IsConfigured("anomaly_detection.logs.internal.enabled") || cfg.GetBool("anomaly_detection.logs.internal.enabled")

	const logsProcessingRulesKey = "anomaly_detection.logs.processing_rules"
	logsRules, err := logsfilter.LoadRules(cfg, logsProcessingRulesKey)
	if err != nil {
		logging.Warnf("%s: invalid rules, proceeding without log filtering: %v", logsProcessingRulesKey, err)
		logsRules = &logsfilter.Rules{}
	}

	if (observerRequired || recorderEnabled) && logsEnabled && agentLogsEnabled {
		minSeverity := cfg.GetString("anomaly_detection.logs.internal.min_severity")
		maxRateHigh := cfg.GetFloat64("anomaly_detection.logs.internal.max_rate_high_priority")
		maxRateMedium := cfg.GetFloat64("anomaly_detection.logs.internal.max_rate_medium_priority")
		maxRateLow := cfg.GetFloat64("anomaly_detection.logs.internal.max_rate_low_priority")
		logsfilter.WarnInvalidMinSeverity("anomaly_detection.logs.internal.min_severity", minSeverity)
		logsfilter.WarnRateLimitDiscrepancies("anomaly_detection.logs.internal", maxRateLow, maxRateMedium, maxRateHigh)
		agentLogsHandle := obs.GetHandle("agent_logs")
		agentLogTap := installAgentLogTap(agentLogsHandle, minSeverity, maxRateHigh, maxRateMedium, maxRateLow, func(priority string) {
			obsTelemetry.recordInputRateLimiterDropped("internal", priority)
		}, func(count uint64) {
			obsTelemetry.recordObservationsDropped("logs", "internal", count)
		}, logsRules)
		deps.Lifecycle.Append(compdef.Hook{
			OnStop: func(_ context.Context) error {
				agentLogTap.stop()
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
					logging.Errorf("dump error: %v", err)
				} else {
					logging.Debugf("dumped metrics to %s", dumpPath)
				}
			}
		}()
	}

	return Provides{Comp: obs}, nil
}

const (
	storagePointInterval = 15 * time.Second
	storageRetentionPad  = 16 * time.Second
)

func storageConfigFromAgentConfig(cfg config.Component, detectors []observerdef.Detector) StorageConfig {
	storageCfg := DefaultStorageConfig()
	maxPoints := maxDetectorPoints(detectors)
	requiredRetention := time.Duration(maxPoints)*storagePointInterval + storageRetentionPad
	storageCfg.MaxPointsPerSeries = maxPoints
	storageCfg.PointRetentionSecs = int64(requiredRetention.Seconds())

	if cfg == nil {
		return storageCfg
	}
	if cfg.IsConfigured("anomaly_detection.storage.max_series") {
		storageCfg.MaxSeries = cfg.GetInt("anomaly_detection.storage.max_series")
	}
	if cfg.IsConfigured("anomaly_detection.storage.eviction_floor_ratio") {
		storageCfg.EvictionFloorRatio = cfg.GetFloat64("anomaly_detection.storage.eviction_floor_ratio")
	}
	if cfg.IsConfigured("anomaly_detection.storage.point_retention") {
		configuredRetention := cfg.GetDuration("anomaly_detection.storage.point_retention")
		switch {
		case configuredRetention == 0:
			// Keep the detector-derived retention.
		case configuredRetention < 0:
			pkglog.Warnf("anomaly_detection.storage.point_retention must be >= 0, got %s; using %s derived from enabled detector windows", configuredRetention, requiredRetention)
		case configuredRetention < requiredRetention:
			pkglog.Warnf("anomaly_detection.storage.point_retention=%s is below the %s required by enabled detector windows; using %s", configuredRetention, requiredRetention, requiredRetention)
		default:
			storageCfg.PointRetentionSecs = int64(configuredRetention.Seconds())
		}
	}
	if cfg.IsConfigured("anomaly_detection.storage.inactive_series_ttl") {
		d := cfg.GetDuration("anomaly_detection.storage.inactive_series_ttl")
		if d < 0 {
			pkglog.Warnf("anomaly_detection.storage.inactive_series_ttl must be >= 0, got %s — using default", d)
		} else {
			storageCfg.InactiveSeriesTTLSeconds = int64(d.Seconds())
		}
	}
	if cfg.IsConfigured("anomaly_detection.storage.inactive_series_check_interval") {
		d := cfg.GetDuration("anomaly_detection.storage.inactive_series_check_interval")
		if d < 0 {
			pkglog.Warnf("anomaly_detection.storage.inactive_series_check_interval must be >= 0, got %s — using default", d)
		} else {
			storageCfg.InactiveSeriesCheckIntervalSeconds = int64(d.Seconds())
		}
	}
	return storageCfg
}

func maxDetectorPoints(detectors []observerdef.Detector) int {
	var maxPoints int
	for _, detector := range detectors {
		if requirement, ok := detector.(observerdef.DetectorPointWindowRequirement); ok {
			maxPoints = max(maxPoints, requirement.DetectorPointWindow().MaxPoints)
		}
	}
	return maxPoints
}

// observerImpl is the implementation of the observer component.
// It is a thin driver around the engine, which holds storage, extractors,
// detectors, correlators, and raw anomaly tracking.
type observerImpl struct {
	engine     *engine
	catalog    *componentCatalog
	obsCh      chan observation
	handleFunc observerdef.HandleFunc // Handle factory (may wrap with recorder middleware)

	telemetry         *observerTelemetry
	digestCleanup     func() // flushes detect digest recording file
	advanceLogCleanup func() // flushes advance log recording file

	// ingestMetricsEnabled gates externally-ingested metrics at the handle
	// factory. When false, handles return a metricDropHandle wrapper that drops
	// ObserveMetric calls. ObserveLog still passes through, and log-derived
	// virtual metrics produced inside the engine by LogMetricsExtractors are
	// unaffected because they bypass the handle.
	ingestMetricsEnabled bool
	metricFilter         *metricsFilterRules

	// replayMu serialises engine access between the run() dispatch loop and
	// the testbench's direct-ingest path (IngestLogForReplay, IngestMetricSync).
	// In production these methods are never called so this mutex is always
	// uncontended. In the testbench it prevents a data race between the
	// agent-internal-log observer (which can post to obsCh while run() is
	// processing) and a concurrent testbench ingest call.
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
			logRequests := o.engine.IngestLog(obs.source, obs.log)
			requests = append(requests, logRequests...)
			if o.telemetry != nil {
				o.telemetry.decrementLogsInFlight(classifyLogSource(obs.source, obs.log.tags))
			}
		}
		for _, req := range requests {
			_ = o.engine.advanceWithReason(req.upToSec, req.reason)
		}
		if len(requests) > 0 {
			o.publishSeriesCount()
		}
		o.replayMu.Unlock()
	}
}

// publishSeriesCount updates the series-count gauge after an analysis advance,
// when any storage capacity eviction triggered by that advance has completed.
func (o *observerImpl) publishSeriesCount() {
	if o.telemetry == nil {
		return
	}
	o.telemetry.setSeriesCount(o.engine.Storage().TotalSeriesCount())
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

	// cachedRefs / cachedGen mirror the pattern used by BOCPDDetector,
	// ScanWelchDetector, and ScanMWDetector: storage.SeriesGeneration() only
	// advances when a brand-new series key is created, so we can avoid the
	// per-Detect full-map series scan on steady-state cardinality.
	cachedRefs []observerdef.SeriesRef
	cachedGen  uint64

	// lastVisibleCount is keyed by the storage's compact SeriesRef so we
	// avoid rebuilding a string key per series per Detect call. SeriesRefs
	// are unique and never reused, so they remain stable for the lifetime of
	// a series.
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

func (a *seriesDetectorAdapter) Ready() bool {
	return a.detector.Ready()
}

// Reset clears adapter-local caches and resets the wrapped detector when supported.
func (a *seriesDetectorAdapter) Reset() {
	a.lastVisibleCount = make(map[observerdef.SeriesRef]int)
	a.cachedRefs = nil
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
// and cachedRefs without a lock is safe under that invariant only.
func (a *seriesDetectorAdapter) RemoveSeries(refs []observerdef.SeriesRef) {
	if len(refs) == 0 {
		return
	}
	if len(a.lastVisibleCount) > 0 {
		for _, ref := range refs {
			delete(a.lastVisibleCount, ref)
		}
	}
	a.cachedRefs = nil
	a.cachedGen = 0
	if remover, ok := a.detector.(observerdef.SeriesRemover); ok {
		remover.RemoveSeries(refs)
	}
}

func (a *seriesDetectorAdapter) Detect(storage observerdef.StorageReader, dataTime int64) observerdef.DetectionResult {
	gen := storage.SeriesGeneration()
	if a.cachedRefs == nil || gen != a.cachedGen {
		a.cachedRefs = workloadSeriesRefs(storage, a.cachedRefs)
		a.cachedGen = gen
	}

	var allAnomalies []observerdef.Anomaly

	for _, ref := range a.cachedRefs {
		visibleCount := storage.PointCountUpTo(ref, dataTime)
		if prev, ok := a.lastVisibleCount[ref]; ok && prev == visibleCount {
			continue
		}
		a.lastVisibleCount[ref] = visibleCount

		for _, agg := range a.aggregations {
			if !supportsSeriesAggregate(storage, ref, agg) {
				continue
			}
			start := int64(0)
			if a.windowSec > 0 {
				start = dataTime - a.windowSec
			}
			series := storage.GetSeriesRange(ref, start, dataTime, agg)
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
					Ref:       ref,
					Aggregate: agg,
				}
			}
			allAnomalies = append(allAnomalies, result.Anomalies...)
		}
	}

	return observerdef.DetectionResult{Anomalies: allAnomalies}
}

// aggSuffix returns a short suffix for the given aggregation type.
func aggSuffix(agg observerdef.Aggregate) string {
	return observerdef.AggregateString(agg)
}

// RawAnomalies returns replay/debug history when anomaly history is enabled.
// Live production mode returns an empty slice.
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
	logging.Infof("getting handle for %s", name)
	return o.handleFunc(name)
}

// innerHandle creates the base handle for a named source. When
// anomaly_detection.metrics.enabled=false, the handle is wrapped with
// metricDropHandle so external metrics are dropped at the edge, while
// ObserveLog calls still pass through.
func (o *observerImpl) innerHandle(name string) observerdef.Handle {
	h := &handle{ch: o.obsCh, source: name, telemetry: o.telemetry, filter: o.metricFilter}
	o.engine.registerHandle(h)
	var out observerdef.Handle = h
	if !o.ingestMetricsEnabled {
		out = &metricDropHandle{inner: out}
	}
	return out
}

// metricDropHandle drops every ObserveMetric call but lets ObserveLog
// through. Used when anomaly_detection.metrics.enabled=false so
// external metric sources (DogStatsD, check samplers) do not feed the engine.
// Virtual metrics produced by LogMetricsExtractors during engine.IngestLog are
// unaffected because they bypass this handle path entirely.
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

// RecordSamplerDropped increments the observer input-rate-limiter drop counter.
func (o *observerImpl) RecordSamplerDropped(source, priority string) {
	if o.telemetry != nil {
		o.telemetry.recordInputRateLimiterDropped(source, priority)
	}
}

// DumpMetrics writes all stored metrics to the specified file as JSON.
func (o *observerImpl) DumpMetrics(path string) error {
	// For simplicity, just dump directly (storage access is single-threaded from run loop,
	// but this is a debug tool so approximate snapshot is fine)
	return o.engine.Storage().DumpToFile(path)
}

// SubscribeSeverityEvents registers listener described by cfg. Delegates to
// the engine scorer when one is configured.
func (o *observerImpl) SubscribeSeverityEvents(cfg severityeventsdef.SeverityEventsConfiguration, listener severityeventsdef.SeverityEventListener) (severityeventsdef.SeverityEventsSubscription, error) {
	o.engine.mu.RLock()
	scorer := o.engine.scorer
	o.engine.mu.RUnlock()
	if scorer == nil {
		return severityeventsdef.SeverityEventsSubscription{}, errors.New("no active anomaly scorer")
	}
	return scorer.SubscribeSeverityEvents(cfg, listener)
}

// SubscribeSeverityEventsReader is a convenience for pull-only consumers.
// Delegates to the engine scorer when one is configured.
func (o *observerImpl) SubscribeSeverityEventsReader(cfg severityeventsdef.SeverityEventsConfiguration) (severityeventsdef.SeverityEventsReaderSubscription, error) {
	o.engine.mu.RLock()
	scorer := o.engine.scorer
	o.engine.mu.RUnlock()
	if scorer == nil {
		return severityeventsdef.SeverityEventsReaderSubscription{}, errors.New("no active anomaly scorer")
	}
	return scorer.SubscribeSeverityEventsReader(cfg)
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
func (o *observerImpl) Reset(settings ComponentSettings, storageCfg StorageConfig) {
	o.Flush()
	detectors, correlators, scorer, extractors, _ := o.catalog.Instantiate(settings)
	// Catalog scorers are unwatched standalone instances. Replay needs the same
	// transition watcher as the live observer so scorer correlation episodes open
	// and close as severity changes; telemetry gauges are optional here.
	if scorer != nil {
		scorer = newAnomalyScorerWithTelemetry(scorer.config, nil, nil)
	}
	o.replayMu.Lock()
	o.metricFilter.muted.Store(nil)
	o.engine.ResetForReplay(detectors, correlators, scorer, extractors, storageCfg, settings.Baseline)
	o.publishSeriesCount()
	o.replayMu.Unlock()
}

// baselineEventSink publishes the mute hash set to the shared filter when the
// baseline window ends.
type baselineEventSink struct {
	filter *metricsFilterRules
}

func (s *baselineEventSink) onEngineEvent(evt engineEvent) {
	if evt.kind != eventBaselineCompleted || evt.baselineCompleted == nil {
		return
	}
	if evt.baselineCompleted.snapshotChanged {
		s.filter.publishMutedSnapshot(evt.baselineCompleted.mutedHashes)
	}
}

// Compile-time assertion: observerImpl satisfies the testbench-extended surface.
// testbenchView in the bench package embeds DebugView and adds this method;
// the assertion ensures it is never silently dropped from observerImpl.
var _ interface{ DebugSubscribeBaselineCompleted(func(int64, []string)) } = (*observerImpl)(nil)

// DebugSubscribeBaselineCompleted registers a one-time callback invoked when the
// baseline window closes. Testbench-only — never called by the live agent.
// The callback receives the freeze timestamp (Unix seconds) and sorted
// "namespace/metricName" group keys for all muted series, resolved from
// storage while series are still alive (before reclaim).
func (o *observerImpl) DebugSubscribeBaselineCompleted(callback func(endSec int64, mutedGroups []string)) {
	if o.engine.baseline == nil {
		return
	}
	o.engine.Subscribe(&baselineCompletedCallbackSink{
		engine:   o.engine,
		callback: callback,
	})
}

// DebugBaselineStatus returns the per-detector baseline state for the
// testbench. It is deliberately outside DebugView's production contract.
func (o *observerImpl) DebugBaselineStatus() BaselineDebugStatus {
	// This method is testbench-only. Serializing it with direct replay and the
	// dispatch loop avoids reading the controller's maps concurrently without
	// adding synchronization or allocations to the live agent's ingest path.
	o.replayMu.Lock()
	defer o.replayMu.Unlock()
	if o.engine.baseline == nil {
		return BaselineDebugStatus{}
	}
	status := o.engine.baseline.debugStatus()
	o.engine.mu.RLock()
	status.AnalyzedThroughSec = o.engine.lastAnalyzedDataTime
	o.engine.mu.RUnlock()
	return status
}

type baselineCompletedCallbackSink struct {
	engine      *engine
	callback    func(int64, []string)
	mutedGroups map[string]struct{}
}

func (s *baselineCompletedCallbackSink) onEngineEvent(evt engineEvent) {
	if evt.kind != eventBaselineCompleted || evt.baselineCompleted == nil {
		return
	}
	if s.mutedGroups == nil {
		s.mutedGroups = make(map[string]struct{})
	}
	for _, ref := range evt.baselineCompleted.mutedRefs {
		meta := s.engine.storage.GetSeriesMeta(ref)
		if meta == nil {
			continue
		}
		key := meta.Namespace + "/" + meta.Name
		s.mutedGroups[key] = struct{}{}
	}
	if !evt.baselineCompleted.allComplete {
		return
	}
	groups := make([]string, 0, len(s.mutedGroups))
	for group := range s.mutedGroups {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	s.callback(evt.timestamp, groups)
	// The sink is retained across testbench replays. Release this run's groups
	// so a later replay reports only its own muted series.
	s.mutedGroups = nil
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
	// preserves extractor state so enrichAnomaly can still attach log pattern
	// context (stored on seriesStats) during replay.
	o.replayMu.Lock()
	o.engine.resetAnalysisState()
	o.engine.ReplayStoredData()
	o.publishSeriesCount()
	o.replayMu.Unlock()
}

// StorageReader returns a read-only view of the engine's time-series storage.
// Used by the testbench to compute windowed log rates in change messages.
// Implements DebugView.
func (o *observerImpl) StorageReader() observerdef.StorageReader {
	return o.engine.storage
}

// IngestLogForReplay feeds a log directly into the engine without driving any
// scheduler-triggered advances. Implements DebugView. Used while pre-loading
// retained data so that extractor state is built up
// and log metrics are written to storage, but detector/correlator advances are
// deferred to the subsequent ReplayStoredData call.
func (o *observerImpl) IngestLogForReplay(source string, msg observerdef.LogView) {
	lo := logObsFromView(msg)
	o.replayMu.Lock()
	// Advance requests are intentionally discarded.
	_ = o.engine.IngestLog(source, lo)
	o.engine.storage.RecordObservationTime(lo.timestampMs / 1000)
	if o.telemetry != nil {
		o.telemetry.recordLogAccepted(classifyLogSource(source, lo.tags), len(lo.content))
	}
	o.replayMu.Unlock()
}

// IngestLogAndAdvance feeds a log directly into the engine and executes any
// scheduler-triggered advances before returning. Implements DebugView.
func (o *observerImpl) IngestLogAndAdvance(source string, msg observerdef.LogView) {
	lo := logObsFromView(msg)
	o.replayMu.Lock()
	requests := o.engine.IngestLog(source, lo)
	for _, req := range requests {
		_ = o.engine.advanceWithReason(req.upToSec, req.reason)
		o.engine.replayAdvances.Add(1)
	}
	o.engine.replayAnomalies.Store(int64(o.engine.TotalAnomalyCount()))
	if o.telemetry != nil {
		o.telemetry.recordLogAccepted(classifyLogSource(source, lo.tags), len(lo.content))
	}
	if len(requests) > 0 {
		o.publishSeriesCount()
	}
	o.replayMu.Unlock()
}

// FinishReplayStream flushes the scheduler at end-of-input without resetting
// the analysis state accumulated by synchronous ingestion. Implements DebugView.
func (o *observerImpl) FinishReplayStream() {
	o.replayMu.Lock()
	o.engine.FinishReplayStream()
	o.publishSeriesCount()
	o.replayMu.Unlock()
}

func logObsFromView(msg observerdef.LogView) *logObs {
	return &logObs{
		content:     msg.GetContent(),
		status:      msg.GetStatus(),
		tags:        copyTags(msg.Tags()),
		hostname:    msg.GetHostname(),
		timestampMs: msg.GetTimestampUnixMilli(),
	}
}

func normalizeMetricSource(name, source string) string {
	if strings.HasPrefix(name, "datadog.") {
		return observerdef.AgentNamespace
	}
	return source
}

type metricIngestDecision struct {
	source string
	metric *metricObs
}

func prepareMetricIngest(source string, sample observerdef.MetricView, filter *metricsFilterRules) metricIngestDecision {
	name := sample.GetName()
	normalizedSource := normalizeMetricSource(name, source)
	precheck := filter.precheck(name, normalizedSource)
	if precheck.reject {
		return metricIngestDecision{source: normalizedSource}
	}

	// Canonicalize once so the mute hash in isMuted matches seriesKeyHash in
	// storage, and downstream Add calls hit the tagsSorted fast path.
	tags := canonicalizeTags(sample.GetRawTags())
	if filter.isMuted(name, normalizedSource, tags) ||
		(precheck.needsTags && !filter.isAllowedByRulesFrom(name, normalizedSource, tags, precheck.firstCandidate)) {
		return metricIngestDecision{source: normalizedSource}
	}

	timestamp := sample.GetTimestampUnix()
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}
	return metricIngestDecision{
		source: normalizedSource,
		metric: &metricObs{
			name:      name,
			value:     sample.GetValue(),
			tags:      tags,
			timestamp: timestamp,
		},
	}
}

// IngestMetricSync feeds a metric directly into the engine, bypassing the
// dispatch channel. Mirrors the handle.ObserveMetricAndReportDrop path without
// the non-blocking channel send. Implements DebugView.
func (o *observerImpl) IngestMetricSync(source string, sample observerdef.MetricView) {
	decision := prepareMetricIngest(source, sample, o.metricFilter)
	if decision.metric == nil {
		if o.telemetry != nil && decision.source != "" {
			o.telemetry.recordFilteredMetric(decision.source)
		}
		return
	}
	o.replayMu.Lock()
	requests := o.engine.IngestMetric(decision.source, decision.metric)
	for _, req := range requests {
		_ = o.engine.advanceWithReason(req.upToSec, req.reason)
		o.engine.replayAdvances.Add(1)
	}
	o.engine.replayAnomalies.Store(int64(o.engine.TotalAnomalyCount()))
	if o.telemetry != nil {
		o.telemetry.recordMetricAccepted(decision.source)
	}
	if len(requests) > 0 {
		o.publishSeriesCount()
	}
	o.replayMu.Unlock()
}

// handle is the lightweight observation interface passed to other components.
// It only holds a channel and source name - all processing happens in the observer.
type handle struct {
	ch        chan<- observation
	source    string
	dropCount atomic.Int64 // per-handle drop counter, collected by engine at advance time
	telemetry *observerTelemetry
	filter    *metricsFilterRules

	// The overwhelmingly common filtered-metric source for a handle is its
	// source. Bind that Prometheus counter lazily to avoid a label lookup and a
	// variadic allocation for every rejected metric.
	filteredMetricOnce   sync.Once
	filteredMetricSource string
	filteredMetric       telemetry.SimpleCounter
}

// ObserveMetric observes a DogStatsD metric sample.
func (h *handle) ObserveMetric(sample observerdef.MetricView) {
	_ = h.ObserveMetricAndReportDrop(sample)
}

// ObserveMetricAndReportDrop observes a metric and reports whether this
// specific call was dropped by observer backpressure (channel full).
// Metrics rejected by processing rules are counted via telemetry but do not
// report a channel drop.
func (h *handle) ObserveMetricAndReportDrop(sample observerdef.MetricView) bool {
	decision := prepareMetricIngest(h.source, sample, h.filter)
	if decision.metric == nil {
		if h.telemetry != nil && decision.source != "" {
			h.recordFilteredMetric(decision.source)
		}
		return false
	}
	obs := observation{
		source: decision.source,
		metric: decision.metric,
	}

	// Non-blocking send - drop if channel is full.
	select {
	case h.ch <- obs:
		if h.telemetry != nil {
			h.telemetry.recordMetricAccepted(decision.source)
		}
		return false
	default:
		h.dropCount.Add(1)
		if h.telemetry != nil {
			h.telemetry.recordObservationDropped("metrics", decision.source)
		}
		return true
	}
}

func (h *handle) recordFilteredMetric(source string) {
	h.filteredMetricOnce.Do(func() {
		h.filteredMetricSource = source
		h.filteredMetric = h.telemetry.filteredMetrics.WithValues(source)
	})
	if h.filteredMetricSource == source {
		h.filteredMetric.Inc()
		return
	}

	// datadog.* metrics are normalized to the agent namespace, rather than the
	// source that created this handle. That path is uncommon, so retain the
	// generic lookup to keep telemetry labels correct.
	h.telemetry.recordFilteredMetric(source)
}

// ObserveLog observes a log message.
func (h *handle) ObserveLog(msg observerdef.LogView) {
	// Use provided timestampMs if available, otherwise use current time
	timestampMs := msg.GetTimestampUnixMilli()
	tags := copyTags(msg.Tags())
	content := msg.GetContent()
	logSource := ""
	if h.telemetry != nil {
		logSource = classifyLogSource(h.source, tags)
		// Increment before enqueue to avoid a race where the consumer dequeues and
		// decrements to zero before this producer increments.
		h.telemetry.incrementLogsInFlight(logSource)
	}

	obs := observation{
		source: h.source,
		log: &logObs{
			content:     content,
			status:      msg.GetStatus(),
			tags:        tags,
			hostname:    msg.GetHostname(),
			timestampMs: timestampMs,
		},
	}

	// Non-blocking send - drop if channel is full.
	select {
	case h.ch <- obs:
		if h.telemetry != nil {
			h.telemetry.recordLogAccepted(logSource, len(content))
		}
	default:
		h.dropCount.Add(1)
		if h.telemetry != nil {
			// Roll back pre-enqueue in-flight increment for dropped logs.
			h.telemetry.decrementLogsInFlight(logSource)
			h.telemetry.recordObservationDropped("logs", logSource)
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
