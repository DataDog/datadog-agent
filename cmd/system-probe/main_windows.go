// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/svc"

	"github.com/DataDog/datadog-agent/cmd/system-probe/app"
	"github.com/DataDog/datadog-agent/cmd/system-probe/windows/service"
)

func main() {
	// if command line arguments are supplied, even in a non interactive session,
	// then just execute that.  Used when the service is executing the executable,
	// for instance to trigger a restart.
	if len(os.Args) == 1 {
		isIntSess, err := svc.IsAnInteractiveSession()
		if err != nil {
			fmt.Printf("Failed to determine if we are running in an interactive session: %v\n", err)
		}
		if !isIntSess {
			service.RunService(false)
			return
		}
	}

	setDefaultCommandIfNonePresent()
	checkForDeprecatedFlags()
	if err := app.SysprobeCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
