// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// mockHandle captures observations for testing.
type mockHandle struct {
	mu      sync.Mutex
	metrics []capturedMetric
	logs    []capturedLog
}

type capturedMetric struct {
	name  string
	value float64
}

type capturedLog struct {
	content string
}

func (h *mockHandle) ObserveMetric(sample observer.MetricView) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.metrics = append(h.metrics, capturedMetric{
		name:  sample.GetName(),
		value: sample.GetValue(),
	})
}

func (h *mockHandle) ObserveLog(msg observer.LogView) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.logs = append(h.logs, capturedLog{
		content: string(msg.GetContent()),
	})
}

func (h *mockHandle) getMetrics() []capturedMetric {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]capturedMetric, len(h.metrics))
	copy(result, h.metrics)
	return result
}

func (h *mockHandle) getLogs() []capturedLog {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]capturedLog, len(h.logs))
	copy(result, h.logs)
	return result
}

func TestGenerator_EmitsMetricsAtScaledInterval(t *testing.T) {
	handle := &mockHandle{}
	config := GeneratorConfig{
		TimeScale:     0.01, // 100x faster
		BaselineNoise: 0.0,  // No noise for predictable testing
	}
	gen := NewDataGenerator(handle, config)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	go gen.Run(ctx)

	// Wait for generator to run
	<-ctx.Done()
	time.Sleep(10 * time.Millisecond) // Allow final tick to process

	metrics := handle.getMetrics()

	// At 0.01 TimeScale, tick interval is 10ms
	// In 50ms, we should get about 5 ticks, each producing 2 metrics
	// Allow some flexibility due to timing
	assert.GreaterOrEqual(t, len(metrics), 4, "expected at least 2 ticks worth of metrics")

	// Verify we get both metric types
	var hasRetransmits, hasLockContention bool
	for _, m := range metrics {
		if m.name == "network.retransmits" {
			hasRetransmits = true
		}
		if m.name == "ebpf.lock_contention_ns" {
			hasLockContention = true
		}
	}
	assert.True(t, hasRetransmits, "expected network.retransmits metric")
	assert.True(t, hasLockContention, "expected ebpf.lock_contention_ns metric")
}

func TestGenerator_BaselinePhaseValues(t *testing.T) {
	handle := &mockHandle{}
	config := GeneratorConfig{
		TimeScale:     0.01, // 100x faster, so 100ms real = 10s simulation (baseline phase)
		BaselineNoise: 0.0,  // No noise for predictable testing
	}
	gen := NewDataGenerator(handle, config)

	// Run for 80ms real time = 8s simulation time (within baseline 0-10s)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	go gen.Run(ctx)
	<-ctx.Done()
	time.Sleep(10 * time.Millisecond)

	metrics := handle.getMetrics()
	require.NotEmpty(t, metrics)

	for _, m := range metrics {
		switch m.name {
		case "network.retransmits":
			assert.Equal(t, 5.0, m.value, "baseline retransmits should be 5")
		case "ebpf.lock_contention_ns":
			assert.Equal(t, 1000.0, m.value, "baseline lock_contention should be 1000")
		}
	}
}

func TestGenerator_IncidentPhaseElevatedValues(t *testing.T) {
	handle := &mockHandle{}
	config := GeneratorConfig{
		TimeScale:     0.01, // 100x faster
		BaselineNoise: 0.0,  // No noise for predictable testing
	}
	gen := NewDataGenerator(handle, config)

	// Run for 200ms real time = 20s simulation time (should be in incident peak 15-25s)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go gen.Run(ctx)
	<-ctx.Done()
	time.Sleep(10 * time.Millisecond)

	metrics := handle.getMetrics()
	require.NotEmpty(t, metrics)

	// Look at recent metrics (should be from incident peak phase)
	var foundElevatedRetransmits, foundElevatedLockContention bool
	for _, m := range metrics[len(metrics)/2:] { // Check latter half
		switch m.name {
		case "network.retransmits":
			if m.value >= 45 { // Peak is 50, but we might catch some ramp
				foundElevatedRetransmits = true
			}
		case "ebpf.lock_contention_ns":
			if m.value >= 9000 { // Peak is 10000, but we might catch some ramp
				foundElevatedLockContention = true
			}
		}
	}
	assert.True(t, foundElevatedRetransmits, "expected elevated retransmits during incident")
	assert.True(t, foundElevatedLockContention, "expected elevated lock_contention during incident")
}

func TestGenerator_LogsContainConnectionErrorPatterns(t *testing.T) {
	handle := &mockHandle{}
	config := GeneratorConfig{
		TimeScale:     0.01, // 100x faster
		BaselineNoise: 0.1,
	}
	gen := NewDataGenerator(handle, config)

	// Run for 100ms to generate some logs
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go gen.Run(ctx)
	<-ctx.Done()
	time.Sleep(10 * time.Millisecond)

	logs := handle.getLogs()
	require.NotEmpty(t, logs, "expected some logs to be generated")

	// Verify logs contain connection error pattern
	for _, log := range logs {
		assert.True(t, strings.Contains(strings.ToLower(log.content), "connection refused"),
			"log should contain 'connection refused' for ConnectionErrorExtractor")
	}
}

func TestGenerator_ContextCancellationStopsGenerator(t *testing.T) {
	handle := &mockHandle{}
	config := GeneratorConfig{
		TimeScale:     0.01,
		BaselineNoise: 0.1,
	}
	gen := NewDataGenerator(handle, config)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		gen.Run(ctx)
		close(done)
	}()

	// Let it run briefly
	time.Sleep(30 * time.Millisecond)

	// Cancel and verify it stops
	cancel()

	select {
	case <-done:
		// Generator stopped as expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("generator did not stop after context cancellation")
	}
}

func TestGenerator_DefaultConfig(t *testing.T) {
	handle := &mockHandle{}

	// Test that zero TimeScale gets default; zero BaselineNoise is allowed
	gen := NewDataGenerator(handle, GeneratorConfig{})

	assert.Equal(t, 1.0, gen.config.TimeScale)
	assert.Equal(t, 0.0, gen.config.BaselineNoise) // Zero is allowed, only negative triggers default

	// Test that negative values get defaults
	gen = NewDataGenerator(handle, GeneratorConfig{TimeScale: -1, BaselineNoise: -1})
	assert.Equal(t, 1.0, gen.config.TimeScale)
	assert.Equal(t, 0.1, gen.config.BaselineNoise)
}

func TestGenerator_NoiseApplication(t *testing.T) {
	handle := &mockHandle{}
	config := GeneratorConfig{
		TimeScale:     0.01,
		BaselineNoise: 0.5, // 50% noise for visible variation
	}
	gen := NewDataGenerator(handle, config)

	// Run for baseline phase to collect multiple samples
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	go gen.Run(ctx)
	<-ctx.Done()
	time.Sleep(10 * time.Millisecond)

	metrics := handle.getMetrics()

	// Collect retransmit values
	var retransmitValues []float64
	for _, m := range metrics {
		if m.name == "network.retransmits" {
			retransmitValues = append(retransmitValues, m.value)
		}
	}

	require.GreaterOrEqual(t, len(retransmitValues), 2, "need at least 2 values to check variation")

	// With 50% noise around baseline of 5, values should be in range [2.5, 7.5]
	for _, v := range retransmitValues {
		assert.GreaterOrEqual(t, v, 2.5, "value should be within noise range")
		assert.LessOrEqual(t, v, 7.5, "value should be within noise range")
	}
}

func TestGenerator_PhaseTransitions(t *testing.T) {
	// Test getPhaseValue directly for precise control
	gen := &DataGenerator{
		config: GeneratorConfig{
			TimeScale:     1.0,
			BaselineNoise: 0.0,
		},
	}

	tests := []struct {
		elapsed  float64
		baseline float64
		peak     float64
		expected float64
		phase    string
	}{
		{0, 5, 50, 5, "baseline start"},
		{9, 5, 50, 5, "baseline end"},
		{10, 5, 50, 5, "ramp start"},
		{12.5, 5, 50, 27.5, "ramp middle"},
		{15, 5, 50, 50, "ramp end"},
		{20, 5, 50, 50, "peak"},
		{25, 5, 50, 50, "recovery start"},
		{27.5, 5, 50, 27.5, "recovery middle"},
		{30, 5, 50, 5, "recovery end"},
		{35, 5, 50, 5, "post-incident"},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			result := gen.getPhaseValue(tt.elapsed, tt.baseline, tt.peak)
			assert.Equal(t, tt.expected, result, "unexpected value for phase: %s", tt.phase)
		})
	}
}

func TestGenerator_LogIntervalByPhase(t *testing.T) {
	gen := &DataGenerator{}

	tests := []struct {
		elapsed  float64
		expected time.Duration
		phase    string
	}{
		{5, 5 * time.Second, "baseline"},
		{12, 2 * time.Second, "ramp"},
		{20, 500 * time.Millisecond, "peak"},
		{27, 2 * time.Second, "recovery"},
		{35, 5 * time.Second, "post-incident"},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			result := gen.getLogInterval(tt.elapsed)
			assert.Equal(t, tt.expected, result)
		})
	}
}
