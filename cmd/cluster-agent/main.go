// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "expvar" // Blank import used because this isn't directly used in this file

	log "github.com/cihub/seelog"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metadata"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	clusterAgentCmd = &cobra.Command{
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
		Short: "Print the version number",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			av, _ := version.New(version.AgentVersion)
			fmt.Println(fmt.Sprintf("Cluster Agent from Agent %s - Codename: %s - Commit: %s - Serialization version: %s", av.GetNumber(), av.Meta, av.Commit, serializer.AgentPayloadVersion))
		},
	}

	confPath string
)

// run the host metadata collector every 14400 seconds (4 hours)
const hostMetadataCollectorInterval = 14400

func init() {
	// attach the command to the root
	clusterAgentCmd.AddCommand(startCmd)
	clusterAgentCmd.AddCommand(versionCmd)

	// local flags
	startCmd.Flags().StringVarP(&confPath, "cfgpath", "c", "", "path to datadog.yaml")
	config.Datadog.BindPFlag("conf_path", startCmd.Flags().Lookup("cfgpath"))
}

func start(cmd *cobra.Command, args []string) error {
	config.Datadog.SetConfigFile(config.Datadog.GetString("conf_path"))
	confErr := config.Datadog.ReadInConfig()

	// Setup logger
	syslogURI := config.GetSyslogURI()
	logFile := config.Datadog.GetString("log_file")
	if config.Datadog.GetBool("disable_file_logging") {
		// this will prevent any logging on file
		logFile = ""
	}
	err := config.SetupLogger(
		config.Datadog.GetString("log_level"),
		logFile,
		syslogURI,
		config.Datadog.GetBool("syslog_rfc"),
		config.Datadog.GetBool("syslog_tls"),
		config.Datadog.GetString("syslog_pem"),
		config.Datadog.GetBool("log_to_console"),
	)
	if err != nil {
		log.Criticalf("Unable to setup logger: %s", err)
		return nil
	}

	if confErr != nil {
		log.Infof("unable to parse Datadog config file, running with env variables: %s", confErr)
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

	// start the cmd HTTP server
	// if err = api.StartServer(); err != nil {
	// 	return log.Errorf("Error while starting api server, exiting: %v", err)
	// }

	// Start the k8s custom metrics server
	custommetrics.StartServer()

	// setup the forwarder
	keysPerDomain, err := config.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}
	f := forwarder.NewDefaultForwarder(keysPerDomain)
	f.Start()
	s := &serializer.Serializer{Forwarder: f}

	hname, err := util.GetHostname()
	if err != nil {
		log.Warnf("Error getting hostname: %s", err)
		hname = ""
	}
	log.Debugf("Using hostname: %s", hname)

	var metaScheduler *metadata.Scheduler
	if config.Datadog.GetBool("enable_metadata_collection") {
		// start metadata collection
		metaScheduler = metadata.NewScheduler(s, hname)

		// add the host metadata collector
		// TODO: make it configurable. We shouldn't report them
		// twice if cluster agent and normal agents are on the same node
		err = metaScheduler.AddCollector("host", hostMetadataCollectorInterval*time.Second)
		if err != nil {
			metaScheduler.Stop()
			return log.Error("Host metadata is supposed to be always available in the catalog!")
		}
	} else {
		log.Warnf("Metadata collection disabled, only do that if another agent/dogstatsd is running on this host")
	}

	aggregatorInstance := aggregator.InitAggregator(s, hname)
	// TODO: run the actual thing
	clusterAgent, err := clusteragent.Run(aggregatorInstance.GetChannels())

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Block here until we receive the interrupt signal
	<-signalCh

	if metaScheduler != nil {
		metaScheduler.Stop()
	}
	custommetrics.StopServer()
	clusterAgent.Stop()
	log.Info("See ya!")
	log.Flush()
	return nil

}

func main() {
	// go_expvar server
	go http.ListenAndServe(
		fmt.Sprintf("127.0.0.1:%d", config.Datadog.GetInt("clusteragent_expvar_port")),
		http.DefaultServeMux)

	if err := clusterAgentCmd.Execute(); err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}
