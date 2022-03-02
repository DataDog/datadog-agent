// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	_ "expvar" // Blank import used because this isn't directly used in this file

	_ "net/http/pprof" // Blank import used because this isn't directly used in this file

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	commonagent "github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/manager"
	"github.com/DataDog/datadog-agent/cmd/security-agent/api"
	"github.com/DataDog/datadog-agent/cmd/security-agent/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	logshttp "github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/remote"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/version"
	ddgostatsd "github.com/DataDog/datadog-go/statsd"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
)

const (
	// loggerName is the name of the security agent logger
	loggerName coreconfig.LoggerName = "SECURITY"
)

var (
	// SecurityAgentCmd is the entry point for security agent CLI commands
	SecurityAgentCmd = &cobra.Command{
		Use:   "datadog-security-agent [command]",
		Short: "Datadog Security Agent at your service.",
		Long: `
Datadog Security Agent takes care of running compliance and security checks.`,
		SilenceUsage: true, // don't print usage on errors
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return common.MergeConfigurationFiles("datadog", confPathArray, cmd.Flags().Lookup("cfgpath").Changed)
		},
	}

	startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start the Security Agent",
		Long:  `Runs Datadog Security agent in the foreground`,
		RunE:  start,
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version info",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			if flagNoColor {
				color.NoColor = true
			}
			av, _ := version.Agent()
			meta := ""
			if av.Meta != "" {
				meta = fmt.Sprintf("- Meta: %s ", color.YellowString(av.Meta))
			}
			fmt.Fprintln(
				color.Output,
				fmt.Sprintf("Security agent %s %s- Commit: '%s' - Serialization version: %s",
					color.BlueString(av.GetNumberAndPre()),
					meta,
					color.GreenString(version.Commit),
					color.MagentaString(serializer.AgentPayloadVersion),
				),
			)
		},
	}

	pidfilePath   string
	confPathArray []string
	flagNoColor   bool

	srv     *api.Server
	stopper startstop.Stopper
)

func init() {
	defaultConfPathArray := []string{
		path.Join(commonagent.DefaultConfPath, "datadog.yaml"),
		path.Join(commonagent.DefaultConfPath, "security-agent.yaml"),
	}
	SecurityAgentCmd.PersistentFlags().StringArrayVarP(&confPathArray, "cfgpath", "c", defaultConfPathArray, "path to a yaml configuration file")
	SecurityAgentCmd.PersistentFlags().BoolVarP(&flagNoColor, "no-color", "n", false, "disable color output")

	SecurityAgentCmd.AddCommand(versionCmd)
	SecurityAgentCmd.AddCommand(complianceCmd)

	if runtimeCmd != nil {
		SecurityAgentCmd.AddCommand(runtimeCmd)
	}

	startCmd.Flags().StringVarP(&pidfilePath, "pidfile", "p", "", "path to the pidfile")
	SecurityAgentCmd.AddCommand(startCmd)
}

func newLogContext(logsConfig *config.LogsConfigKeys, endpointPrefix string, intakeTrackType config.IntakeTrackType, intakeOrigin config.IntakeOrigin, intakeProtocol config.IntakeProtocol) (*config.Endpoints, *client.DestinationsContext, error) {
	endpoints, err := config.BuildHTTPEndpointsWithConfig(logsConfig, endpointPrefix, intakeTrackType, intakeProtocol, intakeOrigin)
	if err != nil {
		endpoints, err = config.BuildHTTPEndpoints(intakeTrackType, intakeProtocol, intakeOrigin)
		if err == nil {
			httpConnectivity := logshttp.CheckConnectivity(endpoints.Main)
			endpoints, err = config.BuildEndpoints(httpConnectivity, intakeTrackType, intakeProtocol, intakeOrigin)
		}
	}

	if err != nil {
		return nil, nil, log.Errorf("Invalid endpoints: %v", err)
	}

	for _, status := range endpoints.GetStatus() {
		log.Info(status)
	}

	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()

	return endpoints, destinationsCtx, nil
}

func start(cmd *cobra.Command, args []string) error {
	// Main context passed to components
	ctx, cancel := context.WithCancel(context.Background())
	defer StopAgent(cancel)

	stopCh := make(chan struct{})
	defer close(stopCh)
	go handleSignals(stopCh)

	err := RunAgent(ctx)
	if err != nil {
		return err
	}

	// Block here until we receive a stop signal
	<-stopCh

	return nil
}

// RunAgent initialized resources and starts API server
func RunAgent(ctx context.Context) (err error) {
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

		return nil
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
	go func() {
		err := http.ListenAndServe("127.0.0.1:"+port, http.DefaultServeMux)
		if err != nil && err != http.ErrServerClosed {
			log.Errorf("Error creating expvar server on port %v: %v", port, err)
		}
	}()

	// get hostname
	// FIXME: use gRPC cross-agent communication API to retrieve hostname
	hostname, err := util.GetHostname(context.TODO())
	if err != nil {
		return log.Errorf("Error while getting hostname, exiting: %v", err)
	}
	log.Infof("Hostname is: %s", hostname)

	// setup the forwarder
	keysPerDomain, err := coreconfig.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}

	forwarderOpts := forwarder.NewOptionsWithResolvers(resolver.NewSingleDomainResolvers(keysPerDomain))
	opts := aggregator.DefaultDemultiplexerOptions(forwarderOpts)
	opts.UseEventPlatformForwarder = false
	opts.UseOrchestratorForwarder = false
	opts.UseContainerLifecycleForwarder = false
	demux := aggregator.InitAndStartAgentDemultiplexer(opts, hostname)
	demux.AddAgentStartupTelemetry(fmt.Sprintf("%s - Datadog Security Agent", version.AgentVersion))

	stopper = startstop.NewSerialStopper()

	// Retrieve statsd host and port from the datadog agent configuration file
	statsdHost := coreconfig.GetBindHost()
	statsdPort := coreconfig.Datadog.GetInt("dogstatsd_port")

	// Create a statsd Client
	statsdAddr := fmt.Sprintf("%s:%d", statsdHost, statsdPort)
	statsdClient, err := ddgostatsd.New(statsdAddr)
	if err != nil {
		return log.Criticalf("Error creating statsd Client: %s", err)
	}

	// Initialize the remote tagger
	if coreconfig.Datadog.GetBool("security_agent.remote_tagger") {
		tagger.SetDefaultTagger(remote.NewTagger())
		err := tagger.Init()
		if err != nil {
			log.Errorf("failed to start the tagger: %s", err)
		}
	}

	complianceAgent, err := startCompliance(hostname, stopper, statsdClient)
	if err != nil {
		return err
	}

	if err = initRuntimeSettings(); err != nil {
		return err
	}

	// start runtime security agent
	runtimeAgent, err := startRuntimeSecurity(hostname, stopper, statsdClient)
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

	log.Infof("Datadog Security Agent is now running.")

	return
}

func initRuntimeSettings() error {
	return settings.RegisterRuntimeSetting(settings.LogLevelRuntimeSetting{})
}

// handleSignals handles OS signals, and sends a message on stopCh when an interrupt
// signal is received.
func handleSignals(stopCh chan struct{}) {
	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGPIPE)

	// Block here until we receive the interrupt signal
	for signo := range signalCh {
		switch signo {
		case syscall.SIGPIPE:
			// By default systemd redirects the stdout to journald. When journald is stopped or crashes we receive a SIGPIPE signal.
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

	log.Info("See ya!")
	log.Flush()
}
