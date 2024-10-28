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

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/cmd/system-probe/common"
	systemprobeconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/comp/agent/autoexit"
	"github.com/DataDog/datadog-agent/comp/agent/autoexit/autoexitimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	healthprobe "github.com/DataDog/datadog-agent/comp/core/healthprobe/def"
	healthprobefx "github.com/DataDog/datadog-agent/comp/core/healthprobe/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	systemprobeloggerfx "github.com/DataDog/datadog-agent/comp/core/log/fx-systemprobe"
	"github.com/DataDog/datadog-agent/comp/core/pid"
	"github.com/DataDog/datadog-agent/comp/core/pid/pidimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	compstatsd "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/rcclientimpl"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	processstatsd "github.com/DataDog/datadog-agent/pkg/process/statsd"
	ddruntime "github.com/DataDog/datadog-agent/pkg/runtime"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
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
				fx.Supply(config.NewAgentParams("", config.WithConfigMissingOK(true))),
				fx.Supply(sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.ConfFilePath), sysprobeconfigimpl.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath))),
				fx.Supply(log.ForDaemon("SYS-PROBE", "log_file", common.DefaultLogFile)),
				fx.Supply(rcclient.Params{AgentName: "system-probe", AgentVersion: version.AgentVersion, IsSystemProbe: true}),
				fx.Supply(optional.NewNoneOption[secrets.Component]()),
				compstatsd.Module(),
				config.Module(),
				telemetryimpl.Module(),
				sysprobeconfigimpl.Module(),
				rcclientimpl.Module(),
				fx.Provide(func(config config.Component, sysprobeconfig sysprobeconfig.Component) healthprobe.Options {
					return healthprobe.Options{
						Port:           sysprobeconfig.SysProbeObject().HealthPort,
						LogsGoroutines: config.GetBool("log_all_goroutines_when_unhealthy"),
					}
				}),
				healthprobefx.Module(),
				systemprobeloggerfx.Module(),
				// workloadmeta setup
				wmcatalog.GetCatalog(),
				workloadmetafx.Module(workloadmeta.Params{
					AgentType: workloadmeta.Remote,
				}),
				autoexitimpl.Module(),
				pidimpl.Module(),
				fx.Supply(pidimpl.NewParams(cliParams.pidfilePath)),
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
			)
		},
	}
	runCmd.Flags().StringVarP(&cliParams.pidfilePath, "pid", "p", "", "path to the pidfile")

	return []*cobra.Command{runCmd}
}

// run starts the main loop.
func run(log log.Component, _ config.Component, statsd compstatsd.Component, telemetry telemetry.Component, sysprobeconfig sysprobeconfig.Component, rcclient rcclient.Component, wmeta workloadmeta.Component, _ pid.Component, _ healthprobe.Component, _ autoexit.Component, settings settings.Component) error {
	defer func() {
		stopSystemProbe()
	}()

	// prepare go runtime
	ddruntime.SetMaxProcs()

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
		//nolint:revive
		for range sigpipeCh {
			// intentionally drain channel
		}
	}()

	if err := startSystemProbe(log, statsd, telemetry, sysprobeconfig, rcclient, wmeta, settings); err != nil {
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

// StartSystemProbeWithDefaults is a temporary way for other packages to use startSystemProbe.
// Starts the agent in the background and then returns.
//
// @ctxChan
//   - After starting the agent the background goroutine waits for a context from
//     this channel, then stops the agent when the context is cancelled.
//
// Returns an error channel that can be used to wait for the agent to stop and get the result.
func StartSystemProbeWithDefaults(ctxChan <-chan context.Context) (<-chan error, error) {
	errChan := make(chan error)

	// run startSystemProbe in the background
	go func() {
		err := runSystemProbe(ctxChan, errChan)
		// notify main routine that this is done, so cleanup can happen
		errChan <- err
	}()

	// Wait for startSystemProbe to complete, or for an error
	err := <-errChan
	if err != nil {
		// startSystemProbe or fx.OneShot failed, caller does not need errChan
		return nil, err
	}

	// startSystemProbe succeeded. provide errChan to caller so they can wait for fxutil.OneShot to stop
	return errChan, nil
}

func runSystemProbe(ctxChan <-chan context.Context, errChan chan error) error {
	return fxutil.OneShot(
		func(log log.Component, _ config.Component, statsd compstatsd.Component, telemetry telemetry.Component, sysprobeconfig sysprobeconfig.Component, rcclient rcclient.Component, wmeta workloadmeta.Component, _ healthprobe.Component, settings settings.Component) error {
			defer StopSystemProbeWithDefaults()
			err := startSystemProbe(log, statsd, telemetry, sysprobeconfig, rcclient, wmeta, settings)
			if err != nil {
				return err
			}

			// notify outer that startAgent finished
			errChan <- err
			// wait for context
			ctx := <-ctxChan

			// Wait for stop signal
			select {
			case <-signals.Stopper:
				log.Info("Received stop command, shutting down...")
			case <-signals.ErrorStopper:
				_ = log.Critical("The Agent has encountered an error, shutting down...")
			case <-ctx.Done():
				log.Info("Received stop from service manager, shutting down...")
			}

			return nil
		},
		// no config file path specification in this situation
		fx.Supply(config.NewAgentParams("", config.WithConfigMissingOK(true))),
		fx.Supply(sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(""))),
		fx.Supply(log.ForDaemon("SYS-PROBE", "log_file", common.DefaultLogFile)),
		fx.Supply(rcclient.Params{AgentName: "system-probe", AgentVersion: version.AgentVersion, IsSystemProbe: true}),
		fx.Supply(optional.NewNoneOption[secrets.Component]()),
		rcclientimpl.Module(),
		config.Module(),
		telemetryimpl.Module(),
		compstatsd.Module(),
		sysprobeconfigimpl.Module(),
		fx.Provide(func(config config.Component, sysprobeconfig sysprobeconfig.Component) healthprobe.Options {
			return healthprobe.Options{
				Port:           sysprobeconfig.SysProbeObject().HealthPort,
				LogsGoroutines: config.GetBool("log_all_goroutines_when_unhealthy"),
			}
		}),
		healthprobefx.Module(),
		// workloadmeta setup
		wmcatalog.GetCatalog(),
		workloadmetafx.Module(workloadmeta.Params{
			AgentType: workloadmeta.Remote,
		}),
		systemprobeloggerfx.Module(),
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
	)
}

// StopSystemProbeWithDefaults is a temporary way for other packages to use stopAgent.
func StopSystemProbeWithDefaults() {
	stopSystemProbe()
}

// startSystemProbe Initializes the system-probe process
func startSystemProbe(log log.Component, statsd compstatsd.Component, telemetry telemetry.Component, sysprobeconfig sysprobeconfig.Component, _ rcclient.Component, wmeta workloadmeta.Component, settings settings.Component) error {
	var err error
	cfg := sysprobeconfig.SysProbeObject()

	log.Infof("starting system-probe v%v", version.AgentVersion)

	logUserAndGroupID(log)
	// Exit if system probe is disabled
	if cfg.ExternalSystemProbe || !cfg.Enabled {
		log.Info("system probe not enabled. exiting")
		return ErrNotEnabled
	}

	if err := util.SetupCoreDump(sysprobeconfig); err != nil {
		log.Warnf("cannot setup core dumps: %s, core dumps might not be available after a crash", err)
	}

	if sysprobeconfig.GetBool("system_probe_config.memory_controller.enabled") {
		memoryPressureLevels := sysprobeconfig.GetStringMapString("system_probe_config.memory_controller.pressure_levels")
		memoryThresholds := sysprobeconfig.GetStringMapString("system_probe_config.memory_controller.thresholds")
		hierarchy := sysprobeconfig.GetString("system_probe_config.memory_controller.hierarchy")
		common.MemoryMonitor, err = utils.NewMemoryMonitor(hierarchy, env.IsContainerized(), memoryPressureLevels, memoryThresholds)
		if err != nil {
			log.Warnf("cannot set up memory controller: %s", err)
		} else {
			common.MemoryMonitor.Start()
		}
	}

	setupInternalProfiling(settings, sysprobeconfig, configPrefix, log)

	if err := processstatsd.Configure(cfg.StatsdHost, cfg.StatsdPort, statsd.CreateForHostPort); err != nil {
		return log.Criticalf("error configuring statsd: %s", err)
	}

	if isValidPort(cfg.DebugPort) {
		if cfg.TelemetryEnabled {
			http.Handle("/telemetry", telemetry.Handler())
			telemetry.RegisterCollector(ebpftelemetry.NewDebugFsStatCollector())
			if pc := ebpftelemetry.NewPerfUsageCollector(); pc != nil {
				telemetry.RegisterCollector(pc)
			}
			if lcc := ddebpf.NewLockContentionCollector(); lcc != nil {
				telemetry.RegisterCollector(lcc)
			}
			if ec := ebpftelemetry.NewEBPFErrorsCollector(); ec != nil {
				telemetry.RegisterCollector(ec)
			}
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

	if err = api.StartServer(cfg, telemetry, wmeta, settings); err != nil {
		return log.Criticalf("error while starting api server, exiting: %v", err)
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
