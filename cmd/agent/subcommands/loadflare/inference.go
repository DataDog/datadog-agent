// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package loadflare

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/comp/logs-library/characterization"
)

const datadogJSONTemplatePath = "./load-flare-assets/datadog-json-template.yaml"

const datadogJSONTemplate = `definitions:
  status:
    !choose ["debug", "info", "notice", "warn", "error", "critical"]
  service:
    !choose ["api", "worker", "scheduler", "web"]
  source:
    !choose ["python", "go", "java", "nginx"]
generator:
  !object
    message:
      !format
        template: "{} request completed"
        args:
          - !reference service
    status: !reference status
    timestamp: !timestamp
    hostname: !choose ["host-a", "host-b", "host-c"]
    service: !reference service
    ddsource: !reference source
    ddtags: !choose ["env:test,team:logs", "env:test,region:us1", "env:test"]
`

func inferLading(snapshot characterization.Snapshot) ([]byte, map[string]any) {
	warnings := []string{}
	limitations := []string{}
	var fileEvents, fileContentBytes, fileRawBytes, distinctFileSources uint64
	var otherEvents uint64
	for _, group := range snapshot.Groups {
		if group.SourceType == "file" {
			fileEvents += group.Aggregate.Events
			fileContentBytes += group.Aggregate.ContentBytes
			fileRawBytes += group.Aggregate.RawBytes
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
	fileSources := uint64(0)
	for _, window := range snapshot.RateWindows {
		if window.FileSourceCount > 0 && (fileSources == 0 || window.FileSourceCount < fileSources) {
			fileSources = window.FileSourceCount
		}
	}
	if fileSources > 0 {
		limitations = append(limitations, "Concurrent file sources are inferred from the minimum non-empty ten-second window cardinality.")
	} else {
		fileSources = distinctFileSources
	}
	if fileSources == distinctFileSources && snapshot.Lifecycle != nil && snapshot.Lifecycle.Rotations > 0 && fileSources > snapshot.Lifecycle.Rotations {
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
	fileBytes := fileRawBytes
	rateInference := "bounded_raw_ingress_bytes"
	if fileBytes == 0 && fileContentBytes > 0 {
		fileBytes = fileContentBytes
		rateInference = "bounded_content_bytes_fallback"
		limitations = append(limitations, "Raw file byte observations were unavailable; content bytes were used as a rate fallback.")
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

	families := filePayloadFamilies(snapshot)
	streams, streamWarnings := inferPayloadStreams(families, aggregateRate, fileSources)
	variant := "mixed"
	if len(streams) > 1 {
		if len(streamWarnings) > 0 && status == "ready" {
			status = "partial"
		}
		warnings = append(warnings, streamWarnings...)
	} else {
		streams = nil
		var payloadStatus, payloadWarning string
		variant, payloadStatus, payloadWarning = dominantVariant(families)
		if payloadStatus == "partial" && status == "ready" {
			status = "partial"
		}
		if payloadStatus == "unsupported" {
			status = "unsupported"
		}
		if payloadWarning != "" {
			warnings = append(warnings, payloadWarning)
		}
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
	streamReport := make([]map[string]any, 0, len(streams))
	for _, stream := range streams {
		streamReport = append(streamReport, map[string]any{
			"family":       stream.Family,
			"source_count": stream.Sources, "bytes_per_second_per_source": stream.Rate,
			"byte_fraction": stream.Fraction, "approximate": stream.Approximate,
			"lading_generator": ladingGeneratorReport(stream.Variant),
		})
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
		"inferred_payload_streams":              streamReport,
		"rate_inference":                        rateInference,
	}
	if status == "unsupported" {
		return nil, report
	}
	flushEvery := inferFlushEvery(snapshot, fileSources, duration)

	seed := "[1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32]"
	var builder strings.Builder
	builder.WriteString("generator:\n")
	if len(streams) == 0 {
		streams = []inferredPayloadStream{{Variant: variant, Sources: fileSources, Rate: perSourceRate, Fraction: 1}}
	}
	hasLifecycle := snapshot.Lifecycle != nil && snapshot.Lifecycle.Rotations > 0 && snapshot.Lifecycle.RotationIntervals.Count > 0
	for index, stream := range streams {
		builder.WriteString("  - file_gen:\n")
		if hasLifecycle {
			interval := snapshot.Lifecycle.RotationIntervals.Sum / float64(snapshot.Lifecycle.RotationIntervals.Count)
			maximumBytes := max(uint64(1), uint64(math.Round(float64(stream.Rate)*interval)))
			streamRotations := uint64(math.Round(float64(snapshot.Lifecycle.Rotations) * float64(stream.Sources) / float64(fileSources)))
			rotations := min(uint64(16), max(uint64(1), streamRotations))
			mountPoint := "./load-flare-source"
			if len(streams) > 1 {
				mountPoint = fmt.Sprintf("./load-flare-source/rotation.%03d", index)
			}
			fmt.Fprintf(&builder, "      logrotate_fs:\n        seed: %s\n        concurrent_logs: %d\n        maximum_bytes_per_log: \"%dB\"\n        total_rotations: %d\n        max_depth: 0\n", seed, stream.Sources, maximumBytes, rotations)
			builder.WriteString(renderedVariant(stream.Variant, "        "))
			fmt.Fprintf(&builder, "        load_profile:\n          constant: \"%dB\"\n        maximum_block_size: \"%s\"\n        maximum_prebuild_cache_size_bytes: \"32MiB\"\n        mount_point: \"%s\"\n", stream.Rate, maximumBlockSize(snapshot), mountPoint)
		} else {
			pathTemplate := "./load-flare-source/part.%NNN%.log"
			if len(streams) > 1 {
				pathTemplate = fmt.Sprintf("./load-flare-source/stream.%03d.part.%%NNN%%.log", index)
			}
			fmt.Fprintf(&builder, "      traditional:\n        seed: %s\n        path_template: \"%s\"\n        rotate: false\n        duplicates: %d\n        flush_every: \"%s\"\n        bytes_per_second: \"%dB\"\n", seed, pathTemplate, stream.Sources, flushEvery, stream.Rate)
			builder.WriteString(renderedVariant(stream.Variant, "        "))
			fmt.Fprintf(&builder, "        maximum_bytes_per_file: \"2GiB\"\n        maximum_block_size: \"%s\"\n        maximum_prebuild_cache_size_bytes: \"32MiB\"\n", maximumBlockSize(snapshot))
		}
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
			warning = "Datadog JSON field shape cannot be recovered from aggregate telemetry; the candidate uses an allowlisted representative template."
		}
		return "templated_json", "partial", warning
	default:
		return "", "unsupported", "The dominant payload family has no safe Lading generator mapping."
	}
}

func lifecycleRepresentation(lifecycle *characterization.Lifecycle) map[string]any {
	if lifecycle == nil || lifecycle.Rotations == 0 {
		return map[string]any{"status": "partial", "warnings": []string{"No rotation occurred inside the bounded window; a longer lifecycle cadence may be present."}}
	}
	if lifecycleCadencesAreHeterogeneous(lifecycle.RotationIntervals) {
		return map[string]any{"status": "partial", "warnings": []string{"Multiple material rotation cadences were observed; the current candidate uses one effective cadence and does not reproduce the lifecycle mixture."}}
	}
	return map[string]any{"status": "ready", "warnings": []string{}}
}

func lifecycleCadencesAreHeterogeneous(histogram characterization.Histogram) bool {
	if histogram.Count == 0 || len(histogram.Bounds) == 0 {
		return false
	}
	type weightedBin struct {
		upperBound float64
		weight     float64
	}
	bins := make([]weightedBin, 0, len(histogram.Bounds))
	var totalWeight float64
	for index, upperBound := range histogram.Bounds {
		if index >= len(histogram.Counts) || histogram.Counts[index] == 0 {
			continue
		}
		weight := upperBound * float64(histogram.Counts[index])
		bins = append(bins, weightedBin{upperBound: upperBound, weight: weight})
		totalWeight += weight
	}
	material := make([]weightedBin, 0, len(bins))
	for _, bin := range bins {
		if totalWeight > 0 && bin.weight/totalWeight >= 0.1 {
			material = append(material, bin)
		}
	}
	return len(material) >= 2 && material[len(material)-1].upperBound/material[0].upperBound >= 1.5
}

func isBursty(windows []characterization.RateWindow) bool {
	if len(windows) < 2 {
		return false
	}
	var sum float64
	for _, window := range windows {
		bytes := window.RawBytes
		if bytes == 0 {
			bytes = window.ContentBytes
		}
		sum += float64(bytes)
	}
	mean := sum / float64(len(windows))
	if mean == 0 {
		return false
	}
	var squares float64
	for _, window := range windows {
		bytes := window.RawBytes
		if bytes == 0 {
			bytes = window.ContentBytes
		}
		delta := float64(bytes) - mean
		squares += delta * delta
	}
	return math.Sqrt(squares/float64(len(windows)))/mean > 0.25
}

type inferredPayloadStream struct {
	Family      string
	Variant     string
	Sources     uint64
	Rate        uint64
	Fraction    float64
	Approximate bool
}

func filePayloadFamilies(snapshot characterization.Snapshot) map[string]characterization.Aggregate {
	if len(snapshot.FilePayloadFamilies) > 0 {
		return snapshot.FilePayloadFamilies
	}
	return snapshot.PayloadFamilies
}

func payloadVariant(family string) (string, bool) {
	switch family {
	case "apache_common", "syslog5424", "json":
		return family, false
	case "datadog_json":
		return "templated_json", true
	case "plain", "empty":
		return "ascii", true
	default:
		return "ascii", true
	}
}

func renderedVariant(variant, indent string) string {
	if variant == "templated_json" {
		return fmt.Sprintf("%svariant:\n%s  templated_json:\n%s    template_path: \"%s\"\n", indent, indent, indent, datadogJSONTemplatePath)
	}
	return fmt.Sprintf("%svariant: \"%s\"\n", indent, variant)
}

func ladingGeneratorReport(variant string) map[string]any {
	if variant == "templated_json" {
		return map[string]any{
			"kind": "templated_json", "template_asset": strings.TrimPrefix(datadogJSONTemplatePath, "./"),
		}
	}
	return map[string]any{"kind": "builtin", "variant": variant}
}

func inferPayloadStreams(families map[string]characterization.Aggregate, aggregateRate, sourceCount uint64) ([]inferredPayloadStream, []string) {
	var total uint64
	for _, aggregate := range families {
		total += aggregate.RawBytes
	}
	useContent := total == 0
	if useContent {
		for _, aggregate := range families {
			total += aggregate.ContentBytes
		}
	}
	useEvents := total == 0
	if useEvents {
		for _, aggregate := range families {
			total += aggregate.Events
		}
	}
	if total == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(families))
	fractions := make(map[string]float64, len(families))
	for family, aggregate := range families {
		value := aggregate.RawBytes
		if useContent {
			value = aggregate.ContentBytes
		}
		if useEvents {
			value = aggregate.Events
		}
		fraction := float64(value) / float64(total)
		if fraction >= 0.01 {
			names = append(names, family)
			fractions[family] = fraction
		}
	}
	if len(names) <= 1 || uint64(len(names)) > sourceCount {
		return nil, nil
	}
	sort.Strings(names)
	allocations := make(map[string]uint64, len(names))
	for _, family := range names {
		allocations[family] = 1
	}
	for remaining := sourceCount - uint64(len(names)); remaining > 0; remaining-- {
		best := names[0]
		bestDeficit := fractions[best]*float64(sourceCount) - float64(allocations[best])
		for _, family := range names[1:] {
			deficit := fractions[family]*float64(sourceCount) - float64(allocations[family])
			if deficit > bestDeficit {
				best, bestDeficit = family, deficit
			}
		}
		allocations[best]++
	}
	warnings := []string{}
	streams := make([]inferredPayloadStream, 0, len(names))
	for _, family := range names {
		variant, approximate := payloadVariant(family)
		if approximate {
			warnings = append(warnings, fmt.Sprintf("Payload family %q is represented approximately by Lading generator %q.", family, variant))
		}
		sources := allocations[family]
		rate := uint64(math.Round(float64(aggregateRate) * fractions[family] / float64(sources)))
		streams = append(streams, inferredPayloadStream{Family: family, Variant: variant, Sources: sources, Rate: max(uint64(1), rate), Fraction: fractions[family], Approximate: approximate})
	}
	return streams, warnings
}

func inferFlushEvery(snapshot characterization.Snapshot, sourceCount uint64, duration float64) string {
	if sourceCount == 0 || duration <= 0 {
		return "1s"
	}
	var tailGaps uint64
	for _, group := range snapshot.Groups {
		if group.SourceType != "file" {
			continue
		}
		histogram := group.Aggregate.Interarrivals
		threshold := -1
		for index, bound := range histogram.Bounds {
			if bound == 0.1 {
				threshold = index
				break
			}
		}
		if threshold < 0 {
			continue
		}
		for index := threshold + 1; index < len(histogram.Counts); index++ {
			tailGaps += histogram.Counts[index]
		}
	}
	if tailGaps/sourceCount < 3 {
		return "1s"
	}
	estimate := duration * float64(sourceCount) / float64(tailGaps)
	choices := []float64{0.25, 0.5, 1, 2, 5, 10}
	selected := choices[0]
	best := math.Abs(math.Log(selected / estimate))
	for _, choice := range choices[1:] {
		distance := math.Abs(math.Log(choice / estimate))
		if distance < best {
			selected, best = choice, distance
		}
	}
	if selected < 1 {
		return fmt.Sprintf("%dms", int(math.Round(selected*1000)))
	}
	return fmt.Sprintf("%ds", int(math.Round(selected)))
}
func maximumBlockSize(snapshot characterization.Snapshot) string {

	if snapshot.Totals.MessageSizes.Max > 4096 {
		return "32KiB"
	}
	return "4KiB"
}
