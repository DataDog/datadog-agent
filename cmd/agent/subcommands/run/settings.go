// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands/run/internal/settings"
	dogstatsddebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
)

// initRuntimeSettings builds the map of runtime settings configurable at runtime.
func initRuntimeSettings(serverDebug dogstatsddebug.Component) error {
	// Runtime-editable settings must be registered here to dynamically populate command-line information
	if err := commonsettings.RegisterRuntimeSetting(commonsettings.NewLogLevelRuntimeSetting()); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(commonsettings.NewRuntimeMutexProfileFraction()); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(commonsettings.NewRuntimeBlockProfileRate()); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(settings.NewDsdStatsRuntimeSetting(serverDebug)); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(settings.NewDsdCaptureDurationRuntimeSetting("dogstatsd_capture_duration")); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(
		settings.NewHAMRRuntimeSetting("ha.enabled", "Enable/disable the HA region subsystem."),
	); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(
		settings.NewHAMRRuntimeSetting("ha.failover", "Enable/disable the HA region failover; enabled submits to the secondary site."),
	); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(commonsettings.NewLogPayloadsRuntimeSetting()); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(commonsettings.NewProfilingGoroutines()); err != nil {
		return err
	}
	return commonsettings.RegisterRuntimeSetting(commonsettings.NewProfilingRuntimeSetting("internal_profiling", "datadog-agent"))
}
