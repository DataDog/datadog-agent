// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package command

import (
	"context"
	"os"
)

// UseWinParams is set to true when ran on Windows
const UseWinParams = false

// RootCmdRun is the main function to run the process agent
func RootCmdRun(globalParams *GlobalParams) {
	// Invoke the Agent
	err := runAgent(context.Background(), globalParams)
	if err != nil {
		// For compatibility with the previous cleanupAndExitHandler implementation, os.Exit() on error.
		// This prevents runcmd.Run() from displaying the error.
		os.Exit(1)
	}
}
