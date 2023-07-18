// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package diagnose implements 'agent diagnose'.
package diagnose

import (
	"fmt"
	"os"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/diagnose"
	"github.com/DataDog/datadog-agent/pkg/diagnose/connectivity"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const (
	metadataEndpoint = "/agent/metadata/"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	// noTrace is the value of the --no-trace flag
	noTrace bool

	// payloadName is the name of the payload to display
	payloadName string
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
					ConfigParams: config.NewAgentParamsWithoutSecrets(globalParams.ConfFilePath),
					LogParams:    log.LogForOneShot(command.LoggerName, "info", true)}),
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
					ConfigParams: config.NewAgentParamsWithoutSecrets(globalParams.ConfFilePath),
					LogParams:    log.LogForOneShot(command.LoggerName, "info", true)}),
				core.Bundle,
			)
		},
	}
	diagnoseDatadogConnectivityCommand.PersistentFlags().BoolVarP(&cliParams.noTrace, "no-trace", "", false, "mute extra information about connection establishment, DNS lookup and TLS handshake")

	showPayloadCommand := &cobra.Command{
		Use:   "show-metadata",
		Short: "Print metadata payloads sent by the agent",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Help() //nolint:errcheck
			os.Exit(0)
			return nil
		},
	}

	payloadV5Cmd := &cobra.Command{
		Use:   "v5",
		Short: "Print the metadata payload for the agent.",
		Long: `
This command print the V5 metadata payload for the Agent. This payload is used to populate the infra list and host map in Datadog. It's called 'V5' because it's the same payload sent since Agent V5. This payload is mandatory in order to create a new host in Datadog.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.payloadName = "v5"
			return fxutil.OneShot(printPayload,
				fx.Supply(cliParams),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle,
			)
		},
	}

	payloadInventoriesCmd := &cobra.Command{
		Use:   "inventory",
		Short: "Print the Inventory metadata payload for the agent.",
		Long: `
This command print the last Inventory metadata payload sent by the Agent. This payload is used by the 'inventories/sql' product.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.payloadName = "inventory"
			return fxutil.OneShot(printPayload,
				fx.Supply(cliParams),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle,
			)
		},
	}
	showPayloadCommand.AddCommand(payloadV5Cmd)
	showPayloadCommand.AddCommand(payloadInventoriesCmd)

	diagnoseCommand := &cobra.Command{
		Use:   "diagnose",
		Short: "Check availability of cloud provider and container metadata endpoints",
		Long:  ``,
		RunE:  diagnoseMetadataAvailabilityCommand.RunE, // default to 'diagnose metadata-availability'
	}
	diagnoseCommand.AddCommand(diagnoseMetadataAvailabilityCommand)
	diagnoseCommand.AddCommand(diagnoseDatadogConnectivityCommand)
	diagnoseCommand.AddCommand(showPayloadCommand)

	return []*cobra.Command{diagnoseCommand}
}

func runAll(log log.Component, config config.Component, cliParams *cliParams) error {
	return diagnose.RunAll(color.Output)
}

func runDatadogConnectivityDiagnose(log log.Component, config config.Component, cliParams *cliParams) error {
	return connectivity.RunDatadogConnectivityDiagnose(color.Output, cliParams.noTrace)
}

func printPayload(log log.Component, config config.Component, cliParams *cliParams) error {
	if err := util.SetAuthToken(); err != nil {
		fmt.Println(err)
		return nil
	}

	c := util.GetClient(false)
	ipcAddress, err := pkgconfig.GetIPCAddress()
	if err != nil {
		return err
	}
	apiConfigURL := fmt.Sprintf("https://%v:%d%s%s",
		ipcAddress, config.GetInt("cmd_port"), metadataEndpoint, cliParams.payloadName)

	r, err := util.DoGet(c, apiConfigURL, util.CloseConnection)
	if err != nil {
		return fmt.Errorf("Could not fetch metadata v5 payload: %s", err)
	}

	fmt.Println(string(r))
	return nil
}
