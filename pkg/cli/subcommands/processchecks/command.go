// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package processchecks implements 'processchecks' command used by the process-agent and core-agent.
package processchecks

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/utils"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const defaultWaitInterval = time.Second

func waitForWorkloadMeta(logger log.Component, wm workloadmeta.Component) {
	logger.Info("Waiting for workloadmeta to be initialized...")
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			logger.Warn("Workloadmeta is not ready after 10 seconds, proceeding anyway")
			return
		case <-ticker.C:
			if wm.IsInitialized() {
				logger.Info("Workloadmeta is ready, proceeding with checks")
				return
			}
		}
	}
}

// CliParams are the command-line arguments for this subcommand
type CliParams struct {
	*command.GlobalParams
	checkName       string
	checkOutputJSON bool
	waitInterval    time.Duration
}

type dependencies struct {
	fx.In

	CliParams *CliParams

	Config   config.Component
	Syscfg   sysprobeconfig.Component
	Log      log.Component
	Hostinfo hostinfo.Component
	// TODO: the tagger is used by the ContainerProvider, which is currently not a component so there is no direct
	// dependency on it. The ContainerProvider needs to be componentized so it can be injected and have fx manage its
	// lifecycle.
	Tagger       tagger.Component
	WorkloadMeta workloadmeta.Component
	FilterStore  workloadfilter.Component
	NpCollector  npcollector.Component
	Checks       []types.CheckComponent `group:"check"`
}

func nextGroupID() func() int32 {
	var groupID int32
	return func() int32 {
		groupID++
		return groupID
	}
}

// MakeCommand returns a `processchecks` command to be used by agent binaries.
func MakeCommand(globalParamsGetter func() *command.GlobalParams, name string, allowlist []string, getFxOptions func(cliParams *CliParams, bundleParams core.BundleParams) []fx.Option) *cobra.Command {
	cliParams := &CliParams{
		GlobalParams: globalParamsGetter(),
	}

	checkCmd := &cobra.Command{
		Use:   name,
		Short: "Run a specific check and print the results. Choose from: " + strings.Join(allowlist, ", "),

		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cliParams.checkName = args[0]

			if !slices.Contains(allowlist, cliParams.checkName) {
				return fmt.Errorf("invalid check '%s'", cliParams.checkName)
			}

			bundleParams := command.GetCoreBundleParamsForOneShot(globalParamsGetter())

			// Disable logging if `--json` is specified. This way the check command will output proper json.
			if cliParams.checkOutputJSON {
				bundleParams.LogParams = log.ForOneShot(string(command.LoggerName), "off", true)
			}

			return fxutil.OneShot(RunCheckCmd, getFxOptions(cliParams, bundleParams)...)
		},
		SilenceUsage: true,
	}

	checkCmd.Flags().BoolVar(&cliParams.checkOutputJSON, "json", false, "Output check results in JSON")
	checkCmd.Flags().DurationVarP(&cliParams.waitInterval, "wait", "w", defaultWaitInterval, "How long to wait before running the check")

	return checkCmd
}

// RunCheckCmd runs the specified check and prints the results.
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

	// Wait for Workloadmeta to be initialized otherwise results may be empty as this is a hard dependency
	// for some checks
	waitForWorkloadMeta(deps.Log, deps.WorkloadMeta)

	names := make([]string, 0, len(deps.Checks))
	for _, checkComponent := range deps.Checks {
		ch := checkComponent.Object()

		names = append(names, ch.Name())

		_, processModuleEnabled := deps.Syscfg.SysProbeObject().EnabledModules[sysconfig.ProcessModule]
		_, networkTracerModuleEnabled := deps.Syscfg.SysProbeObject().EnabledModules[sysconfig.NetworkTracerModule]
		cfg := &checks.SysProbeConfig{
			MaxConnsPerMessage:         deps.Syscfg.SysProbeObject().MaxConnsPerMessage,
			SystemProbeAddress:         deps.Syscfg.SysProbeObject().SocketAddress,
			ProcessModuleEnabled:       processModuleEnabled,
			NetworkTracerModuleEnabled: networkTracerModuleEnabled,
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

func runCheck(log log.Component, cliParams *CliParams, ch checks.Check) error {
	nextGroupID := nextGroupID()

	options := &checks.RunOptions{
		RunStandard: true,
		// disable chunking for all manual checks
		NoChunking: true,
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
