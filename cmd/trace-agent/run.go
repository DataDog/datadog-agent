// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/DataDog/datadog-agent/cmd/manager"
	"github.com/DataDog/datadog-agent/cmd/trace-agent/flags"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/tagger/remote"
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/config/features"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	tracelog "github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	// register all workloadmeta collectors
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors"
)

const messageAgentDisabled = `trace-agent not enabled. Set the environment variable
DD_APM_ENABLED=true or add "apm_config.enabled: true" entry
to your datadog.yaml. Exiting...`

// Run is the entrypoint of our code, which starts the agent.
func Run(ctx context.Context) {
	if flags.Version {
		fmt.Print(info.VersionString())
		return
	}

	cfg, err := loadConfigFile(flags.ConfigPath)
	if err != nil {
		fmt.Println(err) // TODO: remove me
		if err == config.ErrMissingAPIKey {
			fmt.Println(config.ErrMissingAPIKey)

			// a sleep is necessary to ensure that supervisor registers this process as "STARTED"
			// If the exit is "too quick", we enter a BACKOFF->FATAL loop even though this is an expected exit
			// http://supervisord.org/subprocess.html#process-states
			time.Sleep(5 * time.Second)

			// Don't use os.Exit() method here, even with os.Exit(0) the Service Control Manager
			// on Windows will consider the process failed and log an error in the Event Viewer and
			// attempt to restart the process.
			return
		}
		Exitf("%v", err)
	}
	err = info.InitInfo(cfg) // for expvar & -info option
	if err != nil {
		Exitf("%v", err)
	}

	if flags.Info {
		if err := info.Info(os.Stdout, cfg); err != nil {
			Exitf("Failed to print info: %s", err)
		}
		return
	}

	if err := coreconfig.SetupLogger(
		coreconfig.LoggerName("TRACE"),
		cfg.LogLevel,
		cfg.LogFilePath,
		coreconfig.GetSyslogURI(),
		coreconfig.Datadog.GetBool("syslog_rfc"),
		coreconfig.Datadog.GetBool("log_to_console"),
		coreconfig.Datadog.GetBool("log_format_json"),
	); err != nil {
		Exitf("Cannot create logger: %v", err)
	}
	tracelog.SetLogger(corelogger{})
	defer log.Flush()

	if !cfg.Enabled {
		log.Info(messageAgentDisabled)

		// a sleep is necessary to ensure that supervisor registers this process as "STARTED"
		// If the exit is "too quick", we enter a BACKOFF->FATAL loop even though this is an expected exit
		// http://supervisord.org/subprocess.html#process-states
		time.Sleep(5 * time.Second)
		return
	}

	defer watchdog.LogOnPanic()

	if flags.CPUProfile != "" {
		f, err := os.Create(flags.CPUProfile)
		if err != nil {
			log.Error(err)
		}
		pprof.StartCPUProfile(f)
		log.Info("CPU profiling started...")
		defer pprof.StopCPUProfile()
	}

	if flags.PIDFilePath != "" {
		err := pidfile.WritePID(flags.PIDFilePath)
		if err != nil {
			log.Criticalf("Error writing PID file, exiting: %v", err)
			os.Exit(1)
		}

		log.Infof("PID '%d' written to PID file '%s'", os.Getpid(), flags.PIDFilePath)
		defer os.Remove(flags.PIDFilePath)
	}

	if err := util.SetupCoreDump(); err != nil {
		log.Warnf("Can't setup core dumps: %v, core dumps might not be available after a crash", err)
	}

	err = manager.ConfigureAutoExit(ctx)
	if err != nil {
		Exitf("Unable to configure auto-exit, err: %v", err)
		return
	}

	err = metrics.Configure(cfg, []string{"version:" + info.Version})
	if err != nil {
		Exitf("cannot configure dogstatsd: %v", err)
	}
	defer metrics.Flush()
	defer timing.Stop()

	metrics.Count("datadog.trace_agent.started", 1, nil, 1)

	rand.Seed(time.Now().UTC().UnixNano())

	remoteTagger := coreconfig.Datadog.GetBool("apm_config.remote_tagger")
	if remoteTagger {
		tagger.SetDefaultTagger(remote.NewTagger())
		if err := tagger.Init(); err != nil {
			log.Infof("starting remote tagger failed. falling back to local tagger: %s", err)
			remoteTagger = false
		}
	}

	// starts the local tagger if apm_config says so, or if starting the
	// remote tagger has failed.
	if !remoteTagger {
		// Start workload metadata store before tagger
		workloadmeta.GetGlobalStore().Start(context.Background())

		tagger.SetDefaultTagger(local.NewTagger(collectors.DefaultCatalog))
		if err := tagger.Init(); err != nil {
			log.Errorf("failed to start the tagger: %s", err)
		}
	}

	defer func() {
		err := tagger.Stop()
		if err != nil {
			log.Error(err)
		}
	}()

	if features.Has("config_endpoint") {
		client, err := grpc.GetDDAgentSecureClient(context.Background())
		if err != nil {
			Exitf("could not instantiate the tracer remote config client: %v", err)
		}
		token, err := security.FetchAuthToken()
		if err != nil {
			Exitf("could obtain the auth token for the tracer remote config client: %v", err)
		}
		api.AttachEndpoint(api.Endpoint{
			Pattern: "/v0.7/config",
			Handler: func(r *api.HTTPReceiver) http.Handler { return remoteConfigHandler(r, client, token) },
		})
	}

	agnt := agent.NewAgent(ctx, cfg)
	log.Infof("Trace agent running on host %s", cfg.Hostname)
	if cfg.ProfilingSettings != nil {
		cfg.ProfilingSettings.Tags = []string{fmt.Sprintf("version:%s", info.Version)}
		profiling.Start(*cfg.ProfilingSettings)
		log.Infof("Internal profiling enabled: %s.", cfg.ProfilingSettings)
		defer profiling.Stop()
	}
	agnt.Run()

	// collect memory profile
	if flags.MemProfile != "" {
		f, err := os.Create(flags.MemProfile)
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
}

type corelogger struct{}

// Trace implements Logger.
func (corelogger) Trace(v ...interface{}) { log.Trace(v...) }

// Tracef implements Logger.
func (corelogger) Tracef(format string, params ...interface{}) { log.Tracef(format, params...) }

// Debug implements Logger.
func (corelogger) Debug(v ...interface{}) { log.Debug(v...) }

// Debugf implements Logger.
func (corelogger) Debugf(format string, params ...interface{}) { log.Debugf(format, params...) }

// Info implements Logger.
func (corelogger) Info(v ...interface{}) { log.Info(v...) }

// Infof implements Logger.
func (corelogger) Infof(format string, params ...interface{}) { log.Infof(format, params...) }

// Warn implements Logger.
func (corelogger) Warn(v ...interface{}) error { return log.Warn(v...) }

// Warnf implements Logger.
func (corelogger) Warnf(format string, params ...interface{}) error {
	return log.Warnf(format, params...)
}

// Error implements Logger.
func (corelogger) Error(v ...interface{}) error { return log.Error(v...) }

// Errorf implements Logger.
func (corelogger) Errorf(format string, params ...interface{}) error {
	return log.Errorf(format, params...)
}

// Critical implements Logger.
func (corelogger) Critical(v ...interface{}) error { return log.Critical(v...) }

// Criticalf implements Logger.
func (corelogger) Criticalf(format string, params ...interface{}) error {
	return log.Criticalf(format, params...)
}

// Flush implements Logger.
func (corelogger) Flush() { log.Flush() }
