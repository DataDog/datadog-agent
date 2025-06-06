// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostname implements 'agent hostname'.
package hostname

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	logLevelDefaultOff command.LogLevelDefaultOff
	forceLocal         bool
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}
	getHostnameCommand := &cobra.Command{
		Use:   "hostname",
		Short: "Print the hostname used by the Agent",
		Long:  ``,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(printHostname,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:    log.ForOneShot(command.LoggerName, cliParams.logLevelDefaultOff.Value(), false)}), // never output anything but hostname
				core.Bundle(),
				ipcfx.ModuleInsecure(),
			)
		},
	}
	getHostnameCommand.Flags().BoolVarP(&cliParams.forceLocal, "local", "l", false, "Force computing the hostname in the command line instead of the agent process")

	cliParams.logLevelDefaultOff.Register(getHostnameCommand)
	return []*cobra.Command{getHostnameCommand}
}

func printHostname(_ log.Component, params *cliParams, client ipc.HTTPClient) error {
	hname, err := getHostname(params, client)

	if err != nil {
		return fmt.Errorf("Error getting the hostname: %v", err)
	}

	fmt.Println(hname)
	return nil
}

func getHostname(params *cliParams, client ipc.HTTPClient) (string, error) {
	if !params.forceLocal {
		hname, err := getRemoteHostname(client)
		if err == nil {
			return hname, nil
		}

		// print the warning on stderr to avoid polluting the output
		fmt.Fprintln(os.Stderr, color.RedString("Error getting the hostname from the running agent: %v\nComputing the hostname from the command line...", err))
	}

	return hostname.Get(context.Background())
}

func getRemoteHostname(client ipc.HTTPClient) (string, error) {
	endpoint, err := client.NewIPCEndpoint("/agent/hostname")
	if err != nil {
		return "", err
	}

	hname, err := endpoint.DoGet()
	if err != nil {
		return "", err
	}

	var hostname string
	err = json.Unmarshal(hname, &hostname)
	if err != nil {
		return "", err
	}

	return hostname, nil
}
