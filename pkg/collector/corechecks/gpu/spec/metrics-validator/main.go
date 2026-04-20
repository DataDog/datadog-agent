// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main validates emitted GPU metrics against the shared spec.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	var site string
	var lookbackSeconds int64
	var outputFile string

	exitCode := 0

	flag.StringVar(&site, "site", "", "Datadog site")
	flag.Int64Var(&lookbackSeconds, "lookback-seconds", 3600, "Metrics lookback window in seconds")
	flag.StringVar(&outputFile, "output-file", "", "Write JSON results to the given file instead of stdout")
	flag.Parse()

	if err := validateFlags(site, lookbackSeconds, outputFile); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gpu metrics validation failed: %v\n", err)
		os.Exit(1)
	}

	apiKey, appKey, err := authKeysFromEnv()
	if err != nil {
		log.Fatalf("gpu metrics validation failed: %v", err)
	}

	results, err := computeValidation(apiKey, appKey, site, lookbackSeconds)
	if err != nil {
		log.Printf("gpu metrics validation failed: %v", err)
		exitCode = 1
	}

	if err := writeResults(results, outputFile); err != nil {
		log.Printf("gpu metrics validation failed: %v", err)
		exitCode = 1
	}

	os.Exit(exitCode)
}

func writeResults(results orgValidationResults, outputFile string) error {
	outputPath := strings.TrimSpace(outputFile)
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output file %q: %w", outputPath, err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(results); err != nil {
		return fmt.Errorf("encode validation results: %w", err)
	}
	return nil
}

func validateFlags(site string, lookbackSeconds int64, outputFile string) error {
	if strings.TrimSpace(site) == "" {
		return errors.New("--site is required")
	}
	if lookbackSeconds <= 0 {
		return errors.New("--lookback-seconds must be greater than 0")
	}
	if strings.TrimSpace(outputFile) == "" {
		return errors.New("--output-file is required")
	}
	return nil
}

func authKeysFromEnv() (string, string, error) {
	apiKey := strings.TrimSpace(os.Getenv("DD_API_KEY"))
	if apiKey == "" {
		return "", "", errors.New("DD_API_KEY environment variable is required")
	}

	appKey := strings.TrimSpace(os.Getenv("DD_APP_KEY"))
	if appKey == "" {
		return "", "", errors.New("DD_APP_KEY environment variable is required")
	}

	return apiKey, appKey, nil
}
