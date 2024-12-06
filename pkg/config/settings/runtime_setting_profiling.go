// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// ProfilingRuntimeSetting wraps operations to change profiling at runtime
type ProfilingRuntimeSetting struct {
	SettingName string
	Service     string

	ConfigPrefix string
}

// NewProfilingRuntimeSetting returns a new ProfilingRuntimeSetting
func NewProfilingRuntimeSetting(settingName string, service string) *ProfilingRuntimeSetting {
	return &ProfilingRuntimeSetting{
		SettingName: settingName,
		Service:     service,
	}
}

// Description returns the runtime setting's description
func (l *ProfilingRuntimeSetting) Description() string {
	return "Enable/disable profiling on the agent, valid values are: true, false, restart"
}

// Hidden returns whether this setting is hidden from the list of runtime settings
func (l *ProfilingRuntimeSetting) Hidden() bool {
	return true
}

// Name returns the name of the runtime setting
func (l *ProfilingRuntimeSetting) Name() string {
	return l.SettingName
}

// Get returns the current value of the runtime setting
func (l *ProfilingRuntimeSetting) Get(config config.Component) (interface{}, error) {
	return config.GetBool(l.ConfigPrefix + "internal_profiling.enabled"), nil
}

// Set changes the value of the runtime setting
func (l *ProfilingRuntimeSetting) Set(config config.Component, v interface{}, source model.Source) error {
	var profile bool
	var err error

	if v, ok := v.(string); ok && strings.ToLower(v) == "restart" {
		if err := l.Set(config, false, source); err != nil {
			return err
		}
		return l.Set(config, true, source)
	}

	profile, err = GetBool(v)

	if err != nil {
		return fmt.Errorf("Unsupported type for profile runtime setting: %v", err)
	}

	if profile {
		// populate site
		s := pkgconfigsetup.DefaultSite
		if config.IsSet(l.ConfigPrefix + "site") {
			s = config.GetString(l.ConfigPrefix + "site")
		}

		// allow full url override for development use
		site := fmt.Sprintf(profiling.ProfilingURLTemplate, s)
		if config.IsSet(l.ConfigPrefix + "internal_profiling.profile_dd_url") {
			site = config.GetString(l.ConfigPrefix + "internal_profiling.profile_dd_url")
		}

		// Note that we must derive a new profiling.Settings on every
		// invocation, as many of these settings may have changed at runtime.

		tags := config.GetStringSlice(l.ConfigPrefix + "internal_profiling.extra_tags")
		tags = append(tags, fmt.Sprintf("version:%v", version.AgentVersion))

		settings := profiling.Settings{
			ProfilingURL:         site,
			Socket:               config.GetString(l.ConfigPrefix + "internal_profiling.unix_socket"),
			Env:                  config.GetString(l.ConfigPrefix + "env"),
			Service:              l.Service,
			Period:               config.GetDuration(l.ConfigPrefix + "internal_profiling.period"),
			CPUDuration:          config.GetDuration(l.ConfigPrefix + "internal_profiling.cpu_duration"),
			MutexProfileFraction: config.GetInt(l.ConfigPrefix + "internal_profiling.mutex_profile_fraction"),
			BlockProfileRate:     config.GetInt(l.ConfigPrefix + "internal_profiling.block_profile_rate"),
			WithGoroutineProfile: config.GetBool(l.ConfigPrefix + "internal_profiling.enable_goroutine_stacktraces"),
			WithBlockProfile:     config.GetBool(l.ConfigPrefix + "internal_profiling.enable_block_profiling"),
			WithMutexProfile:     config.GetBool(l.ConfigPrefix + "internal_profiling.enable_mutex_profiling"),
			WithDeltaProfiles:    config.GetBool(l.ConfigPrefix + "internal_profiling.delta_profiles"),
			Tags:                 tags,
			CustomAttributes:     config.GetStringSlice(l.ConfigPrefix + "internal_profiling.custom_attributes"),
		}
		err := profiling.Start(settings)
		if err == nil {
			config.Set(l.ConfigPrefix+"internal_profiling.enabled", true, source)
		}
	} else {
		profiling.Stop()
		config.Set(l.ConfigPrefix+"internal_profiling.enabled", false, source)
	}

	return nil
}
