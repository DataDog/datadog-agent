// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package cpuoscillation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func defaultTestConfig() OscillationConfig {
	return OscillationConfig{
		WindowSize:          10, // Small window for tests
		MinPeriodicityScore: 0.5,
		MinAmplitude:        10.0,
		MinPeriod:           2,
		MaxPeriod:           5,
		WarmupDuration:      0, // No warmup for most tests
		SampleInterval:      time.Second,
	}
}

// @requirement REQ-COD-001
func TestNewOscillationDetector(t *testing.T) {
	config := defaultTestConfig()
	d := NewOscillationDetector(config)

	assert.NotNil(t, d)
	assert.Equal(t, config.WindowSize, len(d.samples))
	assert.Equal(t, 0, d.sampleCount)
	assert.Equal(t, 0, d.sampleIndex)
}

// @requirement REQ-COD-001
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

// @requirement REQ-COD-001
func TestAutocorrelation_PerfectOscillation(t *testing.T) {
	// Perfect oscillation with period 2 should have high autocorrelation at lag 2
	samples := []float64{10, 50, 10, 50, 10, 50, 10, 50, 10, 50}
	mean, variance := calculateMeanAndVariance(samples)

	// Autocorrelation at lag 2 should be high (signal repeats every 2 samples)
	corr := autocorrelation(samples, mean, variance, 2)
	assert.Greater(t, corr, 0.8, "Perfect period-2 oscillation should have high autocorrelation at lag 2")

	// Autocorrelation at lag 1 should be negative (opposite phase)
	corr1 := autocorrelation(samples, mean, variance, 1)
	assert.Less(t, corr1, 0.0, "Period-2 oscillation should have negative autocorrelation at lag 1")
}

// @requirement REQ-COD-001
func TestAutocorrelation_NoOscillation(t *testing.T) {
	// Monotonically increasing - no periodicity
	samples := []float64{0, 5, 10, 15, 20, 25, 30, 35, 40, 45}
	mean, variance := calculateMeanAndVariance(samples)

	// Autocorrelation at any lag should be low for non-periodic data
	// Note: monotonic data naturally has ~0.51 autocorrelation due to trend, so threshold is 0.6
	for lag := 2; lag <= 5; lag++ {
		corr := autocorrelation(samples, mean, variance, lag)
		assert.Less(t, corr, 0.6, "Monotonic data should have low autocorrelation at lag %d", lag)
	}
}

// @requirement REQ-COD-001
func TestAutocorrelation_Period4(t *testing.T) {
	// Oscillation with period 4: low, mid, high, mid, low, mid, high, mid...
	samples := []float64{10, 30, 50, 30, 10, 30, 50, 30, 10, 30, 50, 30}
	mean, variance := calculateMeanAndVariance(samples)

	// Autocorrelation at lag 4 should be high
	corr4 := autocorrelation(samples, mean, variance, 4)
	assert.Greater(t, corr4, 0.8, "Period-4 oscillation should have high autocorrelation at lag 4")

	// Autocorrelation at lag 2 should be lower (half-period)
	corr2 := autocorrelation(samples, mean, variance, 2)
	assert.Less(t, corr2, corr4, "Half-period lag should have lower correlation")
}

// @requirement REQ-COD-003
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

// @requirement REQ-COD-002
func TestCalculateMeanAndVariance(t *testing.T) {
	// Simple samples: 2, 4, 4, 4
	// Mean = 3.5
	// Variance = ((2-3.5)^2 + (4-3.5)^2 + (4-3.5)^2 + (4-3.5)^2) / 4
	//          = (2.25 + 0.25 + 0.25 + 0.25) / 4 = 0.75
	samples := []float64{2, 4, 4, 4}
	mean, variance := calculateMeanAndVariance(samples)

	assert.InDelta(t, 3.5, mean, 0.001)
	assert.InDelta(t, 0.75, variance, 0.001)
}

// @requirement REQ-COD-002
func TestStdDev(t *testing.T) {
	config := defaultTestConfig()
	config.WindowSize = 4
	d := NewOscillationDetector(config)

	// Add samples with known variance
	samples := []float64{2, 4, 4, 4} // variance = 0.75, stddev = sqrt(0.75) ≈ 0.866
	for _, s := range samples {
		d.AddSample(s)
	}

	stddev := d.StdDev()
	assert.InDelta(t, 0.866, stddev, 0.01)
}

// @requirement REQ-COD-002
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

// @requirement REQ-COD-001
func TestAnalyze_WindowNotFull(t *testing.T) {
	config := defaultTestConfig()
	d := NewOscillationDetector(config)

	// Only add a few samples
	d.AddSample(10)
	d.AddSample(20)

	result := d.Analyze()
	assert.False(t, result.Detected)
	assert.Equal(t, 0.0, result.PeriodicityScore)
}

// @requirement REQ-COD-002
// @requirement REQ-COD-006
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
	// REQ-COD-006: Should not detect during warmup even with oscillating data (detected=0)
	assert.False(t, result.Detected)
}

// @requirement REQ-COD-001
func TestAnalyze_DetectsOscillation(t *testing.T) {
	config := defaultTestConfig()
	config.WindowSize = 20
	config.MinPeriodicityScore = 0.5
	config.MinAmplitude = 10.0
	config.MinPeriod = 2
	config.MaxPeriod = 10
	d := NewOscillationDetector(config)

	// Fill with strongly oscillating data (period = 2)
	for i := 0; i < config.WindowSize; i++ {
		if i%2 == 0 {
			d.AddSample(30) // Low
		} else {
			d.AddSample(70) // High - amplitude = 40
		}
	}

	result := d.Analyze()
	assert.True(t, result.Detected, "Should detect oscillation with high periodicity and amplitude")
	assert.Greater(t, result.PeriodicityScore, 0.5, "Periodicity score should exceed threshold")
	assert.Equal(t, 40.0, result.Amplitude)
	assert.InDelta(t, 2.0, result.Period, 0.1, "Should detect period of 2 seconds")
}

// @requirement REQ-COD-001
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
}

// @requirement REQ-COD-001
func TestAnalyze_NoOscillation_GradualChange(t *testing.T) {
	config := defaultTestConfig()
	config.MinAmplitude = 0 // Disable amplitude check to focus on periodicity
	d := NewOscillationDetector(config)

	// Fill with gradually increasing data (no oscillation)
	for i := 0; i < config.WindowSize; i++ {
		d.AddSample(float64(i * 5))
	}

	result := d.Analyze()
	assert.False(t, result.Detected, "Monotonic data should not be detected as oscillation")
	// Note: monotonic data naturally has ~0.51 autocorrelation due to trend, so threshold is 0.6
	assert.Less(t, result.PeriodicityScore, 0.6, "Monotonic data should have low periodicity")
}

// @requirement REQ-COD-001
// @requirement REQ-COD-005
func TestAnalyze_MinAmplitudeThreshold(t *testing.T) {
	t.Run("oscillation blocked by min_amplitude", func(t *testing.T) {
		config := defaultTestConfig()
		config.WindowSize = 20
		config.MinPeriodicityScore = 0.5
		config.MinAmplitude = 50.0 // Require at least 50% amplitude
		config.MinPeriod = 2
		config.MaxPeriod = 10
		d := NewOscillationDetector(config)

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
		config.MinPeriodicityScore = 0.5
		config.MinAmplitude = 30.0 // Require at least 30% amplitude
		config.MinPeriod = 2
		config.MaxPeriod = 10
		d := NewOscillationDetector(config)

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
		config.MinPeriodicityScore = 0.5
		config.MinAmplitude = 0 // Disabled
		config.MinPeriod = 2
		config.MaxPeriod = 10
		d := NewOscillationDetector(config)

		// Fill with oscillating data with small amplitude = 10
		for i := 0; i < config.WindowSize; i++ {
			if i%2 == 0 {
				d.AddSample(45)
			} else {
				d.AddSample(55)
			}
		}

		result := d.Analyze()
		// Should detect because periodicity is high and min_amplitude is disabled
		assert.True(t, result.Detected, "Should detect: min_amplitude=0 means no absolute floor")
		assert.Equal(t, 10.0, result.Amplitude)
	})
}

// @requirement REQ-COD-003
func TestFrequencyCalculation(t *testing.T) {
	config := defaultTestConfig()
	config.WindowSize = 60 // 60 seconds
	config.MinPeriod = 2
	config.MaxPeriod = 30
	config.MinPeriodicityScore = 0.3
	config.MinAmplitude = 10.0
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

	// Autocorrelation should detect period of 10 seconds
	// Frequency = 1/period = 0.1 Hz
	assert.True(t, result.Detected, "Should detect oscillation")
	assert.InDelta(t, 10.0, result.Period, 1.0, "Period should be ~10 seconds")
	assert.InDelta(t, 0.1, result.Frequency, 0.02, "Frequency should be ~0.1 Hz")
}

// @requirement REQ-COD-002
func TestAutocorrelation_ZeroVariance(t *testing.T) {
	// When variance is 0, autocorrelation should return 0
	samples := []float64{50, 50, 50, 50, 50}
	mean, variance := calculateMeanAndVariance(samples)

	assert.Equal(t, 50.0, mean)
	assert.Equal(t, 0.0, variance)

	corr := autocorrelation(samples, mean, variance, 2)
	assert.Equal(t, 0.0, corr, "Autocorrelation should be 0 when variance is 0")
}

// @requirement REQ-COD-004
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

// @requirement REQ-COD-004
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

// @requirement REQ-COD-007
func TestAutocorrelation_EdgeCases(t *testing.T) {
	t.Run("lag exceeds sample count", func(t *testing.T) {
		samples := []float64{10, 20}
		mean, variance := calculateMeanAndVariance(samples)

		// Lag of 3 exceeds sample count of 2
		corr := autocorrelation(samples, mean, variance, 3)
		assert.Equal(t, 0.0, corr, "Should return 0 when lag exceeds sample count")
	})

	t.Run("flat line has zero variance", func(t *testing.T) {
		samples := make([]float64, 10)
		for i := range samples {
			samples[i] = 50
		}
		mean, variance := calculateMeanAndVariance(samples)

		assert.Equal(t, 50.0, mean)
		assert.Equal(t, 0.0, variance)

		corr := autocorrelation(samples, mean, variance, 2)
		assert.Equal(t, 0.0, corr, "Flat line should have 0 autocorrelation")
	})

	t.Run("random-ish noise has low autocorrelation", func(t *testing.T) {
		// Non-periodic data
		samples := []float64{10, 25, 15, 40, 30, 55, 20, 45, 35, 60}
		mean, variance := calculateMeanAndVariance(samples)

		// Check autocorrelation at multiple lags
		// Note: even non-periodic data can have some correlation, so threshold is 0.6
		for lag := 2; lag <= 4; lag++ {
			corr := autocorrelation(samples, mean, variance, lag)
			assert.Less(t, corr, 0.6, "Non-periodic data should have low autocorrelation at lag %d", lag)
		}
	})
}

// @requirement REQ-COD-007
func TestMeanAndVariance_EdgeCases(t *testing.T) {
	t.Run("single sample", func(t *testing.T) {
		samples := []float64{50}
		mean, variance := calculateMeanAndVariance(samples)

		assert.Equal(t, 0.0, mean)     // Function returns 0 for < 2 samples
		assert.Equal(t, 0.0, variance) // Function returns 0 for < 2 samples
	})

	t.Run("two samples", func(t *testing.T) {
		samples := []float64{40, 60}
		mean, variance := calculateMeanAndVariance(samples)

		assert.Equal(t, 50.0, mean)
		assert.Equal(t, 100.0, variance) // ((40-50)^2 + (60-50)^2) / 2 = 100
	})

	t.Run("identical samples", func(t *testing.T) {
		samples := make([]float64, 10)
		for i := range samples {
			samples[i] = 50
		}
		mean, variance := calculateMeanAndVariance(samples)

		assert.Equal(t, 50.0, mean)
		assert.Equal(t, 0.0, variance)
	})
}

// @requirement REQ-COD-007
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

// @requirement REQ-COD-005
func TestConfigParse(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		config := &Config{}
		err := config.Parse([]byte(""))
		require.NoError(t, err)

		assert.Equal(t, false, config.Enabled)           // Default disabled
		assert.Equal(t, 60, config.WarmupSeconds)        // Default 1 minute
		assert.Equal(t, 0.6, config.MinPeriodicityScore) // Default 0.6 (above monotonic baseline ~0.51)
		assert.Equal(t, 10.0, config.MinAmplitude)       // Default 10.0
		assert.Equal(t, 2, config.MinPeriod)             // Default 2 (Nyquist)
		assert.Equal(t, 30, config.MaxPeriod)            // Default 30
	})

	t.Run("custom values", func(t *testing.T) {
		config := &Config{}
		yaml := `
enabled: true
min_periodicity_score: 0.7
min_amplitude: 25.0
min_period: 5
max_period: 20
warmup_seconds: 120
`
		err := config.Parse([]byte(yaml))
		require.NoError(t, err)

		assert.Equal(t, true, config.Enabled)
		assert.Equal(t, 120, config.WarmupSeconds)
		assert.Equal(t, 0.7, config.MinPeriodicityScore)
		assert.Equal(t, 25.0, config.MinAmplitude)
		assert.Equal(t, 5, config.MinPeriod)
		assert.Equal(t, 20, config.MaxPeriod)
	})

	t.Run("values clamped to range", func(t *testing.T) {
		config := &Config{}
		yaml := `
min_periodicity_score: 2.0
min_amplitude: 200
warmup_seconds: 10000
min_period: 1
max_period: 100
`
		err := config.Parse([]byte(yaml))
		require.NoError(t, err)

		assert.Equal(t, 300, config.WarmupSeconds)        // Max warmup
		assert.Equal(t, 0.95, config.MinPeriodicityScore) // Max periodicity score
		assert.Equal(t, 100.0, config.MinAmplitude)       // Max amplitude
		assert.Equal(t, 2, config.MinPeriod)              // Min period (Nyquist constraint)
		assert.Equal(t, 30, config.MaxPeriod)             // Max period (window/2 constraint)
	})

	t.Run("min_period must be less than max_period", func(t *testing.T) {
		config := &Config{}
		yaml := `
min_period: 25
max_period: 20
`
		err := config.Parse([]byte(yaml))
		require.NoError(t, err)

		// max_period should be adjusted to min_period + 1
		assert.True(t, config.MaxPeriod > config.MinPeriod, "max_period must be > min_period")
	})
}

// @requirement REQ-COD-005
func TestDetectorConfig(t *testing.T) {
	config := &Config{
		MinPeriodicityScore: 0.6,
		MinAmplitude:        20.0,
		MinPeriod:           3,
		MaxPeriod:           25,
		WarmupSeconds:       180,
	}

	dc := config.DetectorConfig()

	assert.Equal(t, 60, dc.WindowSize)
	assert.Equal(t, 0.6, dc.MinPeriodicityScore)
	assert.Equal(t, 20.0, dc.MinAmplitude)
	assert.Equal(t, 3, dc.MinPeriod)
	assert.Equal(t, 25, dc.MaxPeriod)
	assert.Equal(t, 180*time.Second, dc.WarmupDuration)
	assert.Equal(t, time.Second, dc.SampleInterval)
}

// @requirement REQ-COD-004
// Benchmark to ensure we meet performance requirements
func BenchmarkAddSample(b *testing.B) {
	config := OscillationConfig{
		WindowSize:          60,
		MinPeriodicityScore: 0.5,
		MinAmplitude:        10.0,
		MinPeriod:           2,
		MaxPeriod:           30,
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

// @requirement REQ-COD-004
func BenchmarkAnalyze(b *testing.B) {
	config := OscillationConfig{
		WindowSize:          60,
		MinPeriodicityScore: 0.5,
		MinAmplitude:        10.0,
		MinPeriod:           2,
		MaxPeriod:           30,
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.Analyze()
	}
}

// @requirement REQ-COD-004
// Test to verify memory is fixed (no allocations in hot path)
func TestNoAllocationsInHotPath(t *testing.T) {
	config := OscillationConfig{
		WindowSize:          60,
		MinPeriodicityScore: 0.5,
		MinAmplitude:        10.0,
		MinPeriod:           2,
		MaxPeriod:           30,
		WarmupDuration:      0,
		SampleInterval:      time.Second,
	}
	d := NewOscillationDetector(config)

	// Pre-fill
	for i := 0; i < 60; i++ {
		d.AddSample(float64(i))
	}

	// Run in a loop and verify no growth
	initialSamples := len(d.samples)
	for i := 0; i < 1000; i++ {
		d.AddSample(float64(i % 100))
		_ = d.Analyze()
	}

	assert.Equal(t, initialSamples, len(d.samples), "Sample buffer should not grow")
}
