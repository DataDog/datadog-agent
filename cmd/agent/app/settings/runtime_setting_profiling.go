/// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package settings

import (
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// profilingRuntimeSetting wraps operations to change log level at runtime
type profilingRuntimeSetting string

func (l profilingRuntimeSetting) Description() string {
	return "Enable/disable profiling on the agent, valid values are: true, false"
}

func (l profilingRuntimeSetting) Name() string {
	return string(l)
}

func (l profilingRuntimeSetting) Get() (interface{}, error) {
	return config.Datadog.GetBool("profiling.enabled"), nil
}

func (l profilingRuntimeSetting) Set(v interface{}) error {
	var profile bool
	var err error

	switch p := v.(type) {
	case bool:
		profile = p
	case string:
		profile, err = strconv.ParseBool(p)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("Unsupported type for profile runtime setting")
	}

	if profile {
		// populate site
		s := config.DefaultSite
		if config.Datadog.IsSet("site") {
			s = config.Datadog.GetString("site")
		}

		// allow full url override for development use
		site := fmt.Sprintf(profiling.ProfileURLTemplate, s)
		if config.Datadog.IsSet("profiling.profile_dd_url") {
			site = config.Datadog.GetString("profiling.profile_dd_url")
		}

		v, _ := version.Agent()
		err := profiling.Start(
			config.Datadog.GetString("api_key"),
			site,
			config.Datadog.GetString("env"),
			profiling.ProfileCoreService,
			fmt.Sprintf("version:%v", v),
		)
		if err == nil {
			config.Datadog.Set("profiling.enabled", true)
		}
	} else {
		profiling.Stop()
		config.Datadog.Set("profiling.enabled", false)
	}

	return nil
}
