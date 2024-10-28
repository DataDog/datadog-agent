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
	authtokenimpl "github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	healthprobe "github.com/DataDog/datadog-agent/comp/core/healthprobe/def"
	healthprobefx "github.com/DataDog/datadog-agent/comp/core/healthprobe/fx"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logfx "github.com/DataDog/datadog-agent/comp/core/log/fx"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/secrets/secretsimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog-less"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/dogstatsd"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	orchestratorForwarderImpl "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/inventoryagentimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost/inventoryhostimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/resources"
	"github.com/DataDog/datadog-agent/comp/metadata/resources/resourcesimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/runner"
	metadatarunnerimpl "github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type CLIParams struct {
	confPath   string
	socketPath string
}

type DogstatsdComponents struct {
	DogstatsdServer dogstatsdServer.Component
	DogstatsdStats  *http.Server
	WorkloadMeta    workloadmeta.Component
}

const (
	// loggerName is the name of the dogstatsd logger
	loggerName pkglogsetup.LoggerName = "DSD"
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
	startCmd.PersistentFlags().StringVarP(&cliParams.socketPath, "socket", "s", "", "listen to this socket instead of UDP")

	return startCmd
}

type Params struct {
	DefaultLogFile string
}

func RunDogstatsdFct(cliParams *CLIParams, defaultConfPath string, defaultLogFile string, fct interface{}) error {
	params := &Params{
		DefaultLogFile: defaultLogFile,
	}

	configOptions := []func(*config.Params){
		config.WithConfFilePath(cliParams.confPath),
		config.WithConfigMissingOK(true),
		config.WithConfigName("dogstatsd"),
	}
	if cliParams.socketPath != "" {
		configOptions = append(configOptions, config.WithCLIOverride("dogstatsd_socket", cliParams.socketPath))
	}

	return fxutil.OneShot(fct,
		fx.Supply(cliParams),
		fx.Supply(params),
		fx.Supply(config.NewParams(
			defaultConfPath,
			configOptions...,
		)),
		fx.Provide(func(comp secrets.Component) optional.Option[secrets.Component] {
			return optional.NewOption[secrets.Component](comp)
		}),
		fx.Supply(secrets.NewEnabledParams()),
		telemetryimpl.Module(),
		fx.Supply(log.ForDaemon(string(loggerName), "log_file", params.DefaultLogFile)),
		config.Module(),
		logfx.Module(),
		dogstatsd.Bundle(dogstatsdServer.Params{Serverless: false}),
		forwarder.Bundle(defaultforwarder.NewParams()),
		// workloadmeta setup
		wmcatalog.GetCatalog(),
		workloadmetafx.Module(workloadmeta.Params{
			AgentType:  workloadmeta.NodeAgent,
			InitHelper: common.GetWorkloadmetaInit(),
		}),
		compressionimpl.Module(),
		demultiplexerimpl.Module(demultiplexerimpl.NewDefaultParams(
			demultiplexerimpl.WithContinueOnMissingHostname(),
			demultiplexerimpl.WithDogstatsdNoAggregationPipelineConfig(),
		)),
		secretsimpl.Module(),
		orchestratorForwarderImpl.Module(orchestratorForwarderImpl.NewDisabledParams()),
		eventplatformimpl.Module(eventplatformimpl.NewDisabledParams()),
		eventplatformreceiverimpl.Module(),
		hostnameimpl.Module(),
		taggerimpl.Module(),
		fx.Provide(tagger.NewTaggerParams),
		// injecting the shared Serializer to FX until we migrate it to a prpoper component. This allows other
		// already migrated components to request it.
		fx.Provide(func(demuxInstance demultiplexer.Component) serializer.MetricSerializer {
			return demuxInstance.Serializer()
		}),
		fx.Supply(resourcesimpl.Disabled()),
		metadatarunnerimpl.Module(),
		resourcesimpl.Module(),
		hostimpl.Module(),
		inventoryagentimpl.Module(),
		authtokenimpl.Module(),
		// sysprobeconfig is optionally required by inventoryagent
		sysprobeconfig.NoneModule(),
		inventoryhostimpl.Module(),
		fx.Provide(func(config config.Component) healthprobe.Options {
			return healthprobe.Options{
				Port:           config.GetInt("health_port"),
				LogsGoroutines: config.GetBool("log_all_goroutines_when_unhealthy"),
			}
		}),
		healthprobefx.Module(),
	)
}

func start(
	cliParams *CLIParams,
	config config.Component,
	log log.Component,
	params *Params,
	server dogstatsdServer.Component,
	_ defaultforwarder.Component,
	wmeta workloadmeta.Component,
	_ tagger.Component,
	demultiplexer demultiplexer.Component,
	_ runner.Component,
	_ resources.Component,
	_ host.Component,
	_ inventoryagent.Component,
	_ inventoryhost.Component,
	_ healthprobe.Component,
) error {
	// Main context passed to components
	ctx, cancel := context.WithCancel(context.Background())

	components := &DogstatsdComponents{
		DogstatsdServer: server,
		WorkloadMeta:    wmeta,
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
func RunDogstatsd(_ context.Context, cliParams *CLIParams, config config.Component, log log.Component, params *Params, components *DogstatsdComponents, demultiplexer demultiplexer.Component) (err error) {
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
	syslogURI := pkglogsetup.GetSyslogURI(pkgconfigsetup.Datadog())
	logFile := config.GetString("log_file")
	if logFile == "" {
		logFile = params.DefaultLogFile
	}

	if config.GetBool("disable_file_logging") {
		// this will prevent any logging on file
		logFile = ""
	}

	err = pkglogsetup.SetupLogger(
		loggerName,
		config.GetString("log_level"),
		logFile,
		syslogURI,
		config.GetBool("syslog_rfc"),
		config.GetBool("log_to_console"),
		config.GetBool("log_format_json"),
		pkgconfigsetup.Datadog(),
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

	demultiplexer.AddAgentStartupTelemetry(version.AgentVersion)

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

	pkglog.Info("See ya!")
	pkglog.Flush()
}
