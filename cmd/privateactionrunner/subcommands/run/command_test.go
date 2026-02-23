// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package run

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestPrivateActionRunnerRunCommand(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		// Test when PAR is disabled - should exit cleanly without calling fxutil.Run
		commands := Commands(newGlobalParamsTest(t, false))
		err := commands[0].RunE(nil, []string{"run"})
		require.NoError(t, err)
	})

	t.Run("enabled", func(t *testing.T) {
		// Test when PAR is enabled - should call fxutil.Run
		fxutil.TestRun(t, func() error {
			commands := Commands(newGlobalParamsTest(t, true))
			return commands[0].RunE(nil, []string{"run"})
		})
	})
}

func newGlobalParamsTest(t *testing.T, enabled bool) *command.GlobalParams {
	// Create minimal config for private action runner testing
	configPath := path.Join(t.TempDir(), "datadog.yaml")
	configContent := `
hostname: test
private_action_runner:
  enabled: %v
  private_key: test_private_key
  urn: test_urn
api_key: test_key
`
	err := os.WriteFile(configPath, []byte(fmt.Sprintf(configContent, enabled)), 0644)
	require.NoError(t, err)

	return &command.GlobalParams{
		ConfFilePath: configPath,
	}
}
