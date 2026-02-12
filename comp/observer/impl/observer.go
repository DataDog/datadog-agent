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
	"github.com/DataDog/datadog-agent/comp/core/config"
	logger "github.com/DataDog/datadog-agent/comp/core/log/def"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires declares the input types to the observer component constructor.
type Requires struct {
	// Cfg is the agent config component.
	Cfg config.Component
	// AgentInternalLogTap provides optional overrides for capturing agent-internal logs.
	// When fields are nil, values are read from configuration defaults.
	AgentInternalLogTap AgentInternalLogTapConfig
	Recorder            recorderdef.Component

	// RemoteAgentRegistry enables fetching traces/profiles
	// from remote trace-agents via the ObserverProvider gRPC service.
	RemoteAgentRegistry remoteagentregistry.Component
	Logger              logger.Component
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
		storage:          newTimeSeriesStorage(),
		obsCh:            make(chan observation, 1000),
		maxEvents:        1000, // Keep last 1000 events for debugging
		anomalyDetection: NewAnomalyDetection(deps.Logger),
	}

	// If recorder is provided, wrap handles through it; otherwise use inner handle directly
	if deps.Recorder != nil {
		obs.handleFunc = deps.Recorder.GetHandle(obs.innerHandle)
	} else {
		obs.handleFunc = obs.innerHandle
	}

	cfg := deps.Cfg

	// Note: Parquet recording is now handled by the recorder component.
	// The recorder intercepts metrics at the handle level and writes to parquet.

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

	// Start trace/profile fetcher if traces or profiles collection is enabled
	fetcherConfig := DefaultFetcherConfig()
	fetcherConfig.Enabled = cfg.GetBool("observer.traces.enabled") || cfg.GetBool("observer.profiles.enabled")

	if fetcherConfig.Enabled {
		if interval := cfg.GetDuration("observer.traces.fetch_interval"); interval > 0 {
			fetcherConfig.TraceFetchInterval = interval
		}
		if interval := cfg.GetDuration("observer.profiles.fetch_interval"); interval > 0 {
			fetcherConfig.ProfileFetchInterval = interval
		}
		if batch := cfg.GetInt("observer.traces.max_fetch_batch"); batch > 0 {
			fetcherConfig.MaxTraceBatch = uint32(batch)
		}
		if batch := cfg.GetInt("observer.profiles.max_fetch_batch"); batch > 0 {
			fetcherConfig.MaxProfileBatch = uint32(batch)
		}

		fetchHandle := obs.GetHandle("trace-agent")
		obs.fetcher = newObserverFetcher(
			deps.RemoteAgentRegistry,
			fetchHandle,
			fetcherConfig,
		)
		obs.fetcher.Start()
		pkglog.Info("[observer] trace/profile fetcher started")
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
	handleFunc        observerdef.HandleFunc
	// eventBuffer stores recent events (markers) for debugging/dumping.
	// Ring buffer with maxEvents capacity.
	eventBuffer []observerdef.EventSignal
	maxEvents   int

	// Raw anomaly tracking for test bench display
	rawAnomalies     []observerdef.AnomalyOutput
	rawAnomalyMu     sync.RWMutex
	rawAnomalyWindow int64 // seconds to keep raw anomalies (0 = unlimited)
	// fetcher pulls traces/profiles from remote trace-agents
	fetcher          *observerFetcher
	anomalyDetection *AnomalyDetection
}

// run is the main dispatch loop, processing all observations sequentially.
func (o *observerImpl) run() {
	for obs := range o.obsCh {
		if obs.metric != nil {
			//	o.processMetric(obs.source, obs.metric)
			o.anomalyDetection.ProcessMetric(obs.metric)
		}
		if obs.log != nil {
			//	o.processLog(obs.source, obs.log)
			o.anomalyDetection.ProcessLog(obs.log)
		}
		if obs.trace != nil {
			//	o.processTrace(obs.source, obs.trace)
			o.anomalyDetection.ProcessTrace(obs.trace)
		}
		if obs.profile != nil {
			//o.processProfile(obs.source, obs.profile)
			o.anomalyDetection.ProcessProfile(obs.profile)
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

// RawAnomalies returns a copy of currently tracked raw anomalies.
// Implements observerdef.RawAnomalyState interface.
func (o *observerImpl) RawAnomalies() []observerdef.AnomalyOutput {
	o.rawAnomalyMu.RLock()
	defer o.rawAnomalyMu.RUnlock()

	result := make([]observerdef.AnomalyOutput, len(o.rawAnomalies))
	copy(result, o.rawAnomalies)
	return result
}

// GetHandle returns a lightweight handle for a named source.
func (o *observerImpl) GetHandle(name string) observerdef.Handle {
	return o.handleFunc(name)
}

func (o *observerImpl) innerHandle(name string) observerdef.Handle {
	return &handle{ch: o.obsCh, source: name}
}

// DumpMetrics writes all stored metrics to the specified file as JSON.
func (o *observerImpl) DumpMetrics(path string) error {
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
