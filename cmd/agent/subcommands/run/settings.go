// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands/run/internal/settings"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
)

// initRuntimeSettings builds the map of runtime settings configurable at runtime.
func initRuntimeSettings() error {
	// Runtime-editable settings must be registered here to dynamically populate command-line information
	if err := commonsettings.RegisterRuntimeSetting(commonsettings.LogLevelRuntimeSetting{}); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(commonsettings.RuntimeMutexProfileFraction("runtime_mutex_profile_fraction")); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(commonsettings.RuntimeBlockProfileRate("runtime_block_profile_rate")); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(settings.DsdStatsRuntimeSetting("dogstatsd_stats")); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(settings.DsdCaptureDurationRuntimeSetting("dogstatsd_capture_duration")); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(commonsettings.LogPayloadsRuntimeSetting{}); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(commonsettings.ProfilingGoroutines("internal_profiling_goroutines")); err != nil {
		return err
	}
	return commonsettings.RegisterRuntimeSetting(commonsettings.ProfilingRuntimeSetting("internal_profiling"))
}
