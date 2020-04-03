// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build jmx

package app

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/api"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/embed/jmx"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/jmxfetch"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/spf13/cobra"
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

	checks      = []string{}
	jmxLogLevel string
)

func init() {
	jmxCmd.PersistentFlags().StringVarP(&jmxLogLevel, "log-level", "l", "", "set the log level (default 'debug') (deprecated, use the env var DD_LOG_LEVEL instead)")

	// attach list and collect commands to jmx command
	jmxCmd.AddCommand(jmxListCmd)
	jmxCmd.AddCommand(jmxCollectCmd)

	//attach list commands to list root
	jmxListCmd.AddCommand(jmxListEverythingCmd, jmxListMatchingCmd, jmxListLimitedCmd, jmxListCollectedCmd, jmxListNotMatchingCmd, jmxListWithMetricsCmd, jmxListWithRateMetricsCmd)

	jmxListCmd.PersistentFlags().StringSliceVar(&checks, "checks", []string{}, "JMX checks (ex: jmx,tomcat)")
	jmxCollectCmd.PersistentFlags().StringSliceVar(&checks, "checks", []string{}, "JMX checks (ex: jmx,tomcat)")

	// attach the command to the root
	AgentCmd.AddCommand(jmxCmd)
}

func doJmxCollect(cmd *cobra.Command, args []string) error {
	return RunJmxCommand("collect", jmxfetch.ReporterConsole, nil)
}

func doJmxListEverything(cmd *cobra.Command, args []string) error {
	return RunJmxCommand("list_everything", jmxfetch.ReporterConsole, nil)
}

func doJmxListMatching(cmd *cobra.Command, args []string) error {
	return RunJmxCommand("list_matching_attributes", jmxfetch.ReporterConsole, nil)
}

func doJmxListWithMetrics(cmd *cobra.Command, args []string) error {
	return RunJmxCommand("list_with_metrics", jmxfetch.ReporterConsole, nil)
}

func doJmxListWithRateMetrics(cmd *cobra.Command, args []string) error {
	return RunJmxCommand("list_with_rate_metrics", jmxfetch.ReporterConsole, nil)
}

func doJmxListLimited(cmd *cobra.Command, args []string) error {
	return RunJmxCommand("list_limited_attributes", jmxfetch.ReporterConsole, nil)
}

func doJmxListCollected(cmd *cobra.Command, args []string) error {
	return RunJmxCommand("list_collected_attributes", jmxfetch.ReporterConsole, nil)
}

func doJmxListNotCollected(cmd *cobra.Command, args []string) error {
	return RunJmxCommand("list_not_matching_attributes", jmxfetch.ReporterConsole, nil)
}

func RunJmxCommand(command string, reporter jmxfetch.JMXReporter, output func(...interface{})) error {

	if jmxLogLevel != "" {
		// Honour the deprecated --log-level argument
		overrides := make(map[string]interface{})
		overrides["log_level"] = jmxLogLevel
		config.AddOverrides(overrides)
	} else {
		jmxLogLevel = config.GetEnv("DD_LOG_LEVEL", "debug")
	}

	overrides := make(map[string]interface{})
	overrides["cmd_port"] = 0 // let the os assign an available port
	config.AddOverrides(overrides)

	err := common.SetupConfig(confFilePath)
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}

	err = config.SetupLogger(loggerName, jmxLogLevel, "", "", false, true, false)
	if err != nil {
		fmt.Printf("Cannot setup logger, exiting: %v\n", err)
		return err
	}

	common.SetupAutoConfig(config.Datadog.GetString("confd_path"))

	// start the cmd HTTP server
	if err := api.StartServer(); err != nil {
		return fmt.Errorf("Error while starting api server, exiting: %v", err)
	}

	runner := &jmxfetch.JMXFetch{}

	runner.Reporter = reporter
	runner.Command = command
	runner.IPCPort = api.ServerAddress().Port
	runner.Output = log.Info
	runner.LogLevel = jmxLogLevel
	if output != nil {
		runner.Output = output
	}

	loadConfigs(runner)

	err = runner.Start(false)
	if err != nil {
		return err
	}

	err = runner.Wait()
	if err != nil {
		return err
	}

	fmt.Println("JMXFetch exited successfully. If nothing was displayed please check your configuration, flags and the JMXFetch log file.")
	if runtime.GOOS == "windows" {
		printWindowsUserWarning("jmx")
	}
	return nil
}

// RunJmxListWithMetrics runs the JMX command with "with-metrics", reporting
// the data as a JSON on the console. It is used by the `check jmx` cli command
// of the Agent.
func RunJmxListWithMetrics() error {
	// don't pollute the JSON with the log pattern.
	out := func(a ...interface{}) {
		fmt.Println(a...)
	}
	return RunJmxCommand("list_with_metrics", jmxfetch.ReporterJSON, out)
}

// RunJmxListWithRateMetrics runs the JMX command with "with-rate-metrics", reporting
// the data as a JSON on the console. It is used by the `check jmx --rate` cli command
// of the Agent.
func RunJmxListWithRateMetrics() error {
	// don't pollute the JSON with the log pattern.
	out := func(a ...interface{}) {
		fmt.Println(a...)
	}
	return RunJmxCommand("list_with_rate_metrics", jmxfetch.ReporterJSON, out)
}

func loadConfigs(runner *jmxfetch.JMXFetch) {
	fmt.Println("Loading configs :")

	configs := common.AC.GetAllConfigs()
	includeEverything := len(checks) == 0

	for _, c := range configs {
		if check.IsJMXConfig(c) && (includeEverything || configIncluded(c)) {
			fmt.Println("Config ", c.Name, " was loaded.")
			jmx.AddScheduledConfig(c)
			runner.ConfigureFromInitConfig(c.InitConfig)
			for _, instance := range c.Instances {
				if !check.IsJMXInstance(c.Name, instance, c.InitConfig) {
					continue
				}
				runner.ConfigureFromInstance(instance)
			}
		}
	}
}

func configIncluded(config integration.Config) bool {
	for _, c := range checks {
		if strings.EqualFold(config.Name, c) {
			return true
		}
	}
	return false
}
