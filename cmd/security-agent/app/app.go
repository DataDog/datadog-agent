// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package app

import (
	"fmt"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	_ "expvar" // Blank import used because this isn't directly used in this file
	"net/http"
	_ "net/http/pprof" // Blank import used because this isn't directly used in this file

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	commonagent "github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/security-agent/api"
	"github.com/DataDog/datadog-agent/cmd/security-agent/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	logshttp "github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	ddgostatsd "github.com/DataDog/datadog-go/statsd"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
)

// loggerName is the name of the security agent logger
const loggerName coreconfig.LoggerName = "SECURITY"

var (
	// SecurityAgentCmd is the entry point for security agent CLI commands
	SecurityAgentCmd = &cobra.Command{
		Use:   "datadog-security-agent [command]",
		Short: "Datadog Security Agent at your service.",
		Long: `
Datadog Security Agent takes care of running compliance and security checks.`,
		SilenceUsage: true, // don't print usage on errors
	}

	startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start the Security Agent",
		Long:  `Runs Datadog Security agent in the foreground`,
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
			av, _ := version.Agent()
			meta := ""
			if av.Meta != "" {
				meta = fmt.Sprintf("- Meta: %s ", color.YellowString(av.Meta))
			}
			fmt.Fprintln(
				color.Output,
				fmt.Sprintf("Security agent %s %s- Commit: '%s' - Serialization version: %s",
					color.BlueString(av.GetNumberAndPre()),
					meta,
					color.GreenString(version.Commit),
					color.MagentaString(serializer.AgentPayloadVersion),
				),
			)
		},
	}

	pidfilePath   string
	confPathArray []string
	flagNoColor   bool
	stopCh        chan struct{}
)

func init() {
	var defaultConfPathArray = []string{path.Join(commonagent.DefaultConfPath, "datadog.yaml"),
		path.Join(commonagent.DefaultConfPath, "security-agent.yaml")}
	SecurityAgentCmd.PersistentFlags().StringArrayVarP(&confPathArray, "cfgpath", "c", defaultConfPathArray, "path to a yaml configuration file")
	SecurityAgentCmd.PersistentFlags().BoolVarP(&flagNoColor, "no-color", "n", false, "disable color output")

	SecurityAgentCmd.AddCommand(versionCmd)
	SecurityAgentCmd.AddCommand(complianceCmd)

	if runtimeCmd != nil {
		SecurityAgentCmd.AddCommand(runtimeCmd)
	}

	startCmd.Flags().StringVarP(&pidfilePath, "pidfile", "p", "", "path to the pidfile")
	SecurityAgentCmd.AddCommand(startCmd)
}

func newLogContext() (*config.Endpoints, *client.DestinationsContext, error) {
	httpConnectivity := config.HTTPConnectivityFailure
	if endpoints, err := config.BuildHTTPEndpoints(); err == nil {
		httpConnectivity = logshttp.CheckConnectivity(endpoints.Main)
	}

	endpoints, err := config.BuildEndpoints(httpConnectivity)
	if err != nil {
		return nil, nil, log.Errorf("Invalid endpoints: %v", err)
	}

	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()

	return endpoints, destinationsCtx, nil
}

func start(cmd *cobra.Command, args []string) error {
	defer log.Flush()

	// Read configuration files received from the command line arguments '-c'
	if err := common.MergeConfigurationFiles("datadog", confPathArray); err != nil {
		return err
	}

	// Setup logger
	syslogURI := coreconfig.GetSyslogURI()
	logFile := coreconfig.Datadog.GetString("security_agent.log_file")
	if coreconfig.Datadog.GetBool("disable_file_logging") {
		// this will prevent any logging on file
		logFile = ""
	}

	err := coreconfig.SetupLogger(
		loggerName,
		coreconfig.Datadog.GetString("log_level"),
		logFile,
		syslogURI,
		coreconfig.Datadog.GetBool("syslog_rfc"),
		coreconfig.Datadog.GetBool("log_to_console"),
		coreconfig.Datadog.GetBool("log_format_json"),
	)
	if err != nil {
		log.Criticalf("Unable to setup logger: %s", err)
		return nil
	}

	if pidfilePath != "" {
		err = pidfile.WritePID(pidfilePath)
		if err != nil {
			return log.Errorf("Error while writing PID file, exiting: %v", err)
		}
		defer os.Remove(pidfilePath)
		log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), pidfilePath)
	}

	// Check if we have at least one component to start based on config
	if !coreconfig.Datadog.GetBool("compliance_config.enabled") && !coreconfig.Datadog.GetBool("runtime_security_config.enabled") {
		log.Infof("All security-agent components are deactivated, exiting")

		// A sleep is necessary so that sysV doesn't think the agent has failed
		// to startup because of an error. Only applies on Debian 7 and SUSE 11.
		time.Sleep(5 * time.Second)

		return nil
	}

	if !coreconfig.Datadog.IsSet("api_key") {
		log.Critical("no API key configured, exiting")
		return nil
	}

	// Setup expvar server
	var port = coreconfig.Datadog.GetString("security_agent.expvar_port")
	coreconfig.Datadog.Set("expvar_port", port)
	if coreconfig.Datadog.GetBool("telemetry.enabled") {
		http.Handle("/telemetry", telemetry.Handler())
	}
	go http.ListenAndServe("127.0.0.1:"+port, http.DefaultServeMux) //nolint:errcheck

	// get hostname
	// FIXME: use gRPC cross-agent communication API to retrieve hostname
	hostname, err := util.GetHostname()
	if err != nil {
		return log.Errorf("Error while getting hostname, exiting: %v", err)
	}
	log.Infof("Hostname is: %s", hostname)

	// setup the forwarder
	keysPerDomain, err := coreconfig.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}
	f := forwarder.NewDefaultForwarder(forwarder.NewOptions(keysPerDomain))
	f.Start() //nolint:errcheck
	s := serializer.NewSerializer(f)

	aggregatorInstance := aggregator.InitAggregator(s, hostname)
	aggregatorInstance.AddAgentStartupTelemetry(fmt.Sprintf("%s - Datadog Security Agent", version.AgentVersion))

	stopper := restart.NewSerialStopper()
	defer stopper.Stop()

	endpoints, dstContext, err := newLogContext()
	if err != nil {
		log.Error(err)
	}
	stopper.Add(dstContext)

	// Retrieve statsd host and port from the datadog agent configuration file
	statsdHost := coreconfig.Datadog.GetString("bind_host")
	statsdPort := coreconfig.Datadog.GetInt("dogstatsd_port")

	// Create a statsd Client
	statsdAddr := fmt.Sprintf("%s:%d", statsdHost, statsdPort)
	statsdClient, err := ddgostatsd.New(statsdAddr)
	if err != nil {
		return log.Criticalf("Error creating statsd Client: %s", err)
	}

	if err = startCompliance(hostname, endpoints, dstContext, stopper, statsdClient); err != nil {
		return err
	}

	// start runtime security agent
	runtimeAgent, err := startRuntimeSecurity(hostname, endpoints, dstContext, stopper, statsdClient)
	if err != nil {
		return err
	}

	srv, err := api.NewServer(runtimeAgent)
	if err != nil {
		return log.Errorf("Error while creating api server, exiting: %v", err)
	}

	if err = srv.Start(); err != nil {
		return log.Errorf("Error while starting api server, exiting: %v", err)
	}
	defer srv.Stop()

	log.Infof("Datadog Security Agent is now running.")

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Block here until we receive the interrupt signal
	<-signalCh

	if stopCh != nil {
		close(stopCh)
	}

	log.Info("See ya!")
	return nil
}
