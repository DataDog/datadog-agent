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
	"sync"
	"sync/atomic"
	"time"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/hook"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires declares the input types to the observer component constructor.
type Requires struct {
	// AgentInternalLogTap provides optional overrides for capturing agent-internal logs.
	// When fields are nil, values are read from configuration defaults.
	AgentInternalLogTap AgentInternalLogTapConfig

	// Recorder is an optional component for transparent metric recording.
	// If provided, all handles will be wrapped to record metrics to parquet files.
	Recorder option.Option[recorderdef.Component]

	// RemoteAgentRegistry enables fetching traces/profiles
	// from remote trace-agents via the ObserverProvider gRPC service.
	RemoteAgentRegistry remoteagentregistry.Component

	MetricsHooks []hook.Hook[observerdef.MetricView] `group:"hook"`
	LogsHooks    []hook.Hook[observerdef.LogView]    `group:"hook"`
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

func (m *metricObs) GetTimestamp() float64 {
	return float64(m.timestamp)
}

// Observer does not store samplerate; just return 1.0
func (m *metricObs) GetSampleRate() float64 {
	return 1.0
}

// logObs contains copied log data and implements observerdef.LogView.
type logObs struct {
	content   []byte
	status    string
	tags      []string
	hostname  string
	timestamp int64
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
func (l *logObs) GetTimestamp() int64 {
	return l.timestamp
}

// NewComponent creates an observer.Component.
func NewComponent(deps Requires) Provides {
	correlator := NewCorrelator(CorrelatorConfig{})
	reporter := &StdoutReporter{}

	// Connect the reporter to the correlator's state
	reporter.SetCorrelationState(correlator)

	obs := &observerImpl{
		logDetectors: []observerdef.LogDetector{
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
		},
		metricsDetectors: []observerdef.MetricsDetector{
			NewCUSUMDetector(),
		},
		correlators: []observerdef.Correlator{
			correlator,
		},
		reporters: []observerdef.Reporter{
			reporter,
		},
		storage: newTimeSeriesStorage(),
		obsCh:   make(chan observation, 1000),
	}

	cfg := pkgconfigsetup.Datadog()

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

	go obs.run()

	// Subscribe to metric hooks — copy and forward to the observer's processing pipeline.
	for _, mh := range fxutil.GetAndFilterGroup(deps.MetricsHooks) {
		handle := obs.GetHandle(mh.Name())
		mh.Subscribe("observer-metrics-hook", func(payload observerdef.MetricView) {
			handle.ObserveMetric(payload)
		})
	}

	// Subscribe to log hooks — copy and forward to the observer's processing pipeline.
	for _, lh := range fxutil.GetAndFilterGroup(deps.LogsHooks) {
		handle := obs.GetHandle(lh.Name())
		lh.Subscribe("observer-logs-hook", func(payload observerdef.LogView) {
			handle.ObserveLog(payload)
		})
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
				content:  payload,
				status:   strings.ToLower(level.String()),
				tags:     tags,
				hostname: "",
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
type observerImpl struct {
	logDetectors     []observerdef.LogDetector
	metricsDetectors []observerdef.MetricsDetector
	correlators      []observerdef.Correlator
	reporters        []observerdef.Reporter
	storage          *timeSeriesStorage
	obsCh            chan observation
	handleFunc       observerdef.HandleFunc // Handle factory (may wrap with recorder middleware)

	// Deduplication layer (optional) - filters anomalies before correlation
	deduplicator *AnomalyDeduplicator

	// Raw anomaly tracking for test bench display
	rawAnomalies     []observerdef.Anomaly
	rawAnomalyMu     sync.RWMutex
	rawAnomalyWindow int64 // seconds to keep raw anomalies (0 = unlimited)
	maxRawAnomalies  int   // max number of raw anomalies to keep (0 = unlimited)
	currentDataTime  int64 // latest data timestamp seen

	// fetcher pulls traces/profiles from remote trace-agents
	fetcher              *observerFetcher
	totalAnomalyCount    int                             // total count of all anomalies ever detected (no cap)
	uniqueAnomalySources map[observerdef.MetricName]bool // unique sources that had anomalies
	dedupSkipped         int                             // count of anomalies skipped by dedup
}

// run is the main dispatch loop, processing all observations sequentially.
func (o *observerImpl) run() {
	for obs := range o.obsCh {
		if obs.metric != nil {
			o.processMetric(obs.source, obs.metric)
		}
		if obs.log != nil {
			o.processLog(obs.source, obs.log)
		}
		if obs.trace != nil {
			o.processTrace(obs.source, obs.trace)
		}
		if obs.profile != nil {
			o.processProfile(obs.source, obs.profile)
		}
	}
}

// detectionAggregations defines which aggregations to run metrics detection on.
// This allows detecting both value elevation (average) and frequency elevation (count).
var detectionAggregations = []Aggregate{AggregateAverage, AggregateCount}

// processMetric handles a metric observation.
func (o *observerImpl) processMetric(source string, m *metricObs) {
	// Add to storage
	o.storage.Add(source, m.name, m.value, m.timestamp, m.tags)

	// Run metrics detection on multiple aggregations
	for _, agg := range detectionAggregations {
		if series := o.storage.GetSeries(source, m.name, m.tags, agg); series != nil {
			o.runMetricsDetectors(*series, agg)
		}
	}

	o.flushAndReport()
}

// processLog handles a log observation.
func (o *observerImpl) processLog(source string, l *logObs) {
	// Create a view for log detectors
	view := &logView{obs: l}

	for _, detector := range o.logDetectors {
		result := detector.Process(view)

		// Add metrics from log processing to storage, then run metrics detection
		for _, m := range result.Metrics {
			o.storage.Add(source, m.Name, m.Value, l.timestamp, m.Tags)
			// Run metrics detection on multiple aggregations
			for _, agg := range detectionAggregations {
				if series := o.storage.GetSeries(source, m.Name, m.Tags, agg); series != nil {
					o.runMetricsDetectors(*series, agg)
				}
			}
		}

		// Route directly emitted anomalies through the standard pipeline
		for _, anomaly := range result.Anomalies {
			o.captureRawAnomaly(anomaly)
			o.processAnomaly(anomaly)
		}
	}

	o.flushAndReport()
}

// runMetricsDetectors runs all metrics detectors on a series with the given aggregation.
// It appends an aggregation suffix to the series name for distinct Source tracking.
func (o *observerImpl) runMetricsDetectors(series observerdef.Series, agg Aggregate) {
	// Append aggregation suffix to series name for distinct Source tracking
	seriesWithAgg := series
	seriesWithAgg.Name = series.Name + ":" + aggSuffix(agg)

	for _, metricsDetector := range o.metricsDetectors {
		result := metricsDetector.Detect(seriesWithAgg)
		for _, anomaly := range result.Anomalies {
			// Set the detector name so we can identify who produced this anomaly
			anomaly.DetectorName = metricsDetector.Name()
			anomaly.Source = observerdef.MetricName(seriesWithAgg.Name)
			anomaly.SourceSeriesID = observerdef.SeriesID(seriesKey(series.Namespace, seriesWithAgg.Name, series.Tags))
			// Capture raw anomaly before passing to correlators
			o.captureRawAnomaly(anomaly)
			o.processAnomaly(anomaly)
		}
	}
}

// aggSuffix returns a short suffix for the given aggregation type.
func aggSuffix(agg Aggregate) string {
	switch agg {
	case AggregateAverage:
		return "avg"
	case AggregateSum:
		return "sum"
	case AggregateCount:
		return "count"
	case AggregateMin:
		return "min"
	case AggregateMax:
		return "max"
	default:
		return "unknown"
	}
}

// processAnomaly sends an anomaly to all registered correlators.
// If deduplicator is enabled, filters out duplicate anomalies first.
func (o *observerImpl) processAnomaly(anomaly observerdef.Anomaly) {
	// Check deduplicator if enabled
	if o.deduplicator != nil {
		if !o.deduplicator.ShouldProcess(string(anomaly.SourceSeriesID), anomaly.Timestamp) {
			o.dedupSkipped++
			return // Duplicate, skip
		}
	}

	for _, correlator := range o.correlators {
		correlator.Process(anomaly)
	}
}

// captureRawAnomaly stores a raw anomaly for test bench display.
// Deduplicates by Source+DetectorName, keeping the most recent.
func (o *observerImpl) captureRawAnomaly(anomaly observerdef.Anomaly) {
	o.rawAnomalyMu.Lock()
	defer o.rawAnomalyMu.Unlock()

	// Always increment total count (no cap)
	o.totalAnomalyCount++

	// Track unique sources
	if o.uniqueAnomalySources == nil {
		o.uniqueAnomalySources = make(map[observerdef.MetricName]bool)
	}
	o.uniqueAnomalySources[anomaly.Source] = true

	// Update current data time
	if anomaly.Timestamp > o.currentDataTime {
		o.currentDataTime = anomaly.Timestamp
	}

	// Deduplicate by SourceSeriesID+DetectorName+Timestamp (keep all unique anomalies)
	key := fmt.Sprintf("%s|%s|%d", anomaly.SourceSeriesID, anomaly.DetectorName, anomaly.Timestamp)
	found := false
	for i, existing := range o.rawAnomalies {
		existingKey := fmt.Sprintf("%s|%s|%d", existing.SourceSeriesID, existing.DetectorName, existing.Timestamp)
		if existingKey == key {
			if anomaly.Timestamp > existing.Timestamp {
				o.rawAnomalies[i] = anomaly
			}
			found = true
			break
		}
	}
	if !found {
		o.rawAnomalies = append(o.rawAnomalies, anomaly)
	}

	// Evict old anomalies if window is set
	if o.rawAnomalyWindow > 0 {
		cutoff := o.currentDataTime - o.rawAnomalyWindow
		newBuffer := o.rawAnomalies[:0]
		for _, a := range o.rawAnomalies {
			if a.Timestamp >= cutoff {
				newBuffer = append(newBuffer, a)
			}
		}
		o.rawAnomalies = newBuffer
	}

	// Cap at maxRawAnomalies if set
	if o.maxRawAnomalies > 0 && len(o.rawAnomalies) > o.maxRawAnomalies {
		// Keep most recent anomalies (tail of slice)
		o.rawAnomalies = o.rawAnomalies[len(o.rawAnomalies)-o.maxRawAnomalies:]
	}
}

// RawAnomalies returns a copy of currently tracked raw anomalies.
// Implements observerdef.RawAnomalyState interface.
func (o *observerImpl) RawAnomalies() []observerdef.Anomaly {
	o.rawAnomalyMu.RLock()
	defer o.rawAnomalyMu.RUnlock()

	result := make([]observerdef.Anomaly, len(o.rawAnomalies))
	copy(result, o.rawAnomalies)
	return result
}

// TotalAnomalyCount returns the total number of anomalies ever detected (no cap).
func (o *observerImpl) TotalAnomalyCount() int {
	o.rawAnomalyMu.RLock()
	defer o.rawAnomalyMu.RUnlock()
	return o.totalAnomalyCount
}

// UniqueAnomalySourceCount returns the number of unique sources that had anomalies.
func (o *observerImpl) UniqueAnomalySourceCount() int {
	o.rawAnomalyMu.RLock()
	defer o.rawAnomalyMu.RUnlock()
	return len(o.uniqueAnomalySources)
}

// DedupSkippedCount returns the number of anomalies skipped by deduplication.
func (o *observerImpl) DedupSkippedCount() int {
	return o.dedupSkipped
}

// flushAndReport flushes all correlators and notifies all reporters.
// Reporters are called with an empty report to trigger state-based reporting.
func (o *observerImpl) flushAndReport() {
	// Flush correlators
	for _, correlator := range o.correlators {
		correlator.Flush()
	}
	// Always notify reporters so they can check correlation state
	for _, reporter := range o.reporters {
		reporter.Report(observerdef.ReportOutput{})
	}
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
	// Request dump via channel to ensure thread safety
	type dumpReq struct {
		path   string
		result chan error
	}
	// For simplicity, just dump directly (storage access is single-threaded from run loop,
	// but this is a debug tool so approximate snapshot is fine)
	return o.storage.DumpToFile(path)
}

// handle is the lightweight observation interface passed to other components.
// It only holds a channel and source name - all processing happens in the observer.
type handle struct {
	ch     chan<- observation
	source string
}

// ObserveMetric observes a DogStatsD metric sample.
func (h *handle) ObserveMetric(sample observerdef.MetricView) {
	timestamp := int64(sample.GetTimestamp())
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

// logTimestamper is an optional interface for logs that provide their own timestamp.
type logTimestamper interface {
	GetTimestamp() int64
}

// ObserveLog observes a log message.
func (h *handle) ObserveLog(msg observerdef.LogView) {
	// Use provided timestamp if available, otherwise use current time
	timestamp := time.Now().Unix()
	if ts, ok := msg.(logTimestamper); ok {
		if t := ts.GetTimestamp(); t > 0 {
			timestamp = t
		}
	}

	obs := observation{
		source: h.source,
		log: &logObs{
			content:   copyBytes(msg.GetContent()),
			status:    msg.GetStatus(),
			tags:      copyTags(msg.GetTags()),
			hostname:  msg.GetHostname(),
			timestamp: timestamp,
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
			start:    sv.GetStart(),
			duration: sv.GetDuration(),
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
			timestamp:    trace.GetTimestamp(),
			duration:     trace.GetDuration(),
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
			timestamp:    profile.GetTimestamp(),
			duration:     profile.GetDuration(),
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

func (v *logView) GetContent() []byte  { return v.obs.content }
func (v *logView) GetStatus() string   { return v.obs.status }
func (v *logView) GetTags() []string   { return v.obs.tags }
func (v *logView) GetHostname() string { return v.obs.hostname }

// agentLogView is a minimal LogView implementation for agent-internal logs.
// It is immediately copied by the observer handle, so it must not be retained.
type agentLogView struct {
	content  []byte
	status   string
	tags     []string
	hostname string
}

func (v *agentLogView) GetContent() []byte  { return v.content }
func (v *agentLogView) GetStatus() string   { return v.status }
func (v *agentLogView) GetTags() []string   { return v.tags }
func (v *agentLogView) GetHostname() string { return v.hostname }

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
