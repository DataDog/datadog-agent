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

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/runtime"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MakeCommand returns the start subcommand for the 'trace-agent' command.
func MakeCommand(globalParamsGetter func() *subcommands.GlobalParams) *cobra.Command {

	cliParams := &RunParams{}
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Start datadog trace-agent.",
		Long: `
The Datadog trace-agent aggregates, samples, and forwards traces to datadog
submitted by tracers loaded into your application.`,
		RunE: func(*cobra.Command, []string) error {
			cliParams.GlobalParams = globalParamsGetter()
			return RunTraceAgentFct(cliParams, "./bin/agent/dist/datadog.yaml", Start)
		},
	}

	runCmd.PersistentFlags().StringVarP(&cliParams.PIDFilePath, "pidfile", "p", "", "path for the PID file to be created")
	runCmd.PersistentFlags().StringVarP(&cliParams.CPUProfile, "cpu-profile", "f", "",
		"enables CPU profiling and specifies profile path.")
	runCmd.PersistentFlags().StringVarP(&cliParams.MemProfile, "mem-profile", "m", "",
		"enables memory profiling and specifies profilh.")

	return runCmd
}

type Params struct {
	DefaultLogFile string
}

func RunTraceAgentFct(cliParams *RunParams, defaultConfPath string, fct interface{}) error {
	if cliParams.ConfPath == "" {
		cliParams.ConfPath = defaultConfPath
	}
	return fxutil.OneShot(fct,
		fx.Supply(cliParams),
		fx.Supply(config.NewParams(config.WithTraceConfFilePath(cliParams.ConfPath))),
		config.Module,
		// fx.Supply(coreconfig.NewAgentParamsWithSecrets(cliParams.ConfPath)),
		// coreconfig.Module,
	)
}

func Start(cliParams *RunParams, config config.Component) error {
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

	runAgent(ctx, cliParams, config)

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
