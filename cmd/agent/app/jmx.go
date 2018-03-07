// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
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
		Run:   doJmxCollect,
	}

	jmxListEverythingCmd = &cobra.Command{
		Use:   "everything",
		Short: "List every attributes available that has a type supported by JMXFetch.",
		Long:  ``,
		Run:   doJmxListEverything,
	}

	jmxListMatchingCmd = &cobra.Command{
		Use:   "matching",
		Short: "List attributes that match at least one of your instances configuration.",
		Long:  ``,
		Run:   doJmxListMatching,
	}

	jmxListLimitedCmd = &cobra.Command{
		Use:   "limited",
		Short: "List attributes that do match one of your instances configuration but that are not being collected because it would exceed the number of metrics that can be collected.",
		Long:  ``,
		Run:   doJmxListLimited,
	}

	jmxListCollectedCmd = &cobra.Command{
		Use:   "collected",
		Short: "List attributes that will actually be collected by your current instances configuration.",
		Long:  ``,
		Run:   doJmxListCollected,
	}

	jmxListNotMatchingCmd = &cobra.Command{
		Use:   "not-matching",
		Short: "List attributes that don’t match any of your instances configuration.",
		Long:  ``,
		Run:   doJmxListNotCollected,
	}

	checks = []string{}
)

func init() {
	// attach list and collect commands to jmx command
	jmxCmd.AddCommand(jmxListCmd)
	jmxCmd.AddCommand(jmxCollectCmd)

	//attach list commands to list root
	jmxListCmd.AddCommand(jmxListEverythingCmd, jmxListMatchingCmd, jmxListLimitedCmd, jmxListCollectedCmd, jmxListNotMatchingCmd)

	jmxListCmd.PersistentFlags().StringSliceVar(&checks, "checks", []string{"jmx"}, "JMX checks (ex: jmx,tomcat)")
	jmxCollectCmd.PersistentFlags().StringSliceVar(&checks, "checks", []string{"jmx"}, "JMX checks (ex: jmx,tomcat)")

	// attach the command to the root
	AgentCmd.AddCommand(jmxCmd)
}

func doJmxCollect(cmd *cobra.Command, args []string) {
	runJmxCommand("collect")
}

func doJmxListEverything(cmd *cobra.Command, args []string) {
	runJmxCommand("list_everything")
}

func doJmxListMatching(cmd *cobra.Command, args []string) {
	runJmxCommand("list_matching_attributes")
}

func doJmxListLimited(cmd *cobra.Command, args []string) {
	runJmxCommand("list_limited_attributes")
}

func doJmxListCollected(cmd *cobra.Command, args []string) {
	runJmxCommand("list_collected_attributes")
}

func doJmxListNotCollected(cmd *cobra.Command, args []string) {
	runJmxCommand("list_not_matching_attributes")
}

func runJmxCommand(command string) {
	err := common.SetupConfig(confFilePath)
	if err != nil {
		log.Fatalln("unable to set up global agent configuration: %v", err)
	}

	common.SetupAutoConfig(config.Datadog.GetString("confd_path"))

	runner := jmxfetch.New()

	runner.ReportOnConsole = true
	runner.Command = command

	dir, err := ioutil.TempDir(os.TempDir(), "jmxconfd")
	if err != nil {
		log.Fatalln(err)
	}
	defer os.RemoveAll(dir)
	runner.ConfDirectory = dir

	configs := common.AC.GetAllConfigs()

	for _, c := range checks {
		config, err := findJMXConfigByCheckName(configs, c)
		if err != nil {
			log.Fatalln(err)
		}

		file, err := ioutil.TempFile(dir, fmt.Sprintf("conf_%v", c))
		if err != nil {
			log.Fatalln(err)
		}
		defer file.Close()

		_, err = file.WriteString(config.String())
		if err != nil {
			log.Fatalln(err)
		}

		runner.Checks = append(runner.Checks, filepath.Base(file.Name()))
	}

	err = runner.Run()
	if err != nil {
		log.Fatalln(err)
	}

	err = runner.Wait()
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println("JMXFetch exited successfully. If nothing was displayed please check your configuration, flags and the JMXFetch log file.")
}

func findJMXConfigByCheckName(configs []check.Config, checkName string) (*check.Config, error) {
	for _, c := range configs {
		if strings.EqualFold(c.Name, checkName) {
			if c.IsJMX() {
				return &c, nil
			}
		}
	}
	return nil, fmt.Errorf("Unable to find config named %v", checkName)
}
