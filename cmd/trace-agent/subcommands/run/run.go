// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"time"

	"github.com/DataDog/datadog-agent/cmd/manager"
	remotecfg "github.com/DataDog/datadog-agent/cmd/trace-agent/config/remote"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	rc "github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/tagger/remote"
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	tracecfg "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	tracelog "github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	agentrt "github.com/DataDog/datadog-agent/pkg/runtime"

	// register all workloadmeta collectors
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors"
)

const messageAgentDisabled = `trace-agent not enabled. Set the environment variable
DD_APM_ENABLED=true or add "apm_config.enabled: true" entry
to your datadog.yaml. Exiting...`

// Stack depth of 3 since the `corelogger` struct adds a layer above the logger
const stackDepth = 3

// Run is the entrypoint of our code, which starts the agent.
func runAgent(ctx context.Context, cliParams *RunParams, cfg config.Component) error {

	tracecfg := cfg.Object()
	err := info.InitInfo(tracecfg) // for expvar & -info option
	if err != nil {
		return err
	}

	telemetryCollector := telemetry.NewCollector(tracecfg)

	if err := coreconfig.SetupLogger(
		coreconfig.LoggerName("TRACE"),
		coreconfig.Datadog.GetString("log_level"),
		tracecfg.LogFilePath,
		coreconfig.GetSyslogURI(),
		coreconfig.Datadog.GetBool("syslog_rfc"),
		coreconfig.Datadog.GetBool("log_to_console"),
		coreconfig.Datadog.GetBool("log_format_json"),
	); err != nil {
		telemetryCollector.SendStartupError(telemetry.CantCreateLogger, err)
		return fmt.Errorf("Cannot create logger: %v", err)
	}
	tracelog.SetLogger(corelogger{})
	defer log.Flush()

	if !tracecfg.Enabled {
		log.Info(messageAgentDisabled)
		telemetryCollector.SendStartupError(telemetry.TraceAgentNotEnabled, fmt.Errorf(""))

		return nil
	}

	defer watchdog.LogOnPanic()

	if cliParams.CPUProfile != "" {
		f, err := os.Create(cliParams.CPUProfile)
		if err != nil {
			log.Error(err)
		}
		pprof.StartCPUProfile(f) //nolint:errcheck
		log.Info("CPU profiling started...")
		defer pprof.StopCPUProfile()
	}

	if cliParams.PIDFilePath != "" {
		err := pidfile.WritePID(cliParams.PIDFilePath)
		if err != nil {
			telemetryCollector.SendStartupError(telemetry.CantWritePIDFile, err)
			log.Criticalf("Error writing PID file, exiting: %v", err)
			os.Exit(1)
		}

		log.Infof("PID '%d' written to PID file '%s'", os.Getpid(), cliParams.PIDFilePath)
		defer os.Remove(cliParams.PIDFilePath)
	}

	if err := util.SetupCoreDump(coreconfig.Datadog); err != nil {
		log.Warnf("Can't setup core dumps: %v, core dumps might not be available after a crash", err)
	}

	err = manager.ConfigureAutoExit(ctx, coreconfig.Datadog)
	if err != nil {
		telemetryCollector.SendStartupError(telemetry.CantSetupAutoExit, err)
		return fmt.Errorf("Unable to configure auto-exit, err: %v", err)
	}

	err = metrics.Configure(tracecfg, []string{"version:" + version.AgentVersion})
	if err != nil {
		telemetryCollector.SendStartupError(telemetry.CantConfigureDogstatsd, err)
		return fmt.Errorf("cannot configure dogstatsd: %v", err)
	}
	defer metrics.Flush()
	defer timing.Stop()

	metrics.Count("datadog.trace_agent.started", 1, nil, 1)

	rand.Seed(time.Now().UTC().UnixNano())

	remoteTagger := coreconfig.Datadog.GetBool("apm_config.remote_tagger")
	if remoteTagger {
		options, err := remote.NodeAgentOptions()
		if err != nil {
			log.Errorf("Unable to configure the remote tagger: %s", err)
			remoteTagger = false
		} else {
			tagger.SetDefaultTagger(remote.NewTagger(options))
			if err := tagger.Init(ctx); err != nil {
				log.Infof("Starting remote tagger failed. Falling back to local tagger: %s", err)
				remoteTagger = false
			}
		}
	}

	// starts the local tagger if apm_config says so, or if starting the
	// remote tagger has failed.
	if !remoteTagger {
		store := workloadmeta.CreateGlobalStore(workloadmeta.NodeAgentCatalog)
		store.Start(ctx)

		tagger.SetDefaultTagger(local.NewTagger(store))
		if err := tagger.Init(ctx); err != nil {
			log.Errorf("failed to start the tagger: %s", err)
		}
	}

	defer func() {
		err := tagger.Stop()
		if err != nil {
			log.Error(err)
		}
	}()

	if coreconfig.IsRemoteConfigEnabled(coreconfig.Datadog) {
		// Auth tokens are handled by the rcClient
		rcClient, err := rc.NewAgentGRPCConfigFetcher()
		if err != nil {
			telemetryCollector.SendStartupError(telemetry.CantCreateRCCLient, err)
			return fmt.Errorf("could not instantiate the tracer remote config client: %v", err)
		}
		api.AttachEndpoint(api.Endpoint{
			Pattern: "/v0.7/config",
			Handler: func(r *api.HTTPReceiver) http.Handler { return remotecfg.ConfigHandler(r, rcClient, tracecfg) },
		})
	}

	api.AttachEndpoint(api.Endpoint{
		Pattern: "/config/set",
		Handler: func(r *api.HTTPReceiver) http.Handler {
			return cfg.SetHandler()
		},
	})

	// prepare go runtime
	agentrt.SetMaxProcs()
	procs := runtime.GOMAXPROCS(0)
	if mp, ok := os.LookupEnv("GOMAXPROCS"); ok {
		log.Infof("GOMAXPROCS manually set to %v", mp)
	} else if tracecfg.MaxCPU > 0 {
		allowedCores := int(tracecfg.MaxCPU / 100)
		if allowedCores < 1 {
			allowedCores = 1
		}
		if allowedCores < procs {
			log.Infof("apm_config.max_cpu is less than current GOMAXPROCS. Setting GOMAXPROCS to (%v) %d\n", allowedCores, (allowedCores))
			runtime.GOMAXPROCS(int(allowedCores))
		}
	} else {
		log.Infof("apm_config.max_cpu is disabled. leaving GOMAXPROCS at current value.")
	}
	log.Infof("Trace Agent final GOMAXPROCS: %v", runtime.GOMAXPROCS(0))

	// prepare go runtime
	cgmem, err := agentrt.SetGoMemLimit(coreconfig.IsContainerized())
	if err != nil {
		log.Infof("Couldn't set Go memory limit from cgroup: %s", err)
	}

	if lim, ok := os.LookupEnv("GOMEMLIMIT"); ok {
		log.Infof("GOMEMLIMIT manually set to: %v", lim)
	} else if tracecfg.MaxMemory > 0 && (cgmem == 0 || int64(tracecfg.MaxMemory) < cgmem) {
		// We have a apm_config.max_memory that's lower than the cgroup limit.
		log.Infof("apm_config.max_memory: %vMiB", int64(tracecfg.MaxMemory)/(1024*1024))
		finalmem := int64(tracecfg.MaxMemory * 0.9)
		debug.SetMemoryLimit(finalmem)
		log.Infof("Maximum memory available: %vMiB. Setting GOMEMLIMIT to 90%% of max: %vMiB", int64(tracecfg.MaxMemory)/(1024*1024), finalmem/(1024*1024))
	} else if cgmem > 0 {
		// cgroup already constrained the memory
		log.Infof("Memory constrained by cgroup. Setting GOMEMLIMIT to: %vMiB", cgmem/(1024*1024))
	} else {
		// There are no memory constraints
		log.Infof("GOMEMLIMIT unconstrained.")
	}

	agnt := agent.NewAgent(ctx, tracecfg, telemetryCollector)
	log.Infof("Trace agent running on host %s", tracecfg.Hostname)
	if pcfg := profilingConfig(tracecfg); pcfg != nil {
		if err := profiling.Start(*pcfg); err != nil {
			log.Warn(err)
		} else {
			log.Infof("Internal profiling enabled: %s.", pcfg)
		}
		defer profiling.Stop()
	}
	go func() {
		time.Sleep(time.Second * 30)
		telemetryCollector.SendStartupSuccess()
	}()
	agnt.Run()

	// collect memory profile
	if cliParams.MemProfile != "" {
		f, err := os.Create(cliParams.MemProfile)
		if err != nil {
			log.Error("Could not create memory profile: ", err)
		}

		// get up-to-date statistics
		runtime.GC()
		// Not using WriteHeapProfile but instead calling WriteTo to
		// make sure we pass debug=1 and resolve pointers to names.
		if err := pprof.Lookup("heap").WriteTo(f, 1); err != nil {
			log.Error("Could not write memory profile: ", err)
		}
		f.Close()
	}

	return nil
}

type corelogger struct{}

// Trace implements Logger.
func (corelogger) Trace(v ...interface{}) { log.TraceStackDepth(stackDepth, v...) }

// Tracef implements Logger.
func (corelogger) Tracef(format string, params ...interface{}) {
	log.TracefStackDepth(stackDepth, format, params...)
}

// Debug implements Logger.
func (corelogger) Debug(v ...interface{}) { log.DebugStackDepth(stackDepth, v...) }

// Debugf implements Logger.
func (corelogger) Debugf(format string, params ...interface{}) {
	log.DebugfStackDepth(stackDepth, format, params...)
}

// Info implements Logger.
func (corelogger) Info(v ...interface{}) { log.InfoStackDepth(stackDepth, v...) }

// Infof implements Logger.
func (corelogger) Infof(format string, params ...interface{}) {
	log.InfofStackDepth(stackDepth, format, params...)
}

// Warn implements Logger.
func (corelogger) Warn(v ...interface{}) error { return log.WarnStackDepth(stackDepth, v...) }

// Warnf implements Logger.
func (corelogger) Warnf(format string, params ...interface{}) error {
	return log.WarnfStackDepth(stackDepth, format, params...)
}

// Error implements Logger.
func (corelogger) Error(v ...interface{}) error { return log.ErrorStackDepth(stackDepth, v...) }

// Errorf implements Logger.
func (corelogger) Errorf(format string, params ...interface{}) error {
	return log.ErrorfStackDepth(stackDepth, format, params...)
}

// Critical implements Logger.
func (corelogger) Critical(v ...interface{}) error { return log.CriticalStackDepth(stackDepth, v...) }

// Criticalf implements Logger.
func (corelogger) Criticalf(format string, params ...interface{}) error {
	return log.CriticalfStackDepth(stackDepth, format, params...)
}

// Flush implements Logger.
func (corelogger) Flush() { log.Flush() }

func profilingConfig(tracecfg *tracecfg.AgentConfig) *profiling.Settings {
	if !coreconfig.Datadog.GetBool("apm_config.internal_profiling.enabled") {
		return nil
	}
	endpoint := coreconfig.Datadog.GetString("internal_profiling.profile_dd_url")
	if endpoint == "" {
		endpoint = fmt.Sprintf(profiling.ProfilingURLTemplate, tracecfg.Site)
	}
	return &profiling.Settings{
		ProfilingURL: endpoint,

		// remaining configuration parameters use the top-level `internal_profiling` config
		Period:               coreconfig.Datadog.GetDuration("internal_profiling.period"),
		CPUDuration:          coreconfig.Datadog.GetDuration("internal_profiling.cpu_duration"),
		MutexProfileFraction: coreconfig.Datadog.GetInt("internal_profiling.mutex_profile_fraction"),
		BlockProfileRate:     coreconfig.Datadog.GetInt("internal_profiling.block_profile_rate"),
		WithGoroutineProfile: coreconfig.Datadog.GetBool("internal_profiling.enable_goroutine_stacktraces"),
		Tags:                 []string{fmt.Sprintf("version:%s", version.AgentVersion)},
	}
}
