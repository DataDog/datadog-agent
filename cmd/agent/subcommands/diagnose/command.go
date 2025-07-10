// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package diagnose implements 'agent diagnose'.
package diagnose

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"go.uber.org/fx"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager"
	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager/diagnosesendermanagerimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/comp/core/diagnose/format"
	diagnosefx "github.com/DataDog/datadog-agent/comp/core/diagnose/fx"
	diagnoseLocal "github.com/DataDog/datadog-agent/comp/core/diagnose/local"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	dualTaggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-dual"
	workloadfilterfx "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	haagentfx "github.com/DataDog/datadog-agent/comp/haagent/fx"
	logscompressorfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	metricscompressorfx "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
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

	// JSONOutput will output the diagnosis in JSON format, value of the --json flag
	JSONOutput bool

	// run diagnose on other processes, value of --list flag
	listSuites bool

	// diagnose suites to run as a list of regular expressions
	include []string

	// diagnose suites not to run as a list of regular expressions
	exclude []string

	logLevelDefaultOff command.LogLevelDefaultOff
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
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(cmdDiagnose,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:    log.ForOneShot("CORE", cliParams.logLevelDefaultOff.Value(), true),
				}),
				core.Bundle(),
				// workloadmeta setup
				wmcatalog.GetCatalog(),
				workloadmetafx.Module(workloadmeta.Params{
					AgentType:  workloadmeta.NodeAgent,
					InitHelper: common.GetWorkloadmetaInit(),
				}),
				fx.Supply(option.None[collector.Component]()),
				dualTaggerfx.Module(common.DualTaggerParams()),
				workloadfilterfx.Module(),
				autodiscoveryimpl.Module(),
				diagnosesendermanagerimpl.Module(),
				haagentfx.Module(),
				logscompressorfx.Module(),
				metricscompressorfx.Module(),
				diagnosefx.Module(),
				ipcfx.ModuleInsecure(),
			)
		},
	}

	cliParams.logLevelDefaultOff.Register(diagnoseCommand)

	// Normally a successful diagnosis is printed as a single dot character. If verbose option is specified
	// successful diagnosis is printed fully. With verbose option diagnosis description is also printed.
	diagnoseCommand.PersistentFlags().BoolVarP(&cliParams.verbose, "verbose", "v", false, "verbose output, includes passed diagnoses, and diagnoses description")

	// List names of all registered diagnose suites. Output also will be filtered if include and or exclude
	// options are specified
	diagnoseCommand.PersistentFlags().BoolVarP(&cliParams.listSuites, "list", "t", false, "list diagnose suites")

	// Output the diagnose in JSON format
	diagnoseCommand.PersistentFlags().BoolVarP(&cliParams.JSONOutput, "json", "j", false, "output the diagnose in JSON format")

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
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Help() //nolint:errcheck
			os.Exit(0)
			return nil
		},
	}

	payloadV5Cmd := &cobra.Command{
		Use:   "v5",
		Short: "[internal] Print the metadata payload for the agent.",
		Long: `
This command print the V5 metadata payload for the Agent. This payload is used to populate the infra list and host map in Datadog. It's called 'V5' because it's the same payload sent since Agent V5. This payload is mandatory in order to create a new host in Datadog.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(printPayload,
				fx.Supply(payloadName("v5")),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	payloadGohaiCmd := &cobra.Command{
		Use:   "gohai",
		Short: "[internal] Print the gohai payload for the agent.",
		Long: `
This command prints the gohai data sent by the Agent, including current processes running on the machine.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(printPayload,
				fx.Supply(payloadName("gohai")),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	payloadInventoriesAgentCmd := &cobra.Command{
		Use:   "inventory-agent",
		Short: "[internal] Print the Inventory agent metadata payload.",
		Long: `
This command print the inventory-agent metadata payload. This payload is used by the 'inventories/sql' product.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(printPayload,
				fx.Supply(payloadName("inventory-agent")),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	payloadHostGpuCmd := &cobra.Command{
		Use:   "host-gpu",
		Short: "[internal] Print the host-gpu agent metadata payload.",
		Long: `
This command print the host-gpu metadata payload. This payload is used by the 'host page' product.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(printPayload,
				fx.Supply(payloadName("host-gpu")),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	payloadInventoriesHostCmd := &cobra.Command{
		Use:   "inventory-host",
		Short: "[internal] Print the Inventory host metadata payload.",
		Long: `
This command print the inventory-host metadata payload. This payload is used by the 'inventories/sql' product.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(printPayload,
				fx.Supply(payloadName("inventory-host")),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	payloadInventoriesOtelCmd := &cobra.Command{
		Use:   "inventory-otel",
		Short: "Print the Inventory otel metadata payload.",
		Long: `
This command print the inventory-otel metadata payload. This payload is used by the 'OTel Agent' product.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(printPayload,
				fx.Supply(payloadName("inventory-otel")),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	payloadInventoriesHaAgentCmd := &cobra.Command{
		Use:   "ha-agent",
		Short: "Print the HA Agent Metadata payload.",
		Long: `
This command print the ha-agent metadata payload. This payload is used by the 'HA Agent' feature.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(printPayload,
				fx.Supply(payloadName("ha-agent")),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	payloadInventoriesChecksCmd := &cobra.Command{
		Use:   "inventory-checks",
		Short: "[internal] Print the Inventory checks metadata payload.",
		Long: `
This command print the inventory-checks metadata payload. This payload is used by the 'inventories/sql' product.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(printPayload,
				fx.Supply(payloadName("inventory-checks")),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	payloadInventoriesPkgSigningCmd := &cobra.Command{
		Use:   "package-signing",
		Short: "[internal] Print the Inventory package signing payload.",
		Long: `
This command print the package-signing metadata payload. This payload is used by the 'fleet automation' product.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(printPayload,
				fx.Supply(payloadName("package-signing")),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	payloadSystemProbeCmd := &cobra.Command{
		Use:   "system-probe",
		Short: "[internal] Print the inventory systemprobe metadata payload.",
		Long: `
This command print the system-probe metadata payload. This payload is used by the 'fleet automation' product.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(printPayload,
				fx.Supply(payloadName("system-probe")),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	payloadSecurityAgentCmd := &cobra.Command{
		Use:   "security-agent",
		Short: "[internal] Print the security-agent process metadata payload.",
		Long: `
This command print the security-agent metadata payload. This payload is used by the 'fleet automation' product.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(printPayload,
				fx.Supply(payloadName("security-agent")),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	agentTelemetryCmd := &cobra.Command{
		Use:   "agent-telemetry",
		Short: "[internal] Print agent telemetry payloads sent by the agent.",
		Long:  `.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(printPayload,
				fx.Supply(payloadName("agent-telemetry")),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	agentFullTelemetryCmd := &cobra.Command{
		Use:   "agent-full-telemetry",
		Short: "[internal] Print the full agent telemetry metrics exposed by the agent",
		Long:  `.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(printAgentFullTelemetry,
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	showPayloadCommand.AddCommand(payloadV5Cmd)
	showPayloadCommand.AddCommand(payloadGohaiCmd)
	showPayloadCommand.AddCommand(payloadInventoriesAgentCmd)
	showPayloadCommand.AddCommand(payloadInventoriesHostCmd)
	showPayloadCommand.AddCommand(payloadHostGpuCmd)
	showPayloadCommand.AddCommand(payloadInventoriesOtelCmd)
	showPayloadCommand.AddCommand(payloadInventoriesHaAgentCmd)
	showPayloadCommand.AddCommand(payloadInventoriesChecksCmd)
	showPayloadCommand.AddCommand(payloadInventoriesPkgSigningCmd)
	showPayloadCommand.AddCommand(payloadSystemProbeCmd)
	showPayloadCommand.AddCommand(payloadSecurityAgentCmd)
	showPayloadCommand.AddCommand(agentTelemetryCmd)
	showPayloadCommand.AddCommand(agentFullTelemetryCmd)
	diagnoseCommand.AddCommand(showPayloadCommand)

	return []*cobra.Command{diagnoseCommand}
}

func cmdDiagnose(cliParams *cliParams,
	senderManager diagnosesendermanager.Component,
	wmeta option.Option[workloadmeta.Component],
	ac autodiscovery.Component,
	secretResolver secrets.Component,
	log log.Component,
	tagger tagger.Component,
	diagnoseComponent diagnose.Component,
	config config.Component,
	client ipc.HTTPClient,
) error {

	diagCfg := diagnose.Config{
		Verbose: cliParams.verbose,
		Include: cliParams.include,
		Exclude: cliParams.exclude,
	}
	w := color.Output

	// Is it List command
	if cliParams.listSuites {
		var sortedSuitesName []string
		sortedSuitesName = append(sortedSuitesName, diagnose.AllSuites...)

		sort.Strings(sortedSuitesName)

		fmt.Fprintf(w, "Diagnose suites ...\n")
		for idx, suiteName := range sortedSuitesName {
			fmt.Fprintf(w, "  %d. %s\n", idx+1, suiteName)
		}
		return nil
	}

	// Run command
	var err error
	var result *diagnose.Result
	if !cliParams.runLocal {
		result, err = requestDiagnosesFromAgentProcess(diagCfg, client)

		if err != nil {
			if !cliParams.JSONOutput { // If JSON output is requested, the output should stay a valid JSON
				fmt.Fprintln(w, color.YellowString(fmt.Sprintf("Error running diagnose in Agent process: %s", err)))
				fmt.Fprintln(w, "Running diagnose command locally (may take extra time to run checks locally) ...")
			}
			result, err = diagnoseLocal.Run(diagnoseComponent, diagCfg, log, senderManager, wmeta, ac, secretResolver, tagger, config)
		}
	} else {
		if !cliParams.JSONOutput { // If JSON output is requested, the output should stay a valid JSON
			fmt.Fprintln(w, "Running diagnose command locally (may take extra time to run checks locally) ...")
		}
		result, err = diagnoseLocal.Run(diagnoseComponent, diagCfg, log, senderManager, wmeta, ac, secretResolver, tagger, config)
	}

	if err != nil {
		return err
	}

	if cliParams.JSONOutput {
		return format.JSON(w, result)
	}

	return format.Text(w, diagCfg, result)
}

// NOTE: This and related will be moved to separate "agent telemetry" command in future
func printPayload(name payloadName, _ log.Component, config config.Component, client ipc.HTTPClient) error {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return err
	}
	apiConfigURL := fmt.Sprintf("https://%v:%d%s%s",
		ipcAddress, config.GetInt("cmd_port"), metadataEndpoint, name)

	r, err := client.Get(apiConfigURL, ipchttp.WithCloseConnection)
	if err != nil {
		return fmt.Errorf("Could not fetch metadata payload: %s", err)
	}

	fmt.Println(string(r))
	return nil
}

func requestDiagnosesFromAgentProcess(diagCfg diagnose.Config, client ipc.HTTPClient) (*diagnose.Result, error) {
	// Get client to Agent's RPC call
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, fmt.Errorf("error getting IPC address for the agent: %w", err)
	}

	// Form call end-point
	//nolint:revive // TODO(CINT) Fix revive linter
	diagnoseURL := fmt.Sprintf("https://%v:%v/agent/diagnose", ipcAddress, pkgconfigsetup.Datadog().GetInt("cmd_port"))

	// Serialized diag config to pass it to Agent execution context
	var cfgSer []byte
	if cfgSer, err = json.Marshal(diagCfg); err != nil {
		return nil, fmt.Errorf("error while encoding diagnose configuration: %s", err)
	}

	// Run diagnose code inside Agent process
	var response []byte
	response, err = client.Post(diagnoseURL, "application/json", bytes.NewBuffer(cfgSer))
	if err != nil {
		if response != nil && string(response) != "" {
			return nil, fmt.Errorf("error getting diagnoses from running agent: %s", strings.TrimSpace(string(response)))
		}
		return nil, fmt.Errorf("the agent was unable to get diagnoses from running agent: %w", err)
	}

	// Deserialize results
	var diagnoses diagnose.Result
	err = json.Unmarshal(response, &diagnoses)
	if err != nil {
		return nil, fmt.Errorf("error while decoding diagnose results returned from Agent: %w", err)
	}

	return &diagnoses, nil
}

// printAgentFullTelemetry gets the full telemetry payload exposed by the agent
func printAgentFullTelemetry(config config.Component, client ipc.HTTPClient) error {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(config)
	if err != nil {
		return err
	}
	r, err := client.Get(fmt.Sprintf("http://%s:%s/telemetry", ipcAddress, config.GetString("expvar_port")))
	if err != nil {
		return fmt.Errorf("error getting full telemetry payload: %w", err)
	}
	fmt.Println(string(r))
	return nil
}
