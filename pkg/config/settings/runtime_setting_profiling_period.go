// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"fmt"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// ProfilingPeriod is a runtime setting that overrides the internal profiling upload period.
type ProfilingPeriod struct {
	ConfigPrefix string
	ConfigKey    string
}

// NewProfilingPeriod returns a new ProfilingPeriod runtime setting.
func NewProfilingPeriod() *ProfilingPeriod {
	return &ProfilingPeriod{ConfigKey: "internal_profiling_period"}
}

// Name returns the name of the runtime setting.
func (r *ProfilingPeriod) Name() string {
	return r.ConfigKey
}

// Description returns the runtime setting's description.
func (r *ProfilingPeriod) Description() string {
	return "This setting overrides the internal profiling upload period (requires profiling restart to take effect: config set internal_profiling restart)"
}

// Hidden returns whether this setting is hidden from the list of runtime settings.
func (r *ProfilingPeriod) Hidden() bool {
	return true
}

// Get returns the current value of the internal profiling period.
func (r *ProfilingPeriod) Get(config config.Component) (interface{}, error) {
	return config.GetDuration(r.ConfigPrefix + "internal_profiling.period"), nil
}

// Set overrides the internal profiling upload period. It accepts a Go duration
// string (e.g. "30s", "2m") or a bare integer interpreted as seconds.
//
// Only the period is written, never cpu_duration: dd-trace-go caps the CPU profile at
// the upload period internally, so lowering the period shortens the CPU profile
// automatically and raising it back restores the full duration. Writing cpu_duration
// here would make that one-way (it would stay shortened after the period is restored).
func (r *ProfilingPeriod) Set(config config.Component, v interface{}, source model.Source) error {
	period, err := parseDuration(v)
	if err != nil {
		return err
	}
	if period <= 0 {
		return fmt.Errorf("internal_profiling_period must be positive, got %v", period)
	}
	if period < time.Second {
		return fmt.Errorf("internal_profiling_period must be at least 1s, got %v", period)
	}

	config.Set(r.ConfigPrefix+"internal_profiling.period", period, source)

	return nil
}

// parseDuration accepts a Go duration string (e.g. "30s", "2m") or a bare integer (seconds).
func parseDuration(v interface{}) (time.Duration, error) {
	switch val := v.(type) {
	case time.Duration:
		return val, nil
	case string:
		// Try Go duration first.
		if d, err := time.ParseDuration(val); err == nil {
			return d, nil
		}
		// Fall back to bare integer (seconds).
		if secs, err := strconv.ParseInt(val, 10, 64); err == nil {
			return time.Duration(secs) * time.Second, nil
		}
		return 0, fmt.Errorf("invalid duration %q: must be a Go duration string (e.g. \"30s\", \"2m\") or a whole number of seconds", val)
	case int:
		return time.Duration(val) * time.Second, nil
	case int64:
		return time.Duration(val) * time.Second, nil
	default:
		return 0, fmt.Errorf("unsupported type %T for internal_profiling_period", v)
	}
}
