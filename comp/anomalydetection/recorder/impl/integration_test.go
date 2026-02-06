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

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// mockObserverHandle is a simple mock that tracks what was observed
type mockObserverHandle struct {
	metrics []mockMetric
	logs    []mockLog
}

type mockMetric struct {
	name      string
	value     float64
	timestamp int64
	tags      []string
}

type mockLog struct {
	content   string
	status    string
	hostname  string
	timestamp int64
	tags      []string
}

func (h *mockObserverHandle) ObserveMetric(sample observer.MetricView) {
	h.metrics = append(h.metrics, mockMetric{
		name:      sample.GetName(),
		value:     sample.GetValue(),
		timestamp: int64(sample.GetTimestamp()),
		tags:      sample.GetRawTags(),
	})
}

func (h *mockObserverHandle) ObserveLog(msg observer.LogView) {
	h.logs = append(h.logs, mockLog{
		content:   string(msg.GetContent()),
		status:    msg.GetStatus(),
		hostname:  msg.GetHostname(),
		timestamp: msg.GetTimestamp(),
		tags:      msg.GetTags(),
	})
}

// mockMetricView implements observer.MetricView
type mockMetricView struct {
	name       string
	value      float64
	timestamp  float64
	tags       []string
	sampleRate float64
}

func (m *mockMetricView) GetName() string       { return m.name }
func (m *mockMetricView) GetValue() float64     { return m.value }
func (m *mockMetricView) GetTimestamp() float64 { return m.timestamp }
func (m *mockMetricView) GetRawTags() []string  { return m.tags }
func (m *mockMetricView) GetSampleRate() float64 { return m.sampleRate }

// mockLogView implements observer.LogView
type mockLogView struct {
	content   []byte
	status    string
	hostname  string
	timestamp int64
	tags      []string
}

func (m *mockLogView) GetContent() []byte   { return m.content }
func (m *mockLogView) GetStatus() string    { return m.status }
func (m *mockLogView) GetHostname() string  { return m.hostname }
func (m *mockLogView) GetTimestamp() int64  { return m.timestamp }
func (m *mockLogView) GetTags() []string    { return m.tags }

// TestRecorderHandleWrapping tests that the recorder properly wraps observer handles
func TestRecorderHandleWrapping(t *testing.T) {
	// Create temp directories for output
	metricsDir, err := os.MkdirTemp("", "recorder_metrics_test")
	require.NoError(t, err)
	defer os.RemoveAll(metricsDir)

	logsDir, err := os.MkdirTemp("", "recorder_logs_test")
	require.NoError(t, err)
	defer os.RemoveAll(logsDir)

	// Create recorder with both writers enabled
	parquetWriter, err := NewParquetWriter(metricsDir, 1*time.Hour, 0)
	require.NoError(t, err)

	logWriter, err := NewLogWriter(logsDir, 1*time.Hour, 0)
	require.NoError(t, err)

	recorder := &recorderImpl{
		parquetWriter: parquetWriter,
		logWriter:     logWriter,
	}

	// Create mock observer handle
	mockHandle := &mockObserverHandle{}

	// Create a mock HandleFunc that returns our mock handle
	mockHandleFunc := func(name string) observer.Handle {
		return mockHandle
	}

	// Wrap with recorder
	wrappedHandleFunc := recorder.GetHandle(mockHandleFunc)
	wrappedHandle := wrappedHandleFunc("test-source")

	// Send some metrics
	for i := 0; i < 5; i++ {
		wrappedHandle.ObserveMetric(&mockMetricView{
			name:      "test.metric",
			value:     float64(i * 10),
			timestamp: float64(1705315800 + i),
			tags:      []string{"env:test", "index:" + string(rune('0'+i))},
		})
	}

	// Send some logs
	for i := 0; i < 3; i++ {
		wrappedHandle.ObserveLog(&mockLogView{
			content:   []byte("Test log message " + string(rune('0'+i))),
			status:    "info",
			hostname:  "test-host",
			timestamp: int64(1705315800 + i),
			tags:      []string{"env:test"},
		})
	}

	// Verify mock handle received all observations (forwarding works)
	assert.Len(t, mockHandle.metrics, 5, "Mock handle should receive 5 metrics")
	assert.Len(t, mockHandle.logs, 3, "Mock handle should receive 3 logs")

	// Close writers to flush data
	require.NoError(t, parquetWriter.Close())
	require.NoError(t, logWriter.Close())

	// Brief pause to ensure files are written
	time.Sleep(100 * time.Millisecond)

	// Verify metrics were recorded to parquet
	metricFiles, err := filepath.Glob(filepath.Join(metricsDir, "*.parquet"))
	require.NoError(t, err)
	assert.NotEmpty(t, metricFiles, "Should have parquet files")

	// Read back metrics
	metrics, err := recorder.ReadAllMetrics(metricsDir)
	require.NoError(t, err)
	assert.Len(t, metrics, 5, "Should read back 5 metrics")

	// Verify logs were recorded to jsonl
	logFiles, err := filepath.Glob(filepath.Join(logsDir, "*.jsonl"))
	require.NoError(t, err)
	assert.NotEmpty(t, logFiles, "Should have jsonl files")

	// Read back logs
	logs, err := recorder.ReadAllLogs(logsDir)
	require.NoError(t, err)
	assert.Len(t, logs, 3, "Should read back 3 logs")

	// Verify log content
	assert.Contains(t, logs[0].Content, "Test log message")
	assert.Equal(t, "info", logs[0].Status)
	assert.Equal(t, "test-host", logs[0].Hostname)
}

// TestRecorderNoWriters tests that handle wrapping works even without writers
func TestRecorderNoWriters(t *testing.T) {
	// Create recorder with no writers
	recorder := &recorderImpl{}

	// Create mock observer handle
	mockHandle := &mockObserverHandle{}
	mockHandleFunc := func(name string) observer.Handle {
		return mockHandle
	}

	// Wrap with recorder (should pass through since no writers)
	wrappedHandleFunc := recorder.GetHandle(mockHandleFunc)
	wrappedHandle := wrappedHandleFunc("test-source")

	// Send metric
	wrappedHandle.ObserveMetric(&mockMetricView{
		name:  "test.metric",
		value: 42.0,
	})

	// Send log
	wrappedHandle.ObserveLog(&mockLogView{
		content: []byte("Test log"),
		status:  "info",
	})

	// Verify mock handle still received observations
	assert.Len(t, mockHandle.metrics, 1)
	assert.Len(t, mockHandle.logs, 1)
}

// TestRecorderOnlyMetrics tests recording only metrics (no log writer)
func TestRecorderOnlyMetrics(t *testing.T) {
	metricsDir, err := os.MkdirTemp("", "recorder_only_metrics")
	require.NoError(t, err)
	defer os.RemoveAll(metricsDir)

	parquetWriter, err := NewParquetWriter(metricsDir, 1*time.Hour, 0)
	require.NoError(t, err)

	recorder := &recorderImpl{
		parquetWriter: parquetWriter,
		// No log writer
	}

	mockHandle := &mockObserverHandle{}
	wrappedHandle := recorder.GetHandle(func(name string) observer.Handle {
		return mockHandle
	})("test")

	// Send both metrics and logs
	wrappedHandle.ObserveMetric(&mockMetricView{name: "test", value: 1.0, timestamp: 1705315800})
	wrappedHandle.ObserveLog(&mockLogView{content: []byte("test"), timestamp: 1705315800})

	// Both should be forwarded to mock
	assert.Len(t, mockHandle.metrics, 1)
	assert.Len(t, mockHandle.logs, 1)

	require.NoError(t, parquetWriter.Close())
	time.Sleep(50 * time.Millisecond)

	// Only metrics should be recorded
	metrics, err := recorder.ReadAllMetrics(metricsDir)
	require.NoError(t, err)
	assert.Len(t, metrics, 1)
}

// TestRecorderOnlyLogs tests recording only logs (no parquet writer)
func TestRecorderOnlyLogs(t *testing.T) {
	logsDir, err := os.MkdirTemp("", "recorder_only_logs")
	require.NoError(t, err)
	defer os.RemoveAll(logsDir)

	logWriter, err := NewLogWriter(logsDir, 1*time.Hour, 0)
	require.NoError(t, err)

	recorder := &recorderImpl{
		// No parquet writer
		logWriter: logWriter,
	}

	mockHandle := &mockObserverHandle{}
	wrappedHandle := recorder.GetHandle(func(name string) observer.Handle {
		return mockHandle
	})("test")

	// Send both metrics and logs
	wrappedHandle.ObserveMetric(&mockMetricView{name: "test", value: 1.0, timestamp: 1705315800})
	wrappedHandle.ObserveLog(&mockLogView{content: []byte("test log"), timestamp: 1705315800})

	// Both should be forwarded to mock
	assert.Len(t, mockHandle.metrics, 1)
	assert.Len(t, mockHandle.logs, 1)

	require.NoError(t, logWriter.Close())
	time.Sleep(50 * time.Millisecond)

	// Only logs should be recorded
	logs, err := recorder.ReadAllLogs(logsDir)
	require.NoError(t, err)
	assert.Len(t, logs, 1)
	assert.Equal(t, "test log", logs[0].Content)
}
