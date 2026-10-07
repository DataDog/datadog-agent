// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package loadflare

import (
	"fmt"
	"math"
	"strings"

	"github.com/DataDog/datadog-agent/comp/logs-library/characterization"
)

func inferLading(snapshot characterization.Snapshot) ([]byte, map[string]any) {
	warnings := []string{}
	limitations := []string{}
	var fileEvents, fileBytes, distinctFileSources uint64
	var otherEvents uint64
	for _, group := range snapshot.Groups {
		if group.SourceType == "file" {
			fileEvents += group.Aggregate.Events
			fileBytes += group.Aggregate.ContentBytes
			distinctFileSources += group.Aggregate.SourceCount
		} else {
			otherEvents += group.Aggregate.Events
		}
	}
	status := "ready"
	if fileEvents == 0 {
		status = "unsupported"
		warnings = append(warnings, "No file-source ingress was observed; this command currently emits Lading candidates only for file workloads.")
	}
	if otherEvents > 0 && fileEvents > 0 {
		status = "partial"
		warnings = append(warnings, "Non-file ingress was observed and is not represented by the emitted file generator.")
	}
	fileSources := distinctFileSources
	if snapshot.Lifecycle != nil && snapshot.Lifecycle.Rotations > 0 && fileSources > snapshot.Lifecycle.Rotations {
		fileSources -= snapshot.Lifecycle.Rotations
		limitations = append(limitations, "Concurrent file sources are inferred as distinct bounded-window identities minus observed rotations.")
	}
	if fileSources == 0 {
		fileSources = 1
		if fileEvents > 0 {
			status = "partial"
			warnings = append(warnings, "File source identity cardinality was unavailable; one source is assumed.")
		}
	}
	if fileSources > 64 {
		fileSources = 64
		status = "partial"
		warnings = append(warnings, "Observed file source cardinality exceeded Lading's bounded candidate limit and was capped at 64.")
	}

	duration := snapshot.EndedAt.Sub(snapshot.StartedAt).Seconds()
	if duration <= 0 {
		duration = snapshot.RequestedDurationSeconds
	}
	aggregateRate := uint64(0)
	if duration > 0 {
		aggregateRate = uint64(math.Round(float64(fileBytes) / duration))
	}
	if fileEvents > 0 && aggregateRate < 1024 {
		aggregateRate = 1024
		limitations = append(limitations, "Lading's candidate rate was floored at 1024 bytes per second.")
	}
	perSourceRate := aggregateRate / fileSources
	if perSourceRate < 1 && fileEvents > 0 {
		perSourceRate = 1
	}

	variant, payloadStatus, payloadWarning := dominantVariant(snapshot.PayloadFamilies)
	if payloadStatus == "partial" && status == "ready" {
		status = "partial"
	}
	if payloadStatus == "unsupported" {
		status = "unsupported"
	}
	if payloadWarning != "" {
		warnings = append(warnings, payloadWarning)
	}
	if isBursty(snapshot.RateWindows) && status == "ready" {
		status = "partial"
		warnings = append(warnings, "Material rate-window variation was observed; the candidate preserves mean rate but uses a constant load profile.")
	}

	name := "inferred-" + snapshot.SessionID
	lifecycleReport := lifecycleRepresentation(snapshot.Lifecycle)
	if lifecycleReport["status"] == "partial" && status == "ready" {
		status = "partial"
	}
	report := map[string]any{
		"schema_version":                        1,
		"kind":                                  "logs-characterization-inference-report",
		"candidate":                             map[string]any{"name": name, "generator": "lading", "status": status},
		"source_observation":                    snapshot.SessionID,
		"representability":                      map[string]any{"status": status, "warnings": warnings},
		"lifecycle_representation":              lifecycleReport,
		"limitations":                           limitations,
		"predicted_aggregate_bytes_per_second":  aggregateRate,
		"predicted_per_source_bytes_per_second": perSourceRate,
		"inferred_source_count":                 fileSources,
		"observed_distinct_file_sources":        distinctFileSources,
		"inferred_payload_variant":              variant,
		"rate_inference":                        "bounded_raw_ingress_bytes",
	}
	if status == "unsupported" {
		return nil, report
	}

	seed := "[1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32]"
	var builder strings.Builder
	builder.WriteString("generator:\n  - file_gen:\n")
	if snapshot.Lifecycle != nil && snapshot.Lifecycle.Rotations > 0 && snapshot.Lifecycle.RotationIntervals.Count > 0 {
		interval := snapshot.Lifecycle.RotationIntervals.Sum / float64(snapshot.Lifecycle.RotationIntervals.Count)
		maximumBytes := max(uint64(1), uint64(math.Round(float64(perSourceRate)*interval)))
		rotations := min(uint64(16), max(uint64(1), snapshot.Lifecycle.Rotations))
		fmt.Fprintf(&builder, "      logrotate_fs:\n        seed: %s\n        concurrent_logs: %d\n        maximum_bytes_per_log: \"%dB\"\n        total_rotations: %d\n        max_depth: 0\n        variant: \"%s\"\n        load_profile:\n          constant: \"%dB\"\n        maximum_block_size: \"%s\"\n        maximum_prebuild_cache_size_bytes: \"32MiB\"\n        mount_point: \"./load-flare-source\"\n", seed, fileSources, maximumBytes, rotations, variant, perSourceRate, maximumBlockSize(snapshot))
	} else {
		fmt.Fprintf(&builder, "      traditional:\n        seed: %s\n        path_template: \"./load-flare-source/part.%%NNN%%.log\"\n        rotate: false\n        duplicates: %d\n        flush_every: \"1s\"\n        bytes_per_second: \"%dB\"\n        variant: \"%s\"\n        maximum_bytes_per_file: \"2GiB\"\n        maximum_block_size: \"%s\"\n        maximum_prebuild_cache_size_bytes: \"32MiB\"\n", seed, fileSources, perSourceRate, variant, maximumBlockSize(snapshot))
	}
	builder.WriteString("blackhole:\n  - tcp:\n      binding_addr: \"127.0.0.1:0\"\n")
	return []byte(builder.String()), report
}

func dominantVariant(families map[string]characterization.Aggregate) (string, string, string) {
	var dominant string
	var dominantEvents, total uint64
	for family, aggregate := range families {
		total += aggregate.Events
		if aggregate.Events > dominantEvents {
			dominant, dominantEvents = family, aggregate.Events
		}
	}
	if total == 0 {
		return "", "unsupported", "No payload-family events were observed."
	}
	status := "ready"
	warning := ""
	if float64(dominantEvents)/float64(total) < 0.95 {
		status = "partial"
		warning = "Multiple payload families were observed; the candidate represents only the dominant family."
	}
	switch dominant {
	case "apache_common", "syslog5424", "json":
		return dominant, status, warning
	case "plain":
		if warning == "" {
			warning = "Arbitrary plain-text semantics cannot be recovered; the candidate uses Lading's ASCII generator."
		}
		return "ascii", "partial", warning
	case "datadog_json":
		if warning == "" {
			warning = "Datadog JSON field shape cannot be recovered from aggregate telemetry; the candidate uses generic JSON."
		}
		return "json", "partial", warning
	default:
		return "", "unsupported", "The dominant payload family has no safe Lading generator mapping."
	}
}

func lifecycleRepresentation(lifecycle *characterization.Lifecycle) map[string]any {
	if lifecycle == nil || lifecycle.Rotations == 0 {
		return map[string]any{"status": "partial", "warnings": []string{"No rotation occurred inside the bounded window; a longer lifecycle cadence may be present."}}
	}
	return map[string]any{"status": "ready", "warnings": []string{}}
}

func isBursty(windows []characterization.RateWindow) bool {
	if len(windows) < 2 {
		return false
	}
	var sum float64
	for _, window := range windows {
		sum += float64(window.ContentBytes)
	}
	mean := sum / float64(len(windows))
	if mean == 0 {
		return false
	}
	var squares float64
	for _, window := range windows {
		delta := float64(window.ContentBytes) - mean
		squares += delta * delta
	}
	return math.Sqrt(squares/float64(len(windows)))/mean > 0.25
}

func maximumBlockSize(snapshot characterization.Snapshot) string {
	if snapshot.Totals.MessageSizes.Max > 4096 {
		return "32KiB"
	}
	return "4KiB"
}
