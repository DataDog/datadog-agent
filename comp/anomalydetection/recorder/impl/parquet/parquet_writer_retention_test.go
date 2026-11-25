// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder

package parquet

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRetentionRemovesOnlyEligibleRecorderFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, time.January, 2, 12, 0, 0, 0, time.UTC)
	metric, err := newMetricParquetWriter(dir, time.Hour, time.Hour)
	require.NoError(t, err)
	metric.now = func() time.Time { return now }
	log, err := newLogParquetWriter(dir, time.Hour, time.Hour)
	require.NoError(t, err)
	log.now = func() time.Time { return now }

	old := now.Add(-2 * time.Hour)
	for _, name := range []string{
		"observer-metrics-20260102-030405Z.parquet",
		"observer-metrics-20260102-030405Z_000000001.parquet",
		"observer-logs-20260102-030405Z.parquet",
		"observer-metrics-not-a-recording.parquet",
		"unrelated.parquet",
	} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
		require.NoError(t, os.Chtimes(path, old, old))
	}
	young := filepath.Join(dir, "observer-metrics-20260102-110000Z.parquet")
	require.NoError(t, os.WriteFile(young, []byte("young"), 0o600))
	require.NoError(t, os.Chtimes(young, now, now))

	metric.cleanup()
	require.NoFileExists(t, filepath.Join(dir, "observer-metrics-20260102-030405Z.parquet"))
	require.NoFileExists(t, filepath.Join(dir, "observer-metrics-20260102-030405Z_000000001.parquet"))
	require.FileExists(t, filepath.Join(dir, "observer-logs-20260102-030405Z.parquet"))
	require.FileExists(t, filepath.Join(dir, "observer-metrics-not-a-recording.parquet"))
	require.FileExists(t, filepath.Join(dir, "unrelated.parquet"))
	require.FileExists(t, young)

	log.cleanup()
	require.NoFileExists(t, filepath.Join(dir, "observer-logs-20260102-030405Z.parquet"))
	require.NoError(t, metric.Close())
	require.NoError(t, log.Close())
}
