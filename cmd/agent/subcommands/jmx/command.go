// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build jmx

package jmx

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/comp/agent"
	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger"
	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger/jmxloggerimpl"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager"
	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager/diagnosesendermanagerimpl"
	internalAPI "github.com/DataDog/datadog-agent/comp/api/api"
	authtokenimpl "github.com/DataDog/datadog-agent/comp/api/authtoken/createandfetchimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"

	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/gui"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	serverdebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcserviceha"
	"github.com/DataDog/datadog-agent/pkg/cli/standalone"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

type cliParams struct {
	*command.GlobalParams

	// command is the jmx console command to run
	command string

	cliSelectedChecks     []string
	logFile               string // calculated in runOneShot
	jmxLogLevel           string
	saveFlare             bool
	discoveryTimeout      uint
	discoveryMinInstances uint
	instanceFilter        string
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	var discoveryRetryInterval uint // unused command-line flag
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	jmxCmd := &cobra.Command{
		Use:   "jmx",
		Short: "Run troubleshooting commands on JMXFetch integrations",
		Long:  ``,
	}
	jmxCmd.PersistentFlags().StringVarP(&cliParams.jmxLogLevel, "log-level", "l", "", "set the log level (default 'debug') (deprecated, use the env var DD_LOG_LEVEL instead)")
	jmxCmd.PersistentFlags().UintVarP(&cliParams.discoveryTimeout, "discovery-timeout", "", 5, "max retry duration until Autodiscovery resolves the check template (in seconds)")
	jmxCmd.PersistentFlags().UintVarP(&discoveryRetryInterval, "discovery-retry-interval", "", 1, "(unused)")
	jmxCmd.PersistentFlags().UintVarP(&cliParams.discoveryMinInstances, "discovery-min-instances", "", 1, "minimum number of config instances to be discovered before running the check(s)")
	jmxCmd.PersistentFlags().StringVarP(&cliParams.instanceFilter, "instance-filter", "", "", "filter instances using jq style syntax, example: --instance-filter '.ip_address == \"127.0.0.51\"'")

	// All subcommands use the same provided components, with a different
	// oneShot callback, and with some complex derivation of the
	// core.BundleParams value
	runOneShot := func(callback interface{}) error {
		cliParams.logFile = ""

		// if saving a flare, write a debug log within a directory that will be
		// captured in the flare.
		if cliParams.saveFlare {
			// Windows cannot accept ":" in file names
			filenameSafeTimeStamp := strings.ReplaceAll(time.Now().UTC().Format(time.RFC3339), ":", "-")
			cliParams.logFile = filepath.Join(path.DefaultJMXFlareDirectory, "jmx_"+cliParams.command+"_"+filenameSafeTimeStamp+".log")
			cliParams.jmxLogLevel = "debug"
		}

		// default log level to "debug" if not given
		if cliParams.jmxLogLevel == "" {
			cliParams.jmxLogLevel = "debug"
		}
		params := core.BundleParams{
			ConfigParams: config.NewAgentParams(globalParams.ConfFilePath),
			SecretParams: secrets.NewEnabledParams(),
			LogParams:    logimpl.ForOneShot(command.LoggerName, cliParams.jmxLogLevel, false)}
		if cliParams.logFile != "" {
			params.LogParams.LogToFile(cliParams.logFile)
		}

		return fxutil.OneShot(callback,
			fx.Supply(cliParams),
			fx.Supply(params),
			core.Bundle(),
			compressionimpl.Module(),
			diagnosesendermanagerimpl.Module(),
			// workloadmeta setup
			collectors.GetCatalog(),
			fx.Supply(workloadmeta.Params{
				InitHelper: common.GetWorkloadmetaInit(),
			}),
			workloadmeta.Module(),
			apiimpl.Module(),
			authtokenimpl.Module(),
			// TODO(components): this is a temporary hack as the StartServer() method of the API package was previously called with nil arguments
			// This highlights the fact that the API Server created by JMX (through ExecJmx... function) should be different from the ones created
			// in others commands such as run.
			fx.Provide(func() flare.Component { return nil }),
			fx.Provide(func() dogstatsdServer.Component { return nil }),
			fx.Provide(func() replay.Component { return nil }),
			fx.Provide(func() pidmap.Component { return nil }),
			fx.Provide(func() serverdebug.Component { return nil }),
			fx.Provide(func() host.Component { return nil }),
			fx.Provide(func() inventoryagent.Component { return nil }),
			fx.Provide(func() inventoryhost.Component { return nil }),
			fx.Provide(func() demultiplexer.Component { return nil }),
			fx.Provide(func() inventorychecks.Component { return nil }),
			fx.Provide(func() packagesigning.Component { return nil }),
			fx.Provide(func() optional.Option[rcservice.Component] { return optional.NewNoneOption[rcservice.Component]() }),
			fx.Provide(func() optional.Option[rcserviceha.Component] { return optional.NewNoneOption[rcserviceha.Component]() }),
			fx.Provide(func() status.Component { return nil }),
			fx.Provide(func() eventplatformreceiver.Component { return nil }),
			fx.Provide(func() optional.Option[collector.Component] { return optional.NewNoneOption[collector.Component]() }),
			fx.Provide(tagger.NewTaggerParamsForCoreAgent),
			tagger.Module(),
			autodiscoveryimpl.Module(),
			fx.Supply(optional.NewNoneOption[gui.Component]()),
			agent.Bundle(),
			fx.Supply(jmxloggerimpl.NewCliParams(cliParams.logFile)),
		)
	}

	jmxCollectCmd := &cobra.Command{
		Use:   "collect",
		Short: "Start the collection of metrics based on your current configuration and display them in the console.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.command = "collect"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}
	jmxCollectCmd.PersistentFlags().StringSliceVar(&cliParams.cliSelectedChecks, "checks", []string{}, "JMX checks (ex: jmx,tomcat)")
	jmxCollectCmd.PersistentFlags().BoolVarP(&cliParams.saveFlare, "flare", "", false, "save jmx list results to the log dir so it may be reported in a flare")
	jmxCmd.AddCommand(jmxCollectCmd)

	jmxListEverythingCmd := &cobra.Command{
		Use:   "everything",
		Short: "List every attributes available that has a type supported by JMXFetch.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.command = "list_everything"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListMatchingCmd := &cobra.Command{
		Use:   "matching",
		Short: "List attributes that match at least one of your instances configuration.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.command = "list_matching_attributes"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListWithMetricsCmd := &cobra.Command{
		Use:   "with-metrics",
		Short: "List attributes and metrics data that match at least one of your instances configuration.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.command = "list_with_metrics"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListWithRateMetricsCmd := &cobra.Command{
		Use:   "with-rate-metrics",
		Short: "List attributes and metrics data that match at least one of your instances configuration, including rates and counters.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.command = "list_with_rate_metrics"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListLimitedCmd := &cobra.Command{
		Use:   "limited",
		Short: "List attributes that do match one of your instances configuration but that are not being collected because it would exceed the number of metrics that can be collected.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.command = "list_limited_attributes"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListCollectedCmd := &cobra.Command{
		Use:   "collected",
		Short: "List attributes that will actually be collected by your current instances configuration.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.command = "list_collected_attributes"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListNotMatchingCmd := &cobra.Command{
		Use:   "not-matching",
		Short: "List attributes that don’t match any of your instances configuration.",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliParams.command = "list_not_matching_attributes"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListCmd := &cobra.Command{
		Use:   "list",
		Short: "List attributes matched by JMXFetch.",
		Long:  ``,
	}
	jmxListCmd.AddCommand(
		jmxListEverythingCmd,
		jmxListMatchingCmd,
		jmxListLimitedCmd,
		jmxListCollectedCmd,
		jmxListNotMatchingCmd,
		jmxListWithMetricsCmd,
		jmxListWithRateMetricsCmd,
	)

	jmxListCmd.PersistentFlags().StringSliceVar(&cliParams.cliSelectedChecks, "checks", []string{}, "JMX checks (ex: jmx,tomcat)")
	jmxListCmd.PersistentFlags().BoolVarP(&cliParams.saveFlare, "flare", "", false, "save jmx list results to the log dir so it may be reported in a flare")
	jmxCmd.AddCommand(jmxListCmd)

	// attach the command to the root
	return []*cobra.Command{jmxCmd}
}

// disableCmdPort overrrides the `cmd_port` configuration so that when the
// server starts up, it does not do so on the same port as a running agent.
//
// Ideally, the server wouldn't start up at all, but this workaround has been
// in place for some times.
func disableCmdPort() {
	os.Setenv("DD_CMD_PORT", "0") // 0 indicates the OS should pick an unused port
}

// runJmxCommandConsole sets up the common utils necessary for JMX, and executes the command
// with the Console reporter
func runJmxCommandConsole(config config.Component,
	cliParams *cliParams,
	wmeta workloadmeta.Component,
	taggerComp tagger.Component,
	ac autodiscovery.Component,
	diagnoseSendermanager diagnosesendermanager.Component,
	secretResolver secrets.Component,
	agentAPI internalAPI.Component,
	collector optional.Option[collector.Component],
	jmxLogger jmxlogger.Component) error {
	// This prevents log-spam from "comp/core/workloadmeta/collectors/internal/remote/process_collector/process_collector.go"
	// It appears that this collector creates some contention in AD.
	// Disabling it is both more efficient and gets rid of this log spam
	pkgconfig.Datadog.Set("language_detection.enabled", "false", model.SourceAgentRuntime)

	senderManager, err := diagnoseSendermanager.LazyGetSenderManager()
	if err != nil {
		return err
	}
	// The Autoconfig instance setup happens in the workloadmeta start hook
	// create and setup the Collector and others.
	common.LoadComponents(secretResolver, wmeta, ac, config.GetString("confd_path"))
	ac.LoadAndRun(context.Background())

	// Create the CheckScheduler, but do not attach it to
	// AutoDiscovery.
	pkgcollector.InitCheckScheduler(collector, senderManager)

	// if cliSelectedChecks is empty, then we want to fetch all check configs;
	// otherwise, we fetch only the matching cehck configs.
	waitCtx, cancelTimeout := context.WithTimeout(
		context.Background(), time.Duration(cliParams.discoveryTimeout)*time.Second)
	var allConfigs []integration.Config
	if len(cliParams.cliSelectedChecks) == 0 {
		allConfigs, err = common.WaitForAllConfigsFromAD(waitCtx, ac)
	} else {
		allConfigs, err = common.WaitForConfigsFromAD(waitCtx, cliParams.cliSelectedChecks, int(cliParams.discoveryMinInstances), cliParams.instanceFilter, ac)
	}
	cancelTimeout()
	if err != nil {
		return err
	}

	err = standalone.ExecJMXCommandConsole(cliParams.command, cliParams.cliSelectedChecks, cliParams.jmxLogLevel, allConfigs, wmeta, taggerComp, ac, diagnoseSendermanager, agentAPI, collector, jmxLogger)

	if runtime.GOOS == "windows" {
		standalone.PrintWindowsUserWarning("jmx")
	}

	return err
}
