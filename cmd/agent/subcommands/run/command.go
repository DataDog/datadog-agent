// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package run implements 'agent run' (and deprecated 'agent start').
package run

import (
	"context"
	"errors"
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
	global "github.com/DataDog/datadog-agent/cmd/agent/dogstatsd"
	"github.com/DataDog/datadog-agent/cmd/agent/gui"
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands/run/internal/clcrunnerapi"
	"github.com/DataDog/datadog-agent/cmd/manager"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	netflowServer "github.com/DataDog/datadog-agent/comp/netflow/server"

	// checks implemented as components

	// core components
	internalAPI "github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors"
	"github.com/DataDog/datadog-agent/comp/dogstatsd"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	dogstatsddebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	//nolint:revive // TODO(ASC) Fix revive linter
	pkgforwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	orchestratorForwarderImpl "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	langDetectionCl "github.com/DataDog/datadog-agent/comp/languagedetection/client"
	"github.com/DataDog/datadog-agent/comp/logs"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/comp/metadata/runner"
	"github.com/DataDog/datadog-agent/comp/ndmtmp"
	"github.com/DataDog/datadog-agent/comp/netflow"
	"github.com/DataDog/datadog-agent/comp/otelcol"
	otelcollector "github.com/DataDog/datadog-agent/comp/otelcol/collector"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/api/healthprobe"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/pkg/cloudfoundry/containertagger"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/embed/jmx"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	adScheduler "github.com/DataDog/datadog-agent/pkg/logs/schedulers/ad"
	pkgMetadata "github.com/DataDog/datadog-agent/pkg/metadata"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	otlpStatus "github.com/DataDog/datadog-agent/pkg/status/otlp"
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

	// register core checks
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/helm"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/ksm"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/kubernetesapiserver"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/containerimage"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/containerlifecycle"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/containerd"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/cri"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/docker"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/embed"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/net"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/nvidia/jetson"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/orchestrator/pod"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/sbom"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/cpu"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/disk"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/filehandles"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/memory"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/uptime"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/wincrashdetect"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/winkmem"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/winproc"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/systemd"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/telemetry"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/windows_event_log"

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
			fx.Supply(cliParams),
			fx.Supply(core.BundleParams{
				ConfigParams:         config.NewAgentParams(globalParams.ConfFilePath),
				SecretParams:         secrets.NewEnabledParams(),
				SysprobeConfigParams: sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath)),
				LogParams:            log.ForDaemon(command.LoggerName, "log_file", path.DefaultLogFile),
			}),
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
	//nolint:revive // TODO(ASC) Fix revive linter
	config config.Component,
	flare flare.Component,
	telemetry telemetry.Component,
	sysprobeconfig sysprobeconfig.Component,
	server dogstatsdServer.Component,
	capture replay.Component,
	serverDebug dogstatsddebug.Component,
	forwarder defaultforwarder.Component,
	wmeta workloadmeta.Component,
	rcclient rcclient.Component,
	//nolint:revive // TODO(ASC) Fix revive linter
	metadataRunner runner.Component,
	demultiplexer demultiplexer.Component,
	sharedSerializer serializer.MetricSerializer,
	cliParams *cliParams,
	logsAgent optional.Option[logsAgent.Component],
	otelcollector otelcollector.Component,
	hostMetadata host.Component,
	invAgent inventoryagent.Component,
	invHost inventoryhost.Component,
	secretResolver secrets.Component,
	invChecks inventorychecks.Component,
	_ netflowServer.Component,
	_ langDetectionCl.Component,
	agentAPI internalAPI.Component,
) error {
	defer func() {
		stopAgent(cliParams, server, demultiplexer, agentAPI)
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
		//nolint:revive // TODO(ASC) Fix revive linter
		for range sigpipeCh {
			// do nothing
		}
	}()

	if err := startAgent(cliParams,
		log,
		flare,
		telemetry,
		sysprobeconfig,
		server,
		capture,
		serverDebug,
		wmeta,
		rcclient,
		logsAgent,
		forwarder,
		sharedSerializer,
		otelcollector,
		demultiplexer,
		hostMetadata,
		invAgent,
		invHost,
		secretResolver,
		agentAPI,
		invChecks,
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
		flare.Module,
		core.Bundle,
		fx.Supply(dogstatsdServer.Params{
			Serverless: false,
		}),
		forwarder.Bundle,
		fx.Provide(func(config config.Component, log log.Component) defaultforwarder.Params {
			params := defaultforwarder.NewParams(config, log)
			// Enable core agent specific features like persistence-to-disk
			params.Options.EnabledFeatures = pkgforwarder.SetFeature(params.Options.EnabledFeatures, pkgforwarder.CoreFeatures)
			return params
		}),

		// workloadmeta setup
		collectors.GetCatalog(),
		fx.Provide(func(config config.Component) workloadmeta.Params {
			var agentType workloadmeta.AgentType
			if flavor.GetFlavor() == flavor.ClusterAgent {
				agentType = workloadmeta.ClusterAgent
			} else {
				agentType = workloadmeta.NodeAgent
			}

			return workloadmeta.Params{
				AgentType:  agentType,
				InitHelper: common.GetWorkloadmetaInit(),
			}
		}),
		workloadmeta.Module,
		apiimpl.Module,

		dogstatsd.Bundle,
		otelcol.Bundle,
		rcclient.Module,

		// TODO: (components) - some parts of the agent (such as the logs agent) implicitly depend on the global state
		// set up by LoadComponents. In order for components to use lifecycle hooks that also depend on this global state, we
		// have to ensure this code gets run first. Once the common package is made into a component, this can be removed.
		//
		// Workloadmeta component needs to be initialized before this hook is executed, and thus is included
		// in the function args to order the execution. This pattern might be worth revising because it is
		// error prone.
		fx.Invoke(func(lc fx.Lifecycle, demultiplexer demultiplexer.Component, _ workloadmeta.Component, secretResolver secrets.Component) {
			lc.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {

					// create and setup the Autoconfig instance
					common.LoadComponents(demultiplexer, secretResolver, pkgconfig.Datadog.GetString("confd_path"))
					return nil
				},
			})
		}),
		logs.Bundle,
		langDetectionCl.Module,
		metadata.Bundle,
		// injecting the aggregator demultiplexer to FX until we migrate it to a proper component. This allows
		// other already migrated components to request it.
		fx.Provide(func(config config.Component) demultiplexer.Params {
			opts := aggregator.DefaultAgentDemultiplexerOptions()
			opts.EnableNoAggregationPipeline = config.GetBool("dogstatsd_no_aggregation_pipeline")
			opts.UseDogstatsdContextLimiter = true
			opts.DogstatsdMaxMetricsTags = config.GetInt("dogstatsd_max_metrics_tags")
			return demultiplexer.Params{Options: opts}
		}),
		demultiplexer.Module,
		orchestratorForwarderImpl.Module,
		fx.Supply(orchestratorForwarderImpl.NewDefaultParams()),
		// injecting the shared Serializer to FX until we migrate it to a prpoper component. This allows other
		// already migrated components to request it.
		fx.Provide(func(demuxInstance demultiplexer.Component) serializer.MetricSerializer {
			return demuxInstance.Serializer()
		}),
		ndmtmp.Bundle,
		netflow.Bundle,
		fx.Provide(func(demultiplexer demultiplexer.Component) optional.Option[collector.Collector] {
			return optional.NewOption(common.LoadCollector(demultiplexer))
		}),
	)
}

// startAgent Initializes the agent process
func startAgent(
	cliParams *cliParams,
	log log.Component,
	flare flare.Component,
	telemetry telemetry.Component,
	//nolint:revive // TODO(ASC) Fix revive linter
	sysprobeconfig sysprobeconfig.Component,
	server dogstatsdServer.Component,
	capture replay.Component,
	serverDebug dogstatsddebug.Component,
	wmeta workloadmeta.Component,
	rcclient rcclient.Component,
	logsAgent optional.Option[logsAgent.Component],
	//nolint:revive // TODO(ASC) Fix revive linter
	sharedForwarder defaultforwarder.Component,
	//nolint:revive // TODO(ASC) Fix revive linter
	sharedSerializer serializer.MetricSerializer,
	otelcollector otelcollector.Component,
	demultiplexer demultiplexer.Component,
	hostMetadata host.Component,
	invAgent inventoryagent.Component,
	invHost inventoryhost.Component,
	secretResolver secrets.Component,
	agentAPI internalAPI.Component,
	invChecks inventorychecks.Component,
) error {

	var err error

	// Setup logger
	syslogURI := pkgconfig.GetSyslogURI()
	jmxLogFile := pkgconfig.Datadog.GetString("jmx_log_file")
	if jmxLogFile == "" {
		jmxLogFile = path.DefaultJmxLogFile
	}

	if pkgconfig.Datadog.GetBool("disable_file_logging") {
		// this will prevent any logging on file
		jmxLogFile = ""
	}

	// Setup JMX logger
	jmxLoggerSetupErr := pkgconfig.SetupJMXLogger(
		jmxLogFile,
		syslogURI,
		pkgconfig.Datadog.GetBool("syslog_rfc"),
		pkgconfig.Datadog.GetBool("log_to_console"),
		pkgconfig.Datadog.GetBool("log_format_json"),
	)

	if jmxLoggerSetupErr != nil {
		return fmt.Errorf("Error while setting up logging, exiting: %v", jmxLoggerSetupErr)
	}

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
	if err := initRuntimeSettings(serverDebug, invAgent); err != nil {
		log.Warnf("Can't initiliaze the runtime settings: %v", err)
	}

	// Setup Internal Profiling
	common.SetupInternalProfiling(pkgconfig.Datadog, "")

	// Setup expvar server
	telemetryHandler := telemetry.Handler()

	expvarPort := pkgconfig.Datadog.GetString("expvar_port")
	http.Handle("/telemetry", telemetryHandler)
	go func() {
		common.ExpvarServer = &http.Server{
			Addr:    fmt.Sprintf("127.0.0.1:%s", expvarPort),
			Handler: http.DefaultServeMux,
		}
		if err := common.ExpvarServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("Error creating expvar server on %v: %v", common.ExpvarServer.Addr, err)
		}
	}()

	// Setup healthcheck port
	ctx, _ := pkgcommon.GetMainCtxCancel()
	healthPort := pkgconfig.Datadog.GetInt("health_port")
	if healthPort > 0 {
		err := healthprobe.Serve(ctx, healthPort)
		if err != nil {
			return log.Errorf("Error starting health port, exiting: %v", err)
		}
		log.Debugf("Health check listening on port %d", healthPort)
	}

	if cliParams.pidfilePath != "" {
		err = pidfile.WritePID(cliParams.pidfilePath)
		if err != nil {
			return log.Errorf("Error while writing PID file, exiting: %v", err)
		}
		log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), cliParams.pidfilePath)
	}

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
	var configService *remoteconfig.Service
	if pkgconfig.IsRemoteConfigEnabled(pkgconfig.Datadog) {
		configService, err = remoteconfig.NewService()
		if err != nil {
			log.Errorf("Failed to initialize config management service: %s", err)
		} else {
			configService.Start(context.Background())
		}

		if err := rcclient.Start("core-agent"); err != nil {
			pkglog.Errorf("Failed to start the RC client component: %s", err)
		} else {
			// Subscribe to `AGENT_TASK` product
			rcclient.SubscribeAgentTask()

			if pkgconfig.Datadog.GetBool("remote_configuration.agent_integrations.enabled") {
				// Spin up the config provider to schedule integrations through remote-config
				rcProvider := providers.NewRemoteConfigProvider()
				rcclient.Subscribe(data.ProductAgentIntegrations, rcProvider.IntegrationScheduleCallback)
				// LoadAndRun is called later on
				common.AC.AddConfigProvider(rcProvider, true, 10*time.Second)
			}
		}
	}

	if logsAgent, ok := logsAgent.Get(); ok {
		// TODO: (components) - once adScheduler is a component, inject it into the logs agent.
		logsAgent.AddScheduler(adScheduler.New(common.AC))
	}

	// start the cloudfoundry container tagger
	if pkgconfig.IsFeaturePresent(pkgconfig.CloudFoundry) && !pkgconfig.Datadog.GetBool("cloud_foundry_buildpack") {
		containerTagger, err := containertagger.NewContainerTagger()
		if err != nil {
			log.Errorf("Failed to create Cloud Foundry container tagger: %v", err)
		} else {
			containerTagger.Start(ctx)
		}
	}

	// start the cmd HTTP server
	if err = agentAPI.StartServer(
		configService,
		flare,
		server,
		capture,
		serverDebug,
		wmeta,
		logsAgent,
		demultiplexer,
		hostMetadata,
		invAgent,
		demultiplexer,
		invHost,
		secretResolver,
		invChecks,
	); err != nil {
		return log.Errorf("Error while starting api server, exiting: %v", err)
	}

	// start clc runner server
	// only start when the cluster agent is enabled and a cluster check runner host is enabled
	if pkgconfig.Datadog.GetBool("cluster_agent.enabled") && pkgconfig.Datadog.GetBool("clc_runner_enabled") {
		if err = clcrunnerapi.StartCLCRunnerServer(map[string]http.Handler{
			"/telemetry": telemetryHandler,
		}); err != nil {
			return log.Errorf("Error while starting clc runner api server, exiting: %v", err)
		}
	}

	// Create the Leader election engine without initializing it
	if pkgconfig.Datadog.GetBool("leader_election") {
		leaderelection.CreateGlobalLeaderEngine(ctx)
	}

	// start the GUI server
	guiPort := pkgconfig.Datadog.GetString("GUI_port")
	if guiPort == "-1" {
		log.Infof("GUI server port -1 specified: not starting the GUI.")
	} else if err = gui.StartGUIServer(guiPort, flare, invAgent); err != nil {
		log.Errorf("Error while starting GUI: %v", err)
	}

	// Setup stats telemetry handler
	if sender, err := demultiplexer.GetDefaultSender(); err == nil {
		// TODO: to be removed when default telemetry is enabled.
		pkgTelemetry.RegisterStatsSender(sender)
	}

	// Start SNMP trap server
	if traps.IsEnabled(pkgconfig.Datadog) {
		err = traps.StartServer(hostnameDetected, demultiplexer, pkgconfig.Datadog, log)
		if err != nil {
			log.Errorf("Failed to start snmp-traps server: %s", err)
		}
	}

	// Append version and timestamp to version history log file if this Agent is different than the last run version
	installinfo.LogVersionHistory()

	// TODO: (components) - Until the checks are components we set there context so they can depends on components.
	check.InitializeInventoryChecksContext(invChecks)

	// Set up check collector
	common.AC.AddScheduler("check", collector.InitCheckScheduler(common.Coll, demultiplexer), true)
	common.Coll.Start()

	demultiplexer.AddAgentStartupTelemetry(version.AgentVersion)

	// start dogstatsd
	if pkgconfig.Datadog.GetBool("use_dogstatsd") {
		global.DSD = server
		err := server.Start(demultiplexer)
		if err != nil {
			log.Errorf("Could not start dogstatsd: %s", err)
		} else {
			log.Debugf("dogstatsd started")
		}
	}

	// load and run all configs in AD
	common.AC.LoadAndRun(ctx)

	// check for common misconfigurations and report them to log
	misconfig.ToLog(misconfig.CoreAgent)

	// setup the metadata collection
	//
	// The last metadata provider relying on `pkg/metadata` is "agent_checks" from the collector.
	// TODO: (components) - Remove this once the collector and its metadata provider are a component.
	common.MetadataScheduler = pkgMetadata.NewScheduler(demultiplexer)
	if err := pkgMetadata.SetupMetadataCollection(common.MetadataScheduler, pkgMetadata.AllDefaultCollectors); err != nil {
		return err
	}

	// start dependent services
	go startDependentServices()

	if err := otelcollector.Start(); err != nil {
		return err
	}
	// TODO: (components) remove this once migrating the status package to components
	otlpStatus.SetOtelCollector(otelcollector)

	return nil
}

// StopAgentWithDefaults is a temporary way for other packages to use stopAgent.
func StopAgentWithDefaults(server dogstatsdServer.Component, demultiplexer demultiplexer.Component, agentAPI internalAPI.Component) {
	stopAgent(&cliParams{GlobalParams: &command.GlobalParams{}}, server, demultiplexer, agentAPI)
}

// stopAgent Tears down the agent process
func stopAgent(cliParams *cliParams, server dogstatsdServer.Component, demultiplexer demultiplexer.Component, agentAPI internalAPI.Component) {
	// retrieve the agent health before stopping the components
	// GetReadyNonBlocking has a 100ms timeout to avoid blocking
	health, err := health.GetReadyNonBlocking()
	if err != nil {
		pkglog.Warnf("Agent health unknown: %s", err)
	} else if len(health.Unhealthy) > 0 {
		pkglog.Warnf("Some components were unhealthy: %v", health.Unhealthy)
	}

	if common.ExpvarServer != nil {
		if err := common.ExpvarServer.Shutdown(context.Background()); err != nil {
			pkglog.Errorf("Error shutting down expvar server: %v", err)
		}
	}
	server.Stop()
	if common.AC != nil {
		common.AC.Stop()
	}
	if common.MetadataScheduler != nil {
		common.MetadataScheduler.Stop()
	}
	traps.StopServer()
	agentAPI.StopServer()
	clcrunnerapi.StopCLCRunnerServer()
	jmx.StopJmxfetch()

	if demultiplexer != nil {
		demultiplexer.Stop(true)
	}

	gui.StopGUIServer()
	profiler.Stop()

	os.Remove(cliParams.pidfilePath)

	// gracefully shut down any component
	_, cancel := pkgcommon.GetMainCtxCancel()
	cancel()

	pkglog.Info("See ya!")
	pkglog.Flush()
}
