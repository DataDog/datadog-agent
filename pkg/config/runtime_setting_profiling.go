/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package config

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// profilingRuntimeSetting wraps operations to change log level at runtime
type profilingRuntimeSetting string

func (l profilingRuntimeSetting) Description() string {
	return "Enable/disable profiling on the agent, valid values are: enabled, disabled, true, false, on or off"
}

func (l profilingRuntimeSetting) Name() string {
	return string(l)
}

func (l profilingRuntimeSetting) Get() (interface{}, error) {
	return Datadog.GetBool("profiling.enabled"), nil
}

func (l profilingRuntimeSetting) Set(v interface{}) error {
	profile := v.(bool)
	if profile {
		// populate site
		s := DefaultSite
		if Datadog.IsSet("site") {
			s = Datadog.GetString("site")
		}

		// allow full url override for development use
		site := fmt.Sprintf(profiling.ProfileURLTemplate, s)
		if Datadog.IsSet("profiling.profile_dd_url") {
			site = Datadog.GetString("profiling.profile_dd_url")
		}

		v, _ := version.Agent()
		err := profiling.Start(
			Datadog.GetString("api_key"),
			site,
			Datadog.GetString("env"),
			Datadog.GetString("service"),
			fmt.Sprintf("version:%v", v),
		)
		if err == nil {
			Datadog.Set("profiling.enabled", true)
		}
	} else {
		profiling.Stop()
		Datadog.Set("profiling.enabled", false)
	}

	return nil
}
