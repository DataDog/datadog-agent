// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package main is the entrypoint for private-action-runner process
package main

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/subcommands"
)

func TestPrivateActionRunnerCommandCreation(t *testing.T) {
	// Test that the command can be created successfully
	rootCmd := command.MakeCommand(subcommands.PrivateActionRunnerSubcommands())
	require.NotNil(t, rootCmd)
	require.Equal(t, "datadog-private-action-runner [command]", rootCmd.Use)

	// Test that subcommands are properly registered
	subCommands := rootCmd.Commands()
	require.Greater(t, len(subCommands), 0, "Should have at least one subcommand")

	// Find the run command
	var runCmd *cobra.Command
	for _, cmd := range subCommands {
		if cmd.Use == "run" {
			runCmd = cmd
			break
		}
	}
	require.NotNil(t, runCmd, "Run command should be registered")
	require.Equal(t, "Run the Private Action Runner", runCmd.Short)
}
