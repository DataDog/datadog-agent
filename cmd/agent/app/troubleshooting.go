// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package app

import (
	//"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	//"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/spf13/cobra"
)

var (
	trshootRate  bool
	integrationName  string
	trshootDelay int
	trshootlogLevel  string
)

// Make the check cmd aggregator never flush by setting a very high interval
const trshootCmdFlushInterval = time.Hour

func init() {
	AgentCmd.AddCommand(trshootCmd)

	trshootCmd.Flags().BoolVarP(&trshootRate, "check-rate", "r", false, "check rates by running the check twice")
	trshootCmd.Flags().StringVarP(&trshootlogLevel, "log-level", "l", "", "set the log level (default 'off')")
	trshootCmd.Flags().IntVarP(&trshootDelay, "delay", "d", 100, "delay between running the check and grabbing the metrics in miliseconds")
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
			fmt.Println("no check found")
			return fmt.Errorf("no check found")
		}

		if len(cs) > 1 {
			fmt.Println("Multiple check instances found, running each of them")
		}

		for _, c := range cs {
			runTroubleshoot(c, agg)
			// s := runTroubleshoot(c, agg)

			// Without a small delay some of the metrics will not show up
			time.Sleep(time.Duration(trshootDelay) * time.Millisecond)

			//getTroubleshootMetrics(agg)

			// checkStatus, _ := status.GetCheckStatus(c, s)
			// fmt.Println(string(checkStatus))
		}

		return nil
	},
}

func runTroubleshoot(c check.Check, agg *aggregator.BufferedAggregator) *check.Stats {
	s := check.NewStats(c)
	i := 0
	times := 1
	if checkRate {
		times = 2
	}
	for i < times {
		t0 := time.Now()
		//err := c.Run()
		err := c.Troubleshoot()
		warnings := c.GetWarnings()
		mStats, _ := c.GetMetricStats()
		s.Add(time.Since(t0), err, warnings, mStats)
		i++
	}

	return s
}

// func getTroubleshootMetrics(agg *aggregator.BufferedAggregator) {
// 	series := agg.GetSeries()
// 	if len(series) != 0 {
// 		fmt.Println("Series: ")
// 		j, _ := json.MarshalIndent(series, "", "  ")
// 		fmt.Println(string(j))
// 	}

// 	sketches := agg.GetSketches()
// 	if len(sketches) != 0 {
// 		fmt.Println("Sketches: ")
// 		j, _ := json.MarshalIndent(sketches, "", "  ")
// 		fmt.Println(string(j))
// 	}

// 	serviceChecks := agg.GetServiceChecks()
// 	if len(serviceChecks) != 0 {
// 		fmt.Println("Service Checks: ")
// 		j, _ := json.MarshalIndent(serviceChecks, "", "  ")
// 		fmt.Println(string(j))
// 	}

// 	events := agg.GetEvents()
// 	if len(events) != 0 {
// 		fmt.Println("Events: ")
// 		j, _ := json.MarshalIndent(events, "", "  ")
// 		fmt.Println(string(j))
// 	}
// }
