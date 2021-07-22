// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/app/settings"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/commands"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"

	"github.com/fatih/color"
)

func init() {
	AgentCmd.AddCommand(commands.Config(getSettingsClient))
}

func setupConfig() error {
	if flagNoColor {
		color.NoColor = true
	}

	err := common.SetupConfigWithoutSecrets(confFilePath, "")
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}

	err = config.SetupLogger(loggerName, config.GetEnvDefault("DD_LOG_LEVEL", "off"), "", "", false, true, false)
	if err != nil {
		fmt.Printf("Cannot setup logger, exiting: %v\n", err)
		return err
	}

	return util.SetAuthToken()
}

func getSettingsClient() (commonsettings.Client, error) {
	err := setupConfig()
	if err != nil {
		return nil, err
	}
	return common.NewSettingsClient()
}

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
