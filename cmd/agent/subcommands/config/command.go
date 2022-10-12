// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config implements 'agent config'.
package config

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"

	"github.com/spf13/cobra"
)

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	// utility function to set up logging and config, shared between subcommands
	setupConfigAndLogs := func() error {
		err := common.SetupConfigWithoutSecrets(globalParams.ConfFilePath, "")
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}

		err = config.SetupLogger(config.CoreLoggerName, config.GetEnvDefault("DD_LOG_LEVEL", "off"), "", "", false, true, false)
		if err != nil {
			fmt.Printf("Cannot setup logger, exiting: %v\n", err)
			return err
		}

		err = util.SetAuthToken()
		if err != nil {
			fmt.Printf("Cannot setup auth token, exiting: %v\n", err)
			return err
		}

		return nil
	}

	getClient := func(_ *cobra.Command, _ []string) (settings.Client, error) {
		return common.NewSettingsClient()
	}

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Print the runtime configuration of a running agent",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setupConfigAndLogs(); err != nil {
				return err
			}
			return showRuntimeConfiguration(getClient, cmd, args)
		},
	}

	listRuntimeCmd := &cobra.Command{
		Use:   "list-runtime",
		Short: "List settings that can be changed at runtime",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setupConfigAndLogs(); err != nil {
				return err
			}
			return listRuntimeConfigurableValue(getClient, cmd, args)
		},
	}
	cmd.AddCommand(listRuntimeCmd)

	setCmd := &cobra.Command{
		Use:   "set [setting] [value]",
		Short: "Set, for the current runtime, the value of a given configuration setting",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setupConfigAndLogs(); err != nil {
				return err
			}
			return setConfigValue(getClient, cmd, args)
		},
	}
	cmd.AddCommand(setCmd)

	getCmd := &cobra.Command{
		Use:   "get [setting]",
		Short: "Get, for the current runtime, the value of a given configuration setting",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setupConfigAndLogs(); err != nil {
				return err
			}
			return getConfigValue(getClient, cmd, args)
		},
	}
	cmd.AddCommand(getCmd)

	return []*cobra.Command{cmd}
}

func showRuntimeConfiguration(getClient settings.ClientBuilder, cmd *cobra.Command, args []string) error {
	c, err := getClient(cmd, args)
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

func listRuntimeConfigurableValue(getClient settings.ClientBuilder, cmd *cobra.Command, args []string) error {
	c, err := getClient(cmd, args)
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

func setConfigValue(getClient settings.ClientBuilder, cmd *cobra.Command, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("exactly two parameters are required: the setting name and its value")
	}

	c, err := getClient(cmd, args)
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

func getConfigValue(getClient settings.ClientBuilder, cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("a single setting name must be specified")
	}

	c, err := getClient(cmd, args)
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
