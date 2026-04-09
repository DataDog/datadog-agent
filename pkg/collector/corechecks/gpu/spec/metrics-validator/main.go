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

	apiKey, appKey, err := authKeysFromEnv()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gpu metrics validation failed: %v\n", err)
		os.Exit(1)
	}

	if err := validateFlags(site, lookbackSeconds); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gpu metrics validation failed: %v\n", err)
		os.Exit(1)
	}

	results, err := computeValidation(apiKey, appKey, site, lookbackSeconds)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gpu metrics validation failed: %v\n", err)
		os.Exit(1)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(results); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gpu metrics validation failed: %v\n", err)
		os.Exit(1)
	}
}

func validateFlags(site string, lookbackSeconds int64) error {
	if strings.TrimSpace(site) == "" {
		return fmt.Errorf("--site is required")
	}
	if lookbackSeconds <= 0 {
		return fmt.Errorf("--lookback-seconds must be greater than 0")
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
