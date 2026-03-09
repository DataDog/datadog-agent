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

func main() {
	outputPath := flag.String("input", "", "Path to headless output JSON to score (required)")
	scenariosDir := flag.String("scenarios-dir", "./comp/observer/scenarios", "Directory containing scenario subdirectories (for metadata.json lookup)")
	groundTruthTS := flag.Int64("ground-truth-ts", 0, "Ground truth disruption onset timestamp in unix seconds (overrides metadata.json)")
	sigma := flag.Float64("sigma", 30.0, "Gaussian width in seconds")
	jsonOutput := flag.Bool("json", false, "Output result as JSON")
	flag.Parse()

	if *outputPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: observer-scorer --input <path> [--scenarios-dir <dir>] [--ground-truth-ts <unix>] [--sigma <seconds>] [--json]\n")
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

	if *jsonOutput {
		data, err := json.Marshal(result)
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
}
