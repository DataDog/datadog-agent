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
	var mode string
	var windowSeconds int64
	var metricNameFilter string
	var tagNameFilter string
	var filterTags string

	flag.StringVar(&mode, "mode", "metrics", "Validation mode: metrics or tags")
	flag.StringVar(&site, "site", "", "Datadog site")
	flag.Int64Var(&lookbackSeconds, "lookback-seconds", 3600, "Metrics lookback window in seconds")
	flag.Int64Var(&windowSeconds, "window-seconds", 14400, "Tag lookup window in seconds")
	flag.StringVar(&metricNameFilter, "metric-name-filter", "", "Only validate metrics whose full name contains this substring")
	flag.StringVar(&tagNameFilter, "tag-name-filter", "", "Only validate tag names containing this substring")
	flag.StringVar(&filterTags, "filter-tags", "", "Optional filter[tags] expression for tag lookups")
	flag.Parse()

	if err := validateFlags(mode, site, lookbackSeconds, windowSeconds); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gpu metrics validation failed: %v\n", err)
		os.Exit(1)
	}

	apiKey, appKey, err := authKeysFromEnv()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gpu metrics validation failed: %v\n", err)
		os.Exit(1)
	}

	var result any
	switch mode {
	case "metrics":
		results, err := computeValidation(apiKey, appKey, site, lookbackSeconds)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "gpu metrics validation failed: %v\n", err)
			os.Exit(1)
		}
		result = results
	case "tags":
		results, err := computeTagValidation(apiKey, appKey, site, metricNameFilter, tagNameFilter, windowSeconds, filterTags)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "gpu metrics validation failed: %v\n", err)
			os.Exit(1)
		}
		result = results
	default:
		_, _ = fmt.Fprintf(os.Stderr, "gpu metrics validation failed: --mode must be one of metrics or tags\n")
		os.Exit(1)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gpu metrics validation failed: %v\n", err)
		os.Exit(1)
	}
}

func validateFlags(mode, site string, lookbackSeconds, windowSeconds int64) error {
	if strings.TrimSpace(mode) == "" {
		return fmt.Errorf("--mode is required")
	}
	if strings.TrimSpace(site) == "" {
		return errors.New("--site is required")
	}
	if mode == "metrics" && lookbackSeconds <= 0 {
		return errors.New("--lookback-seconds must be greater than 0")
	}
	if mode == "tags" && windowSeconds <= 0 {
		return errors.New("--window-seconds must be greater than 0")
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
