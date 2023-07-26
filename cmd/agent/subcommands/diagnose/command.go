// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package diagnose implements 'agent diagnose'.
package diagnose

import (
	"fmt"
	"os"
	"regexp"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	pkgdiagnose "github.com/DataDog/datadog-agent/pkg/diagnose"
	"github.com/DataDog/datadog-agent/pkg/diagnose/connectivity"
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
	forceLocal bool

	// run diagnose on other processes, value of --list flag
	listSuites bool

	// noTrace is the value of the --no-trace flag
	noTrace bool

	// payloadName is the name of the payload to display
	payloadName string

	// diagnose suites to run as a list of regular expressions
	include []string

	// diagnose suites not to run as a list of regular expressions
	exclude []string
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
	diagnoseAllCommand := &cobra.Command{
		Use:   "all",
		Short: "Validate Agent installation, configuration and environment",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			utillog.SetupLogger(seelog.Disabled, "off")
			return fxutil.OneShot(cmdAll,
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
	diagnoseAllCommand.PersistentFlags().BoolVarP(&cliParams.verbose, "verbose", "v", false, "verbose output, includes passed diagnoses, and diagnoses description")

	// List names of all registered diagnose suites. Output also will be filtered if include and or exclude
	// options are specified
	diagnoseAllCommand.PersistentFlags().BoolVarP(&cliParams.listSuites, "list", "t", false, "list diagnose suites")

	// Normally internal diagnose functions will run in the context of agent and other services. It can be
	// overridden via --local options and if specified diagnose functions will be executed in context
	// of the agent diagnose CLI process if possible.
	diagnoseAllCommand.PersistentFlags().BoolVarP(&cliParams.forceLocal, "local", "l", false, "force diagnose execution by the command line instead of the agent process (useful when troubleshooting privilege related problems)")

	// List of regular expressions to include and or exclude names of diagnose suites
	diagnoseAllCommand.PersistentFlags().StringSliceVarP(&cliParams.include, "include", "i", []string{}, "diagnose suites to run as a list of regular expressions")
	diagnoseAllCommand.PersistentFlags().StringSliceVarP(&cliParams.exclude, "exclude", "e", []string{}, "diagnose suites not to run as a list of regular expressions")

	diagnoseMetadataAvailabilityCommand := &cobra.Command{
		Use:   "metadata-availability",
		Short: "Check availability of cloud provider and container metadata endpoints",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(runMetadataAvailability,
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
					// This command loads secrets as it needs the API Key which might be provided via secrets
					ConfigParams: config.NewAgentParamsWithSecrets(globalParams.ConfFilePath),
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
		Short: "Validate Agent installation, configuration and environment",
		Long:  ``,
		RunE:  diagnoseAllCommand.RunE, // default to 'diagnose all'
	}

	diagnoseCommand.AddCommand(diagnoseAllCommand)
	diagnoseCommand.AddCommand(diagnoseMetadataAvailabilityCommand)
	diagnoseCommand.AddCommand(diagnoseDatadogConnectivityCommand)
	diagnoseCommand.AddCommand(showPayloadCommand)

	return []*cobra.Command{diagnoseCommand}
}

func strToRegexList(patterns []string) ([]*regexp.Regexp, error) {
	if len(patterns) > 0 {
		res := make([]*regexp.Regexp, 0)
		for _, pattern := range patterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("failed to compile regex pattern %s: %s", pattern, err.Error())
			}
			res = append(res, re)
		}
		return res, nil
	}
	return nil, nil
}

func cmdAll(log log.Component, config config.Component, cliParams *cliParams) error {
	diagCfg := diagnosis.Config{
		Verbose:    cliParams.verbose,
		ForceLocal: cliParams.forceLocal,
	}

	// prepare include/exclude
	var err error
	if diagCfg.Include, err = strToRegexList(cliParams.include); err != nil {
		return err
	}
	if diagCfg.Exclude, err = strToRegexList(cliParams.exclude); err != nil {
		return err
	}

	// List
	if cliParams.listSuites {
		pkgdiagnose.ListAllStdOut(color.Output, diagCfg)
		return nil
	}

	// Run
	pkgdiagnose.RunAllStdOut(color.Output, diagCfg)
	return nil
}

func runMetadataAvailability(log log.Component, config config.Component, cliParams *cliParams) error {
	return pkgdiagnose.RunMetadataAvail(color.Output)
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
