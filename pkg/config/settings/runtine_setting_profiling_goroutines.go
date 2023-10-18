// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"github.com/DataDog/datadog-agent/pkg/config"
)

// ProfilingGoroutines wraps runtime.SetBlockProfileRate setting
type ProfilingGoroutines struct {
	Config       config.ReaderWriter
	ConfigPrefix string
	source       Source
}

// NewProfilingGoroutines returns a new ProfilingGoroutines
func NewProfilingGoroutines() *ProfilingGoroutines {
	return &ProfilingGoroutines{source: SourceDefault}
}

// Name returns the name of the runtime setting
func (r *ProfilingGoroutines) Name() string {
	return "internal_profiling_goroutines"
}

// Description returns the runtime setting's description
func (r *ProfilingGoroutines) Description() string {
	return "This setting controls whether internal profiling will collect goroutine stacktraces (requires profiling restart)"
}

// Hidden returns whether this setting is hidden from the list of runtime settings
func (r *ProfilingGoroutines) Hidden() bool {
	return true
}

// Get returns the current value of the runtime setting
func (r *ProfilingGoroutines) Get() (interface{}, error) {
	var cfg config.ReaderWriter = config.Datadog
	if r.Config != nil {
		cfg = r.Config
	}
	return cfg.GetBool(r.ConfigPrefix + "internal_profiling.enable_goroutine_stacktraces"), nil
}

// Set changes the value of the runtime setting
func (r *ProfilingGoroutines) Set(value interface{}, source Source) error {
	enabled, err := GetBool(value)
	if err != nil {
		return err
	}

	var cfg config.ReaderWriter = config.Datadog
	if r.Config != nil {
		cfg = r.Config
	}
	cfg.Set(r.ConfigPrefix+"internal_profiling.enable_goroutine_stacktraces", enabled)
	r.source = source

	return nil
}

// GetSource returns the source of the ProfilingGoroutines
func (r *ProfilingGoroutines) GetSource() Source {
	return r.source
}
