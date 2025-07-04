// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package start implements start related subcommands
package start

import (
	"context"
	"errors"
	_ "expvar" // Blank import used because this isn't directly used in this file
	"fmt"
	"net/http"
	_ "net/http/pprof" // Blank import used because this isn't directly used in this file
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/cmd/security-agent/api"
	"github.com/DataDog/datadog-agent/cmd/security-agent/command"
	"github.com/DataDog/datadog-agent/cmd/security-agent/subcommands/compliance"
	"github.com/DataDog/datadog-agent/comp/agent/autoexit"
	"github.com/DataDog/datadog-agent/comp/agent/autoexit/autoexitimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/configsync/configsyncimpl"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/pid"
	"github.com/DataDog/datadog-agent/comp/core/pid/pidimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	remoteTaggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-remote"
	taggerTypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog-remote"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	pkgCompliance "github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	ddruntime "github.com/DataDog/datadog-agent/pkg/runtime"
	"github.com/DataDog/datadog-agent/pkg/security/agent"
	"github.com/DataDog/datadog-agent/pkg/security/utils/hostnameutils"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/coredump"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type cliParams struct {
	*command.GlobalParams

	pidfilePath string
}

// Commands returns the start commands
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	params := &cliParams{
		GlobalParams: globalParams,
	}

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Security Agent",
		Long:  `Runs Datadog Security agent in the foreground`,
		RunE: func(_ *cobra.Command, _ []string) error {
			// TODO: Similar to the agent itself, once the security agent is represented as a component, and not a function (start),
			// this will use `fxutil.Run` instead of `fxutil.OneShot`.

			// note that any changes to the arguments to OneShot need to be reflected into
			// the service initialization in ../../main_windows.go
			return fxutil.OneShot(start,
				fx.Supply(core.BundleParams{
					ConfigParams:         config.NewSecurityAgentParams(params.ConfigFilePaths, config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					SysprobeConfigParams: sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.SysProbeConfFilePath), sysprobeconfigimpl.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					SecretParams:         secrets.NewEnabledParams(),
					LogParams:            log.ForDaemon(command.LoggerName, "security_agent.log_file", pkgconfigsetup.DefaultSecurityAgentLogFile),
				}),
				core.Bundle(),
				statsd.Module(),
				// workloadmeta setup
				wmcatalog.GetCatalog(),
				workloadmetafx.Module(workloadmeta.Params{
					AgentType: workloadmeta.Remote,
				}),
				remoteTaggerfx.Module(tagger.RemoteParams{
					RemoteTarget: func(c config.Component) (string, error) {
						return fmt.Sprintf(":%v", c.GetInt("cmd_port")), nil
					},
					RemoteFilter: taggerTypes.NewMatchAllFilter(),
				}),
				fx.Provide(func() startstop.Stopper {
					return startstop.NewSerialStopper()
				}),
				fx.Provide(func(config config.Component, statsd statsd.Component) (ddgostatsd.ClientInterface, error) {
					return statsd.CreateForHostPort(pkgconfigsetup.GetBindHost(config), config.GetInt("dogstatsd_port"))
				}),
				fx.Provide(func(stopper startstop.Stopper, log log.Component, config config.Component, statsdClient ddgostatsd.ClientInterface, wmeta workloadmeta.Component, compression logscompression.Component, ipc ipc.Component) (status.InformationProvider, *agent.RuntimeSecurityAgent, error) {
					hostnameDetected, err := hostnameutils.GetHostnameWithContextAndFallback(context.TODO(), ipc)
					if err != nil {
						return status.NewInformationProvider(nil), nil, err
					}

					runtimeAgent, err := agent.StartRuntimeSecurity(log, config, hostnameDetected, stopper, statsdClient, wmeta, compression)
					if err != nil {
						return status.NewInformationProvider(nil), nil, err
					}

					if runtimeAgent == nil {
						return status.NewInformationProvider(nil), nil, nil
					}

					// TODO - components: Do not remove runtimeAgent ref until "github.com/DataDog/datadog-agent/pkg/security/agent" is a component so they're not GCed
					return status.NewInformationProvider(runtimeAgent.StatusProvider()), runtimeAgent, nil
				}),
				fx.Provide(func(stopper startstop.Stopper, log log.Component, config config.Component, statsdClient ddgostatsd.ClientInterface, sysprobeconfig sysprobeconfig.Component, wmeta workloadmeta.Component, compression logscompression.Component, ipc ipc.Component) (status.InformationProvider, *pkgCompliance.Agent, error) {
					hostnameDetected, err := hostnameutils.GetHostnameWithContextAndFallback(context.TODO(), ipc)
					if err != nil {
						return status.NewInformationProvider(nil), nil, err
					}

					// start compliance security agent
					complianceAgent, err := compliance.StartCompliance(log, config, sysprobeconfig, hostnameDetected, stopper, statsdClient, wmeta, compression, ipc)
					if err != nil {
						return status.NewInformationProvider(nil), nil, err
					}

					if complianceAgent == nil {
						return status.NewInformationProvider(nil), nil, nil
					}

					// TODO - components: Do not remove complianceAgent ref until "github.com/DataDog/datadog-agent/pkg/compliance" is a component so they're not GCed
					return status.NewInformationProvider(complianceAgent.StatusProvider()), complianceAgent, nil
				}),
				fx.Supply(
					status.Params{
						PythonVersionGetFunc: python.GetPythonVersion,
					},
				),
				statusimpl.Module(),
				configsyncimpl.Module(configsyncimpl.NewDefaultParams()),
				autoexitimpl.Module(),
				fx.Supply(pidimpl.NewParams(params.pidfilePath)),
				fx.Provide(func(c config.Component) settings.Params {
					return settings.Params{
						Settings: map[string]settings.RuntimeSetting{
							"log_level": commonsettings.NewLogLevelRuntimeSetting(),
						},
						Config: c,
					}
				}),
				settingsimpl.Module(),
				logscompressionfx.Module(),
				ipcfx.ModuleReadWrite(),
			)
		},
	}

	startCmd.Flags().StringVarP(&params.pidfilePath, "pidfile", "p", "", "path to the pidfile")

	return []*cobra.Command{startCmd}
}

// start will start the security-agent.
//
// TODO(components): note how workloadmeta is passed anonymously, it is still required as it is used
// as a global. This should eventually be fixed and all workloadmeta interactions should be via the
// injected instance.
func start(log log.Component, config config.Component, secrets secrets.Component, _ statsd.Component, _ sysprobeconfig.Component, telemetry telemetry.Component, statusComponent status.Component, _ pid.Component, _ autoexit.Component, settings settings.Component, wmeta workloadmeta.Component, ipc ipc.Component) error {
	defer StopAgent(log)

	// prepare go runtime
	ddruntime.SetMaxProcs()

	if config.GetBool("security_agent.disable_thp") {
		if err := ddruntime.DisableTransparentHugePages(); err != nil {
			log.Warnf("cannot disable transparent huge pages, performance may be degraded: %s", err)
		}
	}

	err := RunAgent(log, config, secrets, telemetry, statusComponent, settings, wmeta, ipc)
	if errors.Is(err, ErrAllComponentsDisabled) || errors.Is(err, errNoAPIKeyConfigured) {
		return nil
	}
	if err != nil {
		return err
	}

	stopCh := make(chan struct{})
	defer close(stopCh)
	go handleSignals(log, stopCh)

	// Block here until we receive a stop signal
	<-stopCh

	return nil
}

// handleSignals handles OS signals, and sends a message on stopCh when an interrupt
// signal is received.
func handleSignals(log log.Component, stopCh chan struct{}) {
	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGPIPE)

	// Block here until we receive the interrupt signal
	for signo := range signalCh {
		switch signo {
		case syscall.SIGPIPE:
			// By default, systemd redirects the stdout to journald. When journald is stopped or crashes we receive a SIGPIPE signal.
			// Go ignores SIGPIPE signals unless it is when stdout or stdout is closed, in this case the agent is stopped.
			// We never want dogstatsd to stop upon receiving SIGPIPE, so we intercept the SIGPIPE signals and just discard them.
		default:
			log.Infof("Received signal '%s', shutting down...", signo)

			stopCh <- struct{}{}
			return
		}
	}
}

var (
	stopper      startstop.Stopper
	srv          *api.Server
	expvarServer *http.Server
)

// ErrAllComponentsDisabled is returned when all components are disabled
var ErrAllComponentsDisabled = errors.New("all security-agent component are disabled")
var errNoAPIKeyConfigured = errors.New("no API key configured")

// RunAgent initialized resources and starts API server
func RunAgent(log log.Component, config config.Component, secrets secrets.Component, telemetry telemetry.Component, statusComponent status.Component, settings settings.Component, wmeta workloadmeta.Component, ipc ipc.Component) (err error) {
	if err := coredump.Setup(config); err != nil {
		log.Warnf("Can't setup core dumps: %v, core dumps might not be available after a crash", err)
	}

	// Check if we have at least one component to start based on config
	if !config.GetBool("compliance_config.enabled") && !config.GetBool("runtime_security_config.enabled") {
		log.Infof("All security-agent components are deactivated, exiting")

		// A sleep is necessary so that sysV doesn't think the agent has failed
		// to startup because of an error. Only applies on Debian 7.
		time.Sleep(5 * time.Second)

		return ErrAllComponentsDisabled
	}

	if !config.IsConfigured("api_key") {
		log.Critical("No API key configured, exiting")

		// A sleep is necessary so that sysV doesn't think the agent has failed
		// to startup because of an error. Only applies on Debian 7.
		time.Sleep(5 * time.Second)

		return errNoAPIKeyConfigured
	}

	// Setup expvar server
	port := config.GetString("security_agent.expvar_port")
	pkgconfigsetup.Datadog().Set("expvar_port", port, model.SourceAgentRuntime)
	if config.GetBool("telemetry.enabled") {
		http.Handle("/telemetry", telemetry.Handler())
	}
	expvarServer := &http.Server{
		Addr:    "127.0.0.1:" + port,
		Handler: http.DefaultServeMux,
	}
	go func() {
		err := expvarServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Errorf("Error creating expvar server on port %v: %v", port, err)
		}
	}()

	srv, err = api.NewServer(statusComponent, settings, wmeta, ipc, secrets)
	if err != nil {
		return log.Errorf("Error while creating api server, exiting: %v", err)
	}

	if err = srv.Start(); err != nil {
		return log.Errorf("Error while starting api server, exiting: %v", err)
	}

	if err := setupInternalProfiling(config); err != nil {
		return log.Errorf("Error while setuping internal profiling, exiting: %v", err)
	}

	log.Infof("Datadog Security Agent is now running.")

	return
}

// StopAgent stops the API server and clean up resources
func StopAgent(log log.Component) {
	// retrieve the agent health before stopping the components
	// GetReadyNonBlocking has a 100ms timeout to avoid blocking
	healthStatus, err := health.GetReadyNonBlocking()
	if err != nil {
		log.Warnf("Security Agent health unknown: %s", err)
	} else if len(healthStatus.Unhealthy) > 0 {
		log.Warnf("Some components were unhealthy: %v", healthStatus.Unhealthy)
	}

	// stop metaScheduler and statsd if they are instantiated
	if stopper != nil {
		stopper.Stop()
	}

	if srv != nil {
		srv.Stop()
	}
	if expvarServer != nil {
		if err := expvarServer.Shutdown(context.Background()); err != nil {
			log.Errorf("Error shutting down expvar server: %v", err)
		}
	}

	log.Info("See ya!")
}

func setupInternalProfiling(config config.Component) error {
	if config.GetBool(secAgentKey("internal_profiling.enabled")) {
		cfgSite := config.GetString(secAgentKey("internal_profiling.site"))
		cfgURL := config.GetString(secAgentKey("internal_profiling.profile_dd_url"))

		// check if TRACE_AGENT_URL is set, in which case, forward the profiles to the trace agent
		var site string
		if traceAgentURL := os.Getenv("TRACE_AGENT_URL"); len(traceAgentURL) > 0 {
			site = fmt.Sprintf(profiling.ProfilingLocalURLTemplate, traceAgentURL)
		} else {
			site = fmt.Sprintf(profiling.ProfilingURLTemplate, cfgSite)
			if cfgURL != "" {
				site = cfgURL
			}
		}

		tags := config.GetStringSlice(secAgentKey("internal_profiling.extra_tags"))
		tags = append(tags, fmt.Sprintf("version:%v", version.AgentVersion))
		tags = append(tags, "__dd_internal_profiling:datadog-agent")

		profSettings := profiling.Settings{
			ProfilingURL:         site,
			Env:                  config.GetString(secAgentKey("internal_profiling.env")),
			Service:              "security-agent",
			Period:               config.GetDuration(secAgentKey("internal_profiling.period")),
			CPUDuration:          config.GetDuration(secAgentKey("internal_profiling.cpu_duration")),
			MutexProfileFraction: config.GetInt(secAgentKey("internal_profiling.mutex_profile_fraction")),
			BlockProfileRate:     config.GetInt(secAgentKey("internal_profiling.block_profile_rate")),
			WithGoroutineProfile: config.GetBool(secAgentKey("internal_profiling.enable_goroutine_stacktraces")),
			WithBlockProfile:     config.GetBool(secAgentKey("internal_profiling.enable_block_profiling")),
			WithMutexProfile:     config.GetBool(secAgentKey("internal_profiling.enable_mutex_profiling")),
			WithDeltaProfiles:    config.GetBool(secAgentKey("internal_profiling.delta_profiles")),
			Socket:               config.GetString(secAgentKey("internal_profiling.unix_socket")),
			Tags:                 tags,
		}

		return profiling.Start(profSettings)
	}

	return nil
}

func secAgentKey(sub string) string {
	return fmt.Sprintf("security_agent.%s", sub)
}
