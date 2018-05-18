// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build jmx

package app

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/api"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/embed"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/jmxfetch"
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

	checks = []string{}
)

func init() {
	// attach list and collect commands to jmx command
	jmxCmd.AddCommand(jmxListCmd)
	jmxCmd.AddCommand(jmxCollectCmd)

	//attach list commands to list root
	jmxListCmd.AddCommand(jmxListEverythingCmd, jmxListMatchingCmd, jmxListLimitedCmd, jmxListCollectedCmd, jmxListNotMatchingCmd)

	jmxListCmd.PersistentFlags().StringSliceVar(&checks, "checks", []string{}, "JMX checks (ex: jmx,tomcat)")
	jmxCollectCmd.PersistentFlags().StringSliceVar(&checks, "checks", []string{}, "JMX checks (ex: jmx,tomcat)")

	// attach the command to the root
	AgentCmd.AddCommand(jmxCmd)
}

func doJmxCollect(cmd *cobra.Command, args []string) error {
	return runJmxCommand("collect")
}

func doJmxListEverything(cmd *cobra.Command, args []string) error {
	return runJmxCommand("list_everything")
}

func doJmxListMatching(cmd *cobra.Command, args []string) error {
	return runJmxCommand("list_matching_attributes")
}

func doJmxListLimited(cmd *cobra.Command, args []string) error {
	return runJmxCommand("list_limited_attributes")
}

func doJmxListCollected(cmd *cobra.Command, args []string) error {
	return runJmxCommand("list_collected_attributes")
}

func doJmxListNotCollected(cmd *cobra.Command, args []string) error {
	return runJmxCommand("list_not_matching_attributes")
}

func setupAgent() error {

	err := common.SetupConfig(confFilePath)
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}

	common.SetupAutoConfig(config.Datadog.GetString("confd_path"))

	// let the os assign an available port
	config.Datadog.Set("cmd_port", 0)

	// start the cmd HTTP server
	if err := api.StartServer(); err != nil {
		return fmt.Errorf("Error while starting api server, exiting: %v", err)
	}

	return nil
}

func runJmxCommand(command string) error {
	err := setupAgent()
	if err != nil {
		return err
	}

	runner := &jmxfetch.JMXFetch{}

	runner.ReportOnConsole = true
	runner.Command = command
	runner.IPCPort = api.ServerAddress().Port

	loadConfigs()

	err = runner.Start()
	if err != nil {
		return err
	}

	err = runner.Wait()
	if err != nil {
		return err
	}

	fmt.Println("JMXFetch exited successfully. If nothing was displayed please check your configuration, flags and the JMXFetch log file.")
	return nil
}

func loadConfigs() {
	fmt.Println("Loading configs :")

	configs := common.AC.GetAllConfigs()
	includeEverything := len(checks) == 0

	for _, c := range configs {
		if check.IsJMXConfig(c.Name, c.InitConfig) && (includeEverything || configIncluded(c)) {
			fmt.Println("Config ", c.Name, " was loaded.")
			embed.AddJMXCachedConfig(c)
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
