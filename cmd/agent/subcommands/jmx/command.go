// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx
// +build jmx

package jmx

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/app"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/internal/standalone"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/config"
)

var (
	cliSelectedChecks = []string{}
	jmxLogLevel       string
	saveFlare         bool
)

var (
	discoveryTimeout       uint
	discoveryRetryInterval uint
	discoveryMinInstances  uint
)

const jmxLoggerName config.LoggerName = "JMXFETCH"

// Command returns the main cobra config command.
func Command(globalArgs *app.GlobalArgs) *cobra.Command {
	jmxCmd := &cobra.Command{
		Use:   "jmx",
		Short: "Run troubleshooting commands on JMXFetch integrations",
		Long:  ``,
	}
	jmxCmd.PersistentFlags().StringVarP(&jmxLogLevel, "log-level", "l", "", "set the log level (default 'debug') (deprecated, use the env var DD_LOG_LEVEL instead)")
	jmxCmd.PersistentFlags().UintVarP(&discoveryTimeout, "discovery-timeout", "", 5, "max retry duration until Autodiscovery resolves the check template (in seconds)")
	jmxCmd.PersistentFlags().UintVarP(&discoveryRetryInterval, "discovery-retry-interval", "", 1, "(unused)")
	jmxCmd.PersistentFlags().UintVarP(&discoveryMinInstances, "discovery-min-instances", "", 1, "minimum number of config instances to be discovered before running the check(s)")

	jmxCollectCmd := &cobra.Command{
		Use:   "collect",
		Short: "Start the collection of metrics based on your current configuration and display them in the console.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJmxCommandConsole(globalArgs, "collect")
		},
	}
	jmxCollectCmd.PersistentFlags().StringSliceVar(&cliSelectedChecks, "checks", []string{}, "JMX checks (ex: jmx,tomcat)")
	jmxCollectCmd.PersistentFlags().BoolVarP(&saveFlare, "flare", "", false, "save jmx list results to the log dir so it may be reported in a flare")
	jmxCmd.AddCommand(jmxCollectCmd)

	jmxListEverythingCmd := &cobra.Command{
		Use:   "everything",
		Short: "List every attributes available that has a type supported by JMXFetch.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJmxCommandConsole(globalArgs, "list_everything")
		},
	}

	jmxListMatchingCmd := &cobra.Command{
		Use:   "matching",
		Short: "List attributes that match at least one of your instances configuration.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJmxCommandConsole(globalArgs, "list_matching_attributes")
		},
	}

	jmxListWithMetricsCmd := &cobra.Command{
		Use:   "with-metrics",
		Short: "List attributes and metrics data that match at least one of your instances configuration.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJmxCommandConsole(globalArgs, "list_with_metrics")
		},
	}

	jmxListWithRateMetricsCmd := &cobra.Command{
		Use:   "with-rate-metrics",
		Short: "List attributes and metrics data that match at least one of your instances configuration, including rates and counters.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJmxCommandConsole(globalArgs, "list_with_rate_metrics")
		},
	}

	jmxListLimitedCmd := &cobra.Command{
		Use:   "limited",
		Short: "List attributes that do match one of your instances configuration but that are not being collected because it would exceed the number of metrics that can be collected.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJmxCommandConsole(globalArgs, "list_limited_attributes")
		},
	}

	jmxListCollectedCmd := &cobra.Command{
		Use:   "collected",
		Short: "List attributes that will actually be collected by your current instances configuration.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJmxCommandConsole(globalArgs, "list_collected_attributes")
		},
	}

	jmxListNotMatchingCmd := &cobra.Command{
		Use:   "not-matching",
		Short: "List attributes that don’t match any of your instances configuration.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJmxCommandConsole(globalArgs, "list_not_matching_attributes")
		},
	}

	jmxListCmd := &cobra.Command{
		Use:   "list",
		Short: "List attributes matched by JMXFetch.",
		Long:  ``,
	}
	jmxListCmd.AddCommand(jmxListEverythingCmd, jmxListMatchingCmd, jmxListLimitedCmd, jmxListCollectedCmd, jmxListNotMatchingCmd, jmxListWithMetricsCmd, jmxListWithRateMetricsCmd)

	jmxListCmd.PersistentFlags().StringSliceVar(&cliSelectedChecks, "checks", []string{}, "JMX checks (ex: jmx,tomcat)")
	jmxListCmd.PersistentFlags().BoolVarP(&saveFlare, "flare", "", false, "save jmx list results to the log dir so it may be reported in a flare")
	jmxCmd.AddCommand(jmxListCmd)

	// attach the command to the root
	return jmxCmd
}

// runJmxCommandConsole sets up the common utils necessary for JMX, and executes the command
// with the Console reporter
func runJmxCommandConsole(globalArgs *app.GlobalArgs, command string) error {
	logFile := ""
	if saveFlare {
		// Windows cannot accept ":" in file names
		filenameSafeTimeStamp := strings.ReplaceAll(time.Now().UTC().Format(time.RFC3339), ":", "-")
		logFile = filepath.Join(common.DefaultJMXFlareDirectory, "jmx_"+command+"_"+filenameSafeTimeStamp+".log")
		jmxLogLevel = "debug"
	}

	logLevel, _, err := standalone.SetupCLI(globalArgs.LoggerName, globalArgs.ConfFilePath, "", logFile, jmxLogLevel, "debug")
	if err != nil {
		fmt.Printf("Cannot initialize command: %v\n", err)
		return err
	}

	err = config.SetupJMXLogger(jmxLoggerName, logFile, "", false, true, false)
	if err != nil {
		return fmt.Errorf("Unable to set up JMX logger: %v", err)
	}

	common.LoadComponents(context.Background(), config.Datadog.GetString("confd_path"))

	// Create the CheckScheduler, but do not attach it to
	// AutoDiscovery.  NOTE: we do not start common.Coll, either.
	collector.InitCheckScheduler(common.Coll)

	// Note: when no checks are selected, cliSelectedChecks will be the empty slice and thus
	//       WaitForConfigsFromAD will timeout and return no AD configs.
	waitCtx, cancelTimeout := context.WithTimeout(
		context.Background(), time.Duration(discoveryTimeout)*time.Second)
	allConfigs := common.WaitForConfigsFromAD(waitCtx, cliSelectedChecks, int(discoveryMinInstances))
	cancelTimeout()

	err = standalone.ExecJMXCommandConsole(command, cliSelectedChecks, logLevel, allConfigs)

	if runtime.GOOS == "windows" {
		standalone.PrintWindowsUserWarning("jmx")
	}

	return err
}
