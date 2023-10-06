// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package start

import (
	"context"
	"errors"
	_ "expvar" // Blank import used because this isn't directly used in this file
	"fmt"
	"net/http"
	_ "net/http/pprof" // Blank import used because this isn't directly used in this file
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/cmd/manager"
	"github.com/DataDog/datadog-agent/cmd/security-agent/api"
	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/cmd/security-agent/flags"
	"github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/compliance"
	"github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/runtime"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/remote"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

type cliParams struct {
	*command.GlobalParams

	pidfilePath string
}

func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	params := &cliParams{
		GlobalParams: globalParams,
	}

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Security Agent",
		Long:  `Runs Datadog Security agent in the foreground`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: Similar to the agent itself, once the security agent is represented as a component, and not a function (start),
			// this will use `fxutil.Run` instead of `fxutil.OneShot`.
			return fxutil.OneShot(start,
				fx.Supply(params),
				fx.Supply(core.BundleParams{
					ConfigParams:         config.NewSecurityAgentParams(params.ConfigFilePaths),
					SysprobeConfigParams: sysprobeconfig.NewParams(sysprobeconfig.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath)),
					LogParams:            log.LogForDaemon(command.LoggerName, "security_agent.log_file", pkgconfig.DefaultSecurityAgentLogFile),
				}),
				core.Bundle,
				forwarder.Bundle,
				fx.Provide(defaultforwarder.NewParamsWithResolvers),
			)
		},
	}

	startCmd.Flags().StringVarP(&params.pidfilePath, flags.PidFile, "p", "", "path to the pidfile")

	return []*cobra.Command{startCmd}
}

func start(log log.Component, config config.Component, sysprobeconfig sysprobeconfig.Component, telemetry telemetry.Component, forwarder defaultforwarder.Component, params *cliParams) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer StopAgent(cancel, log)

	err := RunAgent(ctx, log, config, sysprobeconfig, telemetry, forwarder, params.pidfilePath)
	if errors.Is(err, errAllComponentsDisabled) || errors.Is(err, errNoAPIKeyConfigured) {
		return nil
	}
	if err != nil {
		return err
	}

	stopCh := make(chan struct{})
	defer close(stopCh)
	go handleSignals(log, stopCh)

	// Block here until we receive a stop signal
	<-stopCh

	return nil
}

// handleSignals handles OS signals, and sends a message on stopCh when an interrupt
// signal is received.
func handleSignals(log log.Component, stopCh chan struct{}) {
	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGPIPE)

	// Block here until we receive the interrupt signal
	for signo := range signalCh {
		switch signo {
		case syscall.SIGPIPE:
			// By default, systemd redirects the stdout to journald. When journald is stopped or crashes we receive a SIGPIPE signal.
			// Go ignores SIGPIPE signals unless it is when stdout or stdout is closed, in this case the agent is stopped.
			// We never want dogstatsd to stop upon receiving SIGPIPE, so we intercept the SIGPIPE signals and just discard them.
		default:
			log.Infof("Received signal '%s', shutting down...", signo)

			_ = tagger.Stop()

			stopCh <- struct{}{}
			return
		}
	}
}

var (
	stopper      startstop.Stopper
	srv          *api.Server
	expvarServer *http.Server
)

var errAllComponentsDisabled = errors.New("all security-agent component are disabled")
var errNoAPIKeyConfigured = errors.New("no API key configured")

// RunAgent initialized resources and starts API server
func RunAgent(ctx context.Context, log log.Component, config config.Component, sysprobeconfig sysprobeconfig.Component, telemetry telemetry.Component, forwarder defaultforwarder.Component, pidfilePath string) (err error) {
	if err := util.SetupCoreDump(config); err != nil {
		log.Warnf("Can't setup core dumps: %v, core dumps might not be available after a crash", err)
	}

	if pidfilePath != "" {
		err = pidfile.WritePID(pidfilePath)
		if err != nil {
			return log.Errorf("Error while writing PID file, exiting: %v", err)
		}
		defer os.Remove(pidfilePath)
		log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), pidfilePath)
	}

	// Check if we have at least one component to start based on config
	if !config.GetBool("compliance_config.enabled") && !config.GetBool("runtime_security_config.enabled") {
		log.Infof("All security-agent components are deactivated, exiting")

		// A sleep is necessary so that sysV doesn't think the agent has failed
		// to startup because of an error. Only applies on Debian 7.
		time.Sleep(5 * time.Second)

		return errAllComponentsDisabled
	}

	if !config.IsSet("api_key") {
		log.Critical("No API key configured, exiting")

		// A sleep is necessary so that sysV doesn't think the agent has failed
		// to startup because of an error. Only applies on Debian 7.
		time.Sleep(5 * time.Second)

		return errNoAPIKeyConfigured
	}

	err = manager.ConfigureAutoExit(ctx, config)
	if err != nil {
		log.Criticalf("Unable to configure auto-exit, err: %w", err)
		return nil
	}

	// Setup expvar server
	port := config.GetString("security_agent.expvar_port")
	pkgconfig.Datadog.Set("expvar_port", port)
	if config.GetBool("telemetry.enabled") {
		http.Handle("/telemetry", telemetry.Handler())
	}
	expvarServer := &http.Server{
		Addr:    "127.0.0.1:" + port,
		Handler: http.DefaultServeMux,
	}
	go func() {
		err := expvarServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Errorf("Error creating expvar server on port %v: %v", port, err)
		}
	}()

	hostnameDetected, err := utils.GetHostname()
	if err != nil {
		log.Warnf("Could not resolve hostname from core-agent: %v", err)
		hostnameDetected, err = hostname.Get(ctx)
		if err != nil {
			return log.Errorf("Error while getting hostname, exiting: %v", err)
		}
	}
	log.Infof("Hostname is: %s", hostnameDetected)

	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.UseEventPlatformForwarder = false
	opts.UseOrchestratorForwarder = false
	demux := aggregator.InitAndStartAgentDemultiplexer(log, forwarder, opts, hostnameDetected)
	demux.AddAgentStartupTelemetry(fmt.Sprintf("%s - Datadog Security Agent", version.AgentVersion))

	stopper = startstop.NewSerialStopper()

	// Create a statsd Client
	statsdAddr := os.Getenv("STATSD_URL")
	if statsdAddr == "" {
		// Retrieve statsd host and port from the datadog agent configuration file
		statsdHost := pkgconfig.GetBindHost()
		statsdPort := config.GetInt("dogstatsd_port")

		statsdAddr = fmt.Sprintf("%s:%d", statsdHost, statsdPort)
	}

	statsdClient, err := ddgostatsd.New(statsdAddr)
	if err != nil {
		return log.Criticalf("Error creating statsd Client: %s", err)
	}

	workloadmetaCollectors := workloadmeta.NodeAgentCatalog
	if config.GetBool("security_agent.remote_workloadmeta") {
		workloadmetaCollectors = workloadmeta.RemoteCatalog
	}

	// Start workloadmeta store
	store := workloadmeta.CreateGlobalStore(workloadmetaCollectors)
	store.Start(ctx)

	// Initialize the remote tagger
	if config.GetBool("security_agent.remote_tagger") {
		options, err := remote.NodeAgentOptions()
		if err != nil {
			log.Errorf("unable to configure the remote tagger: %s", err)
		} else {
			tagger.SetDefaultTagger(remote.NewTagger(options))
			err := tagger.Init(ctx)
			if err != nil {
				log.Errorf("failed to start the tagger: %s", err)
			}
		}
	}

	complianceAgent, err := compliance.StartCompliance(log, config, sysprobeconfig, hostnameDetected, stopper, statsdClient)
	if err != nil {
		return err
	}

	if err = initRuntimeSettings(); err != nil {
		return err
	}

	// start runtime security agent
	runtimeAgent, err := runtime.StartRuntimeSecurity(log, config, hostnameDetected, stopper, statsdClient, aggregator.GetSenderManager())
	if err != nil {
		return err
	}

	srv, err = api.NewServer(runtimeAgent, complianceAgent)
	if err != nil {
		return log.Errorf("Error while creating api server, exiting: %v", err)
	}

	if err = srv.Start(); err != nil {
		return log.Errorf("Error while starting api server, exiting: %v", err)
	}

	if err := setupInternalProfiling(config); err != nil {
		return log.Errorf("Error while setuping internal profiling, exiting: %v", err)
	}

	log.Infof("Datadog Security Agent is now running.")

	return
}

func initRuntimeSettings() error {
	return settings.RegisterRuntimeSetting(settings.NewLogLevelRuntimeSetting())
}

// StopAgent stops the API server and clean up resources
func StopAgent(cancel context.CancelFunc, log log.Component) {
	// retrieve the agent health before stopping the components
	// GetReadyNonBlocking has a 100ms timeout to avoid blocking
	healthStatus, err := health.GetReadyNonBlocking()
	if err != nil {
		log.Warnf("Security Agent health unknown: %s", err)
	} else if len(healthStatus.Unhealthy) > 0 {
		log.Warnf("Some components were unhealthy: %v", healthStatus.Unhealthy)
	}

	// gracefully shut down any component
	cancel()

	// stop metaScheduler and statsd if they are instantiated
	if stopper != nil {
		stopper.Stop()
	}

	if srv != nil {
		srv.Stop()
	}
	if expvarServer != nil {
		if err := expvarServer.Shutdown(context.Background()); err != nil {
			log.Errorf("Error shutting down expvar server: %v", err)
		}
	}

	log.Info("See ya!")
}

func setupInternalProfiling(config config.Component) error {
	if config.GetBool(secAgentKey("internal_profiling.enabled")) {
		v, _ := version.Agent()

		cfgSite := config.GetString(secAgentKey("internal_profiling.site"))
		cfgURL := config.GetString(secAgentKey("security_agent.internal_profiling.profile_dd_url"))

		// check if TRACE_AGENT_URL is set, in which case, forward the profiles to the trace agent
		var site string
		if traceAgentURL := os.Getenv("TRACE_AGENT_URL"); len(traceAgentURL) > 0 {
			site = fmt.Sprintf(profiling.ProfilingLocalURLTemplate, traceAgentURL)
		} else {
			site = fmt.Sprintf(profiling.ProfilingURLTemplate, cfgSite)
			if cfgURL != "" {
				site = cfgURL
			}
		}

		tags := config.GetStringSlice(secAgentKey("internal_profiling.extra_tags"))
		tags = append(tags, fmt.Sprintf("version:%v", v))

		profSettings := profiling.Settings{
			ProfilingURL:         site,
			Env:                  config.GetString(secAgentKey("internal_profiling.env")),
			Service:              "security-agent",
			Period:               config.GetDuration(secAgentKey("internal_profiling.period")),
			CPUDuration:          config.GetDuration(secAgentKey("internal_profiling.cpu_duration")),
			MutexProfileFraction: config.GetInt(secAgentKey("internal_profiling.mutex_profile_fraction")),
			BlockProfileRate:     config.GetInt(secAgentKey("internal_profiling.block_profile_rate")),
			WithGoroutineProfile: config.GetBool(secAgentKey("internal_profiling.enable_goroutine_stacktraces")),
			WithDeltaProfiles:    config.GetBool(secAgentKey("internal_profiling.delta_profiles")),
			Socket:               config.GetString(secAgentKey("internal_profiling.unix_socket")),
			Tags:                 tags,
		}

		return profiling.Start(profSettings)
	}

	return nil
}

func secAgentKey(sub string) string {
	return fmt.Sprintf("security_agent.%s", sub)
}
