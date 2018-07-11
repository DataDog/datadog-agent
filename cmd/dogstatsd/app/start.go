// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "expvar"         // Expose internal metrics via expvar
	_ "net/http/pprof" // Pprof for debugging/profiling

	"github.com/spf13/cobra"

	dogstatsdapi "github.com/DataDog/datadog-agent/cmd/dogstatsd/api/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/api"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metadata"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start DogStatsD",
		Long:  `Runs DogStatsD in the foreground`,
		RunE:  start,
	}

	socketPath string
)

// run the host metadata collector every 14400 seconds (4 hours)
const hostMetadataCollectorInterval = 14400

func init() {
	DogstatsdCmd.AddCommand(startCmd)

	// local flags
	startCmd.Flags().StringVarP(&socketPath, "socket", "s", "", "listen to this socket instead of UDP")
	config.Datadog.BindPFlag("dogstatsd_socket", startCmd.Flags().Lookup("socket"))
}

func start(cmd *cobra.Command, args []string) error {
	// go_expvar server
	go http.ListenAndServe(
		fmt.Sprintf("127.0.0.1:%d", config.Datadog.GetInt("dogstatsd_stats_port")),
		http.DefaultServeMux)

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

	// setup the API port and server
	apiServer, err := startAPIServer()
	if err != nil {
		log.Criticalf("Error while starting api server,: %s", err)
		return nil
	}

	// setup the forwarder
	keysPerDomain, err := config.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}
	f := forwarder.NewDefaultForwarder(keysPerDomain)
	f.Start()
	s := serializer.NewSerializer(f)

	hname, err := util.GetHostname()
	if err != nil {
		log.Warnf("Error getting hostname: %s", err)
		hname = ""
	}
	log.Debugf("Using hostname: %s", hname)

	// setup the metadata collector
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
	apiServer.Stop()
	log.Info("See ya!")
	log.Flush()
	return nil
}

func startAPIServer() (*api.Server, error) {
	// Generate the auth token for client requests
	err := apiutil.SetAuthToken()
	if err != nil {
		return nil, err
	}

	// Setup the underlying TCP transport
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%v", config.Datadog.GetInt("cmd_port")))
	if err != nil {
		return nil, err
	}

	// Setup the HTTPS server
	tlsConfig, err := security.GenerateSelfSignedConfig(api.LocalhostHosts)
	if err != nil {
		return nil, err
	}
	server := api.NewServer(listener, tlsConfig, api.DefaultTokenValidator)

	dogstatsdapi.SetupHandlers(server.Router().PathPrefix("/dogstatsd").Subrouter())
	server.Start()

	return server, nil
}
