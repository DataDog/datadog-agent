// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent defines the tracer agent.
package agent

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"sync"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const messageAgentDisabled = `trace-agent not enabled. Set the environment variable
DD_APM_ENABLED=true or add "apm_config.enabled: true" entry
to your datadog.yaml. Exiting...`

type dependencies struct {
	fx.In

	Lc         fx.Lifecycle
	Shutdowner fx.Shutdowner

	Config  config.Component
	Context context.Context
	Params  *Params
}

type agent struct {
	*pkgagent.Agent

	cancel             context.CancelFunc
	config             config.Component
	ctx                context.Context
	params             *Params
	shutdowner         fx.Shutdowner
	telemetryCollector telemetry.TelemetryCollector
	wg                 sync.WaitGroup
}

func newAgent(deps dependencies) Component {
	telemetryCollector := telemetry.NewCollector(deps.Config.Object())
	ctx, cancel := context.WithCancel(deps.Context) // Several related non-components require a shared context to gracefully stop.
	ag := &agent{
		Agent: pkgagent.NewAgent(
			ctx,
			deps.Config.Object(),
			telemetryCollector,
		),
		cancel:             cancel,
		config:             deps.Config,
		ctx:                ctx,
		params:             deps.Params,
		shutdowner:         deps.Shutdowner,
		telemetryCollector: telemetryCollector,
		wg:                 sync.WaitGroup{},
	}

	deps.Lc.Append(fx.Hook{OnStart: ag.start, OnStop: ag.stop})
	return ag
}

// Provided ctx has a timeout, so it can't be used for gracefully stopping long-running components.
// This context is cancelled on a deadline, so it would stop the agent after starting it.
func (ag *agent) start(_ context.Context) error {
	tracecfg := ag.config.Object()
	if !tracecfg.Enabled {
		log.Info(messageAgentDisabled)
		ag.telemetryCollector.SendStartupError(telemetry.TraceAgentNotEnabled, fmt.Errorf(""))
		return fmt.Errorf(messageAgentDisabled)
	}

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

func (ag *agent) stop(_ context.Context) error {
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
