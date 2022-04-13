// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config/settings"

	"github.com/spf13/cobra"
)

// Config returns the main cobra config command.
func Config(getClient settings.ClientBuilder) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Print the runtime configuration of a running agent",
		Long:  ``,
		RunE:  func(cmd *cobra.Command, args []string) error { return showRuntimeConfiguration(getClient, cmd, args) },
	}

	cmd.AddCommand(listRuntime(getClient))
	cmd.AddCommand(set(getClient))
	cmd.AddCommand(get(getClient))

	return cmd
}

// listRuntime returns a cobra command to list the settings that can be changed at runtime.
func listRuntime(getClient settings.ClientBuilder) *cobra.Command {
	return &cobra.Command{
		Use:   "list-runtime",
		Short: "List settings that can be changed at runtime",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listRuntimeConfigurableValue(getClient, cmd, args)
		},
	}
}

// set returns a cobra command to set a config value at runtime.
func set(getClient settings.ClientBuilder) *cobra.Command {
	return &cobra.Command{
		Use:   "set [setting] [value]",
		Short: "Set, for the current runtime, the value of a given configuration setting",
		Long:  ``,
		RunE:  func(cmd *cobra.Command, args []string) error { return setConfigValue(getClient, cmd, args) },
	}
}

// get returns a cobra command to get a runtime config value.
func get(getClient settings.ClientBuilder) *cobra.Command {
	return &cobra.Command{
		Use:   "get [setting]",
		Short: "Get, for the current runtime, the value of a given configuration setting",
		Long:  ``,
		RunE:  func(cmd *cobra.Command, args []string) error { return getConfigValue(getClient, cmd, args) },
	}
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
