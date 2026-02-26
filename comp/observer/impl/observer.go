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
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires declares the input types to the observer component constructor.
type Requires struct {
	// AgentInternalLogTap provides optional overrides for capturing agent-internal logs.
	// When fields are nil, values are read from configuration defaults.
	AgentInternalLogTap AgentInternalLogTapConfig
	// Recorder is an optional component for transparent metric recording.
	// If provided, all handles will be wrapped to record metrics to parquet files.
	// If nil, handles operate without recording.
	Recorder recorderdef.Component
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

	// TODO
	// Wire the reporter to pull raw anomaly state from obs.
	reporter.SetRawAnomalyState(obs)

	// Set up handle function with optional recorder wrapping.
	// If recorder is provided, wrap handles to enable transparent metric recording.
	// Otherwise, use inner handle directly (no recording).
	if deps.Recorder != nil {
		obs.handleFunc = deps.Recorder.GetHandle(obs.innerHandle)
	} else {
		obs.handleFunc = obs.innerHandle
	}

	go obs.run()

	go func() {
		time.Sleep(4 * time.Second)

		signal := observerdef.Signal{
			Source:    "system.disk.free",
			Message:   "What r u doing???",
			Timestamp: time.Now().Unix(),
			Tags:      []string{"celian:true"},
			Value:     100,
			Score:     nil,
		}
		anomaly := obs.signalToAnomaly(signal, "cusum_detector")
		// anomaly := observerdef.AnomalyOutput{
		// 	Source:         observerdef.MetricName("system.disk.free:avg"),
		// 	SourceSeriesID: observerdef.SeriesID(seriesKey("system.disk.free", "avg", []string{"celian:true"})),
		// 	Title:          "Celian",
		// 	Description:    "wow",
		// 	Tags:           []string{"celian:true"},
		// 	Timestamp:      time.Now().Unix(),
		// 	DebugInfo: &observerdef.AnomalyDebugInfo{
		// 		BaselineStart:  time.Now().Unix() - 1000,
		// 		BaselineEnd:    time.Now().Unix(),
		// 		BaselineMean:   100,
		// 		BaselineStddev: 10,
		// 		CurrentValue:   100,
		// 		DeviationSigma: 1,
		// 	},
		// 	// AnalyzerName: "celian_detector",
		// 	AnalyzerName: "cusum_detector",
		// }

		obs.captureRawAnomaly(anomaly)
		obs.processAnomaly(anomaly)
		obs.processSignal(signal)
		obs.flushAndReport()

		// Forward to anomaly processors
		fmt.Printf("[observer] [celian] ALERT: %s\n", anomaly.Description)
	}()

	cfg := pkgconfigsetup.Datadog()

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
	anomalyProcessors []observerdef.AnomalyProcessor // Supports both old (AnomalyOutput) and new (Signal via ProcessSignal)
	reporters         []observerdef.Reporter
	storage           *timeSeriesStorage
	obsCh             chan observation
	handleFunc        observerdef.HandleFunc // Handle factory (may wrap with recorder middleware)

	// NEW path: Signal-based processing (V2 architecture)
	signalEmitters   []observerdef.SignalEmitter   // Layer 1: Point-based anomaly detection
	signalProcessors []observerdef.SignalProcessor // Layer 2: Signal correlation/filtering

	// Deduplication layer (optional) - filters anomalies before correlation
	deduplicator *AnomalyDeduplicator

	// eventBuffer stores recent event signals (as unified Signals) for debugging/dumping.
	// Ring buffer with maxEvents capacity.
	eventBuffer []observerdef.Signal
	maxEvents   int

	// Raw anomaly tracking for test bench display
	rawAnomalies         []observerdef.AnomalyOutput
	rawAnomalyMu         sync.RWMutex
	rawAnomalyWindow     int64                           // seconds to keep raw anomalies (0 = unlimited)
	maxRawAnomalies      int                             // max number of raw anomalies to keep (0 = unlimited)
	currentDataTime      int64                           // latest data timestamp seen
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
	}
}

// analysisAggregations defines which aggregations to run TS analyses on.
// This allows detecting both value elevation (average) and frequency elevation (count).
var analysisAggregations = []Aggregate{AggregateAverage, AggregateCount}

// processMetric handles a metric observation.
func (o *observerImpl) processMetric(source string, m *metricObs) {
	// Add to storage
	o.storage.Add(source, m.name, m.value, m.timestamp, m.tags)

	// Run time series analyses on multiple aggregations
	for _, agg := range analysisAggregations {
		if series := o.storage.GetSeries(source, m.name, m.tags, agg); series != nil {
			// OLD path: Run region-based anomaly detection (TimeSeriesAnalysis)
			o.runTSAnalyses(*series, agg)

			// NEW path: Run point-based signal emitters
			o.runSignalEmitters(*series, agg)
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

// routeEventSignal converts an event log observation to a unified Signal and sends it
// to all SignalProcessors. Events are used as correlation context, not as inputs for
// metric derivation or anomaly detection.
func (o *observerImpl) routeEventSignal(l *logObs) {
	// Extract event type from tags (already standardized at source, e.g., "event_type:agent_startup")
	eventSource := "unknown_event"
	for _, tag := range l.tags {
		if strings.HasPrefix(tag, "event_type:") {
			eventSource = strings.TrimPrefix(tag, "event_type:")
			break
		}
	}

	// Create unified Signal with "event:" prefix to distinguish from metric anomalies
	signal := observerdef.Signal{
		Source:    observerdef.SignalSource("event:" + eventSource),
		Timestamp: l.timestamp,
		Tags:      l.tags,
		Value:     0,   // Events don't have numeric values
		Score:     nil, // Events don't have scores
	}

	// Store in event buffer (ring buffer)
	if o.maxEvents > 0 {
		if len(o.eventBuffer) >= o.maxEvents {
			// Shift out oldest event
			o.eventBuffer = o.eventBuffer[1:]
		}
		o.eventBuffer = append(o.eventBuffer, signal)
	}

	// Send to anomaly processors that support signals (new way - use ProcessSignal method)
	for _, proc := range o.anomalyProcessors {
		// If the processor supports the new Signal-based input, send it
		if sp, ok := proc.(*CrossSignalCorrelator); ok {
			sp.ProcessSignal(signal)
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
			anomaly.Source = observerdef.MetricName(seriesWithAgg.Name)
			anomaly.SourceSeriesID = observerdef.SeriesID(seriesKey(series.Namespace, seriesWithAgg.Name, series.Tags))
			// Capture raw anomaly before passing to processors
			// o.captureRawAnomaly(anomaly)
			// o.processAnomaly(anomaly)
		}
	}
}

// runSignalEmitters runs point-based signal emitters on a series and processes the signals.
// It appends the aggregation suffix to the series name for distinct Source tracking.
func (o *observerImpl) runSignalEmitters(series observerdef.Series, agg Aggregate) {
	// Append aggregation suffix to series name for distinct Source tracking (matches runTSAnalyses)
	seriesWithAgg := series
	seriesWithAgg.Name = series.Name + ":" + aggSuffix(agg)

	for _, emitter := range o.signalEmitters {
		signals := emitter.Emit(seriesWithAgg)

		for _, signal := range signals {
			// Convert signal to anomaly and send to correlators
			anomaly := o.signalToAnomaly(signal, emitter.Name())
			anomaly.SourceSeriesID = observerdef.SeriesID(seriesKey(series.Namespace, seriesWithAgg.Name, series.Tags))
			// o.captureRawAnomaly(anomaly) // For UI display
			// o.processAnomaly(anomaly)    // Send to correlators (GraphSketchCorrelator, etc.)

			// Also send signal to signal processors (Layer 2)
			// o.processSignal(signal)
		}
	}
}

// signalToAnomaly converts a Signal to an AnomalyOutput for use with correlators.
func (o *observerImpl) signalToAnomaly(signal observerdef.Signal, emitterName string) observerdef.AnomalyOutput {
	// Build description from signal data
	desc := fmt.Sprintf("%s signal at timestamp %d", signal.Source, signal.Timestamp)
	if signal.Score != nil {
		desc = fmt.Sprintf("%s (score: %.2f) at timestamp %d", signal.Source, *signal.Score, signal.Timestamp)
	}

	return observerdef.AnomalyOutput{
		Source:       observerdef.MetricName(signal.Source),
		Title:        fmt.Sprintf("Signal: %s", signal.Source),
		Description:  desc,
		Tags:         signal.Tags,
		AnalyzerName: emitterName,
		TimeRange: observerdef.TimeRange{
			Start: signal.Timestamp,
			End:   signal.Timestamp, // Point-based: start == end
		},
	}
}

// processSignal sends a signal to all signal processors.
func (o *observerImpl) processSignal(signal observerdef.Signal) {
	for _, processor := range o.signalProcessors {
		processor.Process(signal)
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
// If deduplicator is enabled, filters out duplicate anomalies first.
func (o *observerImpl) processAnomaly(anomaly observerdef.AnomalyOutput) {
	// Check deduplicator if enabled
	if o.deduplicator != nil {
		ts := anomaly.Timestamp
		if ts == 0 {
			ts = anomaly.TimeRange.End
		}
		if !o.deduplicator.ShouldProcess(string(anomaly.SourceSeriesID), ts) {
			o.dedupSkipped++
			return // Duplicate, skip
		}
	}

	for _, processor := range o.anomalyProcessors {
		processor.Process(anomaly)
	}
}

// captureRawAnomaly stores a raw anomaly for test bench display.
// Deduplicates by Source+AnalyzerName, keeping the most recent.
func (o *observerImpl) captureRawAnomaly(anomaly observerdef.AnomalyOutput) {
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

	// Deduplicate by SourceSeriesID+AnalyzerName+Timestamp (keep all unique anomalies)
	key := fmt.Sprintf("%s|%s|%d", anomaly.SourceSeriesID, anomaly.AnalyzerName, anomaly.TimeRange.End)
	found := false
	for i, existing := range o.rawAnomalies {
		existingKey := fmt.Sprintf("%s|%s|%d", existing.SourceSeriesID, existing.AnalyzerName, existing.TimeRange.End)
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
func (o *observerImpl) RawAnomalies() []observerdef.AnomalyOutput {
	o.rawAnomalyMu.RLock()
	defer o.rawAnomalyMu.RUnlock()

	result := make([]observerdef.AnomalyOutput, len(o.rawAnomalies))
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

// flushAndReport flushes all anomaly processors and notifies all reporters.
// Reporters are called with an empty report to trigger state-based reporting.
func (o *observerImpl) flushAndReport() {
	// Flush anomaly processors (correlators)
	for _, processor := range o.anomalyProcessors {
		processor.Flush()
	}
	// Flush signal processors (Layer 2)
	for _, processor := range o.signalProcessors {
		processor.Flush()
	}
	// Always notify reporters so they can check correlation state
	for _, reporter := range o.reporters {
		reporter.Report(observerdef.ReportOutput{})
	}
}

// GetHandle returns a lightweight handle for a named source.
// If a recorder is configured, the handle will be wrapped to record metrics.
func (o *observerImpl) GetHandle(name string) observerdef.Handle {
	return o.handleFunc(name)
}

// innerHandle creates the base handle without any middleware wrapping.
func (o *observerImpl) innerHandle(name string) observerdef.Handle {
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
		Value     float64  `json:"value"`
	}

	events := make([]dumpEvent, len(o.eventBuffer))
	for i, signal := range o.eventBuffer {
		events[i] = dumpEvent{
			Source:    string(signal.Source),
			Timestamp: signal.Timestamp,
			Tags:      signal.Tags,
			Value:     signal.Value,
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
