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
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager"
	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager/diagnosesendermanagerimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/diagnose"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	utillog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

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
}

// payloadName is the name of the payload to display
type payloadName string

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
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath),
					LogParams:    logimpl.ForOneShot("CORE", "off", true)}),
				core.Bundle(),
				// workloadmeta setup
				collectors.GetCatalog(),
				fx.Provide(func() workloadmeta.Params {
					return workloadmeta.Params{
						AgentType:  workloadmeta.NodeAgent,
						InitHelper: common.GetWorkloadmetaInit(),
						NoInstance: !cliParams.runLocal,
					}
				}),
				fx.Supply(optional.NewNoneOption[collector.Component]()),
				workloadmeta.OptionalModule(),
				tagger.OptionalModule(),
				autodiscoveryimpl.OptionalModule(),
				compressionimpl.Module(),
				diagnosesendermanagerimpl.Module(),
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
			return fxutil.OneShot(printPayload,
				fx.Supply(payloadName("v5")),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
			)
		},
	}

	payloadGohaiCmd := &cobra.Command{
		Use:   "gohai",
		Short: "Print the gohai payload for the agent.",
		Long: `
This command prints the gohai data sent by the Agent, including current processes running on the machine.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(printPayload,
				fx.Supply(payloadName("gohai")),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
			)
		},
	}

	payloadInventoriesAgentCmd := &cobra.Command{
		Use:   "inventory-agent",
		Short: "Print the Inventory agent metadata payload.",
		Long: `
This command print the inventory-agent metadata payload. This payload is used by the 'inventories/sql' product.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(printPayload,
				fx.Supply(payloadName("inventory-agent")),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
			)
		},
	}

	payloadInventoriesHostCmd := &cobra.Command{
		Use:   "inventory-host",
		Short: "Print the Inventory host metadata payload.",
		Long: `
This command print the inventory-host metadata payload. This payload is used by the 'inventories/sql' product.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(printPayload,
				fx.Supply(payloadName("inventory-host")),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
			)
		},
	}

	payloadInventoriesChecksCmd := &cobra.Command{
		Use:   "inventory-checks",
		Short: "Print the Inventory checks metadata payload.",
		Long: `
This command print the inventory-checks metadata payload. This payload is used by the 'inventories/sql' product.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(printPayload,
				fx.Supply(payloadName("inventory-checks")),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
			)
		},
	}

	payloadInventoriesPkgSigningCmd := &cobra.Command{
		Use:   "package-signing",
		Short: "Print the Inventory package signing payload.",
		Long: `
This command print the package-signing metadata payload. This payload is used by the 'fleet automation' product.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(printPayload,
				fx.Supply(payloadName("package-signing")),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
			)
		},
	}

	showPayloadCommand.AddCommand(payloadV5Cmd)
	showPayloadCommand.AddCommand(payloadGohaiCmd)
	showPayloadCommand.AddCommand(payloadInventoriesAgentCmd)
	showPayloadCommand.AddCommand(payloadInventoriesHostCmd)
	showPayloadCommand.AddCommand(payloadInventoriesChecksCmd)
	showPayloadCommand.AddCommand(payloadInventoriesPkgSigningCmd)
	diagnoseCommand.AddCommand(showPayloadCommand)

	return []*cobra.Command{diagnoseCommand}
}

func cmdDiagnose(cliParams *cliParams,
	senderManager diagnosesendermanager.Component,
	wmeta optional.Option[workloadmeta.Component],
	_ optional.Option[tagger.Component],
	ac optional.Option[autodiscovery.Component],
	collector optional.Option[collector.Component],
	secretResolver secrets.Component) error {
	diagCfg := diagnosis.Config{
		Verbose:  cliParams.verbose,
		RunLocal: cliParams.runLocal,
		Include:  cliParams.include,
		Exclude:  cliParams.exclude,
	}

	diagnoseDeps := diagnose.NewSuitesDeps(senderManager, collector, secretResolver, wmeta, ac)
	// Is it List command
	if cliParams.listSuites {
		diagnose.ListStdOut(color.Output, diagCfg, diagnoseDeps)
		return nil
	}

	// Run command
	return diagnose.RunStdOut(color.Output, diagCfg, diagnoseDeps)
}

// NOTE: This and related will be moved to separate "agent telemetry" command in future
func printPayload(name payloadName, _ log.Component, config config.Component) error {
	if err := util.SetAuthToken(config); err != nil {
		fmt.Println(err)
		return nil
	}

	c := util.GetClient(false)
	ipcAddress, err := pkgconfig.GetIPCAddress()
	if err != nil {
		return err
	}
	apiConfigURL := fmt.Sprintf("https://%v:%d%s%s",
		ipcAddress, config.GetInt("cmd_port"), metadataEndpoint, name)

	r, err := util.DoGet(c, apiConfigURL, util.CloseConnection)
	if err != nil {
		return fmt.Errorf("Could not fetch metadata v5 payload: %s", err)
	}

	fmt.Println(string(r))
	return nil
}
