// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux || darwin

// Package oscillation implements the CPU oscillation detection check.
package oscillation

import (
	"time"

	"gopkg.in/yaml.v2"
)

// Config holds the cpu_oscillation check configuration
type Config struct {
	AmplitudeMultiplier float64 `yaml:"amplitude_multiplier"`
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

var (
	warmupSecondsRange = &configValueRange{
		min:          60,   // 1 minute minimum
		max:          1800, // 30 minutes maximum
		defaultValue: 300,  // 5 minutes default
	}

	amplitudeMultiplierRange = &configFloatValueRange{
		min:          0.5, // Very sensitive
		max:          10,  // Very insensitive
		defaultValue: 2.0, // Default: swings must exceed 2x baseline stddev
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

	validateIntValue(&c.WarmupSeconds, warmupSecondsRange)
	validateFloatValue(&c.AmplitudeMultiplier, amplitudeMultiplierRange)

	return nil
}

// DetectorConfig returns an OscillationConfig suitable for the detector
func (c *Config) DetectorConfig() OscillationConfig {
	return OscillationConfig{
		WindowSize:          60,                    // 60 samples = 60 seconds at 1Hz
		MinZeroCrossings:    6,                     // Minimum direction changes to flag
		AmplitudeMultiplier: c.AmplitudeMultiplier, // From config
		DecayFactor:         0.1,                   // Exponential decay alpha
		WarmupDuration:      time.Duration(c.WarmupSeconds) * time.Second,
		SampleInterval:      time.Second, // 1Hz sampling
	}
}
