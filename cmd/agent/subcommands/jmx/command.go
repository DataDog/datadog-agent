// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx

// Package jmx implements 'agent jmx'.
package jmx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/cli/standalone"
	"github.com/DataDog/datadog-agent/pkg/collector"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type cliParams struct {
	*command.GlobalParams

	// command is the jmx console command to run
	command string

	cliSelectedChecks     []string
	logFile               string // calculated in runOneShot
	jmxLogLevel           string
	saveFlare             bool
	discoveryTimeout      uint
	discoveryMinInstances uint
	instanceFilter        string
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	var discoveryRetryInterval uint // unused command-line flag
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	jmxCmd := &cobra.Command{
		Use:   "jmx",
		Short: "Run troubleshooting commands on JMXFetch integrations",
		Long:  ``,
	}
	jmxCmd.PersistentFlags().StringVarP(&cliParams.jmxLogLevel, "log-level", "l", "", "set the log level (default 'debug') (deprecated, use the env var DD_LOG_LEVEL instead)")
	jmxCmd.PersistentFlags().UintVarP(&cliParams.discoveryTimeout, "discovery-timeout", "", 5, "max retry duration until Autodiscovery resolves the check template (in seconds)")
	jmxCmd.PersistentFlags().UintVarP(&discoveryRetryInterval, "discovery-retry-interval", "", 1, "(unused)")
	jmxCmd.PersistentFlags().UintVarP(&cliParams.discoveryMinInstances, "discovery-min-instances", "", 1, "minimum number of config instances to be discovered before running the check(s)")
	jmxCmd.PersistentFlags().StringVarP(&cliParams.instanceFilter, "instance-filter", "", "", "filter instances using jq style syntax, example: --instance-filter '.ip_address == \"127.0.0.51\"'")

	// All subcommands use the same provided components, with a different
	// oneShot callback, and with some complex derivation of the
	// core.BundleParams value
	runOneShot := func(callback interface{}) error {
		cliParams.logFile = ""

		// if saving a flare, write a debug log within a directory that will be
		// captured in the flare.
		if cliParams.saveFlare {
			// Windows cannot accept ":" in file names
			filenameSafeTimeStamp := strings.ReplaceAll(time.Now().UTC().Format(time.RFC3339), ":", "-")
			cliParams.logFile = filepath.Join(path.DefaultJMXFlareDirectory, "jmx_"+cliParams.command+"_"+filenameSafeTimeStamp+".log")
			cliParams.jmxLogLevel = "debug"
		}

		// default log level to "debug" if not given
		if cliParams.jmxLogLevel == "" {
			cliParams.jmxLogLevel = "debug"
		}
		params := core.BundleParams{
			ConfigParams: config.NewAgentParamsWithSecrets(globalParams.ConfFilePath),
			LogParams:    log.LogForOneShot(command.LoggerName, cliParams.jmxLogLevel, false)}
		if cliParams.logFile != "" {
			params.LogParams.LogToFile(cliParams.logFile)
		}

		return fxutil.OneShot(callback,
			fx.Supply(cliParams),
			fx.Supply(params),
			core.Bundle,
		)
	}

	jmxCollectCmd := &cobra.Command{
		Use:   "collect",
		Short: "Start the collection of metrics based on your current configuration and display them in the console.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.command = "collect"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}
	jmxCollectCmd.PersistentFlags().StringSliceVar(&cliParams.cliSelectedChecks, "checks", []string{}, "JMX checks (ex: jmx,tomcat)")
	jmxCollectCmd.PersistentFlags().BoolVarP(&cliParams.saveFlare, "flare", "", false, "save jmx list results to the log dir so it may be reported in a flare")
	jmxCmd.AddCommand(jmxCollectCmd)

	jmxListEverythingCmd := &cobra.Command{
		Use:   "everything",
		Short: "List every attributes available that has a type supported by JMXFetch.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.command = "list_everything"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListMatchingCmd := &cobra.Command{
		Use:   "matching",
		Short: "List attributes that match at least one of your instances configuration.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.command = "list_matching_attributes"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListWithMetricsCmd := &cobra.Command{
		Use:   "with-metrics",
		Short: "List attributes and metrics data that match at least one of your instances configuration.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.command = "list_with_metrics"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListWithRateMetricsCmd := &cobra.Command{
		Use:   "with-rate-metrics",
		Short: "List attributes and metrics data that match at least one of your instances configuration, including rates and counters.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.command = "list_with_rate_metrics"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListLimitedCmd := &cobra.Command{
		Use:   "limited",
		Short: "List attributes that do match one of your instances configuration but that are not being collected because it would exceed the number of metrics that can be collected.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.command = "list_limited_attributes"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListCollectedCmd := &cobra.Command{
		Use:   "collected",
		Short: "List attributes that will actually be collected by your current instances configuration.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.command = "list_collected_attributes"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListNotMatchingCmd := &cobra.Command{
		Use:   "not-matching",
		Short: "List attributes that don’t match any of your instances configuration.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.command = "list_not_matching_attributes"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListCmd := &cobra.Command{
		Use:   "list",
		Short: "List attributes matched by JMXFetch.",
		Long:  ``,
	}
	jmxListCmd.AddCommand(
		jmxListEverythingCmd,
		jmxListMatchingCmd,
		jmxListLimitedCmd,
		jmxListCollectedCmd,
		jmxListNotMatchingCmd,
		jmxListWithMetricsCmd,
		jmxListWithRateMetricsCmd,
	)

	jmxListCmd.PersistentFlags().StringSliceVar(&cliParams.cliSelectedChecks, "checks", []string{}, "JMX checks (ex: jmx,tomcat)")
	jmxListCmd.PersistentFlags().BoolVarP(&cliParams.saveFlare, "flare", "", false, "save jmx list results to the log dir so it may be reported in a flare")
	jmxCmd.AddCommand(jmxListCmd)

	// attach the command to the root
	return []*cobra.Command{jmxCmd}
}

// disableCmdPort overrrides the `cmd_port` configuration so that when the
// server starts up, it does not do so on the same port as a running agent.
//
// Ideally, the server wouldn't start up at all, but this workaround has been
// in place for some times.
func disableCmdPort() {
	os.Setenv("DD_CMD_PORT", "0") // 0 indicates the OS should pick an unused port
}

// runJmxCommandConsole sets up the common utils necessary for JMX, and executes the command
// with the Console reporter
func runJmxCommandConsole(log log.Component, config config.Component, cliParams *cliParams) error {
	err := pkgconfig.SetupJMXLogger(cliParams.logFile, "", false, true, false)
	if err != nil {
		return fmt.Errorf("Unable to set up JMX logger: %v", err)
	}

	common.LoadComponents(context.Background(), config.GetString("confd_path"))
	common.AC.LoadAndRun(context.Background())

	// Create the CheckScheduler, but do not attach it to
	// AutoDiscovery.  NOTE: we do not start common.Coll, either.
	collector.InitCheckScheduler(common.Coll)

	// if cliSelectedChecks is empty, then we want to fetch all check configs;
	// otherwise, we fetch only the matching cehck configs.
	waitCtx, cancelTimeout := context.WithTimeout(
		context.Background(), time.Duration(cliParams.discoveryTimeout)*time.Second)
	var allConfigs []integration.Config
	if len(cliParams.cliSelectedChecks) == 0 {
		allConfigs, err = common.WaitForAllConfigsFromAD(waitCtx)
	} else {
		allConfigs, err = common.WaitForConfigsFromAD(waitCtx, cliParams.cliSelectedChecks, int(cliParams.discoveryMinInstances), cliParams.instanceFilter)
	}
	cancelTimeout()
	if err != nil {
		return err
	}

	err = standalone.ExecJMXCommandConsole(cliParams.command, cliParams.cliSelectedChecks, cliParams.jmxLogLevel, allConfigs)

	if runtime.GOOS == "windows" {
		standalone.PrintWindowsUserWarning("jmx")
	}

	return err
}
