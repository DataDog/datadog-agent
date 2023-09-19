// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package agent

import (
	"context"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/trace/config"
	pkgagent "github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
)

type dependencies struct {
	fx.In

	Lc         fx.Lifecycle
	Shutdowner fx.Shutdowner

	Params             *Params
	Config             config.Component
	TelemetryCollector telemetry.TelemetryCollector
}

type agent struct {
	*pkgagent.Agent

	cancel             context.CancelFunc
	config             config.Component
	ctx                context.Context
	params             *Params
	shutter            fx.Shutdowner
	telemetryCollector telemetry.TelemetryCollector
}

func newAgent(deps dependencies) Component {
	// Several related non-components require a shared context to gracefully stop.
	ctx, cancel := context.WithCancel(context.Background())
	ag := &agent{
		Agent: pkgagent.NewAgent(
			ctx,
			deps.Config.Object(),
			deps.TelemetryCollector,
		),
		cancel:             cancel,
		config:             deps.Config,
		params:             deps.Params,
		shutter:            deps.Shutdowner,
		telemetryCollector: deps.TelemetryCollector,
	}

	deps.Lc.Append(fx.Hook{OnStart: ag.start, OnStop: ag.stop})

	return ag
}

// Provided ctx has a timeout, so it can't be used for gracefully stopping long-running components.
// This context is cancelled on a deadline, so it would stop the agent after starting it.
func (ag *agent) start(_ context.Context) error {
	// Handle stops properly
	go func() {
		defer watchdog.LogOnPanic()
		handleSignal(ag.shutter)
	}()

	if ag.params.CPUProfile != "" {
		f, err := os.Create(ag.params.CPUProfile)
		if err != nil {
			log.Error(err)
		}
		pprof.StartCPUProfile(f) //nolint:errcheck
		log.Info("CPU profiling started...")
		defer pprof.StopCPUProfile()
	}

	if ag.params.PIDFilePath != "" {
		err := pidfile.WritePID(ag.params.PIDFilePath)
		if err != nil {
			ag.telemetryCollector.SendStartupError(telemetry.CantWritePIDFile, err)
			log.Criticalf("Error writing PID file, exiting: %v", err)
			os.Exit(1)
		}

		log.Infof("PID '%d' written to PID file '%s'", os.Getpid(), ag.params.PIDFilePath)
		defer os.Remove(ag.params.PIDFilePath)
	}

	traceCfg := ag.config.Object()
	apiConfigHandler := ag.config.SetHandler()

	if err := runAgentSidekicks(ag.ctx, traceCfg, apiConfigHandler, ag.telemetryCollector); err != nil {
		return err
	}

	go ag.Run()

	return nil
}

func (ag *agent) stop(_ context.Context) error {
	ag.cancel()

	// collect memory profile
	if ag.params.MemProfile != "" {
		f, err := os.Create(ag.params.MemProfile)
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

// handleSignal closes a channel to exit cleanly from routines
func handleSignal(shutter fx.Shutdowner) {
	sigChan := make(chan os.Signal, 10)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGPIPE)
	for signo := range sigChan {
		switch signo {
		case syscall.SIGINT, syscall.SIGTERM:
			log.Infof("received signal %d (%v)", signo, signo)
			_ = shutter.Shutdown()
			return
		case syscall.SIGPIPE:
			// By default systemd redirects the stdout to journald. When journald is stopped or crashes we receive a SIGPIPE signal.
			// Go ignores SIGPIPE signals unless it is when stdout or stdout is closed, in this case the agent is stopped.
			// We never want the agent to stop upon receiving SIGPIPE, so we intercept the SIGPIPE signals and just discard them.
		default:
			_ = log.Warnf("unhandled signal %d (%v)", signo, signo)
		}
	}
}
