// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package config

import (
	"fmt"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type cliParams struct {
	*command.GlobalParams

	command   *cobra.Command
	args      []string
	getClient settings.ClientBuilder
}

func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Print the runtime configuration of a running agent",
		Long:  ``,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cliParams.command = cmd
			cliParams.args = args
			cliParams.getClient = func(cmd *cobra.Command, args []string) (settings.Client, error) {
				return getSettingsClient(cmd, args)
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(
				showRuntimeConfiguration,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
					LogParams:    log.LogForOneShot(command.LoggerName, "off", true)}),
				core.Bundle,
			)
		},
	}

	// listRuntime returns a cobra command to list the settings that can be changed at runtime.
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list-runtime",
			Short: "List settings that can be changed at runtime",
			Long:  ``,
			RunE: func(cmd *cobra.Command, args []string) error {
				return fxutil.OneShot(
					listRuntimeConfigurableValue,
					fx.Supply(cliParams),
					fx.Supply(core.BundleParams{
						ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
						LogParams:    log.LogForOneShot(command.LoggerName, "off", true)}),
					core.Bundle,
				)
			},
		},
	)

	// set returns a cobra command to set a config value at runtime.
	cmd.AddCommand(
		&cobra.Command{
			Use:   "set [setting] [value]",
			Short: "Set, for the current runtime, the value of a given configuration setting",
			Long:  ``,
			RunE: func(cmd *cobra.Command, args []string) error {
				return fxutil.OneShot(
					setConfigValue,
					fx.Supply(cliParams),
					fx.Supply(core.BundleParams{
						ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
						LogParams:    log.LogForOneShot(command.LoggerName, "off", true)}),
					core.Bundle,
				)
			},
		},
	)

	// get returns a cobra command to get a runtime config value.
	cmd.AddCommand(
		&cobra.Command{
			Use:   "get [setting]",
			Short: "Get, for the current runtime, the value of a given configuration setting",
			Long:  ``,
			RunE: func(cmd *cobra.Command, args []string) error {
				return fxutil.OneShot(
					getConfigValue,
					fx.Supply(cliParams),
					fx.Supply(core.BundleParams{
						ConfigParams: config.NewSecurityAgentParams(globalParams.ConfigFilePaths),
						LogParams:    log.LogForOneShot(command.LoggerName, "off", true)}),
					core.Bundle,
				)
			},
		},
	)

	return []*cobra.Command{cmd}
}
func getSettingsClient(_ *cobra.Command, _ []string) (settings.Client, error) {
	err := util.SetAuthToken()
	if err != nil {
		return nil, err
	}

	c := util.GetClient(false)
	apiConfigURL := fmt.Sprintf("https://localhost:%v/agent/config", pkgconfig.Datadog.GetInt("security_agent.cmd_port"))

	return settingshttp.NewClient(c, apiConfigURL, "security-agent"), nil
}

func showRuntimeConfiguration(log log.Component, config config.Component, params *cliParams) error {
	c, err := params.getClient(params.command, params.args)
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

func setConfigValue(log log.Component, config config.Component, params *cliParams) error {
	if len(params.args) != 2 {
		return fmt.Errorf("exactly two parameters are required: the setting name and its value")
	}

	c, err := params.getClient(params.command, params.args)
	if err != nil {
		return err
	}

	hidden, err := c.Set(params.args[0], params.args[1])
	if err != nil {
		return err
	}

	if hidden {
		fmt.Printf("IMPORTANT: you have modified a hidden option, this may incur billing charges or have other unexpected side-effects.\n")
	}

	fmt.Printf("Configuration setting %s is now set to: %s\n", params.args[0], params.args[1])

	return nil
}

func getConfigValue(log log.Component, config config.Component, params *cliParams) error {
	if len(params.args) != 1 {
		return fmt.Errorf("a single setting name must be specified")
	}

	c, err := params.getClient(params.command, params.args)
	if err != nil {
		return err
	}

	value, err := c.Get(params.args[0])
	if err != nil {
		return err
	}

	fmt.Printf("%s is set to: %v\n", params.args[0], value)

	return nil
}

func listRuntimeConfigurableValue(log log.Component, config config.Component, params *cliParams) error {
	c, err := params.getClient(params.command, params.args)
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
