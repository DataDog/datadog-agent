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
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows/svc"
)

var logFile *os.File

func writeLogFile(s string) {
	if logFile != nil {
		logFile.WriteString(s + "\n")
		logFile.Sync()
	}
}

func init() {
	var err error
	logFile, err = os.OpenFile(`C:\ProgramData\Datadog\logs\main.txt`, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0660)
	fmt.Println(err)
	writeLogFile("Init")
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			writeLogFile("Recovered")
			writeLogFile(fmt.Sprintf("%v", r))
		}
		logFile.Close()
	}()
	writeLogFile("Main")
	common.EnableLoggingToFile()
	writeLogFile("EnableLoggingToFile")
	// if command line arguments are supplied, even in a non interactive session,
	// then just execute that.  Used when the service is executing the executable,
	// for instance to trigger a restart.
	if len(os.Args) == 1 {
		writeLogFile("len(os.Args) == 1")
		isIntSess, err := svc.IsAnInteractiveSession()
		writeLogFile("IsAnInteractiveSession")
		if err != nil {
			fmt.Printf("failed to determine if we are running in an interactive session: %v\n", err)
		}
		if !isIntSess {
			writeLogFile("!isIntSess")
			common.EnableLoggingToFile()
			writeLogFile("EnableLoggingToFile")
			service.RunService(false)
			writeLogFile("RunService")
			return
		}
		writeLogFile("return")
	}
	writeLogFile("No service")
	defer log.Flush()

	// Invoke the Agent
	if err := app.AgentCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}
