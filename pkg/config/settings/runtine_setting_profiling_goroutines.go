// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// ProfilingGoroutines wraps runtime.SetBlockProfileRate setting
type ProfilingGoroutines struct {
	ConfigPrefix string
	ConfigKey    string
}

// NewProfilingGoroutines returns a new ProfilingGoroutines
func NewProfilingGoroutines() *ProfilingGoroutines {
	return &ProfilingGoroutines{ConfigKey: "internal_profiling_goroutines"}
}

// Name returns the name of the runtime setting
func (r *ProfilingGoroutines) Name() string {
	return r.ConfigKey
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
func (r *ProfilingGoroutines) Get(config config.Component) (interface{}, error) {
	return config.GetBool(r.ConfigPrefix + "internal_profiling.enable_goroutine_stacktraces"), nil
}

// Set changes the value of the runtime setting
func (r *ProfilingGoroutines) Set(config config.Component, value interface{}, source model.Source) error {
	enabled, err := GetBool(value)
	if err != nil {
		return err
	}

	config.Set(r.ConfigPrefix+"internal_profiling.enable_goroutine_stacktraces", enabled, source)

	return nil
}
