// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/trace"
	"github.com/DataDog/datadog-agent/comp/trace/agent"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MakeCommand returns the run subcommand for the 'trace-agent' command.
func MakeCommand(globalParamsGetter func() *subcommands.GlobalParams) *cobra.Command {
	cliParams := &RunParams{}
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Start datadog trace-agent.",
		Long:  `The Datadog trace-agent aggregates, samples, and forwards traces to datadog submitted by tracers loaded into your application.`,
		RunE: func(*cobra.Command, []string) error {
			cliParams.GlobalParams = globalParamsGetter()
			return Run(cliParams, cliParams.ConfPath)
		},
	}

	setParamFlags(runCmd, cliParams)

	return runCmd
}

func setParamFlags(cmd *cobra.Command, cliParams *RunParams) {
	cmd.PersistentFlags().StringVarP(&cliParams.PIDFilePath, "pidfile", "p", "", "path for the PID file to be created")
	cmd.PersistentFlags().StringVarP(&cliParams.CPUProfile, "cpu-profile", "l", "",
		"enables CPU profiling and specifies profile path.")
	cmd.PersistentFlags().StringVarP(&cliParams.MemProfile, "mem-profile", "m", "",
		"enables memory profiling and specifies profile.")

	setOSSpecificParamFlags(cmd, cliParams)
}

func runFx(ctx context.Context, cliParams *RunParams, defaultConfPath string) error {
	if cliParams.ConfPath == "" {
		cliParams.ConfPath = defaultConfPath
	}
	return fxutil.OneShot(Start,
		fx.Supply(ctx),
		fx.Supply(cliParams),
		fx.Supply(coreconfig.NewAgentParamsWithSecrets(cliParams.ConfPath)),
		// TODO(AIT-8301): Test this injection
		fx.Provide(func(config config.Component) telemetry.TelemetryCollector {
			return telemetry.NewCollector(config.Object())
		}),
		coreconfig.Module,
		trace.Bundle,
	)
}

func Start(ctx context.Context, cliParams *RunParams, config config.Component, telemetryCollector telemetry.TelemetryCollector, ag agent.Component) error {
	// Main context passed to components
	ctx, cancelFunc := context.WithCancel(ctx)

	// Handle stops properly
	go func() {
		defer watchdog.LogOnPanic()
		handleSignal(cancelFunc)
	}()

	traceCfg := config.Object()
	apiConfigHandler := config.SetHandler()
	return runAgent(ctx, cliParams, traceCfg, apiConfigHandler, telemetryCollector, ag)
}

// handleSignal closes a channel to exit cleanly from routines
func handleSignal(onSignal func()) {
	sigChan := make(chan os.Signal, 10)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGPIPE)
	for signo := range sigChan {
		switch signo {
		case syscall.SIGINT, syscall.SIGTERM:
			log.Infof("received signal %d (%v)", signo, signo)
			onSignal()
			return
		case syscall.SIGPIPE:
			// By default systemd redirects the stdout to journald. When journald is stopped or crashes we receive a SIGPIPE signal.
			// Go ignores SIGPIPE signals unless it is when stdout or stdout is closed, in this case the agent is stopped.
			// We never want the agent to stop upon receiving SIGPIPE, so we intercept the SIGPIPE signals and just discard them.
		default:
			log.Warnf("unhandled signal %d (%v)", signo, signo)
		}
	}
}
