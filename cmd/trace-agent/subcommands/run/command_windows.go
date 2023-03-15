// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/runtime"
	tracecfg "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"

	"github.com/spf13/cobra"
)

var elog debug.Log

type RunParams struct {
	*subcommands.GlobalParams

	// PIDFilePath contains the value of the --pidfile flag.
	PIDFilePath string
	// CPUProfile contains the value for the --cpu-profile flag.
	CPUProfile string
	// MemProfile contains the value for the --mem-profile flag.
	MemProfile string
	// Foreground contains the value for the --foreground flag.
	Foreground bool
	// Debug contains the value for the --debug flag.
	Debug bool
}

func setOSSpecificParamFlags(cmd *cobra.Command, cliParams *RunParams) {
	cmd.PersistentFlags().BoolVarP(&cliParams.Foreground, "foreground", "f", false,
		"runs the agent in the foreground.")
	cmd.PersistentFlags().BoolVarP(&cliParams.Debug, "debug", "d", false,
		"runs the agent in debug mode.")
}

func Start(cliParams *RunParams, config config.Component) error {
	// Entrypoint here

	if !cliParams.Foreground {
		isIntSess, err := svc.IsAnInteractiveSession()
		if err != nil {
			fmt.Printf("failed to determine if we are running in an interactive session: %v\n", err)
		}
		if !isIntSess {
			runService(cliParams, config)
			return nil
		}
	}

	ctx, cancelFunc := context.WithCancel(context.Background())

	// prepare go runtime
	runtime.SetMaxProcs()

	// Handle stops properly
	go func() {
		defer watchdog.LogOnPanic()
		handleSignal(cancelFunc)
	}()

	// Invoke the Agent
	return runAgent(ctx, cliParams, config)
}

func runService(cliParams *RunParams, config config.Component) {
	var err error
	if cliParams.Debug {
		elog = debug.New(tracecfg.ServiceName)
	} else {
		elog, err = eventlog.Open(tracecfg.ServiceName)
		if err != nil {
			return
		}
	}
	defer elog.Close()

	run := svc.Run
	if cliParams.Debug {
		run = debug.Run
	}
	elog.Info(0x40000007, tracecfg.ServiceName)
	err = run(tracecfg.ServiceName, &myservice{
		cliParams: cliParams,
		config:    config,
	})

	if err != nil {
		elog.Error(0xc0000008, err.Error())
		return
	}
	elog.Info(0x40000004, tracecfg.ServiceName)
}
