// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main updates the billing allowlist with GPU metrics from the shared spec.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
)

type allowlistEntry struct {
	Prefix      string       `json:"prefix"`
	Origins     []originSpec `json:"origins,omitempty"`
	Description string       `json:"description,omitempty"`
}

type originSpec struct {
	Product       string `json:"product"`
	Subproduct    string `json:"subproduct"`
	ProductDetail string `json:"product_detail"`
}

var gpuMetricOrigins = []originSpec{
	{
		Product:       "gpu_monitoring",
		Subproduct:    "gpu",
		ProductDetail: "agent_gpu_telemetry",
	},
}

func main() {
	var allowlistPath string

	flag.StringVar(&allowlistPath, "allowlist-path", "", "Path to standard_metric_allowlist.json")
	flag.Parse()

	if err := validateFlags(allowlistPath); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gpu metrics allowlist update failed: %v\n", err)
		os.Exit(1)
	}

	if err := updateAllowlistFile(allowlistPath); err != nil {
		log.Fatalf("gpu metrics allowlist update failed: %v", err)
	}
}

func validateFlags(allowlistPath string) error {
	if strings.TrimSpace(allowlistPath) == "" {
		return errors.New("--allowlist-path is required")
	}

	return nil
}

func updateAllowlistFile(allowlistPath string) error {
	entries, err := loadAllowlistEntries(allowlistPath)
	if err != nil {
		return fmt.Errorf("load allowlist entries: %w", err)
	}

	updatedEntries, err := updateAllowlistEntries(entries)
	if err != nil {
		return fmt.Errorf("update allowlist entries: %w", err)
	}
	if err := writeAllowlistEntries(allowlistPath, updatedEntries); err != nil {
		return fmt.Errorf("write allowlist entries: %w", err)
	}

	return nil
}

func updateAllowlistEntries(entries []allowlistEntry) ([]allowlistEntry, error) {
	specs, err := loadSpecs()
	if err != nil {
		return nil, fmt.Errorf("load metrics spec: %w", err)
	}
	metricsSpec := specs.Metrics

	updatedEntries := make([]allowlistEntry, 0, len(entries)+len(metricsSpec.Metrics))
	existingPrefixes := make(map[string]struct{}, len(entries))
	allMetricNames := make([]string, 0, len(entries)+len(metricsSpec.Metrics))
	for _, entry := range entries {
		updatedEntries = append(updatedEntries, entry)
		existingPrefixes[entry.Prefix] = struct{}{}
		allMetricNames = append(allMetricNames, entry.Prefix)
	}

	missingPrefixes := make([]string, 0, len(metricsSpec.Metrics))
	for metricName := range metricsSpec.Metrics {
		prefix := gpuspec.PrefixedMetricName(specs, metricName)
		if _, found := existingPrefixes[prefix]; found {
			continue
		}
		missingPrefixes = append(missingPrefixes, prefix)
		allMetricNames = append(allMetricNames, prefix)
	}

	sort.Strings(missingPrefixes)
	for _, prefix := range missingPrefixes {
		// This is a manual maintenance flow with a small number of GPU metrics.
		// Keep the prefix dedupe logic simple with an O(n^2) scan; performance is not a concern here.
		if hasAnotherMetricAsPrefix(allMetricNames, prefix) {
			continue
		}

		updatedEntries = append(updatedEntries, allowlistEntry{
			Prefix:  prefix,
			Origins: cloneOrigins(gpuMetricOrigins),
		})

		existingPrefixes[prefix] = struct{}{}
	}

	sort.Slice(updatedEntries, func(i, j int) bool {
		return updatedEntries[i].Prefix < updatedEntries[j].Prefix
	})

	return updatedEntries, nil
}

func loadAllowlistEntries(allowlistPath string) ([]allowlistEntry, error) {
	data, err := os.ReadFile(allowlistPath)
	if err != nil {
		return nil, fmt.Errorf("read allowlist file %q: %w", allowlistPath, err)
	}

	var entries []allowlistEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("unmarshal allowlist file %q: %w", allowlistPath, err)
	}

	return entries, nil
}

func writeAllowlistEntries(allowlistPath string, entries []allowlistEntry) error {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("", "    ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(entries); err != nil {
		return fmt.Errorf("encode allowlist JSON: %w", err)
	}

	if err := os.WriteFile(allowlistPath, buffer.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write allowlist file %q: %w", allowlistPath, err)
	}

	return nil
}

func cloneOrigins(origins []originSpec) []originSpec {
	cloned := make([]originSpec, len(origins))
	copy(cloned, origins)
	return cloned
}

func hasAnotherMetricAsPrefix(metricNames []string, candidate string) bool {
	for _, metricName := range metricNames {
		if strings.HasPrefix(candidate, metricName) && metricName != candidate {
			return true
		}
	}
	return false
}
