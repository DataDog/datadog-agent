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
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/tagger/remote"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
			return fxutil.OneShot(runCheckCmd,
				fx.Supply(cliParams),
			)
		},
		SilenceUsage: true,
	}

	checkCmd.Flags().BoolVar(&cliParams.checkOutputJSON, "json", false, "Output check results in JSON")
	checkCmd.Flags().DurationVarP(&cliParams.waitInterval, "wait", "w", defaultWaitInterval, "How long to wait before running the check")

	return []*cobra.Command{checkCmd}
}

func runCheckCmd(cliParams *cliParams) error {
	// Override the log_to_console setting if `--json` is specified. This way the check command will output proper json.
	if cliParams.checkOutputJSON {
		ddconfig.Datadog.Set("log_to_console", false)
	}

	// Override the disable_file_logging setting so that the check command doesn't dump so much noise into the log file.
	ddconfig.Datadog.Set("disable_file_logging", true)

	if err := command.BootstrapConfig(cliParams.GlobalParams.ConfFilePath, true); err != nil {
		return log.Criticalf("Error parsing config: %s", err)
	}

	// For system probe, there is an additional config file that is shared with the system-probe
	syscfg, err := sysconfig.New(cliParams.SysProbeConfFilePath)
	if err != nil {
		return log.Critical(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Now that the logger is configured log host info
	hostStatus := host.GetStatusInformation()
	log.Infof("running on platform: %s", hostStatus.Platform)
	agentVersion, _ := version.Agent()
	log.Infof("running version: %s", agentVersion.GetNumberAndPre())

	// Start workload metadata store before tagger (used for containerCollection)
	var workloadmetaCollectors workloadmeta.CollectorCatalog
	if ddconfig.Datadog.GetBool("process_config.remote_workloadmeta") {
		workloadmetaCollectors = workloadmeta.RemoteCatalog
	} else {
		workloadmetaCollectors = workloadmeta.NodeAgentCatalog
	}
	store := workloadmeta.CreateGlobalStore(workloadmetaCollectors)
	store.Start(ctx)

	// Tagger must be initialized after agent config has been setup
	var t tagger.Tagger
	if ddconfig.Datadog.GetBool("process_config.remote_tagger") {
		options, err := remote.NodeAgentOptions()
		if err != nil {
			log.Errorf("unable to configure the remote tagger: %s", err)
		} else {
			t = remote.NewTagger(options)
		}
	} else {
		t = local.NewTagger(store)
	}

	tagger.SetDefaultTagger(t)
	err = tagger.Init(ctx)
	if err != nil {
		log.Errorf("failed to start the tagger: %s", err)
	}
	defer tagger.Stop() //nolint:errcheck

	hostInfo, err := checks.CollectHostInfo()
	if err != nil {
		log.Errorf("failed to collect system info: %s", err)
	}

	cleanups := make([]func(), 0)
	defer func() {
		for _, c := range cleanups {
			c()
		}
	}()

	// If the sysprobe module is enabled, the process check can call out to the sysprobe for privileged stats
	_, processModuleEnabled := syscfg.EnabledModules[sysconfig.ProcessModule]

	if processModuleEnabled {
		net.SetSystemProbePath(syscfg.SocketAddress)
	}

	// TODO: Remove dependency on syscfg once runCheckCmd is migrated to components
	all := checks.All(syscfg)
	names := make([]string, 0, len(all))
	for _, ch := range all {
		names = append(names, ch.Name())

		cfg := &checks.SysProbeConfig{
			MaxConnsPerMessage:   syscfg.MaxConnsPerMessage,
			SystemProbeAddress:   syscfg.SocketAddress,
			ProcessModuleEnabled: processModuleEnabled,
		}

		if !matchingCheck(cliParams.checkName, ch) {
			continue
		}

		if err = ch.Init(cfg, hostInfo); err != nil {
			return err
		}
		cleanups = append(cleanups, ch.Cleanup)
		return runCheck(cliParams, ch)
	}
	return log.Errorf("invalid check '%s', choose from: %v", cliParams.checkName, names)
}

func matchingCheck(checkName string, ch checks.Check) bool {
	if ch.SupportsRunOptions() {
		if checks.RTName(ch.Name()) == checkName {
			return true
		}
	}

	return ch.Name() == checkName
}

func runCheck(cliParams *cliParams, ch checks.Check) error {
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
