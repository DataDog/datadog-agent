// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && clusterchecks
// +build !windows,clusterchecks

//go:generate go run ../../pkg/config/render_config.go dcacf ../../pkg/config/config_template.yaml ../../cloudfoundry.yaml

package main

import (
	"context"
	_ "expvar" // Blank import used because this isn't directly used in this file
	"fmt"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/commands"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/fatih/color"
	"github.com/gorilla/mux"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent-cloudfoundry/app"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/api"
	dcav1 "github.com/DataDog/datadog-agent/cmd/cluster-agent/api/v1"
	clusteragentapp "github.com/DataDog/datadog-agent/cmd/cluster-agent/app"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/api/healthprobe"
	"github.com/DataDog/datadog-agent/pkg/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
	"github.com/DataDog/datadog-agent/pkg/collector"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"go.uber.org/fx"
	_ "net/http/pprof" // Blank import used because this isn't directly used in this file
	"os"
	"os/signal"
	"syscall"
)

// loggerName is the name of the cluster agent logger
const loggerName pkgconfig.LoggerName = "CLUSTER"

type cliParams struct {
	confPath    string
	flagNoColor bool
}

func MakeRootCommand() *cobra.Command {
	// clusterAgentCmd is the root command
	cliParams := &cliParams{}
	clusterAgentCmd := &cobra.Command{
		Use:   "datadog-cluster-agent-cloudfoundry [command]",
		Short: "Datadog Cluster Agent for Cloud Foundry at your service.",
		Long: `
Datadog Cluster Agent for Cloud Foundry takes care of running checks that need to run only
once per cluster.`,
	}

	for _, cmd := range makeCommands() {
		clusterAgentCmd.AddCommand(cmd)
	}

	clusterAgentCmd.PersistentFlags().StringVarP(&cliParams.confPath, "cfgpath", "c", "", "path to directory containing datadog.yaml")
	clusterAgentCmd.PersistentFlags().BoolVarP(&cliParams.flagNoColor, "no-color", "n", false, "disable color output")
	return clusterAgentCmd

}

func makeCommands() []*cobra.Command {
	cliParams := &cliParams{}

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run the Cluster Agent for Cloud Foundry",
		Long:  `Runs Datadog Cluster Agent for Cloud Foundry in the foreground`,
		RunE: func(*cobra.Command, []string) error {
			return runCFClusterAgentFct(cliParams, run)
		},
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version info",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cliParams.flagNoColor {
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

	clusterChecksCmd := commands.GetClusterChecksCobraCmd(&cliParams.flagNoColor, &cliParams.confPath, loggerName)
	configChecksCmd := commands.GetConfigCheckCobraCmd(&cliParams.flagNoColor, &cliParams.confPath, loggerName)
	flareCmd := clusteragentapp.GetFlareCobraCmd(&cliParams.flagNoColor, loggerName)
	return []*cobra.Command{runCmd, versionCmd, clusterChecksCmd, configChecksCmd}
}

func runCFClusterAgentFct(cliParams *cliParams, fct interface{}) error {
	return fxutil.OneShot(fct,
		fx.Supply(core.BundleParams{
			ConfFilePath:      cliParams.confPath,
			ConfigLoadSecrets: true,
			ConfigMissingOK:   true,
			ConfigName:        "datadog-cluster",
		}),
		core.Bundle,
	)
}

func main() {
	flavor.SetFlavor(flavor.ClusterAgent)

	var returnCode int
	if err := MakeRootCommand().Execute(); err != nil {
		log.Error(err)
		returnCode = -1
	}
	log.Flush()
	os.Exit(returnCode)
}

func run(config config.Component) error {
	// Setup logger
	syslogURI := pkgconfig.GetSyslogURI()
	logFile := config.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultDCALogFile
	}
	if config.GetBool("disable_file_logging") {
		// this will prevent any logging on file
		logFile = ""
	}

	mainCtx, mainCtxCancel := context.WithCancel(context.Background())
	defer mainCtxCancel() // Calling cancel twice is safe

	err := pkgconfig.SetupLogger(
		loggerName,
		config.GetString("log_level"),
		logFile,
		syslogURI,
		config.GetBool("syslog_rfc"),
		config.GetBool("log_to_console"),
		config.GetBool("log_format_json"),
	)
	if err != nil {
		log.Criticalf("Unable to setup logger: %s", err)
		return nil
	}

	if !config.IsSet("api_key") {
		log.Critical("no API key configured, exiting")
		return nil
	}

	// Setup healthcheck port
	var healthPort = config.GetInt("health_port")
	if healthPort > 0 {
		err := healthprobe.Serve(mainCtx, healthPort)
		if err != nil {
			return log.Errorf("Error starting health port, exiting: %v", err)
		}
		log.Debugf("Health check listening on port %d", healthPort)
	}

	// get hostname
	hname, err := hostname.Get(context.TODO())
	if err != nil {
		return log.Errorf("Error while getting hostname, exiting: %v", err)
	}
	log.Infof("Hostname is: %s", hname)

	keysPerDomain, err := pkgconfig.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}

	forwarderOpts := forwarder.NewOptionsWithResolvers(resolver.NewSingleDomainResolvers(keysPerDomain))
	opts := aggregator.DefaultAgentDemultiplexerOptions(forwarderOpts)
	opts.UseEventPlatformForwarder = false
	opts.UseOrchestratorForwarder = false
	opts.UseContainerLifecycleForwarder = false
	demux := aggregator.InitAndStartAgentDemultiplexer(opts, hname)
	demux.AddAgentStartupTelemetry(fmt.Sprintf("%s - Datadog Cluster Agent", version.AgentVersion))

	log.Infof("Datadog Cluster Agent is now running.")

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// initialize CC Cache
	if err = app.InitializeCCCache(mainCtx); err != nil {
		_ = log.Errorf("Error initializing Cloud Foundry CCAPI cache, some advanced tagging features may be missing: %v", err)
	}

	// initialize BBS Cache before starting provider/listener
	if err = app.InitializeBBSCache(mainCtx); err != nil {
		return err
	}

	// create and setup the Autoconfig instance
	common.LoadComponents(mainCtx, config.GetString("confd_path"))

	// Set up check collector
	common.AC.AddScheduler("check", collector.InitCheckScheduler(common.Coll), true)
	common.Coll.Start()

	// start the autoconfig, this will immediately run any configured check
	common.AC.LoadAndRun(mainCtx)

	if err = api.StartServer(); err != nil {
		return log.Errorf("Error while starting agent API, exiting: %v", err)
	}

	var clusterCheckHandler *clusterchecks.Handler
	clusterCheckHandler, err = app.SetupClusterCheck(mainCtx)
	if err == nil {
		api.ModifyAPIRouter(func(r *mux.Router) {
			dcav1.InstallChecksEndpoints(r, clusteragent.ServerContext{ClusterCheckHandler: clusterCheckHandler})
		})
	} else {
		log.Errorf("Error while setting up cluster check Autodiscovery %v", err)
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
