// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package run implements 'agent run' (and deprecated 'agent start').
package run

import (
	"context"
	_ "expvar" // Blank import used because this isn't directly used in this file
	"fmt"
	"net/http"
	_ "net/http/pprof" // Blank import used because this isn't directly used in this file
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"gopkg.in/DataDog/dd-trace-go.v1/profiler"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/misconfig"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands/run/internal/clcrunnerapi"
	internalsettings "github.com/DataDog/datadog-agent/cmd/agent/subcommands/run/internal/settings"
	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	agenttelemetryfx "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/fx"

	// checks implemented as components

	// core components
	"github.com/DataDog/datadog-agent/comp/agent/autoexit"
	"github.com/DataDog/datadog-agent/comp/agent/expvarserver"
	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger"
	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger/jmxloggerimpl"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	demultiplexerendpointfx "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexerendpoint/fx"
	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl"
	internalAPI "github.com/DataDog/datadog-agent/comp/api/api/def"
	authtokenimpl "github.com/DataDog/datadog-agent/comp/api/authtoken/createandfetchimpl"
	commonendpoints "github.com/DataDog/datadog-agent/comp/api/commonendpoints/fx"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/collector/collector/collectorimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/gui"
	"github.com/DataDog/datadog-agent/comp/core/gui/guiimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/pid"
	"github.com/DataDog/datadog-agent/comp/core/pid/pidimpl"
	"github.com/DataDog/datadog-agent/comp/process"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"

	"github.com/DataDog/datadog-agent/comp/agent"
	"github.com/DataDog/datadog-agent/comp/agent/cloudfoundrycontainer"
	healthprobe "github.com/DataDog/datadog-agent/comp/core/healthprobe/def"
	healthprobefx "github.com/DataDog/datadog-agent/comp/core/healthprobe/fx"
	lsof "github.com/DataDog/datadog-agent/comp/core/lsof/fx"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog-core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/defaults"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/dogstatsd"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	dogstatsddebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	dogstatsdStatusimpl "github.com/DataDog/datadog-agent/comp/dogstatsd/status/statusimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	orchestratorForwarderImpl "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	langDetectionCl "github.com/DataDog/datadog-agent/comp/languagedetection/client"
	langDetectionClimpl "github.com/DataDog/datadog-agent/comp/languagedetection/client/clientimpl"
	"github.com/DataDog/datadog-agent/comp/logs"
	"github.com/DataDog/datadog-agent/comp/logs/adscheduler/adschedulerimpl"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/comp/metadata"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryotel"
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning"
	"github.com/DataDog/datadog-agent/comp/metadata/runner"
	securityagentmetadata "github.com/DataDog/datadog-agent/comp/metadata/securityagent/def"
	systemprobemetadata "github.com/DataDog/datadog-agent/comp/metadata/systemprobe/def"
	"github.com/DataDog/datadog-agent/comp/ndmtmp"
	"github.com/DataDog/datadog-agent/comp/netflow"
	netflowServer "github.com/DataDog/datadog-agent/comp/netflow/server"
	"github.com/DataDog/datadog-agent/comp/networkpath"
	"github.com/DataDog/datadog-agent/comp/otelcol"
	otelcollector "github.com/DataDog/datadog-agent/comp/otelcol/collector/def"
	"github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline"
	processAgent "github.com/DataDog/datadog-agent/comp/process/agent"
	processagentStatusImpl "github.com/DataDog/datadog-agent/comp/process/status/statusimpl"
	rdnsquerierfx "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx"
	remoteconfig "github.com/DataDog/datadog-agent/comp/remote-config"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice/rcserviceimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf/rcservicemrfimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rctelemetryreporter/rctelemetryreporterimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps"
	snmptrapsServer "github.com/DataDog/datadog-agent/comp/snmptraps/server"
	traceagentStatusImpl "github.com/DataDog/datadog-agent/comp/trace/status/statusimpl"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/net"
	profileStatus "github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/status"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/commonchecks"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/jmxfetch"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	clusteragentStatus "github.com/DataDog/datadog-agent/pkg/status/clusteragent"
	endpointsStatus "github.com/DataDog/datadog-agent/pkg/status/endpoints"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	httpproxyStatus "github.com/DataDog/datadog-agent/pkg/status/httpproxy"
	jmxStatus "github.com/DataDog/datadog-agent/pkg/status/jmx"
	systemprobeStatus "github.com/DataDog/datadog-agent/pkg/status/systemprobe"
	pkgTelemetry "github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util"
	pkgcommon "github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil/logging"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/version"

	// runtime init routines
	ddruntime "github.com/DataDog/datadog-agent/pkg/runtime"
)

type cliParams struct {
	*command.GlobalParams

	// pidfilePath contains the value of the --pidfile flag.
	pidfilePath string
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}
	runE := func(*cobra.Command, []string) error {
		// TODO: once the agent is represented as a component, and not a function (run),
		// this will use `fxutil.Run` instead of `fxutil.OneShot`.
		return fxutil.OneShot(run,
			fx.Invoke(func(_ log.Component) {
				ddruntime.SetMaxProcs()
			}),
			fx.Supply(core.BundleParams{
				ConfigParams:         config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(cliParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(cliParams.FleetPoliciesDirPath)),
				SecretParams:         secrets.NewEnabledParams(),
				SysprobeConfigParams: sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath), sysprobeconfigimpl.WithFleetPoliciesDirPath(cliParams.FleetPoliciesDirPath)),
				LogParams:            log.ForDaemon(command.LoggerName, "log_file", defaultpaths.LogFile),
			}),
			fx.Supply(pidimpl.NewParams(cliParams.pidfilePath)),
			logging.EnableFxLoggingOnDebug[log.Component](),
			getSharedFxOption(),
			getPlatformModules(),
		)
	}

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run the Agent",
		Long:  `Runs the agent in the foreground`,
		RunE:  runE,
	}
	runCmd.Flags().StringVarP(&cliParams.pidfilePath, "pidfile", "p", "", "path to the pidfile")

	startCmd := &cobra.Command{
		Use:        "start",
		Deprecated: "Use \"run\" instead to start the Agent",
		RunE:       runE,
	}
	startCmd.Flags().StringVarP(&cliParams.pidfilePath, "pidfile", "p", "", "path to the pidfile")

	return []*cobra.Command{startCmd, runCmd}
}

// run starts the main loop.
//
// This is exported because it also used from the deprecated `agent start` command.
func run(log log.Component,
	cfg config.Component,
	flare flare.Component,
	telemetry telemetry.Component,
	sysprobeconfig sysprobeconfig.Component,
	server dogstatsdServer.Component,
	_ replay.Component,
	forwarder defaultforwarder.Component,
	wmeta workloadmeta.Component,
	taggerComp tagger.Component,
	ac autodiscovery.Component,
	rcclient rcclient.Component,
	_ runner.Component,
	demultiplexer demultiplexer.Component,
	sharedSerializer serializer.MetricSerializer,
	logsAgent optional.Option[logsAgent.Component],
	_ statsd.Component,
	processAgent processAgent.Component,
	otelcollector otelcollector.Component,
	_ host.Component,
	_ inventoryagent.Component,
	_ inventoryhost.Component,
	_ inventoryotel.Component,
	_ secrets.Component,
	invChecks inventorychecks.Component,
	logReceiver optional.Option[integrations.Component],
	_ netflowServer.Component,
	_ snmptrapsServer.Component,
	_ langDetectionCl.Component,
	agentAPI internalAPI.Component,
	_ packagesigning.Component,
	_ systemprobemetadata.Component,
	_ securityagentmetadata.Component,
	statusComponent status.Component,
	collector collector.Component,
	cloudfoundrycontainer cloudfoundrycontainer.Component,
	_ expvarserver.Component,
	_ pid.Component,
	jmxlogger jmxlogger.Component,
	_ healthprobe.Component,
	_ autoexit.Component,
	settings settings.Component,
	_ optional.Option[gui.Component],
	_ agenttelemetry.Component,
) error {
	defer func() {
		stopAgent()
	}()

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Make a channel to exit the function
	stopCh := make(chan error)

	go func() {
		// Set up the signals async so we can Start the agent
		select {
		case <-signals.Stopper:
			log.Info("Received stop command, shutting down...")
			stopCh <- nil
		case <-signals.ErrorStopper:
			_ = log.Critical("The Agent has encountered an error, shutting down...")
			stopCh <- fmt.Errorf("shutting down because of an error")
		case sig := <-signalCh:
			log.Infof("Received signal '%s', shutting down...", sig)
			stopCh <- nil
		}
	}()

	// By default systemd redirects the stdout to journald. When journald is stopped or crashes we receive a SIGPIPE signal.
	// Go ignores SIGPIPE signals unless it is when stdout or stdout is closed, in this case the agent is stopped.
	// We never want the agent to stop upon receiving SIGPIPE, so we intercept the SIGPIPE signals and just discard them.
	sigpipeCh := make(chan os.Signal, 1)
	signal.Notify(sigpipeCh, syscall.SIGPIPE)
	go func() {
		//nolint:revive
		for range sigpipeCh {
			// do nothing
		}
	}()

	if err := startAgent(
		log,
		flare,
		telemetry,
		sysprobeconfig,
		server,
		wmeta,
		taggerComp,
		ac,
		rcclient,
		logsAgent,
		processAgent,
		forwarder,
		sharedSerializer,
		otelcollector,
		demultiplexer,
		agentAPI,
		invChecks,
		logReceiver,
		statusComponent,
		collector,
		cfg,
		cloudfoundrycontainer,
		jmxlogger,
		settings,
	); err != nil {
		return err
	}

	return <-stopCh
}

func getSharedFxOption() fx.Option {
	return fx.Options(
		flare.Module(flare.NewParams(
			defaultpaths.GetDistPath(),
			defaultpaths.PyChecksPath,
			defaultpaths.LogFile,
			defaultpaths.JmxLogFile,
			defaultpaths.DogstatsDLogFile,
			defaultpaths.StreamlogsLogFile,
		)),
		core.Bundle(),
		lsof.Module(),
		// Enable core agent specific features like persistence-to-disk
		forwarder.Bundle(defaultforwarder.NewParams(defaultforwarder.WithFeatures(defaultforwarder.CoreFeatures))),
		// workloadmeta setup
		wmcatalog.GetCatalog(),
		workloadmetafx.Module(defaults.DefaultParams()),
		fx.Supply(
			status.Params{
				PythonVersionGetFunc: python.GetPythonVersion,
			},
			status.NewHeaderInformationProvider(net.Provider{}),
			status.NewInformationProvider(jmxStatus.Provider{}),
			status.NewInformationProvider(endpointsStatus.Provider{}),
			status.NewInformationProvider(profileStatus.Provider{}),
		),
		fx.Provide(func(config config.Component) status.InformationProvider {
			return status.NewInformationProvider(clusteragentStatus.GetProvider(config))
		}),
		fx.Provide(func(sysprobeconfig sysprobeconfig.Component) status.InformationProvider {
			return status.NewInformationProvider(systemprobeStatus.GetProvider(sysprobeconfig))
		}),
		fx.Provide(func(config config.Component) status.InformationProvider {
			return status.NewInformationProvider(httpproxyStatus.GetProvider(config))
		}),
		fx.Supply(
			rcclient.Params{
				AgentName:    "core-agent",
				AgentVersion: version.AgentVersion,
			},
		),
		traceagentStatusImpl.Module(),
		processagentStatusImpl.Module(),
		dogstatsdStatusimpl.Module(),
		statsd.Module(),
		statusimpl.Module(),
		authtokenimpl.Module(),
		apiimpl.Module(),
		commonendpoints.Module(),
		compressionimpl.Module(),
		demultiplexerimpl.Module(demultiplexerimpl.NewDefaultParams(demultiplexerimpl.WithDogstatsdNoAggregationPipelineConfig())),
		demultiplexerendpointfx.Module(),
		dogstatsd.Bundle(dogstatsdServer.Params{Serverless: false}),
		fx.Provide(func(logsagent optional.Option[logsAgent.Component]) optional.Option[logsagentpipeline.Component] {
			if la, ok := logsagent.Get(); ok {
				return optional.NewOption[logsagentpipeline.Component](la)
			}
			return optional.NewNoneOption[logsagentpipeline.Component]()
		}),
		otelcol.Bundle(),
		rctelemetryreporterimpl.Module(),
		rcserviceimpl.Module(),
		rcservicemrfimpl.Module(),
		remoteconfig.Bundle(),
		fx.Provide(tagger.NewTaggerParamsForCoreAgent),
		taggerimpl.Module(),
		autodiscoveryimpl.Module(),
		// InitSharedContainerProvider must be called before the application starts so the workloadmeta collector can be initiailized correctly.
		// Since the tagger depends on the workloadmeta collector, we can not make the tagger a dependency of workloadmeta as it would create a circular dependency.
		// TODO: (component) - once we remove the dependency of workloadmeta component from the tagger component
		// we can include the tagger as part of the workloadmeta component.
		fx.Invoke(func(wmeta workloadmeta.Component, tagger tagger.Component) {
			proccontainers.InitSharedContainerProvider(wmeta, tagger)
		}),
		// TODO: (components) - some parts of the agent (such as the logs agent) implicitly depend on the global state
		// set up by LoadComponents. In order for components to use lifecycle hooks that also depend on this global state, we
		// have to ensure this code gets run first. Once the common package is made into a component, this can be removed.
		//
		// Workloadmeta component needs to be initialized before this hook is executed, and thus is included
		// in the function args to order the execution. This pattern might be worth revising because it is
		// error prone.
		fx.Invoke(func(lc fx.Lifecycle, wmeta workloadmeta.Component, _ tagger.Component, ac autodiscovery.Component, secretResolver secrets.Component) {
			lc.Append(fx.Hook{
				OnStart: func(_ context.Context) error {
					//  setup the AutoConfig instance
					common.LoadComponents(secretResolver, wmeta, ac, pkgconfigsetup.Datadog().GetString("confd_path"))
					return nil
				},
			})
		}),
		logs.Bundle(),
		langDetectionClimpl.Module(),
		metadata.Bundle(),
		orchestratorForwarderImpl.Module(orchestratorForwarderImpl.NewDefaultParams()),
		eventplatformimpl.Module(eventplatformimpl.NewDefaultParams()),
		eventplatformreceiverimpl.Module(),

		// injecting the shared Serializer to FX until we migrate it to a proper component. This allows other
		// already migrated components to request it.
		fx.Provide(func(demuxInstance demultiplexer.Component) serializer.MetricSerializer {
			return demuxInstance.Serializer()
		}),
		fx.Provide(func(ms serializer.MetricSerializer) optional.Option[serializer.MetricSerializer] {
			return optional.NewOption[serializer.MetricSerializer](ms)
		}),
		ndmtmp.Bundle(),
		netflow.Bundle(),
		rdnsquerierfx.Module(),
		snmptraps.Bundle(),
		collectorimpl.Module(),
		process.Bundle(),
		guiimpl.Module(),
		agent.Bundle(jmxloggerimpl.NewDefaultParams()),
		fx.Provide(func(config config.Component) healthprobe.Options {
			return healthprobe.Options{
				Port:           config.GetInt("health_port"),
				LogsGoroutines: config.GetBool("log_all_goroutines_when_unhealthy"),
			}
		}),
		healthprobefx.Module(),
		adschedulerimpl.Module(),
		fx.Provide(func(serverDebug dogstatsddebug.Component, config config.Component) settings.Params {
			return settings.Params{
				Settings: map[string]settings.RuntimeSetting{
					"log_level":                              commonsettings.NewLogLevelRuntimeSetting(),
					"runtime_mutex_profile_fraction":         commonsettings.NewRuntimeMutexProfileFraction(),
					"runtime_block_profile_rate":             commonsettings.NewRuntimeBlockProfileRate(),
					"dogstatsd_stats":                        internalsettings.NewDsdStatsRuntimeSetting(serverDebug),
					"dogstatsd_capture_duration":             internalsettings.NewDsdCaptureDurationRuntimeSetting("dogstatsd_capture_duration"),
					"log_payloads":                           commonsettings.NewLogPayloadsRuntimeSetting(),
					"internal_profiling_goroutines":          commonsettings.NewProfilingGoroutines(),
					"multi_region_failover.enabled":          internalsettings.NewMultiRegionFailoverRuntimeSetting("multi_region_failover.enabled", "Enable/disable Multi-Region Failover support."),
					"multi_region_failover.failover_metrics": internalsettings.NewMultiRegionFailoverRuntimeSetting("multi_region_failover.failover_metrics", "Enable/disable redirection of metrics to failover region."),
					"multi_region_failover.failover_logs":    internalsettings.NewMultiRegionFailoverRuntimeSetting("multi_region_failover.failover_logs", "Enable/disable redirection of logs to failover region."),
					"internal_profiling":                     commonsettings.NewProfilingRuntimeSetting("internal_profiling", "datadog-agent"),
				},
				Config: config,
			}
		}),
		settingsimpl.Module(),
		agenttelemetryfx.Module(),
		networkpath.Bundle(),
	)
}

// startAgent Initializes the agent process
func startAgent(
	log log.Component,
	_ flare.Component,
	telemetry telemetry.Component,
	_ sysprobeconfig.Component,
	server dogstatsdServer.Component,
	wmeta workloadmeta.Component,
	tagger tagger.Component,
	ac autodiscovery.Component,
	rcclient rcclient.Component,
	_ optional.Option[logsAgent.Component],
	_ processAgent.Component,
	_ defaultforwarder.Component,
	_ serializer.MetricSerializer,
	_ otelcollector.Component,
	demultiplexer demultiplexer.Component,
	_ internalAPI.Component,
	invChecks inventorychecks.Component,
	logReceiver optional.Option[integrations.Component],
	_ status.Component,
	collector collector.Component,
	cfg config.Component,
	_ cloudfoundrycontainer.Component,
	jmxLogger jmxlogger.Component,
	settings settings.Component,
) error {
	var err error

	if flavor.GetFlavor() == flavor.IotAgent {
		log.Infof("Starting Datadog IoT Agent v%v", version.AgentVersion)
	} else {
		log.Infof("Starting Datadog Agent v%v", version.AgentVersion)
	}

	if err := util.SetupCoreDump(pkgconfigsetup.Datadog()); err != nil {
		log.Warnf("Can't setup core dumps: %v, core dumps might not be available after a crash", err)
	}

	if v := pkgconfigsetup.Datadog().GetBool("internal_profiling.capture_all_allocations"); v {
		runtime.MemProfileRate = 1
		log.Infof("MemProfileRate set to 1, capturing every single memory allocation!")
	}

	// Setup Internal Profiling
	common.SetupInternalProfiling(settings, pkgconfigsetup.Datadog(), "")

	// Setup expvar server
	telemetryHandler := telemetry.Handler()

	http.Handle("/telemetry", telemetryHandler)

	ctx, _ := pkgcommon.GetMainCtxCancel()

	hostnameDetected, err := hostname.Get(context.TODO())
	if err != nil {
		return log.Errorf("Error while getting hostname, exiting: %v", err)
	}
	log.Infof("Hostname is: %s", hostnameDetected)

	// start remote configuration management
	if pkgconfigsetup.IsRemoteConfigEnabled(pkgconfigsetup.Datadog()) {
		// Subscribe to `AGENT_TASK` product
		rcclient.SubscribeAgentTask()

		// Subscribe to `APM_TRACING` product
		rcclient.SubscribeApmTracing()

		if pkgconfigsetup.Datadog().GetBool("remote_configuration.agent_integrations.enabled") {
			// Spin up the config provider to schedule integrations through remote-config
			rcProvider := providers.NewRemoteConfigProvider()
			rcclient.Subscribe(data.ProductAgentIntegrations, rcProvider.IntegrationScheduleCallback)
			// LoadAndRun is called later on
			ac.AddConfigProvider(rcProvider, true, 10*time.Second)
		}
	}

	// start clc runner server
	// only start when the cluster agent is enabled and a cluster check runner host is enabled
	if pkgconfigsetup.Datadog().GetBool("cluster_agent.enabled") && pkgconfigsetup.Datadog().GetBool("clc_runner_enabled") {
		if err = clcrunnerapi.StartCLCRunnerServer(map[string]http.Handler{
			"/telemetry": telemetryHandler,
		}, ac); err != nil {
			return log.Errorf("Error while starting clc runner api server, exiting: %v", err)
		}
	}

	// Create the Leader election engine without initializing it
	if pkgconfigsetup.Datadog().GetBool("leader_election") {
		leaderelection.CreateGlobalLeaderEngine(ctx)
	}

	// Setup stats telemetry handler
	if sender, err := demultiplexer.GetDefaultSender(); err == nil {
		// TODO: to be removed when default telemetry is enabled.
		pkgTelemetry.RegisterStatsSender(sender)
	}

	// Append version and timestamp to version history log file if this Agent is different than the last run version
	installinfo.LogVersionHistory()

	// TODO: (components) - Until the checks are components we set there context so they can depends on components.
	check.InitializeInventoryChecksContext(invChecks)

	// Init JMX runner and inject dogstatsd component
	jmxfetch.InitRunner(server, jmxLogger)
	jmxfetch.RegisterWith(ac)

	// Set up check collector
	commonchecks.RegisterChecks(wmeta, tagger, cfg, telemetry)
	ac.AddScheduler("check", pkgcollector.InitCheckScheduler(optional.NewOption(collector), demultiplexer, logReceiver, tagger), true)

	demultiplexer.AddAgentStartupTelemetry(version.AgentVersion)

	// load and run all configs in AD
	ac.LoadAndRun(ctx)

	// check for common misconfigurations and report them to log
	misconfig.ToLog(misconfig.CoreAgent)

	// start dependent services
	go startDependentServices()

	return nil
}

// StopAgentWithDefaults is a temporary way for other packages to use stopAgent.
func StopAgentWithDefaults() {
	stopAgent()
}

// stopAgent Tears down the agent process
func stopAgent() {
	// retrieve the agent health before stopping the components
	// GetReadyNonBlocking has a 100ms timeout to avoid blocking
	health, err := health.GetReadyNonBlocking()
	if err != nil {
		pkglog.Warnf("Agent health unknown: %s", err)
	} else if len(health.Unhealthy) > 0 {
		pkglog.Warnf("Some components were unhealthy: %v", health.Unhealthy)
	}

	clcrunnerapi.StopCLCRunnerServer()
	jmxfetch.StopJmxfetch()

	profiler.Stop()

	// gracefully shut down any component
	_, cancel := pkgcommon.GetMainCtxCancel()
	cancel()

	pkglog.Info("See ya!")
	pkglog.Flush()
}
