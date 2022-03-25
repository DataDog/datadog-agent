// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/status"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/process-agent/flags"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/tagger/remote"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

var (
	checkJSONOutput = false
)

// CheckCmd is a command that runs the process-agent version data
var CheckCmd = &cobra.Command{
	Use:          "check",
	Short:        "Run a specific check and print the results. Choose from: process, rtprocess, container, rtcontainer, connections, process_discovery",
	Args:         cobra.ExactArgs(1),
	RunE:         runCheckCmd,
	SilenceUsage: true,
}

const loggerName ddconfig.LoggerName = "PROCESS"

func init() {
	CheckCmd.Flags().BoolVar(&checkJSONOutput, flags.JSONFlag, false, "Outputs the check results in json")
}

func runCheckCmd(cmd *cobra.Command, args []string) error {
	// We need to load in the system probe environment variables before we load the config, otherwise an
	// "Unknown environment variable" warning will show up whenever valid system probe environment variables are defined.
	ddconfig.InitSystemProbeConfig(ddconfig.Datadog)

	configPath := cmd.Flag(flags.CfgPath).Value.String()
	sysprobePath := cmd.Flag(flags.SysProbeConfig).Value.String()

	if err := config.LoadConfigIfExists(configPath); err != nil {
		return log.Criticalf("Error parsing config: %s", err)
	}

	// For system probe, there is an additional config file that is shared with the system-probe
	syscfg, err := sysconfig.Merge(sysprobePath)
	if err != nil {
		return log.Critical(err)
	}

	cfg, err := config.NewAgentConfig(loggerName, configPath, syscfg)
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
	store := workloadmeta.GetGlobalStore()
	store.Start(ctx)

	// Tagger must be initialized after agent config has been setup
	var t tagger.Tagger
	if ddconfig.Datadog.GetBool("process_config.remote_tagger") {
		t = remote.NewTagger()
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

	check := args[0]

	// Connections check requires process-check to have occurred first (for process creation ts),
	if check == checks.Connections.Name() {
		checks.Process.Init(cfg, sysInfo)
		checks.Process.Run(cfg, 0) //nolint:errcheck
	}

	var finalCheck checks.Check
	var rt bool
	names := make([]string, 0, len(checks.All))
	for _, ch := range checks.All {
		names = append(names, ch.Name())

		if ch.Name() == check {
			ch.Init(cfg, sysInfo)
			if err != nil {
				return err
			}
			finalCheck = ch
			break
		}

		withRealTime, ok := ch.(checks.CheckWithRealTime)
		if ok && withRealTime.RealTimeName() == check {
			withRealTime.Init(cfg, sysInfo)
			if err != nil {
				return err
			}
			finalCheck = withRealTime
			rt = true
			break
		}
	}
	if finalCheck == nil {
		return log.Errorf("invalid check '%s', choose from: %v", check, names)
	}

	var checkOutput []process.MessageBody
	var stats *checks.Stats
	if rt {
		withRealtime := finalCheck.(checks.CheckWithRealTime)
		stats = checks.NewStatsRealtime(withRealtime)
		checkOutput, err = runCheckAsRealTime(cfg, withRealtime, stats)
	} else {
		stats = checks.NewStats(finalCheck)
		checkOutput, err = runCheckStandard(cfg, finalCheck, stats)
	}
	stats.CheckConfigSource = configPath

	printResultsBanner(stats.CheckName)
	if checkJSONOutput {
		return printResultsJSON(checkOutput)
	} else {
		return printResults(finalCheck, stats, os.Stdout)
	}
}

func runCheckStandard(cfg *config.AgentConfig, ch checks.Check, stats *checks.Stats) ([]process.MessageBody, error) {
	// Run the check once to prime the cache.
	if _, err := ch.Run(cfg, 0); err != nil {
		return nil, fmt.Errorf("collection error: %s", err)
	}

	time.Sleep(1 * time.Second)

	start := time.Now()
	msgs, err := ch.Run(cfg, 1)
	stats.Runtime = time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("collection error: %s", err)
	}
	return msgs, nil
}

func runCheckAsRealTime(cfg *config.AgentConfig, ch checks.CheckWithRealTime, stats *checks.Stats) ([]process.MessageBody, error) {
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
		return nil, fmt.Errorf("collection error: %s", err)
	}

	time.Sleep(1 * time.Second)

	start := time.Now()
	run, err := ch.RunWithOptions(cfg, nextGroupID, options)
	stats.Runtime = time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("collection error: %s", err)
	}

	return run.RealTime, nil
}

func printResultsBanner(name string) {
	fmt.Printf("-----------------------------\n\n")
	fmt.Printf("\nResults for check %s\n", name)
	fmt.Printf("-----------------------------\n\n")
}

func printResults(check checks.Check, stats *checks.Stats, w io.Writer) error {
	b, err := status.GetProcessCheckStatus(check, stats)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
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
