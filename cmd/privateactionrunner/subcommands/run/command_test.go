// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package run

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestPrivateActionRunnerRunCommand(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		t.Setenv("DD_PRIVATE_ACTION_RUNNER_ENABLED", "false")

		// Test when PAR is disabled - should exit cleanly without calling fxutil.Run
		commands := Commands(newGlobalParamsTest(t, true))
		err := commands[0].RunE(nil, []string{"run"})
		require.NoError(t, err)
	})

	t.Run("enabled", func(t *testing.T) {
		unsetEnvForTest(t, "DD_PRIVATE_ACTION_RUNNER_ENABLED")

		// Test when PAR is enabled - should call fxutil.Run
		fxutil.TestRun(t, func() error {
			commands := Commands(newGlobalParamsTest(t, true))
			return commands[0].RunE(nil, []string{"run"})
		})
	})

	t.Run("env enables PAR", func(t *testing.T) {
		t.Setenv("DD_PRIVATE_ACTION_RUNNER_ENABLED", "true")

		fxutil.TestRun(t, func() error {
			commands := Commands(newGlobalParamsTest(t, false))
			return commands[0].RunE(nil, []string{"run"})
		})
	})

	t.Run("extra config enables PAR", func(t *testing.T) {
		unsetEnvForTest(t, "DD_PRIVATE_ACTION_RUNNER_ENABLED")
		params := newGlobalParamsWithExtraConfigTest(t, false, true)

		fxutil.TestRun(t, func() error {
			commands := Commands(params)
			return commands[0].RunE(nil, []string{"run"})
		})
	})

	t.Run("fleet policy config enables PAR", func(t *testing.T) {
		unsetEnvForTest(t, "DD_PRIVATE_ACTION_RUNNER_ENABLED")
		params := newGlobalParamsWithFleetPolicyConfigTest(t, false, true)

		fxutil.TestRun(t, func() error {
			commands := Commands(params)
			return commands[0].RunE(nil, []string{"run"})
		})
	})
}

func TestPrivateActionRunnerEnabled(t *testing.T) {
	t.Run("disabled from config", func(t *testing.T) {
		unsetEnvForTest(t, "DD_PRIVATE_ACTION_RUNNER_ENABLED")
		params := newGlobalParamsTest(t, false)

		enabled, err := privateActionRunnerEnabled(params.ConfFilePath, params.ExtraConfFilePath)
		require.NoError(t, err)
		require.False(t, enabled)
	})

	t.Run("disabled from env", func(t *testing.T) {
		t.Setenv("DD_PRIVATE_ACTION_RUNNER_ENABLED", "false")
		params := newGlobalParamsTest(t, true)

		enabled, err := privateActionRunnerEnabled(params.ConfFilePath, params.ExtraConfFilePath)
		require.NoError(t, err)
		require.False(t, enabled)
	})

	t.Run("enabled from env", func(t *testing.T) {
		t.Setenv("DD_PRIVATE_ACTION_RUNNER_ENABLED", "true")
		params := newGlobalParamsTest(t, false)

		enabled, err := privateActionRunnerEnabled(params.ConfFilePath, params.ExtraConfFilePath)
		require.NoError(t, err)
		require.True(t, enabled)
	})

	t.Run("extra config overrides main config", func(t *testing.T) {
		unsetEnvForTest(t, "DD_PRIVATE_ACTION_RUNNER_ENABLED")
		params := newGlobalParamsWithExtraConfigTest(t, false, true)

		enabled, err := privateActionRunnerEnabled(params.ConfFilePath, params.ExtraConfFilePath)
		require.NoError(t, err)
		require.True(t, enabled)
	})

	t.Run("fleet policy config overrides main config", func(t *testing.T) {
		unsetEnvForTest(t, "DD_PRIVATE_ACTION_RUNNER_ENABLED")
		params := newGlobalParamsWithFleetPolicyConfigTest(t, false, true)

		enabled, err := privateActionRunnerEnabled(params.ConfFilePath, params.ExtraConfFilePath)
		require.NoError(t, err)
		require.True(t, enabled)
	})

	t.Run("fleet policy config overrides env config", func(t *testing.T) {
		t.Setenv("DD_PRIVATE_ACTION_RUNNER_ENABLED", "false")
		params := newGlobalParamsWithFleetPolicyConfigTest(t, false, true)

		enabled, err := privateActionRunnerEnabled(params.ConfFilePath, params.ExtraConfFilePath)
		require.NoError(t, err)
		require.True(t, enabled)
	})
}

func newGlobalParamsTest(t *testing.T, enabled bool) *command.GlobalParams {
	// Create minimal config for private action runner testing
	configPath := filepath.Join(t.TempDir(), "datadog.yaml")
	writeConfig(t, configPath, enabled)

	return &command.GlobalParams{
		ConfFilePath: configPath,
	}
}

func newGlobalParamsWithExtraConfigTest(t *testing.T, mainEnabled bool, extraEnabled bool) *command.GlobalParams {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "datadog.yaml")
	extraConfigPath := filepath.Join(tmpDir, "extra.yaml")
	writeConfig(t, configPath, mainEnabled)
	writeConfig(t, extraConfigPath, extraEnabled)

	return &command.GlobalParams{
		ConfFilePath:      configPath,
		ExtraConfFilePath: []string{extraConfigPath},
	}
}

func newGlobalParamsWithFleetPolicyConfigTest(t *testing.T, mainEnabled bool, fleetEnabled bool) *command.GlobalParams {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "datadog.yaml")
	fleetPoliciesDirPath := filepath.Join(tmpDir, "fleet")
	require.NoError(t, os.Mkdir(fleetPoliciesDirPath, 0755))

	writeConfigWithExtra(t, configPath, mainEnabled, fmt.Sprintf("fleet_policies_dir: %s\n", fleetPoliciesDirPath))
	writeConfig(t, filepath.Join(fleetPoliciesDirPath, "datadog.yaml"), fleetEnabled)

	return &command.GlobalParams{
		ConfFilePath: configPath,
	}
}

func writeConfig(t *testing.T, configPath string, enabled bool) {
	writeConfigWithExtra(t, configPath, enabled, "")
}

func writeConfigWithExtra(t *testing.T, configPath string, enabled bool, extraConfig string) {
	configContent := `
hostname: test
private_action_runner:
  enabled: %v
  private_key: test_private_key
  urn: test_urn
api_key: test_key
%s`
	err := os.WriteFile(configPath, []byte(fmt.Sprintf(configContent, enabled, extraConfig)), 0644)
	require.NoError(t, err)
}

func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()

	value, ok := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if ok {
			require.NoError(t, os.Setenv(key, value))
			return
		}
		require.NoError(t, os.Unsetenv(key))
	})
}
