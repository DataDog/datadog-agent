// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//go:generate go run ../../pkg/config/render_config.go dogstatsd ../../pkg/config/config_template.yaml ./dist/dogstatsd.yaml

package main

import (
	"context"
	_ "expvar"
	"fmt"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/api/healthprobe"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metadata"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	// dogstatsdCmd is the root command
	dogstatsdCmd = &cobra.Command{
		Use:   "dogstatsd [command]",
		Short: "Datadog dogstatsd at your service.",
		Long: `
DogStatsD accepts custom application metrics points over UDP, and then
periodically aggregates and forwards them to Datadog, where they can be graphed
on dashboards. DogStatsD implements the StatsD protocol, along with a few
extensions for special Datadog features.`,
	}

	startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start DogStatsD",
		Long:  `Runs DogStatsD in the foreground`,
		RunE:  start,
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			av, _ := version.Agent()
			fmt.Println(fmt.Sprintf("DogStatsD from Agent %s - Codename: %s - Commit: %s - Serialization version: %s - Go version: %s",
				av.GetNumber(), av.Meta, av.Commit, serializer.AgentPayloadVersion, runtime.Version()))
		},
	}

	confPath   string
	socketPath string

	metaScheduler *metadata.Scheduler
	statsd        *dogstatsd.Server
)

const (
	// loggerName is the name of the dogstatsd logger
	loggerName config.LoggerName = "DSD"
)

func init() {
	// attach the command to the root
	dogstatsdCmd.AddCommand(startCmd)
	dogstatsdCmd.AddCommand(versionCmd)

	// local flags
	startCmd.Flags().StringVarP(&confPath, "cfgpath", "c", "", "path to folder containing dogstatsd.yaml")
	config.Datadog.BindPFlag("conf_path", startCmd.Flags().Lookup("cfgpath")) //nolint:errcheck
	startCmd.Flags().StringVarP(&socketPath, "socket", "s", "", "listen to this socket instead of UDP")
	config.Datadog.BindPFlag("dogstatsd_socket", startCmd.Flags().Lookup("socket")) //nolint:errcheck
}

func start(cmd *cobra.Command, args []string) error {
	// Main context passed to components
	ctx, cancel := context.WithCancel(context.Background())
	defer stopAgent(cancel)

	stopCh := make(chan struct{})
	go handleSignals(stopCh)

	err := runAgent(ctx)
	if err != nil {
		return err
	}

	// Block here until we receive a stop signal
	<-stopCh

	return nil
}

func runAgent(ctx context.Context) (err error) {
	configFound := false

	// a path to the folder containing the config file was passed
	if len(confPath) != 0 {
		// we'll search for a config file named `dogstatsd.yaml`
		config.Datadog.SetConfigName("dogstatsd")
		config.Datadog.AddConfigPath(confPath)
		_, confErr := config.Load()
		if confErr != nil {
			log.Error(confErr)
		} else {
			configFound = true
		}
	}

	if !configFound {
		log.Infof("Config will be read from env variables")
	}

	// Setup logger
	syslogURI := config.GetSyslogURI()
	logFile := config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = defaultLogFile
	}

	if config.Datadog.GetBool("disable_file_logging") {
		// this will prevent any logging on file
		logFile = ""
	}

	err = config.SetupLogger(
		loggerName,
		config.Datadog.GetString("log_level"),
		logFile,
		syslogURI,
		config.Datadog.GetBool("syslog_rfc"),
		config.Datadog.GetBool("log_to_console"),
		config.Datadog.GetBool("log_format_json"),
	)
	if err != nil {
		log.Criticalf("Unable to setup logger: %s", err)
		return
	}

	// set core limits as soon as possible
	if err := util.SetCoreLimit(); err != nil {
		log.Infof("Can't set core size limit: %v, core dumps might not be available after a crash", err)
	}

	if !config.Datadog.IsSet("api_key") {
		err = log.Critical("no API key configured, exiting")
		return
	}

	// Setup healthcheck port
	var healthPort = config.Datadog.GetInt("health_port")
	if healthPort > 0 {
		err = healthprobe.Serve(ctx, healthPort)
		if err != nil {
			err = log.Errorf("Error starting health port, exiting: %v", err)
			return
		}
		log.Debugf("Health check listening on port %d", healthPort)
	}

	// setup the forwarder
	keysPerDomain, err := config.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}
	f := forwarder.NewDefaultForwarder(forwarder.NewOptions(keysPerDomain))
	f.Start() //nolint:errcheck
	s := serializer.NewSerializer(f)

	hname, err := util.GetHostname()
	if err != nil {
		log.Warnf("Error getting hostname: %s", err)
		hname = ""
	}
	log.Debugf("Using hostname: %s", hname)

	// setup the metadata collector
	metaScheduler = metadata.NewScheduler(s)
	if err = metadata.SetupMetadataCollection(metaScheduler, []string{"host"}); err != nil {
		metaScheduler.Stop()
		return
	}

	if config.Datadog.GetBool("inventories_enabled") {
		if err = metadata.SetupInventories(metaScheduler, nil, nil); err != nil {
			return
		}
	}

	// container tagging initialisation if origin detection is on
	if config.Datadog.GetBool("dogstatsd_origin_detection") {
		tagger.Init()
	}

	aggregatorInstance := aggregator.InitAggregator(s, hname)

	statsd, err = dogstatsd.NewServer(aggregatorInstance, nil)
	if err != nil {
		log.Criticalf("Unable to start dogstatsd: %s", err)
		return
	}

	// send a starting metric and event
	aggregatorInstance.AddAgentStartupTelemetry(version.AgentVersion)
	return
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
			stopCh <- struct{}{}
			return
		}
	}
}

func stopAgent(cancel context.CancelFunc) {
	// retrieve the agent health before stopping the components
	// GetReadyNonBlocking has a 100ms timeout to avoid blocking
	health, err := health.GetReadyNonBlocking()
	if err != nil {
		log.Warnf("Dogstatsd health unknown: %s", err)
	} else if len(health.Unhealthy) > 0 {
		log.Warnf("Some components were unhealthy: %v", health.Unhealthy)
	}

	// gracefully shut down any component
	cancel()

	// stop metaScheduler and statsd if they are instantiated
	if metaScheduler != nil {
		metaScheduler.Stop()
	}

	if statsd != nil {
		statsd.Stop()
	}

	log.Info("See ya!")
	log.Flush()
	return
}
