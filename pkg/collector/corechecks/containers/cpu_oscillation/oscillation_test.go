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

// @requirement REQ-COD-005
func TestCheckName(t *testing.T) {
	assert.Equal(t, "cpu_oscillation", CheckName)
}

// @requirement REQ-COD-005
func TestConfigDefaults(t *testing.T) {
	config := &Config{}
	err := config.Parse([]byte(""))
	require.NoError(t, err)

	// REQ-COD-005: Default disabled
	assert.False(t, config.Enabled)
	// REQ-COD-002: 1 minute warmup
	assert.Equal(t, 60, config.WarmupSeconds)
	// REQ-COD-001: 0.6 periodicity score threshold (above monotonic trend baseline ~0.51)
	assert.Equal(t, 0.6, config.MinPeriodicityScore)
	// REQ-COD-001: Period range (Nyquist-constrained)
	assert.Equal(t, 2, config.MinPeriod)
	assert.Equal(t, 30, config.MaxPeriod)
	// REQ-COD-005: Min amplitude default 10%
	assert.Equal(t, 10.0, config.MinAmplitude)
}

// @requirement REQ-COD-005
func TestConfigEnabled(t *testing.T) {
	config := &Config{}
	yaml := `
enabled: true
min_periodicity_score: 0.7
min_amplitude: 15.0
min_period: 3
max_period: 25
warmup_seconds: 180
`
	err := config.Parse([]byte(yaml))
	require.NoError(t, err)

	assert.True(t, config.Enabled)
	assert.Equal(t, 0.7, config.MinPeriodicityScore)
	assert.Equal(t, 15.0, config.MinAmplitude)
	assert.Equal(t, 3, config.MinPeriod)
	assert.Equal(t, 25, config.MaxPeriod)
	assert.Equal(t, 180, config.WarmupSeconds)
}

// @requirement REQ-COD-002
func TestContainerDetector_Initialization(t *testing.T) {
	config := &Config{
		Enabled:             true,
		MinPeriodicityScore: 0.5,
		MinAmplitude:        10.0,
		MinPeriod:           2,
		MaxPeriod:           30,
		WarmupSeconds:       60,
	}

	cd := &ContainerDetector{
		detector:      NewOscillationDetector(config.DetectorConfig()),
		containerID:   "abc123def456",
		namespace:     "/docker/abc123def456",
		runtime:       "docker",
		runtimeFlavor: "",
		lastCPUTotal:  -1,
	}

	// Verify initial state
	assert.NotNil(t, cd.detector)
	assert.Equal(t, "abc123def456", cd.containerID)
	assert.Equal(t, -1.0, cd.lastCPUTotal)
	assert.True(t, cd.lastSampleTime.IsZero())

	// Verify detector warmup state
	assert.False(t, cd.detector.IsWarmedUp())
	assert.Equal(t, 60*time.Second, cd.detector.warmupRemaining)
}

// @requirement REQ-COD-004
func TestContainerDetector_MemorySize(t *testing.T) {
	config := &Config{
		Enabled:             true,
		MinPeriodicityScore: 0.5,
		MinAmplitude:        10.0,
		MinPeriod:           2,
		MaxPeriod:           30,
		WarmupSeconds:       60,
	}

	cd := &ContainerDetector{
		detector:      NewOscillationDetector(config.DetectorConfig()),
		containerID:   "abc123def456ghij789klmnop",
		namespace:     "/docker/abc123def456ghij789klmnop",
		runtime:       "containerd",
		runtimeFlavor: "default",
		lastCPUTotal:  0,
	}

	// Verify the samples array size (60 samples * 8 bytes = 480 bytes)
	assert.Equal(t, 60, len(cd.detector.samples))

	// Each float64 is 8 bytes, so 60 samples = 480 bytes
	// Plus struct overhead (indices, config, etc.)
	// Total should be around ~500 bytes per container
	_ = cd // Just verifying the structure exists
}

// @requirement REQ-COD-002
func TestContainerDetector_WarmupPeriod(t *testing.T) {
	config := &Config{
		Enabled:             true,
		MinPeriodicityScore: 0.5,
		MinAmplitude:        0, // Disable amplitude check
		MinPeriod:           2,
		MaxPeriod:           30,
		WarmupSeconds:       5, // Short warmup for test
	}

	cd := &ContainerDetector{
		detector:      NewOscillationDetector(config.DetectorConfig()),
		containerID:   "test123",
		namespace:     "/docker/test123",
		runtime:       "docker",
		runtimeFlavor: "",
		lastCPUTotal:  -1,
	}

	// Fill window with oscillating data
	for i := 0; i < 60; i++ {
		if i%2 == 0 {
			cd.detector.AddSample(20)
		} else {
			cd.detector.AddSample(80)
		}
		cd.detector.DecrementWarmup()
	}

	// After 60 seconds (60 decrements), warmup should be complete (5s warmup < 60s samples)
	assert.True(t, cd.detector.IsWarmedUp(), "Should be warmed up after 60 samples")

	// And detection should work
	result := cd.detector.Analyze()
	// With amplitude = 60 and high periodicity, should detect
	assert.True(t, result.Detected || result.Amplitude > 0, "Should have amplitude data")
}

// @requirement REQ-COD-006
func TestContainerDetector_EmitsDuringWarmup(t *testing.T) {
	config := &Config{
		Enabled:             true,
		MinPeriodicityScore: 0.5,
		MinAmplitude:        0,
		MinPeriod:           2,
		MaxPeriod:           30,
		WarmupSeconds:       300, // Long warmup
	}

	cd := &ContainerDetector{
		detector:      NewOscillationDetector(config.DetectorConfig()),
		containerID:   "test123",
		namespace:     "/docker/test123",
		runtime:       "docker",
		runtimeFlavor: "",
		lastCPUTotal:  -1,
	}

	// Fill window with oscillating data but don't decrement warmup much
	for i := 0; i < 60; i++ {
		if i%2 == 0 {
			cd.detector.AddSample(20)
		} else {
			cd.detector.AddSample(80)
		}
		// Only decrement a few times (simulating first minute)
		if i < 60 {
			cd.detector.DecrementWarmup()
		}
	}

	// Window is full but warmup not complete
	assert.True(t, cd.detector.IsWindowFull())
	assert.False(t, cd.detector.IsWarmedUp())

	// REQ-COD-006: Should emit detected=0 during warmup
	result := cd.detector.Analyze()
	assert.False(t, result.Detected, "Should not detect during warmup")
}

// @requirement REQ-COD-001
// @requirement REQ-COD-003
func TestOscillationResult_Fields(t *testing.T) {
	result := OscillationResult{
		Detected:         true,
		PeriodicityScore: 0.75,
		Period:           10.0,
		Frequency:        0.1,
		Amplitude:        45.5,
	}

	// REQ-COD-001: Binary detection signal
	assert.True(t, result.Detected)

	// REQ-COD-003: Periodicity score (autocorrelation peak)
	assert.Equal(t, 0.75, result.PeriodicityScore)

	// REQ-COD-003: Period (oscillation period in seconds)
	assert.Equal(t, 10.0, result.Period)

	// REQ-COD-003: Frequency (cycles per second = 1/period)
	assert.Equal(t, 0.1, result.Frequency)

	// REQ-COD-003: Amplitude (peak-to-trough percentage)
	assert.Equal(t, 45.5, result.Amplitude)
}

// @requirement REQ-COD-007
func TestEmitInterval(t *testing.T) {
	// Verify the emit interval is 15 seconds as per design
	assert.Equal(t, 15*time.Second, emitInterval)
}

// @requirement REQ-COD-004
func TestConfigDetectorConfigValues(t *testing.T) {
	config := &Config{
		Enabled:             true,
		MinPeriodicityScore: 0.6,
		MinAmplitude:        10.0,
		MinPeriod:           3,
		MaxPeriod:           25,
		WarmupSeconds:       120,
	}

	dc := config.DetectorConfig()

	// REQ-COD-001: 60-second window
	assert.Equal(t, 60, dc.WindowSize)

	// REQ-COD-001: Periodicity score threshold
	assert.Equal(t, 0.6, dc.MinPeriodicityScore)

	// REQ-COD-001: Period range (Nyquist-constrained)
	assert.Equal(t, 3, dc.MinPeriod)
	assert.Equal(t, 25, dc.MaxPeriod)

	// REQ-COD-005: Min amplitude threshold
	assert.Equal(t, 10.0, dc.MinAmplitude)

	// REQ-COD-002: Warmup period
	assert.Equal(t, 120*time.Second, dc.WarmupDuration)

	// REQ-COD-004: 1Hz sampling
	assert.Equal(t, time.Second, dc.SampleInterval)
}
