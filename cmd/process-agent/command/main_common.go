// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package command

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/common/misconfig"
	"github.com/DataDog/datadog-agent/cmd/manager"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	logComponent "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors"
	compstatsd "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/utils"
	"github.com/DataDog/datadog-agent/comp/process"
	"github.com/DataDog/datadog-agent/comp/process/apiserver"
	"github.com/DataDog/datadog-agent/comp/process/expvars"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo"
	"github.com/DataDog/datadog-agent/comp/process/profiler"
	"github.com/DataDog/datadog-agent/comp/process/runner"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/process/metadata/workloadmeta/collector"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	ddutil "github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
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

func runAgent(ctx context.Context, globalParams *GlobalParams) error {
	if globalParams.PidFilePath != "" {
		err := pidfile.WritePID(globalParams.PidFilePath)
		if err != nil {
			log.Errorf("Error while writing PID file, exiting: %v", err)
			return err
		}

		log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), globalParams.PidFilePath)
		defer func() {
			// remove pidfile if set
			_ = os.Remove(globalParams.PidFilePath)
		}()
	}

	// Now that the logger is configured log host info
	log.Infof("running on platform: %s", hostMetadataUtils.GetPlatformName())
	agentVersion, _ := version.Agent()
	log.Infof("running version: %s", agentVersion.GetNumberAndPre())

	// Log any potential misconfigs that are related to the process agent
	misconfig.ToLog(misconfig.ProcessAgent)

	return runApp(ctx, globalParams)
}

func runApp(ctx context.Context, globalParams *GlobalParams) error {
	exitSignal := make(chan struct{})
	defer log.Flush() // Flush the log in case of an unclean shutdown
	go util.HandleSignals(exitSignal)

	var appInitDeps struct {
		fx.In

		Logger logComponent.Component

		Checks       []types.CheckComponent `group:"check"`
		Syscfg       sysprobeconfig.Component
		Config       config.Component
		WorkloadMeta workloadmeta.Component
	}
	app := fx.New(
		fx.Supply(
			core.BundleParams{
				SysprobeConfigParams: sysprobeconfigimpl.NewParams(
					sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath),
				),
				ConfigParams: config.NewAgentParams(globalParams.ConfFilePath),
				SecretParams: secrets.NewEnabledParams(),
				LogParams:    DaemonLogParams,
			},
		),
		// Populate dependencies required for initialization in this function
		fx.Populate(&appInitDeps),

		// Provide core components
		core.Bundle(),

		// Provide process agent bundle so fx knows where to find components
		process.Bundle(),

		// Provide remote config client module
		rcclient.Module(),

		// Provide workloadmeta module
		workloadmeta.Module(),

		// Provide tagger module
		tagger.Module(),

		// Provide statsd client module
		compstatsd.Module(),

		// Provide the corresponding workloadmeta Params to configure the catalog
		collectors.GetCatalog(),
		fx.Provide(func(c config.Component) workloadmeta.Params {
			var catalog workloadmeta.AgentType

			if c.GetBool("process_config.remote_workloadmeta") {
				catalog = workloadmeta.Remote
			} else {
				catalog = workloadmeta.ProcessAgent
			}

			return workloadmeta.Params{AgentType: catalog}
		}),

		// Provide the corresponding tagger Params to configure the tagger
		fx.Provide(func(c config.Component) tagger.Params {
			if c.GetBool("process_config.remote_tagger") {
				return tagger.NewNodeRemoteTaggerParams()
			}
			return tagger.NewTaggerParams()
		}),

		// Allows for debug logging of fx components if the `TRACE_FX` environment variable is set
		fxutil.FxLoggingOption(),

		// Set `HOST_PROC` and `HOST_SYS` environment variables
		fx.Invoke(SetHostMountEnv),

		// Initialize components not manged by fx
		fx.Invoke(initMisc),

		// Invoke the components that we want to start
		fx.Invoke(func(
			runner.Component,
			profiler.Component,
			expvars.Component,
			apiserver.Component,
		) {
		}),

		// Initialize the remote-config client to update the runtime settings
		fx.Invoke(func(rc rcclient.Component) {
			if ddconfig.IsRemoteConfigEnabled(ddconfig.Datadog) {
				if err := rc.Start("process-agent"); err != nil {
					log.Errorf("Couldn't start the remote-config client of the process agent: %s", err)
				}
			}
		}),
	)

	if err := app.Err(); err != nil {
		// At this point it is not guaranteed that the logger has been successfully initialized. We should fall back to
		// stdout just in case.
		if appInitDeps.Logger == nil {
			fmt.Println("Failed to initialize the process agent: ", fxutil.UnwrapIfErrArgumentsFailed(err))
		} else {
			appInitDeps.Logger.Critical("Failed to initialize the process agent: ", fxutil.UnwrapIfErrArgumentsFailed(err))
		}
		return err
	}

	// Look to see if any checks are enabled, if not, return since the agent doesn't need to be enabled.
	if !shouldEnableProcessAgent(appInitDeps.Checks, appInitDeps.Config) {
		log.Infof(agent6DisabledMessage)
		return nil
	}

	err := app.Start(ctx)
	if err != nil {
		log.Criticalf("Failed to start process agent: %v", err)
		return err
	}

	// Wait for exit signal
	select {
	case <-exitSignal:
		log.Info("Received exit signal, shutting down...")
	case <-ctx.Done():
		log.Info("Received stop from service manager, shutting down...")
	}

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

func shouldEnableProcessAgent(checks []types.CheckComponent, cfg ddconfig.Reader) bool {
	return anyChecksEnabled(checks) || collector.Enabled(cfg)
}

type miscDeps struct {
	fx.In
	Lc fx.Lifecycle

	Config       config.Component
	Statsd       compstatsd.Component
	Syscfg       sysprobeconfig.Component
	HostInfo     hostinfo.Component
	WorkloadMeta workloadmeta.Component
}

// initMisc initializes modules that cannot, or have not yet been componetized.
// Todo: (Components) WorkloadMeta, remoteTagger, statsd
// Todo: move metadata/workloadmeta/collector to workloadmeta
func initMisc(deps miscDeps) error {
	if err := statsd.Configure(ddconfig.GetBindHost(), deps.Config.GetInt("dogstatsd_port"), deps.Statsd.CreateForHostPort); err != nil {
		log.Criticalf("Error configuring statsd: %s", err)
		return err
	}

	if err := ddutil.SetupCoreDump(deps.Config); err != nil {
		log.Warnf("Can't setup core dumps: %v, core dumps might not be available after a crash", err)
	}

	processCollectionServer := collector.NewProcessCollector(deps.Config, deps.Syscfg)

	// TODO(components): still unclear how the initialization of workloadmeta
	//                   store and tagger should be performed.
	// appCtx is a context that cancels when the OnStop hook is called
	appCtx, stopApp := context.WithCancel(context.Background())
	deps.Lc.Append(fx.Hook{
		OnStart: func(startCtx context.Context) error {

			err := manager.ConfigureAutoExit(startCtx, deps.Config)
			if err != nil {
				log.Criticalf("Unable to configure auto-exit, err: %w", err)
				return err
			}

			if collector.Enabled(deps.Config) {
				err := processCollectionServer.Start(appCtx, deps.WorkloadMeta)
				if err != nil {
					return err
				}
			}

			return nil
		},
		OnStop: func(ctx context.Context) error {
			stopApp()

			return nil
		},
	})

	return nil
}
