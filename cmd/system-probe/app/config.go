// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	SysprobeCmd.AddCommand(configCommand)
	configCommand.AddCommand(listRuntimeCommand)
	configCommand.AddCommand(setCommand)
	configCommand.AddCommand(getCommand)
}

var (
	configCommand = &cobra.Command{
		Use:   "config",
		Short: "Print the runtime configuration of a running system-probe",
		Long:  ``,
		RunE:  showRuntimeConfiguration,
	}
	listRuntimeCommand = &cobra.Command{
		Use:   "list-runtime",
		Short: "List settings that can be changed at runtime",
		Long:  ``,
		RunE:  listRuntimeConfigurableValue,
	}
	setCommand = &cobra.Command{
		Use:   "set [setting] [value]",
		Short: "Set, for the current runtime, the value of a given configuration setting",
		Long:  ``,
		RunE:  setConfigValue,
	}
	getCommand = &cobra.Command{
		Use:   "get [setting]",
		Short: "Get, for the current runtime, the value of a given configuration setting",
		Long:  ``,
		RunE:  getConfigValue,
	}
)

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

func showRuntimeConfiguration(_ *cobra.Command, _ []string) error {
	c, err := getSettingsClient()
	if err != nil {
		return err
	}
	runtimeConfig, err := c.FullConfig()
	if err != nil {
		return err
	}

	fmt.Println(runtimeConfig)
	return nil
}

func listRuntimeConfigurableValue(_ *cobra.Command, _ []string) error {
	c, err := getSettingsClient()
	if err != nil {
		return err
	}

	settingsList, err := c.List()
	if err != nil {
		return err
	}

	fmt.Println("=== Settings that can be changed at runtime ===")
	for setting, details := range settingsList {
		if !details.Hidden {
			fmt.Printf("%-30s %s\n", setting, details.Description)
		}
	}
	return nil
}

func getSettingsClient() (settings.Client, error) {
	cfg, err := setupConfig()
	if err != nil {
		return nil, err
	}
	hc := api.GetClient(cfg.SocketAddress)
	return settingshttp.NewClient(hc, "http://localhost/config", "system-probe"), nil
}

func setConfigValue(_ *cobra.Command, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("Exactly two parameters are required: the setting name and its value")
	}
	c, err := getSettingsClient()
	if err != nil {
		return err
	}

	hidden, err := c.Set(args[0], args[1])
	if err != nil {
		return err
	}
	if hidden {
		fmt.Printf("IMPORTANT: you have modified a hidden option, this may incur billing charges or have other unexpected side-effects.\n")
	}
	fmt.Printf("Configuration setting %s is now set to: %s\n", args[0], args[1])
	return nil
}

func getConfigValue(_ *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("A single setting name must be specified")
	}

	c, err := getSettingsClient()
	if err != nil {
		return err
	}
	value, err := c.Get(args[0])
	if err != nil {
		return err
	}

	fmt.Printf("%s is set to: %v\n", args[0], value)
	return nil
}

// initRuntimeSettings builds the map of runtime settings configurable at runtime.
func initRuntimeSettings() error {
	// Runtime-editable settings must be registered here to dynamically populate command-line information
	return settings.RegisterRuntimeSetting(settings.LogLevelRuntimeSetting{ConfigKey: config.Namespace + ".log_level"})
}
