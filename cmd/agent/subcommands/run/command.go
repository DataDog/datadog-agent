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

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"gopkg.in/DataDog/dd-trace-go.v1/profiler"

	"github.com/DataDog/datadog-agent/cmd/agent/api"
	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/misconfig"
	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	global "github.com/DataDog/datadog-agent/cmd/agent/dogstatsd"
	"github.com/DataDog/datadog-agent/cmd/agent/gui"
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands/run/internal/clcrunnerapi"
	"github.com/DataDog/datadog-agent/cmd/manager"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/dogstatsd"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	dogstatsdDebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	pkgforwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/api/healthprobe"
	"github.com/DataDog/datadog-agent/pkg/cloudfoundry/containertagger"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/embed/jmx"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/logs"
	"github.com/DataDog/datadog-agent/pkg/metadata"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	"github.com/DataDog/datadog-agent/pkg/netflow"
	"github.com/DataDog/datadog-agent/pkg/otlp"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
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
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/embed"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/net"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/nvidia/jetson"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/sbom"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/cpu"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/disk"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/filehandles"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/memory"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/uptime"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/winkmem"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/winproc"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/systemd"

	// register metadata providers
	_ "github.com/DataDog/datadog-agent/pkg/collector/metadata"
	_ "github.com/DataDog/datadog-agent/pkg/metadata"
)

// demux is shared between StartAgent and StopAgent.
var demux *aggregator.AgentDemultiplexer

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
				ConfigParams:         config.NewAgentParamsWithSecrets(globalParams.ConfFilePath),
				SysprobeConfigParams: sysprobeconfig.NewParams(sysprobeconfig.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath)),
				LogParams:            log.LogForDaemon(command.LoggerName, "log_file", path.DefaultLogFile),
			}),
			getSharedFxOption(),
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
	config config.Component,
	flare flare.Component,
	sysprobeconfig sysprobeconfig.Component,
	server dogstatsdServer.Component,
	capture replay.Component,
	serverDebug dogstatsdDebug.Component,
	forwarder defaultforwarder.Component,
	cliParams *cliParams,
) error {
	defer func() {
		stopAgent(cliParams, server)
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
		for range sigpipeCh {
			// do nothing
		}
	}()

	if err := startAgent(cliParams, flare, sysprobeconfig, server, capture, serverDebug, forwarder); err != nil {
		return err
	}

	select {
	case err := <-stopCh:
		return err
	}
}

// StartAgentWithDefaults is a temporary way for other packages to use startAgent.
func StartAgentWithDefaults() (dogstatsdServer.Component, error) {
	var dsdServer dogstatsdServer.Component
	// run startAgent in an app, so that the log and config components get initialized
	err := fxutil.OneShot(func(log log.Component,
		config config.Component,
		flare flare.Component,
		sysprobeconfig sysprobeconfig.Component,
		server dogstatsdServer.Component,
		serverDebug dogstatsdDebug.Component,
		capture replay.Component,
		forwarder defaultforwarder.Component,
	) error {
		dsdServer = server

		return startAgent(&cliParams{GlobalParams: &command.GlobalParams{}}, flare, sysprobeconfig, server, capture, serverDebug, forwarder)
	},
		// no config file path specification in this situation
		fx.Supply(core.BundleParams{
			ConfigParams:         config.NewAgentParamsWithSecrets(""),
			SysprobeConfigParams: sysprobeconfig.NewParams(),
			LogParams:            log.LogForDaemon(command.LoggerName, "log_file", path.DefaultLogFile),
		}),
		getSharedFxOption(),
	)

	if err != nil {
		return nil, err
	}
	return dsdServer, nil
}

func getSharedFxOption() fx.Option {
	return fx.Options(
		fx.Supply(flare.NewParams(
			path.GetDistPath(),
			path.PyChecksPath,
			path.DefaultLogFile,
			path.DefaultJmxLogFile,
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
		dogstatsd.Bundle,
	)
}

// startAgent Initializes the agent process
func startAgent(
	cliParams *cliParams,
	flare flare.Component,
	sysprobeconfig sysprobeconfig.Component,
	server dogstatsdServer.Component,
	capture replay.Component,
	serverDebug dogstatsdDebug.Component,
	sharedForwarder defaultforwarder.Component) error {

	var err error

	// Main context passed to components
	common.MainCtx, common.MainCtxCancel = context.WithCancel(context.Background())

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
		pkglog.Infof("Starting Datadog IoT Agent v%v", version.AgentVersion)
	} else {
		pkglog.Infof("Starting Datadog Agent v%v", version.AgentVersion)
	}

	if err := util.SetupCoreDump(pkgconfig.Datadog); err != nil {
		pkglog.Warnf("Can't setup core dumps: %v, core dumps might not be available after a crash", err)
	}

	if v := pkgconfig.Datadog.GetBool("internal_profiling.capture_all_allocations"); v {
		runtime.MemProfileRate = 1
		pkglog.Infof("MemProfileRate set to 1, capturing every single memory allocation!")
	}

	// init settings that can be changed at runtime
	if err := initRuntimeSettings(serverDebug); err != nil {
		pkglog.Warnf("Can't initiliaze the runtime settings: %v", err)
	}

	// Setup Internal Profiling
	common.SetupInternalProfiling(pkgconfig.Datadog, "")

	// Setup expvar server
	telemetryHandler := telemetry.Handler()
	expvarPort := pkgconfig.Datadog.GetString("expvar_port")
	if pkgconfig.Datadog.GetBool("telemetry.enabled") {
		http.Handle("/telemetry", telemetryHandler)
	}
	go func() {
		common.ExpvarServer = &http.Server{
			Addr:    fmt.Sprintf("127.0.0.1:%s", expvarPort),
			Handler: http.DefaultServeMux,
		}
		if err := common.ExpvarServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			pkglog.Errorf("Error creating expvar server on %v: %v", common.ExpvarServer.Addr, err)
		}
	}()

	// Setup healthcheck port
	healthPort := pkgconfig.Datadog.GetInt("health_port")
	if healthPort > 0 {
		err := healthprobe.Serve(common.MainCtx, healthPort)
		if err != nil {
			return pkglog.Errorf("Error starting health port, exiting: %v", err)
		}
		pkglog.Debugf("Health check listening on port %d", healthPort)
	}

	if cliParams.pidfilePath != "" {
		err = pidfile.WritePID(cliParams.pidfilePath)
		if err != nil {
			return pkglog.Errorf("Error while writing PID file, exiting: %v", err)
		}
		pkglog.Infof("pid '%d' written to pid file '%s'", os.Getpid(), cliParams.pidfilePath)
	}

	err = manager.ConfigureAutoExit(common.MainCtx, pkgconfig.Datadog)
	if err != nil {
		return pkglog.Errorf("Unable to configure auto-exit, err: %v", err)
	}

	hostnameDetected, err := hostname.Get(context.TODO())
	if err != nil {
		return pkglog.Errorf("Error while getting hostname, exiting: %v", err)
	}
	pkglog.Infof("Hostname is: %s", hostnameDetected)

	// HACK: init host metadata module (CPU) early to avoid any
	//       COM threading model conflict with the python checks
	err = host.InitHostMetadata()
	if err != nil {
		pkglog.Errorf("Unable to initialize host metadata: %v", err)
	}

	// start remote configuration management
	var configService *remoteconfig.Service
	if pkgconfig.Datadog.GetBool("remote_configuration.enabled") {
		configService, err = remoteconfig.NewService()
		if err != nil {
			pkglog.Errorf("Failed to initialize config management service: %s", err)
		} else if err := configService.Start(context.Background()); err != nil {
			pkglog.Errorf("Failed to start config management service: %s", err)
		}
	}

	// create and setup the Autoconfig instance
	common.LoadComponents(common.MainCtx, pkgconfig.Datadog.GetString("confd_path"))

	// start the cloudfoundry container tagger
	if pkgconfig.IsFeaturePresent(pkgconfig.CloudFoundry) && !pkgconfig.Datadog.GetBool("cloud_foundry_buildpack") {
		containerTagger, err := containertagger.NewContainerTagger()
		if err != nil {
			pkglog.Errorf("Failed to create Cloud Foundry container tagger: %v", err)
		} else {
			containerTagger.Start(common.MainCtx)
		}
	}

	// start the cmd HTTP server
	if err = api.StartServer(configService, flare, server, capture, serverDebug); err != nil {
		return pkglog.Errorf("Error while starting api server, exiting: %v", err)
	}

	// start clc runner server
	// only start when the cluster agent is enabled and a cluster check runner host is enabled
	if pkgconfig.Datadog.GetBool("cluster_agent.enabled") && pkgconfig.Datadog.GetBool("clc_runner_enabled") {
		if err = clcrunnerapi.StartCLCRunnerServer(map[string]http.Handler{
			"/telemetry": telemetryHandler,
		}); err != nil {
			return pkglog.Errorf("Error while starting clc runner api server, exiting: %v", err)
		}
	}

	// start the GUI server
	guiPort := pkgconfig.Datadog.GetString("GUI_port")
	if guiPort == "-1" {
		pkglog.Infof("GUI server port -1 specified: not starting the GUI.")
	} else if err = gui.StartGUIServer(guiPort, flare); err != nil {
		pkglog.Errorf("Error while starting GUI: %v", err)
	}

	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.EnableNoAggregationPipeline = pkgconfig.Datadog.GetBool("dogstatsd_no_aggregation_pipeline")
	demux = aggregator.InitAndStartAgentDemultiplexer(sharedForwarder, opts, hostnameDetected)

	// Setup stats telemetry handler
	if sender, err := demux.GetDefaultSender(); err == nil {
		telemetry.RegisterStatsSender(sender)
	}

	// Start OTLP intake
	otlpEnabled := otlp.IsEnabled(pkgconfig.Datadog)
	inventories.SetAgentMetadata(inventories.AgentOTLPEnabled, otlpEnabled)
	if otlpEnabled {
		var err error
		common.OTLP, err = otlp.BuildAndStart(common.MainCtx, pkgconfig.Datadog, demux.Serializer())
		if err != nil {
			pkglog.Errorf("Could not start OTLP: %s", err)
		} else {
			pkglog.Debug("OTLP pipeline started")
		}
	}

	// Start SNMP trap server
	if traps.IsEnabled() {
		err = traps.StartServer(hostnameDetected, demux)
		if err != nil {
			pkglog.Errorf("Failed to start snmp-traps server: %s", err)
		}
	}

	// Detect Cloud Provider
	go cloudproviders.DetectCloudProvider(context.Background())

	// Append version and timestamp to version history log file if this Agent is different than the last run version
	util.LogVersionHistory()

	// Set up check collector
	common.AC.AddScheduler("check", collector.InitCheckScheduler(common.Coll), true)
	common.Coll.Start()

	demux.AddAgentStartupTelemetry(version.AgentVersion)

	// start dogstatsd
	if pkgconfig.Datadog.GetBool("use_dogstatsd") {
		global.DSD = server
		err := server.Start(demux)
		if err != nil {
			pkglog.Errorf("Could not start dogstatsd: %s", err)
		} else {
			pkglog.Debugf("dogstatsd started")
		}
	}

	// start logs-agent.  This must happen after AutoConfig is set up (via common.LoadComponents)
	if pkgconfig.Datadog.GetBool("logs_enabled") || pkgconfig.Datadog.GetBool("log_enabled") {
		if pkgconfig.Datadog.GetBool("log_enabled") {
			pkglog.Warn(`"log_enabled" is deprecated, use "logs_enabled" instead`)
		}
		if _, err := logs.Start(common.AC); err != nil {
			pkglog.Error("Could not start logs-agent: ", err)
		}
	} else {
		pkglog.Info("logs-agent disabled")
	}

	// Start NetFlow server
	// This must happen after LoadComponents is set up (via common.LoadComponents).
	// netflow.StartServer uses AgentDemultiplexer, that uses ContextResolver, that uses the tagger (initialized by LoadComponents)
	if netflow.IsEnabled() {
		if err = netflow.StartServer(demux); err != nil {
			pkglog.Errorf("Failed to start NetFlow server: %s", err)
		}
	}

	// load and run all configs in AD
	common.AC.LoadAndRun(common.MainCtx)

	// check for common misconfigurations and report them to log
	misconfig.ToLog(misconfig.CoreAgent)

	// setup the metadata collector
	common.MetadataScheduler = metadata.NewScheduler(demux)
	if err := metadata.SetupMetadataCollection(common.MetadataScheduler, metadata.AllDefaultCollectors); err != nil {
		return err
	}

	if err := metadata.SetupInventories(common.MetadataScheduler, common.Coll); err != nil {
		return err
	}

	// start dependent services
	go startDependentServices()

	return nil
}

// StopAgentWithDefaults is a temporary way for other packages to use stopAgent.
func StopAgentWithDefaults(server dogstatsdServer.Component) {
	stopAgent(&cliParams{GlobalParams: &command.GlobalParams{}}, server)
}

// stopAgent Tears down the agent process
func stopAgent(cliParams *cliParams, server dogstatsdServer.Component) {
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
	if common.OTLP != nil {
		common.OTLP.Stop()
	}
	if common.AC != nil {
		common.AC.Stop()
	}
	if common.MetadataScheduler != nil {
		common.MetadataScheduler.Stop()
	}
	traps.StopServer()
	netflow.StopServer()
	api.StopServer()
	clcrunnerapi.StopCLCRunnerServer()
	jmx.StopJmxfetch()

	if demux != nil {
		demux.Stop(true)
	}

	logs.Stop()
	gui.StopGUIServer()
	profiler.Stop()

	os.Remove(cliParams.pidfilePath)

	// gracefully shut down any component
	common.MainCtxCancel()

	pkglog.Info("See ya!")
	pkglog.Flush()
}
