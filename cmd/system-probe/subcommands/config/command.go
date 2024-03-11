// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config implements 'system-probe config'.
package config

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/client"
	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config/fetcher"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	// args contains the positional command-line arguments
	args []string
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}
	oneShotRunE := func(callback interface{}) func(cmd *cobra.Command, args []string) error {
		return func(cmd *cobra.Command, args []string) error {
			cliParams.args = args
			cliParams.GlobalParams = globalParams

			return fxutil.OneShot(callback,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams:         config.NewAgentParams("", config.WithConfigMissingOK(true)),
					SysprobeConfigParams: sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.ConfFilePath)),
					LogParams:            logimpl.ForOneShot("SYS-PROBE", "off", true),
				}),
				// no need to provide sysprobe logger since ForOneShot ignores config values
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

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list-runtime",
			Short: "List settings that can be changed at runtime",
			Long:  ``,
			RunE:  oneShotRunE(listRuntimeConfigurableValue),
		},
	)

	cmd.AddCommand(
		&cobra.Command{
			Use:   "set [setting] [value]",
			Short: "Set, for the current runtime, the value of a given configuration setting",
			Long:  ``,
			Args:  cobra.ExactArgs(2),
			RunE:  oneShotRunE(setConfigValue),
		},
	)
	cmd.AddCommand(
		&cobra.Command{
			Use:   "get [setting]",
			Short: "Get, for the current runtime, the value of a given configuration setting",
			Long:  ``,
			Args:  cobra.ExactArgs(1),
			RunE:  oneShotRunE(getConfigValue),
		},
	)

	return []*cobra.Command{cmd}
}

func getClient(sysprobeconfig sysprobeconfig.Component) (settings.Client, error) {
	cfg := sysprobeconfig.SysProbeObject()
	hc := client.Get(cfg.SocketAddress)
	return settingshttp.NewClient(hc, "http://localhost/config", "system-probe", settingshttp.NewHTTPClientOptions(util.LeaveConnectionOpen)), nil
}

func showRuntimeConfiguration(sysprobeconfig sysprobeconfig.Component, _ *cliParams) error {
	runtimeConfig, err := fetcher.SystemProbeConfig(sysprobeconfig)
	if err != nil {
		return err
	}

	fmt.Println(runtimeConfig)

	return nil
}

func listRuntimeConfigurableValue(sysprobeconfig sysprobeconfig.Component, _ *cliParams) error {
	c, err := getClient(sysprobeconfig)
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

func setConfigValue(sysprobeconfig sysprobeconfig.Component, cliParams *cliParams) error {
	c, err := getClient(sysprobeconfig)
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

func getConfigValue(sysprobeconfig sysprobeconfig.Component, cliParams *cliParams) error {
	c, err := getClient(sysprobeconfig)
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
