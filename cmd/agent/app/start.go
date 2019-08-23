// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package app

import (
	"context"
	"fmt"
	"runtime"

	_ "expvar" // Blank import used because this isn't directly used in this file
	"net/http"
	_ "net/http/pprof" // Blank import used because this isn't directly used in this file

	"os"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/api"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/gui"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/api/healthprobe"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/embed/jmx"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/logs"
	"github.com/DataDog/datadog-agent/pkg/metadata"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"

	// register core checks
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/embed"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/net"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/systemd"

	// register metadata providers
	_ "github.com/DataDog/datadog-agent/pkg/collector/metadata"
	_ "github.com/DataDog/datadog-agent/pkg/metadata"
)

var (
	startCmd = &cobra.Command{
		Use:        "start",
		Deprecated: "Use \"run\" instead to start the Agent",
		RunE:       start,
	}
)

func init() {
	// attach the command to the root
	AgentCmd.AddCommand(startCmd)

	// local flags
	startCmd.Flags().StringVarP(&pidfilePath, "pidfile", "p", "", "path to the pidfile")
}

// Start the main loop
func start(cmd *cobra.Command, args []string) error {
	return run(cmd, args)
}

// StartAgent Initializes the agent process
func StartAgent() error {
	// Main context passed to components
	common.MainCtx, common.MainCtxCancel = context.WithCancel(context.Background())

	// Global Agent configuration
	err := common.SetupConfig(confFilePath)
	if err != nil {
		log.Errorf("Failed to setup config %v", err)
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}

	// Setup logger
	if runtime.GOOS != "android" {
		syslogURI := config.GetSyslogURI()
		logFile := config.Datadog.GetString("log_file")
		if logFile == "" {
			logFile = common.DefaultLogFile
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
	} else {
		err = config.SetupLogger(
			loggerName,
			config.Datadog.GetString("log_level"),
			"", // no log file on android
			"", // no syslog on android,
			false,
			true,  // always log to console
			false, // not in json
		)
	}
	if err != nil {
		return fmt.Errorf("Error while setting up logging, exiting: %v", err)
	}

	log.Infof("Starting Datadog Agent v%v", version.AgentVersion)

	// Setup expvar server
	var port = config.Datadog.GetString("expvar_port")
	go http.ListenAndServe("127.0.0.1:"+port, http.DefaultServeMux)

	// Setup healthcheck port
	var healthPort = config.Datadog.GetInt("health_port")
	if healthPort > 0 {
		err := healthprobe.Serve(common.MainCtx, healthPort)
		if err != nil {
			return log.Errorf("Error starting health port, exiting: %v", err)
		}
		log.Debugf("Health check listening on port %d", healthPort)
	}

	if pidfilePath != "" {
		err = pidfile.WritePID(pidfilePath)
		if err != nil {
			return log.Errorf("Error while writing PID file, exiting: %v", err)
		}
		log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), pidfilePath)
	}

	hostname, err := util.GetHostname()
	if err != nil {
		return log.Errorf("Error while getting hostname, exiting: %v", err)
	}
	log.Infof("Hostname is: %s", hostname)

	// HACK: init host metadata module (CPU) early to avoid any
	//       COM threading model conflict with the python checks
	err = host.InitHostMetadata()
	if err != nil {
		log.Errorf("Unable to initialize host metadata: %v", err)
	}

	// start the cmd HTTP server
	if runtime.GOOS != "android" {
		if err = api.StartServer(); err != nil {
			return log.Errorf("Error while starting api server, exiting: %v", err)
		}
	}

	// start the GUI server
	guiPort := config.Datadog.GetString("GUI_port")
	if guiPort == "-1" {
		log.Infof("GUI server port -1 specified: not starting the GUI.")
	} else if err = gui.StartGUIServer(guiPort); err != nil {
		log.Errorf("Error while starting GUI: %v", err)
	}

	// setup the forwarder
	keysPerDomain, err := config.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}
	common.Forwarder = forwarder.NewDefaultForwarder(keysPerDomain)
	log.Debugf("Starting forwarder")
	common.Forwarder.Start()
	log.Debugf("Forwarder started")

	// setup the aggregator
	s := serializer.NewSerializer(common.Forwarder)
	agg := aggregator.InitAggregator(s, hostname, "agent")
	agg.AddAgentStartupTelemetry(version.AgentVersion)

	// start dogstatsd
	if config.Datadog.GetBool("use_dogstatsd") {
		var err error
		common.DSD, err = dogstatsd.NewServer(agg.GetBufferedChannels())
		if err != nil {
			log.Errorf("Could not start dogstatsd: %s", err)
		}
	}
	log.Debugf("statsd started")

	// start logs-agent
	if config.Datadog.GetBool("logs_enabled") || config.Datadog.GetBool("log_enabled") {
		if config.Datadog.GetBool("log_enabled") {
			log.Warn(`"log_enabled" is deprecated, use "logs_enabled" instead`)
		}
		err := logs.Start()
		if err != nil {
			log.Error("Could not start logs-agent: ", err)
		}
	} else {
		log.Info("logs-agent disabled")
	}

	// create and setup the Autoconfig instance
	common.SetupAutoConfig(config.Datadog.GetString("confd_path"))
	// start the autoconfig, this will immediately run any configured check
	common.StartAutoConfig()

	// setup the metadata collector
	common.MetadataScheduler = metadata.NewScheduler(s, hostname)
	if err := metadata.SetupMetadataCollection(common.MetadataScheduler, metadata.AllDefaultCollectors); err != nil {
		return err
	}

	// start dependent services
	startDependentServices()
	return nil
}

// StopAgent Tears down the agent process
func StopAgent() {
	// retrieve the agent health before stopping the components
	// GetStatusNonBlocking has a 100ms timeout to avoid blocking
	health, err := health.GetStatusNonBlocking()
	if err != nil {
		log.Warnf("Agent health unknown: %s", err)
	} else if len(health.Unhealthy) > 0 {
		log.Warnf("Some components were unhealthy: %v", health.Unhealthy)
	}

	// gracefully shut down any component
	common.MainCtxCancel()

	if common.DSD != nil {
		common.DSD.Stop()
	}
	if common.AC != nil {
		common.AC.Stop()
	}
	if common.MetadataScheduler != nil {
		common.MetadataScheduler.Stop()
	}
	api.StopServer()
	jmx.StopJmxfetch()
	if common.Forwarder != nil {
		common.Forwarder.Stop()
	}
	logs.Stop()
	gui.StopGUIServer()
	os.Remove(pidfilePath)
	log.Info("See ya!")
	log.Flush()
}
