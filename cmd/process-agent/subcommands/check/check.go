// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package check

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors"
	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/utils"
	processComponent "github.com/DataDog/datadog-agent/comp/process"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const defaultWaitInterval = time.Second

type cliParams struct {
	*command.GlobalParams
	checkName       string
	checkOutputJSON bool
	waitInterval    time.Duration
}

type dependencies struct {
	fx.In

	CliParams *cliParams

	Config   config.Component
	Syscfg   sysprobeconfig.Component
	Log      log.Component
	Hostinfo hostinfo.Component
	// TODO: the tagger is used by the ContainerProvider, which is currently not a component so there is no direct
	// dependency on it. The ContainerProvider needs to be componentized so it can be injected and have fx manage its
	// lifecycle.
	Tagger       tagger.Component
	WorkloadMeta workloadmeta.Component
	Checks       []types.CheckComponent `group:"check"`
}

func nextGroupID() func() int32 {
	var groupID int32
	return func() int32 {
		groupID++
		return groupID
	}
}

// Commands returns a slice of subcommands for the `check` command in the Process Agent
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	desc := "Run a specific check and print the results. Choose from: process, rtprocess, container, rtcontainer, connections, process_discovery, process_events"
	return []*cobra.Command{MakeCommand(globalParams, "check", desc)}
}

func MakeCommand(globalParams *command.GlobalParams, name string, short string) *cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	checkCmd := &cobra.Command{
		Use:   name,
		Short: short,

		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.checkName = args[0]

			if !strings.Contains(short, cliParams.checkName) {
				return fmt.Errorf("invalid check '%s'", cliParams.checkName)
			}

			bundleParams := command.GetCoreBundleParamsForOneShot(globalParams)

			// Disable logging if `--json` is specified. This way the check command will output proper json.
			if cliParams.checkOutputJSON {
				bundleParams.LogParams = logimpl.ForOneShot(string(command.LoggerName), "off", true)
			}

			return fxutil.OneShot(RunCheckCmd,
				fx.Supply(cliParams, bundleParams),
				core.Bundle(),
				// Provide workloadmeta module
				workloadmeta.Module(),
				// Provide the corresponding workloadmeta Params to configure the catalog
				collectors.GetCatalog(),
				fx.Provide(func(config config.Component) workloadmeta.Params {

					var catalog workloadmeta.AgentType
					if config.GetBool("process_config.remote_workloadmeta") {
						catalog = workloadmeta.Remote
					} else {
						catalog = workloadmeta.ProcessAgent
					}

					return workloadmeta.Params{AgentType: catalog}
				}),

				// Provide tagger module
				tagger.Module(),
				// Tagger must be initialized after agent config has been setup
				fx.Provide(func(c config.Component) tagger.Params {
					if c.GetBool("process_config.remote_tagger") {
						return tagger.NewNodeRemoteTaggerParams()
					}
					return tagger.NewTaggerParams()
				}),
				processComponent.Bundle(),
			)
		},
		SilenceUsage: true,
	}

	checkCmd.Flags().BoolVar(&cliParams.checkOutputJSON, "json", false, "Output check results in JSON")
	checkCmd.Flags().DurationVarP(&cliParams.waitInterval, "wait", "w", defaultWaitInterval, "How long to wait before running the check")

	return checkCmd
}

func RunCheckCmd(deps dependencies) error {
	command.SetHostMountEnv(deps.Log)

	// Now that the logger is configured log host info
	deps.Log.Infof("running on platform: %s", hostMetadataUtils.GetPlatformName())
	agentVersion, _ := version.Agent()
	deps.Log.Infof("running version: %s", agentVersion.GetNumberAndPre())

	cleanups := make([]func(), 0)
	defer func() {
		for _, c := range cleanups {
			c()
		}
	}()

	names := make([]string, 0, len(deps.Checks))
	for _, checkComponent := range deps.Checks {
		ch := checkComponent.Object()

		names = append(names, ch.Name())

		_, processModuleEnabled := deps.Syscfg.SysProbeObject().EnabledModules[sysconfig.ProcessModule]
		cfg := &checks.SysProbeConfig{
			MaxConnsPerMessage:   deps.Syscfg.SysProbeObject().MaxConnsPerMessage,
			SystemProbeAddress:   deps.Syscfg.SysProbeObject().SocketAddress,
			ProcessModuleEnabled: processModuleEnabled,
		}

		if !matchingCheck(deps.CliParams.checkName, ch) {
			continue
		}

		if err := ch.Init(cfg, deps.Hostinfo.Object(), true); err != nil {
			return err
		}

		cleanups = append(cleanups, ch.Cleanup)
		return runCheck(deps.Log, deps.CliParams, ch)
	}
	return deps.Log.Errorf("invalid check '%s', choose from: %v", deps.CliParams.checkName, names)
}

func matchingCheck(checkName string, ch checks.Check) bool {
	if ch.SupportsRunOptions() {
		if checks.RTName(ch.Name()) == checkName {
			return true
		}
	}

	return ch.Name() == checkName
}

func runCheck(log log.Component, cliParams *cliParams, ch checks.Check) error {
	nextGroupID := nextGroupID()

	options := &checks.RunOptions{
		RunStandard: true,
	}

	if cliParams.checkName == checks.RTName(ch.Name()) {
		options.RunRealtime = true
	}

	// We need to run the check twice in order to initialize the stats
	// Rate calculations rely on having two datapoints
	if _, err := ch.Run(nextGroupID, options); err != nil {
		return fmt.Errorf("collection error: %s", err)
	}

	log.Infof("Waiting %s before running the check", cliParams.waitInterval.String())
	time.Sleep(cliParams.waitInterval)

	if !cliParams.checkOutputJSON {
		printResultsBanner(cliParams.checkName)
	}

	result, err := ch.Run(nextGroupID, options)
	if err != nil {
		return fmt.Errorf("collection error: %s", err)
	}

	var msgs []process.MessageBody

	switch {
	case result == nil:
		break
	case options != nil && options.RunRealtime:
		msgs = result.RealtimePayloads()
	default:
		msgs = result.Payloads()
	}

	return printResults(cliParams.checkName, msgs, cliParams.checkOutputJSON)
}

func printResultsBanner(name string) {
	fmt.Printf("-----------------------------\n\n")
	fmt.Printf("\nResults for check %s\n", name)
	fmt.Printf("-----------------------------\n\n")
}

func printResults(check string, msgs []process.MessageBody, checkOutputJSON bool) error {
	if checkOutputJSON {
		return printResultsJSON(msgs)
	}

	err := checks.HumanFormat(check, msgs, os.Stdout)
	switch err {
	case checks.ErrNoHumanFormat:
		fmt.Println(color.YellowString("Printing output in JSON format for %s\n", check))
		return printResultsJSON(msgs)
	default:
		return err
	}
}

func printResultsJSON(msgs []process.MessageBody) error {
	for _, m := range msgs {
		b, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal error: %s", err)
		}
		fmt.Println(string(b))
	}
	return nil
}
