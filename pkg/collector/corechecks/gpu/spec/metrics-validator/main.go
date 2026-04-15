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
	"os"
	"strings"
)

func main() {
	var site string
	var lookbackSeconds int64

	flag.StringVar(&site, "site", "", "Datadog site")
	flag.Int64Var(&lookbackSeconds, "lookback-seconds", 3600, "Metrics lookback window in seconds")
	flag.Parse()

	if err := validateFlags(site, lookbackSeconds); err != nil {
		exitValidationError(err)
	}

	apiKey, appKey, err := authKeysFromEnv()
	if err != nil {
		exitValidationError(err)
	}

	metricsResults, err := computeValidation(apiKey, appKey, site, lookbackSeconds)
	if err != nil {
		exitValidationError(err)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(metricsResults); err != nil {
		exitValidationError(err)
	}
}

func validateFlags(site string, lookbackSeconds int64) error {
	if strings.TrimSpace(site) == "" {
		return errors.New("--site is required")
	}
	if lookbackSeconds <= 0 {
		return errors.New("--lookback-seconds must be greater than 0")
	}
	return nil
}

func exitValidationError(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "gpu metrics validation failed: %v\n", err)
	os.Exit(1)
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
