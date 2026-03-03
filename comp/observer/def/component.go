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

// LogDetector transforms observed logs into metrics and anomaly events.
// Implementations should be stateless and fast since they run synchronously
// on every observed log.
type LogDetector interface {
	// Name returns the detector name for debugging and logging.
	Name() string
	// Process examines a log and returns any detected signals.
	Process(log LogView) LogDetectionResult
}

// LogDetectionResult contains outputs from processing a log.
type LogDetectionResult struct {
	// Metrics are timeseries values derived from the log.
	Metrics []MetricOutput
	// Anomalies are directly detected anomalies (bypassing the metrics→MetricsDetector path).
	Anomalies []Anomaly
	// Used to debug anomaly detectors
	Telemetry []ObserverTelemetry
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

// AnomalyType distinguishes the source type of an anomaly.
type AnomalyType string

const (
	// AnomalyTypeMetric is a metric-based anomaly detected by a MetricsDetector.
	AnomalyTypeMetric AnomalyType = "metric"
	// AnomalyTypeLog is a log-based anomaly emitted directly by a log detector,
	// bypassing the metrics→MetricsDetector pipeline.
	AnomalyTypeLog AnomalyType = "log"
)

// Anomaly is a detected anomaly event.
// Anomalies represent a point in time where something anomalous was detected.
type Anomaly struct {
	// Type distinguishes log-based anomalies from metric-based ones.
	// Defaults to AnomalyTypeMetric if not set.
	Type AnomalyType
	// Source identifies which metric/signal or log source the anomaly is about.
	// For metric anomalies: the metric name (e.g., "network.retransmits:avg").
	// For log anomalies: a descriptive source identifier (e.g., "logs").
	Source MetricName
	// SourceSeriesID uniquely identifies the source series (namespace + name + tags).
	// Empty for log anomalies.
	SourceSeriesID SeriesID
	// DetectorName identifies which MetricsDetector or LogDetector produced this anomaly.
	DetectorName string
	Title        string
	Description  string
	Tags         []string
	Timestamp    int64    // when the anomaly was detected (unix seconds)
	Score        *float64 // confidence/severity score (nil if not available)
	// DebugInfo contains detector-specific debug information explaining the detection.
	DebugInfo *AnomalyDebugInfo
}

// AnomalyDebugInfo provides detailed information about why an anomaly was detected.
type AnomalyDebugInfo struct {
	// Baseline statistics
	BaselineStart  int64   // timestamp of baseline period start
	BaselineEnd    int64   // timestamp of baseline period end
	BaselineMean   float64 // mean of baseline (for CUSUM)
	BaselineMedian float64 // median of baseline
	BaselineStddev float64 // stddev of baseline (for CUSUM)
	BaselineMAD    float64 // MAD of baseline

	// Detection parameters
	Threshold      float64 // threshold that was crossed
	SlackParam     float64 // k parameter (CUSUM only)
	CurrentValue   float64 // value at detection time
	DeviationSigma float64 // how many sigmas from baseline

	// For CUSUM: the cumulative sum values leading up to detection
	CUSUMValues []float64 // S[t] values (may be truncated to last N points)
}

// ReportOutput is a processed summary from correlators.
// It represents clustered, filtered, or summarized anomaly information ready for display.
type ReportOutput struct {
	Title    string
	Body     string
	Metadata map[string]string
}

// Series is a time series with simple timestamp/value points.
// This is the simplified view passed to MetricsDetector.
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

// Describes a telemetry event that is emitted by the observer.
// This could be a metric or a log for instance.
type ObserverTelemetry struct {
	DetectorName string
	Metric       MetricView
	Log          LogView
}

// MetricsDetector analyzes a time series for anomalies.
// Implementations should be stateless and just do math on the points.
type MetricsDetector interface {
	// Name returns the analysis name for debugging.
	Name() string
	// Detect examines a series and returns any detected anomalies.
	Detect(series Series) MetricsDetectionResult
}

// MetricsDetectionResult contains outputs from metrics detection.
type MetricsDetectionResult struct {
	Anomalies []Anomaly
	// Used to debug anomaly detectors
	Telemetry []ObserverTelemetry
}

// Correlator accumulates anomaly events and produces reports.
// Correlators are stateful and cluster/filter/summarize anomaly streams.
type Correlator interface {
	// Name returns the correlator name for debugging.
	Name() string
	// Process receives an anomaly event for accumulation.
	Process(anomaly Anomaly)
	// Flush processes accumulated anomalies and returns reports.
	Flush() []ReportOutput
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
	MetricNames []MetricName
	Anomalies   []Anomaly // the actual anomalies that triggered this correlation
	FirstSeen   int64     // when pattern first matched (unix seconds, from data)
	LastUpdated int64     // most recent contributing signal (unix seconds, from data)
}

// RawAnomalyState provides read access to raw anomalies before correlation processing.
// Used by test bench reporters to display individual detector outputs.
type RawAnomalyState interface {
	// RawAnomalies returns all anomalies detected by MetricsDetector implementations.
	RawAnomalies() []Anomaly
}
