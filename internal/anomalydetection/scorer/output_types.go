// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// JSON types for reading headless output produced by the anomalydetection testbench.
// These are the read-side DTOs only; the write side lives in the testbench.

package main

import (
	"encoding/json"
	"strings"
)

// ObserverOutput is the top-level JSON structure produced by headless mode.
type ObserverOutput struct {
	Metadata       ObserverMetadata      `json:"metadata"`
	AnomalyPeriods []ObserverCorrelation `json:"anomaly_periods"`
}

// ObserverMetadata describes the scenario and pipeline configuration.
type ObserverMetadata struct {
	Scenario            string                    `json:"scenario"`
	TimelineStart       int64                     `json:"timeline_start"`
	TimelineEnd         int64                     `json:"timeline_end"`
	DetectorsEnabled    []string                  `json:"detectors_enabled"`
	CorrelatorsEnabled  []string                  `json:"correlators_enabled"`
	TotalAnomalyPeriods int                       `json:"total_anomaly_periods"`
	ComponentConfigs    map[string]map[string]any `json:"component_configs,omitempty"`
	Stats               json.RawMessage           `json:"stats,omitempty"`
}

// ObserverCorrelation is one correlation cluster.
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

// metricSource extracts the metric name from an anomaly period.
func (oc *ObserverCorrelation) metricSource() string {
	if len(oc.Anomalies) > 0 {
		return oc.Anomalies[0].Source
	}
	if oc.Title != "" {
		if idx := strings.Index(oc.Title, "]: "); idx >= 0 {
			return oc.Title[idx+3:]
		}
	}
	return ""
}
