// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package start

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/runtime"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type CLIParams struct {
	ConfPath string
	PIDPath  string
}

// MakeCommand returns the start subcommand for the 'dogstatsd' command.
func MakeCommand(defaultLogFile string) *cobra.Command {
	cliParams := &CLIParams{}
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start Datadog trace-agent.",
		Long:  `Runs the trace-agent in the foreground`,
		RunE: func(*cobra.Command, []string) error {
			return RunTraceAgentFct(cliParams, "", defaultLogFile, Start)
		},
	}

	// local flags
	startCmd.PersistentFlags().StringVarP(&cliParams.ConfPath, "config", "c", "", "path to directory containing datadog.yaml")
	startCmd.PersistentFlags().StringVarP(&cliParams.PIDPath, "pid", "p", "", "path for the PID file to be created")

	return startCmd
}

type Params struct {
	DefaultLogFile string
}

func RunTraceAgentFct(cliParams *CLIParams, defaultConfPath string, defaultLogFile string, fct interface{}) error {
	params := &Params{
		DefaultLogFile: defaultLogFile,
	}
	return fxutil.OneShot(fct,
		fx.Supply(cliParams),
		fx.Supply(params),
		fx.Supply(config.NewParams(
			defaultConfPath,
			config.WithConfFilePath(cliParams.ConfPath),
			config.WithConfigLoadSecrets(false),
			config.WithConfigMissingOK(true),
			config.WithConfigName("trace-agent")),
		),
		config.Module,
	)
}

func Start(cliParams *CLIParams, config config.Component, params *Params) error {
	// Entrypoint here

	ctx, cancelFunc := context.WithCancel(context.Background())

	// prepare go runtime
	runtime.SetMaxProcs()
	if err := runtime.SetGoMemLimit(pkgconfig.IsContainerized()); err != nil {
		log.Debugf("Couldn't set Go memory limit: %s", err)
	}

	// Handle stops properly
	go func() {
		defer watchdog.LogOnPanic()
		handleSignal(cancelFunc)
	}()

	Run(ctx)

	return nil
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
