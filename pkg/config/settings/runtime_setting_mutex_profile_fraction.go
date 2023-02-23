// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
)

// RuntimeMutexProfileFraction wraps runtime.SetMutexProfileFraction setting.
type RuntimeMutexProfileFraction struct {
	Config       config.ConfigReaderWriter
	ConfigPrefix string
}

// Name returns the name of the runtime setting
func (r RuntimeMutexProfileFraction) Name() string {
	return "runtime_mutex_profile_fraction"
}

// Description returns the runtime setting's description
func (r RuntimeMutexProfileFraction) Description() string {
	return "This setting controls the fraction of mutex contention events that are reported in the internal mutex profile"
}

// Hidden returns whether or not this setting is hidden from the list of runtime settings
func (r RuntimeMutexProfileFraction) Hidden() bool {
	// Go runtime will start accumulating profile data as soon as this option is set to a
	// non-zero value. There is a risk that left on over a prolonged period of time, it
	// may negatively impact agent performance.
	return true
}

// Get returns the current value of the runtime setting
func (r RuntimeMutexProfileFraction) Get() (interface{}, error) {
	return profiling.GetMutexProfileFraction(), nil
}

// Set changes the value of the runtime setting
func (r RuntimeMutexProfileFraction) Set(value interface{}) error {
	rate, err := GetInt(value)
	if err != nil {
		return err
	}

	err = checkProfilingNeedsRestart(profiling.GetMutexProfileFraction(), rate)

	profiling.SetMutexProfileFraction(rate)
	var cfg config.ConfigReaderWriter = config.Datadog
	if r.Config != nil {
		cfg = r.Config
	}
	cfg.Set(r.ConfigPrefix+"internal_profiling.mutex_profile_fraction", rate)

	return err
}
