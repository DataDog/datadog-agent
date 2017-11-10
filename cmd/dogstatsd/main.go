// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

//go:generate go run ../../pkg/config/render_config.go dogstatsd ../../pkg/config/config_template.yaml ./dist/dogstatsd.yaml

package main

import (
	_ "expvar"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	log "github.com/cihub/seelog"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metadata"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util"
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
			av, _ := version.New(version.AgentVersion)
			fmt.Println(fmt.Sprintf("DogStatsD from Agent %s - Codename: %s - Commit: %s - Serialization version: %s", av.GetNumber(), av.Meta, av.Commit, serializer.AgentPayloadVersion))
		},
	}

	confPath   string
	socketPath string
)

// run the host metadata collector every 14400 seconds (4 hours)
const hostMetadataCollectorInterval = 14400

func init() {
	// attach the command to the root
	dogstatsdCmd.AddCommand(startCmd)
	dogstatsdCmd.AddCommand(versionCmd)

	// local flags
	startCmd.Flags().StringVarP(&confPath, "cfgpath", "c", "", "path to datadog.yaml")
	config.Datadog.BindPFlag("conf_path", startCmd.Flags().Lookup("cfgpath"))
	startCmd.Flags().StringVarP(&socketPath, "socket", "s", "", "listen to this socket instead of UDP")
	config.Datadog.BindPFlag("dogstatsd_socket", startCmd.Flags().Lookup("socket"))
}

func start(cmd *cobra.Command, args []string) error {
	confFilePath := config.Datadog.GetString("conf_path")
	if len(confFilePath) != 0 {
		// if the configuration file path was supplied on the command line,
		// add that first so it's first in line
		config.Datadog.AddConfigPath(confFilePath)
		// If they set a config file directly, let's try to honor that
		if strings.HasSuffix(confFilePath, ".yaml") {
			config.Datadog.SetConfigFile(confFilePath)
		}
	}
	confErr := config.Datadog.ReadInConfig()
	if confErr != nil {
		log.Infof("unable to parse Datadog config file, running with env variables: %s", confErr)
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

	if !config.Datadog.IsSet("api_key") {
		log.Critical("no API key configured, exiting")
		return nil
	}

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
		err = metaScheduler.AddCollector("host", hostMetadataCollectorInterval*time.Second)
		if err != nil {
			metaScheduler.Stop()
			return log.Error("Host metadata is supposed to be always available in the catalog!")
		}
	} else {
		log.Warnf("Metadata collection disabled, only do that if another agent/dogstatsd is running on this host")
	}

	// container tagging initialisation if origin detection is on
	if config.Datadog.GetBool("dogstatsd_origin_detection") {
		err = tagger.Init()
		if err != nil {
			log.Criticalf("Unable to start tagging system: %s", err)
		}
	}

	aggregatorInstance := aggregator.InitAggregator(s, hname)
	statsd, err := dogstatsd.NewServer(aggregatorInstance.GetChannels())
	if err != nil {
		log.Criticalf("Unable to start dogstatsd: %s", err)
		return nil
	}

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Block here until we receive the interrupt signal
	<-signalCh

	if metaScheduler != nil {
		metaScheduler.Stop()
	}
	statsd.Stop()
	log.Info("See ya!")
	log.Flush()
	return nil
}

func main() {
	// go_expvar server
	go http.ListenAndServe(
		fmt.Sprintf("127.0.0.1:%d", config.Datadog.GetInt("dogstatsd_stats_port")),
		http.DefaultServeMux)

	if err := dogstatsdCmd.Execute(); err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}
