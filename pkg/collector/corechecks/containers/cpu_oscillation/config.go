// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package cpuoscillation implements the per-container CPU oscillation detection check.
// REQ-COD-005: Configurable Detection with Default Disabled
package cpuoscillation

import (
	"time"

	"gopkg.in/yaml.v2"
)

// Config holds the cpu_oscillation check configuration
// REQ-COD-005: Configurable Detection with Default Disabled
type Config struct {
	Enabled             bool    `yaml:"enabled"`
	MinAmplitude        float64 `yaml:"min_amplitude"`         // Minimum CPU% swing to detect
	MinPeriodicityScore float64 `yaml:"min_periodicity_score"` // Minimum autocorrelation peak (0.0-1.0)
	MinPeriod           int     `yaml:"min_period"`            // Minimum period in seconds
	MaxPeriod           int     `yaml:"max_period"`            // Maximum period in seconds
	WarmupSeconds       int     `yaml:"warmup_seconds"`
}

// configValueRange defines valid ranges and defaults for config values
type configValueRange struct {
	min          int
	max          int
	defaultValue int
}

// configFloatValueRange defines valid ranges and defaults for float config values
type configFloatValueRange struct {
	min          float64
	max          float64
	defaultValue float64
}

// Nyquist constraints for autocorrelation-based detection:
// - Sample interval: 1 second (1 Hz)
// - Window size: 60 samples (60 seconds)
// - Nyquist limit: minimum detectable period = 2 × sample interval = 2 seconds
// - Autocorrelation needs at least 2 full cycles: max period = window size / 2 = 30 seconds
const (
	sampleIntervalSeconds = 1
	windowSize            = 60
	nyquistMinPeriod      = 2 * sampleIntervalSeconds // 2 seconds (Nyquist limit)
	maxPeriodLimit        = windowSize / 2            // 30 seconds (ensures 2 full cycles)
)

var (
	// REQ-COD-002: Warmup period per container (reduced for autocorrelation)
	warmupSecondsRange = &configValueRange{
		min:          30,  // 30 seconds minimum
		max:          300, // 5 minutes maximum
		defaultValue: 60,  // 1 minute default
	}

	// REQ-COD-005: Minimum amplitude threshold (absolute floor)
	minAmplitudeRange = &configFloatValueRange{
		min:          0,    // Disabled (no absolute minimum)
		max:          100,  // Max possible CPU amplitude
		defaultValue: 10.0, // Default: 10% CPU swing required
	}

	// REQ-COD-001: Minimum periodicity score (autocorrelation peak)
	minPeriodicityScoreRange = &configFloatValueRange{
		min:          0.1,  // Very sensitive (catches weak patterns)
		max:          0.95, // Very strict (only strong patterns)
		defaultValue: 0.5,  // Default: moderate periodicity required
	}

	// REQ-COD-001: Minimum period in seconds (Nyquist-constrained)
	// Hard floor of 2 seconds due to 1Hz sampling (Nyquist theorem)
	minPeriodRange = &configValueRange{
		min:          nyquistMinPeriod,   // 2 seconds (Nyquist limit, not configurable below)
		max:          maxPeriodLimit - 1, // Must be less than max period
		defaultValue: nyquistMinPeriod,   // Default: 2 seconds
	}

	// REQ-COD-001: Maximum period in seconds
	// Hard ceiling ensures at least 2 full cycles fit in 60s window
	maxPeriodRange = &configValueRange{
		min:          nyquistMinPeriod + 1, // Must be greater than min period
		max:          maxPeriodLimit,       // 30 seconds (ensures 2 cycles in window)
		defaultValue: maxPeriodLimit,       // Default: 30 seconds
	}
)

func validateIntValue(val *int, valueRange *configValueRange) {
	if *val == 0 {
		*val = valueRange.defaultValue
	} else if *val < valueRange.min {
		*val = valueRange.min
	} else if *val > valueRange.max {
		*val = valueRange.max
	}
}

func validateFloatValue(val *float64, valueRange *configFloatValueRange) {
	if *val == 0 {
		*val = valueRange.defaultValue
	} else if *val < valueRange.min {
		*val = valueRange.min
	} else if *val > valueRange.max {
		*val = valueRange.max
	}
}

// Parse parses the configuration from raw YAML bytes
func (c *Config) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, c); err != nil {
		return err
	}

	// REQ-COD-005: Default disabled - Enabled field defaults to false (zero value)

	validateIntValue(&c.WarmupSeconds, warmupSecondsRange)
	validateFloatValue(&c.MinPeriodicityScore, minPeriodicityScoreRange)
	validateIntValue(&c.MinPeriod, minPeriodRange)
	validateIntValue(&c.MaxPeriod, maxPeriodRange)

	// Ensure MaxPeriod > MinPeriod (Nyquist constraint enforcement)
	if c.MaxPeriod <= c.MinPeriod {
		c.MaxPeriod = c.MinPeriod + 1
		if c.MaxPeriod > maxPeriodLimit {
			// If we can't satisfy the constraint, use defaults
			c.MinPeriod = minPeriodRange.defaultValue
			c.MaxPeriod = maxPeriodRange.defaultValue
		}
	}

	// MinAmplitude: 0 means disabled, so we only clamp to max if > 100
	if c.MinAmplitude < minAmplitudeRange.min {
		c.MinAmplitude = minAmplitudeRange.min
	} else if c.MinAmplitude > minAmplitudeRange.max {
		c.MinAmplitude = minAmplitudeRange.max
	}

	return nil
}

// DetectorConfig returns an OscillationConfig suitable for the detector
func (c *Config) DetectorConfig() OscillationConfig {
	return OscillationConfig{
		WindowSize:          windowSize,            // 60 samples = 60 seconds at 1Hz
		MinPeriodicityScore: c.MinPeriodicityScore, // REQ-COD-001: autocorrelation threshold
		MinAmplitude:        c.MinAmplitude,        // REQ-COD-005: absolute minimum (0 = disabled)
		MinPeriod:           c.MinPeriod,           // REQ-COD-001: minimum period (Nyquist-constrained)
		MaxPeriod:           c.MaxPeriod,           // REQ-COD-001: maximum period
		WarmupDuration:      time.Duration(c.WarmupSeconds) * time.Second,
		SampleInterval:      time.Second, // 1Hz sampling
	}
}
