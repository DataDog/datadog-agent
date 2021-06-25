/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// ProfilingRuntimeSetting wraps operations to change log level at runtime
type ProfilingRuntimeSetting string

// Description returns the runtime setting's description
func (l ProfilingRuntimeSetting) Description() string {
	return "Enable/disable profiling on the agent, valid values are: true, false"
}

// Hidden returns whether or not this setting is hidden from the list of runtime settings
func (l ProfilingRuntimeSetting) Hidden() bool {
	return true
}

// Name returns the name of the runtime setting
func (l ProfilingRuntimeSetting) Name() string {
	return string(l)
}

// Get returns the current value of the runtime setting
func (l ProfilingRuntimeSetting) Get() (interface{}, error) {
	return config.Datadog.GetBool("internal_profiling.enabled"), nil
}

// Set changes the value of the runtime setting
func (l ProfilingRuntimeSetting) Set(v interface{}) error {
	var profile bool
	var err error

	profile, err = GetBool(v)

	if err != nil {
		return fmt.Errorf("Unsupported type for profile runtime setting: %v", err)
	}

	if profile {
		// populate site
		s := config.DefaultSite
		if config.Datadog.IsSet("site") {
			s = config.Datadog.GetString("site")
		}

		// allow full url override for development use
		site := fmt.Sprintf(profiling.ProfileURLTemplate, s)
		if config.Datadog.IsSet("internal_profiling.profile_dd_url") {
			site = config.Datadog.GetString("internal_profiling.profile_dd_url")
		}

		v, _ := version.Agent()
		err := profiling.Start(
			site,
			config.Datadog.GetString("env"),
			profiling.ProfileCoreService,
			profiling.DefaultProfilingPeriod,
			15*time.Second,
			profiling.GetMutexProfileFraction(),
			profiling.GetBlockProfileRate(),
			config.Datadog.GetBool("internal_profiling.enable_goroutine_stacktraces"),
			fmt.Sprintf("version:%v", v),
		)
		if err == nil {
			config.Datadog.Set("internal_profiling.enabled", true)
		}
	} else {
		profiling.Stop()
		config.Datadog.Set("internal_profiling.enabled", false)
	}

	return nil
}
