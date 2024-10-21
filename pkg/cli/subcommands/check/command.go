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

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger"
	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger/jmxloggerimpl"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl"
	internalAPI "github.com/DataDog/datadog-agent/comp/api/api/def"
	authtokenimpl "github.com/DataDog/datadog-agent/comp/api/authtoken/createandfetchimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
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
	logagent "github.com/DataDog/datadog-agent/comp/logs/agent"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks/inventorychecksimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/cli/standalone"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/commonchecks"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	statuscollector "github.com/DataDog/datadog-agent/pkg/status/collector"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
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
				fx.Provide(tagger.NewTaggerParamsForCoreAgent),
				taggerimpl.Module(),
				autodiscoveryimpl.Module(),
				forwarder.Bundle(defaultforwarder.NewParams(defaultforwarder.WithNoopForwarder())),
				inventorychecksimpl.Module(),
				// inventorychecksimpl depends on a collector and serializer when created to send payload.
				// Here we just want to collect metadata to be displayed, so we don't need a collector.
				collector.NoneModule(),
				fx.Supply(status.NewInformationProvider(statuscollector.Provider{})),
				fx.Provide(func() serializer.MetricSerializer { return nil }),
				compressionimpl.Module(),
				// Initializing the aggregator with a flush interval of 0 (to disable the flush goroutines)
				demultiplexerimpl.Module(demultiplexerimpl.NewDefaultParams(demultiplexerimpl.WithFlushInterval(0))),
				orchestratorForwarderImpl.Module(orchestratorForwarderImpl.NewNoopParams()),
				eventplatformimpl.Module(eventplatforParams),
				eventplatformreceiverimpl.Module(),
				fx.Supply(
					status.Params{
						PythonVersionGetFunc: python.GetPythonVersion,
					},
				),
				statusimpl.Module(),
				// The check command do not have settings that change are runtime
				// still, we need to pass it to ensure the API server is proprely initialized
				settingsimpl.Module(),
				fx.Supply(settings.Params{}),
				// TODO(components): this is a temporary hack as the StartServer() method of the API package was previously called with nil arguments
				// This highlights the fact that the API Server created by JMX (through ExecJmx... function) should be different from the ones created
				// in others commands such as run.
				fx.Supply(optional.NewNoneOption[rcservice.Component]()),
				fx.Supply(optional.NewNoneOption[rcservicemrf.Component]()),
				fx.Supply(optional.NewNoneOption[logagent.Component]()),
				fx.Supply(optional.NewNoneOption[integrations.Component]()),
				fx.Provide(func() server.Component { return nil }),
				fx.Provide(func() replay.Component { return nil }),
				fx.Provide(func() pidmap.Component { return nil }),

				getPlatformModules(),
				jmxloggerimpl.Module(jmxloggerimpl.NewDisabledParams()),
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
	statusComponent status.Component,
	collector optional.Option[collector.Component],
	jmxLogger jmxlogger.Component,
	telemetry telemetry.Component,
	logReceiver optional.Option[integrations.Component],
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

	// TODO: (components) - Until the checks are components we set there context so they can depends on components.
	check.InitializeInventoryChecksContext(invChecks)
	pkgcollector.InitPython(common.GetPythonPaths()...)
	commonchecks.RegisterChecks(wmeta, tagger, config, telemetry)

	common.LoadComponents(secretResolver, wmeta, ac, pkgconfigsetup.Datadog().GetString("confd_path"))
	ac.LoadAndRun(context.Background())

	// Create the CheckScheduler, but do not attach it to
	// AutoDiscovery.
	pkgcollector.InitCheckScheduler(collector, demultiplexer, logReceiver, tagger)

	waitCtx, cancelTimeout := context.WithTimeout(
		context.Background(), time.Duration(cliParams.discoveryTimeout)*time.Second)

	allConfigs, err := common.WaitForConfigsFromAD(waitCtx, []string{cliParams.checkName}, int(cliParams.discoveryMinInstances), cliParams.instanceFilter, ac)
	cancelTimeout()
	if err != nil {
		return err
	}

	// make sure the checks in cs are not JMX checks
	for idx := range allConfigs {
		conf := &allConfigs[idx]
		if conf.Name != cliParams.checkName {
			continue
		}

		if check.IsJMXConfig(*conf) {
			// we'll mimic the check command behavior with JMXFetch by running
			// it with the JSON reporter and the list_with_metrics command.
			fmt.Println("Please consider using the 'jmx' command instead of 'check jmx'")
			selectedChecks := []string{cliParams.checkName}
			if cliParams.checkRate {
				if err := standalone.ExecJmxListWithRateMetricsJSON(selectedChecks, config.GetString("log_level"), allConfigs, agentAPI, jmxLogger); err != nil {
					return fmt.Errorf("while running the jmx check: %v", err)
				}
			} else {
				if err := standalone.ExecJmxListWithMetricsJSON(selectedChecks, config.GetString("log_level"), allConfigs, agentAPI, jmxLogger); err != nil {
					return fmt.Errorf("while running the jmx check: %v", err)
				}
			}

			instances := []integration.Data{}

			// Retain only non-JMX instances for later
			for _, instance := range conf.Instances {
				if check.IsJMXInstance(conf.Name, instance, conf.InitConfig) {
					continue
				}
				instances = append(instances, instance)
			}

			if len(instances) == 0 {
				fmt.Printf("All instances of '%s' are JMXFetch instances, and have completed running\n", cliParams.checkName)
				return nil
			}

			conf.Instances = instances
		}
	}

	if cliParams.profileMemory {
		// If no directory is specified, make a temporary one
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

		for idx := range allConfigs {
			conf := &allConfigs[idx]
			if conf.Name != cliParams.checkName {
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

			data["profile_memory"] = cliParams.profileMemoryDir
			err = populateMemoryProfileConfig(cliParams, data)
			if err != nil {
				return err
			}

			y, _ := yaml.Marshal(data)
			conf.InitConfig = y

			break
		}
	} else if cliParams.breakPoint != "" {
		breakPointLine, err := strconv.Atoi(cliParams.breakPoint)
		if err != nil {
			fmt.Printf("breakpoint must be an integer\n")
			return err
		}

		for idx := range allConfigs {
			conf := &allConfigs[idx]
			if conf.Name != cliParams.checkName {
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

	cs := pkgcollector.GetChecksByNameForConfigs(cliParams.checkName, allConfigs)

	// something happened while getting the check(s), display some info.
	if len(cs) == 0 {
		for check, error := range autodiscoveryimpl.GetConfigErrors() {
			if cliParams.checkName == check {
				fmt.Fprintf(color.Output, "\n%s: invalid config for %s: %s\n", color.RedString("Error"), color.YellowString(check), error)
			}
		}
		for check, errors := range pkgcollector.GetLoaderErrors() {
			if cliParams.checkName == check {
				fmt.Fprintf(color.Output, "\n%s: could not load %s:\n", color.RedString("Error"), color.YellowString(cliParams.checkName))
				for loader, error := range errors {
					fmt.Fprintf(color.Output, "* %s: %s\n", color.YellowString(loader), error)
				}
			}
		}
		for check, warnings := range autodiscoveryimpl.GetResolveWarnings() {
			if cliParams.checkName == check {
				fmt.Fprintf(color.Output, "\n%s: could not resolve %s config:\n", color.YellowString("Warning"), color.YellowString(check))
				for _, warning := range warnings {
					fmt.Fprintf(color.Output, "* %s\n", warning)
				}
			}
		}
		return fmt.Errorf("no valid check found")
	}

	if len(cs) > 1 {
		fmt.Println("Multiple check instances found, running each of them")
	}

	var checkFileOutput bytes.Buffer
	var instancesData []interface{}
	printer := aggregator.AgentDemultiplexerPrinter{DemultiplexerWithAggregator: demultiplexer}
	data, err := statusComponent.GetStatusBySections([]string{status.CollectorSection}, "json", false)

	if err != nil {
		return err
	}

	collectorData := map[string]interface{}{}
	err = json.Unmarshal(data, &collectorData)

	if err != nil {
		return err
	}

	checkRuns := collectorData["runnerStats"].(map[string]interface{})["Checks"].(map[string]interface{})
	for _, c := range cs {
		s := runCheck(cliParams, c, printer)
		resultBytes, err := json.Marshal(s)
		if err != nil {
			return err
		}

		var checkResult map[string]interface{}
		err = json.Unmarshal(resultBytes, &checkResult)
		if err != nil {
			return err
		}

		checkMap := make(map[string]interface{})
		checkMap[string(c.ID())] = checkResult
		checkRuns[c.String()] = checkMap

		// Sleep for a while to allow the aggregator to finish ingesting all the metrics/events/sc
		time.Sleep(time.Duration(cliParams.checkDelay) * time.Millisecond)

		if cliParams.formatJSON {
			aggregatorData := printer.GetMetricsDataForPrint()

			// There is only one checkID per run so we'll just access that
			instanceData := map[string]interface{}{
				"aggregator":  aggregatorData,
				"runner":      s,
				"inventories": collectorData["inventories"],
			}
			instancesData = append(instancesData, instanceData)
		} else if cliParams.profileMemory {
			// Every instance will create its own directory
			instanceID := strings.SplitN(string(c.ID()), ":", 2)[1]
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
			collectorProvider := statuscollector.Provider{}
			buffer := new(bytes.Buffer)
			err := collectorProvider.TextWithData(buffer, collectorData)
			if err != nil {
				return err
			}

			p(buffer.String())

			p("  Metadata\n  ========")

			metadata := check.GetMetadata(c, false)
			for k, v := range metadata {
				p(fmt.Sprintf("    %s: %v", k, v))
			}
			for k, v := range invChecks.GetInstanceMetadata(string(c.ID())) {
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
		color.Yellow("This check type has %d instances. If you're looking for a different check instance, try filtering on a specific one using the --instance-filter flag or set --discovery-min-instances to a higher value", len(cs))
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

func runCheck(cliParams *cliParams, c check.Check, _ aggregator.Demultiplexer) *stats.Stats {
	s := stats.NewStats(c)
	times := cliParams.checkTimes
	pause := cliParams.checkPause
	if cliParams.checkRate {
		if cliParams.checkTimes > 2 {
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
		sStats, _ := c.GetSenderStats()
		s.Add(time.Since(t0), err, warnings, sStats)
		if pause > 0 && i < times-1 {
			time.Sleep(time.Duration(pause) * time.Millisecond)
		}
	}

	return s
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

func populateMemoryProfileConfig(cliParams *cliParams, initConfig map[string]interface{}) error {
	if cliParams.profileMemoryFrames != "" {
		profileMemoryFrames, err := strconv.Atoi(cliParams.profileMemoryFrames)
		if err != nil {
			return fmt.Errorf("--m-frames must be an integer")
		}
		initConfig["profile_memory_frames"] = profileMemoryFrames
	}

	if cliParams.profileMemoryGC != "" {
		profileMemoryGC, err := strconv.Atoi(cliParams.profileMemoryGC)
		if err != nil {
			return fmt.Errorf("--m-gc must be an integer")
		}

		initConfig["profile_memory_gc"] = profileMemoryGC
	}

	if cliParams.profileMemoryCombine != "" {
		profileMemoryCombine, err := strconv.Atoi(cliParams.profileMemoryCombine)
		if err != nil {
			return fmt.Errorf("--m-combine must be an integer")
		}

		if profileMemoryCombine != 0 && cliParams.profileMemorySort == "traceback" {
			return fmt.Errorf("--m-combine cannot be sorted (--m-sort) by traceback")
		}

		initConfig["profile_memory_combine"] = profileMemoryCombine
	}

	if cliParams.profileMemorySort != "" {
		if cliParams.profileMemorySort != "lineno" && cliParams.profileMemorySort != "filename" && cliParams.profileMemorySort != "traceback" {
			return fmt.Errorf("--m-sort must one of: lineno | filename | traceback")
		}
		initConfig["profile_memory_sort"] = cliParams.profileMemorySort
	}

	if cliParams.profileMemoryLimit != "" {
		profileMemoryLimit, err := strconv.Atoi(cliParams.profileMemoryLimit)
		if err != nil {
			return fmt.Errorf("--m-limit must be an integer")
		}
		initConfig["profile_memory_limit"] = profileMemoryLimit
	}

	if cliParams.profileMemoryDiff != "" {
		if cliParams.profileMemoryDiff != "absolute" && cliParams.profileMemoryDiff != "positive" {
			return fmt.Errorf("--m-diff must one of: absolute | positive")
		}
		initConfig["profile_memory_diff"] = cliParams.profileMemoryDiff
	}

	if cliParams.profileMemoryFilters != "" {
		initConfig["profile_memory_filters"] = cliParams.profileMemoryFilters
	}

	if cliParams.profileMemoryUnit != "" {
		initConfig["profile_memory_unit"] = cliParams.profileMemoryUnit
	}

	if cliParams.profileMemoryVerbose != "" {
		profileMemoryVerbose, err := strconv.Atoi(cliParams.profileMemoryVerbose)
		if err != nil {
			return fmt.Errorf("--m-verbose must be an integer")
		}
		initConfig["profile_memory_verbose"] = profileMemoryVerbose
	}

	return nil
}

// disableCmdPort overrrides the `cmd_port` configuration so that when the
// server starts up, it does not do so on the same port as a running agent.
//
// Ideally, the server wouldn't start up at all, but this workaround has been
// in place for some time.
func disableCmdPort() {
	os.Setenv("DD_CMD_PORT", "0") // 0 indicates the OS should pick an unused port
}
