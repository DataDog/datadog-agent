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
	Enabled               bool    `yaml:"enabled"`
	AmplitudeMultiplier   float64 `yaml:"amplitude_multiplier"`
	MinAmplitude          float64 `yaml:"min_amplitude"`
	MinDirectionReversals int     `yaml:"min_direction_reversals"`
	WarmupSeconds         int     `yaml:"warmup_seconds"`
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

var (
	// REQ-COD-002: 5 minute warmup period per container
	warmupSecondsRange = &configValueRange{
		min:          60,   // 1 minute minimum
		max:          1800, // 30 minutes maximum
		defaultValue: 300,  // 5 minutes default
	}

	// REQ-COD-001: Amplitude must exceed 4x baseline stddev
	amplitudeMultiplierRange = &configFloatValueRange{
		min:          0.5, // Very sensitive
		max:          10,  // Very insensitive
		defaultValue: 4.0, // Default: swings must exceed 4x baseline stddev
	}

	// REQ-COD-005: Minimum amplitude threshold (absolute floor)
	minAmplitudeRange = &configFloatValueRange{
		min:          0,   // Disabled (no absolute minimum)
		max:          100, // Max possible CPU amplitude
		defaultValue: 0,   // Default: disabled
	}

	// REQ-COD-001: Minimum direction reversals to flag oscillation
	minDirectionReversalsRange = &configValueRange{
		min:          2,  // At least some oscillation
		max:          55, // Max possible in 60s window (theoretical ~30 cycles)
		defaultValue: 6,  // Default: 6 direction reversals
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
	validateFloatValue(&c.AmplitudeMultiplier, amplitudeMultiplierRange)
	validateIntValue(&c.MinDirectionReversals, minDirectionReversalsRange)
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
		WindowSize:            60,                      // 60 samples = 60 seconds at 1Hz
		MinDirectionReversals: c.MinDirectionReversals, // REQ-COD-001: configurable direction change threshold
		AmplitudeMultiplier:   c.AmplitudeMultiplier,   // REQ-COD-001: exceeds 4x baseline stddev
		MinAmplitude:          c.MinAmplitude,          // REQ-COD-005: absolute minimum (0 = disabled)
		DecayFactor:           0.1,                     // Exponential decay alpha
		WarmupDuration:        time.Duration(c.WarmupSeconds) * time.Second,
		SampleInterval:        time.Second, // 1Hz sampling
	}
}
