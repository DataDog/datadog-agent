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

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires declares the input types to the observer component constructor.
type Requires struct {
	// AgentInternalLogTap provides optional overrides for capturing agent-internal logs.
	// When fields are nil, values are read from configuration defaults.
	AgentInternalLogTap AgentInternalLogTapConfig
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
	source string
	metric *metricObs
	log    *logObs
}

// metricObs contains copied metric data.
type metricObs struct {
	name      string
	value     float64
	tags      []string
	timestamp int64
}

// logObs contains copied log data.
type logObs struct {
	content   []byte
	status    string
	tags      []string
	hostname  string
	timestamp int64
}

// NewComponent creates an observer.Component.
func NewComponent(deps Requires) Provides {
	correlator := NewCorrelator(CorrelatorConfig{})
	reporter := &StdoutReporter{}

	// Connect the reporter to the correlator's state
	reporter.SetCorrelationState(correlator)

	obs := &observerImpl{
		logProcessors: []observerdef.LogProcessor{
			&LogTimeSeriesAnalysis{
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
		tsAnalyses: []observerdef.TimeSeriesAnalysis{
			NewCUSUMDetector(),
		},
		anomalyProcessors: []observerdef.AnomalyProcessor{
			correlator,
		},
		reporters: []observerdef.Reporter{
			reporter,
		},
		storage:   newTimeSeriesStorage(),
		obsCh:     make(chan observation, 1000),
		maxEvents: 1000, // Keep last 1000 events for debugging
	}

	cfg := pkgconfigsetup.Datadog()

	// Initialize parquet writer only if both capture_metrics AND parquet_output_dir are configured.
	// When capture_metrics is false, no metrics are recorded to parquet.
	captureMetrics := cfg.GetBool("observer.capture_metrics")
	if captureMetrics {
		if parquetDir := cfg.GetString("observer.parquet_output_dir"); parquetDir != "" {
			flushInterval := cfg.GetDuration("observer.parquet_flush_interval")
			if flushInterval == 0 {
				flushInterval = 60 * time.Second
			}

			retentionDuration := cfg.GetDuration("observer.parquet_retention")
			// Default to 24 hours if not set or invalid
			if retentionDuration <= 0 {
				retentionDuration = 24 * time.Hour
			}

			writer, err := NewParquetWriter(parquetDir, flushInterval, retentionDuration)
			if err != nil {
				pkglog.Errorf("Failed to create parquet writer: %v", err)
			} else {
				obs.parquetWriter = writer
				pkglog.Infof("Observer parquet writer enabled: dir=%s flush=%v retention=%v", parquetDir, flushInterval, retentionDuration)
			}
		}
	} else {
		pkglog.Debug("Observer parquet writer disabled (observer.capture_metrics is false)")
	}

	go obs.run()

	// Start periodic metric dump if configured
	dumpPath := cfg.GetString("observer.debug_dump_path")
	eventsDumpPath := cfg.GetString("observer.debug_events_dump_path")
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
				// Also dump events if configured
				if eventsDumpPath != "" {
					if err := obs.DumpEvents(eventsDumpPath); err != nil {
						fmt.Fprintf(os.Stderr, "[observer] events dump error: %v\n", err)
					} else {
						fmt.Printf("[observer] dumped events to %s\n", eventsDumpPath)
					}
				}
			}
		}()
	}

	// Capture agent-internal logs into the observer by default (best-effort, non-blocking).
	enabled := cfg.GetBool("observer.capture_agent_internal_logs")
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
			// Emit structured JSON so LogTimeSeriesAnalysis can extract fields consistently.
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
	logProcessors     []observerdef.LogProcessor
	tsAnalyses        []observerdef.TimeSeriesAnalysis
	anomalyProcessors []observerdef.AnomalyProcessor
	reporters         []observerdef.Reporter
	storage           *timeSeriesStorage
	obsCh             chan observation
	// eventBuffer stores recent events (markers) for debugging/dumping.
	// Ring buffer with maxEvents capacity.
	eventBuffer []observerdef.EventSignal
	maxEvents   int

	// Raw anomaly tracking for test bench display
	rawAnomalies     []observerdef.AnomalyOutput
	rawAnomalyMu     sync.RWMutex
	rawAnomalyWindow int64 // seconds to keep raw anomalies (0 = unlimited)
	currentDataTime  int64 // latest data timestamp seen

	// Parquet writer for long-term storage of metrics
	parquetWriter *ParquetWriter
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
	}
}

// analysisAggregations defines which aggregations to run TS analyses on.
// This allows detecting both value elevation (average) and frequency elevation (count).
var analysisAggregations = []Aggregate{AggregateAverage, AggregateCount}

// processMetric handles a metric observation.
func (o *observerImpl) processMetric(source string, m *metricObs) {
	// Add to storage
	o.storage.Add(source, m.name, m.value, m.timestamp, m.tags)

	// Write to parquet if enabled
	if o.parquetWriter != nil {
		o.parquetWriter.WriteMetric(source, m.name, m.value, m.tags, m.timestamp)
	}

	// Run time series analyses on multiple aggregations
	for _, agg := range analysisAggregations {
		if series := o.storage.GetSeries(source, m.name, m.tags, agg); series != nil {
			o.runTSAnalyses(*series, agg)
		}
	}

	o.flushAndReport()
}

// processLog handles a log observation.
func (o *observerImpl) processLog(source string, l *logObs) {
	// Events (from check-events source) are routed as event signals for correlation,
	// not processed through log processors for metric derivation.
	if source == "check-events" {
		o.routeEventSignal(l)
		return
	}

	// Create a view for processors
	view := &logView{obs: l}

	for _, processor := range o.logProcessors {
		result := processor.Process(view)

		// Add metrics from log processing to storage, then run TS analyses
		for _, m := range result.Metrics {
			o.storage.Add(source, m.Name, m.Value, l.timestamp, m.Tags)
			// Run time series analyses on multiple aggregations
			for _, agg := range analysisAggregations {
				if series := o.storage.GetSeries(source, m.Name, m.Tags, agg); series != nil {
					o.runTSAnalyses(*series, agg)
				}
			}
		}

	}

	o.flushAndReport()
}

// routeEventSignal converts an event log observation to an EventSignal and sends it
// to all EventSignalReceivers (typically the correlator). Events are used as correlation
// context, not as inputs for metric derivation or anomaly detection.
func (o *observerImpl) routeEventSignal(l *logObs) {
	// Extract event type from tags (already standardized at source, e.g., "event_type:agent_startup")
	eventSource := "unknown_event"
	for _, tag := range l.tags {
		if strings.HasPrefix(tag, "event_type:") {
			eventSource = strings.TrimPrefix(tag, "event_type:")
			break
		}
	}

	signal := observerdef.EventSignal{
		Source:    eventSource,
		Timestamp: l.timestamp,
		Tags:      l.tags,
		Message:   string(l.content),
	}

	// Store in event buffer (ring buffer)
	if o.maxEvents > 0 {
		if len(o.eventBuffer) >= o.maxEvents {
			// Shift out oldest event
			o.eventBuffer = o.eventBuffer[1:]
		}
		o.eventBuffer = append(o.eventBuffer, signal)
	}

	// Send to all anomaly processors that implement EventSignalReceiver
	for _, proc := range o.anomalyProcessors {
		if receiver, ok := proc.(observerdef.EventSignalReceiver); ok {
			receiver.AddEventSignal(signal)
		}
	}
}

// runTSAnalyses runs all time series analyses on a series with the given aggregation.
// It appends an aggregation suffix to the series name for distinct Source tracking.
func (o *observerImpl) runTSAnalyses(series observerdef.Series, agg Aggregate) {
	// Append aggregation suffix to series name for distinct Source tracking
	seriesWithAgg := series
	seriesWithAgg.Name = series.Name + ":" + aggSuffix(agg)

	for _, tsAnalysis := range o.tsAnalyses {
		result := tsAnalysis.Analyze(seriesWithAgg)
		for _, anomaly := range result.Anomalies {
			// Set the analyzer name so we can identify who produced this anomaly
			anomaly.AnalyzerName = tsAnalysis.Name()
			// Capture raw anomaly before passing to processors
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

// processAnomaly sends an anomaly to all registered anomaly processors.
func (o *observerImpl) processAnomaly(anomaly observerdef.AnomalyOutput) {
	for _, processor := range o.anomalyProcessors {
		processor.Process(anomaly)
	}
}

// captureRawAnomaly stores a raw anomaly for test bench display.
// Deduplicates by Source+AnalyzerName, keeping the most recent.
func (o *observerImpl) captureRawAnomaly(anomaly observerdef.AnomalyOutput) {
	o.rawAnomalyMu.Lock()
	defer o.rawAnomalyMu.Unlock()

	// Update current data time
	if anomaly.Timestamp > o.currentDataTime {
		o.currentDataTime = anomaly.Timestamp
	}

	// Deduplicate by Source+AnalyzerName (keep most recent)
	key := anomaly.Source + "|" + anomaly.AnalyzerName
	found := false
	for i, existing := range o.rawAnomalies {
		existingKey := existing.Source + "|" + existing.AnalyzerName
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
}

// RawAnomalies returns a copy of currently tracked raw anomalies.
// Implements observerdef.RawAnomalyState interface.
func (o *observerImpl) RawAnomalies() []observerdef.AnomalyOutput {
	o.rawAnomalyMu.RLock()
	defer o.rawAnomalyMu.RUnlock()

	result := make([]observerdef.AnomalyOutput, len(o.rawAnomalies))
	copy(result, o.rawAnomalies)
	return result
}

// flushAndReport flushes all anomaly processors and notifies all reporters.
// Reporters are called with an empty report to trigger state-based reporting.
func (o *observerImpl) flushAndReport() {
	for _, processor := range o.anomalyProcessors {
		processor.Flush()
	}
	// Always notify reporters so they can check correlation state
	for _, reporter := range o.reporters {
		reporter.Report(observerdef.ReportOutput{})
	}
}

// GetHandle returns a lightweight handle for a named source.
func (o *observerImpl) GetHandle(name string) observerdef.Handle {
	return &handle{ch: o.obsCh, source: name}
}

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

// DumpEvents writes all buffered event signals to the specified file as JSON.
// Event signals are container lifecycle events (OOM, restart, etc.) used for correlation.
func (o *observerImpl) DumpEvents(path string) error {
	type dumpEvent struct {
		Source    string   `json:"source"`
		Timestamp int64    `json:"timestamp"`
		Tags      []string `json:"tags,omitempty"`
		Message   string   `json:"message"`
	}

	events := make([]dumpEvent, len(o.eventBuffer))
	for i, signal := range o.eventBuffer {
		events[i] = dumpEvent{
			Source:    signal.Source,
			Timestamp: signal.Timestamp,
			Tags:      signal.Tags,
			Message:   signal.Message,
		}
	}

	data, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write events file: %w", err)
	}
	return nil
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
	// TODO: Add telemetry to track dropped observations.
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
	// TODO: Add telemetry to track dropped observations.
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
