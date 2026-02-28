// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package run is the run system-probe subcommand
package run

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	_ "net/http/pprof" // activate pprof profiling
	"os"
	"os/signal"
	"os/user"
	"syscall"
	"time"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/cmd/system-probe/common"
	"github.com/DataDog/datadog-agent/comp/agent/autoexit"
	"github.com/DataDog/datadog-agent/comp/agent/autoexit/autoexitimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/configsync/configsyncimpl"
	delegatedauthnoopfx "github.com/DataDog/datadog-agent/comp/core/delegatedauth/fx-noop"
	fxinstrumentation "github.com/DataDog/datadog-agent/comp/core/fxinstrumentation/fx"
	healthprobe "github.com/DataDog/datadog-agent/comp/core/healthprobe/def"
	healthprobefx "github.com/DataDog/datadog-agent/comp/core/healthprobe/fx"
	"github.com/DataDog/datadog-agent/comp/core/hostname/remotehostnameimpl"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	systemprobeloggerfx "github.com/DataDog/datadog-agent/comp/core/log/fx-systemprobe"
	"github.com/DataDog/datadog-agent/comp/core/pid"
	"github.com/DataDog/datadog-agent/comp/core/pid/pidimpl"
	remoteagentfx "github.com/DataDog/datadog-agent/comp/core/remoteagent/fx-systemprobe"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	remoteTaggerFx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-remote"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	remoteWorkloadfilterfx "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-remote"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog-remote"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	connectionsforwarderfx "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/fx"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl"
	localtraceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/fx-local"
	rdnsquerierfx "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/rcclientimpl"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	ddruntime "github.com/DataDog/datadog-agent/pkg/runtime"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	systemprobeconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/coredump"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// ErrNotEnabled represents the case in which system-probe is not enabled
var ErrNotEnabled = errors.New("system-probe not enabled")

const configPrefix = systemprobeconfig.Namespace + "."

type cliParams struct {
	*command.GlobalParams

	// pidfilePath contains the value of the --pidfile flag.
	pidfilePath string
}

const configSyncTimeout = 10 * time.Second

// Commands returns a slice of subcommands for the 'system-probe' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run the System Probe",
		Long:  `Runs the system-probe in the foreground`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(run,
				fx.Invoke(func(_ log.Component) {
					ddruntime.SetMaxProcs()
				}),
				fx.Supply(config.NewAgentParams(globalParams.DatadogConfFilePath())),
				fx.Supply(sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.ConfFilePath), sysprobeconfigimpl.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath))),
				fx.Supply(pidimpl.NewParams(cliParams.pidfilePath)),
				getSharedFxOption(),
			)
		},
	}
	runCmd.Flags().StringVarP(&cliParams.pidfilePath, "pid", "p", "", "path to the pidfile")

	return []*cobra.Command{runCmd}
}

func getSharedFxOption() fx.Option {
	return fx.Options(
		fx.Supply(log.ForDaemon(command.LoggerName, "log_file", common.DefaultLogFile)),
		config.Module(),
		delegatedauthnoopfx.Module(),
		sysprobeconfigimpl.Module(),
		systemprobeloggerfx.Module(),
		telemetryimpl.Module(),
		pidimpl.Module(),
		fx.Supply(rcclient.Params{AgentName: "system-probe", AgentVersion: version.AgentVersion, IsSystemProbe: true}),
		secretsnoopfx.Module(),
		statsd.Module(),
		rcclientimpl.Module(),
		fx.Provide(func(config config.Component, sysprobeconfig sysprobeconfig.Component) healthprobe.Options {
			return healthprobe.Options{
				Port:           sysprobeconfig.SysProbeObject().HealthPort,
				LogsGoroutines: config.GetBool("log_all_goroutines_when_unhealthy"),
			}
		}),
		healthprobefx.Module(),
		wmcatalog.GetCatalog(),
		workloadmetafx.Module(workloadmeta.Params{
			AgentType: workloadmeta.Remote,
		}),
		remoteWorkloadfilterfx.Module(),
		ipcfx.ModuleReadWrite(),
		remoteTaggerFx.Module(tagger.NewRemoteParams()),
		autoexitimpl.Module(),
		fx.Provide(func(sysprobeconfig sysprobeconfig.Component) settings.Params {
			profilingGoRoutines := commonsettings.NewProfilingGoroutines()
			profilingGoRoutines.ConfigPrefix = configPrefix

			return settings.Params{
				Settings: map[string]settings.RuntimeSetting{
					"log_level":                       commonsettings.NewLogLevelRuntimeSetting(),
					"runtime_mutex_profile_fraction":  &commonsettings.RuntimeMutexProfileFraction{ConfigPrefix: configPrefix},
					"runtime_block_profile_rate":      &commonsettings.RuntimeBlockProfileRate{ConfigPrefix: configPrefix},
					"internal_profiling_goroutines":   profilingGoRoutines,
					commonsettings.MaxDumpSizeConfKey: &commonsettings.ActivityDumpRuntimeSetting{ConfigKey: commonsettings.MaxDumpSizeConfKey},
					"internal_profiling":              &commonsettings.ProfilingRuntimeSetting{SettingName: "internal_profiling", Service: "system-probe", ConfigPrefix: configPrefix},
				},
				Config: sysprobeconfig,
			}
		}),
		settingsimpl.Module(),
		logscompressionfx.Module(),
		fx.Provide(func(config config.Component, statsd statsd.Component) (ddgostatsd.ClientInterface, error) {
			return statsd.CreateForHostPort(configutils.GetBindHost(config), config.GetInt("dogstatsd_port"))
		}),
		remotehostnameimpl.Module(),
		configsyncimpl.Module(configsyncimpl.NewParams(configSyncTimeout, true, configSyncTimeout)),
		remoteagentfx.Module(),
		fxinstrumentation.Module(),
		localtraceroute.Module(),
		connectionsforwarderfx.Module(),
		eventplatformreceiverimpl.Module(),
		eventplatformimpl.Module(eventplatformimpl.NewDefaultParams()),
		rdnsquerierfx.Module(),
		npcollectorimpl.Module(),
	)
}

// run starts the main loop.
func run(
	_ config.Component,
	rcclient rcclient.Component,
	_ pid.Component,
	_ healthprobe.Component,
	_ autoexit.Component,
	settings settings.Component,
	_ ipc.Component,
	deps module.FactoryDependencies,
) error {
	defer stopSystemProbe()

	if deps.SysprobeConfig.GetBool("system_probe_config.disable_thp") {
		if err := ddruntime.DisableTransparentHugePages(); err != nil {
			deps.Log.Warnf("cannot disable transparent huge pages, performance may be degraded: %s", err)
		}
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
			deps.Log.Info("Received stop command, shutting down...")
			stopCh <- nil
		case <-signals.ErrorStopper:
			_ = deps.Log.Critical("system-probe has encountered an error, shutting down...")
			stopCh <- errors.New("shutting down because of an error")
		case sig := <-signalCh:
			deps.Log.Infof("Received signal '%s', shutting down...", sig)
			stopCh <- nil
		}
	}()

	// By default, systemd redirects the stdout to journald. When journald is stopped or crashes we receive a SIGPIPE signal.
	// Go ignores SIGPIPE signals unless it is when stdout or stdout is closed, in this case the agent is stopped.
	// We never want the agent to stop upon receiving SIGPIPE, so we intercept the SIGPIPE signals and just discard them.
	sigpipeCh := make(chan os.Signal, 1)
	signal.Notify(sigpipeCh, syscall.SIGPIPE)
	go func() {
		//nolint:revive
		for range sigpipeCh {
			// intentionally drain channel
		}
	}()

	if err := startSystemProbe(rcclient, settings, deps); err != nil {
		if errors.Is(err, ErrNotEnabled) {
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

// startSystemProbe Initializes the system-probe process
func startSystemProbe(rcclient rcclient.Component, settings settings.Component, deps module.FactoryDependencies) error {
	var err error
	cfg := deps.SysprobeConfig.SysProbeObject()

	deps.Log.Infof("starting system-probe v%v", version.AgentVersion)

	logUserAndGroupID(deps.Log)
	// Exit if system probe is disabled
	if cfg.ExternalSystemProbe || !cfg.Enabled {
		deps.Log.Info("system probe not enabled. exiting")
		return ErrNotEnabled
	}

	if err := coredump.Setup(deps.SysprobeConfig); err != nil {
		deps.Log.Warnf("cannot setup core dumps: %s, core dumps might not be available after a crash", err)
	}

	if deps.SysprobeConfig.GetBool("system_probe_config.memory_controller.enabled") {
		memoryPressureLevels := deps.SysprobeConfig.GetStringMapString("system_probe_config.memory_controller.pressure_levels")
		memoryThresholds := deps.SysprobeConfig.GetStringMapString("system_probe_config.memory_controller.thresholds")
		hierarchy := deps.SysprobeConfig.GetString("system_probe_config.memory_controller.hierarchy")
		common.MemoryMonitor, err = utils.NewMemoryMonitor(hierarchy, env.IsContainerized(), memoryPressureLevels, memoryThresholds)
		if err != nil {
			deps.Log.Warnf("cannot set up memory controller: %s", err)
		} else {
			common.MemoryMonitor.Start()
		}
	}

	setupInternalProfiling(settings, deps.SysprobeConfig, configPrefix, deps.Log)

	if isValidPort(cfg.DebugPort) {
		if cfg.TelemetryEnabled {
			http.Handle("/telemetry", deps.Telemetry.Handler())
			deps.Telemetry.RegisterCollector(ebpftelemetry.NewDebugFsStatCollector())
			if pc := ebpftelemetry.NewPerfUsageCollector(); pc != nil {
				deps.Telemetry.RegisterCollector(pc)
			}
			if lcc := ddebpf.NewLockContentionCollector(); lcc != nil {
				deps.Telemetry.RegisterCollector(lcc)
			}
			if ec := ebpftelemetry.NewEBPFErrorsCollector(); ec != nil {
				deps.Telemetry.RegisterCollector(ec)
			}
		}
		go func() {
			common.ExpvarServer = &http.Server{
				Addr:    fmt.Sprintf("127.0.0.1:%d", cfg.DebugPort),
				Handler: http.DefaultServeMux,
			}
			if err := common.ExpvarServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				deps.Log.Errorf("error creating expvar server on %v: %v", common.ExpvarServer.Addr, err)
			}
		}()
	}

	if err = api.StartServer(cfg, settings, rcclient, deps); err != nil {
		return deps.Log.Criticalf("error while starting api server, exiting: %v", err)
	}
	return nil
}

// stopSystemProbe Tears down the system-probe process
func stopSystemProbe() {
	module.Close()
	if common.ExpvarServer != nil {
		if err := common.ExpvarServer.Shutdown(context.Background()); err != nil {
			pkglog.Errorf("error shutting down expvar server: %s", err)
		}
	}
	profiling.Stop()
	if common.MemoryMonitor != nil {
		common.MemoryMonitor.Stop()
	}

	pkglog.Flush()
}

// setupInternalProfiling is a common helper to configure runtime settings for internal profiling.
func setupInternalProfiling(settings settings.Component, cfg model.Reader, configPrefix string, log log.Component) {
	if v := cfg.GetInt(configPrefix + "internal_profiling.block_profile_rate"); v > 0 {
		if err := settings.SetRuntimeSetting("runtime_block_profile_rate", v, model.SourceAgentRuntime); err != nil {
			log.Errorf("Error setting block profile rate: %v", err)
		}
	}

	if v := cfg.GetInt(configPrefix + "internal_profiling.mutex_profile_fraction"); v > 0 {
		if err := settings.SetRuntimeSetting("runtime_mutex_profile_fraction", v, model.SourceAgentRuntime); err != nil {
			log.Errorf("Error mutex profile fraction: %v", err)
		}
	}

	if cfg.GetBool(configPrefix + "internal_profiling.enabled") {
		err := settings.SetRuntimeSetting("internal_profiling", true, model.SourceAgentRuntime)
		if err != nil {
			log.Errorf("Error starting profiler: %v", err)
		}
	}
}

func isValidPort(port int) bool {
	return port > 0 && port < 65536
}

func logUserAndGroupID(log log.Component) {
	currentUser, err := user.Current()
	if err != nil {
		log.Warnf("error fetching current user: %s", err)
		return
	}
	uid := currentUser.Uid
	gid := currentUser.Gid
	log.Infof("current user id/name: %s/%s", uid, currentUser.Name)
	currentGroup, err := user.LookupGroupId(gid)
	if err == nil {
		log.Infof("current group id/name: %s/%s", gid, currentGroup.Name)
	} else {
		log.Warnf("unable to resolve group: %s", err)
	}
}
