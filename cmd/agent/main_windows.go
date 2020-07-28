// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !android

package main

import (
	_ "expvar"
	"fmt"
	_ "net/http/pprof"
	"os"
	"syscall"

	"github.com/DataDog/datadog-agent/cmd/agent/app"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/windows/service"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows/svc"
)

// https://docs.microsoft.com/en-us/windows/console/handlerroutine
const (
	ctrlCEvent        = uint(0)
	ctrlBreakEvent    = uint(1)
	ctrlCloseEvent    = uint(2)
	ctrlLogOffEvent   = uint(5)
	ctrlShutdownEvent = uint(6)
)

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	setConsoleCtrlHandler = kernel32.NewProc("SetConsoleCtrlHandler")
)

func main() {
	common.EnableLoggingToFile()
	// if command line arguments are supplied, even in a non interactive session,
	// then just execute that.  Used when the service is executing the executable,
	// for instance to trigger a restart.
	if len(os.Args) == 1 {
		isIntSess, err := svc.IsAnInteractiveSession()
		if err != nil {
			fmt.Printf("failed to determine if we are running in an interactive session: %v\n", err)
		}
		if !isIntSess {
			common.EnableLoggingToFile()
			service.RunService(false)
			return
		}
	}
	defer log.Flush()
	setConsoleCtrlHandler.Call(
		syscall.NewCallback(func(controlType uint) uint {
			var sigStr string
			switch controlType {
			case ctrlCEvent:
				sigStr = "CTRL+C"
			case ctrlBreakEvent:
				sigStr = "CTRL+BREAK"
			case ctrlCloseEvent:
				sigStr = "CTRL+CLOSE"
			case ctrlLogOffEvent:
				sigStr = "CTRL+LOG_OFF"
			case ctrlShutdownEvent:
				sigStr = "CTRL+SHUTDOWN"
			}
			log.Infof("Received control event '%s', shutting down...", sigStr)
			// signals.Stopper <- true
			// Completely bypass the run command stop logic as it takes too long to stop
			app.StopAgent()
			os.Exit(0)
			// We won't reach this code but the signature requires a return code.
			// Returning 1 in SetConsoleCtrlHandler means we handled the signal.
			return 1
		}), 1)

	// Invoke the Agent
	if err := app.AgentCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}
