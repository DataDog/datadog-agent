// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util"
)

var (
	checkRate  bool
	checkName  string
	checkDelay int
	logLevel   string
)

// Make the check cmd aggregator never flush by setting a very high interval
const checkCmdFlushInterval = time.Hour

func init() {
	AgentCmd.AddCommand(checkCmd)

	checkCmd.Flags().BoolVarP(&checkRate, "check-rate", "r", false, "check rates by running the check twice")
	checkCmd.Flags().StringVarP(&logLevel, "log-level", "l", "", "set the log level (default 'off')")
	checkCmd.Flags().IntVarP(&checkDelay, "delay", "d", 100, "delay between running the check and grabbing the metrics in miliseconds")
	checkCmd.SetArgs([]string{"checkName"})
}

var checkCmd = &cobra.Command{
	Use:   "check <check_name>",
	Short: "Run the specified check",
	Long:  `Use this to run a specific check with a specific rate`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Global Agent configuration
		err := common.SetupConfig(confFilePath)
		if err != nil {
			fmt.Printf("Cannot setup config, exiting: %v\n", err)
			return err
		}

		if flagNoColor {
			color.NoColor = true
		}

		if logLevel == "" {
			if confFilePath != "" {
				logLevel = config.Datadog.GetString("log_level")
			} else {
				logLevel = "off"
			}
		}

		// Setup logger
		err = config.SetupLogger(logLevel, "", "", false, true, false)
		if err != nil {
			fmt.Printf("Cannot setup logger, exiting: %v\n", err)
			return err
		}

		if len(args) != 0 {
			checkName = args[0]
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
		agg := aggregator.InitAggregatorWithFlushInterval(s, hostname, checkCmdFlushInterval)
		common.SetupAutoConfig(config.Datadog.GetString("confd_path"))
		cs := collector.GetChecksByNameForConfigs(checkName, common.AC.GetAllConfigs())
		if len(cs) == 0 {
			for check, error := range autodiscovery.GetConfigErrors() {
				if checkName == check {
					fmt.Fprintln(color.Output, fmt.Sprintf("\n%s: invalid config for %s: %s", color.RedString("Error"), color.YellowString(check), error))
				}
			}
			for check, errors := range collector.GetLoaderErrors() {
				if checkName == check {
					fmt.Fprintln(color.Output, fmt.Sprintf("\n%s: could not load %s:", color.RedString("Error"), color.YellowString(checkName)))
					for loader, error := range errors {
						fmt.Fprintln(color.Output, fmt.Sprintf("* %s: %s", color.YellowString(loader), error))
					}
				}
			}
			for check, warnings := range autodiscovery.GetResolveWarnings() {
				if checkName == check {
					fmt.Fprintln(color.Output, fmt.Sprintf("\n%s: could not resolve %s config:", color.YellowString("Warning"), color.YellowString(check)))
					for _, warning := range warnings {
						fmt.Fprintln(color.Output, fmt.Sprintf("* %s", warning))
					}
				}
			}
			return fmt.Errorf("no valid check found")
		}

		if len(cs) > 1 {
			fmt.Println("Multiple check instances found, running each of them")
		}

		for _, c := range cs {
			s := runCheck(c, agg)

			// Sleep for a while to allow the aggregator to finish ingesting all the metrics/events/sc
			time.Sleep(time.Duration(checkDelay) * time.Millisecond)

			printMetrics(agg)

			checkStatus, _ := status.GetCheckStatus(c, s)
			fmt.Println(string(checkStatus))
		}

		if checkRate == false {
			color.Yellow("Check has run only once, if some metrics are missing you can try again with --check-rate to see any other metric if available.")
		}

		return nil
	},
}

func runCheck(c check.Check, agg *aggregator.BufferedAggregator) *check.Stats {
	s := check.NewStats(c)
	i := 0
	times := 1
	if checkRate {
		times = 2
	}
	for i < times {
		t0 := time.Now()
		err := c.Run()
		warnings := c.GetWarnings()
		mStats, _ := c.GetMetricStats()
		s.Add(time.Since(t0), err, warnings, mStats)
		i++
	}

	return s
}

func printMetrics(agg *aggregator.BufferedAggregator) {
	series := agg.GetSeries()
	if len(series) != 0 {
		fmt.Fprintln(color.Output, fmt.Sprintf("=== %s ===", color.BlueString("Series")))
		j, _ := json.MarshalIndent(series, "", "  ")
		fmt.Println(string(j))
	}

	sketches := agg.GetSketches()
	if len(sketches) != 0 {
		fmt.Fprintln(color.Output, fmt.Sprintf("=== %s ===", color.BlueString("Sketches")))
		j, _ := json.MarshalIndent(sketches, "", "  ")
		fmt.Println(string(j))
	}

	serviceChecks := agg.GetServiceChecks()
	if len(serviceChecks) != 0 {
		fmt.Fprintln(color.Output, fmt.Sprintf("=== %s ===", color.BlueString("Service Checks")))
		j, _ := json.MarshalIndent(serviceChecks, "", "  ")
		fmt.Println(string(j))
	}

	events := agg.GetEvents()
	if len(events) != 0 {
		fmt.Fprintln(color.Output, fmt.Sprintf("=== %s ===", color.BlueString("Events")))
		j, _ := json.MarshalIndent(events, "", "  ")
		fmt.Println(string(j))
	}
}
