// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package anomaly

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testObserver is a simple observer for testing
type testObserver struct {
	anomalies []Anomaly
	mu        sync.Mutex
}

func (o *testObserver) OnAnomaly(anomaly Anomaly) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.anomalies = append(o.anomalies, anomaly)
}

func (o *testObserver) getAnomalies() []Anomaly {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]Anomaly{}, o.anomalies...)
}

func (o *testObserver) count() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.anomalies)
}

func TestNewHeuristicDetector(t *testing.T) {
	config := DefaultConfig()
	observer := &testObserver{}

	detector := NewHeuristicDetector(config)
	detector.Subscribe(observer)

	require.NotNil(t, detector)
	assert.Equal(t, config, detector.GetConfig())
	assert.Equal(t, 0, observer.count())
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, uint64(100), config.WindowSize)
	assert.Equal(t, 3.0, config.SpikeThreshold)
	assert.Equal(t, uint64(20), config.MinSamples)
	assert.Equal(t, 5*time.Minute, config.CooldownDuration)
}

func TestRecordMetric_NoAnomalyBeforeMinSamples(t *testing.T) {
	observer := &testObserver{}
	detector := NewHeuristicDetector(
		DetectionConfig{
			WindowSize:     10,
			MinSamples:     5,
			SpikeThreshold: 2.0,
		},
	)
	detector.Subscribe(observer)

	// Record fewer than MinSamples
	for i := 0; i < 4; i++ {
		detector.RecordMetric("test.metric", 50.0, float64(time.Now().Unix()))
	}

	assert.Equal(t, 0, observer.count(), "Should not detect anomalies before MinSamples reached")
}

func TestSpikeDetection(t *testing.T) {
	observer := &testObserver{}
	detector := NewHeuristicDetector(
		DetectionConfig{
			WindowSize:     20,
			MinSamples:     10,
			SpikeThreshold: 2.0, // 200% of baseline
		},
	)
	detector.Subscribe(observer)

	timestamp := float64(time.Now().Unix())

	// Establish baseline around 50
	for i := 0; i < 10; i++ {
		detector.RecordMetric("cpu.usage", 50.0, timestamp+float64(i))
	}

	// Send a spike (150 is 3x the baseline of 50, exceeds 2x threshold)
	detector.RecordMetric("cpu.usage", 150.0, timestamp+10.0)

	// Find spike anomaly
	detectedAnomalies := observer.getAnomalies()
	var spikeAnomaly *Anomaly
	for i := range detectedAnomalies {
		if detectedAnomalies[i].Type == AnomalyTypeSpike {
			spikeAnomaly = &detectedAnomalies[i]
			break
		}
	}

	require.NotNil(t, spikeAnomaly, "Should detect spike anomaly")
	assert.Equal(t, "cpu.usage", spikeAnomaly.MetricName)
	assert.Equal(t, 150.0, spikeAnomaly.Value)
	assert.InDelta(t, 50.0, spikeAnomaly.Baseline, 15.0)
	assert.Greater(t, spikeAnomaly.Severity, 0.0)
	assert.LessOrEqual(t, spikeAnomaly.Severity, 1.0)
}

func TestNoFalsePositiveSpike(t *testing.T) {
	observer := &testObserver{}
	detector := NewHeuristicDetector(
		DetectionConfig{
			WindowSize:     20,
			MinSamples:     10,
			SpikeThreshold: 2.0,
		},
	)
	detector.Subscribe(observer)

	timestamp := float64(time.Now().Unix())

	// Send values that vary but don't spike
	for i := 0; i < 20; i++ {
		value := 50.0 + float64(i%5) // 50, 51, 52, 53, 54, 50, ...
		detector.RecordMetric("cpu.usage", value, timestamp+float64(i))
	}

	// Should not detect any spikes
	assert.Equal(t, 0, observer.count(), "Should not detect false positive spikes")
}

func TestMultipleMetrics(t *testing.T) {
	observer := &testObserver{}
	detector := NewHeuristicDetector(
		DetectionConfig{
			WindowSize:     20,
			MinSamples:     10,
			SpikeThreshold: 2.0,
		},
	)
	detector.Subscribe(observer)

	timestamp := float64(time.Now().Unix())

	// Send normal values for multiple metrics
	for i := 0; i < 15; i++ {
		detector.RecordMetric("cpu.usage", 50.0, timestamp+float64(i))
		detector.RecordMetric("memory.usage", 60.0, timestamp+float64(i))
		detector.RecordMetric("disk.usage", 70.0, timestamp+float64(i))
	}

	// Trigger spike only on cpu.usage
	detector.RecordMetric("cpu.usage", 150.0, timestamp+15.0)

	detectedMetrics := make(map[string]int)
	for _, a := range observer.getAnomalies() {
		detectedMetrics[a.MetricName]++
	}

	assert.Greater(t, detectedMetrics["cpu.usage"], 0, "Should detect spike on cpu.usage")
	assert.Equal(t, 0, detectedMetrics["memory.usage"], "Should not detect spike on memory.usage")
	assert.Equal(t, 0, detectedMetrics["disk.usage"], "Should not detect spike on disk.usage")
}

func TestGetMetricHistory(t *testing.T) {
	detector := NewHeuristicDetector(DefaultConfig())

	timestamp := float64(time.Now().Unix())

	// Record some metrics
	for i := 0; i < 5; i++ {
		detector.RecordMetric("test.metric", float64(10+i), timestamp+float64(i))
	}

	history := detector.GetMetricHistory("test.metric")
	require.NotNil(t, history)

	// Check that we have samples (the ring buffer returns capacity-sized slices)
	assert.Len(t, history, int(DefaultConfig().WindowSize))

	// Verify at least some samples have values
	nonZeroCount := 0
	for _, sample := range history {
		if sample.Timestamp > 0 {
			nonZeroCount++
		}
	}
	assert.GreaterOrEqual(t, nonZeroCount, 5)
}

func TestGetMetricHistory_NonExistent(t *testing.T) {
	detector := NewHeuristicDetector(DefaultConfig())

	history := detector.GetMetricHistory("nonexistent.metric")
	assert.Nil(t, history)
}

func TestClear(t *testing.T) {
	detector := NewHeuristicDetector(DefaultConfig())

	timestamp := float64(time.Now().Unix())

	// Record metrics
	detector.RecordMetric("test.metric", 50.0, timestamp)
	detector.RecordMetric("another.metric", 100.0, timestamp)

	// Verify history exists
	history1 := detector.GetMetricHistory("test.metric")
	require.NotNil(t, history1)

	// Clear all
	detector.Clear()

	// History should still exist but be cleared
	history2 := detector.GetMetricHistory("test.metric")
	require.NotNil(t, history2)

	// All samples should have zero timestamps after clear
	for _, sample := range history2 {
		assert.Equal(t, 0.0, sample.Timestamp)
	}
}

func TestZeroTimestamp_UsesCurrentTime(t *testing.T) {
	observer := &testObserver{}
	detector := NewHeuristicDetector(
		DetectionConfig{
			WindowSize:     20,
			MinSamples:     10,
			SpikeThreshold: 2.0,
		},
	)
	detector.Subscribe(observer)

	beforeTime := float64(time.Now().Unix())

	// Record with zero timestamp (should use current time)
	for i := 0; i < 10; i++ {
		detector.RecordMetric("test.metric", 50.0, 0)
		time.Sleep(1 * time.Millisecond)
	}
	detector.RecordMetric("test.metric", 150.0, 0)

	afterTime := float64(time.Now().Unix())

	// Should have detected anomaly
	assert.Greater(t, observer.count(), 0)

	// Timestamp should be between before and after
	for _, a := range observer.getAnomalies() {
		assert.GreaterOrEqual(t, a.Timestamp, beforeTime)
		assert.LessOrEqual(t, a.Timestamp, afterTime+1) // +1 for tolerance
	}
}

func TestConcurrentRecordMetric(t *testing.T) {
	observer := &testObserver{}
	detector := NewHeuristicDetector(
		DetectionConfig{
			WindowSize:     100,
			MinSamples:     20,
			SpikeThreshold: 2.0,
		},
	)
	detector.Subscribe(observer)

	var wg sync.WaitGroup
	timestamp := float64(time.Now().Unix())

	// Concurrently record metrics
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(val float64) {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				detector.RecordMetric("concurrent.metric", val, timestamp+float64(j))
			}
		}(float64(50 + i))
	}

	wg.Wait()

	// Should not panic and should have processed all metrics
	history := detector.GetMetricHistory("concurrent.metric")
	require.NotNil(t, history)
}

func TestSeverityCalculation(t *testing.T) {
	tests := []struct {
		name            string
		baseline        float64
		value           float64
		severityInRange bool
	}{
		{
			name:            "Spike - 2x baseline",
			baseline:        50.0,
			value:           100.0,
			severityInRange: true, // Should be 0.0-1.0
		},
		{
			name:            "Spike - 5x baseline",
			baseline:        50.0,
			value:           250.0,
			severityInRange: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observer := &testObserver{}
			detector := NewHeuristicDetector(
				DetectionConfig{
					WindowSize:     20,
					MinSamples:     10,
					SpikeThreshold: 1.5,
				},
			)
			detector.Subscribe(observer)

			timestamp := float64(time.Now().Unix())

			// Establish baseline
			for i := 0; i < 15; i++ {
				detector.RecordMetric("test.metric", tt.baseline, timestamp+float64(i))
			}

			// Send anomalous value
			detector.RecordMetric("test.metric", tt.value, timestamp+15.0)

			if tt.severityInRange {
				anomalies := observer.getAnomalies()
				require.Greater(t, len(anomalies), 0)
				detectedAnomaly := anomalies[0]
				assert.GreaterOrEqual(t, detectedAnomaly.Severity, 0.0)
				assert.LessOrEqual(t, detectedAnomaly.Severity, 1.0)
			}
		})
	}
}

func TestCooldownPreventsDoubleAlert(t *testing.T) {
	observer := &testObserver{}
	detector := NewHeuristicDetector(
		DetectionConfig{
			WindowSize:       20,
			MinSamples:       10,
			SpikeThreshold:   2.0,
			CooldownDuration: 1 * time.Minute, // 1 minute cooldown
		},
	)
	detector.Subscribe(observer)

	timestamp := float64(time.Now().Unix())

	// Establish baseline around 50
	for i := 0; i < 10; i++ {
		detector.RecordMetric("cpu.usage", 50.0, timestamp+float64(i))
	}

	// Send first spike (150 is 3x the baseline of 50)
	detector.RecordMetric("cpu.usage", 150.0, timestamp+10.0)

	// Should have detected first anomaly
	assert.Equal(t, 1, observer.count(), "Should detect first spike")

	// Send same spike value immediately after (within cooldown)
	detector.RecordMetric("cpu.usage", 150.0, timestamp+11.0)

	// Should NOT detect second anomaly (within cooldown period)
	assert.Equal(t, 1, observer.count(), "Should not detect duplicate alert within cooldown")

	// Send another spike after a short time (still within cooldown)
	detector.RecordMetric("cpu.usage", 155.0, timestamp+12.0)

	// Should still have only 1 alert
	assert.Equal(t, 1, observer.count(), "Should not alert again within cooldown period")
}

func TestCooldownAllowsAlertAfterExpiry(t *testing.T) {
	observer := &testObserver{}

	shortCooldown := 100 * time.Millisecond
	detector := NewHeuristicDetector(
		DetectionConfig{
			WindowSize:       20,
			MinSamples:       10,
			SpikeThreshold:   2.0,
			CooldownDuration: shortCooldown,
		},
	)
	detector.Subscribe(observer)

	timestamp := float64(time.Now().Unix())

	// Establish baseline around 50
	for i := 0; i < 10; i++ {
		detector.RecordMetric("cpu.usage", 50.0, timestamp+float64(i))
	}

	// Send first spike
	detector.RecordMetric("cpu.usage", 150.0, timestamp+10.0)

	initialCount := observer.count()
	assert.Equal(t, 1, initialCount, "Should detect first spike")

	// Wait for cooldown to expire
	time.Sleep(shortCooldown + 50*time.Millisecond)

	// Send another spike after cooldown expires
	detector.RecordMetric("cpu.usage", 150.0, timestamp+11.0)

	finalCount := observer.count()
	assert.Equal(t, 2, finalCount, "Should detect second spike after cooldown expires")
}

func TestCooldownPerMetric(t *testing.T) {
	observer := &testObserver{}
	detector := NewHeuristicDetector(
		DetectionConfig{
			WindowSize:       20,
			MinSamples:       10,
			SpikeThreshold:   2.0,
			CooldownDuration: 1 * time.Minute,
		},
	)
	detector.Subscribe(observer)

	timestamp := float64(time.Now().Unix())

	// Establish baselines for multiple metrics
	for i := 0; i < 15; i++ {
		detector.RecordMetric("cpu.usage", 50.0, timestamp+float64(i))
		detector.RecordMetric("memory.usage", 60.0, timestamp+float64(i))
	}

	// Trigger spike on cpu.usage
	detector.RecordMetric("cpu.usage", 150.0, timestamp+15.0)

	// Trigger spike on memory.usage
	detector.RecordMetric("memory.usage", 180.0, timestamp+15.0)

	detectedMetrics := make(map[string]int)
	for _, a := range observer.getAnomalies() {
		detectedMetrics[a.MetricName]++
	}

	// Both should alert once
	assert.Equal(t, 1, detectedMetrics["cpu.usage"], "CPU should alert once")
	assert.Equal(t, 1, detectedMetrics["memory.usage"], "Memory should alert once")

	// Send spikes again on both metrics (within cooldown)
	detector.RecordMetric("cpu.usage", 150.0, timestamp+16.0)
	detector.RecordMetric("memory.usage", 180.0, timestamp+16.0)

	detectedMetrics2 := make(map[string]int)
	for _, a := range observer.getAnomalies() {
		detectedMetrics2[a.MetricName]++
	}

	// Neither should alert again (cooldown is per-metric)
	assert.Equal(t, 1, detectedMetrics2["cpu.usage"], "CPU should not alert again within cooldown")
	assert.Equal(t, 1, detectedMetrics2["memory.usage"], "Memory should not alert again within cooldown")
}

func TestClearResetsAlertTracking(t *testing.T) {
	observer := &testObserver{}
	detector := NewHeuristicDetector(
		DetectionConfig{
			WindowSize:       20,
			MinSamples:       10,
			SpikeThreshold:   2.0,
			CooldownDuration: 5 * time.Minute, // Long cooldown
		},
	)
	detector.Subscribe(observer)

	timestamp := float64(time.Now().Unix())

	// Establish baseline
	for i := 0; i < 10; i++ {
		detector.RecordMetric("cpu.usage", 50.0, timestamp+float64(i))
	}

	// Send first spike
	detector.RecordMetric("cpu.usage", 150.0, timestamp+10.0)

	assert.Equal(t, 1, observer.count(), "Should detect first spike")

	// Clear detector (should reset alert tracking)
	detector.Clear()

	// Re-establish baseline
	for i := 0; i < 10; i++ {
		detector.RecordMetric("cpu.usage", 50.0, timestamp+float64(20+i))
	}

	// Send another spike (should alert even though cooldown hasn't expired)
	detector.RecordMetric("cpu.usage", 150.0, timestamp+30.0)

	assert.Equal(t, 2, observer.count(), "Should detect spike after Clear() resets tracking")
}

// Benchmarks

func BenchmarkRecordMetric(b *testing.B) {
	detector := NewHeuristicDetector(DefaultConfig())
	timestamp := float64(time.Now().Unix())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.RecordMetric("benchmark.metric", float64(50+i%10), timestamp+float64(i))
	}
}

func BenchmarkRecordMetric_WithObserver(b *testing.B) {
	observer := &testObserver{}
	detector := NewHeuristicDetector(DefaultConfig())
	detector.Subscribe(observer)
	timestamp := float64(time.Now().Unix())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.RecordMetric("benchmark.metric", float64(50+i%10), timestamp+float64(i))
	}
}

func BenchmarkRecordMetric_MultipleMetrics(b *testing.B) {
	detector := NewHeuristicDetector(DefaultConfig())
	timestamp := float64(time.Now().Unix())

	metricNames := []string{
		"cpu.usage",
		"memory.usage",
		"disk.usage",
		"network.usage",
		"requests.count",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metricName := metricNames[i%len(metricNames)]
		detector.RecordMetric(metricName, float64(50+i%10), timestamp+float64(i))
	}
}

func BenchmarkSpikeDetection(b *testing.B) {
	timestamp := float64(time.Now().Unix())

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		observer := &testObserver{}
		detector := NewHeuristicDetector(
			DetectionConfig{
				WindowSize:     20,
				MinSamples:     10,
				SpikeThreshold: 2.0,
			},
		)
		detector.Subscribe(observer)

		// Establish baseline
		for j := 0; j < 10; j++ {
			detector.RecordMetric("test.metric", 50.0, timestamp+float64(j))
		}

		b.StartTimer()
		// Trigger spike detection
		detector.RecordMetric("test.metric", 150.0, timestamp+10.0)
	}
}

func BenchmarkGetMetricHistory(b *testing.B) {
	detector := NewHeuristicDetector(DefaultConfig())
	timestamp := float64(time.Now().Unix())

	// Populate with data
	for i := 0; i < 100; i++ {
		detector.RecordMetric("test.metric", float64(50+i), timestamp+float64(i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = detector.GetMetricHistory("test.metric")
	}
}

func BenchmarkConcurrentRecordMetric(b *testing.B) {
	detector := NewHeuristicDetector(DefaultConfig())
	timestamp := float64(time.Now().Unix())

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			detector.RecordMetric("concurrent.metric", float64(50+i%10), timestamp+float64(i))
			i++
		}
	})
}

func BenchmarkClear(b *testing.B) {
	detector := NewHeuristicDetector(DefaultConfig())
	timestamp := float64(time.Now().Unix())

	// Populate with data
	for i := 0; i < 100; i++ {
		detector.RecordMetric("test.metric", float64(50+i), timestamp+float64(i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.Clear()
	}
}
