// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux || darwin

package oscillation

import (
	"math"
	"time"
)

// OscillationConfig holds configuration for the detector
type OscillationConfig struct {
	WindowSize          int           // Number of samples in ring buffer (default: 60)
	MinZeroCrossings    int           // Minimum direction changes to flag (default: 6)
	AmplitudeMultiplier float64       // Baseline multiplier for significance (default: 2.0)
	DecayFactor         float64       // Exponential decay alpha (default: 0.1)
	WarmupDuration      time.Duration // Initial learning period (default: 5m)
	SampleInterval      time.Duration // Time between samples (default: 1s)
}

// OscillationResult contains the results of oscillation analysis
type OscillationResult struct {
	Detected      bool
	Amplitude     float64 // Peak-to-trough percentage
	Frequency     float64 // Cycles per second (Hz)
	ZeroCrossings int     // Number of direction changes
}

// OscillationDetector analyzes CPU samples for oscillation patterns
type OscillationDetector struct {
	// Ring buffer for CPU samples (fixed size, no allocation after init)
	samples     []float64
	sampleIndex int
	sampleCount int

	// Baseline tracking with exponential decay
	baselineVariance float64

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

// countZeroCrossings counts direction changes in the CPU samples
func (d *OscillationDetector) countZeroCrossings() int {
	if d.sampleCount < 3 {
		return 0
	}

	crossings := 0
	var lastNonZeroDir int // -1 for decreasing, +1 for increasing, 0 for unset

	for i := 1; i < d.sampleCount; i++ {
		curr := d.getSample(i)
		prev := d.getSample(i - 1)
		currDiff := curr - prev

		// Determine current direction (skip zero diffs)
		var currDir int
		if currDiff > 0 {
			currDir = 1
		} else if currDiff < 0 {
			currDir = -1
		} else {
			continue // Skip flat sections
		}

		// Count sign change from last non-zero direction
		if lastNonZeroDir != 0 && currDir != lastNonZeroDir {
			crossings++
		}
		lastNonZeroDir = currDir
	}
	return crossings
}

// calculateAmplitude returns the peak-to-trough difference in the window
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

// calculateVariance computes the variance of samples in the current window
func (d *OscillationDetector) calculateVariance() float64 {
	if d.sampleCount < 2 {
		return 0
	}

	// Calculate mean
	sum := 0.0
	for i := 0; i < d.sampleCount; i++ {
		sum += d.getSample(i)
	}
	mean := sum / float64(d.sampleCount)

	// Calculate variance
	sumSquaredDiff := 0.0
	for i := 0; i < d.sampleCount; i++ {
		diff := d.getSample(i) - mean
		sumSquaredDiff += diff * diff
	}

	return sumSquaredDiff / float64(d.sampleCount)
}

// updateBaseline updates the baseline variance using exponential decay
func (d *OscillationDetector) updateBaseline(newVariance float64) {
	if d.baselineVariance == 0 {
		// First sample
		d.baselineVariance = newVariance
		return
	}

	// Exponential decay: new = α * current + (1-α) * old
	alpha := d.config.DecayFactor
	d.baselineVariance = alpha*newVariance + (1-alpha)*d.baselineVariance
}

// DecrementWarmup decreases the warmup timer by one sample interval
func (d *OscillationDetector) DecrementWarmup() {
	if d.warmupRemaining > 0 {
		d.warmupRemaining -= d.config.SampleInterval
		if d.warmupRemaining < 0 {
			d.warmupRemaining = 0
		}
	}
}

// IsWarmedUp returns true if the warmup period has completed
func (d *OscillationDetector) IsWarmedUp() bool {
	return d.warmupRemaining <= 0
}

// IsWindowFull returns true if we have enough samples to analyze
func (d *OscillationDetector) IsWindowFull() bool {
	return d.sampleCount >= d.config.WindowSize
}

// BaselineStdDev returns the current baseline standard deviation
func (d *OscillationDetector) BaselineStdDev() float64 {
	return math.Sqrt(d.baselineVariance)
}

// Analyze performs oscillation detection on the current window
func (d *OscillationDetector) Analyze() OscillationResult {
	result := OscillationResult{}

	// No analysis until window is full (60 samples)
	if !d.IsWindowFull() {
		return result
	}

	currentVariance := d.calculateVariance()

	// Still in warmup - learn baseline but don't flag oscillation
	if !d.IsWarmedUp() {
		d.updateBaseline(currentVariance)
		return result
	}

	zeroCrossings := d.countZeroCrossings()
	amplitude := d.calculateAmplitude()

	// Update baseline (continuous learning)
	d.updateBaseline(currentVariance)

	// Check oscillation criteria
	baselineStdDev := d.BaselineStdDev()
	amplitudeThreshold := d.config.AmplitudeMultiplier * baselineStdDev

	result.ZeroCrossings = zeroCrossings
	result.Amplitude = amplitude
	// Frequency = cycles per second. Zero crossings / 2 = cycles, divided by window size in seconds
	result.Frequency = float64(zeroCrossings) / float64(d.config.WindowSize) / 2.0

	if zeroCrossings >= d.config.MinZeroCrossings && amplitude > amplitudeThreshold {
		result.Detected = true
	}

	return result
}
