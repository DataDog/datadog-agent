// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observer provides a component for observing data flowing through the agent.
//
// The observer component allows other components to report metrics, logs, and other
// signals for sampling and analysis. It provides lightweight handles that can be
// passed to data pipelines without adding significant overhead.
package observer

// team: agent-metric-pipelines

// Component is the central observer that receives data via handles.
type Component interface {
	// GetHandle returns a lightweight handle for a named source.
	// The source name is used to identify where observations originate.
	GetHandle(name string) Handle

	// DumpMetrics writes all stored metrics to the specified file (for debugging).
	DumpMetrics(path string) error
}

// Handle is the lightweight observation interface passed to other components.
type Handle interface {
	// ObserveMetric observes a DogStatsD metric sample.
	ObserveMetric(sample MetricView)

	// ObserveLog observes a log message.
	ObserveLog(msg LogView)
}

// HandleFunc is a function that returns a handle for a named source.
type HandleFunc func(name string) Handle

// MetricView provides read-only access to a metric sample.
//
// This interface exists to prevent data races. The underlying metric data may be
// reused immediately after ObserveMetric returns, so implementations must not
// store the MetricView itself. Copy any needed values synchronously.
type MetricView interface {
	GetName() string
	GetValue() float64
	GetRawTags() []string
	GetTimestamp() float64
	GetSampleRate() float64
}

// LogView provides read-only access to a log message.
//
// This interface exists to prevent data races. Implementations must not store
// the LogView itself. Copy any needed values synchronously.
type LogView interface {
	GetContent() []byte
	GetStatus() string
	GetTags() []string
	GetHostname() string
}

// LogProcessor transforms observed logs into metrics and anomaly events.
// Implementations should be stateless and fast since they run synchronously
// on every observed log.
type LogProcessor interface {
	// Name returns the processor name for debugging and logging.
	Name() string
	// Process examines a log and returns any detected signals.
	Process(log LogView) LogProcessorResult
}

// LogProcessorResult contains outputs from processing a log.
type LogProcessorResult struct {
	// Metrics are timeseries values derived from the log.
	Metrics []MetricOutput
}

// MetricOutput is a timeseries value derived from log analysis.
// The storage keeps full summaries (min/max/sum/count) so aggregation
// is specified at read time, not write time.
type MetricOutput struct {
	Name  string
	Value float64
	Tags  []string
}

// MetricName is a human-readable metric identifier (e.g., "cpu.user:avg").
// Multiple series can share a MetricName if they differ by tags.
type MetricName string

// SeriesID uniquely identifies a time series (namespace + name + tags).
type SeriesID string

// SignalSource identifies a signal stream source (metric or event-style source name).
type SignalSource string

// EventSource identifies a discrete event source.
type EventSource string

// TimeRange represents a time period covered by an analysis.
type TimeRange struct {
	Start int64 // earliest timestamp in analyzed data (unix seconds)
	End   int64 // latest timestamp in analyzed data (unix seconds)
}

// AnomalyOutput is a detected anomaly event.
// Anomalies represent a point in time where something anomalous was detected.
type AnomalyOutput struct {
	// Source identifies which metric/signal the anomaly is about (e.g., "network.retransmits").
	Source MetricName
	// SourceSeriesID uniquely identifies the source series (namespace + name + tags).
	SourceSeriesID SeriesID
	// AnalyzerName identifies which TimeSeriesAnalysis or LogProcessor produced this anomaly.
	AnalyzerName string
	Title        string
	Description  string
	Tags         []string
	Timestamp    int64     // when the anomaly was detected (unix seconds)
	TimeRange    TimeRange // period covered by the analysis that produced this anomaly
	// DebugInfo contains analyzer-specific debug information explaining the detection.
	DebugInfo *AnomalyDebugInfo
}

// AnomalyDebugInfo provides detailed information about why an anomaly was detected.
type AnomalyDebugInfo struct {
	// Baseline statistics
	BaselineStart  int64   // timestamp of baseline period start
	BaselineEnd    int64   // timestamp of baseline period end
	BaselineMean   float64 // mean of baseline (for CUSUM)
	BaselineMedian float64 // median of baseline (for robust z-score)
	BaselineStddev float64 // stddev of baseline (for CUSUM)
	BaselineMAD    float64 // MAD of baseline (for robust z-score)

	// Detection parameters
	Threshold      float64 // threshold that was crossed
	SlackParam     float64 // k parameter (CUSUM only)
	CurrentValue   float64 // value at detection time
	DeviationSigma float64 // how many sigmas from baseline

	// For CUSUM: the cumulative sum values leading up to detection
	CUSUMValues []float64 // S[t] values (may be truncated to last N points)
}

// ReportOutput is a processed summary from anomaly processors.
// It represents clustered, filtered, or summarized anomaly information ready for display.
type ReportOutput struct {
	Title    string
	Body     string
	Metadata map[string]string
}

// Series is a time series with simple timestamp/value points.
// This is the simplified view passed to TimeSeriesAnalysis.
type Series struct {
	Namespace string
	Name      string
	Tags      []string
	Points    []Point
}

// Point is a single timestamp/value pair.
type Point struct {
	Timestamp int64
	Value     float64
}

// TimeSeriesAnalysis analyzes a time series for anomalies.
// Implementations should be stateless and just do math on the points.
type TimeSeriesAnalysis interface {
	// Name returns the analysis name for debugging.
	Name() string
	// Analyze examines a series and returns any detected anomalies.
	Analyze(series Series) TimeSeriesAnalysisResult
}

// TimeSeriesAnalysisResult contains outputs from time series analysis.
type TimeSeriesAnalysisResult struct {
	Anomalies []AnomalyOutput
}

// AnomalyProcessor accumulates anomaly events and produces reports.
// Unlike analyses, processors are stateful and cluster/filter/summarize anomaly streams.
type AnomalyProcessor interface {
	// Name returns the processor name for debugging.
	Name() string
	// Process receives an anomaly event for accumulation.
	Process(anomaly AnomalyOutput)
	// Flush processes accumulated anomalies and returns reports.
	Flush() []ReportOutput
}

// SignalEmitter produces point signals from time series data.
// This replaces TimeSeriesAnalysis with a simpler point-based output model.
// Layer 1: Emitters detect anomalous conditions and emit signals immediately.
type SignalEmitter interface {
	// Name returns the emitter name for debugging.
	Name() string
	// Emit analyzes a series and returns point signals for anomalous conditions.
	Emit(series Series) []Signal
}

// SignalProcessor processes signal streams and maintains internal state.
// This replaces AnomalyProcessor, using a pull-based model where reporters
// query state via typed interfaces (e.g., CorrelationState, ClusterState).
// Layer 2: Processors correlate, cluster, or filter signals.
type SignalProcessor interface {
	// Name returns the processor name for debugging.
	Name() string
	// Process receives a signal for accumulation/correlation.
	Process(signal Signal)
	// Flush updates internal state. Reporters pull state via typed interfaces.
	Flush()
}

// EventSignalReceiver is an optional interface for processors that accept discrete event signals.
// Events like container OOMs, restarts, and lifecycle transitions are routed here
// instead of being processed as logs (no metric derivation).
type EventSignalReceiver interface {
	// AddEventSignal adds a discrete event signal for correlation context.
	AddEventSignal(signal EventSignal)
}

// EventSignal represents a discrete event used as correlation evidence or annotation.
// Unlike anomalies (which are detected from time series analysis), event signals are
// explicit events such as container OOMs, restarts, or lifecycle transitions.
type EventSignal struct {
	Source    EventSource
	Timestamp int64    // when the event occurred (unix seconds)
	Tags      []string // event tags for filtering/grouping
	Message   string   // optional human-readable description
}

// Reporter receives reports and displays or delivers them.
type Reporter interface {
	// Name returns the reporter name for debugging.
	Name() string
	// Report delivers a report to its destination (stdout, file, webserver, etc).
	Report(report ReportOutput)
}

// CorrelationState provides read access to active correlations.
// Reporters use this to display current correlation status.
type CorrelationState interface {
	// ActiveCorrelations returns currently detected correlation patterns.
	ActiveCorrelations() []ActiveCorrelation
}

// ActiveCorrelation represents a detected correlation pattern.
type ActiveCorrelation struct {
	Pattern string // pattern name, e.g. "kernel_bottleneck"
	Title   string // display title, e.g. "Correlated: Kernel network bottleneck"
	// MemberSeriesIDs are the concrete series identities participating in this correlation.
	MemberSeriesIDs []SeriesID
	// MetricNames are display-oriented metric names participating in this correlation.
	MetricNames  []MetricName
	Anomalies    []AnomalyOutput // the actual anomalies that triggered this correlation
	EventSignals []EventSignal   // discrete events (EventSignal-based, for processors using AddEventSignal)
	FirstSeen    int64           // when pattern first matched (unix seconds, from data)
	LastUpdated  int64           // most recent contributing signal (unix seconds, from data)
}

// ClusterState provides read access to clustered signal regions.
// TimeClusterer implements this interface to expose grouped signals.
type ClusterState interface {
	// ActiveRegions returns currently active signal regions.
	ActiveRegions() []SignalRegion
}

// SignalRegion represents a time region with grouped point signals.
// Created by TimeClusterer from point signals that occur close together in time.
type SignalRegion struct {
	Source    SignalSource
	TimeRange TimeRange // start and end of the region
	Signals   []Signal  // contributing point signals
}

// Signal represents a point-in-time observation of interest.
// Signals unify metric anomalies and discrete events into a common type.
type Signal struct {
	Source    SignalSource
	Timestamp int64    // unix timestamp (seconds)
	Tags      []string // metadata tags
	Message   string   // optional human-readable description (for events)

	// Optional fields (algorithm-dependent)
	Value float64  // current metric value (if applicable)
	Score *float64 // confidence/severity (nil if algorithm doesn't provide)
}

// RawAnomalyState provides read access to raw anomalies before correlation processing.
// Used by test bench reporters to display individual analyzer outputs.
type RawAnomalyState interface {
	// RawAnomalies returns all anomalies detected by TimeSeriesAnalysis implementations.
	RawAnomalies() []AnomalyOutput
}

// SeriesFilter specifies criteria for selecting series.
type SeriesFilter struct {
	Namespace   string            // exact match (empty = any)
	NamePattern string            // prefix match (empty = any)
	TagMatchers map[string]string // required tag key=value pairs
}

// SeriesKey identifies a specific series.
type SeriesKey struct {
	Namespace string
	Name      string
	Tags      []string
}

// StorageReader provides read access to time series data.
// Analyses use this to pull whatever data they need.
type StorageReader interface {
	// ListSeries returns keys of all series matching the filter.
	ListSeries(filter SeriesFilter) []SeriesKey

	// GetSeriesByKey returns the full series for a key.
	GetSeriesByKey(key SeriesKey, agg Aggregate) *Series

	// GetSeriesRange returns points within a time range [start, end].
	GetSeriesRange(key SeriesKey, start, end int64, agg Aggregate) *Series

	// ReadSince returns points with timestamp > cursor, plus the new cursor position.
	// Use cursor=0 to read all points.
	ReadSince(key SeriesKey, cursor int64, agg Aggregate) (points []Point, newCursor int64)
}

// Aggregate specifies which statistic to extract from summary stats.
type Aggregate int

const (
	AggregateAverage Aggregate = iota
	AggregateSum
	AggregateCount
	AggregateMin
	AggregateMax
)

// MultiSeriesAnalysis is the flexible analysis interface where analyses pull data from storage.
// This supports multivariate analysis across multiple series.
type MultiSeriesAnalysis interface {
	Name() string

	// Analyze is called periodically by the scheduler.
	// The analysis queries storage for whatever data it needs.
	// dataTime is the current data timestamp (for determinism - only read data <= dataTime).
	Analyze(storage StorageReader, dataTime int64) []AnomalyOutput
}
