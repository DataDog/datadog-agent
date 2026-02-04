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

func (h *mockHandle) ObserveTrace(_ observer.TraceView) {
	// No-op for test mock - traces not used in demo generator tests
}

func (h *mockHandle) ObserveProfile(_ observer.ProfileView) {
	// No-op for test mock - profiles not used in demo generator tests
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
	// In 50ms, we should get about 5 ticks, each producing 5 incident metrics + 4 background metrics
	assert.GreaterOrEqual(t, len(metrics), 4, "expected at least 2 ticks worth of metrics")

	// Verify we get all incident metrics
	var hasHeap, hasGC, hasLatency, hasErrorRate, hasCPU bool
	for _, m := range metrics {
		switch m.name {
		case "runtime.heap.used_mb":
			hasHeap = true
		case "runtime.gc.pause_ms":
			hasGC = true
		case "app.request.latency_p99_ms":
			hasLatency = true
		case "app.request.error_rate":
			hasErrorRate = true
		case "system.cpu.user_percent":
			hasCPU = true
		}
	}
	assert.True(t, hasHeap, "expected runtime.heap.used_mb metric")
	assert.True(t, hasGC, "expected runtime.gc.pause_ms metric")
	assert.True(t, hasLatency, "expected app.request.latency_p99_ms metric")
	assert.True(t, hasErrorRate, "expected app.request.error_rate metric")
	assert.True(t, hasCPU, "expected system.cpu.user_percent metric")
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
		case "runtime.heap.used_mb":
			assert.Equal(t, 512.0, m.value, "baseline heap should be 512 MB")
		case "runtime.gc.pause_ms":
			assert.Equal(t, 15.0, m.value, "baseline GC pause should be 15 ms")
		case "app.request.latency_p99_ms":
			assert.Equal(t, 45.0, m.value, "baseline latency should be 45 ms")
		case "app.request.error_rate":
			assert.Equal(t, 0.1, m.value, "baseline error rate should be 0.1%")
		case "system.cpu.user_percent":
			assert.Equal(t, 35.0, m.value, "baseline CPU should be 35%")
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

	// Run for 400ms real time = 40s simulation time (should be in incident peak 30-45s)
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	go gen.Run(ctx)
	<-ctx.Done()
	time.Sleep(10 * time.Millisecond)

	metrics := handle.getMetrics()
	require.NotEmpty(t, metrics)

	// Look at recent metrics (should be from incident peak phase)
	var foundElevatedHeap, foundElevatedGC, foundElevatedLatency bool
	for _, m := range metrics[len(metrics)/2:] { // Check latter half
		switch m.name {
		case "runtime.heap.used_mb":
			if m.value >= 850 { // Peak is 900
				foundElevatedHeap = true
			}
		case "runtime.gc.pause_ms":
			if m.value >= 130 { // Peak is 150
				foundElevatedGC = true
			}
		case "app.request.latency_p99_ms":
			if m.value >= 450 { // Peak is 500
				foundElevatedLatency = true
			}
		}
	}
	assert.True(t, foundElevatedHeap, "expected elevated heap during incident")
	assert.True(t, foundElevatedGC, "expected elevated GC pause during incident")
	assert.True(t, foundElevatedLatency, "expected elevated latency during incident")
}

func TestGenerator_LogsContainErrorPatterns(t *testing.T) {
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

	// Verify logs contain one of the valid error messages
	for _, log := range logs {
		found := false
		for _, validMsg := range errorLogMessages {
			if strings.Contains(log.content, validMsg) {
				found = true
				break
			}
		}
		assert.True(t, found, "log should contain one of the known error messages, got: %s", log.content)
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

	// Collect GC pause values (baseline 15ms with 50% noise = [7.5, 22.5])
	var gcValues []float64
	for _, m := range metrics {
		if m.name == "runtime.gc.pause_ms" {
			gcValues = append(gcValues, m.value)
		}
	}

	require.GreaterOrEqual(t, len(gcValues), 2, "need at least 2 values to check variation")

	// With 50% noise around baseline of 15, values should be in range [7.5, 22.5]
	for _, v := range gcValues {
		assert.GreaterOrEqual(t, v, 7.5, "value should be within noise range")
		assert.LessOrEqual(t, v, 22.5, "value should be within noise range")
	}
}

func TestGenerator_PhaseTransitionsWithDelay(t *testing.T) {
	// Test getPhaseValueWithDelay directly for precise control
	gen := &DataGenerator{
		config: GeneratorConfig{
			TimeScale:     1.0,
			BaselineNoise: 0.0,
		},
	}

	// Test with no delay: baseline 0-25s, ramp 25-30s, peak 30-45s, recovery 45-50s
	tests := []struct {
		elapsed  float64
		baseline float64
		peak     float64
		delay    float64
		expected float64
		phase    string
	}{
		// GC (no delay)
		{0, 15, 150, 0, 15, "gc baseline start"},
		{24, 15, 150, 0, 15, "gc baseline end"},
		{25, 15, 150, 0, 15, "gc ramp start"},
		{27.5, 15, 150, 0, 82.5, "gc ramp middle"},
		{30, 15, 150, 0, 150, "gc ramp end"},
		{37, 15, 150, 0, 150, "gc peak"},
		{45, 15, 150, 0, 150, "gc recovery start"},
		{47.5, 15, 150, 0, 82.5, "gc recovery middle"},
		{50, 15, 150, 0, 15, "gc recovery end"},
		{60, 15, 150, 0, 15, "gc post-incident"},

		// Latency (1s delay)
		{25, 45, 500, 1, 45, "latency still baseline at t=25 due to 1s delay"},
		{26, 45, 500, 1, 45, "latency ramp start at t=26"},
		{28.5, 45, 500, 1, 272.5, "latency ramp middle"},
		{31, 45, 500, 1, 500, "latency reaches peak at t=31"},

		// Error rate (2s delay)
		{26, 0.1, 8.0, 2, 0.1, "errors still baseline at t=26 due to 2s delay"},
		{27, 0.1, 8.0, 2, 0.1, "errors ramp start at t=27"},
		{29.5, 0.1, 8.0, 2, 4.05, "errors ramp middle"},
		{32, 0.1, 8.0, 2, 8.0, "errors reaches peak at t=32"},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			result := gen.getPhaseValueWithDelay(tt.elapsed, tt.baseline, tt.peak, tt.delay)
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
		{27, 2 * time.Second, "ramp"},
		{37, 500 * time.Millisecond, "peak"},
		{47, 2 * time.Second, "recovery"},
		{55, 5 * time.Second, "post-incident"},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			result := gen.getLogInterval(tt.elapsed)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerator_NonCorrelatedSpikes(t *testing.T) {
	handle := &mockHandle{}
	gen := NewDataGenerator(handle, GeneratorConfig{
		TimeScale:     1.0,
		BaselineNoise: 0.0,
	})

	// Test GC spike at 10-12s (triangle wave peaking at 11s)
	assert.Equal(t, 15.0, gen.getGCPauseValue(9), "before spike should be baseline")
	assert.Greater(t, gen.getGCPauseValue(11), 50.0, "spike midpoint should be elevated")
	assert.Equal(t, 15.0, gen.getGCPauseValue(13), "after spike should return to baseline")

	// During GC spike, other metrics should stay at baseline
	assert.Equal(t, 45.0, gen.getLatencyValue(11), "latency should be baseline during GC spike")
	assert.Equal(t, 0.1, gen.getErrorRateValue(11), "error rate should be baseline during GC spike")

	// Test latency spike at 17-19s (triangle wave peaking at 18s)
	assert.Equal(t, 45.0, gen.getLatencyValue(16), "before spike should be baseline")
	assert.Greater(t, gen.getLatencyValue(18), 100.0, "spike midpoint should be elevated")
	assert.Equal(t, 45.0, gen.getLatencyValue(20), "after spike should return to baseline")

	// During latency spike, other metrics should stay at baseline
	assert.Equal(t, 15.0, gen.getGCPauseValue(18), "GC should be baseline during latency spike")
	assert.Equal(t, 0.1, gen.getErrorRateValue(18), "error rate should be baseline during latency spike")
}

func TestGenerator_BackgroundMetrics(t *testing.T) {
	handle := &mockHandle{}
	config := GeneratorConfig{
		TimeScale:     0.01, // 100x faster
		BaselineNoise: 0.0,  // No noise for predictable testing
	}
	gen := NewDataGenerator(handle, config)

	// Run for 50ms to collect some metrics
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	go gen.Run(ctx)
	<-ctx.Done()
	time.Sleep(10 * time.Millisecond)

	metrics := handle.getMetrics()
	require.NotEmpty(t, metrics)

	// Verify we get all expected background metrics
	expectedMetrics := map[string]float64{
		"system.disk.read_ops":  150,
		"system.disk.write_ops": 80,
		"system.net.bytes_recv": 50000,
		"system.net.bytes_sent": 45000,
	}

	foundMetrics := make(map[string]bool)
	for _, m := range metrics {
		if expected, ok := expectedMetrics[m.name]; ok {
			foundMetrics[m.name] = true
			assert.Equal(t, expected, m.value, "background metric %s should be at baseline", m.name)
		}
	}

	for name := range expectedMetrics {
		assert.True(t, foundMetrics[name], "expected to find background metric: %s", name)
	}
}

func TestGenerator_TriangleSpike(t *testing.T) {
	handle := &mockHandle{}
	gen := NewDataGenerator(handle, GeneratorConfig{})

	// Test triangle spike helper directly
	// Spike from 10 to 12, baseline 5, peak 30
	assert.Equal(t, 5.0, gen.triangleSpike(10, 10, 12, 5, 30), "start should be baseline")
	assert.Equal(t, 30.0, gen.triangleSpike(11, 10, 12, 5, 30), "midpoint should be peak")
	assert.Equal(t, 5.0, gen.triangleSpike(12, 10, 12, 5, 30), "end should be baseline")
	assert.Equal(t, 17.5, gen.triangleSpike(10.5, 10, 12, 5, 30), "quarter way should be midpoint value")
}

func TestGenerator_CausalChainDelays(t *testing.T) {
	handle := &mockHandle{}
	gen := NewDataGenerator(handle, GeneratorConfig{
		TimeScale:     1.0,
		BaselineNoise: 0.0,
	})

	// At t=25.5s: GC should be ramping, latency and errors still at baseline
	gcAt25_5 := gen.getGCPauseValue(25.5)
	latencyAt25_5 := gen.getLatencyValue(25.5)
	errorsAt25_5 := gen.getErrorRateValue(25.5)

	assert.Greater(t, gcAt25_5, 15.0, "GC should be ramping at t=25.5")
	assert.Equal(t, 45.0, latencyAt25_5, "latency should still be at baseline at t=25.5 (delay=1s)")
	assert.Equal(t, 0.1, errorsAt25_5, "errors should still be at baseline at t=25.5 (delay=2s)")

	// At t=26.5s: GC ramping further, latency starting to ramp, errors still baseline
	gcAt26_5 := gen.getGCPauseValue(26.5)
	latencyAt26_5 := gen.getLatencyValue(26.5)
	errorsAt26_5 := gen.getErrorRateValue(26.5)

	assert.Greater(t, gcAt26_5, gcAt25_5, "GC should be ramping further at t=26.5")
	assert.Greater(t, latencyAt26_5, 45.0, "latency should be ramping at t=26.5")
	assert.Equal(t, 0.1, errorsAt26_5, "errors should still be at baseline at t=26.5 (delay=2s)")

	// At t=27.5s: all metrics should be ramping
	gcAt27_5 := gen.getGCPauseValue(27.5)
	latencyAt27_5 := gen.getLatencyValue(27.5)
	errorsAt27_5 := gen.getErrorRateValue(27.5)

	assert.Greater(t, gcAt27_5, gcAt26_5, "GC should continue ramping at t=27.5")
	assert.Greater(t, latencyAt27_5, latencyAt26_5, "latency should continue ramping at t=27.5")
	assert.Greater(t, errorsAt27_5, 0.1, "errors should start ramping at t=27.5")
}

func TestGenerator_HeapLeadingIndicator(t *testing.T) {
	handle := &mockHandle{}
	gen := NewDataGenerator(handle, GeneratorConfig{
		TimeScale:     1.0,
		BaselineNoise: 0.0,
	})

	// Heap should start rising at t=22s (heapLeadTime=-3s before incident at t=25s)
	assert.Equal(t, 512.0, gen.getHeapUsedValue(21), "heap should be at baseline before t=22")
	assert.Greater(t, gen.getHeapUsedValue(23), 512.0, "heap should be rising at t=23")
	assert.Equal(t, 15.0, gen.getGCPauseValue(23), "GC should still be at baseline at t=23")

	// By t=25s, heap should be ramping while GC just starts
	heapAt25 := gen.getHeapUsedValue(25)
	gcAt25 := gen.getGCPauseValue(25)
	assert.Greater(t, heapAt25, 700.0, "heap should be elevated at t=25")
	assert.Equal(t, 15.0, gcAt25, "GC should just be starting at t=25")
}
