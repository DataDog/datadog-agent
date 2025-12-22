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

// Analysis transforms observed logs into metrics and events.
// Implementations should be stateless and fast since they run synchronously
// on every observed log.
type Analysis interface {
	// Name returns the analysis name for debugging and logging.
	Name() string
	// Analyze examines a log and returns any detected signals.
	Analyze(log LogView) AnalysisResult
}

// AnalysisResult contains outputs from analyzing a log.
type AnalysisResult struct {
	// Metrics are timeseries values derived from the log.
	Metrics []MetricOutput
	// Anomalies are detected anomaly events.
	Anomalies []AnomalyOutput
}

// MetricOutput is a timeseries value derived from log analysis.
type MetricOutput struct {
	Name  string
	Value float64
	Tags  []string
}

// AnomalyOutput is a detected anomaly event.
type AnomalyOutput struct {
	Title       string
	Description string
	Tags        []string
}

// SeriesStats contains accumulated statistics for a time series.
type SeriesStats struct {
	Namespace string
	Name      string
	Tags      []string
	Points    []StatPoint
}

// StatPoint holds summary statistics for a single time bucket.
type StatPoint struct {
	Timestamp int64 // Unix seconds (bucket start)
	Sum       float64
	Count     int64
	Min       float64
	Max       float64
}

// Value returns the mean for this point.
func (p *StatPoint) Value() float64 {
	if p.Count == 0 {
		return 0
	}
	return p.Sum / float64(p.Count)
}

// TimeSeriesAnalysis analyzes a time series for anomalies.
// Implementations should be stateless and fast since they run synchronously.
type TimeSeriesAnalysis interface {
	// Name returns the analysis name for debugging.
	Name() string
	// Analyze examines a series and returns any detected anomalies.
	Analyze(series *SeriesStats) TimeSeriesAnalysisResult
}

// TimeSeriesAnalysisResult contains outputs from time series analysis.
type TimeSeriesAnalysisResult struct {
	Anomalies []AnomalyOutput
}
