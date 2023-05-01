// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	"fmt"
	_ "net/http/pprof"
	"os"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/common/misconfig"
	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/DataDog/datadog-agent/cmd/manager"
	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	"github.com/DataDog/datadog-agent/cmd/process-agent/subcommands"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	logComponent "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/process"
	"github.com/DataDog/datadog-agent/comp/process/apiserver"
	"github.com/DataDog/datadog-agent/comp/process/expvars"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo"
	"github.com/DataDog/datadog-agent/comp/process/profiler"
	runnerComp "github.com/DataDog/datadog-agent/comp/process/runner"
	"github.com/DataDog/datadog-agent/comp/process/types"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/tagger/remote"
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

	if globalParams.PidFilePath != "" {
		err := pidfile.WritePID(globalParams.PidFilePath)
		if err != nil {
			_ = log.Errorf("Error while writing PID file, exiting: %v", err)
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

	err := runApp(exit, globalParams)
	if err != nil {
		cleanupAndExit(1)
	}
}

func runApp(exit chan struct{}, globalParams *command.GlobalParams) error {
	defer log.Flush() // Flush the log in case of an unclean shutdown
	go util.HandleSignals(exit)

	var appInitDeps struct {
		fx.In

		Logger logComponent.Component

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

		// Set `HOST_PROC` and `HOST_SYS` environment variables
		fx.Invoke(command.SetHostMountEnv),

		// Initialize components not manged by fx
		fx.Invoke(initMisc),

		// Invoke the components that we want to start
		fx.Invoke(func(
			runnerComp.Component,
			profiler.Component,
			expvars.Component,
			apiserver.Component,
		) {
		}),
	)

	if err := app.Err(); err != nil {
		// At this point it is not guaranteed that the logger has been successfully initialized. We should fall back to
		// stdout just in case.
		if appInitDeps.Logger == nil {
			fmt.Println("Failed to initialize the process agent: ", fxutil.UnwrapIfErrArgumentsFailed(err))
		} else {
			_ = appInitDeps.Logger.Critical("Failed to initialize the process agent: ", fxutil.UnwrapIfErrArgumentsFailed(err))
		}
		return err
	}

	// Look to see if any checks are enabled, if not, return since the agent doesn't need to be enabled.
	if !anyChecksEnabled(appInitDeps.Checks) {
		log.Infof(agent6DisabledMessage)
		return nil
	}

	err := app.Start(context.Background())
	if err != nil {
		log.Criticalf("Failed to start process agent: %v", err)
		return err
	}

	// Set up an exit channel
	<-exit
	err = app.Stop(context.Background())
	if err != nil {
		log.Criticalf("Failed to properly stop the process agent: %v", err)
	} else {
		log.Info("The process-agent has successfully been shut down")
	}

	return nil
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

type miscDeps struct {
	fx.In
	Lc fx.Lifecycle

	Config   config.Component
	Syscfg   sysprobeconfig.Component
	HostInfo hostinfo.Component
}

// initMisc initializes modules that cannot, or have not yet been componetized.
// Todo: (Components) WorkloadMeta, remoteTagger, statsd
func initMisc(deps miscDeps) error {
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

	deps.Lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			store.Start(ctx)

			err := tagger.Init(ctx)
			if err != nil {
				_ = log.Errorf("failed to start the tagger: %s", err)
			}

			err = manager.ConfigureAutoExit(ctx, deps.Config)
			if err != nil {
				_ = log.Criticalf("Unable to configure auto-exit, err: %w", err)
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

			return nil
		},
	})

	return nil
}
