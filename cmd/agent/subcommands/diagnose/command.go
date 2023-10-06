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
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	pkgdiagnose "github.com/DataDog/datadog-agent/pkg/diagnose"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	utillog "github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/cihub/seelog"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const (
	metadataEndpoint = "/agent/metadata/"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	// verbose will show details not only failed diagnosis but also succesfull diagnosis
	// it is the, value of the --verbose flag
	verbose bool

	// run diagnose in the context of CLI process instead of running in the context of agent service irunni, value of the --local flag
	runLocal bool

	// run diagnose on other processes, value of --list flag
	listSuites bool

	// diagnose suites to run as a list of regular expressions
	include []string

	// diagnose suites not to run as a list of regular expressions
	exclude []string

	// payloadName is the name of the payload to display
	payloadName string
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	// From the CLI standpoint most of the changes are covered by the “agent diagnose all” sub-command.
	// Other, previous sub-commands left AS IS for now for compatibility reasons. But further changes
	// are possible, e.g. removal of all sub-commands and using command-line options to fine-tune
	// diagnose depth, breadth and output format. Suggestions are welcome.
	diagnoseCommand := &cobra.Command{
		Use:   "diagnose",
		Short: "Validate Agent installation, configuration and environment",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			utillog.SetupLogger(seelog.Disabled, "off")
			return fxutil.OneShot(cmdDiagnose,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParamsWithoutSecrets(globalParams.ConfFilePath),
					LogParams:    log.LogForOneShot("CORE", "off", true)}),
				core.Bundle,
			)
		},
	}

	// Normally a successful diagnosis is printed as a single dot character. If verbose option is specified
	// successful diagnosis is printed fully. With verbose option diagnosis description is also printed.
	diagnoseCommand.PersistentFlags().BoolVarP(&cliParams.verbose, "verbose", "v", false, "verbose output, includes passed diagnoses, and diagnoses description")

	// List names of all registered diagnose suites. Output also will be filtered if include and or exclude
	// options are specified
	diagnoseCommand.PersistentFlags().BoolVarP(&cliParams.listSuites, "list", "t", false, "list diagnose suites")

	// Normally internal diagnose functions will run in the context of agent and other services. It can be
	// overridden via --local options and if specified diagnose functions will be executed in context
	// of the agent diagnose CLI process if possible.
	diagnoseCommand.PersistentFlags().BoolVarP(&cliParams.runLocal, "local", "o", false, "run diagnose by the CLI process instead of the agent process (useful to troubleshooting privilege related problems)")

	// List of regular expressions to include and or exclude names of diagnose suites
	diagnoseCommand.PersistentFlags().StringSliceVarP(&cliParams.include, "include", "i", []string{}, "diagnose suites to run as a list of regular expressions")
	diagnoseCommand.PersistentFlags().StringSliceVarP(&cliParams.exclude, "exclude", "e", []string{}, "diagnose suites not to run as a list of regular expressions")

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
	diagnoseCommand.AddCommand(showPayloadCommand)

	return []*cobra.Command{diagnoseCommand}
}

func cmdDiagnose(log log.Component, config config.Component, cliParams *cliParams) error {
	diagCfg := diagnosis.Config{
		Verbose:  cliParams.verbose,
		RunLocal: cliParams.runLocal,
		Include:  cliParams.include,
		Exclude:  cliParams.exclude,
	}

	// Is it List command
	if cliParams.listSuites {
		pkgdiagnose.ListStdOut(color.Output, diagCfg)
		return nil
	}

	// Run command
	return pkgdiagnose.RunStdOut(color.Output, diagCfg, aggregator.GetSenderManager())
}

// NOTE: This and related will be moved to separate "agent telemetry" command in future
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
