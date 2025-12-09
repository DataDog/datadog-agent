// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metric_history

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// Config holds the metric history configuration.
type Config struct {
	Enabled         bool
	IncludePrefixes []string
	RecentDuration  time.Duration
	MediumDuration  time.Duration
	LongDuration    time.Duration
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:         false,
		IncludePrefixes: []string{"system."},
		RecentDuration:  5 * time.Minute,
		MediumDuration:  1 * time.Hour,
		LongDuration:    24 * time.Hour,
	}
}

// LoadConfig reads configuration from the agent config.
func LoadConfig(cfg model.Reader) Config {
	result := DefaultConfig()

	if cfg.IsSet("metric_history.enabled") {
		result.Enabled = cfg.GetBool("metric_history.enabled")
	}

	if cfg.IsSet("metric_history.include_metrics") {
		result.IncludePrefixes = cfg.GetStringSlice("metric_history.include_metrics")
	}

	if cfg.IsSet("metric_history.retention.recent_duration") {
		if duration, err := time.ParseDuration(cfg.GetString("metric_history.retention.recent_duration")); err == nil {
			result.RecentDuration = duration
		}
	}

	if cfg.IsSet("metric_history.retention.medium_duration") {
		if duration, err := time.ParseDuration(cfg.GetString("metric_history.retention.medium_duration")); err == nil {
			result.MediumDuration = duration
		}
	}

	if cfg.IsSet("metric_history.retention.long_duration") {
		if duration, err := time.ParseDuration(cfg.GetString("metric_history.retention.long_duration")); err == nil {
			result.LongDuration = duration
		}
	}

	return result
}

// RecentCapacity returns the number of points to store at flush resolution.
// Assumes 15-second flush interval.
func (c Config) RecentCapacity() int {
	return int(c.RecentDuration / (15 * time.Second))
}

// MediumCapacity returns the number of 1-minute rollup points to store.
func (c Config) MediumCapacity() int {
	return int(c.MediumDuration / time.Minute)
}

// LongCapacity returns the number of 1-hour rollup points to store.
func (c Config) LongCapacity() int {
	return int(c.LongDuration / time.Hour)
}
