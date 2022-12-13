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
	"github.com/DataDog/datadog-agent/pkg/process/config"
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

	// We need to load in the system probe environment variables before we load the config, otherwise an
	// "Unknown environment variable" warning will show up whenever valid system probe environment variables are defined.
	ddconfig.InitSystemProbeConfig(ddconfig.Datadog)

	if err := config.LoadConfigIfExists(cliParams.ConfFilePath); err != nil {
		return log.Criticalf("Error parsing config: %s", err)
	}

	// For system probe, there is an additional config file that is shared with the system-probe
	syscfg, err := sysconfig.Merge(cliParams.SysProbeConfFilePath)
	if err != nil {
		return log.Critical(err)
	}

	cfg, err := config.NewAgentConfig(command.LoggerName, cliParams.ConfFilePath, syscfg)
	if err != nil {
		return log.Criticalf("Error parsing config: %s", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Now that the logger is configured log host info
	hostInfo := host.GetStatusInformation()
	log.Infof("running on platform: %s", hostInfo.Platform)
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

	sysInfo, err := checks.CollectSystemInfo(cfg)
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
	_, checks.Process.SysprobeProcessModuleEnabled = syscfg.EnabledModules[sysconfig.ProcessModule]

	if checks.Process.SysprobeProcessModuleEnabled {
		net.SetSystemProbePath(cfg.SystemProbeAddress)
	}

	// Connections check requires process-check to have occurred first (for process creation ts),
	if cliParams.checkName == checks.Connections.Name() {
		// use a different client ID to prevent destructive querying of connections data
		checks.ProcessAgentClientID = "process-agent-cli-check-id"
		checks.Process.Init(cfg, sysInfo)
		checks.Process.Run(cfg, 0) //nolint:errcheck
		// Clean up the process check state only after the connections check is executed
		cleanups = append(cleanups, checks.Process.Cleanup)
	}

	names := make([]string, 0, len(checks.All))
	for _, ch := range checks.All {
		names = append(names, ch.Name())

		if ch.Name() == cliParams.checkName {
			ch.Init(cfg, sysInfo)
			cleanups = append(cleanups, ch.Cleanup)
			return runCheck(cliParams, cfg, ch)
		}

		withRealTime, ok := ch.(checks.CheckWithRealTime)
		if ok && withRealTime.RealTimeName() == cliParams.checkName {
			withRealTime.Init(cfg, sysInfo)
			cleanups = append(cleanups, withRealTime.Cleanup)
			return runCheckAsRealTime(cliParams, cfg, withRealTime)
		}
	}
	return log.Errorf("invalid check '%s', choose from: %v", cliParams.checkName, names)
}

func runCheck(cliParams *cliParams, cfg *config.AgentConfig, ch checks.Check) error {
	// Run the check once to prime the cache.
	if _, err := ch.Run(cfg, 0); err != nil {
		return fmt.Errorf("collection error: %s", err)
	}

	log.Infof("Waiting %s before running the check", cliParams.waitInterval.String())
	time.Sleep(cliParams.waitInterval)

	if !cliParams.checkOutputJSON {
		printResultsBanner(ch.Name())
	}

	msgs, err := ch.Run(cfg, 1)
	if err != nil {
		return fmt.Errorf("collection error: %s", err)
	}
	return printResults(ch.Name(), msgs, cliParams.checkOutputJSON)
}

func runCheckAsRealTime(cliParams *cliParams, cfg *config.AgentConfig, ch checks.CheckWithRealTime) error {
	options := checks.RunOptions{
		RunStandard: true,
		RunRealTime: true,
	}
	var (
		groupID     int32
		nextGroupID = func() int32 {
			groupID++
			return groupID
		}
	)

	// We need to run the check twice in order to initialize the stats
	// Rate calculations rely on having two datapoints
	if _, err := ch.RunWithOptions(cfg, nextGroupID, options); err != nil {
		return fmt.Errorf("collection error: %s", err)
	}

	log.Infof("Waiting %s before running the check", cliParams.waitInterval.String())
	time.Sleep(cliParams.waitInterval)

	if !cliParams.checkOutputJSON {
		printResultsBanner(ch.RealTimeName())
	}

	run, err := ch.RunWithOptions(cfg, nextGroupID, options)
	if err != nil {
		return fmt.Errorf("collection error: %s", err)
	}

	return printResults(ch.RealTimeName(), run.RealTime, cliParams.checkOutputJSON)
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
