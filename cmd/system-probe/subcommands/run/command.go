// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/cmd/manager"
	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/cmd/system-probe/common"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	ddruntime "github.com/DataDog/datadog-agent/pkg/runtime"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// ErrNotEnabled represents the case in which system-probe is not enabled
var ErrNotEnabled = errors.New("system-probe not enabled")

type cliParams struct {
	*command.GlobalParams

	// pidfilePath contains the value of the --pidfile flag.
	pidfilePath string
}

// Commands returns a slice of subcommands for the 'system-probe' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run the System Probe",
		Long:  `Runs the system-probe in the foreground`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(run,
				fx.Supply(cliParams),
				fx.Supply(config.NewAgentParamsWithoutSecrets("", config.WithConfigMissingOK(true))),
				fx.Supply(sysprobeconfig.NewParams(sysprobeconfig.WithSysProbeConfFilePath(globalParams.ConfFilePath))),
				fx.Supply(log.LogForDaemon("SYS-PROBE", "log_file", common.DefaultLogFile)),
				config.Module,
				sysprobeconfig.Module,
				// use system-probe config instead of agent config for logging
				fx.Provide(func(lc fx.Lifecycle, params log.Params, sysprobeconfig sysprobeconfig.Component) (log.Component, error) {
					return log.NewLogger(lc, params, sysprobeconfig)
				}),
			)
		},
	}
	runCmd.Flags().StringVarP(&cliParams.pidfilePath, "pid", "p", "", "path to the pidfile")

	return []*cobra.Command{runCmd}
}

// run starts the main loop.
func run(log log.Component, config config.Component, sysprobeconfig sysprobeconfig.Component, cliParams *cliParams) error {
	defer func() {
		stopSystemProbe(cliParams)
	}()

	// prepare go runtime
	ddruntime.SetMaxProcs()
	if err := ddruntime.SetGoMemLimit(ddconfig.IsContainerized()); err != nil {
		log.Debugf("Couldn't set Go memory limit: %s", err)
	}

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Make a channel to exit the function
	stopCh := make(chan error)

	go func() {
		// Set up the signals async, so we can start the system-probe
		select {
		case <-signals.Stopper:
			log.Info("Received stop command, shutting down...")
			stopCh <- nil
		case <-signals.ErrorStopper:
			_ = log.Critical("system-probe has encountered an error, shutting down...")
			stopCh <- fmt.Errorf("shutting down because of an error")
		case sig := <-signalCh:
			log.Infof("Received signal '%s', shutting down...", sig)
			stopCh <- nil
		}
	}()

	// By default, systemd redirects the stdout to journald. When journald is stopped or crashes we receive a SIGPIPE signal.
	// Go ignores SIGPIPE signals unless it is when stdout or stdout is closed, in this case the agent is stopped.
	// We never want the agent to stop upon receiving SIGPIPE, so we intercept the SIGPIPE signals and just discard them.
	sigpipeCh := make(chan os.Signal, 1)
	signal.Notify(sigpipeCh, syscall.SIGPIPE)
	go func() {
		for range sigpipeCh {
			// do nothing
		}
	}()

	if err := startSystemProbe(cliParams, log, sysprobeconfig); err != nil {
		if err == ErrNotEnabled {
			// A sleep is necessary to ensure that supervisor registers this process as "STARTED"
			// If the exit is "too quick", we enter a BACKOFF->FATAL loop even though this is an expected exit
			// http://supervisord.org/subprocess.html#process-states
			time.Sleep(5 * time.Second)
			return nil
		}
		return err
	}
	return <-stopCh
}

// StartSystemProbeWithDefaults is a temporary way for other packages to use startSystemProbe.
func StartSystemProbeWithDefaults() error {
	// run startSystemProbe in an app, so that the log and config components get initialized
	return fxutil.OneShot(
		func(log log.Component, config config.Component, sysprobeconfig sysprobeconfig.Component) error {
			return startSystemProbe(&cliParams{GlobalParams: &command.GlobalParams{}}, log, sysprobeconfig)
		},
		// no config file path specification in this situation
		fx.Supply(config.NewAgentParamsWithoutSecrets("", config.WithConfigMissingOK(true))),
		fx.Supply(sysprobeconfig.NewParams(sysprobeconfig.WithSysProbeConfFilePath(""))),
		fx.Supply(log.LogForDaemon("SYS-PROBE", "log_file", common.DefaultLogFile)),
		config.Module,
		sysprobeconfig.Module,
		// use system-probe config instead of agent config for logging
		fx.Provide(func(lc fx.Lifecycle, params log.Params, sysprobeconfig sysprobeconfig.Component) (log.Component, error) {
			return log.NewLogger(lc, params, sysprobeconfig)
		}),
	)
}

// startSystemProbe Initializes the system-probe process
func startSystemProbe(cliParams *cliParams, log log.Component, sysprobeconfig sysprobeconfig.Component) error {
	var err error
	var ctx context.Context
	ctx, common.MainCtxCancel = context.WithCancel(context.Background())
	cfg := sysprobeconfig.Object()

	log.Infof("starting system-probe v%v", version.AgentVersion)

	// Exit if system probe is disabled
	if cfg.ExternalSystemProbe || !cfg.Enabled {
		log.Info("system probe not enabled. exiting")
		return ErrNotEnabled
	}

	if err := util.SetupCoreDump(sysprobeconfig); err != nil {
		log.Warnf("cannot setup core dumps: %s, core dumps might not be available after a crash", err)
	}

	if err := initRuntimeSettings(); err != nil {
		log.Warnf("cannot initialize the runtime settings: %s", err)
	}

	setupInternalProfiling(sysprobeconfig, configPrefix, log)

	if cliParams.pidfilePath != "" {
		if err := pidfile.WritePID(cliParams.pidfilePath); err != nil {
			return log.Errorf("error while writing PID file, exiting: %s", err)
		}
		log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), cliParams.pidfilePath)
	}

	err = manager.ConfigureAutoExit(ctx, sysprobeconfig)
	if err != nil {
		return log.Criticalf("unable to configure auto-exit: %s", err)
	}

	if err := statsd.Configure(cfg.StatsdHost, cfg.StatsdPort); err != nil {
		return log.Criticalf("error configuring statsd: %s", err)
	}

	if isValidPort(cfg.DebugPort) {
		if cfg.TelemetryEnabled {
			http.Handle("/telemetry", telemetry.Handler())
		}
		go func() {
			common.ExpvarServer = &http.Server{
				Addr:    fmt.Sprintf("127.0.0.1:%d", cfg.DebugPort),
				Handler: http.DefaultServeMux,
			}
			if err := common.ExpvarServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Errorf("error creating expvar server on %v: %v", common.ExpvarServer.Addr, err)
			}
		}()
	}

	if err = api.StartServer(cfg); err != nil {
		return log.Criticalf("error while starting api server, exiting: %v", err)
	}
	return nil
}

// StopSystemProbeWithDefaults is a temporary way for other packages to use stopAgent.
func StopSystemProbeWithDefaults() {
	stopSystemProbe(&cliParams{GlobalParams: &command.GlobalParams{}})
}

// stopSystemProbe Tears down the system-probe process
func stopSystemProbe(cliParams *cliParams) {
	module.Close()
	if common.ExpvarServer != nil {
		if err := common.ExpvarServer.Shutdown(context.Background()); err != nil {
			pkglog.Errorf("error shutting down expvar server: %s", err)
		}
	}
	profiling.Stop()

	_ = os.Remove(cliParams.pidfilePath)

	// gracefully shut down any component
	common.MainCtxCancel()
	pkglog.Flush()
}

// setupInternalProfiling is a common helper to configure runtime settings for internal profiling.
func setupInternalProfiling(cfg ddconfig.ConfigReader, configPrefix string, log log.Component) {
	if v := cfg.GetInt(configPrefix + "internal_profiling.block_profile_rate"); v > 0 {
		if err := settings.SetRuntimeSetting("runtime_block_profile_rate", v); err != nil {
			log.Errorf("Error setting block profile rate: %v", err)
		}
	}

	if v := cfg.GetInt(configPrefix + "internal_profiling.mutex_profile_fraction"); v > 0 {
		if err := settings.SetRuntimeSetting("runtime_mutex_profile_fraction", v); err != nil {
			log.Errorf("Error mutex profile fraction: %v", err)
		}
	}

	if cfg.GetBool(configPrefix + "internal_profiling.enabled") {
		err := settings.SetRuntimeSetting("internal_profiling", true)
		if err != nil {
			log.Errorf("Error starting profiler: %v", err)
		}
	}
}

func isValidPort(port int) bool {
	return port > 0 && port < 65536
}
