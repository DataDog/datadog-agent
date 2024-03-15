// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package start

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"

	//nolint:revive // TODO(AML) Fix revive linter
	logComponent "github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors"
	"github.com/DataDog/datadog-agent/comp/dogstatsd"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	orchestratorForwarderImpl "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost/inventoryhostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/resources"
	"github.com/DataDog/datadog-agent/comp/metadata/resources/resourcesimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/runner"
	metadatarunnerimpl "github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/api/healthprobe"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type CLIParams struct {
	confPath string
}

type DogstatsdComponents struct {
	DogstatsdServer dogstatsdServer.Component
	DogstatsdStats  *http.Server
	WorkloadMeta    workloadmeta.Component
}

const (
	// loggerName is the name of the dogstatsd logger
	loggerName pkgconfig.LoggerName = "DSD"
)

// MakeCommand returns the start subcommand for the 'dogstatsd' command.
func MakeCommand(defaultLogFile string) *cobra.Command {
	cliParams := &CLIParams{}
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start DogStatsD",
		Long:  `Runs DogStatsD in the foreground`,
		RunE: func(*cobra.Command, []string) error {
			return RunDogstatsdFct(cliParams, "", defaultLogFile, start)
		},
	}

	// local flags
	startCmd.PersistentFlags().StringVarP(&cliParams.confPath, "cfgpath", "c", "", "path to directory containing datadog.yaml")

	var socketPath string
	startCmd.Flags().StringVarP(&socketPath, "socket", "s", "", "listen to this socket instead of UDP")
	pkgconfig.Datadog.BindPFlag("dogstatsd_socket", startCmd.Flags().Lookup("socket")) //nolint:errcheck

	return startCmd
}

type Params struct {
	DefaultLogFile string
}

func RunDogstatsdFct(cliParams *CLIParams, defaultConfPath string, defaultLogFile string, fct interface{}) error {
	params := &Params{
		DefaultLogFile: defaultLogFile,
	}
	return fxutil.OneShot(fct,
		fx.Supply(cliParams),
		fx.Supply(params),
		fx.Supply(config.NewParams(
			defaultConfPath,
			config.WithConfFilePath(cliParams.confPath),
			config.WithConfigMissingOK(true),
			config.WithConfigName("dogstatsd")),
		),
		fx.Supply(secrets.NewEnabledParams()),
		fx.Supply(logComponent.ForDaemon(string(loggerName), "log_file", params.DefaultLogFile)),
		config.Module(),
		logComponent.Module(),
		fx.Supply(dogstatsdServer.Params{
			Serverless: false,
		}),
		dogstatsd.Bundle(),
		forwarder.Bundle(),
		fx.Provide(defaultforwarder.NewParams),
		// workloadmeta setup
		collectors.GetCatalog(),
		fx.Provide(func(config config.Component) workloadmeta.Params {
			catalog := workloadmeta.NodeAgent
			instantiate := config.GetBool("dogstatsd_origin_detection")

			return workloadmeta.Params{
				AgentType:  catalog,
				InitHelper: common.GetWorkloadmetaInit(),
				NoInstance: !instantiate,
			}
		}),
		workloadmeta.OptionalModule(),
		demultiplexerimpl.Module(),
		secretsimpl.Module(),
		orchestratorForwarderImpl.Module(),
		fx.Supply(orchestratorForwarderImpl.NewDisabledParams()),
		// injecting the shared Serializer to FX until we migrate it to a prpoper component. This allows other
		// already migrated components to request it.
		fx.Provide(func(demuxInstance demultiplexer.Component) serializer.MetricSerializer {
			return demuxInstance.Serializer()
		}),
		fx.Provide(func(config config.Component) demultiplexerimpl.Params {
			params := demultiplexerimpl.NewDefaultParams()
			params.UseEventPlatformForwarder = false
			params.EnableNoAggregationPipeline = config.GetBool("dogstatsd_no_aggregation_pipeline")
			params.ContinueOnMissingHostname = true
			return params
		}),
		fx.Supply(resourcesimpl.Disabled()),
		metadatarunnerimpl.Module(),
		resourcesimpl.Module(),
		hostimpl.Module(),
		inventoryagent.Module(),
		// sysprobeconfig is optionally required by inventoryagent
		sysprobeconfig.NoneModule(),
		inventoryhostimpl.Module(),
	)
}

func start(
	cliParams *CLIParams,
	config config.Component,
	log log.Component,
	params *Params,
	server dogstatsdServer.Component,
	_ defaultforwarder.Component,
	wmeta optional.Option[workloadmeta.Component],
	demultiplexer demultiplexer.Component,
	_ runner.Component,
	_ resources.Component,
	_ host.Component,
	_ inventoryagent.Component,
	_ inventoryhost.Component,
) error {
	// Main context passed to components
	ctx, cancel := context.WithCancel(context.Background())

	w, _ := wmeta.Get()
	components := &DogstatsdComponents{
		DogstatsdServer: server,
		WorkloadMeta:    w,
	}
	defer StopAgent(cancel, components)

	stopCh := make(chan struct{})
	go handleSignals(stopCh)

	err := RunDogstatsd(ctx, cliParams, config, log, params, components, demultiplexer)
	if err != nil {
		return err
	}

	// Block here until we receive a stop signal
	<-stopCh

	return nil
}

// RunDogstatsd starts the dogstatsd server
func RunDogstatsd(ctx context.Context, cliParams *CLIParams, config config.Component, log log.Component, params *Params, components *DogstatsdComponents, demultiplexer demultiplexer.Component) (err error) {
	if len(cliParams.confPath) == 0 {
		log.Infof("Config will be read from env variables")
	}

	// go_expvar server
	port := config.GetInt("dogstatsd_stats_port")
	components.DogstatsdStats = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: http.DefaultServeMux,
	}
	go func() {
		if err := components.DogstatsdStats.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("Error creating dogstatsd stats server on port %d: %s", port, err)
		}
	}()

	// Setup logger
	syslogURI := pkgconfig.GetSyslogURI()
	logFile := config.GetString("log_file")
	if logFile == "" {
		logFile = params.DefaultLogFile
	}

	if config.GetBool("disable_file_logging") {
		// this will prevent any logging on file
		logFile = ""
	}

	err = pkgconfig.SetupLogger(
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
		return
	}

	if err := util.SetupCoreDump(config); err != nil {
		log.Warnf("Can't setup core dumps: %v, core dumps might not be available after a crash", err)
	}

	if !config.IsSet("api_key") {
		err = log.Critical("no API key configured, exiting")
		return
	}

	// Setup healthcheck port
	var healthPort = config.GetInt("health_port")
	if healthPort > 0 {
		err = healthprobe.Serve(ctx, healthPort)
		if err != nil {
			err = log.Errorf("Error starting health port, exiting: %v", err)
			return
		}
		log.Debugf("Health check listening on port %d", healthPort)
	}

	demultiplexer.AddAgentStartupTelemetry(version.AgentVersion)

	// container tagging initialisation if origin detection is on
	if config.GetBool("dogstatsd_origin_detection") && components.WorkloadMeta != nil {

		tagger.SetDefaultTagger(local.NewTagger(components.WorkloadMeta))
		if err := tagger.Init(ctx); err != nil {
			log.Errorf("failed to start the tagger: %s", err)
		}
	}

	err = components.DogstatsdServer.Start(demultiplexer)
	if err != nil {
		log.Criticalf("Unable to start dogstatsd: %s", err)
		return
	}
	return
}

// handleSignals handles OS signals, and sends a message on stopCh when an interrupt
// signal is received.
func handleSignals(stopCh chan struct{}) {
	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGPIPE)

	// Block here until we receive the interrupt signal
	for signo := range signalCh {
		switch signo {
		case syscall.SIGPIPE:
			// By default systemd redirects the stdout to journald. When journald is stopped or crashes we receive a SIGPIPE signal.
			// Go ignores SIGPIPE signals unless it is when stdout or stdout is closed, in this case the agent is stopped.
			// We never want dogstatsd to stop upon receiving SIGPIPE, so we intercept the SIGPIPE signals and just discard them.
		default:
			pkglog.Infof("Received signal '%s', shutting down...", signo)
			stopCh <- struct{}{}
			return
		}
	}
}

func StopAgent(cancel context.CancelFunc, components *DogstatsdComponents) {
	// retrieve the agent health before stopping the components
	// GetReadyNonBlocking has a 100ms timeout to avoid blocking
	health, err := health.GetReadyNonBlocking()
	if err != nil {
		pkglog.Warnf("Dogstatsd health unknown: %s", err)
	} else if len(health.Unhealthy) > 0 {
		pkglog.Warnf("Some components were unhealthy: %v", health.Unhealthy)
	}

	// gracefully shut down any component
	cancel()

	if components.DogstatsdStats != nil {
		if err := components.DogstatsdStats.Shutdown(context.Background()); err != nil {
			pkglog.Errorf("Error shutting down dogstatsd stats server: %s", err)
		}
	}

	components.DogstatsdServer.Stop()

	pkglog.Info("See ya!")
	pkglog.Flush()
}
