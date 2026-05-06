// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package output defines the JSON contract for observer evaluation output.
// The testbench produces this format and the scorer consumes it.
package output

// ObserverOutput is the top-level JSON structure produced by headless mode.
// The Go field uses ObserverCorrelation (domain type) while the JSON
// key is "anomaly_periods" — the consumer-facing name that describes what each
// entry represents: a time period during which correlated anomalies were active.
type ObserverOutput struct {
	Metadata       ObserverMetadata      `json:"metadata"`
	AnomalyPeriods []ObserverCorrelation `json:"anomaly_periods"`
}

// ObserverMetadata describes the scenario and pipeline configuration.
type ObserverMetadata struct {
	Scenario            string   `json:"scenario"`
	TimelineStart       int64    `json:"timeline_start"`
	TimelineEnd         int64    `json:"timeline_end"`
	DetectorsEnabled    []string `json:"detectors_enabled"`
	CorrelatorsEnabled  []string `json:"correlators_enabled"`
	TotalAnomalyPeriods int      `json:"total_anomaly_periods"`
	// ComponentConfigs holds the active configuration of every component in the
	// --config params-file format: { "bocpd": { "enabled": true, "hazard": 0.05, ... }, ... }.
	// This can be copy-pasted into a file and passed to --config to reproduce the run.
	ComponentConfigs map[string]map[string]any `json:"component_configs,omitempty"`
	Stats            *ReplayStats              `json:"stats,omitempty"`
}

// ObserverCorrelation is one correlation cluster.
// Always includes the time span (pattern, period_start, period_end).
// Verbose mode adds title, message, tags, member_series, and nested anomalies.
type ObserverCorrelation struct {
	Pattern      string            `json:"pattern"`
	PeriodStart  int64             `json:"period_start"`
	PeriodEnd    int64             `json:"period_end"`
	Title        string            `json:"title,omitempty"`
	Message      string            `json:"message,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	MemberSeries []string          `json:"member_series,omitempty"`
	Anomalies    []ObserverAnomaly `json:"anomalies,omitempty"`
}

// ObserverAnomaly is a single anomaly nested inside a correlation (verbose only).
type ObserverAnomaly struct {
	Timestamp      int64  `json:"timestamp"`
	Source         string `json:"source"`
	SourceSeriesID string `json:"source_series_id"`
	Detector       string `json:"detector"`
}

// DetectorProcessingStats holds aggregate processing-time statistics for a single
// detector (or extractor / correlator) across all advance calls during a replay.
// Times are in nanoseconds.
type DetectorProcessingStats struct {
	Name string `json:"name"`
	// Kind is the component kind: "detector", "correlator", or "extractor".
	Kind     string  `json:"kind"`
	Count    int     `json:"count"`
	AvgNs    float64 `json:"avg_ns"`
	MedianNs float64 `json:"median_ns"`
	P99Ns    float64 `json:"p99_ns"`
	TotalNs  float64 `json:"total_ns"`
}

// ReplayStats aggregates all statistics produced during a replay run.
type ReplayStats struct {
	// DetectorStats holds per-detector processing-time statistics keyed by detector name.
	DetectorStats map[string]DetectorProcessingStats `json:"detector_stats,omitempty"`
	// InputMetricsCount is the total number of metric data points (samples) in the scenario.
	InputMetricsCount int64 `json:"input_metrics_count"`
	// InputMetricsCardinality is the number of unique metric series (name + tag combinations).
	InputMetricsCardinality int `json:"input_metrics_cardinality"`
	// InputLogsCount is the number of raw log entries present in the scenario.
	InputLogsCount int `json:"input_logs_count"`
	// InputAnomaliesCount is the total number of anomalies produced by detectors,
	// which is the input volume processed by correlators.
	InputAnomaliesCount int `json:"input_anomalies_count"`
}
