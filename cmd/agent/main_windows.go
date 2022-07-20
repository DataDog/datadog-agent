// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !android
// +build !android

package main

import (
	_ "expvar"
	"fmt"
	_ "net/http/pprof"
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/app"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/windows/service"
	"github.com/DataDog/datadog-agent/pkg/traceinit"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows/svc"
)

func main() {
	traceinit.TraceFunction("MAIN 1 ")
	common.EnableLoggingToFile()
	traceinit.TraceFunction("MAIN 2 ")
	// if command line arguments are supplied, even in a non interactive session,
	// then just execute that.  Used when the service is executing the executable,
	// for instance to trigger a restart.
	if len(os.Args) == 1 {
		traceinit.TraceFunction("MAIN 3 ")
		isIntSess, err := svc.IsAnInteractiveSession()
		traceinit.TraceFunction("MAIN 4 ")
		if err != nil {
			fmt.Printf("failed to determine if we are running in an interactive session: %v\n", err)
		}
		if !isIntSess {
			traceinit.TraceFunction("MAIN 5 ")
			common.EnableLoggingToFile()
			traceinit.TraceFunction("MAIN 6 ")
			service.RunService(false)
			traceinit.TraceFunction("MAIN 7 ")
			return
		}
	}
	defer log.Flush()

	// Invoke the Agent
	if err := app.AgentCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}
