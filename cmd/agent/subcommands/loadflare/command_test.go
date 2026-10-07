// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package loadflare

import (
	"archive/zip"
	"encoding/json"
	"path/filepath"
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
	config := configmock.NewFromYAML(t, "api_key: secret\nlogs_enabled: true\n")
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

	for _, file := range reader.File {
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
	require.Equal(t, "partial", report["candidate"].(map[string]any)["status"])
}

func TestInferLadingAccountsForRotatedSourceIdentities(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	snapshot := characterization.Snapshot{
		SessionID: "rotation", StartedAt: start, EndedAt: start.Add(20 * time.Second), RequestedDurationSeconds: 20,
		Groups:          []characterization.Group{{SourceType: "file", Pipeline: "0", Aggregate: characterization.Aggregate{Events: 100, ContentBytes: 10485760, SourceCount: 8}}},
		PayloadFamilies: map[string]characterization.Aggregate{"apache_common": {Events: 99996}, "plain": {Events: 4}},
		Lifecycle:       &characterization.Lifecycle{Rotations: 4, RotationIntervals: characterization.Histogram{Count: 4, Sum: 60}},
	}
	lading, report := inferLading(snapshot)
	require.Contains(t, string(lading), "concurrent_logs: 4")
	require.Contains(t, string(lading), `constant: "131072B"`)
	require.Equal(t, uint64(8), report["observed_distinct_file_sources"])
	require.Equal(t, uint64(4), report["inferred_source_count"])
	require.Equal(t, "ready", report["candidate"].(map[string]any)["status"])
}
