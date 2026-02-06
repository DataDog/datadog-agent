// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package recorderimpl

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// TestE2E_CollectStoreReplay simulates the full live pipeline:
//
//  1. COLLECT: Wire recorder around an observer handle, push metrics + logs
//     (as the real agent does via demux + logs pipeline)
//  2. STORE: Flush to disk (parquet for metrics, JSONL for logs)
//  3. REPLAY: Read back via ReadAllMetrics/ReadAllLogs and verify data integrity,
//     then feed logs back through LogData.LogView interface to verify replay compatibility
func TestE2E_CollectStoreReplay(t *testing.T) {
	// --- Setup: create output directories ---
	scenarioDir, err := os.MkdirTemp("", "e2e_scenario")
	require.NoError(t, err)
	defer os.RemoveAll(scenarioDir)

	metricsDir := filepath.Join(scenarioDir, "parquet")
	logsDir := filepath.Join(scenarioDir, "logs")
	require.NoError(t, os.MkdirAll(metricsDir, 0755))
	require.NoError(t, os.MkdirAll(logsDir, 0755))

	// --- Phase 1: COLLECT ---
	// Create recorder writers (simulates what NewComponent does with config)
	parquetWriter, err := NewParquetWriter(metricsDir, 1*time.Hour, 0)
	require.NoError(t, err)
	logWriter, err := NewLogWriter(logsDir, 1*time.Hour, 0)
	require.NoError(t, err)

	rec := &recorderImpl{
		parquetWriter: parquetWriter,
		logWriter:     logWriter,
	}

	// The observer handle that the recorder wraps (simulates the real observer)
	observerSeen := &mockObserverHandle{}
	wrappedHandleFunc := rec.GetHandle(func(name string) observer.Handle {
		return observerSeen
	})

	// Simulate two sources (as the real agent has: "all-metrics" for demux, "logs" for log pipeline)
	metricsHandle := wrappedHandleFunc("all-metrics")
	logsHandle := wrappedHandleFunc("logs")

	// Push a realistic scenario: CPU spike + error logs
	baseTime := int64(1705315800)

	// Normal baseline metrics (CPU at ~30%)
	for i := 0; i < 20; i++ {
		metricsHandle.ObserveMetric(&mockMetricView{
			name:      "system.cpu.user",
			value:     30.0 + float64(i%3),
			timestamp: float64(baseTime + int64(i)),
			tags:      []string{"host:web-01"},
		})
	}

	// Anomalous spike (CPU jumps to 95%)
	for i := 20; i < 30; i++ {
		metricsHandle.ObserveMetric(&mockMetricView{
			name:      "system.cpu.user",
			value:     95.0 + float64(i%5),
			timestamp: float64(baseTime + int64(i)),
			tags:      []string{"host:web-01"},
		})
	}

	// Recovery (CPU back to ~30%)
	for i := 30; i < 40; i++ {
		metricsHandle.ObserveMetric(&mockMetricView{
			name:      "system.cpu.user",
			value:     30.0 + float64(i%3),
			timestamp: float64(baseTime + int64(i)),
			tags:      []string{"host:web-01"},
		})
	}

	// Logs from the logs pipeline
	normalLogs := []struct {
		content  string
		status   string
		hostname string
	}{
		{"Request processed successfully", "info", "web-01"},
		{"Cache hit for key user:1234", "debug", "web-01"},
		{"Health check passed", "info", "web-01"},
	}
	for i, l := range normalLogs {
		logsHandle.ObserveLog(&mockLogView{
			content:   []byte(l.content),
			status:    l.status,
			hostname:  l.hostname,
			timestamp: baseTime + int64(i),
			tags:      []string{"service:api", "env:prod"},
		})
	}

	// Error logs during the spike
	errorLogs := []struct {
		content  string
		status   string
		hostname string
	}{
		{"connection pool exhausted: max connections reached", "error", "web-01"},
		{"request timeout after 30s: upstream unresponsive", "error", "web-01"},
		{"circuit breaker open: 5 failures in 10s window", "error", "web-01"},
		{"OOM kill detected for process api-server (pid 4521)", "error", "web-01"},
		{"disk I/O latency spike: p99=450ms (threshold 100ms)", "warn", "web-01"},
	}
	for i, l := range errorLogs {
		logsHandle.ObserveLog(&mockLogView{
			content:   []byte(l.content),
			status:    l.status,
			hostname:  l.hostname,
			timestamp: baseTime + int64(20+i), // During the spike window
			tags:      []string{"service:api", "env:prod"},
		})
	}

	// Verify the observer received everything (forwarding works)
	assert.Equal(t, 40, len(observerSeen.metrics), "Observer should see all 40 metrics")
	assert.Equal(t, 8, len(observerSeen.logs), "Observer should see all 8 logs")

	// --- Phase 2: STORE ---
	require.NoError(t, parquetWriter.Close())
	require.NoError(t, logWriter.Close())
	time.Sleep(100 * time.Millisecond) // Ensure files are flushed

	// Verify files exist
	parquetFiles, err := filepath.Glob(filepath.Join(metricsDir, "*.parquet"))
	require.NoError(t, err)
	assert.NotEmpty(t, parquetFiles, "Should have parquet files")

	jsonlFiles, err := filepath.Glob(filepath.Join(logsDir, "*.jsonl"))
	require.NoError(t, err)
	assert.NotEmpty(t, jsonlFiles, "Should have JSONL files")

	// --- Phase 3: REPLAY ---
	// Read back all data (as the testbench does via ReadAllMetrics/ReadAllLogs)
	metrics, err := rec.ReadAllMetrics(metricsDir)
	require.NoError(t, err)
	assert.Equal(t, 40, len(metrics), "Should replay all 40 metrics")

	logs, err := rec.ReadAllLogs(logsDir)
	require.NoError(t, err)
	assert.Equal(t, 8, len(logs), "Should replay all 8 logs")

	// Verify metric data integrity
	var spikeMetrics []recorderdef.MetricData
	for _, m := range metrics {
		assert.Equal(t, "system.cpu.user", m.Name)
		if m.Value > 90 {
			spikeMetrics = append(spikeMetrics, m)
		}
	}
	assert.Equal(t, 10, len(spikeMetrics), "Should find 10 spike metrics")

	// Verify log data integrity
	var errorCount, warnCount, infoCount int
	for _, l := range logs {
		assert.NotEmpty(t, l.Content, "Log content should not be empty")
		assert.NotEmpty(t, l.Hostname, "Hostname should be preserved")
		assert.Equal(t, "web-01", l.Hostname)
		assert.Contains(t, l.Tags, "service:api")
		assert.Contains(t, l.Tags, "env:prod")

		switch l.Status {
		case "error":
			errorCount++
		case "warn":
			warnCount++
		case "info":
			infoCount++
		case "debug":
			// OK
		}
	}
	assert.Equal(t, 4, errorCount, "Should have 4 error logs")
	assert.Equal(t, 1, warnCount, "Should have 1 warn log")
	assert.Equal(t, 2, infoCount, "Should have 2 info logs")

	// Verify logs are in chronological order
	for i := 1; i < len(logs); i++ {
		assert.GreaterOrEqual(t, logs[i].Timestamp, logs[i-1].Timestamp,
			"Logs should be in chronological order at index %d", i)
	}

	// Verify LogData implements LogView (the key for replay compatibility)
	// This proves recorder output can be fed directly back into log processors
	for i := range logs {
		var view observer.LogView = &logs[i]
		assert.Equal(t, logs[i].Content, string(view.GetContent()))
		assert.Equal(t, logs[i].Status, view.GetStatus())
		assert.Equal(t, logs[i].Hostname, view.GetHostname())
		assert.Equal(t, logs[i].Timestamp, view.GetTimestamp())
		assert.Equal(t, logs[i].Tags, view.GetTags())
	}

	// Verify source tracking (recorder tags each observation with the handle name)
	for _, l := range logs {
		assert.Equal(t, "logs", l.Source, "Log source should be the handle name")
	}
	for _, m := range metrics {
		assert.Equal(t, "all-metrics", m.Source, "Metric source should be the handle name")
	}

	fmt.Printf("E2E test passed: %d metrics + %d logs collected, stored, and replayed\n",
		len(metrics), len(logs))
}
