// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package check builds a 'check' command to be used in binaries.
package check

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	apiTypes "github.com/DataDog/datadog-agent/comp/api/api/types"
	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	authtokenimpl "github.com/DataDog/datadog-agent/comp/api/authtoken/createandfetchimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks/inventorychecksimpl"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/cli/standalone"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	// cmd is the running cobra.Command
	cmd *cobra.Command

	// args are the positional command line args
	args []string

	// subcommand-specific params

	checkRate                 bool
	checkTimes                int
	checkPause                int
	checkName                 string
	checkDelay                int
	instanceFilter            string
	logLevel                  string
	formatJSON                bool
	formatTable               bool
	breakPoint                string
	fullSketches              bool
	saveFlare                 bool
	profileMemory             bool
	profileMemoryDir          string
	profileMemoryFrames       string
	profileMemoryGC           string
	profileMemoryCombine      string
	profileMemorySort         string
	profileMemoryLimit        string
	profileMemoryDiff         string
	profileMemoryFilters      string
	profileMemoryUnit         string
	profileMemoryVerbose      string
	discoveryTimeout          uint
	discoveryRetryInterval    uint
	discoveryMinInstances     uint
	generateIntegrationTraces bool
}

// GlobalParams contains the values of agent-global Cobra flags.
//
// A pointer to this type is passed to SubcommandFactory's, but its contents
// are not valid until Cobra calls the subcommand's Run or RunE function.
type GlobalParams struct {
	ConfFilePath         string
	ExtraConfFilePaths   []string
	SysProbeConfFilePath string
	FleetPoliciesDirPath string
	ConfigName           string
	LoggerName           string
}

// MakeCommand returns a `check` command to be used by agent binaries.
func MakeCommand(globalParamsGetter func() GlobalParams) *cobra.Command {
	cliParams := &cliParams{}
	cmd := &cobra.Command{
		Use:   "check <check_name>",
		Short: "Run the specified check",
		Long:  `Use this to run a specific check with a specific rate`,
		RunE: func(cmd *cobra.Command, args []string) error {
			globalParams := globalParamsGetter()

			// handle the deprecated --log-level flag
			if cliParams.logLevel != "" {
				os.Setenv("DD_LOG_LEVEL", cliParams.logLevel)
			}
			cliParams.cmd = cmd
			cliParams.args = args

			disableCmdPort()
			return fxutil.OneShot(run,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams:         config.NewAgentParams(globalParams.ConfFilePath, config.WithConfigName(globalParams.ConfigName), config.WithExtraConfFiles(globalParams.ExtraConfFilePaths), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					SecretParams:         secrets.NewEnabledParams(),
					SysprobeConfigParams: sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath), sysprobeconfigimpl.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:            log.ForOneShot(globalParams.LoggerName, "off", true)}),
				core.Bundle(),
				authtokenimpl.Module(),
				fx.Supply(context.Background()),
				getPlatformModules(),
			)
		},
	}

	cmd.Flags().BoolVarP(&cliParams.checkRate, "check-rate", "r", false, "check rates by running the check twice with a 1sec-pause between the 2 runs")
	cmd.Flags().IntVarP(&cliParams.checkTimes, "check-times", "t", 1, "number of times to run the check")
	cmd.Flags().IntVar(&cliParams.checkPause, "pause", 0, "pause between multiple runs of the check, in milliseconds")
	cmd.Flags().StringVarP(&cliParams.logLevel, "log-level", "l", "", "set the log level (default 'off') (deprecated, use the env var DD_LOG_LEVEL instead)")
	cmd.Flags().IntVarP(&cliParams.checkDelay, "delay", "d", 100, "delay between running the check and grabbing the metrics in milliseconds")
	cmd.Flags().StringVarP(&cliParams.instanceFilter, "instance-filter", "", "", "filter instances using jq style syntax, example: --instance-filter '.ip_address == \"127.0.0.51\"'")
	cmd.Flags().BoolVarP(&cliParams.formatJSON, "json", "", false, "format aggregator and check runner output as json")
	cmd.Flags().BoolVarP(&cliParams.formatTable, "table", "", false, "format aggregator and check runner output as an ascii table")
	cmd.Flags().StringVarP(&cliParams.breakPoint, "breakpoint", "b", "", "set a breakpoint at a particular line number (Python checks only)")
	cmd.Flags().BoolVarP(&cliParams.profileMemory, "profile-memory", "m", false, "run the memory profiler (Python checks only)")
	cmd.Flags().BoolVar(&cliParams.fullSketches, "full-sketches", false, "output sketches with bins information")
	cmd.Flags().BoolVarP(&cliParams.saveFlare, "flare", "", false, "save check results to the log dir so it may be reported in a flare")
	cmd.Flags().UintVarP(&cliParams.discoveryTimeout, "discovery-timeout", "", 5, "max retry duration until Autodiscovery resolves the check template (in seconds)")
	cmd.Flags().UintVarP(&cliParams.discoveryRetryInterval, "discovery-retry-interval", "", 1, "(unused)")
	cmd.Flags().UintVarP(&cliParams.discoveryMinInstances, "discovery-min-instances", "", 1, "minimum number of config instances to be discovered before running the check(s)")

	// Power user flags - mark as hidden
	createHiddenStringFlag(cmd, &cliParams.profileMemoryDir, "m-dir", "", "an existing directory in which to store memory profiling data, ignoring clean-up")
	createHiddenStringFlag(cmd, &cliParams.profileMemoryFrames, "m-frames", "", "the number of stack frames to consider")
	createHiddenStringFlag(cmd, &cliParams.profileMemoryGC, "m-gc", "", "whether or not to run the garbage collector to remove noise")
	createHiddenStringFlag(cmd, &cliParams.profileMemoryCombine, "m-combine", "", "whether or not to aggregate over all traceback frames")
	createHiddenStringFlag(cmd, &cliParams.profileMemorySort, "m-sort", "", "what to sort by between: lineno | filename | traceback")
	createHiddenStringFlag(cmd, &cliParams.profileMemoryLimit, "m-limit", "", "the maximum number of sorted results to show")
	createHiddenStringFlag(cmd, &cliParams.profileMemoryDiff, "m-diff", "", "how to order diff results between: absolute | positive")
	createHiddenStringFlag(cmd, &cliParams.profileMemoryFilters, "m-filters", "", "comma-separated list of file path glob patterns to filter by")
	createHiddenStringFlag(cmd, &cliParams.profileMemoryUnit, "m-unit", "", "the binary unit to represent memory usage (kib, mb, etc.). the default is dynamic")
	createHiddenStringFlag(cmd, &cliParams.profileMemoryVerbose, "m-verbose", "", "whether or not to include potentially noisy sources")
	createHiddenBooleanFlag(cmd, &cliParams.generateIntegrationTraces, "m-trace", false, "send the integration traces")

	cmd.SetArgs([]string{"checkName"})

	return cmd
}

func run(
	config config.Component,
	cliParams *cliParams,
	_ authtoken.Component,
) error {
	previousIntegrationTracing := false
	previousIntegrationTracingExhaustive := false
	if cliParams.generateIntegrationTraces {
		if pkgconfigsetup.Datadog().IsSet("integration_tracing") {
			previousIntegrationTracing = pkgconfigsetup.Datadog().GetBool("integration_tracing")

		}
		if pkgconfigsetup.Datadog().IsSet("integration_tracing_exhaustive") {
			previousIntegrationTracingExhaustive = pkgconfigsetup.Datadog().GetBool("integration_tracing_exhaustive")
		}
		pkgconfigsetup.Datadog().Set("integration_tracing", true, model.SourceAgentRuntime)
		pkgconfigsetup.Datadog().Set("integration_tracing_exhaustive", true, model.SourceAgentRuntime)
	}

	if len(cliParams.args) != 0 {
		cliParams.checkName = cliParams.args[0]
	} else {
		cliParams.cmd.Help() //nolint:errcheck
		return nil
	}

	if cliParams.profileMemory {
		// If no directory is specified, make a temporary one
		var err error
		if cliParams.profileMemoryDir == "" {
			cliParams.profileMemoryDir, err = os.MkdirTemp("", "datadog-agent-memory-profiler")
			if err != nil {
				return err
			}

			defer func() {
				cleanupErr := os.RemoveAll(cliParams.profileMemoryDir)
				if cleanupErr != nil {
					fmt.Printf("%s\n", cleanupErr)
				}
			}()
		}
	}

	checkRequest := apiTypes.CheckRequest{
		Name:  cliParams.checkName,
		Times: cliParams.checkTimes,
		Pause: cliParams.checkPause,
		Delay: cliParams.checkDelay,
		ProfileConfig: apiTypes.MemoryProfileConfig{
			Dir:     cliParams.profileMemoryDir,
			Frames:  cliParams.profileMemoryFrames,
			GC:      cliParams.profileMemoryGC,
			Combine: cliParams.profileMemoryCombine,
			Sort:    cliParams.profileMemorySort,
			Limit:   cliParams.profileMemoryLimit,
			Diff:    cliParams.profileMemoryDiff,
			Filters: cliParams.profileMemoryFilters,
			Unit:    cliParams.profileMemoryUnit,
			Verbose: cliParams.profileMemoryVerbose,
		},
		Breakpoint: cliParams.breakPoint,
	}

	ipcAddress, err := pkgconfigsetup.GetIPCAddress(config)
	if err != nil {
		return err
	}

	c := util.GetClient(false) // FIX: get certificates right then make this true
	// TODO fix port number
	urlstr := fmt.Sprintf("https://%v:%v/check/run", ipcAddress, 5001)

	postData, err := json.Marshal(checkRequest)

	if err != nil {
		return fmt.Errorf("error marshalling request: %v", err)
	}

	r, err := util.DoPost(c, urlstr, "application/json", bytes.NewBuffer(postData))

	result := apiTypes.CheckResponse{}

	marshalErr := json.Unmarshal(r, &result)

	if marshalErr != nil {
		return fmt.Errorf("error unmarshalling response: %v", marshalErr)
	}

	if err != nil {
		for _, error := range result.Errors {
			fmt.Fprintf(color.Output, "\n%s: running %s check: %s\n", color.RedString("Error"), color.YellowString(cliParams.checkName), error)
		}

		for _, warning := range result.Warnings {
			fmt.Fprintf(color.Output, "\n%s: running %s check: %s\n", color.YellowString("Warning"), color.YellowString(cliParams.checkName), warning)
		}
		return nil
	}

	// TODO: fix hardcoded port
	apiConfigURL := fmt.Sprintf("https://%v:%d/agent/metadata/inventory-checks",
		ipcAddress, 5001)

	r, err = util.DoGet(c, apiConfigURL, util.CloseConnection)

	if err != nil {
		return fmt.Errorf("Could not fetch metadata payload: %s", err)
	}

	inventoryChecksPayload := inventorychecksimpl.Payload{}

	marshalErr = json.Unmarshal(r, &inventoryChecksPayload)

	if marshalErr != nil {
		return fmt.Errorf("error unmarshalling response: %v", marshalErr)
	}

	var checkFileOutput bytes.Buffer
	var instancesData []interface{}

	for _, c := range result.Results {
		inventoryData := map[string]interface{}{}

		for _, metadata := range inventoryChecksPayload.Metadata[string(c.CheckID)] {
			for k, v := range metadata {
				inventoryData[k] = v
			}
		}

		if cliParams.formatJSON {
			// There is only one checkID per run so we'll just access that
			instanceData := map[string]interface{}{
				"aggregator": result.AggregatorData,
				"runner":     c,
				"inventory":  inventoryData,
			}
			instancesData = append(instancesData, instanceData)
		} else if cliParams.profileMemory {
			// Every instance will create its own directory
			instanceID := strings.SplitN(string(c.CheckID), ":", 2)[1]
			// Colons can't be part of Windows file paths
			instanceID = strings.Replace(instanceID, ":", "_", -1)
			profileDataDir := filepath.Join(cliParams.profileMemoryDir, cliParams.checkName, instanceID)

			snapshotDir := filepath.Join(profileDataDir, "snapshots")
			if _, err := os.Stat(snapshotDir); !os.IsNotExist(err) {
				snapshots, err := os.ReadDir(snapshotDir)
				if err != nil {
					return err
				}

				numSnapshots := len(snapshots)
				if numSnapshots > 0 {
					lastSnapshot := snapshots[numSnapshots-1]
					snapshotContents, err := os.ReadFile(filepath.Join(snapshotDir, lastSnapshot.Name()))
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
				diffs, err := os.ReadDir(diffDir)
				if err != nil {
					return err
				}

				numDiffs := len(diffs)
				if numDiffs > 0 {
					lastDiff := diffs[numDiffs-1]
					diffContents, err := os.ReadFile(filepath.Join(diffDir, lastDiff.Name()))
					if err != nil {
						return err
					}

					color.HiCyan(fmt.Sprintf("\n%s\n\n", strings.Repeat("=", 50)))
					color.HiWhite(string(diffContents))
				} else {
					return fmt.Errorf("no diffs found in %s", diffDir)
				}
			} else if !singleCheckRun(cliParams) {
				return fmt.Errorf("no diff data found in %s", profileDataDir)
			}
		} else {
			printMetrics(result.AggregatorData, &checkFileOutput, cliParams.formatTable)

			p := func(data string) {
				fmt.Println(data)
				checkFileOutput.WriteString(data + "\n")
			}

			if c.LongRunning {
				p(longRunningCheckTemplate(c))
			} else {
				p(checkTemplate(c))
			}

			p("  Metadata\n  ========")

			for k, v := range result.Metadata[string(c.CheckID)] {
				p(fmt.Sprintf("    %s: %v", k, v))
			}
			for k, v := range inventoryData {
				p(fmt.Sprintf("    %s: %v", k, v))
			}
		}
	}

	if runtime.GOOS == "windows" {
		standalone.PrintWindowsUserWarning("check")
	}

	if cliParams.formatJSON {
		instancesJSON, _ := json.MarshalIndent(instancesData, "", "  ")
		instanceJSONString := string(instancesJSON)

		fmt.Println(instanceJSONString)
		checkFileOutput.WriteString(instanceJSONString + "\n")
	} else if singleCheckRun(cliParams) {
		if cliParams.profileMemory {
			color.Yellow("Check has run only once, to collect diff data run the check multiple times with the -t/--check-times flag.")
		} else {
			color.Yellow("Check has run only once, if some metrics are missing you can try again with --check-rate to see any other metric if available.")
		}
		color.Yellow("This check type has %d instances. If you're looking for a different check instance, try filtering on a specific one using the --instance-filter flag or set --discovery-min-instances to a higher value", len(result.Results))
	}

	warnings := config.Warnings()
	if warnings != nil && warnings.TraceMallocEnabledWithPy2 {
		return errors.New("tracemalloc is enabled but unavailable with python version 2")
	}

	if cliParams.saveFlare {
		writeCheckToFile(cliParams.checkName, &checkFileOutput)
	}

	if cliParams.generateIntegrationTraces {
		pkgconfigsetup.Datadog().Set("integration_tracing", previousIntegrationTracing, model.SourceAgentRuntime)
		pkgconfigsetup.Datadog().Set("integration_tracing_exhaustive", previousIntegrationTracingExhaustive, model.SourceAgentRuntime)
	}

	return nil
}

var checksTemplate = `
%s
%s

Instance ID: %s %s
Configuration Source: %s
Total Runs: %s
Metric Samples: Last Run: %s, Total: %s
Events: Last Run: %s, Total: %s
%s
Service Checks: Last Run: %s, Total: %s
Histogram Buckets: Last Run: %s, Total: %s
Average Execution Time :%s
Last Execution Date : %s
Last Successful Execution Date : %s
Cancelling: %t
%s
%s
`

func checkTemplate(c *stats.Stats) string {
	name := c.CheckName

	if c.CheckVersion != "" {
		name = fmt.Sprintf("%s (%s)", name, c.CheckVersion)
	}

	averageExcution, _ := time.ParseDuration(fmt.Sprintf("%dms", c.AverageExecutionTime))

	lastSuccesfulExecution := "Never"
	if c.LastSuccessDate > 0 {
		lastSuccesfulExecution = formatUnixTime(c.LastSuccessDate)
	}

	lastExcutionDate := formatUnixTime(c.UpdateTimestamp)
	var eventPlatformEvents string
	for instance, value := range c.TotalEventPlatformEvents {
		eventPlatformEvents += fmt.Sprintf("%s: Last Run: %s, Total: %s\n", instance, humanize.Commaf(float64(c.EventPlatformEvents[instance])), humanize.Commaf(float64(value)))
	}

	var errorString string

	if c.LastError != "" {
		var lastErrorArray []map[string]string
		err := json.Unmarshal([]byte(c.LastError), &lastErrorArray)
		if err != nil {
			errorString = fmt.Sprintf("Error: %s\n", lastErrorArray[0]["message"])
			errorString += lastErrorArray[0]["traceback"]
		}
	}

	var warningString string

	if len(c.LastWarnings) > 0 {
		for _, warning := range c.LastWarnings {
			warningString += fmt.Sprintf("Warning: %s\n", warning)
		}
	}

	return fmt.Sprintf(
		checksTemplate,
		name,
		strings.Repeat("-", len(name)),
		c.CheckID,
		status(c),
		c.CheckConfigSource,
		humanize.Commaf(float64(c.TotalRuns)),
		humanize.Commaf(float64(c.MetricSamples)),
		humanize.Commaf(float64(c.TotalMetricSamples)),
		humanize.Commaf(float64(c.Events)),
		humanize.Commaf(float64(c.TotalEvents)),
		eventPlatformEvents,
		humanize.Commaf(float64(c.ServiceChecks)),
		humanize.Commaf(float64(c.TotalServiceChecks)),
		humanize.Commaf(float64(c.HistogramBuckets)),
		humanize.Commaf(float64(c.TotalHistogramBuckets)),
		averageExcution.String(),
		lastExcutionDate,
		lastSuccesfulExecution,
		c.Cancelling,
		errorString,
		warningString,
	)
}

var longRunningChecksTemplate = `
%s
%s

Instance ID: %s %s
Long Running Check: true
Configuration Source: %s
Total Metric Samples: %s
Total Events: %s
%s
Total Service Checks: %s
Total Histogram Buckets: %s
%s
%s
`

func longRunningCheckTemplate(c *stats.Stats) string {
	name := c.CheckName

	if c.CheckVersion != "" {
		name = fmt.Sprintf("%s (%s)", name, c.CheckVersion)
	}

	var eventPlatformEvents string
	for instance, value := range c.TotalEventPlatformEvents {
		eventPlatformEvents += fmt.Sprintf("Total %s: %s\n", instance, humanize.Commaf(float64(value)))
	}

	var errorString string

	if c.LastError != "" {
		var lastErrorArray []map[string]string
		err := json.Unmarshal([]byte(c.LastError), &lastErrorArray)
		if err != nil {
			errorString = fmt.Sprintf("Error: %s\n", lastErrorArray[0]["message"])
			errorString += lastErrorArray[0]["traceback"]
		}
	}

	var warningString string

	if len(c.LastWarnings) > 0 {
		for _, warning := range c.LastWarnings {
			warningString += fmt.Sprintf("Warning: %s\n", warning)
		}
	}

	return fmt.Sprintf(
		longRunningChecksTemplate,
		name,
		strings.Repeat("-", len(name)),
		c.CheckID,
		status(c),
		c.CheckConfigSource,
		humanize.Commaf(float64(c.TotalMetricSamples)),
		humanize.Commaf(float64(c.TotalEvents)),
		eventPlatformEvents,
		humanize.Commaf(float64(c.TotalServiceChecks)),
		humanize.Commaf(float64(c.TotalHistogramBuckets)),
		errorString,
		warningString,
	)
}

func status(c *stats.Stats) string {
	if c.LastError != "" {
		return fmt.Sprintf("[%s]", color.RedString("ERROR"))
	}
	if len(c.LastWarnings) != 0 {
		return fmt.Sprintf("[%s]", color.YellowString("WARNING"))
	}
	return fmt.Sprintf("[%s]", color.GreenString("OK"))
}

const timeFormat = "2006-01-02 15:04:05.999 MST"

// formatUnixTime formats the unix time to make it more readable
func formatUnixTime(rawUnixTime int64) string {
	t := time.Unix(0, rawUnixTime)
	// If year returned 1970, assume unixTime actually in seconds
	if t.Year() == time.Unix(0, 0).Year() {
		t = time.Unix(rawUnixTime, 0)
	}

	_, tzoffset := t.Zone()
	result := t.Format(timeFormat)
	if tzoffset != 0 {
		result += " / " + t.UTC().Format(timeFormat)
	}
	msec := t.UnixNano() / int64(time.Millisecond)
	result += " (" + strconv.Itoa(int(msec)) + ")"

	return result
}

func writeCheckToFile(checkName string, checkFileOutput *bytes.Buffer) {
	_ = os.Mkdir(defaultpaths.CheckFlareDirectory, os.ModeDir)

	// Windows cannot accept ":" in file names
	filenameSafeTimeStamp := strings.ReplaceAll(time.Now().UTC().Format(time.RFC3339), ":", "-")
	flarePath := filepath.Join(defaultpaths.CheckFlareDirectory, "check_"+checkName+"_"+filenameSafeTimeStamp+".log")

	scrubbed, err := scrubber.ScrubBytes(checkFileOutput.Bytes())
	if err != nil {
		fmt.Println("Error while scrubbing the check file:", err)
	}
	err = os.WriteFile(flarePath, scrubbed, os.ModePerm)

	if err != nil {
		fmt.Println("Error while writing the check file (is the location writable by the dd-agent user?):", err)
	} else {
		fmt.Println("check written to:", flarePath)
	}
}

func singleCheckRun(cliParams *cliParams) bool {
	return !cliParams.checkRate && cliParams.checkTimes < 2
}

func createHiddenStringFlag(cmd *cobra.Command, p *string, name string, value string, usage string) {
	cmd.Flags().StringVar(p, name, value, usage)
	cmd.Flags().MarkHidden(name) //nolint:errcheck
}

func createHiddenBooleanFlag(cmd *cobra.Command, p *bool, name string, value bool, usage string) {
	cmd.Flags().BoolVar(p, name, value, usage)
	cmd.Flags().MarkHidden(name) //nolint:errcheck
}

func printMetrics(aggregatorData apiTypes.AggregatorData, checkFileOutput *bytes.Buffer, formatTable bool) {
	if len(aggregatorData.Series) != 0 {
		fmt.Fprintf(color.Output, "=== %s ===\n", color.BlueString("Series"))

		if formatTable {
			headers, data := aggregatorData.Series.MarshalStrings()
			var buffer bytes.Buffer

			// plain table with no borders
			table := tablewriter.NewWriter(&buffer)
			table.SetHeader(headers)
			table.SetAutoWrapText(false)
			table.SetAutoFormatHeaders(true)
			table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
			table.SetAlignment(tablewriter.ALIGN_LEFT)
			table.SetCenterSeparator("")
			table.SetColumnSeparator("")
			table.SetRowSeparator("")
			table.SetHeaderLine(false)
			table.SetBorder(false)
			table.SetTablePadding("\t")

			table.AppendBulk(data)
			table.Render()
			fmt.Println(buffer.String())
			checkFileOutput.WriteString(buffer.String() + "\n")
		} else {
			j, _ := json.MarshalIndent(aggregatorData.Series, "", "  ")
			fmt.Println(string(j))
			checkFileOutput.WriteString(string(j) + "\n")
		}
	}
	if len(aggregatorData.SketchSeries) != 0 {
		fmt.Fprintf(color.Output, "=== %s ===\n", color.BlueString("Sketches"))
		j, _ := json.MarshalIndent(aggregatorData.SketchSeries, "", "  ")
		fmt.Println(string(j))
		checkFileOutput.WriteString(string(j) + "\n")
	}

	if len(aggregatorData.ServiceCheck) != 0 {
		fmt.Fprintf(color.Output, "=== %s ===\n", color.BlueString("Service Checks"))

		if formatTable {
			headers, data := aggregatorData.ServiceCheck.MarshalStrings()
			var buffer bytes.Buffer

			// plain table with no borders
			table := tablewriter.NewWriter(&buffer)
			table.SetHeader(headers)
			table.SetAutoWrapText(false)
			table.SetAutoFormatHeaders(true)
			table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
			table.SetAlignment(tablewriter.ALIGN_LEFT)
			table.SetCenterSeparator("")
			table.SetColumnSeparator("")
			table.SetRowSeparator("")
			table.SetHeaderLine(false)
			table.SetBorder(false)
			table.SetTablePadding("\t")

			table.AppendBulk(data)
			table.Render()
			fmt.Println(buffer.String())
			checkFileOutput.WriteString(buffer.String() + "\n")
		} else {
			j, _ := json.MarshalIndent(aggregatorData.ServiceCheck, "", "  ")
			fmt.Println(string(j))
			checkFileOutput.WriteString(string(j) + "\n")
		}
	}

	if len(aggregatorData.Events) != 0 {
		fmt.Fprintf(color.Output, "=== %s ===\n", color.BlueString("Events"))
		checkFileOutput.WriteString("=== Events ===\n")
		j, _ := json.MarshalIndent(aggregatorData.Events, "", "  ")
		fmt.Println(string(j))
		checkFileOutput.WriteString(string(j) + "\n")
	}

	for k, v := range aggregatorData.EventPlatformEvents {
		if len(v) > 0 {
			if translated, ok := stats.EventPlatformNameTranslations[k]; ok {
				k = translated
			}
			fmt.Fprintf(color.Output, "=== %s ===\n", color.BlueString(k))
			checkFileOutput.WriteString(fmt.Sprintf("=== %s ===\n", k))
			j, _ := json.MarshalIndent(v, "", "  ")
			fmt.Println(string(j))
			checkFileOutput.WriteString(string(j) + "\n")
		}
	}
}

// disableCmdPort overrrides the `cmd_port` configuration so that when the
// server starts up, it does not do so on the same port as a running agent.
//
// Ideally, the server wouldn't start up at all, but this workaround has been
// in place for some time.
func disableCmdPort() {
	os.Setenv("DD_CMD_PORT", "0") // 0 indicates the OS should pick an unused port
}
