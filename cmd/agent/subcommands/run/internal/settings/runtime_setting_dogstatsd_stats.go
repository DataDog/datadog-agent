// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	dogstatsddebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
)

// DsdStatsRuntimeSetting wraps operations to change the collection of dogstatsd stats at runtime.
type DsdStatsRuntimeSetting struct {
	ServerDebug dogstatsddebug.Component
}

// NewDsdStatsRuntimeSetting creates a new instance of DsdStatsRuntimeSetting
func NewDsdStatsRuntimeSetting(serverDebug dogstatsddebug.Component) *DsdStatsRuntimeSetting {
	return &DsdStatsRuntimeSetting{
		ServerDebug: serverDebug,
	}
}

// Description returns the runtime setting's description
func (s *DsdStatsRuntimeSetting) Description() string {
	return "Enable/disable the dogstatsd debug stats. Possible values: true, false"
}

// Hidden returns whether or not this setting is hidden from the list of runtime settings
func (s *DsdStatsRuntimeSetting) Hidden() bool {
	return false
}

// Name returns the name of the runtime setting
func (s *DsdStatsRuntimeSetting) Name() string {
	return string("dogstatsd_stats")
}

// Get returns the current value of the runtime setting
func (s *DsdStatsRuntimeSetting) Get(_ config.Component) (interface{}, error) {
	return s.ServerDebug.IsDebugEnabled(), nil
}

// Set changes the value of the runtime setting
func (s *DsdStatsRuntimeSetting) Set(config config.Component, v interface{}, source model.Source) error {
	var newValue bool
	var err error

	if newValue, err = settings.GetBool(v); err != nil {
		return fmt.Errorf("DsdStatsRuntimeSetting: %v", err)
	}

	s.ServerDebug.SetMetricStatsEnabled(newValue)

	config.Set("dogstatsd_metrics_stats_enable", newValue, source)
	return nil
}
