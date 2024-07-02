// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package main is the entry point for the run command
package main

import (
	"os"

	"github.com/spf13/cobra"

	runCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/cmd/run/common"
	agentCmd "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/cmd/run/subcommands/agent"

	"testing"
)

func main() {
	// for testing.T
	testing.Init()

	var rootCmd = &cobra.Command{
		Use:   "run",
		Short: "Run E2E functions against a remote host",
		// Hide usage when a command returns an error
		SilenceUsage: true,
	}

	runCommon.Init(rootCmd)
	Init(rootCmd)
	agentCmd.Init(rootCmd)

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
