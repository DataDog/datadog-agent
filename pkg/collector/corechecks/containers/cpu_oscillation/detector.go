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
	WindowSize            int           // Number of samples in ring buffer (default: 60)
	MinDirectionReversals int           // Minimum direction changes to flag (default: 6)
	AmplitudeMultiplier   float64       // Baseline multiplier for significance (default: 4.0)
	MinAmplitude          float64       // Absolute minimum amplitude to trigger (default: 0, disabled)
	DecayFactor           float64       // Exponential decay alpha (default: 0.1)
	WarmupDuration        time.Duration // Initial learning period (default: 5m)
	SampleInterval        time.Duration // Time between samples (default: 1s)
}

// OscillationResult contains the results of oscillation analysis
// REQ-COD-003: Report Oscillation Characteristics with Container Tags
type OscillationResult struct {
	Detected           bool
	Amplitude          float64 // Peak-to-trough percentage
	Frequency          float64 // Cycles per second (Hz)
	DirectionReversals int     // Number of direction changes (rising→falling or falling→rising)
}

// OscillationDetector analyzes CPU samples for oscillation patterns
// One instance per container
// REQ-COD-001: Direction reversal count + amplitude detection
// REQ-COD-002: Baseline tracking with exponential decay
// REQ-COD-004: Fixed memory per container (~500 bytes)
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

// countDirectionReversals counts direction changes in the CPU samples
// REQ-COD-001: Count direction changes (rising to falling, falling to rising)
func (d *OscillationDetector) countDirectionReversals() int {
	if d.sampleCount < 3 {
		return 0
	}

	reversals := 0
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
			reversals++
		}
		lastNonZeroDir = currDir
	}
	return reversals
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
// REQ-COD-002: Update baseline variance using exponential decay
func (d *OscillationDetector) updateBaseline(newVariance float64) {
	if d.baselineVariance == 0 {
		// First sample
		d.baselineVariance = newVariance
		return
	}

	// Exponential decay: new = alpha * current + (1-alpha) * old
	alpha := d.config.DecayFactor
	d.baselineVariance = alpha*newVariance + (1-alpha)*d.baselineVariance
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

// BaselineStdDev returns the current baseline standard deviation
func (d *OscillationDetector) BaselineStdDev() float64 {
	return math.Sqrt(d.baselineVariance)
}

// Analyze performs oscillation detection on the current window
// REQ-COD-001: Detection logic - direction reversals >= 6 AND amplitude > 4x baseline stddev
// REQ-COD-006: Returns result for all containers (detected=false during warmup)
func (d *OscillationDetector) Analyze() OscillationResult {
	result := OscillationResult{}

	// No analysis until window is full (60 samples)
	if !d.IsWindowFull() {
		return result
	}

	currentVariance := d.calculateVariance()

	// Still in warmup - learn baseline but don't flag oscillation
	// REQ-COD-002: Learn baseline during warmup without flagging
	// REQ-COD-006: Emit detected=0 during warmup
	if !d.IsWarmedUp() {
		d.updateBaseline(currentVariance)
		return result
	}

	directionReversals := d.countDirectionReversals()
	amplitude := d.calculateAmplitude()

	// Update baseline (continuous learning)
	d.updateBaseline(currentVariance)

	// Check oscillation criteria
	baselineStdDev := d.BaselineStdDev()
	amplitudeThreshold := d.config.AmplitudeMultiplier * baselineStdDev

	result.DirectionReversals = directionReversals
	result.Amplitude = amplitude
	// REQ-COD-003: Frequency = cycles per second. Direction reversals / 2 = cycles, divided by window size in seconds
	result.Frequency = float64(directionReversals) / float64(d.config.WindowSize) / 2.0

	// REQ-COD-001: Check oscillation criteria:
	// 1. Enough direction changes (rapid cycling) - more than 6 times within 60s
	// 2. Amplitude exceeds baseline-relative threshold - exceeds 4x baseline stddev
	// 3. Amplitude exceeds absolute minimum (if configured)
	meetsReversalCount := directionReversals >= d.config.MinDirectionReversals
	meetsRelativeThreshold := amplitude > amplitudeThreshold
	meetsAbsoluteThreshold := d.config.MinAmplitude == 0 || amplitude > d.config.MinAmplitude

	if meetsReversalCount && meetsRelativeThreshold && meetsAbsoluteThreshold {
		result.Detected = true
	}

	return result
}
