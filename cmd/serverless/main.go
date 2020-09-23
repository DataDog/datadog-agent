// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

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
	"time"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serverless"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const defaultLogFile = "/var/log/datadog/serverless-agent.log"

var (
	// serverlessAgentCmd is the root command
	serverlessAgentCmd = &cobra.Command{
		Use:   "agent [command]",
		Short: "Datadog Agent at your service.",
		Long: `
Datadog Serverless Agent accepts custom application metrics points over UDP, aggregates and forwards them to Datadog,
where they can be graphed on dashboards. The Datadog Serverless Agent implements the StatsD protocol, along with a few extensions for special Datadog features.`,
	}

	startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start the Serverless Agent",
		Long:  `Runs the Serverless Agent`,
		RunE:  start,
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			av, _ := version.Agent()
			fmt.Println(fmt.Sprintf("Serverless Agent %s - Codename: %s - Commit: %s - Serialization version: %s - Go version: %s",
				av.GetNumber(), av.Meta, av.Commit, serializer.AgentPayloadVersion, runtime.Version()))
		},
	}

	confPath   string
	socketPath string

	statsdServer *dogstatsd.Server

	// envParameters are all the parameters that will be tried to be read from
	// the serverless environment.
	envParameters = []string{
		"DD_API_KEY", "DD_SITE", "DD_LOG_LEVEL",
	}
)

const (
	// loggerName is the name of the serverless agent logger
	loggerName config.LoggerName = "SAGENT"
)

func init() {
	// attach the command to the root
	serverlessAgentCmd.AddCommand(startCmd)
	serverlessAgentCmd.AddCommand(versionCmd)
}

func start(cmd *cobra.Command, args []string) error {
	// Main context passed to components
	ctx, cancel := context.WithCancel(context.Background())
	defer stopCallback(cancel)

	stopCh := make(chan struct{})

	// handle SIGTERM
	go handleSignals(stopCh)

	// run the agent
	err := runAgent(ctx, stopCh)
	if err != nil {
		return err
	}

	// block here until we receive a stop signal
	<-stopCh
	return nil
}

func main() {
	flavor.SetFlavor(flavor.ServerlessAgent)

	// go_expvar server // TODO(remy): shouldn't we remove that for the serverless agent?
	go http.ListenAndServe( //nolint:errcheck
		fmt.Sprintf("127.0.0.1:%d", config.Datadog.GetInt("dogstatsd_stats_port")),
		http.DefaultServeMux)

	// if not command has been provided, run start
	if len(os.Args) == 1 {
		os.Args = append(os.Args, "start")
	}

	if err := serverlessAgentCmd.Execute(); err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}

func runAgent(ctx context.Context, stopCh chan struct{}) (err error) {
	startTime := time.Now()

	// setup logger
	// -----------

	// init the logger configuring it to not log in a file (the first empty string)
	if err = config.SetupLogger(
		loggerName,
		"info", // will be re-set later with the value from the env var
		"",     // logFile -> by setting this to an empty string, we don't write the logs to any file
		"",     // syslog URI
		false,  // syslog_rfc
		true,   // log_to_console
		false,  // log_format_json
	); err != nil {
		log.Criticalf("Unable to setup logger: %s", err)
		return
	}

	// immediately starts the communication server
	daemon := serverless.StartDaemon(stopCh)

	// serverless parts
	// ----------------

	// register
	serverlessId, err := serverless.Register()
	if err != nil {
		// at this point, we were not even able to register, thus, we don't have
		// any Id assigned, thus, we can't report an error to the init error route
		// which needs an Id.
		log.Criticalf("Can't register as a serverless agent: %s", err)
		return
	}

	// read configuration from the environment vars
	// --------------------------------------------

	if _, confErr := config.Load(); confErr != nil {
		log.Info("Configuration will be read from environment variables")
	} else {
		log.Warn("A configuration file has been found, which should not happen in this mode.")
	}

	if !config.Datadog.IsSet("api_key") {
		serverless.ReportInitError(serverlessId, serverless.FatalNoApiKey)
		log.Critical("No API key configured, exiting")
		return
	}

	if logLevel := os.Getenv("DD_LOG_LEVEL"); len(logLevel) > 0 {
		config.ChangeLogLevel(logLevel)
	}

	// setup the forwarder, serializer and aggregator
	// ----------------------------------------------

	keysPerDomain, err := config.GetMultipleEndpoints()
	if err != nil {
		serverless.ReportInitError(serverlessId, serverless.FatalBadEndpoint)
		log.Criticalf("Misconfiguration of agent endpoints: %s", err)
		return
	}
	f := forwarder.NewDefaultForwarder(forwarder.NewOptions(keysPerDomain))
	f.Start() //nolint:errcheck
	serializer := serializer.NewSerializer(f)

	aggregatorInstance := aggregator.InitAggregator(serializer, "serverless")

	// initializes the DogStatsD server
	// --------------------------------

	statsdServer, err = dogstatsd.NewServer(aggregatorInstance)
	if err != nil {
		serverless.ReportInitError(serverlessId, serverless.FatalDogstatsdInit)
		log.Criticalf("Unable to start the DogStatsD server: %s", err)
		return
	}
	statsdServer.ServerlessMode = true // we're running in a serverless environment (will removed host field from samples)

	// run the invocation loop in a routine
	// we don't want to start this mainloop before because once we're waiting on
	// the invocation route, we can't report init errors anymore.
	go func() {
		for {
			serverless.WaitForNextInvocation(stopCh, statsdServer, serverlessId)
		}
	}()

	// DogStatsD daemon ready.
	daemon.SetStatsdServer(statsdServer)
	daemon.ReadyWg.Done()

	log.Debugf("serverless agent ready in %v", time.Since(startTime))
	return
}

// handleSignals handles OS signals, if a SIGTERM is received,
// the serverless agent stops.
func handleSignals(stopCh chan struct{}) {
	// setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// block here until we receive the interrupt signal
	// when received, shutdown the serverless agent.
	for signo := range signalCh {
		switch signo {
		default:
			log.Infof("Received signal '%s', shutting down...", signo)
			stopCh <- struct{}{}
			return
		}
	}
}

func stopCallback(cancel context.CancelFunc) {
	// gracefully shut down any component
	cancel()

	if statsdServer != nil {
		statsdServer.Stop()
	}

	log.Info("See ya!")
	log.Flush()
	return
}
