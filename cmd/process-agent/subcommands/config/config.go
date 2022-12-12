// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	"github.com/DataDog/datadog-agent/pkg/process/config"
)

// Commands returns a slice of subcommands for the `config` command in the Process Agent
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Print the runtime configuration of a running agent",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return showRuntimeConfiguration(globalParams)
		},
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list-runtime",
			Short: "List settings that can be changed at runtime",
			Long:  ``,
			RunE: func(cmd *cobra.Command, args []string) error {
				return listRuntimeConfigurableValue(globalParams)
			},
		},
	)

	cmd.AddCommand(
		&cobra.Command{
			Use:   "set [setting] [value]",
			Short: "Set, for the current runtime, the value of a given configuration setting",
			Long:  ``,
			RunE: func(cmd *cobra.Command, args []string) error {
				return setConfigValue(globalParams, args)
			},
		},
	)
	cmd.AddCommand(
		&cobra.Command{
			Use:   "get [setting]",
			Short: "Get, for the current runtime, the value of a given configuration setting",
			Long:  ``,
			RunE: func(cmd *cobra.Command, args []string) error {
				return getConfigValue(globalParams, args)
			},
		},
	)

	return []*cobra.Command{cmd}
}

func showRuntimeConfiguration(globalParams *command.GlobalParams) error {
	c, err := getClient(globalParams)
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

func listRuntimeConfigurableValue(globalParams *command.GlobalParams) error {
	c, err := getClient(globalParams)
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

func setConfigValue(globalParams *command.GlobalParams, args []string) error {
	c, err := getClient(globalParams)
	if err != nil {
		return err
	}

	if len(args) != 2 {
		return fmt.Errorf("exactly two parameters are required: the setting name and its value")
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

func getConfigValue(globalParams *command.GlobalParams, args []string) error {
	c, err := getClient(globalParams)
	if err != nil {
		return err
	}

	if len(args) != 1 {
		return fmt.Errorf("a single setting name must be specified")
	}

	value, err := c.Get(args[0])
	if err != nil {
		return err
	}

	fmt.Printf("%s is set to: %v\n", args[0], value)

	return nil
}

func getClient(globalParams *command.GlobalParams) (settings.Client, error) {
	// Set up the config so we can get the port later
	// We set this up differently from the main process-agent because this way is quieter
	cfg := config.NewDefaultAgentConfig()
	if globalParams.ConfFilePath != "" {
		if err := config.LoadConfigIfExists(globalParams.ConfFilePath); err != nil {
			return nil, err
		}
	}
	err := cfg.LoadAgentConfig(globalParams.ConfFilePath)
	if err != nil {
		return nil, err
	}

	httpClient := apiutil.GetClient(false)
	ipcAddress, err := ddconfig.GetIPCAddress()

	port := ddconfig.Datadog.GetInt("process_config.cmd_port")
	if port <= 0 {
		return nil, fmt.Errorf("invalid process_config.cmd_port -- %d", port)
	}

	ipcAddressWithPort := fmt.Sprintf("http://%s:%d/config", ipcAddress, port)
	if err != nil {
		return nil, err
	}
	settingsClient := settingshttp.NewClient(httpClient, ipcAddressWithPort, "process-agent")
	return settingsClient, nil
}
