// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package app

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/spf13/cobra"
)

var (
	trshootRate  bool
	integrationName  string
	//trshootDelay int
	trshootlogLevel  string
)

// Make the check cmd aggregator never flush by setting a very high interval
const trshootCmdFlushInterval = time.Hour

func init() {
	AgentCmd.AddCommand(trshootCmd)

	trshootCmd.Flags().BoolVarP(&trshootRate, "check-rate", "r", false, "check rates by troubleshooting the integration twice")
	trshootCmd.Flags().StringVarP(&trshootlogLevel, "log-level", "l", "", "set the log level (default 'off')")
	//trshootCmd.Flags().IntVarP(&trshootDelay, "delay", "d", 100, "delay between running the check and grabbing the metrics in miliseconds")
	trshootCmd.SetArgs([]string{"integrationName"})
}

var trshootCmd = &cobra.Command{
	Use:   "troubleshoot <integration_name>",
	Short: "Troubleshooting the specified integration",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Global Agent configuration
		err := common.SetupConfig(confFilePath)
		if err != nil {
			fmt.Printf("Cannot setup config, exiting: %v\n", err)
			return err
		}

		if trshootlogLevel == "" {
			if confFilePath != "" {
				trshootlogLevel = config.Datadog.GetString("log_level")
			} else {
				trshootlogLevel = "off"
			}
		}

		// Setup logger
		err = config.SetupLogger(trshootlogLevel, "", "", false, false, "", true)
		if err != nil {
			fmt.Printf("Cannot setup logger, exiting: %v\n", err)
			return err
		}

		if len(args) != 0 {
			integrationName = args[0]
		} else {
			cmd.Help()
			return nil
		}

		hostname, err := util.GetHostname()
		if err != nil {
			fmt.Printf("Cannot get hostname, exiting: %v\n", err)
			return err
		}

		s := &serializer.Serializer{Forwarder: common.Forwarder}
		agg := aggregator.InitAggregatorWithFlushInterval(s, hostname, trshootCmdFlushInterval)
		common.SetupAutoConfig(config.Datadog.GetString("confd_path"))
		cs := common.AC.GetChecksByName(integrationName)
		if len(cs) == 0 {
			fmt.Println("no integration found")
			return fmt.Errorf("no integration found")
		}

		if len(cs) > 1 {
			fmt.Println("Multiple integration instances found, running each of them")
		}

		for _, c := range cs {
			result := runTroubleshoot(c, agg)
			fmt.Println(result)
			// s := runTroubleshoot(c, agg)

			// Without a small delay some of the metrics will not show up
			//time.Sleep(time.Duration(trshootDelay) * time.Millisecond)

			// checkStatus, _ := status.GetCheckStatus(c, s)
			// fmt.Println(string(checkStatus))
		}

		return nil
	},
}

func runTroubleshoot(c check.Check, agg *aggregator.BufferedAggregator) string {
	s := check.NewStats(c)
	result := "Unrun Troubleshoot commands"
	var err error
	i := 0
	times := 1
	if trshootRate {
		times = 2
	}
	for i < times {
		t0 := time.Now()
		//err := c.Run()
		result, err = c.Troubleshoot()
		warnings := c.GetWarnings()
		mStats, _ := c.GetMetricStats()
		s.Add(time.Since(t0), err, warnings, mStats)
		i++
	}

	return result
}