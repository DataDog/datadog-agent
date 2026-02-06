// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package recorderimpl

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
)

// TestReadAllLogs verifies the ReadAllLogs method works correctly
func TestReadAllLogs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recorder_logs_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create log files directly
	writer, err := NewLogWriter(tmpDir, 1*time.Hour, 0)
	require.NoError(t, err)

	writer.WriteLog(1705315800, "Log message 1", []string{"env:test"}, "host1", "info", "source1")
	writer.WriteLog(1705315801, "Log message 2", []string{"env:prod"}, "host2", "error", "source2")
	writer.WriteLog(1705315802, "Log message 3", nil, "host1", "warn", "source1")

	err = writer.Close()
	require.NoError(t, err)

	// Create recorder and read logs
	r := &recorderImpl{}
	logs, err := r.ReadAllLogs(tmpDir)
	require.NoError(t, err)

	assert.Len(t, logs, 3)

	// Verify logs are sorted by timestamp
	assert.Equal(t, "Log message 1", logs[0].Content)
	assert.Equal(t, "host1", logs[0].Hostname)
	assert.Equal(t, "info", logs[0].Status)
	assert.Equal(t, []string{"env:test"}, logs[0].Tags)

	assert.Equal(t, "Log message 2", logs[1].Content)
	assert.Equal(t, "error", logs[1].Status)

	assert.Equal(t, "Log message 3", logs[2].Content)
	assert.Equal(t, "warn", logs[2].Status)
}

// TestReadAllMetricsAndLogs verifies both metrics and logs can be read
func TestReadAllMetricsAndLogs(t *testing.T) {
	// Create separate dirs for metrics and logs
	metricsDir, err := os.MkdirTemp("", "recorder_metrics_test")
	require.NoError(t, err)
	defer os.RemoveAll(metricsDir)

	logsDir, err := os.MkdirTemp("", "recorder_logs_test")
	require.NoError(t, err)
	defer os.RemoveAll(logsDir)

	// Write metrics
	metricWriter, err := NewParquetWriter(metricsDir, 1*time.Second, 0)
	require.NoError(t, err)
	metricWriter.WriteMetric("test-source", "cpu.usage", 75.5, []string{"host:server1"}, 1705315800)
	metricWriter.WriteMetric("test-source", "memory.usage", 60.0, []string{"host:server1"}, 1705315801)
	require.NoError(t, metricWriter.Close())

	// Write logs
	logWriter, err := NewLogWriter(logsDir, 1*time.Hour, 0)
	require.NoError(t, err)
	logWriter.WriteLog(1705315800, "CPU spike detected", []string{"host:server1"}, "server1", "warn", "monitor")
	logWriter.WriteLog(1705315801, "Memory pressure", []string{"host:server1"}, "server1", "error", "monitor")
	require.NoError(t, logWriter.Close())

	// Brief pause to ensure files are flushed
	time.Sleep(100 * time.Millisecond)

	// Read both using recorder
	r := &recorderImpl{}

	metrics, err := r.ReadAllMetrics(metricsDir)
	require.NoError(t, err)
	assert.Len(t, metrics, 2)
	assert.Equal(t, "cpu.usage", metrics[0].Name)
	assert.Equal(t, 75.5, metrics[0].Value)

	logs, err := r.ReadAllLogs(logsDir)
	require.NoError(t, err)
	assert.Len(t, logs, 2)
	assert.Equal(t, "CPU spike detected", logs[0].Content)
	assert.Equal(t, "warn", logs[0].Status)
}

// TestLogDataStruct verifies the LogData struct fields
func TestLogDataStruct(t *testing.T) {
	log := recorderdef.LogData{
		Source:    "my-app",
		Content:   "Application started",
		Status:    "info",
		Hostname:  "app-server-1",
		Timestamp: 1705315800,
		Tags:      []string{"env:prod", "version:1.0"},
	}

	assert.Equal(t, "my-app", log.Source)
	assert.Equal(t, "Application started", log.Content)
	assert.Equal(t, "info", log.Status)
	assert.Equal(t, "app-server-1", log.Hostname)
	assert.Equal(t, int64(1705315800), log.Timestamp)
	assert.Equal(t, []string{"env:prod", "version:1.0"}, log.Tags)
}

// TestMultipleLogFiles verifies reading from multiple log files
func TestMultipleLogFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "multi_log_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create first log file manually
	file1 := filepath.Join(tmpDir, "logs-001.jsonl")
	err = os.WriteFile(file1, []byte(
		`{"timestamp":1705315800,"content":"First file log 1","status":"info"}
{"timestamp":1705315802,"content":"First file log 2","status":"warn"}
`), 0644)
	require.NoError(t, err)

	// Create second log file
	file2 := filepath.Join(tmpDir, "logs-002.jsonl")
	err = os.WriteFile(file2, []byte(
		`{"timestamp":1705315801,"content":"Second file log","status":"error"}
`), 0644)
	require.NoError(t, err)

	// Read all logs
	r := &recorderImpl{}
	logs, err := r.ReadAllLogs(tmpDir)
	require.NoError(t, err)

	// Should have 3 logs from both files, sorted by timestamp
	assert.Len(t, logs, 3)
	assert.Equal(t, "First file log 1", logs[0].Content)   // ts: 1705315800
	assert.Equal(t, "Second file log", logs[1].Content)    // ts: 1705315801
	assert.Equal(t, "First file log 2", logs[2].Content)   // ts: 1705315802
}
