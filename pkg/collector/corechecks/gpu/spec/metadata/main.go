// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main updates integrations-core GPU metadata from the shared spec.
package main

import (
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"log"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
)

const (
	defaultOrientation = "0"
)

var metadataHeaders = []string{
	"metric_name",
	"metric_type",
	"interval",
	"unit_name",
	"per_unit_name",
	"description",
	"orientation",
	"integration",
	"short_name",
	"curated_metric",
	"sample_tags",
}

type metadataEntry struct {
	MetricName    string
	MetricType    string
	Interval      string
	UnitName      string
	PerUnitName   string
	Description   string
	Orientation   string
	Integration   string
	ShortName     string
	CuratedMetric string
	SampleTags    string
}

type config struct {
	metadataPath    string
	defaultInterval int
}

func main() {
	cfg := parseFlags()
	if err := validateFlags(cfg); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gpu metadata update failed: %v\n", err)
		os.Exit(1)
	}

	if err := updateMetadataFile(cfg); err != nil {
		log.Fatalf("gpu metadata update failed: %v", err)
	}
}

func parseFlags() config {
	cfg := config{}
	flag.StringVar(&cfg.metadataPath, "metadata-path", "", "Path to integrations-core gpu/metadata.csv")
	flag.IntVar(&cfg.defaultInterval, "default-interval", 16, "Interval value to write to metadata.csv for all GPU metrics")
	flag.Parse()
	return cfg
}

func validateFlags(cfg config) error {
	if strings.TrimSpace(cfg.metadataPath) == "" {
		return errors.New("--metadata-path is required")
	}
	if cfg.defaultInterval <= 0 {
		return errors.New("--default-interval must be positive")
	}
	return nil
}

func updateMetadataFile(cfg config) error {
	entries, err := loadMetadataEntries(cfg.metadataPath)
	if err != nil {
		return fmt.Errorf("load metadata entries: %w", err)
	}

	updatedEntries, err := updateMetadataEntries(entries, cfg)
	if err != nil {
		return fmt.Errorf("update metadata entries: %w", err)
	}

	if err := writeMetadataEntries(cfg.metadataPath, updatedEntries); err != nil {
		return fmt.Errorf("write metadata entries: %w", err)
	}

	return nil
}

func loadMetadataEntries(metadataPath string) (map[string]metadataEntry, error) {
	file, err := os.Open(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("open metadata file %q: %w", metadataPath, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	// metadata.csv rows can omit trailing empty columns; allow variable-width records
	// and treat missing trailing fields as empty strings in csvValue below.
	reader.FieldsPerRecord = -1

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read metadata CSV %q: %w", metadataPath, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("metadata CSV %q is empty", metadataPath)
	}

	headerIndex := make(map[string]int, len(rows[0]))
	for i, column := range rows[0] {
		headerIndex[column] = i
	}
	for _, requiredHeader := range metadataHeaders {
		if _, found := headerIndex[requiredHeader]; !found {
			return nil, fmt.Errorf("metadata CSV %q missing required column %q", metadataPath, requiredHeader)
		}
	}

	entries := make(map[string]metadataEntry, len(rows)-1)
	for rowIdx, row := range rows[1:] {
		entry := metadataEntry{
			MetricName:    csvValue(row, headerIndex, "metric_name"),
			MetricType:    csvValue(row, headerIndex, "metric_type"),
			Interval:      csvValue(row, headerIndex, "interval"),
			UnitName:      csvValue(row, headerIndex, "unit_name"),
			PerUnitName:   csvValue(row, headerIndex, "per_unit_name"),
			Description:   csvValue(row, headerIndex, "description"),
			Orientation:   csvValue(row, headerIndex, "orientation"),
			Integration:   csvValue(row, headerIndex, "integration"),
			ShortName:     csvValue(row, headerIndex, "short_name"),
			CuratedMetric: csvValue(row, headerIndex, "curated_metric"),
			SampleTags:    csvValue(row, headerIndex, "sample_tags"),
		}
		if strings.TrimSpace(entry.MetricName) == "" {
			return nil, fmt.Errorf("metadata CSV %q row %d has empty metric_name", metadataPath, rowIdx+2)
		}
		entries[entry.MetricName] = entry
	}

	return entries, nil
}

func updateMetadataEntries(entries map[string]metadataEntry, cfg config) (map[string]metadataEntry, error) {
	specs, err := gpuspec.LoadSpecs()
	if err != nil {
		return nil, fmt.Errorf("load metrics spec: %w", err)
	}

	for metricName, metricSpec := range specs.Metrics.Metrics {
		if metricSpec.Metadata == nil {
			return nil, fmt.Errorf("metric %q missing metadata", metricName)
		}

		prefixedMetricName := gpuspec.PrefixedMetricName(specs, metricName)
		entry, found := entries[prefixedMetricName]
		if !found {
			entry = metadataEntry{
				MetricName:    prefixedMetricName,
				Orientation:   defaultOrientation,
				Integration:   "gpu",
				ShortName:     metricName,
				CuratedMetric: "",
				SampleTags:    "",
			}
		}

		var err error
		entry.UnitName, entry.PerUnitName, err = splitUnit(metricSpec.Metadata.Unit)
		if err != nil {
			return nil, fmt.Errorf("split unit: %w", err)
		}

		entry.MetricType = metricSpec.Metadata.MetricType
		entry.Interval = strconv.Itoa(cfg.defaultInterval)
		entry.Description = metricSpec.Metadata.Description

		entries[prefixedMetricName] = entry
	}

	return entries, nil
}

func splitUnit(unit string) (string, string, error) {
	trimmed := strings.TrimSpace(unit)
	if trimmed == "" {
		return "", "", nil
	}

	parts := strings.Split(trimmed, "/")
	if len(parts) > 2 {
		return "", "", fmt.Errorf("unit %q must contain at most one '/'", unit)
	}
	if len(parts) == 1 {
		return strings.TrimSpace(parts[0]), "", nil
	}

	unitName := strings.TrimSpace(parts[0])
	perUnitName := strings.TrimSpace(parts[1])
	if unitName == "" || perUnitName == "" {
		return "", "", fmt.Errorf("unit %q must not contain empty segments", unit)
	}

	return unitName, perUnitName, nil
}

func writeMetadataEntries(metadataPath string, entries map[string]metadataEntry) error {
	file, err := os.Create(metadataPath)
	if err != nil {
		return fmt.Errorf("create metadata file %q: %w", metadataPath, err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	if err := writer.Write(metadataHeaders); err != nil {
		return fmt.Errorf("write metadata header: %w", err)
	}

	for _, metricName := range slices.Sorted(maps.Keys(entries)) {
		entry := entries[metricName]
		record := []string{
			entry.MetricName,
			entry.MetricType,
			entry.Interval,
			entry.UnitName,
			entry.PerUnitName,
			entry.Description,
			entry.Orientation,
			entry.Integration,
			entry.ShortName,
			entry.CuratedMetric,
			entry.SampleTags,
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("write metadata row for %q: %w", entry.MetricName, err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("flush metadata CSV %q: %w", metadataPath, err)
	}

	return nil
}

func csvValue(row []string, headerIndex map[string]int, header string) string {
	idx := headerIndex[header]
	if idx >= len(row) {
		return ""
	}
	return row[idx]
}
