// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package app

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "expvar" // Blank import used because this isn't directly used in this file

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/api"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/fatih/color"
)

var stopCh chan struct{}

// FIXME: move SetupAutoConfig and StartAutoConfig in their own package so we don't import cmd/agent
var (
	ClusterAgentCmd = &cobra.Command{
		Use:   "cluster-agent [command]",
		Short: "Datadog Cluster Agent at your service.",
		Long: `
Datadog Cluster Agent takes care of running checks that need run only once per cluster.
It also exposes an API for other Datadog agents that provides them with cluster-level
metadata for their metrics.`,
	}

	startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start the Cluster Agent",
		Long:  `Runs Datadog Cluster agent in the foreground`,
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
			av, _ := version.New(version.DCAVersion, version.Commit)
			meta := ""
			if av.Meta != "" {
				meta = fmt.Sprintf("- Meta: %s ", color.YellowString(av.Meta))
			}
			fmt.Fprintln(
				color.Output,
				fmt.Sprintf("Agent %s %s- Commit: '%s' - Serialization version: %s",
					color.BlueString(av.GetNumberAndPre()),
					meta,
					color.GreenString(version.Commit),
					color.MagentaString(serializer.AgentPayloadVersion),
				),
			)
		},
	}

	confPath    string
	flagNoColor bool
)

func init() {
	// attach the command to the root
	ClusterAgentCmd.AddCommand(startCmd)
	ClusterAgentCmd.AddCommand(versionCmd)

	ClusterAgentCmd.PersistentFlags().StringVarP(&confPath, "cfgpath", "c", "", "path to directory containing datadog.yaml")
	ClusterAgentCmd.PersistentFlags().BoolVarP(&flagNoColor, "no-color", "n", false, "disable color output")
	custommetrics.AddFlags(startCmd.Flags())
}

func start(cmd *cobra.Command, args []string) error {
	// Global Agent configuration
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

	err = config.SetupLogger(
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

	// get hostname
	hostname, err := util.GetHostname()
	if err != nil {
		return log.Errorf("Error while getting hostname, exiting: %v", err)
	}
	log.Infof("Hostname is: %s", hostname)

	// start the cmd HTTPS server
	if err = api.StartServer(); err != nil {
		return log.Errorf("Error while starting api server, exiting: %v", err)
	}

	// setup the forwarder
	keysPerDomain, err := config.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}
	f := forwarder.NewDefaultForwarder(keysPerDomain)
	f.Start()
	s := serializer.NewSerializer(f)

	aggregatorInstance := aggregator.InitAggregator(s, hostname)
	aggregatorInstance.AddAgentStartupEvent(fmt.Sprintf("%s - Datadog Cluster Agent", version.DCAVersion))

	clusterAgent, err := clusteragent.Run(aggregatorInstance.GetChannels())
	if err != nil {
		log.Errorf("Could not start the Cluster Agent Process.")

	}

	// Start the Service Mapper.
	asc, err := apiserver.GetAPIClient()
	if err != nil {
		log.Errorf("Could not instantiate the API Server Client: %s", err.Error())
	} else {
		stopCh = make(chan struct{})
		asc.StartClusterMetadataMapping(stopCh)
	}

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	// create and setup the Autoconfig instance
	common.SetupAutoConfig(config.Datadog.GetString("confd_path"))
	// start the autoconfig, this will immediately run any configured check
	common.StartAutoConfig()

	// HPA Process
	if config.Datadog.GetBool("external_metrics_provider.enabled") {
		err = custommetrics.ValidateArgs(args)
		if err != nil {
			log.Error("Couldn't validate args for k8s custom metrics server, not starting it: ", err)

		} else {
			// Start the k8s custom metrics server. This is a blocking call
			err = custommetrics.StartServer()
			if err != nil {
				log.Errorf("Could not start the custom metrics API server: %s", err.Error())
			}
		}
	}
	// Block here until we receive the interrupt signal
	<-signalCh
	if config.Datadog.GetBool("external_metrics_provider.enabled") {
		custommetrics.StopServer()
	}
	clusterAgent.Stop()
	if stopCh != nil {
		close(stopCh)
	}
	log.Info("See ya!")
	log.Flush()
	return nil
}
