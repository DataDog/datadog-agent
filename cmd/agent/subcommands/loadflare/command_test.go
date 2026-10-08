// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package loadflare

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs-library/characterization"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestWriteArchive(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	snapshot := characterization.Snapshot{
		SchemaVersion: 1, Kind: characterization.Kind, SessionID: "session", State: "completed",
		StartedAt: start, EndsAt: start.Add(time.Minute), EndedAt: start.Add(time.Minute), RequestedDurationSeconds: 60,
		PayloadFamilies: map[string]characterization.Aggregate{},
	}
	config := configmock.NewFromYAML(t, `api_key: api-secret-value
app_key: application-secret-value
proxy:
  http: http://proxy-user:proxy-secret-value@proxy.local:3128
logs_enabled: true
`)
	output := filepath.Join(t.TempDir(), "load-flare.zip")
	require.NoError(t, writeArchive(output, snapshot, config))

	reader, err := zip.OpenReader(output)
	require.NoError(t, err)
	defer reader.Close()
	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	require.Contains(t, names, "manifest.json")
	require.Contains(t, names, "observation.json")
	require.Contains(t, names, "inference-report.json")
	require.NotContains(t, names, "lading.yaml")
	var checkedConfig bool

	for _, file := range reader.File {
		if file.Name == "agent-config.json" {
			stream, err := file.Open()
			require.NoError(t, err)
			content, err := io.ReadAll(stream)
			require.NoError(t, err)
			require.NotContains(t, string(content), "api-secret-value")
			require.NotContains(t, string(content), "application-secret-value")
			require.NotContains(t, string(content), "proxy-secret-value")
			require.NoError(t, stream.Close())
			checkedConfig = true
		}
		if file.Name != "manifest.json" {
			continue
		}
		stream, err := file.Open()
		require.NoError(t, err)
		var value manifest
		require.NoError(t, json.NewDecoder(stream).Decode(&value))
		require.Equal(t, "datadog-agent-load-flare", value.Kind)
		require.NotEmpty(t, value.Artifacts)
		require.NoError(t, stream.Close())
	}
	require.True(t, checkedConfig)
}

func TestInferLadingForSupportedFileLoad(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	snapshot := characterization.Snapshot{
		SessionID: "session", StartedAt: start, EndedAt: start.Add(10 * time.Second), RequestedDurationSeconds: 10,
		Groups:          []characterization.Group{{SourceType: "file", Pipeline: "0", Aggregate: characterization.Aggregate{Events: 100, ContentBytes: 100000, SourceCount: 2}}},
		PayloadFamilies: map[string]characterization.Aggregate{"apache_common": {Events: 100}},
	}
	lading, report := inferLading(snapshot)
	require.Contains(t, string(lading), "traditional:")
	require.Contains(t, string(lading), "duplicates: 2")
	require.Contains(t, string(lading), `variant: "apache_common"`)
	require.Equal(t, "ready", report["candidate"].(map[string]any)["status"])
}

func TestInferLadingAccountsForRotatedSourceIdentities(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	snapshot := characterization.Snapshot{
		SessionID: "rotation", StartedAt: start, EndedAt: start.Add(20 * time.Second), RequestedDurationSeconds: 20,
		Groups:          []characterization.Group{{SourceType: "file", Pipeline: "0", Aggregate: characterization.Aggregate{Events: 100, ContentBytes: 10485760, SourceCount: 8}}},
		PayloadFamilies: map[string]characterization.Aggregate{"apache_common": {Events: 99996}, "plain": {Events: 4}},
		Lifecycle:       &characterization.Lifecycle{Rotations: 4, RotationIntervals: characterization.Histogram{Count: 4, Sum: 60}},
		RateWindows:     []characterization.RateWindow{{FileSourceCount: 4}},
	}
	lading, report := inferLading(snapshot)
	require.Contains(t, string(lading), "concurrent_logs: 4")
	require.Contains(t, string(lading), `constant: "131072B"`)
	require.Equal(t, uint64(8), report["observed_distinct_file_sources"])
	require.Equal(t, uint64(4), report["inferred_source_count"])
	require.Equal(t, "ready", report["candidate"].(map[string]any)["status"])
}

func TestInferLadingUsesRawFileBytesForGeneratorRate(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	snapshot := characterization.Snapshot{
		SessionID: "raw-rate", StartedAt: start, EndedAt: start.Add(10 * time.Second), RequestedDurationSeconds: 10,
		Groups: []characterization.Group{{
			SourceType: "file",
			Pipeline:   "0",
			Aggregate:  characterization.Aggregate{Events: 100, ContentBytes: 200000, RawBytes: 100000, SourceCount: 2},
		}},
		PayloadFamilies: map[string]characterization.Aggregate{"apache_common": {Events: 100}},
	}
	lading, report := inferLading(snapshot)
	require.Contains(t, string(lading), `bytes_per_second: "5000B"`)
	require.Equal(t, uint64(10000), report["predicted_aggregate_bytes_per_second"])
	require.Equal(t, "bounded_raw_ingress_bytes", report["rate_inference"])
}

func TestInferLadingEmitsMultipleFilePayloadStreams(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	snapshot := characterization.Snapshot{
		SessionID: "mixed", StartedAt: start, EndedAt: start.Add(10 * time.Second), RequestedDurationSeconds: 10,
		Groups: []characterization.Group{{
			SourceType: "file", Pipeline: "0",
			Aggregate: characterization.Aggregate{Events: 300, RawBytes: 30000, SourceCount: 3},
		}},
		FilePayloadFamilies: map[string]characterization.Aggregate{
			"apache_common": {Events: 100, RawBytes: 10000},
			"json":          {Events: 200, RawBytes: 20000},
		},
		RateWindows: []characterization.RateWindow{{FileSourceCount: 3}},
	}
	lading, report := inferLading(snapshot)
	rendered := string(lading)
	require.Equal(t, 2, strings.Count(rendered, "  - file_gen:"))
	require.Contains(t, rendered, `duplicates: 1`)
	require.Contains(t, rendered, `duplicates: 2`)
	require.Contains(t, rendered, `variant: "apache_common"`)
	require.Contains(t, rendered, `variant: "json"`)
	require.NotContains(t, report, "inferred_payload_variant")
	require.Equal(t, "mixed", report["observed_payload_family"])
	require.Len(t, report["inferred_payload_streams"], 2)
}

func TestInferFlushEveryFromFileInterarrivals(t *testing.T) {
	snapshot := characterization.Snapshot{
		Groups: []characterization.Group{{
			SourceType: "file",
			Aggregate: characterization.Aggregate{Interarrivals: characterization.Histogram{
				Bounds: []float64{0.001, 0.01, 0.1, 1, 5, 30, 60, 300},
				Counts: []uint64{0, 0, 0, 20, 0, 0, 0, 0, 0},
				Count:  20,
			}},
		}},
	}
	require.Equal(t, "2s", inferFlushEvery(snapshot, 4, 10))
}

func TestLifecycleRepresentationDetectsHeterogeneousCadences(t *testing.T) {
	lifecycle := &characterization.Lifecycle{
		Rotations: 10,
		RotationIntervals: characterization.Histogram{
			Bounds: []float64{5, 15},
			Counts: []uint64{4, 6, 0},
			Count:  10,
		},
	}
	require.Equal(t, "partial", lifecycleRepresentation(lifecycle)["status"])
}

func TestLifecycleRepresentationSeparatesUnobservedAndInsufficientEvidence(t *testing.T) {
	unobserved := lifecycleRepresentation(nil)
	require.Equal(t, "not_observed", unobserved["status"])
	require.Contains(t, fmt.Sprint(unobserved["warnings"]), "outside the window")

	insufficient := lifecycleRepresentation(&characterization.Lifecycle{Rotations: 1})
	require.Equal(t, "partial", insufficient["status"])
	require.Contains(t, fmt.Sprint(insufficient["warnings"]), "two in-session")
}

func TestBurstDetectionPrefersRawIngressBytes(t *testing.T) {
	windows := []characterization.RateWindow{
		{RawBytes: 100, ContentBytes: 100},
		{RawBytes: 100, ContentBytes: 1000},
	}
	require.False(t, isBursty(windows))
}

func TestInferLadingUsesLinearRateWindows(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	windows := make([]characterization.RateWindow, 0, 6)
	var rawBytes uint64
	for index := 0; index < 6; index++ {
		windowStart := start.Add(time.Duration(index*10) * time.Second)
		rate := uint64(100000 + 10000*(index*10+5))
		bytes := rate * 10
		rawBytes += bytes
		windows = append(windows, characterization.RateWindow{
			StartedAt: windowStart, EndsAt: windowStart.Add(10 * time.Second), RawBytes: bytes, FileSourceCount: 1,
		})
	}
	snapshot := characterization.Snapshot{
		SessionID: "linear", StartedAt: start, EndedAt: start.Add(60 * time.Second), RequestedDurationSeconds: 60,
		Groups:              []characterization.Group{{SourceType: "file", Aggregate: characterization.Aggregate{Events: 100, RawBytes: rawBytes, SourceCount: 1}}},
		FilePayloadFamilies: map[string]characterization.Aggregate{"apache_common": {Events: 100, RawBytes: rawBytes}},
		RateWindows:         windows,
	}
	lading, report := inferLading(snapshot)
	rendered := string(lading)
	require.Contains(t, rendered, "throttle:\n          linear:")
	require.Contains(t, rendered, `initial: "100000B"`)
	require.Contains(t, rendered, `maximum: "700000B"`)
	require.Contains(t, rendered, `rate_of_change: "10000B"`)
	require.Equal(t, "linear_ingress_rate_windows", report["rate_inference"])
	profile := report["inferred_rate_profile"].(map[string]any)
	require.InDelta(t, 1, profile["r_squared"], 0.0001)
	require.Equal(t, "raw_with_content_fallback", profile["byte_basis"])
	require.Contains(t, fmt.Sprint(report["limitations"]), "bounded to the projected rate")
}

func TestRateVariationWarningIsNotSuppressedByExistingPartialStatus(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	windows := make([]characterization.RateWindow, 0, 6)
	for index, rate := range []uint64{100000, 900000, 100000, 900000, 100000, 900000} {
		windowStart := start.Add(time.Duration(index*10) * time.Second)
		windows = append(windows, characterization.RateWindow{
			StartedAt: windowStart, EndsAt: windowStart.Add(10 * time.Second), RawBytes: rate * 10, FileSourceCount: 1,
		})
	}
	snapshot := characterization.Snapshot{
		SessionID: "variable", StartedAt: start, EndedAt: start.Add(60 * time.Second), RequestedDurationSeconds: 60,
		Groups:              []characterization.Group{{SourceType: "file", Aggregate: characterization.Aggregate{Events: 100, RawBytes: 30000000, SourceCount: 1}}},
		FilePayloadFamilies: map[string]characterization.Aggregate{"plain": {Events: 100, RawBytes: 30000000}},
		RateWindows:         windows,
	}
	_, report := inferLading(snapshot)
	warnings := fmt.Sprint(report["representability"].(map[string]any)["warnings"])
	require.Contains(t, warnings, "Material rate-window variation")
}

func TestWriteArchivePackagesDatadogJSONTemplate(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	snapshot := characterization.Snapshot{
		SchemaVersion: 1, Kind: characterization.Kind, SessionID: "datadog-json", State: "completed",
		StartedAt: start, EndsAt: start.Add(10 * time.Second), EndedAt: start.Add(10 * time.Second), RequestedDurationSeconds: 10,
		Groups: []characterization.Group{{
			SourceType: "file", Pipeline: "0",
			Aggregate: characterization.Aggregate{Events: 100, RawBytes: 100000, SourceCount: 1},
		}},
		FilePayloadFamilies: map[string]characterization.Aggregate{
			"datadog_json": {Events: 100, RawBytes: 100000},
		},
		RateWindows: []characterization.RateWindow{{FileSourceCount: 1}},
	}
	config := configmock.NewFromYAML(t, "api_key: secret\nlogs_enabled: true\n")
	output := filepath.Join(t.TempDir(), "load-flare.zip")
	require.NoError(t, writeArchive(output, snapshot, config))

	reader, err := zip.OpenReader(output)
	require.NoError(t, err)
	defer reader.Close()
	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
		if file.Name == "lading.yaml" {
			stream, openErr := file.Open()
			require.NoError(t, openErr)
			content, readErr := io.ReadAll(stream)
			require.NoError(t, readErr)
			require.Contains(t, string(content), "templated_json:")
			require.Contains(t, string(content), datadogJSONTemplatePath)
			require.NoError(t, stream.Close())
		}
	}
	require.Contains(t, names, "load-flare-assets/datadog-json-template.yaml")
}

func TestDatadogJSONMappingUsesExplicitLadingGenerator(t *testing.T) {
	variant, approximate := payloadVariant("datadog_json", characterization.Aggregate{})
	require.Equal(t, "templated_json", variant)
	require.True(t, approximate)
	require.Equal(t, map[string]any{
		"kind": "templated_json", "template_asset": "load-flare-assets/datadog-json-template.yaml",
	}, ladingGeneratorReport(variant))
}

func TestInferenceReportSeparatesObservedFamilyFromLadingGenerator(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	snapshot := characterization.Snapshot{
		SessionID: "datadog-json", StartedAt: start, EndedAt: start.Add(10 * time.Second), RequestedDurationSeconds: 10,
		Groups: []characterization.Group{{
			SourceType: "file", Pipeline: "0",
			Aggregate: characterization.Aggregate{Events: 100, ContentBytes: 20000, RawBytes: 21000, SourceCount: 1},
		}},
		PayloadFamilies: map[string]characterization.Aggregate{
			"datadog_json": {Events: 100, ContentBytes: 20000, RawBytes: 21000},
		},
	}
	_, report := inferLading(snapshot)
	require.Equal(t, "datadog_json", report["observed_payload_family"])
	require.NotContains(t, report, "inferred_payload_variant")
	require.Equal(t, map[string]any{
		"kind": "templated_json", "template_asset": "load-flare-assets/datadog-json-template.yaml",
	}, report["inferred_lading_generator"])
	require.NotContains(t, report["representability"].(map[string]any)["warnings"], "small_json_template")
}
func TestDominantUnknownPayloadRemainsUnsupported(t *testing.T) {
	variant, status, warning := dominantVariant(map[string]characterization.Aggregate{
		"unknown": {Events: 10, RawBytes: 100},
	})
	require.Empty(t, variant)
	require.Equal(t, "unsupported", status)
	require.Contains(t, warning, "no safe Lading generator")
}

func TestPayloadVariantUsesFamilyMeanSizeProfiles(t *testing.T) {
	tests := []struct {
		family  string
		content uint64
		events  uint64
		want    string
	}{
		{family: "json", content: 3200, events: 100, want: "small_json_template"},
		{family: "json", content: 12000, events: 100, want: "compact_json_static"},
		{family: "json", content: 300000, events: 100, want: "large_json_template"},
		{family: "plain", content: 12000, events: 100, want: "logfmt_static"},
	}
	for _, test := range tests {
		variant, approximate := payloadVariant(test.family, characterization.Aggregate{
			ContentBytes: test.content, Events: test.events,
		})
		require.Equal(t, test.want, variant)
		require.True(t, approximate)
	}
}

func TestInferBimodalJSONStreams(t *testing.T) {
	families := map[string]characterization.Aggregate{
		"json": {
			Events: 1000, ContentBytes: 109650, RawBytes: 110650,
			MessageSizes: characterization.Histogram{
				Bounds: []float64{64, 128, 256, 512, 1024, 4096, 16384},
				Counts: []uint64{990, 0, 0, 0, 0, 0, 10, 0},
				Count:  1000,
			},
		},
		"plain": {Events: 1, ContentBytes: 10, RawBytes: 10},
	}
	streams := inferBimodalJSONStreams(families, 262144, 4)
	require.Len(t, streams, 2)
	require.Equal(t, "small_json_template", streams[0].Variant)
	require.Equal(t, "large_json_template", streams[1].Variant)
	require.Equal(t, uint64(4), streams[0].Sources+streams[1].Sources)
	require.Contains(t, renderedVariant(streams[0].Variant, "        "), smallJSONTemplatePath)
	require.Contains(t, renderedVariant(streams[1].Variant, "        "), largeJSONTemplatePath)
}

func TestInferFamilySizeStreamsPreservesMixedJSONShapes(t *testing.T) {
	aggregate := characterization.Aggregate{
		Events: 1610, ContentBytes: 193000,
		MessageSizes: characterization.Histogram{
			Bounds: []float64{64, 128, 256, 512, 1024, 4096, 16384},
			Counts: []uint64{1000, 500, 0, 100, 0, 0, 10, 0},
			Count:  1610,
		},
	}
	streams := inferFamilySizeStreams("json", aggregate, 320000, 4, 0.5)
	require.Len(t, streams, 4)
	variants := make([]string, 0, len(streams))
	var sources uint64
	var fraction float64
	for _, stream := range streams {
		variants = append(variants, stream.Variant)
		sources += stream.Sources
		fraction += stream.Fraction
	}
	require.ElementsMatch(t, []string{
		"small_json_template", "compact_json_static", "json", "large_json_template",
	}, variants)
	require.Equal(t, uint64(4), sources)
	require.InDelta(t, 0.5, fraction, 0.0001)
}

func TestInferLinearRateProfileRejectsUnsupportedShapes(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	for name, rates := range map[string][]uint64{
		"falling": {700000, 600000, 500000, 400000, 300000, 200000},
		"step":    {100000, 100000, 100000, 900000, 900000, 900000},
		"plateau": {100000, 300000, 500000, 500000, 500000, 500000},
	} {
		t.Run(name, func(t *testing.T) {
			windows := make([]characterization.RateWindow, 0, len(rates))
			for index, rate := range rates {
				windowStart := start.Add(time.Duration(index*10) * time.Second)
				windows = append(windows, characterization.RateWindow{
					StartedAt: windowStart, EndsAt: windowStart.Add(10 * time.Second), RawBytes: rate * 10,
				})
			}
			_, ok := inferLinearRateProfile(windows, start, 60)
			require.False(t, ok)
		})
	}
}

func TestInferLadingReportsEmptySessionAsUnsupported(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	lading, report := inferLading(characterization.Snapshot{
		SessionID: "idle", StartedAt: start, EndedAt: start.Add(time.Minute), RequestedDurationSeconds: 60,
	})
	require.Empty(t, lading)
	require.Equal(t, "unsupported", report["candidate"].(map[string]any)["status"])
	require.Equal(t, "not_observed", report["lifecycle_representation"].(map[string]any)["status"])
	require.Contains(t, fmt.Sprint(report["representability"].(map[string]any)["warnings"]), "No file-source ingress")
}

func TestInferLadingMarksMixedFileAndNonFileIngressPartial(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	snapshot := characterization.Snapshot{
		SessionID: "mixed-inputs", StartedAt: start, EndedAt: start.Add(10 * time.Second), RequestedDurationSeconds: 10,
		Groups: []characterization.Group{
			{SourceType: "file", Pipeline: "0", Aggregate: characterization.Aggregate{Events: 100, RawBytes: 100000, SourceCount: 1}},
			{SourceType: "tcp", Pipeline: "0", Aggregate: characterization.Aggregate{Events: 50, RawBytes: 50000, SourceCount: 1}},
		},
		FilePayloadFamilies: map[string]characterization.Aggregate{"apache_common": {Events: 100, RawBytes: 100000}},
		RateWindows:         []characterization.RateWindow{{FileSourceCount: 1}},
	}
	lading, report := inferLading(snapshot)
	require.NotEmpty(t, lading)
	require.Equal(t, "partial", report["candidate"].(map[string]any)["status"])
	require.Contains(t, fmt.Sprint(report["representability"].(map[string]any)["warnings"]), "Non-file ingress")
}

func TestInferLadingCapsCandidateSourceCount(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	snapshot := characterization.Snapshot{
		SessionID: "many-sources", StartedAt: start, EndedAt: start.Add(10 * time.Second), RequestedDurationSeconds: 10,
		Groups: []characterization.Group{{
			SourceType: "file", Pipeline: "0",
			Aggregate: characterization.Aggregate{Events: 1000, RawBytes: 640000, SourceCount: 100},
		}},
		FilePayloadFamilies: map[string]characterization.Aggregate{"apache_common": {Events: 1000, RawBytes: 640000}},
		RateWindows:         []characterization.RateWindow{{FileSourceCount: 100}},
	}
	lading, report := inferLading(snapshot)
	require.Contains(t, string(lading), "duplicates: 64")
	require.Equal(t, uint64(64), report["inferred_source_count"])
	require.Equal(t, "partial", report["candidate"].(map[string]any)["status"])
	require.Contains(t, fmt.Sprint(report["representability"].(map[string]any)["warnings"]), "capped at 64")
}
