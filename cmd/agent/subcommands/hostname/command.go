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

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
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
			return fxutil.OneShot(getHostname,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:    log.ForOneShot(command.LoggerName, cliParams.logLevelDefaultOff.Value(), false)}), // never output anything but hostname
				core.Bundle(),
			)
		},
	}
	getHostnameCommand.Flags().BoolVarP(&cliParams.forceLocal, "local", "l", false, "Force computing the hostname in the command line instead of the agent process")

	cliParams.logLevelDefaultOff.Register(getHostnameCommand)
	return []*cobra.Command{getHostnameCommand}
}

func getHostname(_ log.Component, config config.Component, params *cliParams) error {
	var hname string
	var err error

	if !params.forceLocal {
		hname, err = getRemoteHostname(config)
		if err != nil {
			// print the warning on stderr to avoid polluting the output
			fmt.Fprintf(os.Stderr, "Error getting the hostname from the running agent: %v\nComputing the hostname from the command line...\n", err)
		}
	}

	if hname == "" {
		hname, err = hostname.Get(context.Background())
		if err != nil {
			return fmt.Errorf("Error getting the hostname: %v", err)
		}
	}

	fmt.Println(hname)
	return nil
}

func getRemoteHostname(config config.Component) (string, error) {
	endpoint, err := apiutil.NewIPCEndpoint(config, "/agent/hostname")
	if err != nil {
		return "", err
	}

	res, err := endpoint.DoGet()
	if err != nil {
		return "", err
	}

	var hname string
	err = json.Unmarshal(res, &hname)
	if err != nil {
		return "", err
	}

	return hname, nil
}
