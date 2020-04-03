// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build jmx

package app

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/app/standalone"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/config"
)

var (
	jmxCmd = &cobra.Command{
		Use:   "jmx",
		Short: "",
		Long:  ``,
	}

	jmxListCmd = &cobra.Command{
		Use:   "list",
		Short: "List attributes matched by JMXFetch.",
		Long:  ``,
	}

	jmxCollectCmd = &cobra.Command{
		Use:   "collect",
		Short: "Start the collection of metrics based on your current configuration and display them in the console.",
		Long:  ``,
		RunE:  doJmxCollect,
	}

	jmxListEverythingCmd = &cobra.Command{
		Use:   "everything",
		Short: "List every attributes available that has a type supported by JMXFetch.",
		Long:  ``,
		RunE:  doJmxListEverything,
	}

	jmxListMatchingCmd = &cobra.Command{
		Use:   "matching",
		Short: "List attributes that match at least one of your instances configuration.",
		Long:  ``,
		RunE:  doJmxListMatching,
	}

	jmxListWithMetricsCmd = &cobra.Command{
		Use:   "with-metrics",
		Short: "List attributes and metrics data that match at least one of your instances configuration.",
		Long:  ``,
		RunE:  doJmxListWithMetrics,
	}

	jmxListWithRateMetricsCmd = &cobra.Command{
		Use:   "with-rate-metrics",
		Short: "List attributes and metrics data that match at least one of your instances configuration, including rates and counters.",
		Long:  ``,
		RunE:  doJmxListWithRateMetrics,
	}

	jmxListLimitedCmd = &cobra.Command{
		Use:   "limited",
		Short: "List attributes that do match one of your instances configuration but that are not being collected because it would exceed the number of metrics that can be collected.",
		Long:  ``,
		RunE:  doJmxListLimited,
	}

	jmxListCollectedCmd = &cobra.Command{
		Use:   "collected",
		Short: "List attributes that will actually be collected by your current instances configuration.",
		Long:  ``,
		RunE:  doJmxListCollected,
	}

	jmxListNotMatchingCmd = &cobra.Command{
		Use:   "not-matching",
		Short: "List attributes that don’t match any of your instances configuration.",
		Long:  ``,
		RunE:  doJmxListNotCollected,
	}

	cliSelectedChecks = []string{}
	jmxLogLevel       string
)

func init() {
	jmxCmd.PersistentFlags().StringVarP(&jmxLogLevel, "log-level", "l", "", "set the log level (default 'debug') (deprecated, use the env var DD_LOG_LEVEL instead)")

	// attach list and collect commands to jmx command
	jmxCmd.AddCommand(jmxListCmd)
	jmxCmd.AddCommand(jmxCollectCmd)

	//attach list commands to list root
	jmxListCmd.AddCommand(jmxListEverythingCmd, jmxListMatchingCmd, jmxListLimitedCmd, jmxListCollectedCmd, jmxListNotMatchingCmd, jmxListWithMetricsCmd, jmxListWithRateMetricsCmd)

	jmxListCmd.PersistentFlags().StringSliceVar(&cliSelectedChecks, "checks", []string{}, "JMX checks (ex: jmx,tomcat)")
	jmxCollectCmd.PersistentFlags().StringSliceVar(&cliSelectedChecks, "checks", []string{}, "JMX checks (ex: jmx,tomcat)")

	// attach the command to the root
	AgentCmd.AddCommand(jmxCmd)
}

func doJmxCollect(cmd *cobra.Command, args []string) error {
	return runJmxCommandConsole("collect")
}

func doJmxListEverything(cmd *cobra.Command, args []string) error {
	return runJmxCommandConsole("list_everything")
}

func doJmxListMatching(cmd *cobra.Command, args []string) error {
	return runJmxCommandConsole("list_matching_attributes")
}

func doJmxListWithMetrics(cmd *cobra.Command, args []string) error {
	return runJmxCommandConsole("list_with_metrics")
}

func doJmxListWithRateMetrics(cmd *cobra.Command, args []string) error {
	return runJmxCommandConsole("list_with_rate_metrics")
}

func doJmxListLimited(cmd *cobra.Command, args []string) error {
	return runJmxCommandConsole("list_limited_attributes")
}

func doJmxListCollected(cmd *cobra.Command, args []string) error {
	return runJmxCommandConsole("list_collected_attributes")
}

func doJmxListNotCollected(cmd *cobra.Command, args []string) error {
	return runJmxCommandConsole("list_not_matching_attributes")
}

// runJmxCommandConsole sets up the common utils necessary for JMX, and executes the command
// with the Console reporter
func runJmxCommandConsole(command string) error {
	logLevel, err := standalone.SetupCLI(loggerName, confFilePath, jmxLogLevel, "debug")
	if err != nil {
		fmt.Printf("Cannot initialize command: %v\n", err)
		return err
	}

	common.SetupAutoConfig(config.Datadog.GetString("confd_path"))

	err = standalone.ExecJMXCommandConsole(command, cliSelectedChecks, logLevel)

	if runtime.GOOS == "windows" {
		standalone.PrintWindowsUserWarning("jmx")
	}

	return err
}
