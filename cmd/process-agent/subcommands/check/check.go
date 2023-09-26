// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/utils"
	processComponent "github.com/DataDog/datadog-agent/comp/process"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/tagger/remote"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
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
	Checks   []types.CheckComponent `group:"check"`
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
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "Run a specific check and print the results. Choose from: process, rtprocess, container, rtcontainer, connections, process_discovery, process_events",

		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.checkName = args[0]

			bundleParams := command.GetCoreBundleParamsForOneShot(globalParams)

			// Disable logging if `--json` is specified. This way the check command will output proper json.
			if cliParams.checkOutputJSON {
				bundleParams.LogParams = log.LogForOneShot(string(command.LoggerName), "off", true)
			}

			return fxutil.OneShot(runCheckCmd,
				fx.Supply(cliParams, bundleParams),
				processComponent.Bundle,
			)
		},
		SilenceUsage: true,
	}

	checkCmd.Flags().BoolVar(&cliParams.checkOutputJSON, "json", false, "Output check results in JSON")
	checkCmd.Flags().DurationVarP(&cliParams.waitInterval, "wait", "w", defaultWaitInterval, "How long to wait before running the check")

	return []*cobra.Command{checkCmd}
}

func runCheckCmd(deps dependencies) error {
	command.SetHostMountEnv(deps.Log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Now that the logger is configured log host info
	deps.Log.Infof("running on platform: %s", hostMetadataUtils.GetPlatformName())
	agentVersion, _ := version.Agent()
	deps.Log.Infof("running version: %s", agentVersion.GetNumberAndPre())

	// Start workload metadata store before tagger (used for containerCollection)
	// TODO: (Components) Add to dependencies once workloadmeta is migrated to components
	var workloadmetaCollectors workloadmeta.CollectorCatalog
	if deps.Config.GetBool("process_config.remote_workloadmeta") {
		workloadmetaCollectors = workloadmeta.RemoteCatalog
	} else {
		workloadmetaCollectors = workloadmeta.NodeAgentCatalog
	}
	store := workloadmeta.CreateGlobalStore(workloadmetaCollectors)
	store.Start(ctx)

	// Tagger must be initialized after agent config has been setup
	// TODO: (Components) Add to dependencies once tagger is migrated to components
	var t tagger.Tagger
	if deps.Config.GetBool("process_config.remote_tagger") {
		options, err := remote.NodeAgentOptions()
		if err != nil {
			_ = deps.Log.Errorf("unable to configure the remote tagger: %s", err)
		} else {
			t = remote.NewTagger(options)
		}
	} else {
		t = local.NewTagger(store)
	}

	tagger.SetDefaultTagger(t)
	err := tagger.Init(ctx)
	if err != nil {
		_ = deps.Log.Errorf("failed to start the tagger: %s", err)
	}
	defer tagger.Stop() //nolint:errcheck

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
			GRPCServerEnabled:    deps.Syscfg.SysProbeObject().GRPCServerEnabled,
		}

		if !matchingCheck(deps.CliParams.checkName, ch) {
			continue
		}

		if err = ch.Init(cfg, deps.Hostinfo.Object()); err != nil {
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
