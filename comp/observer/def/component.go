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

import "time"

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
	// Anomalies are detected anomaly events.
	Anomalies []AnomalyOutput
}

// MetricOutput is a timeseries value derived from log analysis.
// The storage keeps full summaries (min/max/sum/count) so aggregation
// is specified at read time, not write time.
type MetricOutput struct {
	Name  string
	Value float64
	Tags  []string
}

// AnomalyOutput is a detected anomaly event.
type AnomalyOutput struct {
	// Source identifies which metric/signal the anomaly is about (e.g., "network.retransmits").
	Source      string
	Title       string
	Description string
	Tags        []string
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

// Reporter receives reports and displays or delivers them.
type Reporter interface {
	// Name returns the reporter name for debugging.
	Name() string
	// Report delivers a report to its destination (stdout, file, webserver, etc).
	Report(report ReportOutput)
}

// MetricStorageReader provides read-only access to stored metric series.
// This allows reporters to query historical data for visualization.
type MetricStorageReader interface {
	// GetSeries returns a series by namespace, name, and tags.
	// Returns nil if series doesn't exist.
	GetSeries(namespace, name string, tags []string) *Series
	// AllSeries returns all series in a namespace.
	AllSeries(namespace string) []Series
}

// CorrelationState provides read access to active correlations.
// Reporters use this to display current correlation status.
type CorrelationState interface {
	// ActiveCorrelations returns currently detected correlation patterns.
	ActiveCorrelations() []ActiveCorrelation
}

// ActiveCorrelation represents a detected correlation pattern.
type ActiveCorrelation struct {
	Pattern     string          // pattern name, e.g. "kernel_bottleneck"
	Title       string          // display title, e.g. "Correlated: Kernel network bottleneck"
	Signals     []string        // contributing signal sources
	Anomalies   []AnomalyOutput // the actual anomalies that triggered this correlation
	FirstSeen   time.Time       // when pattern first matched
	LastUpdated time.Time       // most recent contributing signal
}
