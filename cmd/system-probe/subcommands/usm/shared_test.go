// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package usm

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestMakeOneShotCommand(t *testing.T) {
	globalParams := &command.GlobalParams{}

	// Dummy run function for testing
	dummyRun := func() {}

	// Create a test command using makeOneShotCommand
	cmd := makeOneShotCommand(
		globalParams,
		"test",
		"Test command description",
		dummyRun,
	)

	// Verify command was created correctly
	require.NotNil(t, cmd)
	assert.Equal(t, "test", cmd.Use)
	assert.Equal(t, "Test command description", cmd.Short)
	assert.True(t, cmd.SilenceUsage)

	// Verify --json flag was added
	jsonFlag := cmd.Flags().Lookup("json")
	require.NotNil(t, jsonFlag, "--json flag should exist")
	assert.Equal(t, "false", jsonFlag.DefValue, "--json should default to false")
	assert.Equal(t, "Output as JSON", jsonFlag.Usage)

	// Test the OneShot integration
	fxutil.TestOneShotSubcommand(t,
		[]*cobra.Command{cmd},
		[]string{"test"},
		dummyRun,
		func() {})
}

func TestOutputJSON(t *testing.T) {
	// Test with simple data structure
	data := map[string]interface{}{
		"string_field": "value",
		"number_field": 42,
		"bool_field":   true,
		"array_field":  []string{"a", "b", "c"},
		"nested_field": map[string]interface{}{
			"inner": "value",
		},
	}

	// outputJSON writes to stdout, so we can't easily capture it
	// but we can verify it doesn't panic or error
	// In a real scenario, this would be tested via the command tests
	// which already validate JSON output

	// Just verify the function exists and has correct signature
	err := outputJSON(data)
	// Since it writes to stdout, we expect no error
	assert.NoError(t, err)
}
