// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// ProfilingRuntimeSetting wraps operations to change profiling at runtime
type ProfilingRuntimeSetting struct {
	SettingName string
	Service     string

	Config       config.ConfigReaderWriter
	ConfigPrefix string
	source       Source
}

func NewProfilingRuntimeSetting(settingName string, service string) *ProfilingRuntimeSetting {
	return &ProfilingRuntimeSetting{
		SettingName: settingName,
		Service:     service,
		source:      SourceDefault,
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
func (l *ProfilingRuntimeSetting) Get() (interface{}, error) {
	var cfg config.ConfigReaderWriter = config.Datadog
	if l.Config != nil {
		cfg = l.Config
	}
	return cfg.GetBool(l.ConfigPrefix + "internal_profiling.enabled"), nil
}

// Set changes the value of the runtime setting
func (l *ProfilingRuntimeSetting) Set(v interface{}, source Source) error {
	var profile bool
	var err error

	if v, ok := v.(string); ok && strings.ToLower(v) == "restart" {
		if err := l.Set(false, source); err != nil {
			return err
		}
		return l.Set(true, source)
	}

	profile, err = GetBool(v)

	if err != nil {
		return fmt.Errorf("Unsupported type for profile runtime setting: %v", err)
	}

	var cfg config.ConfigReaderWriter = config.Datadog
	if l.Config != nil {
		cfg = l.Config
	}

	if profile {
		// populate site
		s := config.DefaultSite
		if cfg.IsSet(l.ConfigPrefix + "site") {
			s = cfg.GetString(l.ConfigPrefix + "site")
		}

		// allow full url override for development use
		site := fmt.Sprintf(profiling.ProfilingURLTemplate, s)
		if cfg.IsSet(l.ConfigPrefix + "internal_profiling.profile_dd_url") {
			site = cfg.GetString(l.ConfigPrefix + "internal_profiling.profile_dd_url")
		}

		// Note that we must derive a new profiling.Settings on every
		// invocation, as many of these settings may have changed at runtime.
		v, _ := version.Agent()

		tags := cfg.GetStringSlice(l.ConfigPrefix + "internal_profiling.extra_tags")
		tags = append(tags, fmt.Sprintf("version:%v", v))

		settings := profiling.Settings{
			ProfilingURL:         site,
			Socket:               cfg.GetString(l.ConfigPrefix + "internal_profiling.unix_socket"),
			Env:                  cfg.GetString(l.ConfigPrefix + "env"),
			Service:              l.Service,
			Period:               cfg.GetDuration(l.ConfigPrefix + "internal_profiling.period"),
			CPUDuration:          cfg.GetDuration(l.ConfigPrefix + "internal_profiling.cpu_duration"),
			MutexProfileFraction: cfg.GetInt(l.ConfigPrefix + "internal_profiling.mutex_profile_fraction"),
			BlockProfileRate:     cfg.GetInt(l.ConfigPrefix + "internal_profiling.block_profile_rate"),
			WithGoroutineProfile: cfg.GetBool(l.ConfigPrefix + "internal_profiling.enable_goroutine_stacktraces"),
			WithDeltaProfiles:    cfg.GetBool(l.ConfigPrefix + "internal_profiling.delta_profiles"),
			Tags:                 tags,
			CustomAttributes:     cfg.GetStringSlice(l.ConfigPrefix + "internal_profiling.custom_attributes"),
		}
		err := profiling.Start(settings)
		if err == nil {
			cfg.Set(l.ConfigPrefix+"internal_profiling.enabled", true)
		}
	} else {
		profiling.Stop()
		cfg.Set(l.ConfigPrefix+"internal_profiling.enabled", false)
	}

	l.source = source
	return nil
}

func (l *ProfilingRuntimeSetting) GetSource() Source {
	return l.source
}
