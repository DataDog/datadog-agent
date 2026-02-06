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
)

func TestLogWriterBasic(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "log_writer_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create log writer with long flush interval (we'll close manually)
	writer, err := NewLogWriter(tmpDir, 1*time.Hour, 0)
	require.NoError(t, err)

	// Write some logs
	writer.WriteLog(1705315800, "First log message", []string{"env:test"}, "host1", "info", "test-source")
	writer.WriteLog(1705315801, "Error occurred", []string{"env:test", "service:api"}, "host1", "error", "test-source")
	writer.WriteLog(1705315802, "Warning message", nil, "host2", "warn", "test-source")

	// Close writer (flushes data)
	err = writer.Close()
	require.NoError(t, err)

	// Verify files were created
	files, err := filepath.Glob(filepath.Join(tmpDir, "*.jsonl"))
	require.NoError(t, err)
	assert.Len(t, files, 1, "Expected one log file")

	// Read back using log reader
	reader, err := NewLogReader(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, 3, reader.Len(), "Expected 3 log entries")

	// Verify logs are in order
	logs := reader.All()
	assert.Equal(t, "First log message", logs[0].Content)
	assert.Equal(t, "info", logs[0].Status)
	assert.Equal(t, "host1", logs[0].Hostname)
	assert.Equal(t, int64(1705315800), logs[0].Timestamp)

	assert.Equal(t, "Error occurred", logs[1].Content)
	assert.Equal(t, "error", logs[1].Status)
	assert.Equal(t, []string{"env:test", "service:api"}, logs[1].Tags)

	assert.Equal(t, "Warning message", logs[2].Content)
	assert.Equal(t, "warn", logs[2].Status)
	assert.Equal(t, "host2", logs[2].Hostname)
}

func TestLogReaderIterator(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "log_reader_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create log writer
	writer, err := NewLogWriter(tmpDir, 1*time.Hour, 0)
	require.NoError(t, err)

	// Write logs with different timestamps (out of order)
	writer.WriteLog(1705315803, "Third", nil, "", "info", "src")
	writer.WriteLog(1705315801, "First", nil, "", "info", "src")
	writer.WriteLog(1705315802, "Second", nil, "", "info", "src")

	err = writer.Close()
	require.NoError(t, err)

	// Read with iterator
	reader, err := NewLogReader(tmpDir)
	require.NoError(t, err)

	// Logs should be sorted by timestamp
	log1 := reader.Next()
	require.NotNil(t, log1)
	assert.Equal(t, "First", log1.Content)
	assert.Equal(t, int64(1705315801), log1.Timestamp)

	log2 := reader.Next()
	require.NotNil(t, log2)
	assert.Equal(t, "Second", log2.Content)

	log3 := reader.Next()
	require.NotNil(t, log3)
	assert.Equal(t, "Third", log3.Content)

	// No more logs
	assert.Nil(t, reader.Next())

	// Reset and read again
	reader.Reset()
	assert.Equal(t, "First", reader.Next().Content)

	// Check time range
	assert.Equal(t, int64(1705315801), reader.StartTime())
	assert.Equal(t, int64(1705315803), reader.EndTime())
}

func TestLogWriterEmptyTags(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "log_empty_tags_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	writer, err := NewLogWriter(tmpDir, 1*time.Hour, 0)
	require.NoError(t, err)

	// Write log with no tags
	writer.WriteLog(1705315800, "No tags log", nil, "", "", "")

	err = writer.Close()
	require.NoError(t, err)

	reader, err := NewLogReader(tmpDir)
	require.NoError(t, err)

	log := reader.Next()
	require.NotNil(t, log)
	assert.Equal(t, "No tags log", log.Content)
	assert.Empty(t, log.Tags)
	assert.Empty(t, log.Hostname)
	assert.Empty(t, log.Status)
}

func TestLogReaderNoFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "log_no_files_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Try to read from empty directory
	_, err = NewLogReader(tmpDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no log files found")
}

func TestLogWriterDoubleClose(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "log_double_close_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	writer, err := NewLogWriter(tmpDir, 1*time.Hour, 0)
	require.NoError(t, err)

	// First close should succeed
	err = writer.Close()
	require.NoError(t, err)

	// Second close should also succeed (idempotent)
	err = writer.Close()
	require.NoError(t, err)
}
