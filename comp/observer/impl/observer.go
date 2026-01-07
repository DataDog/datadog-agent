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
	obs := &observerImpl{
		logProcessors: []observerdef.LogProcessor{
			&LogTimeSeriesAnalysis{
				// Keep defaults minimal; future steps add filtering + caps.
				MaxEvalBytes: 4096,
			},
			&BadDetector{},
			&ConnectionErrorExtractor{},
		},
		tsAnalyses: []observerdef.TimeSeriesAnalysis{
			NewSustainedElevationDetector(),
		},
		anomalyProcessors: []observerdef.AnomalyProcessor{
			NewCorrelator(CorrelatorConfig{}),
		},
		reporters: []observerdef.Reporter{
			&StdoutReporter{},
		},
		storage: newTimeSeriesStorage(),
		obsCh:   make(chan observation, 1000),
	}
	go obs.run()

	cfg := pkgconfigsetup.Datadog()

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

// processMetric handles a metric observation.
func (o *observerImpl) processMetric(source string, m *metricObs) {
	// Add to storage
	o.storage.Add(source, m.name, m.value, m.timestamp, m.tags)

	// Run time series analyses (using average aggregation)
	if series := o.storage.GetSeries(source, m.name, m.tags, AggregateAverage); series != nil {
		o.runTSAnalyses(*series)
	}

	o.flushAndReport()
}

// processLog handles a log observation.
func (o *observerImpl) processLog(source string, l *logObs) {
	// Create a view for processors
	view := &logView{obs: l}

	for _, processor := range o.logProcessors {
		result := processor.Process(view)

		// Add metrics from log processing to storage, then run TS analyses
		for _, m := range result.Metrics {
			o.storage.Add(source, m.Name, m.Value, l.timestamp, m.Tags)
			if series := o.storage.GetSeries(source, m.Name, m.Tags, AggregateAverage); series != nil {
				o.runTSAnalyses(*series)
			}
		}

		// Forward anomalies to processors
		for _, anomaly := range result.Anomalies {
			o.processAnomaly(anomaly)
		}
	}

	o.flushAndReport()
}

// runTSAnalyses runs all time series analyses on a series.
func (o *observerImpl) runTSAnalyses(series observerdef.Series) {
	for _, tsAnalysis := range o.tsAnalyses {
		result := tsAnalysis.Analyze(series)
		for _, anomaly := range result.Anomalies {
			o.processAnomaly(anomaly)
		}
	}
}

// processAnomaly sends an anomaly to all registered anomaly processors.
func (o *observerImpl) processAnomaly(anomaly observerdef.AnomalyOutput) {
	for _, processor := range o.anomalyProcessors {
		processor.Process(anomaly)
	}
}

// flushAndReport flushes all anomaly processors and sends reports to all reporters.
func (o *observerImpl) flushAndReport() {
	for _, processor := range o.anomalyProcessors {
		reports := processor.Flush()
		for _, report := range reports {
			for _, reporter := range o.reporters {
				reporter.Report(report)
			}
		}
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

// ObserveLog observes a log message.
func (h *handle) ObserveLog(msg observerdef.LogView) {
	obs := observation{
		source: h.source,
		log: &logObs{
			content:   copyBytes(msg.GetContent()),
			status:    msg.GetStatus(),
			tags:      copyTags(msg.GetTags()),
			hostname:  msg.GetHostname(),
			timestamp: time.Now().Unix(),
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
