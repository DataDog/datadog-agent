// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package command

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/common/misconfig"
	"github.com/DataDog/datadog-agent/comp/agent/autoexit"
	"github.com/DataDog/datadog-agent/comp/agent/autoexit/autoexitimpl"
	"github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/configsync"
	"github.com/DataDog/datadog-agent/comp/core/configsync/configsyncimpl"
	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/pid"
	"github.com/DataDog/datadog-agent/comp/core/pid/pidimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/status"
	coreStatusImpl "github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	compstatsd "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	hostMetadataUtils "github.com/DataDog/datadog-agent/comp/metadata/host/hostimpl/utils"
	"github.com/DataDog/datadog-agent/comp/networkpath"
	"github.com/DataDog/datadog-agent/comp/process"
	"github.com/DataDog/datadog-agent/comp/process/agent"
	"github.com/DataDog/datadog-agent/comp/process/apiserver"
	"github.com/DataDog/datadog-agent/comp/process/expvars"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo"
	"github.com/DataDog/datadog-agent/comp/process/profiler"
	"github.com/DataDog/datadog-agent/comp/process/status/statusimpl"
	"github.com/DataDog/datadog-agent/comp/process/types"
	rdnsquerierfx "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx"
	remoteconfig "github.com/DataDog/datadog-agent/comp/remote-config"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/process/metadata/workloadmeta/collector"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	ddutil "github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil/logging"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// errAgentDisabled indicates that the process-agent wasn't enabled through environment variable or config.
var errAgentDisabled = errors.New("process-agent not enabled")

func runAgent(ctx context.Context, globalParams *GlobalParams) error {
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

		Logger logcomp.Component

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
					sysprobeconfigimpl.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath),
				),
				ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
				SecretParams: secrets.NewEnabledParams(),
				LogParams:    DaemonLogParams,
			},
		),
		fx.Supply(
			status.Params{
				PythonVersionGetFunc: python.GetPythonVersion,
			},
		),
		fx.Supply(
			// Provide remote config client configuration
			rcclient.Params{
				AgentName:    "process-agent",
				AgentVersion: version.AgentVersion,
			},
		),

		// Populate dependencies required for initialization in this function
		fx.Populate(&appInitDeps),

		// Provide core components
		core.Bundle(),

		// Provide process agent bundle so fx knows where to find components
		process.Bundle(),

		eventplatformreceiverimpl.Module(),
		eventplatformimpl.Module(eventplatformimpl.NewDefaultParams()),

		// Provides the rdnssquerier module
		rdnsquerierfx.Module(),

		// Provide network path bundle
		networkpath.Bundle(),

		// Provide remote config client bundle
		remoteconfig.Bundle(),

		// Provide tagger module
		taggerimpl.Module(),

		// Provide status modules
		statusimpl.Module(),
		coreStatusImpl.Module(),

		// Provide statsd client module
		compstatsd.Module(),

		// Provide authtoken module
		fetchonlyimpl.Module(),

		// Provide configsync module
		configsyncimpl.OptionalModule(),

		// Provide autoexit module
		autoexitimpl.Module(),

		// Provide the corresponding workloadmeta Params to configure the catalog
		wmcatalog.GetCatalog(),

		// Provide workloadmeta module
		workloadmetafx.ModuleWithProvider(func(c config.Component) workloadmeta.Params {
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
			if c.GetBool("process_config.remote_tagger") ||
				// If the agent is running in ECS or ECS Fargate and the ECS task collection is enabled, use the remote tagger
				// as remote tagger can return more tags than the local tagger.
				((env.IsECS() || env.IsECSFargate()) && c.GetBool("ecs_task_collection_enabled")) {
				return tagger.NewNodeRemoteTaggerParams()
			}
			return tagger.NewTaggerParams()
		}),

		// Provides specific features to our own fx wrapper (logging, lifecycle, shutdowner)
		fxutil.FxAgentBase(),
		logging.EnableFxLoggingOnDebug[logcomp.Component](),

		// Set the pid file path
		fx.Supply(pidimpl.NewParams(globalParams.PidFilePath)),

		// Set `HOST_PROC` and `HOST_SYS` environment variables
		fx.Invoke(SetHostMountEnv),

		// Initialize components not manged by fx
		fx.Invoke(initMisc),

		// Invoke the components that we want to start
		fx.Invoke(func(
			_ profiler.Component,
			_ expvars.Component,
			_ apiserver.Component,
			cfg config.Component,
			_ optional.Option[configsync.Component],
			// TODO: This is needed by the container-provider which is not currently a component.
			// We should ensure the tagger is a dependency when converting to a component.
			_ tagger.Component,
			_ pid.Component,
			processAgent agent.Component,
			_ autoexit.Component,
		) error {
			if !processAgent.Enabled() && !collector.Enabled(cfg) {
				return errAgentDisabled
			}
			return nil
		}),
		fx.Provide(func(c config.Component) settings.Params {
			return settings.Params{
				Settings: map[string]settings.RuntimeSetting{
					"log_level":                      commonsettings.NewLogLevelRuntimeSetting(),
					"runtime_mutex_profile_fraction": commonsettings.NewRuntimeMutexProfileFraction(),
					"runtime_block_profile_rate":     commonsettings.NewRuntimeBlockProfileRate(),
					"internal_profiling_goroutines":  commonsettings.NewProfilingGoroutines(),
					"internal_profiling":             commonsettings.NewProfilingRuntimeSetting("internal_profiling", "process-agent"),
				},
				Config: c,
			}
		}),
		settingsimpl.Module(),
	)

	err := app.Start(ctx)
	if err != nil {
		if errors.Is(err, errAgentDisabled) {
			if !shouldStayAlive(appInitDeps.Config) {
				log.Info("process-agent is not enabled, exiting...")
				return nil
			}
		} else {
			// At this point it is not guaranteed that the logger has been successfully initialized. We should fall back to
			// stdout just in case.
			if appInitDeps.Logger == nil {
				fmt.Println("Failed to initialize the process agent: ", fxutil.UnwrapIfErrArgumentsFailed(err))
			} else {
				appInitDeps.Logger.Critical("Failed to initialize the process agent: ", fxutil.UnwrapIfErrArgumentsFailed(err))
			}
			return err
		}
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

type miscDeps struct {
	fx.In
	Lc fx.Lifecycle

	Config       config.Component
	Syscfg       sysprobeconfig.Component
	HostInfo     hostinfo.Component
	WorkloadMeta workloadmeta.Component
	Logger       logcomp.Component
	Tagger       tagger.Component
}

// initMisc initializes modules that cannot, or have not yet been componetized.
// Todo: (Components) WorkloadMeta, remoteTagger
// Todo: move metadata/workloadmeta/collector to workloadmeta
func initMisc(deps miscDeps) error {
	if err := ddutil.SetupCoreDump(deps.Config); err != nil {
		deps.Logger.Warnf("Can't setup core dumps: %v, core dumps might not be available after a crash", err)
	}

	// InitSharedContainerProvider must be called before the application starts so the workloadmeta collector can be initiailized correctly.
	// Since the tagger depends on the workloadmeta collector, we can not make the tagger a dependency of workloadmeta as it would create a circular dependency.
	// TODO: (component) - once we remove the dependency of workloadmeta component from the tagger component
	// we can include the tagger as part of the workloadmeta component.
	proccontainers.InitSharedContainerProvider(deps.WorkloadMeta, deps.Tagger)

	processCollectionServer := collector.NewProcessCollector(deps.Config, deps.Syscfg)

	// TODO(components): still unclear how the initialization of workloadmeta
	//                   store and tagger should be performed.
	// appCtx is a context that cancels when the OnStop hook is called
	appCtx, stopApp := context.WithCancel(context.Background())
	deps.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			if collector.Enabled(deps.Config) {
				err := processCollectionServer.Start(appCtx, deps.WorkloadMeta)
				if err != nil {
					return err
				}
			}

			return nil
		},
		OnStop: func(_ context.Context) error {
			stopApp()

			return nil
		},
	})

	return nil
}

// shouldStayAlive determines whether the process agent should stay alive when no checks are running.
// This can happen when the checks are running on the core agent but a process agent container is
// still brought up. The process-agent is kept alive to prevent crash loops.
func shouldStayAlive(cfg model.Reader) bool {
	if env.IsKubernetes() && cfg.GetBool("process_config.run_in_core_agent.enabled") {
		log.Warn("The process-agent is staying alive to prevent crash loops due to the checks running on the core agent. Thus, the process-agent is idle. Update your Helm chart or Datadog Operator to the latest version to prevent this (https://docs.datadoghq.com/containers/kubernetes/installation/).")
		return true
	}

	return false
}
