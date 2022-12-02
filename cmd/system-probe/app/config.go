// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	cmdconfig "github.com/DataDog/datadog-agent/cmd/system-probe/commands/config"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
)

func init() {
	SysprobeCmd.AddCommand(cmdconfig.Config(getSettingsClient))
}

func setupConfig() (*config.Config, error) {
	if flagNoColor {
		color.NoColor = true
	}

	cfg, err := config.New(configPath)
	if err != nil {
		return nil, fmt.Errorf("unable to set up system-probe configuration: %v", err)
	}

	err = ddconfig.SetupLogger(loggerName, ddconfig.GetEnvDefault("DD_LOG_LEVEL", "off"), "", "", false, true, false)
	if err != nil {
		fmt.Printf("Cannot setup logger, exiting: %v\n", err)
		return nil, err
	}

	return cfg, nil
}

func getSettingsClient(cmd *cobra.Command, _ []string) (settings.Client, error) {
	cfg, err := setupConfig()
	if err != nil {
		return nil, err
	}
	hc := api.GetClient(cfg.SocketAddress)
	return settingshttp.NewClient(hc, "http://localhost/config", "system-probe"), nil
}

// initRuntimeSettings builds the map of runtime settings configurable at runtime.
func initRuntimeSettings() error {
	// Runtime-editable settings must be registered here to dynamically populate command-line information
	err := settings.RegisterRuntimeSetting(settings.LogLevelRuntimeSetting{ConfigKey: config.Namespace + ".log_level"})
	if err != nil {
		return err
	}

	err = settings.RegisterRuntimeSetting(settings.ActivityDumpRuntimeSetting{ConfigKey: settings.MaxDumpSizeConfKey})
	if err != nil {
		return err
	}

	return nil
}
