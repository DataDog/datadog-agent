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
	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands/run/internal/clcrunnerapi"
	"github.com/DataDog/datadog-agent/cmd/manager"

	// checks implemented as components

	// core components

	"github.com/DataDog/datadog-agent/comp/agent/expvarserver"
	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger"
	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger/jmxloggerimpl"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	internalAPI "github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl"
	authtokenimpl "github.com/DataDog/datadog-agent/comp/api/authtoken/createandfetchimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/collector/collector/collectorimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/gui/guiimpl"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/pid"
	"github.com/DataDog/datadog-agent/comp/core/pid/pidimpl"
	"github.com/DataDog/datadog-agent/comp/process"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"

	"github.com/DataDog/datadog-agent/comp/agent"
	"github.com/DataDog/datadog-agent/comp/agent/metadatascheduler"
	"github.com/DataDog/datadog-agent/comp/core/healthprobe"
	"github.com/DataDog/datadog-agent/comp/core/healthprobe/healthprobeimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/defaults"
	"github.com/DataDog/datadog-agent/comp/dogstatsd"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	dogstatsddebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	dogstatsdStatusimpl "github.com/DataDog/datadog-agent/comp/dogstatsd/status/statusimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	orchestratorForwarderImpl "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	langDetectionCl "github.com/DataDog/datadog-agent/comp/languagedetection/client"
	langDetectionClimpl "github.com/DataDog/datadog-agent/comp/languagedetection/client/clientimpl"
	"github.com/DataDog/datadog-agent/comp/logs"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/comp/metadata/packagesigning"
	"github.com/DataDog/datadog-agent/comp/metadata/runner"
	"github.com/DataDog/datadog-agent/comp/ndmtmp"
	"github.com/DataDog/datadog-agent/comp/netflow"
	netflowServer "github.com/DataDog/datadog-agent/comp/netflow/server"
	"github.com/DataDog/datadog-agent/comp/otelcol"
	otelcollector "github.com/DataDog/datadog-agent/comp/otelcol/collector"
	processAgent "github.com/DataDog/datadog-agent/comp/process/agent"
	processagentStatusImpl "github.com/DataDog/datadog-agent/comp/process/status/statusimpl"
	remoteconfig "github.com/DataDog/datadog-agent/comp/remote-config"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice/rcserviceimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcserviceha/rcservicehaimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rctelemetryreporter/rctelemetryreporterimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps"
	snmptrapsServer "github.com/DataDog/datadog-agent/comp/snmptraps/server"
	traceagentStatusImpl "github.com/DataDog/datadog-agent/comp/trace/status/statusimpl"
	"github.com/DataDog/datadog-agent/pkg/cloudfoundry/containertagger"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/embed/jmx"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/net"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/commonchecks"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	adScheduler "github.com/DataDog/datadog-agent/pkg/logs/schedulers/ad"
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
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/installinfo"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/version"

	// runtime init routines
	ddruntime "github.com/DataDog/datadog-agent/pkg/runtime"

	// register metadata providers
	_ "github.com/DataDog/datadog-agent/pkg/collector/metadata"
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
			fx.Supply(core.BundleParams{
				ConfigParams:         config.NewAgentParams(globalParams.ConfFilePath),
				SecretParams:         secrets.NewEnabledParams(),
				SysprobeConfigParams: sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath)),
				LogParams:            logimpl.ForDaemon(command.LoggerName, "log_file", path.DefaultLogFile),
			}),
			fx.Supply(pidimpl.NewParams(cliParams.pidfilePath)),
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
	_ config.Component,
	flare flare.Component,
	telemetry telemetry.Component,
	sysprobeconfig sysprobeconfig.Component,
	server dogstatsdServer.Component,
	_ replay.Component,
	serverDebug dogstatsddebug.Component,
	forwarder defaultforwarder.Component,
	wmeta workloadmeta.Component,
	taggerComp tagger.Component,
	ac autodiscovery.Component,
	rcclient rcclient.Component,
	_ runner.Component,
	demultiplexer demultiplexer.Component,
	sharedSerializer serializer.MetricSerializer,
	logsAgent optional.Option[logsAgent.Component],
	processAgent processAgent.Component,
	otelcollector otelcollector.Component,
	_ host.Component,
	_ inventoryagent.Component,
	_ inventoryhost.Component,
	_ secrets.Component,
	invChecks inventorychecks.Component,
	_ netflowServer.Component,
	_ snmptrapsServer.Component,
	_ langDetectionCl.Component,
	agentAPI internalAPI.Component,
	_ packagesigning.Component,
	statusComponent status.Component,
	collector collector.Component,
	_ expvarserver.Component,
	_ pid.Component,
	metadatascheduler metadatascheduler.Component,
	jmxlogger jmxlogger.Component,
	_ healthprobe.Component,
) error {
	defer func() {
		stopAgent(agentAPI)
	}()

	// prepare go runtime
	ddruntime.SetMaxProcs()

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
		serverDebug,
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
		statusComponent,
		collector,
		metadatascheduler,
		jmxlogger,
	); err != nil {
		return err
	}

	return <-stopCh
}

func getSharedFxOption() fx.Option {
	return fx.Options(
		fx.Supply(flare.NewParams(
			path.GetDistPath(),
			path.PyChecksPath,
			path.DefaultLogFile,
			path.DefaultJmxLogFile,
			path.DefaultDogstatsDLogFile,
		)),
		flare.Module(),
		core.Bundle(),
		fx.Supply(dogstatsdServer.Params{
			Serverless: false,
		}),
		forwarder.Bundle(),
		fx.Provide(func(config config.Component, log log.Component) defaultforwarder.Params {
			params := defaultforwarder.NewParams(config, log)
			// Enable core agent specific features like persistence-to-disk
			params.Options.EnabledFeatures = defaultforwarder.SetFeature(params.Options.EnabledFeatures, defaultforwarder.CoreFeatures)
			return params
		}),

		// workloadmeta setup
		collectors.GetCatalog(),
		fx.Provide(defaults.DefaultParams),
		workloadmeta.Module(),
		fx.Supply(
			status.Params{
				PythonVersionGetFunc: python.GetPythonVersion,
			},
			status.NewHeaderInformationProvider(net.Provider{}),
			status.NewInformationProvider(jmxStatus.Provider{}),
			status.NewInformationProvider(endpointsStatus.Provider{}),
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
		traceagentStatusImpl.Module(),
		processagentStatusImpl.Module(),
		dogstatsdStatusimpl.Module(),
		statusimpl.Module(),
		authtokenimpl.Module(),
		apiimpl.Module(),
		compressionimpl.Module(),
		demultiplexerimpl.Module(),
		dogstatsd.Bundle(),
		otelcol.Bundle(),
		rctelemetryreporterimpl.Module(),
		rcserviceimpl.Module(),
		rcservicehaimpl.Module(),
		remoteconfig.Bundle(),
		fx.Provide(tagger.NewTaggerParamsForCoreAgent),
		tagger.Module(),
		autodiscoveryimpl.Module(),
		fx.Provide(func(ac autodiscovery.Component) optional.Option[autodiscovery.Component] {
			return optional.NewOption[autodiscovery.Component](ac)
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
					common.LoadComponents(secretResolver, wmeta, ac, pkgconfig.Datadog.GetString("confd_path"))
					return nil
				},
			})
		}),
		logs.Bundle(),
		langDetectionClimpl.Module(),
		metadata.Bundle(),
		// injecting the aggregator demultiplexer to FX until we migrate it to a proper component. This allows
		// other already migrated components to request it.
		fx.Provide(func(config config.Component) demultiplexerimpl.Params {
			params := demultiplexerimpl.NewDefaultParams()
			params.EnableNoAggregationPipeline = config.GetBool("dogstatsd_no_aggregation_pipeline")
			return params
		}),
		orchestratorForwarderImpl.Module(),
		fx.Supply(orchestratorForwarderImpl.NewDefaultParams()),
		eventplatformimpl.Module(),
		fx.Supply(eventplatformimpl.NewDefaultParams()),
		eventplatformreceiverimpl.Module(),

		// injecting the shared Serializer to FX until we migrate it to a proper component. This allows other
		// already migrated components to request it.
		fx.Provide(func(demuxInstance demultiplexer.Component) serializer.MetricSerializer {
			return demuxInstance.Serializer()
		}),
		ndmtmp.Bundle(),
		netflow.Bundle(),
		snmptraps.Bundle(),
		collectorimpl.Module(),
		process.Bundle(),
		guiimpl.Module(),
		agent.Bundle(),
		fx.Supply(jmxloggerimpl.NewDefaultParams()),
		fx.Provide(func(config config.Component) healthprobe.Options {
			return healthprobe.Options{
				Port:           config.GetInt("health_port"),
				LogsGoroutines: config.GetBool("log_all_goroutines_when_unhealthy"),
			}
		}),
		healthprobeimpl.Module(),
	)
}

// startAgent Initializes the agent process
func startAgent(
	log log.Component,
	_ flare.Component,
	telemetry telemetry.Component,
	_ sysprobeconfig.Component,
	server dogstatsdServer.Component,
	serverDebug dogstatsddebug.Component,
	wmeta workloadmeta.Component,
	taggerComp tagger.Component,
	ac autodiscovery.Component,
	rcclient rcclient.Component,
	logsAgent optional.Option[logsAgent.Component],
	_ processAgent.Component,
	_ defaultforwarder.Component,
	_ serializer.MetricSerializer,
	otelcollector otelcollector.Component,
	demultiplexer demultiplexer.Component,
	agentAPI internalAPI.Component,
	invChecks inventorychecks.Component,
	_ status.Component,
	collector collector.Component,
	_ metadatascheduler.Component,
	jmxLogger jmxlogger.Component,
) error {

	var err error

	if flavor.GetFlavor() == flavor.IotAgent {
		log.Infof("Starting Datadog IoT Agent v%v", version.AgentVersion)
	} else {
		log.Infof("Starting Datadog Agent v%v", version.AgentVersion)
	}

	if err := util.SetupCoreDump(pkgconfig.Datadog); err != nil {
		log.Warnf("Can't setup core dumps: %v, core dumps might not be available after a crash", err)
	}

	if v := pkgconfig.Datadog.GetBool("internal_profiling.capture_all_allocations"); v {
		runtime.MemProfileRate = 1
		log.Infof("MemProfileRate set to 1, capturing every single memory allocation!")
	}

	// init settings that can be changed at runtime
	if err := initRuntimeSettings(serverDebug); err != nil {
		log.Warnf("Can't initiliaze the runtime settings: %v", err)
	}

	// Setup Internal Profiling
	common.SetupInternalProfiling(pkgconfig.Datadog, "")

	// Setup expvar server
	telemetryHandler := telemetry.Handler()

	http.Handle("/telemetry", telemetryHandler)

	ctx, _ := pkgcommon.GetMainCtxCancel()

	err = manager.ConfigureAutoExit(ctx, pkgconfig.Datadog)
	if err != nil {
		return log.Errorf("Unable to configure auto-exit, err: %v", err)
	}

	hostnameDetected, err := hostname.Get(context.TODO())
	if err != nil {
		return log.Errorf("Error while getting hostname, exiting: %v", err)
	}
	log.Infof("Hostname is: %s", hostnameDetected)

	// start remote configuration management
	if pkgconfig.IsRemoteConfigEnabled(pkgconfig.Datadog) {
		if err := rcclient.Start("core-agent"); err != nil {
			pkglog.Errorf("Failed to start the RC client component: %s", err)
		} else {
			// Subscribe to `AGENT_TASK` product
			rcclient.SubscribeAgentTask()

			// Subscribe to `APM_TRACING` product
			rcclient.SubscribeApmTracing()

			if pkgconfig.Datadog.GetBool("remote_configuration.agent_integrations.enabled") {
				// Spin up the config provider to schedule integrations through remote-config
				rcProvider := providers.NewRemoteConfigProvider()
				rcclient.Subscribe(data.ProductAgentIntegrations, rcProvider.IntegrationScheduleCallback)
				// LoadAndRun is called later on
				ac.AddConfigProvider(rcProvider, true, 10*time.Second)
			}
		}
	}

	if logsAgent, ok := logsAgent.Get(); ok {
		// TODO: (components) - once adScheduler is a component, inject it into the logs agent.
		logsAgent.AddScheduler(adScheduler.New(ac))
	}

	// start the cloudfoundry container tagger
	if pkgconfig.IsFeaturePresent(pkgconfig.CloudFoundry) && !pkgconfig.Datadog.GetBool("cloud_foundry_buildpack") {
		containerTagger, err := containertagger.NewContainerTagger(wmeta)
		if err != nil {
			log.Errorf("Failed to create Cloud Foundry container tagger: %v", err)
		} else {
			containerTagger.Start(ctx)
		}
	}

	// start the cmd HTTP server
	if err = agentAPI.StartServer(
		wmeta,
		taggerComp,
		ac,
		logsAgent,
		demultiplexer,
		optional.NewOption(collector),
	); err != nil {
		return log.Errorf("Error while starting api server, exiting: %v", err)
	}

	// start clc runner server
	// only start when the cluster agent is enabled and a cluster check runner host is enabled
	if pkgconfig.Datadog.GetBool("cluster_agent.enabled") && pkgconfig.Datadog.GetBool("clc_runner_enabled") {
		if err = clcrunnerapi.StartCLCRunnerServer(map[string]http.Handler{
			"/telemetry": telemetryHandler,
		}, ac); err != nil {
			return log.Errorf("Error while starting clc runner api server, exiting: %v", err)
		}
	}

	// Create the Leader election engine without initializing it
	if pkgconfig.Datadog.GetBool("leader_election") {
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
	jmx.InitRunner(server, jmxLogger)

	// Set up check collector
	commonchecks.RegisterChecks(wmeta)
	ac.AddScheduler("check", pkgcollector.InitCheckScheduler(optional.NewOption(collector), demultiplexer), true)

	demultiplexer.AddAgentStartupTelemetry(version.AgentVersion)

	// load and run all configs in AD
	ac.LoadAndRun(ctx)

	// check for common misconfigurations and report them to log
	misconfig.ToLog(misconfig.CoreAgent)

	// start dependent services
	go startDependentServices()

	return otelcollector.Start()
}

// StopAgentWithDefaults is a temporary way for other packages to use stopAgent.
func StopAgentWithDefaults(agentAPI internalAPI.Component) {
	stopAgent(agentAPI)
}

// stopAgent Tears down the agent process
func stopAgent(agentAPI internalAPI.Component) {
	// retrieve the agent health before stopping the components
	// GetReadyNonBlocking has a 100ms timeout to avoid blocking
	health, err := health.GetReadyNonBlocking()
	if err != nil {
		pkglog.Warnf("Agent health unknown: %s", err)
	} else if len(health.Unhealthy) > 0 {
		pkglog.Warnf("Some components were unhealthy: %v", health.Unhealthy)
	}

	agentAPI.StopServer()
	clcrunnerapi.StopCLCRunnerServer()
	jmx.StopJmxfetch()

	profiler.Stop()

	// gracefully shut down any component
	_, cancel := pkgcommon.GetMainCtxCancel()
	cancel()

	pkglog.Info("See ya!")
	pkglog.Flush()
}
