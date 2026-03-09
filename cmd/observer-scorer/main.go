// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main provides a standalone scorer for observer eval output.
// It reads a headless output JSON, resolves ground truth from the scenario's
// metadata.json, and computes a Gaussian F1 score.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	observerimpl "github.com/DataDog/datadog-agent/comp/observer/impl"
)

// combinedResult is the JSON output when both timestamp and metric scoring are enabled.
type combinedResult struct {
	observerimpl.ScoreResult
	Metrics *observerimpl.MetricScoreResult `json:"metrics,omitempty"`
}

func main() {
	outputPath := flag.String("input", "", "Path to headless output JSON to score (required)")
	scenariosDir := flag.String("scenarios-dir", "./comp/observer/scenarios", "Directory containing scenario subdirectories (for metadata.json lookup)")
	groundTruthTS := flag.Int64("ground-truth-ts", 0, "Ground truth disruption onset timestamp in unix seconds (overrides metadata.json)")
	sigma := flag.Float64("sigma", 30.0, "Gaussian width in seconds")
	jsonOutput := flag.Bool("json", false, "Output result as JSON")
	scoreMetrics := flag.Bool("score-metrics", false, "Also score per-metric TP/FP classification against metadata ground truth")
	flag.Parse()

	if *outputPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: observer-scorer --input <path> [--scenarios-dir <dir>] [--ground-truth-ts <unix>] [--sigma <seconds>] [--json] [--score-metrics]\n")
		os.Exit(1)
	}

	var gtTimestamps []int64
	if *groundTruthTS != 0 {
		gtTimestamps = []int64{*groundTruthTS}
	}

	result, err := observerimpl.ScoreOutputFile(*outputPath, gtTimestamps, *scenariosDir, *sigma)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Scoring failed: %v\n", err)
		os.Exit(1)
	}

	// Optional: per-metric TP/FP scoring
	var metricResult *observerimpl.MetricScoreResult
	if *scoreMetrics {
		metricResult, err = scoreMetricsFromFile(*outputPath, *scenariosDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Metric scoring failed: %v\n", err)
			// Non-fatal: continue with timestamp score
		}
	}

	if *jsonOutput {
		out := combinedResult{ScoreResult: *result, Metrics: metricResult}
		data, err := json.Marshal(out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "JSON marshal failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}

	fmt.Printf("Gaussian F1 Score\n")
	fmt.Printf("  Input:       %s\n", *outputPath)
	fmt.Printf("  Sigma:       %.1fs\n", *sigma)
	fmt.Printf("  Predictions: %d scored, %d warmup filtered, %d cascading filtered\n",
		result.NumPredictions, result.NumFilteredWarmup, result.NumFilteredCascading)
	fmt.Println()
	fmt.Printf("  F1:        %.4f\n", result.F1)
	fmt.Printf("  Precision: %.4f\n", result.Precision)
	fmt.Printf("  Recall:    %.4f\n", result.Recall)
	fmt.Printf("  TP: %.4f  FP: %.4f  FN: %.4f\n", result.TP, result.FP, result.FN)

	if metricResult != nil {
		fmt.Printf("\nMetric-Level Classification\n")
		fmt.Printf("  TP: %d  FP: %d  Unknown: %d  (of %d periods)\n",
			metricResult.TPCount, metricResult.FPCount, metricResult.UnknownCount, metricResult.TotalCount)
		fmt.Printf("  Metric Precision: %.4f  Recall: %.4f  F1: %.4f\n",
			metricResult.MetricPrecision, metricResult.MetricRecall, metricResult.MetricF1)
		if len(metricResult.TPMetricsMissed) > 0 {
			fmt.Printf("  Missed TPs: %v\n", metricResult.TPMetricsMissed)
		}
		if len(metricResult.FPMetricsFired) > 0 {
			fmt.Printf("  FP metrics fired: %v\n", metricResult.FPMetricsFired)
		}
	}
}

// scoreMetricsFromFile loads the output JSON and ground truth, then runs metric scoring.
func scoreMetricsFromFile(outputPath, scenariosDir string) (*observerimpl.MetricScoreResult, error) {
	data, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("reading output: %w", err)
	}

	var output observerimpl.ObserverOutput
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, fmt.Errorf("parsing output JSON: %w", err)
	}

	if output.Metadata.Scenario == "" {
		return nil, fmt.Errorf("output missing scenario name in metadata")
	}

	gt, err := observerimpl.LoadMetricGroundTruth(scenariosDir, output.Metadata.Scenario)
	if err != nil {
		return nil, fmt.Errorf("loading metric ground truth: %w", err)
	}

	if len(gt.TruePositives) == 0 && len(gt.FalsePositives) == 0 {
		return nil, fmt.Errorf("no TP/FP metrics in metadata.json for %s", output.Metadata.Scenario)
	}

	return observerimpl.ScoreMetrics(&output, gt), nil
}
