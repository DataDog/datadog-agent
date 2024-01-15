// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
)

// RuntimeBlockProfileRate wraps runtime.SetBlockProfileRate setting
type RuntimeBlockProfileRate struct {
	Config       config.ReaderWriter
	ConfigPrefix string
}

// NewRuntimeBlockProfileRate returns a new RuntimeBlockProfileRate
func NewRuntimeBlockProfileRate() *RuntimeBlockProfileRate {
	return &RuntimeBlockProfileRate{}
}

// Name returns the name of the runtime setting
func (r *RuntimeBlockProfileRate) Name() string {
	return "runtime_block_profile_rate"
}

// Description returns the runtime setting's description
func (r *RuntimeBlockProfileRate) Description() string {
	return "This setting controls the fraction of goroutine blocking events that are reported in the internal blocking profile"
}

// Hidden returns whether this setting is hidden from the list of runtime settings
func (r *RuntimeBlockProfileRate) Hidden() bool {
	// Go runtime will start accumulating profile data as soon as this option is set to a
	// non-zero value. There is a risk that left on over a prolonged period of time, it
	// may negatively impact agent performance.
	return true
}

// Get returns the current value of the runtime setting
func (r *RuntimeBlockProfileRate) Get() (interface{}, error) {
	return profiling.GetBlockProfileRate(), nil
}

// Set changes the value of the runtime setting
func (r *RuntimeBlockProfileRate) Set(value interface{}, source model.Source) error {
	rate, err := GetInt(value)
	if err != nil {
		return err
	}

	err = checkProfilingNeedsRestart(profiling.GetBlockProfileRate(), rate)

	profiling.SetBlockProfileRate(rate)
	var cfg config.ReaderWriter = config.Datadog
	if r.Config != nil {
		cfg = r.Config
	}
	cfg.Set(r.ConfigPrefix+"internal_profiling.block_profile_rate", rate, source)

	return err
}
