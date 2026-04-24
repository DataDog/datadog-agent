// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main generates a GPU metric list CSV from the shared spec.
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
	"strings"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
)

var outputHeaders = []string{
	"Metric name",
	"Min supported architecture",
	"Tagset",
	"Extra tags",
	"MIG support",
	"vGPU support",
	"Aggregation type",
	"Time aggregation",
	"Group aggregation",
	"Granularity aggregation",
	"Granularity tags",
	"Used in GPU Mon",
}

type config struct {
	outputPath string
}

type metricRow struct {
	metricName                   string
	minSupportedArchitecture     string
	tagset                       string
	extraTags                    string
	migSupport                   string
	vgpuSupport                  string
	aggregationType              string
	timeAggregation              string
	groupAggregation             string
	granularityAggregation       string
	granularityTags              string
	usedInGPUMon                 string
}

func main() {
	cfg := parseFlags()
	if err := validateFlags(cfg); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gpu metric list generation failed: %v\n", err)
		os.Exit(1)
	}

	if err := writeMetricList(cfg); err != nil {
		log.Fatalf("gpu metric list generation failed: %v", err)
	}
}

func parseFlags() config {
	cfg := config{}
	flag.StringVar(&cfg.outputPath, "output-path", "", "Path to write generated metric list CSV")
	flag.Parse()
	return cfg
}

func validateFlags(cfg config) error {
	if strings.TrimSpace(cfg.outputPath) == "" {
		return errors.New("--output-path is required")
	}
	return nil
}

func writeMetricList(cfg config) error {
	specs, err := gpuspec.LoadSpecs()
	if err != nil {
		return fmt.Errorf("load specs: %w", err)
	}

	rows, err := buildMetricRows(specs)
	if err != nil {
		return fmt.Errorf("build metric rows: %w", err)
	}

	file, err := os.Create(cfg.outputPath)
	if err != nil {
		return fmt.Errorf("create output file %q: %w", cfg.outputPath, err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	writer.Comma = '\t'
	if err := writer.Write(outputHeaders); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	for _, row := range rows {
		record := []string{
			row.metricName,
			row.minSupportedArchitecture,
			row.tagset,
			row.extraTags,
			row.migSupport,
			row.vgpuSupport,
			row.aggregationType,
			row.timeAggregation,
			row.groupAggregation,
			row.granularityAggregation,
			row.granularityTags,
			row.usedInGPUMon,
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("write row for %q: %w", row.metricName, err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("flush CSV %q: %w", cfg.outputPath, err)
	}

	return nil
}

func buildMetricRows(specs *gpuspec.Specs) ([]metricRow, error) {
	rows := make([]metricRow, 0, len(specs.Metrics.Metrics))
	for metricName, metricSpec := range specs.Metrics.Metrics {
		if metricSpec.Metadata == nil {
			return nil, fmt.Errorf("metric %q missing metadata", metricName)
		}

		aggregationType := metricSpec.Metadata.Aggregation
		if aggregationType == "" {
			// This report is aggregation-centric; metrics without aggregation metadata are skipped.
			continue
		}

		aggregationSpec, found := specs.Aggregations.Aggregations[aggregationType]
		if !found {
			return nil, fmt.Errorf("metric %q references unknown aggregation %q", metricName, aggregationType)
		}

		minSupportedArchitecture := earliestSupportedArchitecture(specs, metricSpec)
		if minSupportedArchitecture == "" {
			return nil, fmt.Errorf("metric %q is unsupported on all known architectures", metricName)
		}

		row := metricRow{
			metricName:               gpuspec.PrefixedMetricName(specs, metricName),
			minSupportedArchitecture: minSupportedArchitecture,
			tagset:                   strings.Join(metricSpec.Tagsets, "|"),
			extraTags:                strings.Join(metricSpec.CustomTags, "|"),
			migSupport:               boolString(metricSpec.SupportsDeviceMode(gpuspec.DeviceModeMIG)),
			vgpuSupport:              boolString(metricSpec.SupportsDeviceMode(gpuspec.DeviceModeVGPU)),
			aggregationType:          aggregationType,
			timeAggregation:          aggregationSpec.TimeAggregator,
			groupAggregation:         aggregationSpec.GroupAggregator,
			granularityAggregation:   aggregationSpec.GranularityAggregator,
			granularityTags:          granularityTags(metricSpec),
			usedInGPUMon:             boolString(metricSpec.Metadata.UsedInDDUI),
		}
		rows = append(rows, row)
	}

	slices.SortFunc(rows, func(a, b metricRow) int {
		return strings.Compare(a.metricName, b.metricName)
	})

	return rows, nil
}

func granularityTags(metricSpec gpuspec.MetricSpec) string {
	tags := []string{"gpu_uuid", "host"}
	if slices.Contains(metricSpec.Tagsets, "process") {
		tags = append(tags, "pid")
	}
	tags = append(tags, metricSpec.CustomTags...)
	return "{" + strings.Join(tags, ",") + "}"
}

func earliestSupportedArchitecture(specs *gpuspec.Specs, metricSpec gpuspec.MetricSpec) string {
	architectures := slices.Sorted(maps.Keys(specs.Architectures.Architectures))
	slices.SortFunc(architectures, compareArchitectureName)

	for _, arch := range architectures {
		if metricSpec.SupportsArchitecture(arch) {
			return arch
		}
	}
	return ""
}

func compareArchitectureName(a, b string) int {
	// Keep this mapping local so this generator remains buildable on non-Linux hosts
	// and without NVML/test-only imports.
	knownOrder := map[string]int{
		// Not currently in spec/architectures.yaml, but keep full historical order
		// to avoid accidental mis-sorting if older architectures are added later.
		"fermi":     0,
		"kepler":    1,
		"maxwell":   2,
		"pascal":    3,
		"volta":     4,
		"turing":    5,
		"ampere":    6,
		"hopper":    7,
		"ada":       8,
		"blackwell": 9,
	}

	aRank, aKnown := knownOrder[a]
	bRank, bKnown := knownOrder[b]

	switch {
	case aKnown && bKnown:
		return aRank - bRank
	case aKnown:
		return -1
	case bKnown:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

func boolString(v bool) string {
	if v {
		return "TRUE"
	}
	return "FALSE"
}

