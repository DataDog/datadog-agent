// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"os"
	"path/filepath"
	"testing"
)

// sampleRecordingsDir points to the v2 parquet recordings used for local smoke tests.
// Tests are skipped when the directory does not exist (CI and other contributors).
const sampleRecordingsDir = "/Users/celian.raimbault/Documents/recordings"

// sampleSubsetDir is a smaller subset used to keep test runtime short.
const sampleSubsetDir = "/tmp/wave4-v2-sample"

func TestDetectParquetFormat(t *testing.T) {
	t.Run("v2 when contexts.parquet present", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "contexts.parquet"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		if got := detectParquetFormat(dir); got != FormatV2 {
			t.Fatalf("expected v2, got %q", got)
		}
	})

	t.Run("v1 when no contexts.parquet", func(t *testing.T) {
		dir := t.TempDir()
		if got := detectParquetFormat(dir); got != FormatV1 {
			t.Fatalf("expected v1, got %q", got)
		}
	})
}

func TestReadAllMetricsV2_Smoke(t *testing.T) {
	dir := sampleSubsetDir
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("sample dir %s not present; run: mkdir -p %s && cp %s/contexts.parquet %s && ls %s/metrics-*.parquet | head -5 | xargs -I{} cp {} %s",
			dir, dir, sampleRecordingsDir, dir, sampleRecordingsDir, dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "contexts.parquet")); os.IsNotExist(err) {
		t.Skipf("contexts.parquet not present in %s", dir)
	}

	metrics, err := readAllMetricsV2(dir)
	if err != nil {
		t.Fatalf("readAllMetricsV2: %v", err)
	}
	if len(metrics) == 0 {
		t.Fatal("expected metrics, got none")
	}
	t.Logf("loaded %d metrics", len(metrics))
	// Spot-check first metric.
	m := metrics[0]
	if m.Name == "" {
		t.Error("metric name is empty")
	}
	if m.Timestamp == 0 {
		t.Error("metric timestamp is zero")
	}
	t.Logf("first metric: name=%q ts=%d tags=%v", m.Name, m.Timestamp, m.Tags)
}

func TestReadAllLogsV2_Smoke(t *testing.T) {
	dir := sampleSubsetDir
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("sample dir %s not present", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "contexts.parquet")); os.IsNotExist(err) {
		t.Skipf("contexts.parquet not present in %s", dir)
	}

	logs, err := readAllLogsV2(dir)
	if err != nil {
		t.Fatalf("readAllLogsV2: %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("expected logs, got none")
	}
	t.Logf("loaded %d logs", len(logs))
	l := logs[0]
	if len(l.Content) == 0 {
		t.Error("log content is empty")
	}
	t.Logf("first log: ts=%d host=%q len(content)=%d tags=%v", l.TimestampMs, l.Hostname, len(l.Content), l.Tags)
}
