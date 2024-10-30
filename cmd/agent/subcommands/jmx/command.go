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
	"github.com/DataDog/datadog-agent/comp/agent"
	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger"
	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger/jmxloggerimpl"
	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager"
	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager/diagnosesendermanagerimpl"
	authtokenimpl "github.com/DataDog/datadog-agent/comp/api/authtoken/createandfetchimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"

	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl"
	internalAPI "github.com/DataDog/datadog-agent/comp/api/api/def"
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
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/defaults"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/cli/standalone"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
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
			cliParams.logFile = filepath.Join(defaultpaths.JMXFlareDirectory, "jmx_"+cliParams.command+"_"+filenameSafeTimeStamp+".log")
			cliParams.jmxLogLevel = "debug"
		}

		// default log level to "debug" if not given
		if cliParams.jmxLogLevel == "" {
			cliParams.jmxLogLevel = "debug"
		}
		params := core.BundleParams{
			ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
			SecretParams: secrets.NewEnabledParams(),
			LogParams:    log.ForOneShot(command.LoggerName, cliParams.jmxLogLevel, false)}
		if cliParams.logFile != "" {
			params.LogParams.LogToFile(cliParams.logFile)
		}

		return fxutil.OneShot(callback,
			fx.Supply(cliParams),
			fx.Supply(params),
			core.Bundle(),
			compressionimpl.Module(),
			diagnosesendermanagerimpl.Module(),
			fx.Supply(func(diagnoseSenderManager diagnosesendermanager.Component) (sender.SenderManager, error) {
				return diagnoseSenderManager.LazyGetSenderManager()
			}),
			// workloadmeta setup
			wmcatalog.GetCatalog(),
			workloadmetafx.Module(defaults.DefaultParams()),
			apiimpl.Module(),
			authtokenimpl.Module(),
			// The jmx command do not have settings that change are runtime
			// still, we need to pass it to ensure the API server is proprely initialized
			settingsimpl.Module(),
			fx.Supply(settings.Params{}),

			// TODO(components): this is a temporary hack as the StartServer() method of the API package was previously called with nil arguments
			// This highlights the fact that the API Server created by JMX (through ExecJmx... function) should be different from the ones created
			// in others commands such as run.
			fx.Supply(optional.NewNoneOption[rcservice.Component]()),
			fx.Supply(optional.NewNoneOption[rcservicemrf.Component]()),
			fx.Supply(optional.NewNoneOption[collector.Component]()),
			fx.Supply(optional.NewNoneOption[logsAgent.Component]()),
			fx.Supply(optional.NewNoneOption[integrations.Component]()),
			fx.Provide(func() dogstatsdServer.Component { return nil }),
			fx.Provide(func() pidmap.Component { return nil }),
			fx.Provide(func() replay.Component { return nil }),
			fx.Provide(func() status.Component { return nil }),

			fx.Provide(tagger.NewTaggerParamsForCoreAgent),
			taggerimpl.Module(),
			autodiscoveryimpl.Module(),
			agent.Bundle(jmxloggerimpl.NewCliParams(cliParams.logFile)),
			// InitSharedContainerProvider must be called before the application starts so the workloadmeta collector can be initiailized correctly.
			// Since the tagger depends on the workloadmeta collector, we can not make the tagger a dependency of workloadmeta as it would create a circular dependency.
			// TODO: (component) - once we remove the dependency of workloadmeta component from the tagger component
			// we can include the tagger as part of the workloadmeta component.
			fx.Invoke(func(wmeta workloadmeta.Component, tagger tagger.Component) {
				proccontainers.InitSharedContainerProvider(wmeta, tagger)
			}),
		)
	}

	jmxCollectCmd := &cobra.Command{
		Use:   "collect",
		Short: "Start the collection of metrics based on your current configuration and display them in the console.",
		Long:  ``,
		RunE: func(*cobra.Command, []string) error {
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
		RunE: func(*cobra.Command, []string) error {
			cliParams.command = "list_everything"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListMatchingCmd := &cobra.Command{
		Use:   "matching",
		Short: "List attributes that match at least one of your instances configuration.",
		Long:  ``,
		RunE: func(*cobra.Command, []string) error {
			cliParams.command = "list_matching_attributes"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListWithMetricsCmd := &cobra.Command{
		Use:   "with-metrics",
		Short: "List attributes and metrics data that match at least one of your instances configuration.",
		Long:  ``,
		RunE: func(*cobra.Command, []string) error {
			cliParams.command = "list_with_metrics"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListWithRateMetricsCmd := &cobra.Command{
		Use:   "with-rate-metrics",
		Short: "List attributes and metrics data that match at least one of your instances configuration, including rates and counters.",
		Long:  ``,
		RunE: func(*cobra.Command, []string) error {
			cliParams.command = "list_with_rate_metrics"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListLimitedCmd := &cobra.Command{
		Use:   "limited",
		Short: "List attributes that do match one of your instances configuration but that are not being collected because it would exceed the number of metrics that can be collected.",
		Long:  ``,
		RunE: func(*cobra.Command, []string) error {
			cliParams.command = "list_limited_attributes"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListCollectedCmd := &cobra.Command{
		Use:   "collected",
		Short: "List attributes that will actually be collected by your current instances configuration.",
		Long:  ``,
		RunE: func(*cobra.Command, []string) error {
			cliParams.command = "list_collected_attributes"
			disableCmdPort()
			return runOneShot(runJmxCommandConsole)
		},
	}

	jmxListNotMatchingCmd := &cobra.Command{
		Use:   "not-matching",
		Short: "List attributes that don’t match any of your instances configuration.",
		Long:  ``,
		RunE: func(*cobra.Command, []string) error {
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
	ac autodiscovery.Component,
	diagnoseSendermanager diagnosesendermanager.Component,
	secretResolver secrets.Component,
	agentAPI internalAPI.Component,
	collector optional.Option[collector.Component],
	jmxLogger jmxlogger.Component,
	logReceiver optional.Option[integrations.Component],
	tagger tagger.Component) error {
	// This prevents log-spam from "comp/core/workloadmeta/collectors/internal/remote/process_collector/process_collector.go"
	// It appears that this collector creates some contention in AD.
	// Disabling it is both more efficient and gets rid of this log spam
	config.Set("language_detection.enabled", "false", model.SourceAgentRuntime)

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
	pkgcollector.InitCheckScheduler(collector, senderManager, logReceiver, tagger)

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

	err = standalone.ExecJMXCommandConsole(cliParams.command, cliParams.cliSelectedChecks, cliParams.jmxLogLevel, allConfigs, agentAPI, jmxLogger)

	if runtime.GOOS == "windows" {
		standalone.PrintWindowsUserWarning("jmx")
	}

	return err
}
