// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package modrestart is the module-restart system-probe subcommand
package modrestart

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
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
	moduleRestartCommand := &cobra.Command{
		Use:   "module-restart [module]",
		Short: "Restart a given system-probe module",
		Long:  ``,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.args = args
			return fxutil.OneShot(moduleRestart,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams:         config.NewAgentParams("", config.WithConfigMissingOK(true)),
					SysprobeConfigParams: sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.ConfFilePath)),
					LogParams:            logimpl.ForOneShot("SYS-PROBE", "off", false),
				}),
				// no need to provide sysprobe logger since ForOneShot ignores config values
				core.Bundle(),
			)
		},
	}

	return []*cobra.Command{moduleRestartCommand}
}

func moduleRestart(sysprobeconfig sysprobeconfig.Component, cliParams *cliParams) error {
	cfg := sysprobeconfig.SysProbeObject()
	client := api.GetClient(cfg.SocketAddress)
	url := fmt.Sprintf("http://localhost/module-restart/%s", cliParams.args[0])
	resp, err := client.Post(url, "", nil)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("error restarting module: %s", body)
		return err
	}

	return nil
}
