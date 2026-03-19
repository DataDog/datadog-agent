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
	"sort"
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

// MetricDetection records when and how often a specific ground truth metric was detected.
type MetricDetection struct {
	Service             string  `json:"service"`
	Metric              string  `json:"metric"`
	Classification      string  `json:"classification"` // "tp" or "fp"
	Detected            bool    `json:"detected"`
	Count               int     `json:"count"`
	FirstSeenUnix       int64   `json:"first_seen_unix,omitempty"`
	DeltaFromDisruption float64 `json:"delta_from_disruption_sec,omitempty"`
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
	TPMetricsFound  []string `json:"tp_metrics_found"`
	TPMetricsMissed []string `json:"tp_metrics_missed"`
	FPMetricsFired  []string `json:"fp_metrics_fired"`

	// Per-metric detection timeline (TP and FP entries from ground truth).
	Detections            []MetricDetection `json:"detections,omitempty"`
	UnknownMetricCount    int               `json:"unknown_metric_count"`
	UnknownDetectionCount int               `json:"unknown_detection_count"`
}

// LoadMetricGroundTruth reads TP metric lists from ground_truth.json in the
// scenarios directory. This file is committed and keyed by scenario name,
// separate from metadata.json which is downloaded from S3.
func LoadMetricGroundTruth(scenariosDir, scenarioName string) (*MetricGroundTruth, error) {
	path := filepath.Join(scenariosDir, "ground_truth.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading ground truth %s: %w", path, err)
	}

	var allGT map[string]*MetricGroundTruth
	if err := json.Unmarshal(data, &allGT); err != nil {
		return nil, fmt.Errorf("parsing ground truth JSON: %w", err)
	}

	gt, ok := allGT[scenarioName]
	if !ok {
		return nil, fmt.Errorf("scenario %q not found in ground_truth.json", scenarioName)
	}

	return gt, nil
}

// LoadDisruptionStartUnix returns the disruption start timestamp in unix seconds
// from a scenario's metadata.json, or 0 if unavailable.
func LoadDisruptionStartUnix(scenariosDir, scenarioName string) int64 {
	sm, err := loadScoringMetadata(scenariosDir, scenarioName)
	if err != nil || len(sm.groundTruthTimestamps) == 0 {
		return 0
	}
	return sm.groundTruthTimestamps[0]
}

// ScoreMetrics classifies each anomaly period's metric as TP, FP, or unknown
// by matching against the ground truth metric lists.
//
// disruptionStartUnix is used to compute delta_from_disruption_sec on detections.
// Pass 0 if unavailable (deltas will be zero).
//
// Matching strategy: metric-only. An anomaly's Source (metric name) is checked
// against each ground truth entry's metrics list using substring matching with
// aggregate suffix stripping (e.g., "redis.cpu.sys:avg" matches "redis.cpu.sys").
func ScoreMetrics(output *ObserverOutput, gt *MetricGroundTruth, disruptionStartUnix int64) *MetricScoreResult {
	type gtEntry struct {
		service        string
		metric         string
		classification string // "tp" or "fp"
	}
	tpSet := make(map[string]gtEntry) // "service:metric" -> entry
	fpSet := make(map[string]gtEntry)
	allTPKeys := make(map[string]bool)

	for _, entry := range gt.TruePositives {
		for _, m := range entry.Metrics {
			key := entry.Service + ":" + m
			tpSet[key] = gtEntry{service: entry.Service, metric: m, classification: "tp"}
			allTPKeys[key] = true
		}
	}
	for _, entry := range gt.FalsePositives {
		for _, m := range entry.Metrics {
			key := entry.Service + ":" + m
			fpSet[key] = gtEntry{service: entry.Service, metric: m, classification: "fp"}
		}
	}

	result := &MetricScoreResult{
		TotalCount: len(output.AnomalyPeriods),
	}

	type metricHit struct {
		firstSeen int64
		count     int
	}
	foundTPKeys := make(map[string]*metricHit)
	firedFPKeys := make(map[string]*metricHit)

	for _, period := range output.AnomalyPeriods {
		source := period.metricSource()
		if source == "" {
			result.UnknownCount++
			result.UnknownDetectionCount++
			continue
		}

		matched := false
		// Check against TP metrics
		for key := range tpSet {
			if metricMatches(source, key) {
				result.TPCount++
				if hit, ok := foundTPKeys[key]; ok {
					hit.count++
				} else {
					foundTPKeys[key] = &metricHit{firstSeen: period.PeriodStart, count: 1}
				}
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
				if hit, ok := firedFPKeys[key]; ok {
					hit.count++
				} else {
					firedFPKeys[key] = &metricHit{firstSeen: period.PeriodStart, count: 1}
				}
				matched = true
				break
			}
		}
		if matched {
			continue
		}

		result.UnknownCount++
		result.UnknownDetectionCount++
	}

	// Collect found/missed TP metrics
	for key := range allTPKeys {
		if _, ok := foundTPKeys[key]; ok {
			result.TPMetricsFound = append(result.TPMetricsFound, key)
		} else {
			result.TPMetricsMissed = append(result.TPMetricsMissed, key)
		}
	}
	for key := range firedFPKeys {
		result.FPMetricsFired = append(result.FPMetricsFired, key)
	}

	// Build per-metric detection timeline
	var detections []MetricDetection

	for key, entry := range tpSet {
		d := MetricDetection{
			Service:        entry.service,
			Metric:         entry.metric,
			Classification: "tp",
		}
		if hit, ok := foundTPKeys[key]; ok {
			d.Detected = true
			d.Count = hit.count
			d.FirstSeenUnix = hit.firstSeen
			if disruptionStartUnix > 0 {
				d.DeltaFromDisruption = float64(hit.firstSeen - disruptionStartUnix)
			}
		}
		detections = append(detections, d)
	}

	for key, entry := range fpSet {
		d := MetricDetection{
			Service:        entry.service,
			Metric:         entry.metric,
			Classification: "fp",
		}
		if hit, ok := firedFPKeys[key]; ok {
			d.Detected = true
			d.Count = hit.count
			d.FirstSeenUnix = hit.firstSeen
			if disruptionStartUnix > 0 {
				d.DeltaFromDisruption = float64(hit.firstSeen - disruptionStartUnix)
			}
		}
		detections = append(detections, d)
	}

	// Stable sort: classification (fp < tp), then service, then metric
	sort.Slice(detections, func(i, j int) bool {
		if detections[i].Classification != detections[j].Classification {
			return detections[i].Classification < detections[j].Classification
		}
		if detections[i].Service != detections[j].Service {
			return detections[i].Service < detections[j].Service
		}
		return detections[i].Metric < detections[j].Metric
	})

	result.Detections = detections
	result.UnknownMetricCount = result.UnknownDetectionCount

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
// whose Source is the metric name. Falls back to parsing the Title field.
func (oc *ObserverCorrelation) metricSource() string {
	// Verbose output: anomalies are populated
	if len(oc.Anomalies) > 0 {
		return oc.Anomalies[0].Source
	}
	// Non-verbose: Title from passthrough has format "Passthrough[detector]: metric_name"
	if oc.Title != "" {
		if idx := strings.Index(oc.Title, "]: "); idx >= 0 {
			return oc.Title[idx+3:]
		}
	}
	return ""
}

// metricMatches checks if an anomaly source matches a ground truth key.
// key format: "service:metric_name" (e.g., "redis:redis.cpu.sys")
//
// Matching rules:
// 1. Strip aggregate suffix from source (e.g., ":avg", ":max")
// 2. Check if stripped source == metric or contains metric as substring
func metricMatches(source, key string) bool {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return false
	}
	metric := parts[1]

	// Strip aggregate suffix (e.g., "redis.cpu.sys:avg" -> "redis.cpu.sys")
	sourceName := source
	if colonIdx := strings.LastIndex(source, ":"); colonIdx >= 0 {
		suffix := source[colonIdx+1:]
		if len(suffix) <= 5 && !strings.Contains(suffix, ".") {
			sourceName = source[:colonIdx]
		}
	}

	return sourceName == metric || strings.Contains(sourceName, metric)
}
