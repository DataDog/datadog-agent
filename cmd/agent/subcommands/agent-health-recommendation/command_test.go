// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package agenthealthrecommendation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestCommands(t *testing.T) {
	// Create a mock global params
	globalParams := &command.GlobalParams{
		ConfFilePath:      "test-config.yaml",
		ExtraConfFilePath: []string{},
	}

	// Get the commands
	commands := Commands(globalParams)

	// Verify we got exactly one command
	require.Len(t, commands, 1)

	cmd := commands[0]

	// Verify command properties
	assert.Equal(t, "agent-health-recommendation", cmd.Use)
	assert.Equal(t, "Run health checks from all subcomponents and display issues found", cmd.Short)
	assert.True(t, cmd.SilenceUsage)

	// Verify flags exist
	assert.NotNil(t, cmd.Flags().Lookup("verbose"))
	assert.NotNil(t, cmd.Flags().Lookup("json"))
	assert.NotNil(t, cmd.Flags().Lookup("severity"))
	assert.NotNil(t, cmd.Flags().Lookup("location"))
	assert.NotNil(t, cmd.Flags().Lookup("integration"))

	// Verify flag types
	verboseFlag := cmd.Flags().Lookup("verbose")
	assert.Equal(t, "bool", verboseFlag.Value.Type())

	jsonFlag := cmd.Flags().Lookup("json")
	assert.Equal(t, "bool", jsonFlag.Value.Type())

	severityFlag := cmd.Flags().Lookup("severity")
	assert.Equal(t, "string", severityFlag.Value.Type())

	locationFlag := cmd.Flags().Lookup("location")
	assert.Equal(t, "string", locationFlag.Value.Type())

	integrationFlag := cmd.Flags().Lookup("integration")
	assert.Equal(t, "string", integrationFlag.Value.Type())
}

func TestCommandExecution(t *testing.T) {
	// Create a mock global params
	globalParams := &command.GlobalParams{
		ConfFilePath:      "test-config.yaml",
		ExtraConfFilePath: []string{},
	}

	// Test the command execution using fxutil.TestOneShot
	// This will test that the fxutil.OneShot call in the command doesn't have missing dependencies
	fxutil.TestOneShot(t, func() {
		commands := Commands(globalParams)
		require.Len(t, commands, 1)

		// Execute the command to trigger fxutil.OneShot
		cmd := commands[0]
		cmd.SetArgs([]string{})
		_ = cmd.Execute()
	})
}
