// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !windows

package main

import (
	"context"
	"os"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/flags"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"

	"github.com/spf13/cobra"
)

var ()

// Start the main loop
func run(cmd *cobra.Command, args []string) error {
	ctx, cancelFunc := context.WithCancel(cmd.Context())

	// Handle stops properly
	go func() {
		defer watchdog.LogOnPanic()
		handleSignal(cancelFunc)
	}()

	agent.Run(ctx)

	return nil
}

// main is the main application entry point
func main() {
	// set the command
	flags.TraceCmd.RunE = run

	// Invoke the Agent
	if err := flags.TraceCmd.Execute(); err != nil {
		os.Exit(-1)
	}

}
