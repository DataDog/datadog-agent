// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/py"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util"
)

var (
	checkRate     bool
	checkName     string
	checkDelay    int
	isCpuProfiled bool
	logLevel      string
	runs          int
	output        string
)

const CPUProfileMsgTmpl = `Open CPU profiles with:
* Go profile: go tool pprof %s
* Embedded python profile: python -m pstats %s
`

// Make the check cmd aggregator never flush by setting a very high interval
const checkCmdFlushInterval = time.Hour

type CPUProfile struct {
	enabled       bool
	pyProfilePath string
	goProfilePath string
}

func init() {
	AgentCmd.AddCommand(checkCmd)

	checkCmd.Flags().BoolVarP(&checkRate, "check-rate", "r", false, "check rates by running the check twice")
	checkCmd.Flags().StringVarP(&logLevel, "log-level", "l", "", "set the log level (default 'off')")
	checkCmd.Flags().IntVarP(&checkDelay, "delay", "d", 100, "delay between running the check and grabbing the metrics in miliseconds")
	checkCmd.Flags().BoolVarP(&isCpuProfiled, "cpu-profile", "p", false, "write cpu profiles of the check run(s) in the working directory")
	checkCmd.Flags().IntVarP(&runs, "runs", "t", 1, "force check to run n times, set to '2' if --check-rate is used")
	checkCmd.Flags().StringVarP(&output, "output-file", "o", "", "write metrics/events/service checks to file instead of stdout")
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
		err = config.SetupLogger(logLevel, "", "", false, false, "", true, false)
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
		cs := common.AC.GetChecksByName(checkName)
		if len(cs) == 0 {
			for check, error := range autodiscovery.GetConfigErrors() {
				if checkName == check {
					fmt.Fprintln(color.Output, fmt.Sprintf("\n%s: invalid config for %s: %s", color.RedString("Error"), color.YellowString(check), error))
				}
			}
			for check, errors := range autodiscovery.GetLoaderErrors() {
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

		times := runs
		if checkRate {
			times = 2
		}

		for _, c := range cs {
			cpuProfile := CPUProfile{enabled: false}
			if isCpuProfiled {
				cpuProfile.enabled = true
				baseProfilePath := strings.Replace(fmt.Sprintf("%s_%s.prof", checkName, c.ID()), ":", "_", -1)
				cpuProfile.pyProfilePath = fmt.Sprintf("py_%s", baseProfilePath)
				cpuProfile.goProfilePath = fmt.Sprintf("go_%s", baseProfilePath)
			}

			s := runCheck(c, times, agg, cpuProfile)

			// Sleep for a while to allow the aggregator to finish ingesting all the metrics/events/sc
			time.Sleep(time.Duration(checkDelay) * time.Millisecond)

			if output == "" {
				FprintMetrics(color.Output, agg)
			} else {
				outputFile, err := os.Create(output)
				if err != nil {
					color.Red("Could not create output file '%s': %v", output, err)
				}
				FprintMetrics(outputFile, agg)
				outputFile.Close()
			}

			checkStatus, _ := status.GetCheckStatus(c, s)
			fmt.Println(string(checkStatus))

			if cpuProfile.enabled {
				color.Green(CPUProfileMsgTmpl, cpuProfile.goProfilePath, cpuProfile.pyProfilePath)
			}
		}

		if times == 1 {
			color.Yellow("Check has run only once, if some metrics are missing you can try again with --check-rate to see any other metric if available.")
		}

		return nil
	},
}

func runCheck(c check.Check, times int, agg *aggregator.BufferedAggregator, cpuProfile CPUProfile) *check.Stats {
	s := check.NewStats(c)
	i := 0

	if cpuProfile.enabled {
		cpuProfile.start()
		defer cpuProfile.stop()
	}
	for i < times {
		t0 := time.Now()
		err := c.Run()
		warnings := c.GetWarnings()
		mStats, _ := c.GetMetricStats()
		s.Add(time.Since(t0), err, warnings, mStats)
		i++

		if i == 1 && times > 1 && !cpuProfile.enabled {
			py.StartMemProfile()
		}
	}
	if !cpuProfile.enabled {
		py.PrintMemDiff()
	}

	return s
}

func (p *CPUProfile) start() {
	err := py.StartCPUProfile()
	if err != nil {
		color.Red("Could not start py profile: %v", err)
	}

	goProfile, err := os.Create(p.goProfilePath)
	if err != nil {
		color.Red("Could not write go profile to '%s': %v", p.goProfilePath, err)
	}
	pprof.StartCPUProfile(goProfile)
}

func (p *CPUProfile) stop() {
	pprof.StopCPUProfile()

	err := py.StopCPUProfile(p.pyProfilePath)
	if err != nil {
		color.Red("Could not stop py profile: %v", err)
	}
}

func FprintMetrics(w io.Writer, agg *aggregator.BufferedAggregator) {
	series := agg.GetSeries()
	if len(series) != 0 {
		fmt.Fprintln(w, fmt.Sprintf("=== %s ===", color.BlueString("Series")))
		j, _ := json.MarshalIndent(series, "", "  ")
		fmt.Fprintln(w, string(j))
	}

	sketches := agg.GetSketches()
	if len(sketches) != 0 {
		fmt.Fprintln(w, fmt.Sprintf("=== %s ===", color.BlueString("Sketches")))
		j, _ := json.MarshalIndent(sketches, "", "  ")
		fmt.Fprintln(w, string(j))
	}

	serviceChecks := agg.GetServiceChecks()
	if len(serviceChecks) != 0 {
		fmt.Fprintln(w, fmt.Sprintf("=== %s ===", color.BlueString("Service Checks")))
		j, _ := json.MarshalIndent(serviceChecks, "", "  ")
		fmt.Fprintln(w, string(j))
	}

	events := agg.GetEvents()
	if len(events) != 0 {
		fmt.Fprintln(w, fmt.Sprintf("=== %s ===", color.BlueString("Events")))
		j, _ := json.MarshalIndent(events, "", "  ")
		fmt.Fprintln(w, string(j))
	}
}
