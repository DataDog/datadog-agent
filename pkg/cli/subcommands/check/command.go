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
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger"
	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger/jmxloggerimpl"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl"
	internalAPI "github.com/DataDog/datadog-agent/comp/api/api/def"
	apiTypes "github.com/DataDog/datadog-agent/comp/api/api/types"
	authtokenimpl "github.com/DataDog/datadog-agent/comp/api/authtoken/createandfetchimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	dualTaggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-dual"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/defaults"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	orchestratorForwarderImpl "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	haagentfx "github.com/DataDog/datadog-agent/comp/haagent/fx"
	logagent "github.com/DataDog/datadog-agent/comp/logs/agent"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks/inventorychecksimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/cli/standalone"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
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

			eventplatforParams := eventplatformimpl.NewDefaultParams()
			eventplatforParams.UseNoopEventPlatformForwarder = true

			disableCmdPort()
			return fxutil.OneShot(run,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams:         config.NewAgentParams(globalParams.ConfFilePath, config.WithConfigName(globalParams.ConfigName), config.WithExtraConfFiles(globalParams.ExtraConfFilePaths), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					SecretParams:         secrets.NewEnabledParams(),
					SysprobeConfigParams: sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath), sysprobeconfigimpl.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:            log.ForOneShot(globalParams.LoggerName, "off", true)}),
				core.Bundle(),

				// workloadmeta setup
				wmcatalog.GetCatalog(),
				workloadmetafx.Module(defaults.DefaultParams()),
				apiimpl.Module(),
				authtokenimpl.Module(),
				fx.Supply(context.Background()),
				dualTaggerfx.Module(common.DualTaggerParams()),
				autodiscoveryimpl.Module(),
				forwarder.Bundle(defaultforwarder.NewParams(defaultforwarder.WithNoopForwarder())),
				inventorychecksimpl.Module(),
				logscompression.Module(),
				metricscompression.Module(),
				// inventorychecksimpl depends on a collector and serializer when created to send payload.
				// Here we just want to collect metadata to be displayed, so we don't need a collector.
				collector.NoneModule(),
				fx.Provide(func() serializer.MetricSerializer { return nil }),
				// Initializing the aggregator with a flush interval of 0 (to disable the flush goroutines)
				demultiplexerimpl.Module(demultiplexerimpl.NewDefaultParams(demultiplexerimpl.WithFlushInterval(0))),
				orchestratorForwarderImpl.Module(orchestratorForwarderImpl.NewNoopParams()),
				eventplatformimpl.Module(eventplatforParams),
				eventplatformreceiverimpl.Module(),
				// The check command do not have settings that change are runtime
				// still, we need to pass it to ensure the API server is proprely initialized
				settingsimpl.Module(),
				fx.Supply(settings.Params{}),
				// TODO(components): this is a temporary hack as the StartServer() method of the API package was previously called with nil arguments
				// This highlights the fact that the API Server created by JMX (through ExecJmx... function) should be different from the ones created
				// in others commands such as run.
				fx.Supply(option.None[rcservice.Component]()),
				fx.Supply(option.None[rcservicemrf.Component]()),
				fx.Supply(option.None[logagent.Component]()),
				fx.Supply(option.None[integrations.Component]()),
				fx.Provide(func() server.Component { return nil }),
				fx.Provide(func() replay.Component { return nil }),
				fx.Provide(func() pidmap.Component { return nil }),
				fx.Provide(func() remoteagentregistry.Component { return nil }),

				getPlatformModules(),
				jmxloggerimpl.Module(jmxloggerimpl.NewDisabledParams()),
				haagentfx.Module(),
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
	demultiplexer demultiplexer.Component,
	wmeta workloadmeta.Component,
	tagger tagger.Component,
	ac autodiscovery.Component,
	secretResolver secrets.Component,
	agentAPI internalAPI.Component,
	invChecks inventorychecks.Component,
	collector option.Option[collector.Component],
	jmxLogger jmxlogger.Component,
	telemetry telemetry.Component,
	logReceiver option.Option[integrations.Component],
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

	fmt.Println(string(r))

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
	printer := aggregator.AgentDemultiplexerPrinter{DemultiplexerWithAggregator: demultiplexer}

	for _, c := range result.Results {
		inventoryData := map[string]interface{}{}

		for _, metadata := range inventoryChecksPayload.Metadata[string(c.CheckID)] {
			for k, v := range metadata {
				inventoryData[k] = v
			}
		}

		if cliParams.formatJSON {
			aggregatorData := printer.GetMetricsDataForPrint()

			// There is only one checkID per run so we'll just access that
			instanceData := map[string]interface{}{
				"aggregator": aggregatorData,
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
			printer.PrintMetrics(&checkFileOutput, cliParams.formatTable)

			p := func(data string) {
				fmt.Println(data)
				checkFileOutput.WriteString(data + "\n")
			}

			// workaround for this one use case of the status component
			// we want to render the collector text format with custom data
			checkInformation := checkTemplate(c)

			p(checkInformation)

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
Service Checks: Last Run: %s, Total: %s
Histogram Buckets: Last Run: %s, Total: %s
Average Execution Time :%s
Last Execution Date : %s
Last Successful Execution Date : %s
Cancelling: %b
`

func checkTemplate(c *stats.Stats) string {
	name := c.CheckName

	if c.CheckVersion != "" {
		name = fmt.Sprintf("%s (%s)", name, c.CheckVersion)
	}

	averageExcution, _ := time.ParseDuration(fmt.Sprintf("%fms", c.AverageExecutionTime))
	// TODO
	// {{ if .LastSuccessDate }}{{formatUnixTime .LastSuccessDate}}{{ else }}Never{{ end }}
	lastSuccesfulExecution := "Never"
	// TODO
	// {{formatUnixTime .UpdateTimestamp}}
	lastExcutionDate := ""

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
		humanize.Commaf(float64(c.ServiceChecks)),
		humanize.Commaf(float64(c.TotalServiceChecks)),
		humanize.Commaf(float64(c.HistogramBuckets)),
		humanize.Commaf(float64(c.TotalHistogramBuckets)),
		averageExcution.String(),
		lastExcutionDate,
		lastSuccesfulExecution,
		c.Cancelling,
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

func checkInventory() map[string]interface{} {
	return nil
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

// disableCmdPort overrrides the `cmd_port` configuration so that when the
// server starts up, it does not do so on the same port as a running agent.
//
// Ideally, the server wouldn't start up at all, but this workaround has been
// in place for some time.
func disableCmdPort() {
	os.Setenv("DD_CMD_PORT", "0") // 0 indicates the OS should pick an unused port
}
