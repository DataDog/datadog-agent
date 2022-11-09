// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config implements 'agent config'.
package config

import (
	"fmt"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/spf13/cobra"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	// args are the positional command line args
	args []string
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}
	// All subcommands use the same provided components, with a different
	// oneShot callback.
	oneShotRunE := func(callback interface{}) func(cmd *cobra.Command, args []string) error {
		return func(cmd *cobra.Command, args []string) error {
			cliParams.args = args
			return fxutil.OneShot(callback,
				fx.Supply(cliParams),
				fx.Supply(core.CreateAgentBundleParams(globalParams.ConfFilePath, false).LogForOneShot("CORE", "off", true)),
				core.Bundle,
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

	return []*cobra.Command{cmd}
}

func showRuntimeConfiguration(log log.Component, config config.Component, cliParams *cliParams) error {
	err := util.SetAuthToken()
	if err != nil {
		return err
	}

	c, err := common.NewSettingsClient()
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

func listRuntimeConfigurableValue(log log.Component, config config.Component, cliParams *cliParams) error {
	err := util.SetAuthToken()
	if err != nil {
		return err
	}

	c, err := common.NewSettingsClient()
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

func setConfigValue(log log.Component, config config.Component, cliParams *cliParams) error {
	if len(cliParams.args) != 2 {
		return fmt.Errorf("exactly two parameters are required: the setting name and its value")
	}

	err := util.SetAuthToken()
	if err != nil {
		return err
	}

	c, err := common.NewSettingsClient()
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

func getConfigValue(log log.Component, config config.Component, cliParams *cliParams) error {
	if len(cliParams.args) != 1 {
		return fmt.Errorf("a single setting name must be specified")
	}

	err := util.SetAuthToken()
	if err != nil {
		return err
	}

	c, err := common.NewSettingsClient()
	if err != nil {
		return err
	}

	value, err := c.Get(cliParams.args[0])
	if err != nil {
		return err
	}

	fmt.Printf("%s is set to: %v\n", cliParams.args[0], value)

	return nil
}
