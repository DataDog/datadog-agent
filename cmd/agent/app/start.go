// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package app

import (
	"fmt"
	"runtime"
	"syscall"
	"time"

	_ "expvar" // Blank import used because this isn't directly used in this file
	"net/http"
	_ "net/http/pprof" // Blank import used because this isn't directly used in this file

	"os"
	"os/signal"

	"github.com/DataDog/datadog-agent/cmd/agent/api"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/cmd/agent/gui"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/logs"
	"github.com/DataDog/datadog-agent/pkg/metadata"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/version"
	log "github.com/cihub/seelog"
	"github.com/spf13/cobra"

	// register core checks
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/embed"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/network"
	_ "github.com/DataDog/datadog-agent/pkg/collector/corechecks/system"

	// register metadata providers
	_ "github.com/DataDog/datadog-agent/pkg/collector/metadata"
	_ "github.com/DataDog/datadog-agent/pkg/metadata"
)

var (
	startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start the Agent",
		Long:  `Runs the agent in the foreground`,
		RunE:  start,
	}
)

var (
	// flags variables
	runForeground bool
	pidfilePath   string
)

// run the host metadata collector every 14400 seconds (4 hours)
const hostMetadataCollectorInterval = 14400

// run the agent checks metadata collector every 600 seconds (10 minutes)
const agentChecksMetadataCollectorInterval = 600

// run the resources metadata collector every 300 seconds (5 minutes) by default, configurable
const defaultResourcesMetadataCollectorInterval = 300

func init() {
	// attach the command to the root
	AgentCmd.AddCommand(startCmd)

	// local flags
	startCmd.Flags().StringVarP(&pidfilePath, "pidfile", "p", "", "path to the pidfile")
}

// Start the main loop
func start(cmd *cobra.Command, args []string) error {
	defer func() {
		StopAgent()
	}()

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Make a channel to exit the function
	stopCh := make(chan error)

	go func() {
		// Set up the signals async so we can Start the agent
		select {
		case <-signals.Stopper:
			log.Info("Received stop command, shutting down...")
			stopCh <- nil
		case <-signals.ErrorStopper:
			log.Critical("The Agent has encountered an error, shutting down...")
			stopCh <- fmt.Errorf("shutting down because of an error")
		case sig := <-signalCh:
			log.Infof("Received signal '%s', shutting down...", sig)
			stopCh <- nil
		}
	}()

	if err := StartAgent(); err != nil {
		return err
	}

	select {
	case err := <-stopCh:
		return err
	}
}

// StartAgent Initializes the agent process
func StartAgent() error {

	// Global Agent configuration
	err := common.SetupConfig(confFilePath)
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}
	err = common.SetupConfigOSSpecifics()
	if err != nil {
		return fmt.Errorf("unable to set up OS-specific configuration: %v", err)
	}

	// Setup logger
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
		config.Datadog.GetString("log_level"),
		logFile,
		syslogURI,
		config.Datadog.GetBool("syslog_rfc"),
		config.Datadog.GetBool("syslog_tls"),
		config.Datadog.GetString("syslog_pem"),
		config.Datadog.GetBool("log_to_console"),
		config.Datadog.GetBool("log_format_json"),
	)
	if err != nil {
		return log.Errorf("Error while setting up logging, exiting: %v", err)
	}

	log.Infof("Starting Datadog Agent v%v", version.AgentVersion)

	// Setup expvar server
	var port = config.Datadog.GetString("expvar_port")
	go http.ListenAndServe("127.0.0.1:"+port, http.DefaultServeMux)

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
	if err = api.StartServer(); err != nil {
		return log.Errorf("Error while starting api server, exiting: %v", err)
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
	s := &serializer.Serializer{Forwarder: common.Forwarder}
	agg := aggregator.InitAggregator(s, hostname)
	agg.AddAgentStartupEvent(version.AgentVersion)

	// start dogstatsd
	if config.Datadog.GetBool("use_dogstatsd") {
		var err error
		common.DSD, err = dogstatsd.NewServer(agg.GetChannels())
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

	// setup the metadata collector, this needs a working Python env to function
	if config.Datadog.GetBool("enable_metadata_collection") {
		err = setupMetadataCollection(s, hostname)
		if err != nil {
			return err
		}
	} else {
		log.Warnf("Metadata collection disabled, only do that if another agent/dogstatsd is running on this host")
	}

	// start dependent services
	startDependentServices()
	return nil
}

// setupMetadataCollection initializes the metadata scheduler and its collectors based on the config
func setupMetadataCollection(s *serializer.Serializer, hostname string) error {
	addDefaultResourcesCollector := true
	common.MetadataScheduler = metadata.NewScheduler(s, hostname)
	var C []config.MetadataProviders
	err := config.Datadog.UnmarshalKey("metadata_providers", &C)
	if err == nil {
		log.Debugf("Adding configured providers to the metadata collector")
		for _, c := range C {
			if c.Name == "host" || c.Name == "agent_checks" {
				continue
			}
			if c.Name == "resources" {
				addDefaultResourcesCollector = false
			}
			if c.Interval == 0 {
				log.Infof("Interval of metadata provider '%v' set to 0, skipping provider", c.Name)
				continue
			}
			intl := c.Interval * time.Second
			err = common.MetadataScheduler.AddCollector(c.Name, intl)
			if err != nil {
				log.Errorf("Unable to add '%s' metadata provider: %v", c.Name, err)
			} else {
				log.Infof("Scheduled metadata provider '%v' to run every %v", c.Name, intl)
			}
		}
	} else {
		log.Errorf("Unable to parse metadata_providers config: %v", err)
	}
	// Should be always true, except in some edge cases (multiple agents per host)
	err = common.MetadataScheduler.AddCollector("host", hostMetadataCollectorInterval*time.Second)
	if err != nil {
		return log.Error("Host metadata is supposed to be always available in the catalog!")
	}
	err = common.MetadataScheduler.AddCollector("agent_checks", agentChecksMetadataCollectorInterval*time.Second)
	if err != nil {
		return log.Error("Agent Checks metadata is supposed to be always available in the catalog!")
	}
	if addDefaultResourcesCollector && runtime.GOOS == "linux" {
		err = common.MetadataScheduler.AddCollector("resources", defaultResourcesMetadataCollectorInterval*time.Second)
		if err != nil {
			log.Warn("Could not add resources metadata provider: ", err)
		}
	}

	return nil
}

// StopAgent Tears down the agent process
func StopAgent() {
	// gracefully shut down any component
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
	if common.Forwarder != nil {
		common.Forwarder.Stop()
	}
	logs.Stop()
	gui.StopGUIServer()
	os.Remove(pidfilePath)
	log.Info("See ya!")
	log.Flush()
}
