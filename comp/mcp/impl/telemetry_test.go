// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package mcpimpl

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMCPTelemetry_NewTelemetry(t *testing.T) {
	tel := newMCPTelemetry()
	assert.NotNil(t, tel)
	assert.Equal(t, int64(0), tel.GetActiveConnections())
}

func TestMCPTelemetry_RequestCount(t *testing.T) {
	tel := newMCPTelemetry()

	// Initial count should be 0
	assert.Equal(t, int64(0), tel.GetRequestCount("test-tool"))

	// Increment and verify
	tel.IncrementRequestCount("test-tool")
	assert.Equal(t, int64(1), tel.GetRequestCount("test-tool"))

	// Multiple increments
	tel.IncrementRequestCount("test-tool")
	tel.IncrementRequestCount("test-tool")
	assert.Equal(t, int64(3), tel.GetRequestCount("test-tool"))

	// Different tools should have separate counts
	tel.IncrementRequestCount("other-tool")
	assert.Equal(t, int64(1), tel.GetRequestCount("other-tool"))
	assert.Equal(t, int64(3), tel.GetRequestCount("test-tool"))
}

func TestMCPTelemetry_ErrorCount(t *testing.T) {
	tel := newMCPTelemetry()

	// Initial count should be 0
	assert.Equal(t, int64(0), tel.GetErrorCount("test-tool"))

	// Increment and verify
	tel.IncrementErrorCount("test-tool")
	assert.Equal(t, int64(1), tel.GetErrorCount("test-tool"))

	// Multiple increments
	tel.IncrementErrorCount("test-tool")
	assert.Equal(t, int64(2), tel.GetErrorCount("test-tool"))

	// Different tools should have separate counts
	tel.IncrementErrorCount("other-tool")
	assert.Equal(t, int64(1), tel.GetErrorCount("other-tool"))
}

func TestMCPTelemetry_ActiveConnections(t *testing.T) {
	tel := newMCPTelemetry()

	// Initial count should be 0
	assert.Equal(t, int64(0), tel.GetActiveConnections())

	// Increment
	tel.IncrementActiveConnections()
	assert.Equal(t, int64(1), tel.GetActiveConnections())

	tel.IncrementActiveConnections()
	assert.Equal(t, int64(2), tel.GetActiveConnections())

	// Decrement
	tel.DecrementActiveConnections()
	assert.Equal(t, int64(1), tel.GetActiveConnections())

	tel.DecrementActiveConnections()
	assert.Equal(t, int64(0), tel.GetActiveConnections())
}

func TestMCPTelemetry_Latency(t *testing.T) {
	tel := newMCPTelemetry()

	// Initial average should be 0
	assert.Equal(t, float64(0), tel.GetAverageLatency("test-tool"))

	// Record some latencies
	tel.RecordLatency("test-tool", 100*time.Millisecond)
	tel.RecordLatency("test-tool", 200*time.Millisecond)
	tel.RecordLatency("test-tool", 300*time.Millisecond)

	// Average should be 200ms
	avg := tel.GetAverageLatency("test-tool")
	assert.InDelta(t, 200.0, avg, 1.0)
}

func TestMCPTelemetry_Stats(t *testing.T) {
	tel := newMCPTelemetry()

	// Add some data
	tel.IncrementRequestCount("tool-a")
	tel.IncrementRequestCount("tool-a")
	tel.IncrementRequestCount("tool-b")
	tel.IncrementErrorCount("tool-a")
	tel.RecordLatency("tool-a", 100*time.Millisecond)
	tel.RecordLatency("tool-b", 50*time.Millisecond)
	tel.IncrementActiveConnections()

	stats := tel.Stats()

	// Check active connections
	assert.Equal(t, int64(1), stats["active_connections"])

	// Check request counts
	reqCounts := stats["request_counts"].(map[string]int64)
	assert.Equal(t, int64(2), reqCounts["tool-a"])
	assert.Equal(t, int64(1), reqCounts["tool-b"])

	// Check error counts
	errCounts := stats["error_counts"].(map[string]int64)
	assert.Equal(t, int64(1), errCounts["tool-a"])

	// Check latencies
	latencies := stats["average_latencies_ms"].(map[string]float64)
	assert.InDelta(t, 100.0, latencies["tool-a"], 1.0)
	assert.InDelta(t, 50.0, latencies["tool-b"], 1.0)
}

func TestMCPTelemetry_ConcurrentAccess(t *testing.T) {
	tel := newMCPTelemetry()
	var wg sync.WaitGroup

	// Concurrent increments
	numGoroutines := 100
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			tel.IncrementRequestCount("concurrent-tool")
			tel.IncrementErrorCount("concurrent-tool")
			tel.RecordLatency("concurrent-tool", 10*time.Millisecond)
			tel.IncrementActiveConnections()
			tel.DecrementActiveConnections()
		}()
	}

	wg.Wait()

	// All increments should be counted
	assert.Equal(t, int64(numGoroutines), tel.GetRequestCount("concurrent-tool"))
	assert.Equal(t, int64(numGoroutines), tel.GetErrorCount("concurrent-tool"))
	assert.Equal(t, int64(0), tel.GetActiveConnections()) // Should be back to 0
	assert.InDelta(t, 10.0, tel.GetAverageLatency("concurrent-tool"), 1.0)
}
