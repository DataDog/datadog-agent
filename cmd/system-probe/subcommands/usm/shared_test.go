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

	// Test the OneShot integration
	fxutil.TestOneShotSubcommand(t,
		[]*cobra.Command{cmd},
		[]string{"test"},
		dummyRun,
		func() {})
}
