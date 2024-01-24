// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent defines the tracer agent.
package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"sync"
	"syscall"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const messageAgentDisabled = `trace-agent not enabled. Set the environment variable
DD_APM_ENABLED=true or add "apm_config.enabled: true" entry
to your datadog.yaml. Exiting...`

// ErrAgentDisabled indicates that the trace-agent wasn't enabled through environment variable or config.
var ErrAgentDisabled = errors.New(messageAgentDisabled)

type dependencies struct {
	fx.In

	Lc         fx.Lifecycle
	Shutdowner fx.Shutdowner

	Config             config.Component
	Context            context.Context
	Params             *Params
	TelemetryCollector telemetry.TelemetryCollector
	Workloadmeta       workloadmeta.Component
	Statsd             statsd.Component
	Tagger             tagger.Component
}

type component struct{}

type agent struct {
	*pkgagent.Agent

	cancel             context.CancelFunc
	config             config.Component
	ctx                context.Context
	params             *Params
	shutdowner         fx.Shutdowner
	tagger             tagger.Component
	telemetryCollector telemetry.TelemetryCollector
	statsd             statsd.Component
	workloadmeta       workloadmeta.Component
	wg                 sync.WaitGroup
}

func newAgent(deps dependencies) Component {
	c := component{}
	tracecfg := deps.Config.Object()
	if !tracecfg.Enabled {
		log.Info(messageAgentDisabled)
		deps.TelemetryCollector.SendStartupError(telemetry.TraceAgentNotEnabled, fmt.Errorf(""))
		// Required to signal that the whole app must stop.
		_ = deps.Shutdowner.Shutdown()
		return c
	}
	ctx, cancel := context.WithCancel(deps.Context) // Several related non-components require a shared context to gracefully stop.
	ag := &agent{
		Agent: pkgagent.NewAgent(
			ctx,
			deps.Config.Object(),
			deps.TelemetryCollector,
		),
		cancel:             cancel,
		config:             deps.Config,
		statsd:             deps.Statsd,
		ctx:                ctx,
		params:             deps.Params,
		shutdowner:         deps.Shutdowner,
		workloadmeta:       deps.Workloadmeta,
		telemetryCollector: deps.TelemetryCollector,
		tagger:             deps.Tagger,
		wg:                 sync.WaitGroup{},
	}
	deps.Lc.Append(fx.Hook{
		// Provided contexts have a timeout, so it can't be used for gracefully stopping long-running components.
		// These contexts are cancelled on a deadline, so they would have side effects on the agent.
		OnStart: func(_ context.Context) error { return start(ag) },
		OnStop:  func(_ context.Context) error { return stop(ag) }})
	return c
}

func start(ag *agent) error {
	setupShutdown(ag.ctx, ag.shutdowner)
	if ag.params.CPUProfile != "" {
		f, err := os.Create(ag.params.CPUProfile)
		if err != nil {
			log.Error(err)
		}
		pprof.StartCPUProfile(f) //nolint:errcheck
		log.Info("CPU profiling started...")
	}
	if ag.params.PIDFilePath != "" {
		err := pidfile.WritePID(ag.params.PIDFilePath)
		if err != nil {
			ag.telemetryCollector.SendStartupError(telemetry.CantWritePIDFile, err)
			log.Criticalf("Error writing PID file, exiting: %v", err)
			os.Exit(1)
		}

		log.Infof("PID '%d' written to PID file '%s'", os.Getpid(), ag.params.PIDFilePath)
	}

	if err := setupMetrics(ag.statsd, ag.config, ag.telemetryCollector); err != nil {
		return err
	}

	if err := runAgentSidekicks(ag.ctx, ag.config, ag.telemetryCollector); err != nil {
		return err
	}
	ag.wg.Add(1)
	go func() {
		defer ag.wg.Done()
		ag.Run()
	}()
	return nil
}

func setupMetrics(statsd statsd.Component, cfg config.Component, telemetryCollector telemetry.TelemetryCollector) error {
	tracecfg := cfg.Object()

	// TODO: Try to use statsd.Get() everywhere instead in the long run.
	err := metrics.Configure(tracecfg, []string{"version:" + version.AgentVersion}, statsd.CreateForAddr)
	if err != nil {
		telemetryCollector.SendStartupError(telemetry.CantConfigureDogstatsd, err)
		return fmt.Errorf("cannot configure dogstatsd: %v", err)
	}

	metrics.Count("datadog.trace_agent.started", 1, nil, 1)
	return nil
}

func stop(ag *agent) error {
	ag.cancel()
	ag.wg.Wait()
	stopAgentSidekicks(ag.config)
	if ag.params.CPUProfile != "" {
		pprof.StopCPUProfile()
	}
	if ag.params.PIDFilePath != "" {
		os.Remove(ag.params.PIDFilePath)
	}
	if ag.params.MemProfile == "" {
		return nil
	}
	// prepare to collect memory profile
	f, err := os.Create(ag.params.MemProfile)
	if err != nil {
		log.Error("Could not create memory profile: ", err)
	}
	defer f.Close()

	// get up-to-date statistics
	runtime.GC()
	// Not using WriteHeapProfile but instead calling WriteTo to
	// make sure we pass debug=1 and resolve pointers to names.
	if err := pprof.Lookup("heap").WriteTo(f, 1); err != nil {
		log.Error("Could not write memory profile: ", err)
	}
	return nil
}

// handleSignal closes a channel to exit cleanly from routines
func handleSignal(shutdowner fx.Shutdowner) {
	defer watchdog.LogOnPanic()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGPIPE)
	for signo := range sigChan {
		switch signo {
		case syscall.SIGINT, syscall.SIGTERM:
			log.Infof("Received signal %d (%v)", signo, signo)
			_ = shutdowner.Shutdown()
			return
		case syscall.SIGPIPE:
			// By default systemd redirects the stdout to journald. When journald is stopped or crashes we receive a SIGPIPE signal.
			// Go ignores SIGPIPE signals unless it is when stdout or stdout is closed, in this case the agent is stopped.
			// We never want the agent to stop upon receiving SIGPIPE, so we intercept the SIGPIPE signals and just discard them.
		default:
			log.Warnf("Unhandled signal %d (%v)", signo, signo)
		}
	}
}
