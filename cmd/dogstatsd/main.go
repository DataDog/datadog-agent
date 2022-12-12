// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate go run ../../pkg/config/render_config.go dogstatsd ../../pkg/config/config_template.yaml ./dist/dogstatsd.yaml

package main

import (
	"context"
	_ "expvar"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/api/healthprobe"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metadata"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	// register all workloadmeta collectors
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors"
)

var (
	metaScheduler  *metadata.Scheduler
	statsd         *dogstatsd.Server
	dogstatsdStats *http.Server
)

type cliParams struct {
	confPath string
}

const (
	// loggerName is the name of the dogstatsd logger
	loggerName pkgconfig.LoggerName = "DSD"
)

func MakeRootCommand() *cobra.Command {
	// dogstatsdCmd is the root command
	dogstatsdCmd := &cobra.Command{
		Use:   "dogstatsd [command]",
		Short: "Datadog dogstatsd at your service.",
		Long: `
DogStatsD accepts custom application metrics points over UDP, and then
periodically aggregates and forwards them to Datadog, where they can be graphed
on dashboards. DogStatsD implements the StatsD protocol, along with a few
extensions for special Datadog features.`,
	}

	for _, cmd := range makeCommands() {
		dogstatsdCmd.AddCommand(cmd)
	}

	return dogstatsdCmd
}

func makeCommands() []*cobra.Command {
	cliParams := &cliParams{}
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start DogStatsD",
		Long:  `Runs DogStatsD in the foreground`,
		RunE: func(*cobra.Command, []string) error {
			return runDogstatsdFct(cliParams, "", start)
		},
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			av, _ := version.Agent()
			fmt.Println(fmt.Sprintf("DogStatsD from Agent %s - Codename: %s - Commit: %s - Serialization version: %s - Go version: %s",
				av.GetNumber(), av.Meta, av.Commit, serializer.AgentPayloadVersion, runtime.Version()))
		},
	}

	// local flags
	startCmd.Flags().StringVarP(&cliParams.confPath, "cfgpath", "c", "", "path to folder containing dogstatsd.yaml")
	pkgconfig.Datadog.BindPFlag("conf_path", startCmd.Flags().Lookup("cfgpath")) //nolint:errcheck
	var socketPath string
	startCmd.Flags().StringVarP(&socketPath, "socket", "s", "", "listen to this socket instead of UDP")
	pkgconfig.Datadog.BindPFlag("dogstatsd_socket", startCmd.Flags().Lookup("socket")) //nolint:errcheck

	return []*cobra.Command{startCmd, versionCmd}
}

func runDogstatsdFct(cliParams *cliParams, defaultConfPath string, fct interface{}) error {
	return fxutil.OneShot(fct,
		fx.Supply(cliParams),
		fx.Supply(core.CreateBundleParams(
			defaultConfPath,
			core.WithConfFilePath(cliParams.confPath),
			core.WithConfigLoadSecrets(true),
			core.WithConfigMissingOK(true),
			core.WithConfigName("dogstatsd"),
		)),
		core.Bundle,
	)
}

func start(cliParams *cliParams, config config.Component) error {
	// Main context passed to components
	ctx, cancel := context.WithCancel(context.Background())
	defer stopAgent(cancel)

	stopCh := make(chan struct{})
	go handleSignals(stopCh)

	err := runAgent(ctx, cliParams, config)
	if err != nil {
		return err
	}

	// Block here until we receive a stop signal
	<-stopCh

	return nil
}

func runAgent(ctx context.Context, cliParams *cliParams, config config.Component) (err error) {
	if len(cliParams.confPath) == 0 {
		log.Infof("Config will be read from env variables")
	}

	// go_expvar server
	port := config.GetInt("dogstatsd_stats_port")
	dogstatsdStats = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: http.DefaultServeMux,
	}
	go func() {
		if err := dogstatsdStats.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("Error creating dogstatsd stats server on port %d: %s", port, err)
		}
	}()

	// Setup logger
	syslogURI := pkgconfig.GetSyslogURI()
	logFile := config.GetString("log_file")
	if logFile == "" {
		logFile = defaultLogFile
	}

	if config.GetBool("disable_file_logging") {
		// this will prevent any logging on file
		logFile = ""
	}

	err = pkgconfig.SetupLogger(
		loggerName,
		config.GetString("log_level"),
		logFile,
		syslogURI,
		config.GetBool("syslog_rfc"),
		config.GetBool("log_to_console"),
		config.GetBool("log_format_json"),
	)
	if err != nil {
		log.Criticalf("Unable to setup logger: %s", err)
		return
	}

	if err := util.SetupCoreDump(); err != nil {
		log.Warnf("Can't setup core dumps: %v, core dumps might not be available after a crash", err)
	}

	if !config.IsSet("api_key") {
		err = log.Critical("no API key configured, exiting")
		return
	}

	// Setup healthcheck port
	var healthPort = config.GetInt("health_port")
	if healthPort > 0 {
		err = healthprobe.Serve(ctx, healthPort)
		if err != nil {
			err = log.Errorf("Error starting health port, exiting: %v", err)
			return
		}
		log.Debugf("Health check listening on port %d", healthPort)
	}

	// setup the demultiplexer
	keysPerDomain, err := pkgconfig.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}

	forwarderOpts := forwarder.NewOptions(keysPerDomain)
	opts := aggregator.DefaultAgentDemultiplexerOptions(forwarderOpts)
	opts.UseOrchestratorForwarder = false
	opts.UseEventPlatformForwarder = false
	opts.UseContainerLifecycleForwarder = false
	opts.EnableNoAggregationPipeline = config.GetBool("dogstatsd_no_aggregation_pipeline")
	hname, err := hostname.Get(context.TODO())
	if err != nil {
		log.Warnf("Error getting hostname: %s", err)
		hname = ""
	}
	log.Debugf("Using hostname: %s", hname)
	demux := aggregator.InitAndStartAgentDemultiplexer(opts, hname)
	demux.AddAgentStartupTelemetry(version.AgentVersion)

	// setup the metadata collector
	metaScheduler = metadata.NewScheduler(demux)
	if err = metadata.SetupMetadataCollection(metaScheduler, []string{"host"}); err != nil {
		metaScheduler.Stop()
		return
	}

	if err = metadata.SetupInventories(metaScheduler, nil); err != nil {
		return
	}

	// container tagging initialisation if origin detection is on
	if config.GetBool("dogstatsd_origin_detection") {
		store := workloadmeta.CreateGlobalStore(workloadmeta.NodeAgentCatalog)
		store.Start(ctx)

		tagger.SetDefaultTagger(local.NewTagger(store))
		if err := tagger.Init(ctx); err != nil {
			log.Errorf("failed to start the tagger: %s", err)
		}
	}

	statsd, err = dogstatsd.NewServer(demux, false)
	if err != nil {
		log.Criticalf("Unable to start dogstatsd: %s", err)
		return
	}
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

	if dogstatsdStats != nil {
		if err := dogstatsdStats.Shutdown(context.Background()); err != nil {
			log.Errorf("Error shutting down dogstatsd stats server: %s", err)
		}
	}

	if statsd != nil {
		statsd.Stop()
	}

	log.Info("See ya!")
	log.Flush()
}
