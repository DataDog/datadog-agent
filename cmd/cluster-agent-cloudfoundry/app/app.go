// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build clusterchecks

package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/api"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/commands"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/api/healthprobe"
	"github.com/DataDog/datadog-agent/pkg/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// loggerName is the name of the cluster agent logger
const loggerName config.LoggerName = "CLUSTER"

// FIXME: move SetupAutoConfig and StartAutoConfig in their own package so we don't import cmd/agent
var (
	ClusterAgentCmd = &cobra.Command{
		Use:   "datadog-cluster-agent-cloudfoundry [command]",
		Short: "Datadog Cluster Agent for Cloud Foundry at your service.",
		Long: `
Datadog Cluster Agent for Cloud Foundry takes care of running checks that need to run only
once per cluster.`,
	}

	runCmd = &cobra.Command{
		Use:   "run",
		Short: "Run the Cluster Agent for Cloud Foundry",
		Long:  `Runs Datadog Cluster Agent for Cloud Foundry in the foreground`,
		RunE:  run,
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version info",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagNoColor {
				color.NoColor = true
			}
			av, err := version.Agent()
			if err != nil {
				return err
			}
			meta := ""
			if av.Meta != "" {
				meta = fmt.Sprintf("- Meta: %s ", color.YellowString(av.Meta))
			}
			fmt.Fprintln(
				color.Output,
				fmt.Sprintf("Cluster agent for Cloud Foundry %s %s- Commit: '%s' - Serialization version: %s",
					color.BlueString(av.GetNumberAndPre()),
					meta,
					color.GreenString(version.Commit),
					color.MagentaString(serializer.AgentPayloadVersion),
				),
			)
			return nil
		},
	}

	confPath    string
	flagNoColor bool
)

func init() {
	// attach the commands to the root
	ClusterAgentCmd.AddCommand(runCmd)
	ClusterAgentCmd.AddCommand(versionCmd)
	ClusterAgentCmd.AddCommand(commands.GetClusterChecksCobraCmd(&flagNoColor, &confPath, loggerName))
	ClusterAgentCmd.AddCommand(commands.GetConfigCheckCobraCmd(&flagNoColor, &confPath, loggerName))

	ClusterAgentCmd.PersistentFlags().StringVarP(&confPath, "cfgpath", "c", "", "path to directory containing datadog.yaml")
	ClusterAgentCmd.PersistentFlags().BoolVarP(&flagNoColor, "no-color", "n", false, "disable color output")
}

func run(cmd *cobra.Command, args []string) error {
	// we'll search for a config file named `datadog-cluster.yaml`
	config.Datadog.SetConfigName("datadog-cluster")
	err := common.SetupConfig(confPath)
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}
	// Setup logger
	syslogURI := config.GetSyslogURI()
	logFile := config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultDCALogFile
	}
	if config.Datadog.GetBool("disable_file_logging") {
		// this will prevent any logging on file
		logFile = ""
	}

	mainCtx, mainCtxCancel := context.WithCancel(context.Background())
	defer mainCtxCancel() // Calling cancel twice is safe

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
		return nil
	}

	if !config.Datadog.IsSet("api_key") {
		log.Critical("no API key configured, exiting")
		return nil
	}

	// Setup healthcheck port
	var healthPort = config.Datadog.GetInt("health_port")
	if healthPort > 0 {
		err := healthprobe.Serve(mainCtx, healthPort)
		if err != nil {
			return log.Errorf("Error starting health port, exiting: %v", err)
		}
		log.Debugf("Health check listening on port %d", healthPort)
	}

	// get hostname
	hostname, err := util.GetHostname()
	if err != nil {
		return log.Errorf("Error while getting hostname, exiting: %v", err)
	}
	log.Infof("Hostname is: %s", hostname)

	// setup the forwarder
	keysPerDomain, err := config.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}
	f := forwarder.NewDefaultForwarder(forwarder.NewOptions(keysPerDomain))
	f.Start() //nolint:errcheck
	s := serializer.NewSerializer(f)

	aggregatorInstance := aggregator.InitAggregator(s, hostname, aggregator.ClusterAgentName)
	aggregatorInstance.AddAgentStartupTelemetry(fmt.Sprintf("%s - Datadog Cluster Agent", version.AgentVersion))

	log.Infof("Datadog Cluster Agent is now running.")

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// initialize BBS Cache before starting provider/listener
	err = initializeBBSCache(mainCtx)
	if err != nil {
		return err
	}

	// create and setup the Autoconfig instance
	common.SetupAutoConfig(config.Datadog.GetString("confd_path"))
	// start the autoconfig, this will immediately run any configured check
	common.StartAutoConfig()

	var clusterCheckHandler *clusterchecks.Handler
	clusterCheckHandler, err = setupClusterCheck(mainCtx)
	if err != nil {
		log.Errorf("Error while setting up cluster check Autodiscovery %v", err)
	}

	// Start the cmd HTTPS server
	// We always need to start it, even with nil clusterCheckHandler
	// as it's also used to perform the agent commands (e.g. agent status)
	sc := clusteragent.ServerContext{
		ClusterCheckHandler: clusterCheckHandler,
	}
	if err = api.StartServer(sc); err != nil {
		return log.Errorf("Error while starting agent API, exiting: %v", err)
	}

	// Block here until we receive the interrupt signal
	<-signalCh

	// retrieve the agent health before stopping the components
	// GetReadyNonBlocking has a 100ms timeout to avoid blocking
	health, err := health.GetReadyNonBlocking()
	if err != nil {
		log.Warnf("Cluster Agent health unknown: %s", err)
	} else if len(health.Unhealthy) > 0 {
		log.Warnf("Some components were unhealthy: %v", health.Unhealthy)
	}

	// Cancel the main context to stop components
	mainCtxCancel()

	log.Info("See ya!")
	log.Flush()
	return nil
}

func initializeBBSCache(ctx context.Context) error {
	pollInterval := time.Second * time.Duration(config.Datadog.GetInt("cloud_foundry_bbs.poll_interval"))
	// NOTE: we can't use GetPollInterval in ConfigureGlobalBBSCache, as that causes import cycle
	bc, err := cloudfoundry.ConfigureGlobalBBSCache(
		ctx,
		config.Datadog.GetString("cloud_foundry_bbs.url"),
		config.Datadog.GetString("cloud_foundry_bbs.ca_file"),
		config.Datadog.GetString("cloud_foundry_bbs.cert_file"),
		config.Datadog.GetString("cloud_foundry_bbs.key_file"),
		pollInterval,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize BBS Cache: %s", err.Error())
	}
	log.Info("Waiting for initial warmup of BBS Cache")
	ticker := time.NewTicker(time.Second)
	timer := time.NewTimer(pollInterval * 5)
	for {
		select {
		case <-ticker.C:
			if bc.LastUpdated().After(time.Time{}) {
				return nil
			}
		case <-timer.C:
			ticker.Stop()
			return fmt.Errorf("BBS Cache failed to warm up. Misconfiguration error? Inspect logs")
		}
	}
}

func setupClusterCheck(ctx context.Context) (*clusterchecks.Handler, error) {
	handler, err := clusterchecks.NewHandler(common.AC)
	if err != nil {
		return nil, err
	}
	go handler.Run(ctx)

	log.Info("Started cluster check Autodiscovery")
	return handler, nil
}
