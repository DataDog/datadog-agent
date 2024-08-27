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
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	verbose bool
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	configCheckCommand := &cobra.Command{
		Use:     "configcheck",
		Aliases: []string{"checkconfig"},
		Short:   "Print all configurations loaded & resolved of a running agent",
		Long:    ``,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(run,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(cliParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(cliParams.FleetPoliciesDirPath)),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    log.ForOneShot("CORE", "off", true)}),
				core.Bundle(),
			)
		},
	}
	configCheckCommand.Flags().BoolVarP(&cliParams.verbose, "verbose", "v", false, "print additional debug info")

	return []*cobra.Command{configCheckCommand}
}

func run(config config.Component, cliParams *cliParams, _ log.Component) error {
	endpoint, err := apiutil.NewIPCEndpoint(config, "/agent/config-check")
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
