// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package mcpimpl

import (
	"sync"
	"sync/atomic"
	"time"
)

// mcpTelemetry tracks MCP-related metrics for monitoring and debugging.
type mcpTelemetry struct {
	// Request counts by tool name
	requestCounts sync.Map // map[string]*int64

	// Error counts by tool name
	errorCounts sync.Map // map[string]*int64

	// Total latency by tool name (in nanoseconds)
	totalLatency sync.Map // map[string]*int64

	// Request count by tool name for latency calculation
	latencyRequestCounts sync.Map // map[string]*int64

	// Active connections
	activeConnections int64
}

// newMCPTelemetry creates a new mcpTelemetry instance.
func newMCPTelemetry() *mcpTelemetry {
	return &mcpTelemetry{}
}

// IncrementRequestCount increments the request count for a specific tool.
func (t *mcpTelemetry) IncrementRequestCount(toolName string) {
	counter := t.getOrCreateCounter(&t.requestCounts, toolName)
	atomic.AddInt64(counter, 1)
}

// IncrementErrorCount increments the error count for a specific tool.
func (t *mcpTelemetry) IncrementErrorCount(toolName string) {
	counter := t.getOrCreateCounter(&t.errorCounts, toolName)
	atomic.AddInt64(counter, 1)
}

// RecordLatency records the latency for a specific tool request.
func (t *mcpTelemetry) RecordLatency(toolName string, duration time.Duration) {
	latencyCounter := t.getOrCreateCounter(&t.totalLatency, toolName)
	atomic.AddInt64(latencyCounter, int64(duration))

	requestCounter := t.getOrCreateCounter(&t.latencyRequestCounts, toolName)
	atomic.AddInt64(requestCounter, 1)
}

// IncrementActiveConnections increments the active connection count.
func (t *mcpTelemetry) IncrementActiveConnections() {
	atomic.AddInt64(&t.activeConnections, 1)
}

// DecrementActiveConnections decrements the active connection count.
func (t *mcpTelemetry) DecrementActiveConnections() {
	atomic.AddInt64(&t.activeConnections, -1)
}

// GetRequestCount returns the request count for a specific tool.
func (t *mcpTelemetry) GetRequestCount(toolName string) int64 {
	if counter, ok := t.requestCounts.Load(toolName); ok {
		return atomic.LoadInt64(counter.(*int64))
	}
	return 0
}

// GetErrorCount returns the error count for a specific tool.
func (t *mcpTelemetry) GetErrorCount(toolName string) int64 {
	if counter, ok := t.errorCounts.Load(toolName); ok {
		return atomic.LoadInt64(counter.(*int64))
	}
	return 0
}

// GetAverageLatency returns the average latency for a specific tool in milliseconds.
func (t *mcpTelemetry) GetAverageLatency(toolName string) float64 {
	totalLatencyVal, latencyOk := t.totalLatency.Load(toolName)
	requestCountVal, countOk := t.latencyRequestCounts.Load(toolName)

	if !latencyOk || !countOk {
		return 0
	}

	total := atomic.LoadInt64(totalLatencyVal.(*int64))
	count := atomic.LoadInt64(requestCountVal.(*int64))

	if count == 0 {
		return 0
	}

	// Convert nanoseconds to milliseconds
	return float64(total) / float64(count) / float64(time.Millisecond)
}

// GetActiveConnections returns the current number of active connections.
func (t *mcpTelemetry) GetActiveConnections() int64 {
	return atomic.LoadInt64(&t.activeConnections)
}

// getOrCreateCounter returns an existing counter or creates a new one atomically.
func (t *mcpTelemetry) getOrCreateCounter(m *sync.Map, key string) *int64 {
	if val, ok := m.Load(key); ok {
		return val.(*int64)
	}

	newCounter := new(int64)
	actual, _ := m.LoadOrStore(key, newCounter)
	return actual.(*int64)
}

// Stats returns a map of all telemetry statistics for monitoring.
func (t *mcpTelemetry) Stats() map[string]interface{} {
	stats := make(map[string]interface{})

	stats["active_connections"] = t.GetActiveConnections()

	// Collect request counts
	requestCounts := make(map[string]int64)
	t.requestCounts.Range(func(key, value interface{}) bool {
		requestCounts[key.(string)] = atomic.LoadInt64(value.(*int64))
		return true
	})
	stats["request_counts"] = requestCounts

	// Collect error counts
	errorCounts := make(map[string]int64)
	t.errorCounts.Range(func(key, value interface{}) bool {
		errorCounts[key.(string)] = atomic.LoadInt64(value.(*int64))
		return true
	})
	stats["error_counts"] = errorCounts

	// Collect average latencies
	avgLatencies := make(map[string]float64)
	t.latencyRequestCounts.Range(func(key, _ interface{}) bool {
		avgLatencies[key.(string)] = t.GetAverageLatency(key.(string))
		return true
	})
	stats["average_latencies_ms"] = avgLatencies

	return stats
}
