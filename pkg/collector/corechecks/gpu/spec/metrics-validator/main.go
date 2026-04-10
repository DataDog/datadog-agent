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
		log.Fatalf("gpu metrics validation failed: %v", err)
	}

	if err := writeResults(results, outputFile); err != nil {
		log.Fatalf("gpu metrics validation failed: %v", err)
	}
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
		return fmt.Errorf("--site is required")
	}
	if lookbackSeconds <= 0 {
		return fmt.Errorf("--lookback-seconds must be greater than 0")
	}
	if strings.TrimSpace(outputFile) == "" {
		return fmt.Errorf("--output-file is required")
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
