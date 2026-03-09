// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MetricGroundTruth holds the TP/FP metric lists from a scenario's metadata.json.
type MetricGroundTruth struct {
	TruePositives  []MetricGroundTruthEntry `json:"true_positives"`
	FalsePositives []MetricGroundTruthEntry `json:"false_positives"`
}

// MetricGroundTruthEntry is one service's labeled metrics.
type MetricGroundTruthEntry struct {
	Service string   `json:"service"`
	Metrics []string `json:"metrics"`
}

// MetricScoreResult contains per-metric TP/FP classification results.
type MetricScoreResult struct {
	// Counts of anomaly periods whose metric matched a TP, FP, or neither.
	TPCount      int `json:"tp_count"`
	FPCount      int `json:"fp_count"`
	UnknownCount int `json:"unknown_count"`
	TotalCount   int `json:"total_count"`

	// Precision = TP / (TP + FP), ignoring unknowns.
	MetricPrecision float64 `json:"metric_precision"`
	// Recall = matched TPs / total TP metrics in ground truth.
	MetricRecall float64 `json:"metric_recall"`
	// F1 = harmonic mean of MetricPrecision and MetricRecall.
	MetricF1 float64 `json:"metric_f1"`

	// Which TP metrics were detected and which were missed.
	TPMetricsFound   []string `json:"tp_metrics_found"`
	TPMetricsMissed  []string `json:"tp_metrics_missed"`
	FPMetricsFired   []string `json:"fp_metrics_fired"`
}

// LoadMetricGroundTruth reads TP/FP metric lists from a scenario's metadata.json.
func LoadMetricGroundTruth(scenariosDir, scenarioName string) (*MetricGroundTruth, error) {
	path := filepath.Join(scenariosDir, scenarioName, "metadata.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading metadata %s: %w", path, err)
	}

	var gt MetricGroundTruth
	if err := json.Unmarshal(data, &gt); err != nil {
		return nil, fmt.Errorf("parsing metadata JSON: %w", err)
	}

	return &gt, nil
}

// ScoreMetrics classifies each anomaly period's metric as TP, FP, or unknown
// by matching against the ground truth metric lists.
//
// Matching strategy: an anomaly's Source (metric name) is checked against each
// ground truth entry's metrics list. The match is substring-based to handle
// tag suffixes (e.g., anomaly source "redis.cpu.sys:avg" matches ground truth
// "redis.cpu.sys").
func ScoreMetrics(output *ObserverOutput, gt *MetricGroundTruth) *MetricScoreResult {
	// Build lookup sets: service:metric → "tp" or "fp"
	tpSet := make(map[string]bool) // "service:metric" → true
	fpSet := make(map[string]bool)
	allTPKeys := make(map[string]bool) // all TP service:metric keys

	for _, entry := range gt.TruePositives {
		for _, m := range entry.Metrics {
			key := entry.Service + ":" + m
			tpSet[key] = true
			allTPKeys[key] = true
		}
	}
	for _, entry := range gt.FalsePositives {
		for _, m := range entry.Metrics {
			fpSet[entry.Service+":"+m] = true
		}
	}

	result := &MetricScoreResult{
		TotalCount: len(output.AnomalyPeriods),
	}

	foundTPKeys := make(map[string]bool)
	firedFPKeys := make(map[string]bool)

	for _, period := range output.AnomalyPeriods {
		source := period.metricSource()
		if source == "" {
			result.UnknownCount++
			continue
		}

		matched := false
		// Check against TP metrics
		for key := range tpSet {
			if metricMatches(source, key) {
				result.TPCount++
				foundTPKeys[key] = true
				matched = true
				break
			}
		}
		if matched {
			continue
		}

		// Check against FP metrics
		for key := range fpSet {
			if metricMatches(source, key) {
				result.FPCount++
				firedFPKeys[key] = true
				matched = true
				break
			}
		}
		if !matched {
			result.UnknownCount++
		}
	}

	// Collect found/missed TP metrics
	for key := range allTPKeys {
		if foundTPKeys[key] {
			result.TPMetricsFound = append(result.TPMetricsFound, key)
		} else {
			result.TPMetricsMissed = append(result.TPMetricsMissed, key)
		}
	}
	for key := range firedFPKeys {
		result.FPMetricsFired = append(result.FPMetricsFired, key)
	}

	// Compute precision/recall/F1
	labeled := result.TPCount + result.FPCount
	if labeled > 0 {
		result.MetricPrecision = float64(result.TPCount) / float64(labeled)
	}
	if len(allTPKeys) > 0 {
		result.MetricRecall = float64(len(foundTPKeys)) / float64(len(allTPKeys))
	}
	if result.MetricPrecision+result.MetricRecall > 0 {
		result.MetricF1 = 2 * result.MetricPrecision * result.MetricRecall / (result.MetricPrecision + result.MetricRecall)
	}

	return result
}

// metricSource extracts the metric name from an anomaly period.
// For passthrough correlator output, each period has exactly one anomaly
// whose Source is the metric name. For verbose output, we use the first anomaly.
// Falls back to parsing the pattern name.
func (oc *ObserverCorrelation) metricSource() string {
	// Verbose output: anomalies are populated
	if len(oc.Anomalies) > 0 {
		return oc.Anomalies[0].Source
	}
	// Non-verbose: try MemberSeries (contains SourceSeriesID, not metric name)
	// The Title field from passthrough has format "Passthrough[detector]: metric_name"
	if oc.Title != "" {
		if idx := strings.Index(oc.Title, "]: "); idx >= 0 {
			return oc.Title[idx+3:]
		}
	}
	return ""
}

// metricMatches checks if an anomaly source matches a ground truth key.
// key format: "service:metric_name" (e.g., "redis:redis.cpu.sys")
// source may have tag suffixes like ":avg" or contain service prefixes.
//
// Matching rules:
// 1. Exact metric name match (source == metric or source starts with metric+":")
// 2. Source contains the metric name as a substring
func metricMatches(source, key string) bool {
	// key is "service:metric" — extract just the metric part
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return false
	}
	metric := parts[1]

	// Strip any aggregate suffix from source (e.g., "redis.cpu.sys:avg" → "redis.cpu.sys")
	sourceName := source
	if colonIdx := strings.LastIndex(source, ":"); colonIdx >= 0 {
		candidate := source[:colonIdx]
		// Only strip if the suffix looks like an aggregate (short, no dots)
		suffix := source[colonIdx+1:]
		if len(suffix) <= 5 && !strings.Contains(suffix, ".") {
			sourceName = candidate
		}
	}

	return sourceName == metric || strings.Contains(sourceName, metric)
}
