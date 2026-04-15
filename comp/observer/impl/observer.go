// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerimpl implements the observer component.
package observerimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	compdef "github.com/DataDog/datadog-agent/comp/def"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	taggerdef "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	workloadfilterdef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmetadef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/comp/observer/impl/hfrunner"
	"github.com/DataDog/datadog-agent/pkg/metrics"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires declares the input types to the observer component constructor.
type Requires struct {
	// AgentInternalLogTap provides optional overrides for capturing agent-internal logs.
	// When fields are nil, values are read from configuration defaults.
	AgentInternalLogTap AgentInternalLogTapConfig

	Lifecycle compdef.Lifecycle
	Config    config.Component
	Log       log.Component
	Telemetry telemetry.Component

	// Recorder is an optional component for transparent metric recording.
	// If provided, all handles will be wrapped to record metrics to parquet files.
	Recorder option.Option[recorderdef.Component]

	// RemoteAgentRegistry enables fetching traces/profiles
	// from remote trace-agents via the ObserverProvider gRPC service.
	RemoteAgentRegistry remoteagentregistry.Component

	// WMeta, FilterStore, Tagger are optional — required only when
	// observer.high_frequency_container_checks.enabled is true.
	// Using option.Option so the observer can start without them (e.g. in tests
	// or agent binaries that don't include container infrastructure).
	WMeta       option.Option[workloadmetadef.Component]
	FilterStore option.Option[workloadfilterdef.Component]
	Tagger      option.Option[taggerdef.Component]
}

type AgentInternalLogTapConfig struct {
	Enabled         *bool
	SampleRateInfo  *float64
	SampleRateDebug *float64
	SampleRateTrace *float64
}

// Provides defines the output of the observer component.
type Provides struct {
	Comp observerdef.Component
}

// observation is a message sent from handles to the observer.
type observation struct {
	source  string
	metric  *metricObs
	log     *logObs
	profile *profileObs
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
	content     []byte
	status      string
	tags        []string
	hostname    string
	timestampMs int64
}

// profileObs contains copied profile data.
type profileObs struct {
	profileID    string
	profileType  string
	service      string
	env          string
	version      string
	hostname     string
	containerID  string
	timestamp    int64
	duration     int64
	tags         map[string]string
	contentType  string
	rawData      []byte
	externalPath string
}

// Ensure logObs implements observerdef.LogView
var _ observerdef.LogView = (*logObs)(nil)

func (l *logObs) GetContent() []byte {
	return l.content
}

func (l *logObs) GetStatus() string {
	return l.status
}

func (l *logObs) GetTags() []string {
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
//	observer.components.<name>.enabled        (bool)
//	observer.components.<name>.<field>        (type-specific)
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
		prefix := "observer.components." + entry.name + "."
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

// NewComponent creates an observer.Component.
func NewComponent(deps Requires) Provides {
	cfg := deps.Config
	catalog := defaultCatalog()
	settings := settingsFromAgentConfig(catalog, cfg)
	detectors, correlators, extractors, _ := catalog.Instantiate(settings)

	eng := newEngine(engineConfig{
		storage:          newTimeSeriesStorage(),
		extractors:       extractors,
		detectors:        detectors,
		correlators:      correlators,
		contextProviders: collectContextProviders(extractors),
		scheduler:        &currentBehaviorPolicy{},
	})

	// Wire reporters via event subscription.
	// The reporterEventSink queries stateView for active correlations on each advance,
	// so reporters receive all needed data through ReportOutput without backdoor access.
	reporter := &StdoutReporter{}
	eng.Subscribe(&reporterEventSink{
		reporters: []observerdef.Reporter{reporter},
		state:     eng.StateView(),
	})

	telemetryComp := deps.Telemetry
	if telemetryComp == nil {
		telemetryComp = noopsimpl.GetCompatComponent()
	}

	hfSystemEnabled := cfg.GetBool("observer.high_frequency_system_checks.enabled")
	hfContainerEnabled := cfg.GetBool("observer.high_frequency_container_checks.enabled")
	th := newTelemetryHandler(telemetryComp)

	// Build the set of MetricSource values to suppress from the "all-metrics"
	// pipeline. Sources are added later, only after their respective HF runners
	// are confirmed started, to avoid suppressing 15s metrics when the HF
	// replacement can't start.
	hfFilterSources := make(map[metrics.MetricSource]struct{})

	obs := &observerImpl{
		engine:             eng,
		obsCh:              make(chan observation, 1000),
		telemetryHandler:   th,
		dropCounter:        th.telemetryCounters[telemetryObsChannelDropped],
		hfContainerEnabled: hfContainerEnabled,
		hfFilterSources:    hfFilterSources,
	}

	// Set up handle function based on recording and analysis configuration.
	// Recording (observer.recording.enabled) enables parquet writers and the fetcher.
	// Analysis (observer.analysis.enabled) enables the anomaly detection pipeline.
	analysisEnabled := cfg.GetBool("observer.analysis.enabled")

	obs.handleFunc = obs.noopHandle
	if analysisEnabled {
		obs.handleFunc = obs.innerHandle
	}

	recordingEnabled := cfg.GetBool("observer.recording.enabled")
	recorderAvailable := false
	if recorder, ok := deps.Recorder.Get(); ok {
		recorderAvailable = true
		obs.handleFunc = recorder.GetHandle(obs.handleFunc)

		// Record detect digests and advance log alongside parquet for parity debugging.
		parquetDir := cfg.GetString("observer.recording.parquet_output_dir")
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

	// Optionally add the event reporter when sending is enabled via config.
	if cfg.GetBool("observer.event_reporter.sending_enabled") {
		if sender, err := newEventSender(deps.Config, deps.Log, eng.Storage()); err != nil {
			deps.Log.Warnf("[observer] event_reporter disabled: %v", err)
		} else {
			eventReporter := &EventReporter{sender: sender, logger: deps.Log}
			eng.Subscribe(&reporterEventSink{
				reporters: []observerdef.Reporter{eventReporter},
				state:     eng.StateView(),
			})
		}
	}

	go obs.run()

	// Start high-frequency system check runner if enabled.
	// Checks run at 1s and route metrics into the observer via a dedicated
	// "system-checks-hf" handle, never touching the aggregator or forwarder.
	if hfSystemEnabled {
		hfHandle := obs.GetHandle(hfrunner.HFSource)
		obs.hfRunner = hfrunner.New(hfHandle)
		obs.hfRunner.Start()
		obs.hfEnabled = true
		for src := range systemCheckSources {
			obs.hfFilterSources[src] = struct{}{}
		}
		pkglog.Info("[observer] high-frequency system check runner started (1s interval)")
		deps.Lifecycle.Append(compdef.Hook{
			OnStop: func(_ context.Context) error {
				obs.hfRunner.Stop()
				return nil
			},
		})
	}

	// Start high-frequency container check runner if enabled.
	// Uses the generic container check with WLM + tagger for full per-container
	// cardinality. Metrics route via "container-checks-hf" and never reach intake.
	if hfContainerEnabled {
		wmeta, wmetaOk := deps.WMeta.Get()
		filterStore, filterOk := deps.FilterStore.Get()
		tagger, taggerOk := deps.Tagger.Get()
		if wmetaOk && filterOk && taggerOk {
			containerHandle := obs.GetHandle(hfrunner.HFContainerSource)
			obs.hfContainerRunner = hfrunner.NewContainer(containerHandle, hfrunner.ContainerDeps{
				WMeta:       wmeta,
				FilterStore: filterStore,
				Tagger:      tagger,
			})
			if obs.hfContainerRunner != nil {
				obs.hfContainerRunner.Start()
				pkglog.Info("[observer] high-frequency container check runner started (1s interval)")
				// Only suppress 15s container metrics now that the HF replacement is confirmed running.
				for src := range containerCheckSources {
					obs.hfFilterSources[src] = struct{}{}
				}
				deps.Lifecycle.Append(compdef.Hook{
					OnStop: func(_ context.Context) error {
						obs.hfContainerRunner.Stop()
						return nil
					},
				})
			}
		} else {
			pkglog.Warn("[observer] high_frequency_container_checks.enabled=true but WMeta/FilterStore/Tagger not available; skipping")
		}
	}

	// Start periodic metric dump if configured
	dumpPath := cfg.GetString("observer.debug_dump_path")
	dumpInterval := cfg.GetDuration("observer.debug_dump_interval")
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

	// Capture agent-internal logs into the observer by default (best-effort, non-blocking).
	enabled := cfg.GetBool("observer.capture_agent_internal_logs.enabled")
	if deps.AgentInternalLogTap.Enabled != nil {
		enabled = *deps.AgentInternalLogTap.Enabled
	}
	if enabled {
		sampleInfo := cfg.GetFloat64("observer.capture_agent_internal_logs.sample_rate_info")
		sampleDebug := cfg.GetFloat64("observer.capture_agent_internal_logs.sample_rate_debug")
		sampleTrace := cfg.GetFloat64("observer.capture_agent_internal_logs.sample_rate_trace")
		if deps.AgentInternalLogTap.SampleRateInfo != nil {
			sampleInfo = *deps.AgentInternalLogTap.SampleRateInfo
		}
		if deps.AgentInternalLogTap.SampleRateDebug != nil {
			sampleDebug = *deps.AgentInternalLogTap.SampleRateDebug
		}
		if deps.AgentInternalLogTap.SampleRateTrace != nil {
			sampleTrace = *deps.AgentInternalLogTap.SampleRateTrace
		}

		handle := obs.GetHandle("agent-internal-logs")
		baseTags := []string{"source:datadog-agent"}

		var infoN, debugN, traceN uint64
		shouldSample := func(level pkglog.LogLevel) bool {
			var rate float64
			switch level {
			case pkglog.WarnLvl, pkglog.ErrorLvl, pkglog.CriticalLvl:
				return true
			case pkglog.InfoLvl:
				rate = sampleInfo
				n := atomic.AddUint64(&infoN, 1)
				return samplePass(rate, n)
			case pkglog.DebugLvl:
				rate = sampleDebug
				n := atomic.AddUint64(&debugN, 1)
				return samplePass(rate, n)
			case pkglog.TraceLvl:
				rate = sampleTrace
				n := atomic.AddUint64(&traceN, 1)
				return samplePass(rate, n)
			default:
				// Unknown level: treat as info.
				n := atomic.AddUint64(&infoN, 1)
				return samplePass(sampleInfo, n)
			}
		}

		pkglog.SetLogObserver(func(level pkglog.LogLevel, message string) {
			if !shouldSample(level) {
				return
			}
			// Build tags per callback so component:<...> stays accurate if the logger name changes.
			tags := make([]string, 0, 3)
			tags = append(tags, baseTags...)
			if name := pkglog.GetLoggerName(); name != "" {
				tags = append(tags, "component:"+name)
			}
			tags = append(tags, "level:"+strings.ToLower(level.String()))
			// Emit structured JSON so LogMetricsExtractor can extract fields consistently.
			// Level is carried as a tag (separate timeseries per level).
			payload, _ := json.Marshal(map[string]any{
				"msg": message,
			})
			handle.ObserveLog(&agentLogView{
				content:     payload,
				status:      strings.ToLower(level.String()),
				tags:        tags,
				hostname:    "",
				timestampMs: time.Now().UnixMilli(),
			})
		})
	}

	// Start the profile fetcher only when recording is enabled.
	// Profiles are not analyzed by the observer; fetching them is
	// only useful for the recorder-backed parquet path.
	if recordingEnabled && recorderAvailable {
		fetchHandle := obs.GetHandle("profile-agent")
		obs.fetcher = newObserverFetcher(
			deps.RemoteAgentRegistry,
			fetchHandle,
		)
		obs.fetcher.Start()
		pkglog.Info("[observer] profile fetcher started")
	}

	return Provides{Comp: obs}
}

func samplePass(rate float64, n uint64) bool {
	if rate <= 0 {
		return false
	}
	if rate >= 1 {
		return true
	}
	const denom = 1000
	threshold := uint64(rate * denom)
	// Ensure very small non-zero rates still occasionally pass.
	if threshold == 0 {
		threshold = 1
	}
	return (n % denom) < threshold
}

// observerImpl is the implementation of the observer component.
// It is a thin driver around the engine, which holds storage, extractors,
// detectors, correlators, and raw anomaly tracking.
type observerImpl struct {
	engine     *engine
	obsCh      chan observation
	handleFunc observerdef.HandleFunc // Handle factory (may wrap with recorder middleware)

	// fetcher pulls traces/profiles from remote trace-agents
	fetcher *observerFetcher

	telemetryHandler  *telemetryHandler
	digestCleanup     func() // flushes detect digest recording file
	advanceLogCleanup func() // flushes advance log recording file

	// dropCounter counts observations silently dropped when the channel is full.
	// Tagged by source for Prometheus visibility. Complements engine.droppedObs
	// which tracks drops for live/replay parity analysis.
	dropCounter telemetry.Counter

	// hfRunner is the high-frequency system check runner, non-nil when enabled.
	hfRunner *hfrunner.Runner

	// hfContainerRunner is the high-frequency container check runner, non-nil when enabled.
	hfContainerRunner *hfrunner.Runner

	// hfFilterSources is the combined set of MetricSource values to suppress from
	// the "all-metrics" pipeline when their HF counterpart is active. Built at
	// construction time from whichever HF flags are enabled.
	hfFilterSources map[metrics.MetricSource]struct{}

	// hfEnabled is true when high-frequency system check collection is active.
	hfEnabled bool

	// hfContainerEnabled is true when high-frequency container check collection is active.
	hfContainerEnabled bool
}

// run is the main dispatch loop, processing all observations sequentially.
func (o *observerImpl) run() {
	for obs := range o.obsCh {
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
		if obs.profile != nil {
			o.processProfile(obs.source, obs.profile)
		}
		for _, req := range requests {
			result := o.engine.advanceWithReason(req.upToSec, req.reason)
			o.telemetryHandler.handleTelemetry(result.telemetry)
		}
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

	lastVisibleCount map[string]int
}

func newSeriesDetectorAdapter(detector observerdef.SeriesDetector, aggregations []observerdef.Aggregate) *seriesDetectorAdapter {
	return &seriesDetectorAdapter{
		detector:         detector,
		aggregations:     aggregations,
		windowSec:        defaultDetectorWindowSec,
		lastVisibleCount: make(map[string]int),
	}
}

func (a *seriesDetectorAdapter) Name() string {
	return a.detector.Name()
}

// Reset clears adapter-local caches and resets the wrapped detector when supported.
func (a *seriesDetectorAdapter) Reset() {
	a.lastVisibleCount = make(map[string]int)
	if resetter, ok := a.detector.(interface{ Reset() }); ok {
		resetter.Reset()
	}
}

func (a *seriesDetectorAdapter) Detect(storage observerdef.StorageReader, dataTime int64) observerdef.DetectionResult {
	allSeries := storage.ListSeries(observerdef.WorkloadSeriesFilter())

	var allAnomalies []observerdef.Anomaly
	var allTelemetry []observerdef.ObserverTelemetry

	for _, meta := range allSeries {
		keyStr := seriesKey(meta.Namespace, meta.Name, meta.Tags)
		visibleCount := storage.PointCountUpTo(meta.Ref, dataTime)
		if prev, ok := a.lastVisibleCount[keyStr]; ok && prev == visibleCount {
			continue
		}
		a.lastVisibleCount[keyStr] = visibleCount

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

// processProfile handles a profile observation.
// Currently this is a placeholder that logs the profile; full implementation
// will include parquet metadata storage and binary file storage for large profiles.
func (o *observerImpl) processProfile(source string, p *profileObs) {
	// TODO: Implement profile metadata storage to parquet
	// TODO: Implement binary file storage for large profiles
	pkglog.Debugf("[observer] profile observed from %s: profileID=%s type=%s service=%s size=%d",
		source, p.profileID, p.profileType, p.service, len(p.rawData))
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
func (o *observerImpl) innerHandle(name string) observerdef.Handle {
	h := &handle{ch: o.obsCh, source: name, dropCounter: o.dropCounter}
	o.engine.registerHandle(h)
	if len(o.hfFilterSources) > 0 && name == "all-metrics" {
		return &hfFilteredHandle{inner: h, sources: o.hfFilterSources}
	}
	return h
}

// sourceProvider is a structural interface satisfied by *metrics.MetricSample,
// which carries a MetricSource enum populated by the standard check sender.
// Using a type assertion (rather than adding GetSource to MetricView) avoids
// importing pkg/metrics into comp/observer/def.
type sourceProvider interface {
	GetSource() metrics.MetricSource
}

// systemCheckSources is the set of MetricSource values produced by the system
// checks that the HF runner executes. It mirrors the check list in hfrunner/runner.go.
var systemCheckSources = map[metrics.MetricSource]struct{}{
	metrics.MetricSourceCPU:        {},
	metrics.MetricSourceLoad:       {},
	metrics.MetricSourceMemory:     {},
	metrics.MetricSourceIo:         {},
	metrics.MetricSourceDisk:       {},
	metrics.MetricSourceNetwork:    {},
	metrics.MetricSourceUptime:     {},
	metrics.MetricSourceFileHandle: {},
}

// containerCheckSources is the set of MetricSource values produced by the
// container checks that the HF container runner executes. Only MetricSourceContainer
// is included because the HF runner uses the generic container check (check name
// "container"), which maps to MetricSourceContainer regardless of runtime.
// The legacy per-runtime checks (containerd, cri, docker) have their own
// MetricSource values but are not run by the HF runner.
var containerCheckSources = map[metrics.MetricSource]struct{}{
	metrics.MetricSourceContainer: {},
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

func (f *hfFilteredHandle) ObserveLog(msg observerdef.LogView)             { f.inner.ObserveLog(msg) }
func (f *hfFilteredHandle) ObserveTrace(_ observerdef.TraceView)           {}
func (f *hfFilteredHandle) ObserveTraceStats(_ observerdef.TraceStatsView) {}
func (f *hfFilteredHandle) ObserveProfile(p observerdef.ProfileView)       { f.inner.ObserveProfile(p) }

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
func (h *noopObserveHandle) ObserveLog(_ observerdef.LogView)               {}
func (h *noopObserveHandle) ObserveTrace(_ observerdef.TraceView)           {}
func (h *noopObserveHandle) ObserveTraceStats(_ observerdef.TraceStatsView) {}
func (h *noopObserveHandle) ObserveProfile(_ observerdef.ProfileView)       {}

// DumpMetrics writes all stored metrics to the specified file as JSON.
func (o *observerImpl) DumpMetrics(path string) error {
	// For simplicity, just dump directly (storage access is single-threaded from run loop,
	// but this is a debug tool so approximate snapshot is fine)
	return o.engine.Storage().DumpToFile(path)
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
			content:     copyBytes(msg.GetContent()),
			status:      msg.GetStatus(),
			tags:        copyTags(msg.GetTags()),
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

// ObserveTrace is a no-op. Trace processing is not used.
func (h *handle) ObserveTrace(_ observerdef.TraceView) {}

// ObserveTraceStats is a no-op. Trace stats processing is deprioritized.
func (h *handle) ObserveTraceStats(_ observerdef.TraceStatsView) {}

// ObserveProfile observes a profiling sample.
func (h *handle) ObserveProfile(profile observerdef.ProfileView) {
	obs := observation{
		source: h.source,
		profile: &profileObs{
			profileID:    profile.GetProfileID(),
			profileType:  profile.GetProfileType(),
			service:      profile.GetService(),
			env:          profile.GetEnv(),
			version:      profile.GetVersion(),
			hostname:     profile.GetHostname(),
			containerID:  profile.GetContainerID(),
			timestamp:    profile.GetTimestampUnixNano(),
			duration:     profile.GetDurationNano(),
			tags:         copyStringMap(profile.GetTags()),
			contentType:  profile.GetContentType(),
			rawData:      copyBytes(profile.GetRawData()),
			externalPath: profile.GetExternalPath(),
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

func (v *logView) GetContent() []byte           { return v.obs.content }
func (v *logView) GetStatus() string            { return v.obs.status }
func (v *logView) GetTags() []string            { return v.obs.tags }
func (v *logView) GetHostname() string          { return v.obs.hostname }
func (v *logView) GetTimestampUnixMilli() int64 { return v.obs.timestampMs }

// agentLogView is a minimal LogView implementation for agent-internal logs.
// It is immediately copied by the observer handle, so it must not be retained.
type agentLogView struct {
	content     []byte
	status      string
	tags        []string
	hostname    string
	timestampMs int64
}

func (v *agentLogView) GetContent() []byte           { return v.content }
func (v *agentLogView) GetStatus() string            { return v.status }
func (v *agentLogView) GetTags() []string            { return v.tags }
func (v *agentLogView) GetHostname() string          { return v.hostname }
func (v *agentLogView) GetTimestampUnixMilli() int64 { return v.timestampMs }

// copyBytes creates a copy of a byte slice.
func copyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	result := make([]byte, len(b))
	copy(result, b)
	return result
}

// copyStringMap creates a copy of a string map.
func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
