// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerimpl implements the observer component.
package observerimpl

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires declares the input types to the observer component constructor.
type Requires struct {
	// AgentInternalLogTap provides optional overrides for capturing agent-internal logs.
	// When fields are nil, values are read from configuration defaults.
	AgentInternalLogTap AgentInternalLogTapConfig

	Config config.Component
	Log    log.Component

	// Recorder is an optional component for transparent metric recording.
	// If provided, all handles will be wrapped to record metrics to parquet files.
	Recorder option.Option[recorderdef.Component]

	// RemoteAgentRegistry enables fetching traces/profiles
	// from remote trace-agents via the ObserverProvider gRPC service.
	RemoteAgentRegistry remoteagentregistry.Component
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
	trace   *traceObs
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

// traceObs contains copied trace data.
type traceObs struct {
	traceIDHigh  uint64
	traceIDLow   uint64
	spans        []spanObs
	env          string
	service      string
	hostname     string
	containerID  string
	timestamp    int64
	duration     int64
	priority     int32
	isError      bool
	tags         map[string]string
	receivedAtNs int64
}

// spanObs contains copied span data.
type spanObs struct {
	spanID   uint64
	parentID uint64
	service  string
	name     string
	resource string
	spanType string
	start    int64
	duration int64
	error    int32
	meta     map[string]string
	metrics  map[string]float64
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

// NewComponent creates an observer.Component.
func NewComponent(deps Requires) Provides {
	cfg := pkgconfigsetup.Datadog()

	catalog := defaultCatalog()
	if tf := cfg.GetFloat64("observer.cusum.threshold_factor"); tf > 0 {
		deps.Log.Infof("[observer] cusum threshold_factor set to %.2f from config", tf)
		catalog = catalog.WithOverride("cusum", func() any {
			d := NewCUSUMDetector()
			d.ThresholdFactor = tf
			return d
		})
	}
	catalogOverrides := map[string]bool{
		"fanout": cfg.GetBool("observer.correlator.fanout.enabled"),
	}
	detectors, correlators, _ := catalog.Instantiate(catalogOverrides)

	extractors := []observerdef.LogMetricsExtractor{
		&LogMetricsExtractor{
			MaxEvalBytes: 4096,
			// Exclude metadata fields that shouldn't be metrics.
			// These are common timestamp/ID fields that appear in event JSON.
			ExcludeFields: map[string]struct{}{
				"timestamp": {}, // event.Event.Ts serializes as "timestamp"
				"ts":        {}, // alternate timestamp field name
				"time":      {},
				"pid":       {},
				"ppid":      {},
				"uid":       {},
				"gid":       {},
			},
		},
		&ConnectionErrorExtractor{},
	}

	eng := newEngine(engineConfig{
		storage:     newTimeSeriesStorage(),
		extractors:  extractors,
		detectors:   detectors,
		correlators: correlators,
		scheduler:   &currentBehaviorPolicy{},
	})

	// Wire reporters via event subscription.
	// The reporterEventSink queries stateView for active correlations on each advance,
	// so reporters receive all needed data through ReportOutput without backdoor access.
	reporter := &StdoutReporter{}
	eng.Subscribe(&reporterEventSink{
		reporters: []observerdef.Reporter{reporter},
		state:     eng.StateView(),
	})

	obs := &observerImpl{
		engine: eng,
		obsCh:  make(chan observation, 1000),
	}

	// Set up handle function based on recording and analysis configuration.
	// Recording (observer.recording.enabled) enables parquet writers and the fetcher.
	// Analysis (observer.analysis.enabled) enables the anomaly detection pipeline.
	analysisEnabled := cfg.GetBool("observer.analysis.enabled")

	obs.handleFunc = obs.noopHandle
	if analysisEnabled {
		obs.handleFunc = obs.innerHandle
	}

	if recorder, ok := deps.Recorder.Get(); ok {
		obs.handleFunc = recorder.GetHandle(obs.handleFunc)
	}

	// Optionally add the event reporter when sending is enabled via config.
	if deps.Config != nil && deps.Config.GetBool("observer.event_reporter.sending_enabled") {
		deps.Log.Infof("[observer] event_reporter: sending_enabled=true, initialising sender")
		if sender, err := newEventSender(deps.Config, deps.Log); err != nil {
			deps.Log.Warnf("[observer] event_reporter disabled: %v", err)
		} else {
			eventReporter := &EventReporter{sender: sender, logger: deps.Log}
			eng.Subscribe(&reporterEventSink{
				reporters: []observerdef.Reporter{eventReporter},
				state:     eng.StateView(),
			})
			deps.Log.Infof("[observer] event_reporter: registered successfully")
		}
	} else {
		deps.Log.Infof("[observer] event_reporter: sending_enabled=false, no events will be sent (set observer.event_reporter.sending_enabled: true to enable)")
	}

	go obs.run()

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

	// Start trace/profile fetcher if traces or profiles collection is enabled

	fetchHandle := obs.GetHandle("trace-agent")
	obs.fetcher = newObserverFetcher(
		deps.RemoteAgentRegistry,
		fetchHandle,
	)
	obs.fetcher.Start()
	pkglog.Info("[observer] trace/profile fetcher started")

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
}

// run is the main dispatch loop, processing all observations sequentially.
func (o *observerImpl) run() {
	for obs := range o.obsCh {
		var requests []advanceRequest
		if obs.metric != nil {
			requests = o.engine.IngestMetric(obs.source, obs.metric)
		}
		if obs.log != nil {
			requests = append(requests, o.engine.IngestLog(obs.source, obs.log)...)
		}
		if obs.trace != nil {
			o.processTrace(obs.source, obs.trace)
		}
		if obs.profile != nil {
			o.processProfile(obs.source, obs.profile)
		}
		for _, req := range requests {
			o.engine.advanceWithReason(req.upToSec, req.reason)
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
// The adapter tracks the point count per series/aggregation so it can skip
// re-running the detector when no new data has arrived. This is important
// because stateless detectors re-analyze the full series from scratch each
// call — without this guard, per-second advancement would be O(N²).
type seriesDetectorAdapter struct {
	detector     observerdef.SeriesDetector
	aggregations []observerdef.Aggregate

	// windowSec limits how far back GetSeriesRange reads. 0 means unbounded
	// (read from timestamp 0). A positive value reads [dataTime-windowSec, dataTime],
	// bounding per-call cost to O(windowSec) instead of O(totalPoints).
	windowSec int64

	// Per-series caching: PointCountUpTo (binary search, no copying) tells us
	// cheaply whether a series has new visible data. When it hasn't changed,
	// we skip detection entirely so callers do not see duplicate anomaly or
	// telemetry events for unchanged data.
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
	seriesKeys := storage.ListSeries(observerdef.SeriesFilter{})

	var allAnomalies []observerdef.Anomaly
	var allTelemetry []observerdef.ObserverTelemetry

	for _, key := range seriesKeys {
		k := seriesKey(key.Namespace, key.Name, key.Tags)

		// Cheap check: has this series gained any newly visible points?
		visibleCount := storage.PointCountUpTo(key, dataTime)
		if prev, ok := a.lastVisibleCount[k]; ok && prev == visibleCount {
			// No new data — do not re-run the detector or re-emit prior outputs.
			continue
		}
		a.lastVisibleCount[k] = visibleCount

		// Series has new data — run detector on each aggregation.
		var seriesAnomalies []observerdef.Anomaly
		var seriesTelemetry []observerdef.ObserverTelemetry

		for _, agg := range a.aggregations {
			start := int64(0)
			if a.windowSec > 0 {
				start = dataTime - a.windowSec
			}
			series := storage.GetSeriesRange(key, start, dataTime, agg)
			if series == nil || len(series.Points) == 0 {
				continue
			}

			seriesWithAgg := *series
			seriesWithAgg.Name = series.Name + ":" + aggSuffix(agg)

			result := a.detector.Detect(seriesWithAgg)
			for i := range result.Anomalies {
				result.Anomalies[i].Type = observerdef.AnomalyTypeMetric
				result.Anomalies[i].DetectorName = a.detector.Name()
				result.Anomalies[i].Source = observerdef.MetricName(seriesWithAgg.Name)
				result.Anomalies[i].SourceSeriesID = observerdef.SeriesID(seriesKey(series.Namespace, seriesWithAgg.Name, series.Tags))
			}
			seriesAnomalies = append(seriesAnomalies, result.Anomalies...)
			seriesTelemetry = append(seriesTelemetry, result.Telemetry...)
		}

		allAnomalies = append(allAnomalies, seriesAnomalies...)
		allTelemetry = append(allTelemetry, seriesTelemetry...)
	}

	return observerdef.DetectionResult{
		Anomalies: allAnomalies,
		Telemetry: allTelemetry,
	}
}

// aggSuffix returns a short suffix for the given aggregation type.
func aggSuffix(agg observerdef.Aggregate) string {
	switch agg {
	case observerdef.AggregateAverage:
		return "avg"
	case observerdef.AggregateSum:
		return "sum"
	case observerdef.AggregateCount:
		return "count"
	case observerdef.AggregateMin:
		return "min"
	case observerdef.AggregateMax:
		return "max"
	default:
		return "unknown"
	}
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

// processTrace handles a trace observation.
// Currently this is a placeholder that logs the trace; full implementation
// will include parquet storage and trace-specific analysis.
func (o *observerImpl) processTrace(source string, t *traceObs) {
	// TODO: Implement trace storage to parquet
	// TODO: Implement trace-specific analysis (latency anomalies, error patterns)
	pkglog.Debugf("[observer] trace observed from %s: traceID=%x%x spans=%d service=%s",
		source, t.traceIDHigh, t.traceIDLow, len(t.spans), t.service)
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
func (o *observerImpl) innerHandle(name string) observerdef.Handle {
	return &handle{ch: o.obsCh, source: name}
}

// noopHandle returns a handle that discards all observations.
// Used when analysis is disabled so the analysis pipeline is not started.
func (o *observerImpl) noopHandle(_ string) observerdef.Handle {
	return &noopObserveHandle{}
}

// noopObserveHandle discards all observations.
type noopObserveHandle struct{}

func (h *noopObserveHandle) ObserveMetric(_ observerdef.MetricView)         {}
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
	ch     chan<- observation
	source string
}

// ObserveMetric observes a DogStatsD metric sample.
func (h *handle) ObserveMetric(sample observerdef.MetricView) {
	timestamp := sample.GetTimestampUnix()
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}

	obs := observation{
		source: h.source,
		metric: &metricObs{
			name:      sample.GetName(),
			value:     sample.GetValue(),
			tags:      copyTags(sample.GetRawTags()),
			timestamp: timestamp,
		},
	}

	// Non-blocking send - drop if channel is full.
	// In production, this prevents slow consumers from blocking data ingestion.
	// For demo comparison, this means faster correlators see more data.
	select {
	case h.ch <- obs:
	default:
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
	}
}

// ObserveTrace observes a trace (collection of spans with the same trace ID).
func (h *handle) ObserveTrace(trace observerdef.TraceView) {
	high, low := trace.GetTraceID()

	// Copy all spans from the iterator
	var spans []spanObs
	iter := trace.GetSpans()
	for iter.Next() {
		sv := iter.Span()
		spans = append(spans, spanObs{
			spanID:   sv.GetSpanID(),
			parentID: sv.GetParentID(),
			service:  sv.GetService(),
			name:     sv.GetName(),
			resource: sv.GetResource(),
			spanType: sv.GetType(),
			start:    sv.GetStartUnixNano(),
			duration: sv.GetDurationNano(),
			error:    sv.GetError(),
			meta:     copyStringMap(sv.GetMeta()),
			metrics:  copyFloat64Map(sv.GetMetrics()),
		})
	}

	obs := observation{
		source: h.source,
		trace: &traceObs{
			traceIDHigh:  high,
			traceIDLow:   low,
			spans:        spans,
			env:          trace.GetEnv(),
			service:      trace.GetService(),
			hostname:     trace.GetHostname(),
			containerID:  trace.GetContainerID(),
			timestamp:    trace.GetTimestampUnixNano(),
			duration:     trace.GetDurationNano(),
			priority:     trace.GetPriority(),
			isError:      trace.IsError(),
			tags:         copyStringMap(trace.GetTags()),
			receivedAtNs: time.Now().UnixNano(),
		},
	}

	// Non-blocking send - drop if channel is full.
	select {
	case h.ch <- obs:
	default:
	}
}

// ObserveTraceStats processes APM stats by deriving in-memory metrics.
// Note: metrics are emitted directly on h (the inner observer handle), not on any
// outer recording handle, so derived metrics live in memory only and are never
// written to the metrics parquet file.
func (h *handle) ObserveTraceStats(stats observerdef.TraceStatsView) {
	processStatsView(h, stats)
}

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

// copyFloat64Map creates a copy of a float64 map.
func copyFloat64Map(m map[string]float64) map[string]float64 {
	if m == nil {
		return nil
	}
	result := make(map[string]float64, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
