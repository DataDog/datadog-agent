// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config builds a 'config' command to be used in binaries.
package config

import (
	"fmt"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/spf13/cobra"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	GlobalParams

	// source enables detailed information about each source and its value
	source bool

	// args are the positional command line args
	args []string
}

// GlobalParams contains the values of agent-global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	ConfFilePath   string
	ConfigName     string
	LoggerName     string
	SettingsClient func() (settings.Client, error)
}

// MakeCommand returns a `config` command to be used by agent binaries.
func MakeCommand(globalParamsGetter func() GlobalParams) *cobra.Command {
	cliParams := &cliParams{}
	// All subcommands use the same provided components, with a different
	// oneShot callback.
	oneShotRunE := func(callback interface{}) func(cmd *cobra.Command, args []string) error {
		return func(cmd *cobra.Command, args []string) error {
			globalParams := globalParamsGetter()

			cliParams.args = args
			cliParams.GlobalParams = globalParams

			return fxutil.OneShot(callback,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithConfigName(globalParams.ConfigName)),
					LogParams:    logimpl.ForOneShot(globalParams.LoggerName, "off", true)}),
				core.Bundle(),
			)
		}
	}
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Print the runtime configuration of a running agent",
		Long:  ``,
		RunE:  oneShotRunE(showRuntimeConfiguration),
	}

	listRuntimeCmd := &cobra.Command{
		Use:   "list-runtime",
		Short: "List settings that can be changed at runtime",
		Long:  ``,
		RunE:  oneShotRunE(listRuntimeConfigurableValue),
	}
	cmd.AddCommand(listRuntimeCmd)

	setCmd := &cobra.Command{
		Use:   "set [setting] [value]",
		Short: "Set, for the current runtime, the value of a given configuration setting",
		Long:  ``,
		RunE:  oneShotRunE(setConfigValue),
	}
	cmd.AddCommand(setCmd)

	getCmd := &cobra.Command{
		Use:   "get [setting]",
		Short: "Get, for the current runtime, the value of a given configuration setting",
		Long:  ``,
		RunE:  oneShotRunE(getConfigValue),
	}
	cmd.AddCommand(getCmd)
	getCmd.Flags().BoolVarP(&cliParams.source, "source", "s", false, "print every source and its value")

	return cmd
}

func showRuntimeConfiguration(_ log.Component, config config.Component, cliParams *cliParams) error {
	err := util.SetAuthToken(config)
	if err != nil {
		return err
	}

	c, err := cliParams.GlobalParams.SettingsClient()
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

func listRuntimeConfigurableValue(_ log.Component, config config.Component, cliParams *cliParams) error {
	err := util.SetAuthToken(config)
	if err != nil {
		return err
	}

	c, err := cliParams.GlobalParams.SettingsClient()
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

func setConfigValue(_ log.Component, config config.Component, cliParams *cliParams) error {
	if len(cliParams.args) != 2 {
		return fmt.Errorf("exactly two parameters are required: the setting name and its value")
	}

	err := util.SetAuthToken(config)
	if err != nil {
		return err
	}

	c, err := cliParams.GlobalParams.SettingsClient()
	if err != nil {
		return err
	}

	hidden, err := c.Set(cliParams.args[0], cliParams.args[1])
	if err != nil {
		return err
	}

	if hidden {
		fmt.Printf("IMPORTANT: you have modified a hidden option, this may incur billing charges or have other unexpected side-effects.\n")
	}

	fmt.Printf("Configuration setting %s is now set to: %s\n", cliParams.args[0], cliParams.args[1])

	return nil
}

func getConfigValue(_ log.Component, config config.Component, cliParams *cliParams) error {
	if len(cliParams.args) != 1 {
		return fmt.Errorf("a single setting name must be specified")
	}

	err := util.SetAuthToken(config)
	if err != nil {
		return err
	}

	c, err := cliParams.GlobalParams.SettingsClient()
	if err != nil {
		return err
	}

	resp, err := c.GetWithSources(cliParams.args[0])
	if err != nil {
		return err
	}

	fmt.Printf("%s is set to: %v\n", cliParams.args[0], resp["value"])

	if cliParams.source {
		sourcesVal, ok := resp["sources_value"].([]interface{})
		if !ok {
			return fmt.Errorf("failed to cast sources_value to []map[interface{}]interface{}")
		}

		fmt.Printf("sources and their value:\n")
		for _, sourceVal := range sourcesVal {
			sourceVal, ok := sourceVal.(map[string]interface{})
			if !ok {
				return fmt.Errorf("failed to cast sourceVal to map[string]interface{}")
			}
			fmt.Printf("  %s: %v\n", sourceVal["Source"], sourceVal["Value"])
		}
	}

	return nil
}
