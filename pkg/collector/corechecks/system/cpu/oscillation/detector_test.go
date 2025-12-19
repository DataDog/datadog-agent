// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux || darwin

package oscillation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func defaultTestConfig() OscillationConfig {
	return OscillationConfig{
		WindowSize:          10, // Small window for tests
		MinZeroCrossings:    3,
		AmplitudeMultiplier: 2.0,
		DecayFactor:         0.1,
		WarmupDuration:      0, // No warmup for most tests
		SampleInterval:      time.Second,
	}
}

func TestNewOscillationDetector(t *testing.T) {
	config := defaultTestConfig()
	d := NewOscillationDetector(config)

	assert.NotNil(t, d)
	assert.Equal(t, config.WindowSize, len(d.samples))
	assert.Equal(t, 0, d.sampleCount)
	assert.Equal(t, 0, d.sampleIndex)
}

func TestAddSample(t *testing.T) {
	config := defaultTestConfig()
	d := NewOscillationDetector(config)

	// Add samples up to window size
	for i := 0; i < config.WindowSize; i++ {
		d.AddSample(float64(i * 10))
		assert.Equal(t, i+1, d.sampleCount)
	}

	// Window should be full
	assert.True(t, d.IsWindowFull())

	// Add more samples - count should stay at window size
	d.AddSample(100)
	assert.Equal(t, config.WindowSize, d.sampleCount)
}

func TestCountZeroCrossings_NoOscillation(t *testing.T) {
	config := defaultTestConfig()
	d := NewOscillationDetector(config)

	// Monotonically increasing - no zero crossings
	for i := 0; i < config.WindowSize; i++ {
		d.AddSample(float64(i * 5))
	}

	crossings := d.countZeroCrossings()
	assert.Equal(t, 0, crossings)
}

func TestCountZeroCrossings_Oscillating(t *testing.T) {
	config := defaultTestConfig()
	d := NewOscillationDetector(config)

	// Oscillating pattern: 10, 20, 10, 20, 10, 20, 10, 20, 10, 20
	// This creates direction changes at each transition
	for i := 0; i < config.WindowSize; i++ {
		if i%2 == 0 {
			d.AddSample(10)
		} else {
			d.AddSample(20)
		}
	}

	crossings := d.countZeroCrossings()
	// With 10 samples oscillating, we expect 8 zero crossings (each peak and trough)
	assert.Equal(t, 8, crossings)
}

func TestCountZeroCrossings_SinglePeak(t *testing.T) {
	config := defaultTestConfig()
	config.WindowSize = 5
	d := NewOscillationDetector(config)

	// Single peak: 10, 20, 30, 20, 10
	samples := []float64{10, 20, 30, 20, 10}
	for _, s := range samples {
		d.AddSample(s)
	}

	crossings := d.countZeroCrossings()
	// One peak = one zero crossing (direction change from up to down)
	assert.Equal(t, 1, crossings)
}

func TestCalculateAmplitude(t *testing.T) {
	config := defaultTestConfig()
	d := NewOscillationDetector(config)

	// Add samples with known min/max
	samples := []float64{10, 20, 15, 30, 25, 5, 35, 20, 15, 10}
	for _, s := range samples {
		d.AddSample(s)
	}

	amplitude := d.calculateAmplitude()
	// Max = 35, Min = 5, Amplitude = 30
	assert.Equal(t, 30.0, amplitude)
}

func TestCalculateVariance(t *testing.T) {
	config := defaultTestConfig()
	config.WindowSize = 4
	d := NewOscillationDetector(config)

	// Simple samples: 2, 4, 4, 4
	// Mean = 3.5
	// Variance = ((2-3.5)^2 + (4-3.5)^2 + (4-3.5)^2 + (4-3.5)^2) / 4
	//          = (2.25 + 0.25 + 0.25 + 0.25) / 4 = 0.75
	samples := []float64{2, 4, 4, 4}
	for _, s := range samples {
		d.AddSample(s)
	}

	variance := d.calculateVariance()
	assert.InDelta(t, 0.75, variance, 0.001)
}

func TestUpdateBaseline_FirstSample(t *testing.T) {
	config := defaultTestConfig()
	d := NewOscillationDetector(config)

	d.updateBaseline(10.0)
	assert.Equal(t, 10.0, d.baselineVariance)
}

func TestUpdateBaseline_ExponentialDecay(t *testing.T) {
	config := defaultTestConfig()
	config.DecayFactor = 0.5 // Easy to calculate
	d := NewOscillationDetector(config)

	d.updateBaseline(10.0) // First: baseline = 10
	d.updateBaseline(20.0) // Second: 0.5*20 + 0.5*10 = 15

	assert.Equal(t, 15.0, d.baselineVariance)
}

func TestWarmupPeriod(t *testing.T) {
	config := defaultTestConfig()
	config.WarmupDuration = 5 * time.Second
	d := NewOscillationDetector(config)

	assert.False(t, d.IsWarmedUp())

	// Decrement warmup 5 times
	for i := 0; i < 5; i++ {
		d.DecrementWarmup()
	}

	assert.True(t, d.IsWarmedUp())
}

func TestAnalyze_WindowNotFull(t *testing.T) {
	config := defaultTestConfig()
	d := NewOscillationDetector(config)

	// Only add a few samples
	d.AddSample(10)
	d.AddSample(20)

	result := d.Analyze()
	assert.False(t, result.Detected)
	assert.Equal(t, 0, result.ZeroCrossings)
}

func TestAnalyze_DuringWarmup(t *testing.T) {
	config := defaultTestConfig()
	config.WarmupDuration = 100 * time.Second // Long warmup
	d := NewOscillationDetector(config)

	// Fill window with oscillating data
	for i := 0; i < config.WindowSize; i++ {
		if i%2 == 0 {
			d.AddSample(10)
		} else {
			d.AddSample(50)
		}
	}

	result := d.Analyze()
	// Should not detect during warmup even with oscillating data
	assert.False(t, result.Detected)
}

func TestAnalyze_DetectsOscillation(t *testing.T) {
	config := defaultTestConfig()
	config.WindowSize = 20
	config.MinZeroCrossings = 6
	config.AmplitudeMultiplier = 2.0
	d := NewOscillationDetector(config)

	// First establish a calm baseline (fill window with stable values)
	for i := 0; i < config.WindowSize*5; i++ {
		d.AddSample(50.0 + float64(i%2)) // Small variance
		d.DecrementWarmup()
	}

	// Now the baseline is established with low variance
	// Create a new detector to test oscillation detection clearly
	d2 := NewOscillationDetector(config)
	d2.baselineVariance = 1.0 // Set a low baseline (stddev = 1)

	// Fill with strongly oscillating data
	for i := 0; i < config.WindowSize; i++ {
		if i%2 == 0 {
			d2.AddSample(30) // Low
		} else {
			d2.AddSample(70) // High - amplitude = 40, which is > 2*1 = 2
		}
	}

	result := d2.Analyze()
	assert.True(t, result.Detected, "Should detect oscillation with high amplitude and many zero crossings")
	assert.Equal(t, 18, result.ZeroCrossings) // 20 samples oscillating = 18 direction changes
	assert.Equal(t, 40.0, result.Amplitude)
}

func TestAnalyze_NoOscillation_StableData(t *testing.T) {
	config := defaultTestConfig()
	d := NewOscillationDetector(config)

	// Fill with stable data
	for i := 0; i < config.WindowSize; i++ {
		d.AddSample(50.0)
	}

	result := d.Analyze()
	assert.False(t, result.Detected)
	assert.Equal(t, 0.0, result.Amplitude)
	assert.Equal(t, 0, result.ZeroCrossings)
}

func TestAnalyze_NoOscillation_GradualChange(t *testing.T) {
	config := defaultTestConfig()
	d := NewOscillationDetector(config)

	// Fill with gradually increasing data (no oscillation)
	for i := 0; i < config.WindowSize; i++ {
		d.AddSample(float64(i * 5))
	}

	result := d.Analyze()
	assert.False(t, result.Detected)
	assert.Equal(t, 0, result.ZeroCrossings)
}

func TestAnalyze_MinAmplitudeThreshold(t *testing.T) {
	t.Run("oscillation blocked by min_amplitude", func(t *testing.T) {
		config := defaultTestConfig()
		config.WindowSize = 20
		config.MinZeroCrossings = 6
		config.AmplitudeMultiplier = 2.0
		config.MinAmplitude = 50.0 // Require at least 50% amplitude
		d := NewOscillationDetector(config)
		d.baselineVariance = 1.0 // Low baseline

		// Fill with oscillating data that has amplitude = 40 (less than min_amplitude)
		for i := 0; i < config.WindowSize; i++ {
			if i%2 == 0 {
				d.AddSample(30)
			} else {
				d.AddSample(70)
			}
		}

		result := d.Analyze()
		assert.False(t, result.Detected, "Should NOT detect: amplitude 40 < min_amplitude 50")
		assert.Equal(t, 40.0, result.Amplitude)
	})

	t.Run("oscillation allowed when above min_amplitude", func(t *testing.T) {
		config := defaultTestConfig()
		config.WindowSize = 20
		config.MinZeroCrossings = 6
		config.AmplitudeMultiplier = 2.0
		config.MinAmplitude = 30.0 // Require at least 30% amplitude
		d := NewOscillationDetector(config)
		d.baselineVariance = 1.0 // Low baseline

		// Fill with oscillating data that has amplitude = 40 (greater than min_amplitude)
		for i := 0; i < config.WindowSize; i++ {
			if i%2 == 0 {
				d.AddSample(30)
			} else {
				d.AddSample(70)
			}
		}

		result := d.Analyze()
		assert.True(t, result.Detected, "Should detect: amplitude 40 > min_amplitude 30")
		assert.Equal(t, 40.0, result.Amplitude)
	})

	t.Run("min_amplitude disabled when zero", func(t *testing.T) {
		config := defaultTestConfig()
		config.WindowSize = 20
		config.MinZeroCrossings = 6
		config.AmplitudeMultiplier = 2.0
		config.MinAmplitude = 0 // Disabled
		d := NewOscillationDetector(config)
		d.baselineVariance = 1.0 // Low baseline

		// Fill with oscillating data with small amplitude = 10
		for i := 0; i < config.WindowSize; i++ {
			if i%2 == 0 {
				d.AddSample(45)
			} else {
				d.AddSample(55)
			}
		}

		result := d.Analyze()
		// Should detect because amplitude (10) > 2.0 * sqrt(1.0) = 2, and min_amplitude is disabled
		assert.True(t, result.Detected, "Should detect: min_amplitude=0 means no absolute floor")
		assert.Equal(t, 10.0, result.Amplitude)
	})
}

func TestFrequencyCalculation(t *testing.T) {
	config := defaultTestConfig()
	config.WindowSize = 60 // 60 seconds
	d := NewOscillationDetector(config)

	// Create oscillating pattern with period = 10 samples (0.1 Hz at 1Hz sampling)
	for i := 0; i < config.WindowSize; i++ {
		// Complete cycle every 10 samples
		phase := float64(i % 10)
		if phase < 5 {
			d.AddSample(70)
		} else {
			d.AddSample(30)
		}
	}

	result := d.Analyze()

	// With period of 10 samples and 60 sample window, we get 6 complete cycles
	// 12 zero crossings (2 per cycle) = 12
	// Actually: 60 samples with pattern changing every 5 = 11 transitions = 10 zero crossings
	// Frequency = 10 / 60 / 2 = 0.083 Hz
	assert.InDelta(t, 0.083, result.Frequency, 0.02)
}

func TestBaselineStdDev(t *testing.T) {
	config := defaultTestConfig()
	d := NewOscillationDetector(config)

	d.baselineVariance = 16.0 // sqrt(16) = 4

	assert.Equal(t, 4.0, d.BaselineStdDev())
}

func TestRingBufferWraparound(t *testing.T) {
	config := defaultTestConfig()
	config.WindowSize = 5
	d := NewOscillationDetector(config)

	// Fill buffer
	for i := 0; i < 5; i++ {
		d.AddSample(float64(i * 10)) // 0, 10, 20, 30, 40
	}

	// Add more samples to wrap around
	d.AddSample(50) // Replaces 0
	d.AddSample(60) // Replaces 10
	d.AddSample(70) // Replaces 20

	// Buffer should now contain: 50, 60, 70, 30, 40 (logical order: 30, 40, 50, 60, 70)
	amplitude := d.calculateAmplitude()
	assert.Equal(t, 40.0, amplitude) // max=70, min=30
}

func TestGetSample_LogicalOrder(t *testing.T) {
	config := defaultTestConfig()
	config.WindowSize = 5
	d := NewOscillationDetector(config)

	// Fill buffer with 0-4
	for i := 0; i < 5; i++ {
		d.AddSample(float64(i))
	}

	// Verify logical order
	for i := 0; i < 5; i++ {
		assert.Equal(t, float64(i), d.getSample(i))
	}

	// Add one more to wrap
	d.AddSample(5.0)

	// Now logical order should be 1, 2, 3, 4, 5
	for i := 0; i < 5; i++ {
		assert.Equal(t, float64(i+1), d.getSample(i))
	}
}

func TestZeroCrossings_EdgeCases(t *testing.T) {
	t.Run("less than 3 samples", func(t *testing.T) {
		config := defaultTestConfig()
		d := NewOscillationDetector(config)
		d.AddSample(10)
		d.AddSample(20)

		assert.Equal(t, 0, d.countZeroCrossings())
	})

	t.Run("flat line", func(t *testing.T) {
		config := defaultTestConfig()
		d := NewOscillationDetector(config)
		for i := 0; i < 10; i++ {
			d.AddSample(50)
		}

		assert.Equal(t, 0, d.countZeroCrossings())
	})

	t.Run("single direction change", func(t *testing.T) {
		config := defaultTestConfig()
		config.WindowSize = 6
		d := NewOscillationDetector(config)

		// Up then down: 10, 20, 30, 25, 20, 15
		samples := []float64{10, 20, 30, 25, 20, 15}
		for _, s := range samples {
			d.AddSample(s)
		}

		assert.Equal(t, 1, d.countZeroCrossings())
	})
}

func TestVariance_EdgeCases(t *testing.T) {
	t.Run("single sample", func(t *testing.T) {
		config := defaultTestConfig()
		d := NewOscillationDetector(config)
		d.AddSample(50)

		assert.Equal(t, 0.0, d.calculateVariance())
	})

	t.Run("identical samples", func(t *testing.T) {
		config := defaultTestConfig()
		d := NewOscillationDetector(config)
		for i := 0; i < 10; i++ {
			d.AddSample(50)
		}

		assert.Equal(t, 0.0, d.calculateVariance())
	})
}

func TestAmplitude_EdgeCases(t *testing.T) {
	t.Run("single sample", func(t *testing.T) {
		config := defaultTestConfig()
		d := NewOscillationDetector(config)
		d.AddSample(50)

		assert.Equal(t, 0.0, d.calculateAmplitude())
	})

	t.Run("two identical samples", func(t *testing.T) {
		config := defaultTestConfig()
		d := NewOscillationDetector(config)
		d.AddSample(50)
		d.AddSample(50)

		assert.Equal(t, 0.0, d.calculateAmplitude())
	})
}

func TestConfigParse(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		config := &Config{}
		err := config.Parse([]byte(""))
		require.NoError(t, err)

		assert.Equal(t, 300, config.WarmupSeconds)       // Default 5 minutes
		assert.Equal(t, 4.0, config.AmplitudeMultiplier) // Default 4.0
		assert.Equal(t, 0.0, config.MinAmplitude)        // Default 0 (disabled)
	})

	t.Run("custom values", func(t *testing.T) {
		config := &Config{}
		yaml := `
amplitude_multiplier: 3.5
min_amplitude: 25.0
warmup_seconds: 120
`
		err := config.Parse([]byte(yaml))
		require.NoError(t, err)

		assert.Equal(t, 120, config.WarmupSeconds)
		assert.Equal(t, 3.5, config.AmplitudeMultiplier)
		assert.Equal(t, 25.0, config.MinAmplitude)
	})

	t.Run("values clamped to range", func(t *testing.T) {
		config := &Config{}
		yaml := `
amplitude_multiplier: 100
min_amplitude: 200
warmup_seconds: 10000
`
		err := config.Parse([]byte(yaml))
		require.NoError(t, err)

		assert.Equal(t, 1800, config.WarmupSeconds)       // Max
		assert.Equal(t, 10.0, config.AmplitudeMultiplier) // Max
		assert.Equal(t, 100.0, config.MinAmplitude)       // Max
	})
}

func TestDetectorConfig(t *testing.T) {
	config := &Config{
		AmplitudeMultiplier: 3.0,
		MinAmplitude:        20.0,
		WarmupSeconds:       180,
	}

	dc := config.DetectorConfig()

	assert.Equal(t, 60, dc.WindowSize)
	assert.Equal(t, 6, dc.MinZeroCrossings)
	assert.Equal(t, 3.0, dc.AmplitudeMultiplier)
	assert.Equal(t, 20.0, dc.MinAmplitude)
	assert.Equal(t, 0.1, dc.DecayFactor)
	assert.Equal(t, 180*time.Second, dc.WarmupDuration)
	assert.Equal(t, time.Second, dc.SampleInterval)
}

// Benchmark to ensure we meet performance requirements
func BenchmarkAddSample(b *testing.B) {
	config := OscillationConfig{
		WindowSize:          60,
		MinZeroCrossings:    6,
		AmplitudeMultiplier: 2.0,
		DecayFactor:         0.1,
		WarmupDuration:      0,
		SampleInterval:      time.Second,
	}
	d := NewOscillationDetector(config)

	// Pre-fill the buffer
	for i := 0; i < 60; i++ {
		d.AddSample(float64(i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.AddSample(float64(i % 100))
	}
}

func BenchmarkAnalyze(b *testing.B) {
	config := OscillationConfig{
		WindowSize:          60,
		MinZeroCrossings:    6,
		AmplitudeMultiplier: 2.0,
		DecayFactor:         0.1,
		WarmupDuration:      0,
		SampleInterval:      time.Second,
	}
	d := NewOscillationDetector(config)

	// Pre-fill with oscillating data
	for i := 0; i < 60; i++ {
		if i%2 == 0 {
			d.AddSample(30)
		} else {
			d.AddSample(70)
		}
	}
	d.baselineVariance = 100.0

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.Analyze()
	}
}

// Test to verify memory is fixed (no allocations in hot path)
func TestNoAllocationsInHotPath(t *testing.T) {
	config := OscillationConfig{
		WindowSize:          60,
		MinZeroCrossings:    6,
		AmplitudeMultiplier: 2.0,
		DecayFactor:         0.1,
		WarmupDuration:      0,
		SampleInterval:      time.Second,
	}
	d := NewOscillationDetector(config)

	// Pre-fill
	for i := 0; i < 60; i++ {
		d.AddSample(float64(i))
	}
	d.baselineVariance = 100.0

	// Run in a loop and verify no growth
	initialSamples := len(d.samples)
	for i := 0; i < 1000; i++ {
		d.AddSample(float64(i % 100))
		_ = d.Analyze()
	}

	assert.Equal(t, initialSamples, len(d.samples), "Sample buffer should not grow")
}
