// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !android

package main

/*
#include <Windows.h>

extern BOOL handleCtrlHandler(DWORD fdwCtrlType);

// The C control handler will call the Go control handler
static BOOL WINAPI CtrlHandler(DWORD fdwCtrlType)
{
    return handleCtrlHandler(fdwCtrlType);
}

// This method is called to hookup the console control handler
static void setupConsoleCtrlHandler() {
	SetConsoleCtrlHandler(CtrlHandler, TRUE);
}
*/
import "C"

import (
	_ "expvar"
	"fmt"
	_ "net/http/pprof"
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/app"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	"github.com/DataDog/datadog-agent/cmd/agent/windows/service"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows/svc"
)

// https://docs.microsoft.com/en-us/windows/console/handlerroutine
const (
	ctrlCEvent        = C.DWORD(0)
	ctrlBreakEvent    = C.DWORD(1)
	ctrlCloseEvent    = C.DWORD(2)
	ctrlLogOffEvent   = C.DWORD(5)
	ctrlShutdownEvent = C.DWORD(6)
)

//export handleCtrlHandler
func handleCtrlHandler(signal C.DWORD) C.BOOL {
	var sigStr string
	switch signal {
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
	log.Infof("Received signal '%s', shutting down...", sigStr)
	signals.Stopper <- true
	return 1
}

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
	C.setupConsoleCtrlHandler()

	// Invoke the Agent
	if err := app.AgentCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}
