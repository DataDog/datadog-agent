// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package app

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util"
)

var (
	checkRate            bool
	checkTimes           int
	checkPause           int
	checkName            string
	checkDelay           int
	logLevel             string
	formatJSON           bool
	breakPoint           string
	profileMemory        bool
	profileMemoryDir     string
	profileMemoryFrames  string
	profileMemoryGC      string
	profileMemoryCombine string
	profileMemorySort    string
	profileMemoryLimit   string
	profileMemoryDiff    string
	profileMemoryFilters string
	profileMemoryUnit    string
	profileMemoryVerbose string
)

// Make the check cmd aggregator never flush by setting a very high interval
const checkCmdFlushInterval = time.Hour

func init() {
	AgentCmd.AddCommand(checkCmd)

	checkCmd.Flags().BoolVarP(&checkRate, "check-rate", "r", false, "check rates by running the check twice with a 1sec-pause between the 2 runs")
	checkCmd.Flags().IntVarP(&checkTimes, "check-times", "t", 1, "number of times to run the check")
	checkCmd.Flags().IntVar(&checkPause, "pause", 0, "pause between multiple runs of the check, in milliseconds")
	checkCmd.Flags().StringVarP(&logLevel, "log-level", "l", "", "set the log level (default 'off') (deprecated, use the env var DD_LOG_LEVEL instead)")
	checkCmd.Flags().IntVarP(&checkDelay, "delay", "d", 100, "delay between running the check and grabbing the metrics in milliseconds")
	checkCmd.Flags().BoolVarP(&formatJSON, "json", "", false, "format aggregator and check runner output as json")
	checkCmd.Flags().StringVarP(&breakPoint, "breakpoint", "b", "", "set a breakpoint at a particular line number (Python checks only)")
	checkCmd.Flags().BoolVarP(&profileMemory, "profile-memory", "m", false, "run the memory profiler (Python checks only)")

	// Power user flags - mark as hidden
	createHiddenStringFlag(&profileMemoryDir, "m-dir", "", "an existing directory in which to store memory profiling data, ignoring clean-up")
	createHiddenStringFlag(&profileMemoryFrames, "m-frames", "", "the number of stack frames to consider")
	createHiddenStringFlag(&profileMemoryGC, "m-gc", "", "whether or not to run the garbage collector to remove noise")
	createHiddenStringFlag(&profileMemoryCombine, "m-combine", "", "whether or not to aggregate over all traceback frames")
	createHiddenStringFlag(&profileMemorySort, "m-sort", "", "what to sort by between: lineno | filename | traceback")
	createHiddenStringFlag(&profileMemoryLimit, "m-limit", "", "the maximum number of sorted results to show")
	createHiddenStringFlag(&profileMemoryDiff, "m-diff", "", "how to order diff results between: absolute | positive")
	createHiddenStringFlag(&profileMemoryFilters, "m-filters", "", "comma-separated list of file path glob patterns to filter by")
	createHiddenStringFlag(&profileMemoryUnit, "m-unit", "", "the binary unit to represent memory usage (kib, mb, etc.). the default is dynamic")
	createHiddenStringFlag(&profileMemoryVerbose, "m-verbose", "", "whether or not to include potentially noisy sources")

	checkCmd.SetArgs([]string{"checkName"})
}

var checkCmd = &cobra.Command{
	Use:   "check <check_name>",
	Short: "Run the specified check",
	Long:  `Use this to run a specific check with a specific rate`,
	RunE: func(cmd *cobra.Command, args []string) error {

		if logLevel != "" {
			// Honour the deprecated --log-level argument
			overrides := make(map[string]interface{})
			overrides["log_level"] = logLevel
			config.AddOverrides(overrides)
		} else {
			logLevel = config.GetEnv("DD_LOG_LEVEL", "off")
		}

		if flagNoColor {
			color.NoColor = true
		}

		err := common.SetupConfig(confFilePath)
		if err != nil {
			return fmt.Errorf("unable to set up global agent configuration: %v", err)
		}

		err = config.SetupLogger(loggerName, logLevel, "", "", false, true, false)
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

		s := serializer.NewSerializer(common.Forwarder)
		agg := aggregator.InitAggregatorWithFlushInterval(s, hostname, "agent", checkCmdFlushInterval)
		common.SetupAutoConfig(config.Datadog.GetString("confd_path"))

		if config.Datadog.GetBool("inventories_enabled") {
			metadata.SetupInventoriesExpvar(common.AC, common.Coll)
		}

		allConfigs := common.AC.GetAllConfigs()

		// make sure the checks in cs are not JMX checks
		for _, conf := range allConfigs {
			if conf.Name != checkName {
				continue
			}

			if check.IsJMXConfig(conf.Name, conf.InitConfig) {
				// we'll mimic the check command behavior with JMXFetch by running
				// it with the JSON reporter and the list_with_metrics command.
				fmt.Println("Please consider using the 'jmx' command instead of 'check jmx'")
				if err := RunJmxListWithMetrics(); err != nil {
					return fmt.Errorf("while running the jmx check: %v", err)
				}
				return nil
			}
		}

		if profileMemory {
			// If no directory is specified, make a temporary one
			if profileMemoryDir == "" {
				profileMemoryDir, err = ioutil.TempDir("", "datadog-agent-memory-profiler")
				if err != nil {
					return err
				}

				defer func() {
					cleanupErr := os.RemoveAll(profileMemoryDir)
					if cleanupErr != nil {
						fmt.Printf("%s\n", cleanupErr)
					}
				}()
			}

			for idx := range allConfigs {
				conf := &allConfigs[idx]
				if conf.Name != checkName {
					continue
				}

				var data map[string]interface{}

				err = yaml.Unmarshal(conf.InitConfig, &data)
				if err != nil {
					return err
				}

				if data == nil {
					data = make(map[string]interface{})
				}

				data["profile_memory"] = profileMemoryDir
				err = populateMemoryProfileConfig(data)
				if err != nil {
					return err
				}

				y, _ := yaml.Marshal(data)
				conf.InitConfig = y

				break
			}
		} else if breakPoint != "" {
			breakPointLine, err := strconv.Atoi(breakPoint)
			if err != nil {
				fmt.Printf("breakpoint must be an integer\n")
				return err
			}

			for idx := range allConfigs {
				conf := &allConfigs[idx]
				if conf.Name != checkName {
					continue
				}

				var data map[string]interface{}

				err = yaml.Unmarshal(conf.InitConfig, &data)
				if err != nil {
					return err
				}

				if data == nil {
					data = make(map[string]interface{})
				}

				data["set_breakpoint"] = breakPointLine

				y, _ := yaml.Marshal(data)
				conf.InitConfig = y

				break
			}
		}

		cs := collector.GetChecksByNameForConfigs(checkName, allConfigs)

		// something happened while getting the check(s), display some info.
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

		var instancesData []interface{}
		for _, c := range cs {
			s := runCheck(c, agg)

			// Sleep for a while to allow the aggregator to finish ingesting all the metrics/events/sc
			time.Sleep(time.Duration(checkDelay) * time.Millisecond)

			if formatJSON {
				aggregatorData := getMetricsData(agg)
				var collectorData map[string]interface{}

				collectorJSON, _ := status.GetCheckStatusJSON(c, s)
				err = json.Unmarshal(collectorJSON, &collectorData)
				if err != nil {
					return err
				}

				checkRuns := collectorData["runnerStats"].(map[string]interface{})["Checks"].(map[string]interface{})[checkName].(map[string]interface{})

				// There is only one checkID per run so we'll just access that
				var runnerData map[string]interface{}
				for _, checkIDData := range checkRuns {
					runnerData = checkIDData.(map[string]interface{})
					break
				}

				instanceData := map[string]interface{}{
					"aggregator":  aggregatorData,
					"runner":      runnerData,
					"inventories": collectorData["inventories"],
				}
				instancesData = append(instancesData, instanceData)
			} else if profileMemory {
				// Every instance will create its own directory
				instanceID := strings.SplitN(string(c.ID()), ":", 2)[1]
				// Colons can't be part of Windows file paths
				instanceID = strings.Replace(instanceID, ":", "_", -1)
				profileDataDir := filepath.Join(profileMemoryDir, checkName, instanceID)

				snapshotDir := filepath.Join(profileDataDir, "snapshots")
				if _, err := os.Stat(snapshotDir); !os.IsNotExist(err) {
					snapshots, err := ioutil.ReadDir(snapshotDir)
					if err != nil {
						return err
					}

					numSnapshots := len(snapshots)
					if numSnapshots > 0 {
						lastSnapshot := snapshots[numSnapshots-1]
						snapshotContents, err := ioutil.ReadFile(filepath.Join(snapshotDir, lastSnapshot.Name()))
						if err != nil {
							return err
						}

						color.HiWhite(string(snapshotContents))
					} else {
						return fmt.Errorf("no snapshots found in %s", snapshotDir)
					}
				} else {
					return fmt.Errorf("no snapshot data found in %s", profileDataDir)
				}

				diffDir := filepath.Join(profileDataDir, "diffs")
				if _, err := os.Stat(diffDir); !os.IsNotExist(err) {
					diffs, err := ioutil.ReadDir(diffDir)
					if err != nil {
						return err
					}

					numDiffs := len(diffs)
					if numDiffs > 0 {
						lastDiff := diffs[numDiffs-1]
						diffContents, err := ioutil.ReadFile(filepath.Join(diffDir, lastDiff.Name()))
						if err != nil {
							return err
						}

						color.HiCyan(fmt.Sprintf("\n%s\n\n", strings.Repeat("=", 50)))
						color.HiWhite(string(diffContents))
					} else {
						return fmt.Errorf("no diffs found in %s", diffDir)
					}
				} else if !singleCheckRun() {
					return fmt.Errorf("no diff data found in %s", profileDataDir)
				}
			} else {
				printMetrics(agg)
				checkStatus, _ := status.GetCheckStatus(c, s)
				fmt.Println(string(checkStatus))
			}
		}

		if runtime.GOOS == "windows" {
			printWindowsUserWarning("check")
		}

		if formatJSON {
			fmt.Fprintln(color.Output, fmt.Sprintf("=== %s ===", color.BlueString("JSON")))
			instancesJSON, _ := json.MarshalIndent(instancesData, "", "  ")
			fmt.Println(string(instancesJSON))
		} else if singleCheckRun() {
			if profileMemory {
				color.Yellow("Check has run only once, to collect diff data run the check multiple times with the -t/--check-times flag.")
			} else {
				color.Yellow("Check has run only once, if some metrics are missing you can try again with --check-rate to see any other metric if available.")
			}
		}

		return nil
	},
}

func runCheck(c check.Check, agg *aggregator.BufferedAggregator) *check.Stats {
	s := check.NewStats(c)
	times := checkTimes
	pause := checkPause
	if checkRate {
		if checkTimes > 2 {
			color.Yellow("The check-rate option is overriding check-times to 2")
		}
		if pause > 0 {
			color.Yellow("The check-rate option is overriding pause to 1000ms")
		}
		times = 2
		pause = 1000
	}
	for i := 0; i < times; i++ {
		t0 := time.Now()
		err := c.Run()
		warnings := c.GetWarnings()
		mStats, _ := c.GetMetricStats()
		s.Add(time.Since(t0), err, warnings, mStats)
		if pause > 0 && i < times-1 {
			time.Sleep(time.Duration(pause) * time.Millisecond)
		}
	}

	return s
}

func printMetrics(agg *aggregator.BufferedAggregator) {
	series, sketches := agg.GetSeriesAndSketches()
	if len(series) != 0 {
		fmt.Fprintln(color.Output, fmt.Sprintf("=== %s ===", color.BlueString("Series")))
		j, _ := json.MarshalIndent(series, "", "  ")
		fmt.Println(string(j))
	}
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

func getMetricsData(agg *aggregator.BufferedAggregator) map[string]interface{} {
	aggData := make(map[string]interface{})

	series, sketches := agg.GetSeriesAndSketches()
	if len(series) != 0 {
		// Workaround to get the raw sequence of metrics, see:
		// https://github.com/DataDog/datadog-agent/blob/b2d9527ec0ec0eba1a7ae64585df443c5b761610/pkg/metrics/series.go#L109-L122
		var data map[string]interface{}
		sj, _ := json.Marshal(series)
		json.Unmarshal(sj, &data)

		aggData["metrics"] = data["series"]
	}
	if len(sketches) != 0 {
		aggData["sketches"] = sketches
	}

	serviceChecks := agg.GetServiceChecks()
	if len(serviceChecks) != 0 {
		aggData["service_checks"] = serviceChecks
	}

	events := agg.GetEvents()
	if len(events) != 0 {
		aggData["events"] = events
	}

	return aggData
}
func printWindowsUserWarning(op string) {
	fmt.Printf("\nNOTE:\n")
	fmt.Printf("The %s command runs in a different user context than the running service\n", op)
	fmt.Printf("This could affect results if the command relies on specific permissions and/or user contexts\n")
	fmt.Printf("\n")
}

func singleCheckRun() bool {
	return checkRate == false && checkTimes < 2
}

func createHiddenStringFlag(p *string, name string, value string, usage string) {
	checkCmd.Flags().StringVar(p, name, value, usage)
	checkCmd.Flags().MarkHidden(name)
}

func populateMemoryProfileConfig(initConfig map[string]interface{}) error {
	if profileMemoryFrames != "" {
		profileMemoryFrames, err := strconv.Atoi(profileMemoryFrames)
		if err != nil {
			return fmt.Errorf("--m-frames must be an integer")
		}
		initConfig["profile_memory_frames"] = profileMemoryFrames
	}

	if profileMemoryGC != "" {
		profileMemoryGC, err := strconv.Atoi(profileMemoryGC)
		if err != nil {
			return fmt.Errorf("--m-gc must be an integer")
		}

		initConfig["profile_memory_gc"] = profileMemoryGC
	}

	if profileMemoryCombine != "" {
		profileMemoryCombine, err := strconv.Atoi(profileMemoryCombine)
		if err != nil {
			return fmt.Errorf("--m-combine must be an integer")
		}

		if profileMemoryCombine != 0 && profileMemorySort == "traceback" {
			return fmt.Errorf("--m-combine cannot be sorted (--m-sort) by traceback")
		}

		initConfig["profile_memory_combine"] = profileMemoryCombine
	}

	if profileMemorySort != "" {
		if profileMemorySort != "lineno" && profileMemorySort != "filename" && profileMemorySort != "traceback" {
			return fmt.Errorf("--m-sort must one of: lineno | filename | traceback")
		}
		initConfig["profile_memory_sort"] = profileMemorySort
	}

	if profileMemoryLimit != "" {
		profileMemoryLimit, err := strconv.Atoi(profileMemoryLimit)
		if err != nil {
			return fmt.Errorf("--m-limit must be an integer")
		}
		initConfig["profile_memory_limit"] = profileMemoryLimit
	}

	if profileMemoryDiff != "" {
		if profileMemoryDiff != "absolute" && profileMemoryDiff != "positive" {
			return fmt.Errorf("--m-diff must one of: absolute | positive")
		}
		initConfig["profile_memory_diff"] = profileMemoryDiff
	}

	if profileMemoryFilters != "" {
		initConfig["profile_memory_filters"] = profileMemoryFilters
	}

	if profileMemoryUnit != "" {
		initConfig["profile_memory_unit"] = profileMemoryUnit
	}

	if profileMemoryVerbose != "" {
		profileMemoryVerbose, err := strconv.Atoi(profileMemoryVerbose)
		if err != nil {
			return fmt.Errorf("--m-verbose must be an integer")
		}
		initConfig["profile_memory_verbose"] = profileMemoryVerbose
	}

	return nil
}
