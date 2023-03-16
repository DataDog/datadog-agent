// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands/run/internal/settings"
	dogstatsdDebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
)

// initRuntimeSettings builds the map of runtime settings configurable at runtime.
func initRuntimeSettings(serverDebug dogstatsdDebug.Component) error {
	// Runtime-editable settings must be registered here to dynamically populate command-line information
	if err := commonsettings.RegisterRuntimeSetting(commonsettings.LogLevelRuntimeSetting{}); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(commonsettings.RuntimeMutexProfileFraction{}); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(commonsettings.RuntimeBlockProfileRate{}); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(settings.NewDsdStatsRuntimeSetting(serverDebug)); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(settings.DsdCaptureDurationRuntimeSetting("dogstatsd_capture_duration")); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(commonsettings.LogPayloadsRuntimeSetting{}); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(commonsettings.ProfilingGoroutines{}); err != nil {
		return err
	}
	return commonsettings.RegisterRuntimeSetting(commonsettings.ProfilingRuntimeSetting{SettingName: "internal_profiling", Service: "datadog-agent"})
}
