// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/common/misconfig"
	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/DataDog/datadog-agent/cmd/manager"
	"github.com/DataDog/datadog-agent/cmd/process-agent/api"
	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	"github.com/DataDog/datadog-agent/cmd/process-agent/subcommands"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/process"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo"
	"github.com/DataDog/datadog-agent/comp/process/profiler"
	runnerComp "github.com/DataDog/datadog-agent/comp/process/runner"
	"github.com/DataDog/datadog-agent/comp/process/types"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/runner"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/process/status"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/tagger/remote"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	ddutil "github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	// register all workloadmeta collectors
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors"
)

const (
	agent6DisabledMessage = `process-agent not enabled.
Set env var DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED=true or add
process_config:
  process_collection:
    enabled: true
to your datadog.yaml file.
Exiting.`
)

// main is the main application entry point
func main() {
	os.Args = command.FixDeprecatedFlags(os.Args, os.Stdout)

	rootCmd := command.MakeCommand(subcommands.ProcessAgentSubcommands(), useWinParams, rootCmdRun)
	os.Exit(runcmd.Run(rootCmd))
}

func runAgent(globalParams *command.GlobalParams, exit chan struct{}) {
	cleanupAndExit := cleanupAndExitHandler(globalParams)

	if !globalParams.Info && globalParams.PidFilePath != "" {
		err := pidfile.WritePID(globalParams.PidFilePath)
		if err != nil {
			log.Errorf("Error while writing PID file, exiting: %v", err)
			cleanupAndExit(1)
		}

		log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), globalParams.PidFilePath)
		defer func() {
			// remove pidfile if set
			_ = os.Remove(globalParams.PidFilePath)
		}()
	}

	// Now that the logger is configured log host info
	hostStatus := host.GetStatusInformation()
	log.Infof("running on platform: %s", hostStatus.Platform)
	agentVersion, _ := version.Agent()
	log.Infof("running version: %s", agentVersion.GetNumberAndPre())

	// Log any potential misconfigs that are related to the process agent
	misconfig.ToLog(misconfig.ProcessAgent)

	exitCode := runApp(exit, globalParams)
	cleanupAndExit(exitCode)
}

func runApp(exit chan struct{}, globalParams *command.GlobalParams) int {
	go util.HandleSignals(exit)

	var appInitDeps struct {
		fx.In

		Checks []types.CheckComponent `group:"check"`
		Syscfg sysprobeconfig.Component
		Config config.Component
	}
	app := fx.New(
		fx.Supply(
			core.BundleParams{
				SysprobeConfigParams: sysprobeconfig.NewParams(
					sysprobeconfig.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath),
				),
				ConfigParams: config.NewAgentParamsWithSecrets(globalParams.ConfFilePath),
				LogParams:    command.DaemonLogParams,
			},
		),
		// Populate dependencies required for initialization in this function.
		fx.Populate(&appInitDeps),

		// Provide process agent bundle so fx knows where to find components.
		process.Bundle,

		// Allows for debug logging of fx components if the `TRACE_FX` environment variable is set
		fxutil.FxLoggingOption(),

		// Initialize components not manged by fx
		fx.Invoke(initMisc),

		// Invoke the components that we want to start
		fx.Invoke(func(runnerComp.Component, profiler.Component) {}),
	)

	if globalParams.Info {
		// using the debug port to get info to work
		url := fmt.Sprintf("http://localhost:%d/debug/vars", getExpvarPort(appInitDeps.Config))
		if err := status.Info(os.Stdout, url); err != nil {
			_ = log.Criticalf("Failed to render info:", err.Error())
			return 1
		}
		return 0
	}

	// Look to see if any checks are enabled, if not, return since the agent doesn't need to be enabled.
	if !anyChecksEnabled(appInitDeps.Checks) {
		log.Infof(agent6DisabledMessage)

		// a sleep is necessary to ensure that supervisor registers this process as "STARTED"
		// If the exit is "too quick", we enter a BACKOFF->FATAL loop even though this is an expected exit
		// http://supervisord.org/subprocess.html#process-states
		time.Sleep(5 * time.Second)
		return 0
	}

	err := app.Start(context.Background())
	if err != nil {
		log.Criticalf("Failed to start process agent: %v", err)
		return 1
	}

	// Set up an exit channel
	<-exit
	err = app.Stop(context.Background())
	if err != nil {
		log.Criticalf("Failed to properly stop the process agent: %v", err)
	} else {
		log.Info("The process-agent has successfully been shut down")
	}

	return 0
}

func anyChecksEnabled(checks []types.CheckComponent) bool {
	for _, check := range checks {
		if check.Object().IsEnabled() {
			return true
		}
	}
	return false
}

// cleanupAndExitHandler cleans all resources allocated by the agent before calling os.Exit
func cleanupAndExitHandler(globalParams *command.GlobalParams) func(int) {
	return func(status int) {
		// remove pidfile if set
		if globalParams.PidFilePath != "" {
			if _, err := os.Stat(globalParams.PidFilePath); err == nil {
				os.Remove(globalParams.PidFilePath)
			}
		}

		os.Exit(status)
	}
}

// initRuntimeSettings registers settings to be added to the runtime config.
func initRuntimeSettings() {
	// NOTE: Any settings you want to register should simply be added here
	processRuntimeSettings := []settings.RuntimeSetting{
		settings.LogLevelRuntimeSetting{},
		settings.RuntimeMutexProfileFraction{},
		settings.RuntimeBlockProfileRate{},
		settings.ProfilingGoroutines{},
		settings.ProfilingRuntimeSetting{SettingName: "internal_profiling", Service: "process-agent"},
	}

	// Before we begin listening, register runtime settings
	for _, setting := range processRuntimeSettings {
		err := settings.RegisterRuntimeSetting(setting)
		if err != nil {
			_ = log.Warnf("Cannot initialize the runtime setting %s: %v", setting.Name(), err)
		}
	}
}

type miscDeps struct {
	fx.In
	Lc fx.Lifecycle

	Config   config.Component
	Syscfg   sysprobeconfig.Component
	HostInfo hostinfo.Component
}

// initMisc initializes modules that cannot, or have not yet been componetized.
// Todo: (Components) WorkloadMeta, remoteTagger, telemetry Server, expvars, api server
func initMisc(deps miscDeps) error {
	initRuntimeSettings()

	if err := statsd.Configure(ddconfig.GetBindHost(), deps.Config.GetInt("dogstatsd_port")); err != nil {
		_ = log.Criticalf("Error configuring statsd: %s", err)
		return err
	}

	if err := ddutil.SetupCoreDump(deps.Config); err != nil {
		_ = log.Warnf("Can't setup core dumps: %v, core dumps might not be available after a crash", err)
	}

	// Setup workloadmeta
	var workloadmetaCollectors workloadmeta.CollectorCatalog
	if deps.Config.GetBool("process_config.remote_workloadmeta") {
		workloadmetaCollectors = workloadmeta.RemoteCatalog
	} else {
		workloadmetaCollectors = workloadmeta.NodeAgentCatalog
	}
	store := workloadmeta.CreateGlobalStore(workloadmetaCollectors)

	// Setup remote tagger
	var t tagger.Tagger
	if deps.Config.GetBool("process_config.remote_tagger") {
		options, err := remote.NodeAgentOptions()
		if err != nil {
			_ = log.Errorf("unable to deps.Configure the remote tagger: %s", err)
		} else {
			t = remote.NewTagger(options)
		}
	} else {
		t = local.NewTagger(store)
	}
	tagger.SetDefaultTagger(t)

	// Run a profile & telemetry server.
	if deps.Config.GetBool("telemetry.enabled") {
		http.Handle("/telemetry", telemetry.Handler())
	}

	expvarPort := getExpvarPort(deps.Config)
	expvarServer := &http.Server{Addr: fmt.Sprintf("localhost:%d", expvarPort), Handler: http.DefaultServeMux}

	// Initialize status
	err := initStatus(deps.HostInfo.Object(), deps.Syscfg)
	if err != nil {
		log.Critical("Failed to initialize status:", err)
	}

	deps.Lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			store.Start(ctx)

			err := tagger.Init(ctx)
			if err != nil {
				_ = log.Errorf("failed to start the tagger: %s", err)
			}

			go func() {
				_ = expvarServer.ListenAndServe()
			}()

			// Run API server
			err = api.StartServer()
			if err != nil {
				_ = log.Error(err)
			}

			err = manager.ConfigureAutoExit(ctx, deps.Config)
			if err != nil {
				_ = log.Criticalf("Unable to deps.Configure auto-exit, err: %w", err)
				return err
			}

			return nil
		},
		OnStop: func(ctx context.Context) error {
			// Stop the remote tagger
			err := tagger.Stop()
			if err != nil {
				return err
			}

			if err := expvarServer.Shutdown(ctx); err != nil {
				log.Errorf("Error shutting down expvar server on port %v: %v", getExpvarPort(deps.Config), err)
			}

			return nil
		},
	})

	return nil
}

func initStatus(hostInfo *checks.HostInfo, syscfg sysprobeconfig.Component) error {
	// update docker socket path in info
	dockerSock, err := util.GetDockerSocketPath()
	if err != nil {
		log.Debugf("Docker is not available on this host")
	}
	status.UpdateDockerSocket(dockerSock)

	// If the sysprobe module is enabled, the process check can call out to the sysprobe for privileged stats
	_, processModuleEnabled := syscfg.Object().EnabledModules[sysconfig.ProcessModule]
	eps, err := runner.GetAPIEndpoints()
	if err != nil {
		log.Criticalf("Failed to initialize Api Endpoints: %s", err.Error())
	}
	err = status.InitInfo(hostInfo.HostName, processModuleEnabled, eps)
	if err != nil {
		_ = log.Criticalf("Error initializing info: %s", err)
	}
	return nil
}

func getExpvarPort(config ddconfig.ConfigReader) int {
	expVarPort := config.GetInt("process_config.expvar_port")
	if expVarPort <= 0 {
		log.Warnf("Invalid process_config.expvar_port -- %d, using default port %d", expVarPort, ddconfig.DefaultProcessExpVarPort)
		expVarPort = ddconfig.DefaultProcessExpVarPort
	}
	return expVarPort
}
