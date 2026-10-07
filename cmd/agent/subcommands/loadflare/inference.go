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
const smallJSONTemplatePath = "./load-flare-assets/small-json-template.yaml"
const largeJSONTemplatePath = "./load-flare-assets/large-json-template.yaml"
const compactJSONStaticPath = "./load-flare-assets/compact-json.log"
const logfmtStaticPath = "./load-flare-assets/logfmt.log"

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

const smallJSONTemplate = `generator:
  !object
    level: !choose ["info", "warn", "error"]
    message: !choose ["ok", "retry", "failed"]
`

const largeJSONTemplate = `generator:
  !object
    timestamp: !timestamp
    level: !choose ["info", "warn", "error"]
    service: !choose ["api", "worker", "scheduler"]
    message: !choose ["large structured event", "large batch result"]
    values:
      !array
        length: { min: 2048, max: 2048 }
        element: !range { min: 0, max: 255 }
`

const compactJSONStatic = `{"level":"info","service":"api","message":"request ok","status":200,"region":"us-east-1"}
{"level":"warn","service":"worker","message":"retry job","status":429,"region":"eu-west-1"}
{"level":"error","service":"scheduler","message":"task failed","status":500,"region":"us-west-2"}
`

const logfmtStatic = `ts=2026-10-06T21:00:00Z level=info service=api method=GET path=/v1/items status=200 duration_ms=12
ts=2026-10-06T21:00:01Z level=warn service=api method=POST path=/v1/orders status=429 duration_ms=83
ts=2026-10-06T21:00:02Z level=error service=worker job=settlement error="upstream timeout" retry=2
`

var loadFlarePayloadAssets = map[string]string{
	strings.TrimPrefix(datadogJSONTemplatePath, "./"): datadogJSONTemplate,
	strings.TrimPrefix(smallJSONTemplatePath, "./"):   smallJSONTemplate,
	strings.TrimPrefix(largeJSONTemplatePath, "./"):   largeJSONTemplate,
	strings.TrimPrefix(compactJSONStaticPath, "./"):   compactJSONStatic,
	strings.TrimPrefix(logfmtStaticPath, "./"):        logfmtStatic,
}

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
	payloadFamily := observedPayloadFamily(families)
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
		"observed_payload_family":               payloadFamily,
		"inferred_payload_variant":              payloadFamily,
		"inferred_payload_streams":              streamReport,
		"rate_inference":                        rateInference,
	}
	if len(streamReport) == 0 && variant != "" {
		report["inferred_lading_generator"] = ladingGeneratorReport(variant)
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
	switch dominant {
	case "apache_common", "syslog5424", "json", "plain", "empty", "datadog_json":
	default:
		return "", "unsupported", "The dominant payload family has no safe Lading generator mapping."
	}
	warning := ""
	if float64(dominantEvents)/float64(total) < 0.95 {
		status = "partial"
		warning = "Multiple payload families were observed; the candidate represents only the dominant family."
	}
	variant, approximate := payloadVariant(dominant, families[dominant])
	if approximate {
		status = "partial"
		if warning == "" {
			warning = fmt.Sprintf("Payload family %q is represented approximately by Lading %s.", dominant, ladingGeneratorDescription(variant))
		}
	}
	return variant, status, warning
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

func observedPayloadFamily(families map[string]characterization.Aggregate) string {
	if len(families) == 0 {
		return "unknown"
	}
	if len(families) == 1 {
		for family := range families {
			return family
		}
	}
	return "mixed"
}

func ladingGeneratorDescription(variant string) string {
	generator := ladingGeneratorReport(variant)
	return fmt.Sprintf("generator kind %q", generator["kind"])
}

func payloadVariant(family string, aggregate characterization.Aggregate) (string, bool) {
	meanBytes := 0.0
	if aggregate.Events > 0 {
		meanBytes = float64(aggregate.ContentBytes) / float64(aggregate.Events)
	}
	switch family {
	case "apache_common", "syslog5424":
		return family, false
	case "json":
		switch {
		case meanBytes > 0 && meanBytes <= 64:
			return "small_json_template", true
		case meanBytes > 0 && meanBytes <= 192:
			return "compact_json_static", true
		case meanBytes >= 2048:
			return "large_json_template", true
		default:
			return "json", false
		}
	case "datadog_json":
		return "templated_json", true
	case "plain", "empty":
		if meanBytes > 0 && meanBytes <= 256 {
			return "logfmt_static", true
		}
		return "ascii", true
	default:
		return "ascii", true
	}
}

func renderedVariant(variant, indent string) string {
	templatePath := ""
	staticPath := ""
	switch variant {
	case "templated_json":
		templatePath = datadogJSONTemplatePath
	case "small_json_template":
		templatePath = smallJSONTemplatePath
	case "large_json_template":
		templatePath = largeJSONTemplatePath
	case "compact_json_static":
		staticPath = compactJSONStaticPath
	case "logfmt_static":
		staticPath = logfmtStaticPath
	}
	if templatePath != "" {
		return fmt.Sprintf("%svariant:\n%s  templated_json:\n%s    template_path: \"%s\"\n", indent, indent, indent, templatePath)
	}
	if staticPath != "" {
		return fmt.Sprintf("%svariant:\n%s  static_chunks:\n%s    static_path: \"%s\"\n", indent, indent, indent, staticPath)
	}
	return fmt.Sprintf("%svariant: \"%s\"\n", indent, variant)
}

func ladingGeneratorReport(variant string) map[string]any {
	if variant == "templated_json" {
		return map[string]any{
			"kind": "templated_json", "template_asset": strings.TrimPrefix(datadogJSONTemplatePath, "./"),
		}
	}
	for preset, asset := range map[string]string{
		"small_json_template": smallJSONTemplatePath,
		"large_json_template": largeJSONTemplatePath,
	} {
		if variant == preset {
			return map[string]any{"kind": "templated_json", "template_asset": strings.TrimPrefix(asset, "./")}
		}
	}
	for preset, asset := range map[string]string{"compact_json_static": compactJSONStaticPath, "logfmt_static": logfmtStaticPath} {
		if variant == preset {
			return map[string]any{"kind": "static_chunks", "static_asset": strings.TrimPrefix(asset, "./")}
		}
	}
	return map[string]any{"kind": "builtin", "variant": variant}
}

func inferPayloadStreams(families map[string]characterization.Aggregate, aggregateRate, sourceCount uint64) ([]inferredPayloadStream, []string) {
	if streams := inferBimodalJSONStreams(families, aggregateRate, sourceCount); len(streams) > 0 {
		return streams, []string{"A bimodal JSON size distribution is represented approximately by bounded small and large JSON templates."}
	}
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
		if sized := inferFamilySizeStreams(family, families[family], aggregateRate, allocations[family], fractions[family]); len(sized) > 0 {
			streams = append(streams, sized...)
			warnings = append(warnings, fmt.Sprintf("Payload family %q is split into approximate size-profile generators.", family))
			continue
		}
		variant, approximate := payloadVariant(family, families[family])
		if approximate {
			warnings = append(warnings, fmt.Sprintf("Payload family %q is represented approximately by Lading %s.", family, ladingGeneratorDescription(variant)))
		}
		sources := allocations[family]
		rate := uint64(math.Round(float64(aggregateRate) * fractions[family] / float64(sources)))
		streams = append(streams, inferredPayloadStream{Family: family, Variant: variant, Sources: sources, Rate: max(uint64(1), rate), Fraction: fractions[family], Approximate: approximate})
	}
	return streams, warnings
}

func inferFamilySizeStreams(family string, aggregate characterization.Aggregate, aggregateRate, sourceCount uint64, familyFraction float64) []inferredPayloadStream {
	if (family != "json" && family != "plain") || sourceCount < 2 || aggregate.Events == 0 {
		return nil
	}
	counts := map[string]uint64{}
	for index, count := range aggregate.MessageSizes.Counts {
		if count == 0 {
			continue
		}
		upperBound := aggregate.MessageSizes.Max
		if index < len(aggregate.MessageSizes.Bounds) {
			upperBound = aggregate.MessageSizes.Bounds[index]
		}
		variant := ""
		switch family {
		case "json":
			switch {
			case upperBound <= 64:
				variant = "small_json_template"
			case upperBound <= 256:
				variant = "compact_json_static"
			case upperBound > 4096:
				variant = "large_json_template"
			default:
				variant = "json"
			}
		case "plain":
			if upperBound <= 256 {
				variant = "logfmt_static"
			} else {
				variant = "ascii"
			}
		}
		counts[variant] += count
	}
	means := map[string]float64{
		"small_json_template": 35,
		"compact_json_static": 100,
		"json":                330,
		"large_json_template": 7500,
		"logfmt_static":       120,
		"ascii":               max(512, float64(aggregate.ContentBytes)/float64(aggregate.Events)),
	}
	var totalWeight float64
	weights := map[string]float64{}
	for variant, count := range counts {
		weight := float64(count) * means[variant]
		weights[variant] = weight
		totalWeight += weight
	}
	variants := make([]string, 0, len(weights))
	for variant, weight := range weights {
		if totalWeight > 0 && weight/totalWeight >= 0.02 {
			variants = append(variants, variant)
		}
	}
	if len(variants) <= 1 || uint64(len(variants)) > sourceCount {
		return nil
	}
	sort.Strings(variants)
	allocations := make(map[string]uint64, len(variants))
	for _, variant := range variants {
		allocations[variant] = 1
	}
	for remaining := sourceCount - uint64(len(variants)); remaining > 0; remaining-- {
		best := variants[0]
		bestDeficit := weights[best]/totalWeight*float64(sourceCount) - float64(allocations[best])
		for _, variant := range variants[1:] {
			deficit := weights[variant]/totalWeight*float64(sourceCount) - float64(allocations[variant])
			if deficit > bestDeficit {
				best, bestDeficit = variant, deficit
			}
		}
		allocations[best]++
	}
	streams := make([]inferredPayloadStream, 0, len(variants))
	for _, variant := range variants {
		fraction := weights[variant] / totalWeight
		sources := allocations[variant]
		rate := uint64(math.Round(float64(aggregateRate) * familyFraction * fraction / float64(sources)))
		streams = append(streams, inferredPayloadStream{Family: family, Variant: variant, Sources: sources, Rate: max(uint64(1), rate), Fraction: familyFraction * fraction, Approximate: true})
	}
	return streams
}

func inferBimodalJSONStreams(families map[string]characterization.Aggregate, aggregateRate, sourceCount uint64) []inferredPayloadStream {
	aggregate, found := families["json"]
	if !found || len(families) != 1 || sourceCount < 2 || aggregate.Events == 0 {
		return nil
	}
	var largeEvents uint64
	for index, count := range aggregate.MessageSizes.Counts {
		if index >= len(aggregate.MessageSizes.Bounds) || aggregate.MessageSizes.Bounds[index] > 4096 {
			largeEvents += count
		}
	}
	largeEventFraction := float64(largeEvents) / float64(aggregate.Events)
	if largeEventFraction < 0.003 {
		return nil
	}
	observedMean := float64(aggregate.ContentBytes) / float64(aggregate.Events)
	const smallMean = 35.0
	const largeMean = 7500.0
	modeledMean := (1-largeEventFraction)*smallMean + largeEventFraction*largeMean
	if observedMean <= 0 || math.Abs(modeledMean-observedMean)/observedMean > 0.25 {
		return nil
	}
	largeByteFraction := min(0.9, max(0.1, largeEventFraction*largeMean/observedMean))
	largeSources := uint64(math.Round(float64(sourceCount) * largeByteFraction))
	largeSources = min(sourceCount-1, max(uint64(1), largeSources))
	smallSources := sourceCount - largeSources
	return []inferredPayloadStream{
		{
			Family: "json", Variant: "small_json_template", Sources: smallSources,
			Rate:     max(uint64(1), uint64(math.Round(float64(aggregateRate)*(1-largeByteFraction)/float64(smallSources)))),
			Fraction: 1 - largeByteFraction, Approximate: true,
		},
		{
			Family: "json", Variant: "large_json_template", Sources: largeSources,
			Rate:     max(uint64(1), uint64(math.Round(float64(aggregateRate)*largeByteFraction/float64(largeSources)))),
			Fraction: largeByteFraction, Approximate: true,
		},
	}
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
