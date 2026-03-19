// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main provides a standalone scorer for observer eval output.
// It has two mutually exclusive scoring modes:
//
//   - Default (F1 scoring): Reads a headless output JSON produced with time_cluster,
//     resolves ground truth timestamps from the scenario's metadata.json, and computes
//     a Gaussian F1 score measuring whether the disruption was detected at the right time.
//
//   - --score-tp (TP metric scoring): Reads a headless output JSON produced with the
//     passthrough correlator, loads metric ground truth from ground_truth.json, and
//     classifies each detected anomaly by metric name match. Measures whether the
//     right metrics were flagged. Requires passthrough output because it extracts
//     metric names from each anomaly's Source field.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	observerimpl "github.com/DataDog/datadog-agent/comp/observer/impl"
)

func main() {
	outputPath := flag.String("input", "", "Path to headless output JSON to score (required)")
	scenariosDir := flag.String("scenarios-dir", "./comp/observer/scenarios", "Directory containing scenario subdirectories (for metadata.json lookup)")
	groundTruthTS := flag.Int64("ground-truth-ts", 0, "Ground truth disruption onset timestamp in unix seconds (overrides metadata.json)")
	sigma := flag.Float64("sigma", 30.0, "Gaussian width in seconds")
	jsonOutput := flag.Bool("json", false, "Output result as JSON")
	scoreTP := flag.Bool("score-tp", false, "Score true positive detection using metric ground truth from ground_truth.json. Requires passthrough correlator output.")
	flag.Parse()

	if *outputPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: observer-scorer --input <path> [--scenarios-dir <dir>] [--ground-truth-ts <unix>] [--sigma <seconds>] [--score-tp] [--json]\n")
		os.Exit(1)
	}

	var gtTimestamps []int64
	if *groundTruthTS != 0 {
		gtTimestamps = []int64{*groundTruthTS}
	}

	// When --score-tp is used, only run metric TP scoring (skip Gaussian F1).
	if *scoreTP {
		metricResult, err := scoreMetricTP(*outputPath, *scenariosDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Metric TP scoring failed: %v\n", err)
			os.Exit(1)
		}
		if *jsonOutput {
			data, err := json.Marshal(metricResult)
			if err != nil {
				fmt.Fprintf(os.Stderr, "JSON marshal failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(string(data))
			return
		}
		printMetricTPScore(metricResult)
		return
	}

	result, err := observerimpl.ScoreOutputFile(*outputPath, gtTimestamps, *scenariosDir, *sigma)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Scoring failed: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		data, err := json.Marshal(result)
		if err != nil {
			fmt.Fprintf(os.Stderr, "JSON marshal failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}

	// Text output: timestamp score
	fmt.Printf("Gaussian F1 Score\n")
	fmt.Printf("  Input:       %s\n", *outputPath)
	fmt.Printf("  Sigma:       %.1fs\n", *sigma)
	fmt.Printf("  Predictions: %d scored, %d warmup filtered, %d post-onset ignored\n",
		result.NumPredictions, result.NumFilteredWarmup, result.NumFilteredCascading)
	fmt.Println()
	fmt.Printf("  F1:        %.4f\n", result.F1)
	fmt.Printf("  Precision: %.4f\n", result.Precision)
	fmt.Printf("  Recall:    %.4f\n", result.Recall)
	fmt.Printf("  TP: %.4f  FP: %.4f  FN: %.4f\n", result.TP, result.FP, result.FN)
}

func printMetricTPScore(metricResult *observerimpl.MetricScoreResult) {
	if metricResult == nil {
		return
	}
	fmt.Printf("Metric TP Score\n")
	fmt.Printf("  Total anomaly periods: %d\n", metricResult.TotalCount)
	fmt.Printf("  TP: %d  Unknown: %d\n", metricResult.TPCount, metricResult.UnknownCount)
	fmt.Println()
	fmt.Printf("  Metric F1:        %.4f\n", metricResult.MetricF1)
	fmt.Printf("  Metric Precision: %.4f\n", metricResult.MetricPrecision)
	fmt.Printf("  Metric Recall:    %.4f\n", metricResult.MetricRecall)
	fmt.Println()
	if len(metricResult.TPMetricsFound) > 0 {
		fmt.Printf("  TP metrics found:  %v\n", metricResult.TPMetricsFound)
	}
	if len(metricResult.TPMetricsMissed) > 0 {
		fmt.Printf("  TP metrics missed: %v\n", metricResult.TPMetricsMissed)
	}
	if len(metricResult.Detections) > 0 {
		fmt.Println()
		fmt.Printf("  Detections:\n")
		for _, d := range metricResult.Detections {
			status := "MISS"
			if d.Detected {
				status = fmt.Sprintf("HIT (count=%d, first=%.0fs after disruption)", d.Count, d.DeltaFromDisruption)
			}
			fmt.Printf("    [%s] %s/%s: %s\n", d.Classification, d.Service, d.Metric, status)
		}
	}
}

// scoreMetricTP loads the output and ground truth, then runs metric-level TP scoring.
func scoreMetricTP(outputPath, scenariosDir string) (*observerimpl.MetricScoreResult, error) {
	data, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("reading output file: %w", err)
	}

	var output observerimpl.ObserverOutput
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, fmt.Errorf("parsing output JSON: %w", err)
	}

	scenarioName := output.Metadata.Scenario
	if scenarioName == "" {
		return nil, errors.New("output file missing metadata.scenario")
	}

	gt, err := observerimpl.LoadMetricGroundTruth(scenariosDir, scenarioName)
	if err != nil {
		return nil, fmt.Errorf("loading metric ground truth: %w", err)
	}

	if len(gt.TruePositives) == 0 {
		return nil, fmt.Errorf("no true_positives in metadata for scenario %q", scenarioName)
	}

	disruptionStart := observerimpl.LoadDisruptionStartUnix(scenariosDir, scenarioName)

	return observerimpl.ScoreMetrics(&output, gt, disruptionStart), nil
}
