// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build anomalydetection_recorder

package recorderimpl

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMetricParquetFlushesInSameSecondKeepEveryBatch(t *testing.T) {
	dir := t.TempDir()
	fixedTime := time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)
	writer, err := newMetricParquetWriter(dir, time.Hour, 0)
	require.NoError(t, err)
	writer.now = func() time.Time { return fixedTime }
	require.True(t, writer.WriteMetric("check", "first", 1, nil, 1, false))
	writer.flush()
	require.True(t, writer.WriteMetric("check", "second", 2, nil, 2, false))
	require.NoError(t, writer.Close())

	// A new writer must also avoid clobbering files left by the previous one.
	restarted, err := newMetricParquetWriter(dir, time.Hour, 0)
	require.NoError(t, err)
	restarted.now = func() time.Time { return fixedTime }
	require.True(t, restarted.WriteMetric("check", "third", 3, nil, 3, false))
	require.NoError(t, restarted.Close())

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 3)
	require.Equal(t, []string{
		"observer-metrics-20260928-120000Z.parquet",
		"observer-metrics-20260928-120000Z_000000001.parquet",
		"observer-metrics-20260928-120000Z_000000002.parquet",
	}, []string{entries[0].Name(), entries[1].Name(), entries[2].Name()})
	reader, err := newParquetReader(dir)
	require.NoError(t, err)
	require.Equal(t, 3, reader.Len())
	require.Equal(t, "first", reader.Next().MetricName)
	require.Equal(t, "second", reader.Next().MetricName)
	require.Equal(t, "third", reader.Next().MetricName)
}

func TestLogParquetFlushesInSameSecondKeepEveryBatch(t *testing.T) {
	dir := t.TempDir()
	fixedTime := time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)
	writer, err := newLogParquetWriter(dir, time.Hour, 0)
	require.NoError(t, err)
	writer.now = func() time.Time { return fixedTime }
	require.True(t, writer.WriteLog("logs", []byte("first"), "info", "agent-a", nil, 1000))
	writer.flush()
	require.True(t, writer.WriteLog("logs", []byte("second"), "info", "agent-a", nil, 2000))
	require.NoError(t, writer.Close())

	files, err := filepath.Glob(filepath.Join(dir, "observer-logs-*.parquet"))
	require.NoError(t, err)
	require.Len(t, files, 2)
	reader, err := NewLogParquetReader(dir)
	require.NoError(t, err)
	logs := reader.ReadAll()
	require.Len(t, logs, 2)
	require.Equal(t, []byte("first"), logs[0].Content)
	require.Equal(t, []byte("second"), logs[1].Content)
}
