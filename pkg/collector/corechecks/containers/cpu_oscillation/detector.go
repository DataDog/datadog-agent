// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package cpuoscillation implements the per-container CPU oscillation detection check.
// REQ-COD-001: Detect Rapid CPU Cycling Per Container
// REQ-COD-002: Establish Container-Specific Baseline
package cpuoscillation

import (
	"math"
	"time"
)

// OscillationConfig holds configuration for the detector
// REQ-COD-005: Configurable Detection with Default Disabled
type OscillationConfig struct {
	WindowSize          int           // Number of samples in ring buffer (default: 60)
	MinPeriodicityScore float64       // Minimum autocorrelation peak to detect (default: 0.6)
	MinAmplitude        float64       // Absolute minimum amplitude to trigger (default: 10.0)
	MinPeriod           int           // Minimum period in samples/seconds (default: 2, Nyquist limit)
	MaxPeriod           int           // Maximum period in samples/seconds (default: 30)
	WarmupDuration      time.Duration // Initial learning period (default: 60s)
	SampleInterval      time.Duration // Time between samples (default: 1s)
}

// OscillationResult contains the results of oscillation analysis
// REQ-COD-003: Report Oscillation Characteristics with Container Tags
type OscillationResult struct {
	Detected         bool
	PeriodicityScore float64 // Peak autocorrelation value (0.0-1.0)
	Period           float64 // Detected period in seconds
	Frequency        float64 // Cycles per second (Hz = 1/Period)
	Amplitude        float64 // Peak-to-trough percentage
}

// OscillationDetector analyzes CPU samples for periodic oscillation patterns
// using autocorrelation-based detection.
// One instance per container.
// REQ-COD-001: Autocorrelation-based periodicity detection
// REQ-COD-004: Fixed memory per container (~500 bytes)
type OscillationDetector struct {
	// Ring buffer for CPU samples (fixed size, no allocation after init)
	samples     []float64
	sampleIndex int
	sampleCount int

	// Configuration
	config OscillationConfig

	// State
	warmupRemaining time.Duration
}

// NewOscillationDetector creates a new detector with the given configuration
func NewOscillationDetector(config OscillationConfig) *OscillationDetector {
	return &OscillationDetector{
		samples:         make([]float64, config.WindowSize),
		config:          config,
		warmupRemaining: config.WarmupDuration,
	}
}

// AddSample adds a new CPU percentage sample to the ring buffer
func (d *OscillationDetector) AddSample(cpuPercent float64) {
	d.samples[d.sampleIndex] = cpuPercent
	d.sampleIndex = (d.sampleIndex + 1) % d.config.WindowSize

	if d.sampleCount < d.config.WindowSize {
		d.sampleCount++
	}
}

// getSample returns the sample at logical index i (0 = oldest in buffer)
func (d *OscillationDetector) getSample(i int) float64 {
	// Calculate actual index in ring buffer
	// sampleIndex points to where next sample will go (oldest when full)
	if d.sampleCount < d.config.WindowSize {
		return d.samples[i]
	}
	actualIndex := (d.sampleIndex + i) % d.config.WindowSize
	return d.samples[actualIndex]
}

// getSamplesInOrder returns a copy of samples in chronological order (oldest first)
func (d *OscillationDetector) getSamplesInOrder() []float64 {
	result := make([]float64, d.sampleCount)
	for i := 0; i < d.sampleCount; i++ {
		result[i] = d.getSample(i)
	}
	return result
}

// autocorrelation computes the normalized autocorrelation at a given lag
// Returns a value in [-1, 1] where:
// - 1.0 means perfect positive correlation (signal repeats exactly)
// - 0.0 means no correlation (random noise)
// - -1.0 means perfect negative correlation (signal inverts)
func autocorrelation(samples []float64, mean, variance float64, lag int) float64 {
	if variance == 0 || lag >= len(samples) {
		return 0
	}

	n := len(samples)
	sum := 0.0
	count := n - lag

	for i := 0; i < count; i++ {
		sum += (samples[i] - mean) * (samples[i+lag] - mean)
	}

	if count == 0 {
		return 0
	}

	// Normalize by variance to get correlation coefficient in [-1, 1]
	return sum / (float64(count) * variance)
}

// calculateAmplitude returns the peak-to-trough difference in the window
// REQ-COD-003: Report swing amplitude (peak-to-trough percentage)
func (d *OscillationDetector) calculateAmplitude() float64 {
	if d.sampleCount < 2 {
		return 0
	}

	min, max := d.getSample(0), d.getSample(0)
	for i := 1; i < d.sampleCount; i++ {
		v := d.getSample(i)
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return max - min
}

// calculateMeanAndVariance computes the mean and variance of a sample slice
func calculateMeanAndVariance(samples []float64) (mean, variance float64) {
	n := len(samples)
	if n < 2 {
		return 0, 0
	}

	// Calculate mean
	sum := 0.0
	for _, v := range samples {
		sum += v
	}
	mean = sum / float64(n)

	// Calculate variance
	sumSquaredDiff := 0.0
	for _, v := range samples {
		diff := v - mean
		sumSquaredDiff += diff * diff
	}
	variance = sumSquaredDiff / float64(n)

	return mean, variance
}

// StdDev returns the standard deviation of samples in the current window
func (d *OscillationDetector) StdDev() float64 {
	samples := d.getSamplesInOrder()
	_, variance := calculateMeanAndVariance(samples)
	return math.Sqrt(variance)
}

// DecrementWarmup decreases the warmup timer by one sample interval
// REQ-COD-002: Warmup period tracking
func (d *OscillationDetector) DecrementWarmup() {
	if d.warmupRemaining > 0 {
		d.warmupRemaining -= d.config.SampleInterval
		if d.warmupRemaining < 0 {
			d.warmupRemaining = 0
		}
	}
}

// IsWarmedUp returns true if the warmup period has completed
// REQ-COD-002: 5-minute warmup per container
func (d *OscillationDetector) IsWarmedUp() bool {
	return d.warmupRemaining <= 0
}

// IsWindowFull returns true if we have enough samples to analyze
func (d *OscillationDetector) IsWindowFull() bool {
	return d.sampleCount >= d.config.WindowSize
}

// Analyze performs autocorrelation-based oscillation detection on the current window
// REQ-COD-001: Detection logic - periodicity score >= threshold AND amplitude >= min_amplitude
// REQ-COD-006: Returns result for all containers (detected=false during warmup)
func (d *OscillationDetector) Analyze() OscillationResult {
	result := OscillationResult{}

	// No analysis until window is full (60 samples)
	if !d.IsWindowFull() {
		return result
	}

	// Still in warmup - don't flag oscillation
	// REQ-COD-002: Warmup period before detection
	// REQ-COD-006: Emit detected=0 during warmup
	if !d.IsWarmedUp() {
		return result
	}

	// Get samples and compute statistics
	samples := d.getSamplesInOrder()
	mean, variance := calculateMeanAndVariance(samples)
	amplitude := d.calculateAmplitude()

	result.Amplitude = amplitude

	// Early exit if amplitude is below threshold (no need to compute autocorrelation)
	if d.config.MinAmplitude > 0 && amplitude < d.config.MinAmplitude {
		return result
	}

	// Compute autocorrelation for lags in [MinPeriod, MaxPeriod]
	// Find the lag with the highest autocorrelation (strongest periodicity)
	bestLag := 0
	bestCorr := 0.0

	for lag := d.config.MinPeriod; lag <= d.config.MaxPeriod; lag++ {
		corr := autocorrelation(samples, mean, variance, lag)
		if corr > bestCorr {
			bestCorr = corr
			bestLag = lag
		}
	}

	result.PeriodicityScore = bestCorr

	// Convert lag (in samples) to period (in seconds)
	if bestLag > 0 {
		result.Period = float64(bestLag) * d.config.SampleInterval.Seconds()
		result.Frequency = 1.0 / result.Period
	}

	// REQ-COD-001: Detection criteria (all must be met):
	// 1. Periodicity score exceeds threshold (autocorrelation peak is significant)
	// 2. Amplitude exceeds minimum threshold
	meetsPeriodicityThreshold := bestCorr >= d.config.MinPeriodicityScore
	meetsAmplitudeThreshold := d.config.MinAmplitude == 0 || amplitude >= d.config.MinAmplitude

	if meetsPeriodicityThreshold && meetsAmplitudeThreshold {
		result.Detected = true
	}

	return result
}
