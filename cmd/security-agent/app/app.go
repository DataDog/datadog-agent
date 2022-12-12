// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"time"

	_ "expvar" // Blank import used because this isn't directly used in this file

	_ "net/http/pprof" // Blank import used because this isn't directly used in this file

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	commonagent "github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/manager"
	"github.com/DataDog/datadog-agent/cmd/security-agent/api"
	"github.com/DataDog/datadog-agent/cmd/security-agent/app/common"
	"github.com/DataDog/datadog-agent/cmd/security-agent/app/subcommands/compliance"
	subconfig "github.com/DataDog/datadog-agent/cmd/security-agent/app/subcommands/config"
	"github.com/DataDog/datadog-agent/cmd/security-agent/app/subcommands/flare"
	"github.com/DataDog/datadog-agent/cmd/security-agent/app/subcommands/runtime"
	"github.com/DataDog/datadog-agent/cmd/security-agent/app/subcommands/status"
	subversion "github.com/DataDog/datadog-agent/cmd/security-agent/app/subcommands/version"
	compconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/remote"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
)

const (
	// loggerName is the name of the security agent logger
	loggerName coreconfig.LoggerName = common.LoggerName
)

var (
	srv          *api.Server
	expvarServer *http.Server
	stopper      startstop.Stopper
)

func CreateSecurityAgentCmd() *cobra.Command {
	globalParams := common.GlobalParams{}
	var flagNoColor bool

	SecurityAgentCmd := &cobra.Command{
		Use:   "datadog-security-agent [command]",
		Short: "Datadog Security Agent at your service.",
		Long: `
Datadog Security Agent takes care of running compliance and security checks.`,
		SilenceUsage: true, // don't print usage on errors
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if flagNoColor {
				color.NoColor = true
			}

			// TODO(paulcacheux): remove this once all subcommands have been converted to use config component
			_, err := compconfig.MergeConfigurationFiles("datadog", globalParams.ConfPathArray, cmd.Flags().Lookup("cfgpath").Changed)
			return err
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			log.Flush()
		},
	}

	defaultConfPathArray := []string{
		path.Join(commonagent.DefaultConfPath, "datadog.yaml"),
		path.Join(commonagent.DefaultConfPath, "security-agent.yaml"),
	}
	SecurityAgentCmd.PersistentFlags().StringArrayVarP(&globalParams.ConfPathArray, "cfgpath", "c", defaultConfPathArray, "path to a yaml configuration file")
	SecurityAgentCmd.PersistentFlags().BoolVarP(&flagNoColor, "no-color", "n", false, "disable color output")

	factories := []common.SubcommandFactory{
		status.Commands,
		flare.Commands,
		subconfig.Commands,
		compliance.Commands,
		runtime.Commands,
		subversion.Commands,
		StartCommands,
	}

	for _, factory := range factories {
		for _, subcmd := range factory(&globalParams) {
			SecurityAgentCmd.AddCommand(subcmd)
		}
	}

	return SecurityAgentCmd
}

var errAllComponentsDisabled = errors.New("all security-agent component are disabled")

// RunAgent initialized resources and starts API server
func RunAgent(ctx context.Context, pidfilePath string) (err error) {
	// Setup logger
	syslogURI := coreconfig.GetSyslogURI()
	logFile := coreconfig.Datadog.GetString("security_agent.log_file")
	if coreconfig.Datadog.GetBool("disable_file_logging") {
		// this will prevent any logging on file
		logFile = ""
	}

	err = coreconfig.SetupLogger(
		loggerName,
		coreconfig.Datadog.GetString("log_level"),
		logFile,
		syslogURI,
		coreconfig.Datadog.GetBool("syslog_rfc"),
		coreconfig.Datadog.GetBool("log_to_console"),
		coreconfig.Datadog.GetBool("log_format_json"),
	)
	if err != nil {
		log.Criticalf("Unable to setup logger: %s", err)
		return nil
	}

	if err := util.SetupCoreDump(); err != nil {
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
	if !coreconfig.Datadog.GetBool("compliance_config.enabled") && !coreconfig.Datadog.GetBool("runtime_security_config.enabled") {
		log.Infof("All security-agent components are deactivated, exiting")

		// A sleep is necessary so that sysV doesn't think the agent has failed
		// to startup because of an error. Only applies on Debian 7.
		time.Sleep(5 * time.Second)

		return errAllComponentsDisabled
	}

	if !coreconfig.Datadog.IsSet("api_key") {
		log.Critical("no API key configured, exiting")
		return nil
	}

	err = manager.ConfigureAutoExit(ctx)
	if err != nil {
		log.Criticalf("Unable to configure auto-exit, err: %w", err)
		return nil
	}

	// Setup expvar server
	port := coreconfig.Datadog.GetString("security_agent.expvar_port")
	coreconfig.Datadog.Set("expvar_port", port)
	if coreconfig.Datadog.GetBool("telemetry.enabled") {
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

	// get hostname
	// FIXME: use gRPC cross-agent communication API to retrieve hostname
	hostnameDetected, err := hostname.Get(context.TODO())
	if err != nil {
		return log.Errorf("Error while getting hostname, exiting: %v", err)
	}
	log.Infof("Hostname is: %s", hostnameDetected)

	// setup the forwarder
	keysPerDomain, err := coreconfig.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}

	forwarderOpts := forwarder.NewOptionsWithResolvers(resolver.NewSingleDomainResolvers(keysPerDomain))
	opts := aggregator.DefaultAgentDemultiplexerOptions(forwarderOpts)
	opts.UseEventPlatformForwarder = false
	opts.UseOrchestratorForwarder = false
	opts.UseContainerLifecycleForwarder = false
	demux := aggregator.InitAndStartAgentDemultiplexer(opts, hostnameDetected)
	demux.AddAgentStartupTelemetry(fmt.Sprintf("%s - Datadog Security Agent", version.AgentVersion))

	stopper = startstop.NewSerialStopper()

	// Create a statsd Client
	statsdAddr := os.Getenv("STATSD_URL")
	if statsdAddr == "" {
		// Retrieve statsd host and port from the datadog agent configuration file
		statsdHost := coreconfig.GetBindHost()
		statsdPort := coreconfig.Datadog.GetInt("dogstatsd_port")

		statsdAddr = fmt.Sprintf("%s:%d", statsdHost, statsdPort)
	}

	statsdClient, err := ddgostatsd.New(statsdAddr)
	if err != nil {
		return log.Criticalf("Error creating statsd Client: %s", err)
	}

	workloadmetaCollectors := workloadmeta.NodeAgentCatalog
	if coreconfig.Datadog.GetBool("security_agent.remote_workloadmeta") {
		workloadmetaCollectors = workloadmeta.RemoteCatalog
	}

	// Start workloadmeta store
	store := workloadmeta.CreateGlobalStore(workloadmetaCollectors)
	store.Start(ctx)

	// Initialize the remote tagger
	if coreconfig.Datadog.GetBool("security_agent.remote_tagger") {
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

	complianceAgent, err := compliance.StartCompliance(hostnameDetected, stopper, statsdClient)
	if err != nil {
		return err
	}

	if err = initRuntimeSettings(); err != nil {
		return err
	}

	// start runtime security agent
	runtimeAgent, err := runtime.StartRuntimeSecurity(hostnameDetected, stopper, statsdClient)
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

	if err := setupInternalProfiling(); err != nil {
		return log.Errorf("Error while setuping internal profiling, exiting: %v", err)
	}

	log.Infof("Datadog Security Agent is now running.")

	return
}

func initRuntimeSettings() error {
	return settings.RegisterRuntimeSetting(settings.LogLevelRuntimeSetting{})
}

// StopAgent stops the API server and clean up resources
func StopAgent(cancel context.CancelFunc) {
	// retrieve the agent health before stopping the components
	// GetReadyNonBlocking has a 100ms timeout to avoid blocking
	health, err := health.GetReadyNonBlocking()
	if err != nil {
		log.Warnf("Security Agent health unknown: %s", err)
	} else if len(health.Unhealthy) > 0 {
		log.Warnf("Some components were unhealthy: %v", health.Unhealthy)
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
	log.Flush()
}

func setupInternalProfiling() error {
	cfg := coreconfig.Datadog
	if cfg.GetBool(secAgentKey("internal_profiling.enabled")) {
		v, _ := version.Agent()

		cfgSite := cfg.GetString(secAgentKey("internal_profiling.site"))
		cfgURL := cfg.GetString(secAgentKey("security_agent.internal_profiling.profile_dd_url"))

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

		profSettings := profiling.Settings{
			ProfilingURL:         site,
			Env:                  cfg.GetString(secAgentKey("internal_profiling.env")),
			Service:              "security-agent",
			Period:               cfg.GetDuration(secAgentKey("internal_profiling.period")),
			CPUDuration:          cfg.GetDuration("internal_profiling.cpu_duration"),
			MutexProfileFraction: cfg.GetInt(secAgentKey("internal_profiling.mutex_profile_fraction")),
			BlockProfileRate:     cfg.GetInt(secAgentKey("internal_profiling.block_profile_rate")),
			WithGoroutineProfile: cfg.GetBool(secAgentKey("internal_profiling.enable_goroutine_stacktraces")),
			Tags:                 []string{fmt.Sprintf("version:%v", v)},
		}

		return profiling.Start(profSettings)
	}

	return nil
}

func secAgentKey(sub string) string {
	return fmt.Sprintf("security_agent.%s", sub)
}
