// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/api/healthprobe"
	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/agent"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// loggerName is the name of the security agent logger
const loggerName coreconfig.LoggerName = "SECURITY"

var (
	SecurityAgentCmd = &cobra.Command{
		Use:   "datadog-security-agent [command]",
		Short: "Datadog Security Agent at your service.",
		Long: `
Datadog Security Agent takes care of running compliance and security checks.`,
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

	confPath    string
	flagNoColor bool
	stopCh      chan struct{}
)

func init() {
	// attach the command to the root
	SecurityAgentCmd.AddCommand(startCmd)
	SecurityAgentCmd.AddCommand(versionCmd)
	SecurityAgentCmd.AddCommand(complianceCmd)

	SecurityAgentCmd.PersistentFlags().StringVarP(&confPath, "cfgpath", "c", "", "path to directory containing datadog.yaml")
	SecurityAgentCmd.PersistentFlags().BoolVarP(&flagNoColor, "no-color", "n", false, "disable color output")
}

func start(cmd *cobra.Command, args []string) error {
	// we'll search for a config file named `datadog-security.yaml`
	coreconfig.Datadog.SetConfigName("datadog-security")
	err := common.SetupConfig(confPath)
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}
	// Setup logger
	syslogURI := coreconfig.GetSyslogURI()
	logFile := coreconfig.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultDCALogFile
	}
	if coreconfig.Datadog.GetBool("disable_file_logging") {
		// this will prevent any logging on file
		logFile = ""
	}

	mainCtx, mainCtxCancel := context.WithCancel(context.Background())
	defer mainCtxCancel() // Calling cancel twice is safe

	err = coreconfig.SetupLogger(
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

	if !coreconfig.Datadog.IsSet("api_key") {
		log.Critical("no API key configured, exiting")
		return nil
	}

	// Setup healthcheck port
	var healthPort = coreconfig.Datadog.GetInt("health_port")
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
	keysPerDomain, err := coreconfig.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}
	f := forwarder.NewDefaultForwarder(forwarder.NewOptions(keysPerDomain))
	f.Start() //nolint:errcheck
	s := serializer.NewSerializer(f)

	aggregatorInstance := aggregator.InitAggregator(s, hostname, aggregator.SecurityAgentName)
	aggregatorInstance.AddAgentStartupTelemetry(fmt.Sprintf("%s - Datadog Security Agent", version.AgentVersion))

	health := health.RegisterLiveness("security-agent")

	complianceEnabled := coreconfig.Datadog.GetBool("compliance_config.enabled")
	if complianceEnabled {

		httpConnectivity := config.HTTPConnectivityFailure
		if endpoints, err := config.BuildHTTPEndpoints(); err == nil {
			httpConnectivity = http.CheckConnectivity(endpoints.Main)
		}

		endpoints, err := config.BuildEndpoints(httpConnectivity)
		if err != nil {
			return fmt.Errorf("Invalid endpoints: %v", err)
		}

		destinationsCtx := client.NewDestinationsContext()
		destinationsCtx.Start()
		defer destinationsCtx.Stop()

		// setup the auditor
		auditor := auditor.New(coreconfig.Datadog.GetString("compliance_config.run_path"), health)
		auditor.Start()
		defer auditor.Stop()

		// setup the pipeline provider that provides pairs of processor and sender
		pipelineProvider := pipeline.NewProvider(config.NumberOfPipelines, auditor, nil, endpoints, destinationsCtx)
		pipelineProvider.Start()
		defer pipelineProvider.Stop()

		logSource := config.NewLogSource("compliance-agent", &config.LogsConfig{
			Type:    "compliance",
			Service: "compliance-agent",
			Source:  "compliance-agent",
		})

		reporter := compliance.NewReporter(logSource, pipelineProvider.NextPipelineChan())

		runner := runner.NewRunner()
		defer runner.Stop()

		scheduler := scheduler.NewScheduler(runner.GetChan())
		runner.SetScheduler(scheduler)

		checkInterval := coreconfig.Datadog.GetDuration("compliance_config.check_interval")
		configDir := coreconfig.Datadog.GetString("compliance_config.dir")

		compAgent := agent.New(reporter, scheduler, configDir, checkInterval)
		err = compAgent.Run()
		if err != nil {
			log.Errorf("Error starting compliance agent, exiting: %v", err)
		}
		defer compAgent.Stop()

		log.Infof("Running compliance checks every %s", checkInterval.String())
	}

	log.Infof("Datadog Security Agent is now running.")

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Block here until we receive the interrupt signal
	<-signalCh

	// Cancel the main context to stop components
	mainCtxCancel()

	if stopCh != nil {
		close(stopCh)
	}

	log.Info("See ya!")
	log.Flush()
	return nil
}
