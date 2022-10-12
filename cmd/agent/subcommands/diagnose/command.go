// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package diagnose implements 'agent diagnose'.
package diagnose

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/diagnose"
	"github.com/DataDog/datadog-agent/pkg/diagnose/connectivity"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	// noTrace is the value of the --no-trace flag
	noTrace bool
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	diagnoseMetadataAvailabilityCommand := &cobra.Command{
		Use:   "metadata-availability",
		Short: "Check availability of cloud provider and container metadata endpoints",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(runAll,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfFilePath:      globalParams.ConfFilePath,
					ConfigLoadSecrets: false,
				}.LogForOneShot("CORE", "info", true)),
				core.Bundle,
			)
		},
	}

	diagnoseDatadogConnectivityCommand := &cobra.Command{
		Use:   "datadog-connectivity",
		Short: "Check connectivity between your system and Datadog endpoints",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(runDatadogConnectivityDiagnose,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfFilePath:      globalParams.ConfFilePath,
					ConfigLoadSecrets: false,
				}.LogForOneShot("CORE", "info", true)),
				core.Bundle,
			)
		},
	}
	diagnoseDatadogConnectivityCommand.PersistentFlags().BoolVarP(&cliParams.noTrace, "no-trace", "", false, "mute extra information about connection establishment, DNS lookup and TLS handshake")

	diagnoseCommand := &cobra.Command{
		Use:   "diagnose",
		Short: "Check availability of cloud provider and container metadata endpoints",
		Long:  ``,
		RunE:  diagnoseMetadataAvailabilityCommand.RunE, // default to 'diagnose metadata-availability'
	}
	diagnoseCommand.AddCommand(diagnoseMetadataAvailabilityCommand)
	diagnoseCommand.AddCommand(diagnoseDatadogConnectivityCommand)

	return []*cobra.Command{diagnoseCommand}
}

func runAll(log log.Component, config config.Component, cliParams *cliParams) error {
	return diagnose.RunAll(color.Output)
}

func runDatadogConnectivityDiagnose(log log.Component, config config.Component, cliParams *cliParams) error {
	return connectivity.RunDatadogConnectivityDiagnose(color.Output, cliParams.noTrace)
}
