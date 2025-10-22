// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package configcheck implements 'agent configcheck'.
package configcheck

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	// args are the positional command-line arguments
	args []string

	verbose bool
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	configCheckCommand := &cobra.Command{
		Use:     "configcheck [check]",
		Aliases: []string{"checkconfig"},
		Short:   "Print all configurations loaded & resolved of a running agent",
		Long:    ``,
		RunE: func(_ *cobra.Command, args []string) error {
			cliParams.args = args

			return fxutil.OneShot(run,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(cliParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(cliParams.FleetPoliciesDirPath)),
					LogParams:    log.ForOneShot("CORE", "off", true)}),
				secretfx.Module(),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}
	configCheckCommand.Flags().BoolVarP(&cliParams.verbose, "verbose", "v", false, "print additional debug info")

	return []*cobra.Command{configCheckCommand}
}

func run(cliParams *cliParams, log log.Component, client ipc.HTTPClient) error {
	if len(cliParams.args) < 1 {
		return fullConfigCmd(cliParams, log, client)
	}

	return checkCmd(cliParams, log, client)
}

func fullConfigCmd(cliParams *cliParams, _ log.Component, client ipc.HTTPClient) error {
	endpoint, err := client.NewIPCEndpoint("/agent/config-check")
	if err != nil {
		return err
	}

	res, err := endpoint.DoGet()
	if err != nil {
		return fmt.Errorf("the agent ran into an error while checking config: %v", err)
	}

	cr := integration.ConfigCheckResponse{}
	err = json.Unmarshal(res, &cr)
	if err != nil {
		return fmt.Errorf("unable to parse configcheck: %v", err)
	}

	var b bytes.Buffer
	color.Output = &b

	flare.PrintConfigCheck(color.Output, cr, cliParams.verbose)

	fmt.Println(b.String())
	return nil
}

func checkCmd(cliParams *cliParams, _ log.Component, client ipc.HTTPClient) error {
	if len(cliParams.args) > 1 {
		return fmt.Errorf("only one check must be specified")
	}

	endpoint, err := client.NewIPCEndpoint("/agent/config-check")
	if err != nil {
		return err
	}

	res, err := endpoint.DoGet()
	if err != nil {
		return fmt.Errorf("the agent ran into an error while checking config: %v", err)
	}

	cr := integration.ConfigCheckResponse{}
	err = json.Unmarshal(res, &cr)
	if err != nil {
		return fmt.Errorf("unable to parse configcheck: %v", err)
	}

	// search through the configs for a check with the same name
	for _, configResponse := range cr.Configs {
		if cliParams.args[0] == configResponse.Config.Name {
			var b bytes.Buffer
			color.Output = &b

			flare.PrintConfigWithInstanceIDs(color.Output, configResponse.Config, configResponse.InstanceIDs, "")

			fmt.Println(b.String())
			return nil
		}
	}

	// return an error if the name wasn't found in the checks list
	return fmt.Errorf("no check with the name %q was found", cliParams.args[0])
}
