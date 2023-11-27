// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
)

const configPrefix = config.Namespace + "."

// initRuntimeSettings builds the map of runtime settings configurable at runtime.
func initRuntimeSettings() error {
	// Runtime-editable settings must be registered here to dynamically populate command-line information
	if err := commonsettings.RegisterRuntimeSetting(&commonsettings.LogLevelRuntimeSetting{ConfigKey: configPrefix + "log_level", Config: pkgconfig.SystemProbe}); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(&commonsettings.RuntimeMutexProfileFraction{ConfigPrefix: configPrefix, Config: pkgconfig.SystemProbe}); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(&commonsettings.RuntimeBlockProfileRate{ConfigPrefix: configPrefix, Config: pkgconfig.SystemProbe}); err != nil {
		return err
	}
	profilingGoRoutines := commonsettings.NewProfilingGoroutines()
	profilingGoRoutines.Config = pkgconfig.SystemProbe
	profilingGoRoutines.ConfigPrefix = configPrefix
	if err := commonsettings.RegisterRuntimeSetting(profilingGoRoutines); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(&commonsettings.ProfilingRuntimeSetting{SettingName: "internal_profiling", Service: "system-probe", ConfigPrefix: configPrefix, Config: pkgconfig.SystemProbe}); err != nil {
		return err
	}
	if err := commonsettings.RegisterRuntimeSetting(&commonsettings.ActivityDumpRuntimeSetting{ConfigKey: commonsettings.MaxDumpSizeConfKey}); err != nil {
		return err
	}
	return nil
}
