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
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestPrivateActionRunnerRunCommand(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		commands := Commands(newGlobalParamsTest(t, false))
		err := commands[0].RunE(nil, []string{"run"})
		require.NoError(t, err)
	})

	t.Run("enabled", func(t *testing.T) {
		fxutil.TestRun(t, func() error {
			commands := Commands(newGlobalParamsTest(t, true))
			return commands[0].RunE(nil, []string{"run"})
		})
	})
}

func TestParEnabled(t *testing.T) {
	for _, tc := range []struct {
		name          string
		configEnabled bool
		envValue      string
		wantEnabled   bool
	}{
		{
			name:          "yaml disabled",
			configEnabled: false,
			wantEnabled:   false,
		},
		{
			name:          "yaml enabled",
			configEnabled: true,
			wantEnabled:   true,
		},
		{
			name:          "env enables PAR",
			configEnabled: false,
			envValue:      "true",
			wantEnabled:   true,
		},
		{
			name:          "env disables PAR",
			configEnabled: true,
			envValue:      "false",
			wantEnabled:   false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envValue != "" {
				t.Setenv("DD_PRIVATE_ACTION_RUNNER_ENABLED", tc.envValue)
			}
			assertParEnabled(t, writeTestConfig(t, tc.configEnabled), nil, tc.wantEnabled)
		})
	}

	t.Run("extra config file overrides main config", func(t *testing.T) {
		tmpDir := t.TempDir()
		mainPath := filepath.Join(tmpDir, "datadog.yaml")
		extraPath := filepath.Join(tmpDir, "extra.yaml")
		writeConfigAt(t, mainPath, false, "")
		writeConfigAt(t, extraPath, true, "")
		assertParEnabled(t, mainPath, []string{extraPath}, true)
	})

	t.Run("fleet policy overrides main config", func(t *testing.T) {
		mainPath, fleetDir := newConfigWithFleetDir(t, false)
		writeConfigAt(t, filepath.Join(fleetDir, "datadog.yaml"), true, "")
		assertParEnabled(t, mainPath, nil, true)
	})

	t.Run("fleet policy overrides env var", func(t *testing.T) {
		t.Setenv("DD_PRIVATE_ACTION_RUNNER_ENABLED", "false")
		mainPath, fleetDir := newConfigWithFleetDir(t, false)
		writeConfigAt(t, filepath.Join(fleetDir, "datadog.yaml"), true, "")
		assertParEnabled(t, mainPath, nil, true)
	})

	for _, tc := range []struct {
		name        string
		secretValue string
		wantEnabled bool
	}{
		{name: "secret backend enables PAR", secretValue: "true", wantEnabled: true},
		{name: "secret backend disables PAR", secretValue: "false", wantEnabled: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTestConfigValue(t, "ENC[par_enabled]", "secret_backend_command: test_secret_backend\n")
			resolver := secretsmock.New(t)
			resolver.SetSecrets(map[string]string{"par_enabled": tc.secretValue})

			enabled, err := parEnabledWithSecretResolver(path, nil, func() secrets.Component {
				return resolver
			})

			require.NoError(t, err)
			assert.Equal(t, tc.wantEnabled, enabled)
		})
	}

	t.Run("explicit confPath that does not exist is fatal", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")
		enabled, err := parEnabled(missing, nil)
		require.Error(t, err, "an explicit --cfgpath that doesn't exist must be surfaced, not silently treated as disabled")
		assert.False(t, enabled)
	})
}

func assertParEnabled(t *testing.T, confPath string, extraConfFiles []string, wantEnabled bool) {
	t.Helper()
	enabled, err := parEnabled(confPath, extraConfFiles)
	require.NoError(t, err)
	assert.Equal(t, wantEnabled, enabled)
}

func newGlobalParamsTest(t *testing.T, enabled bool) *command.GlobalParams {
	return &command.GlobalParams{
		ConfFilePath: writeTestConfig(t, enabled),
	}
}

func writeTestConfig(t *testing.T, enabled bool) string {
	configPath := filepath.Join(t.TempDir(), "datadog.yaml")
	writeConfigAt(t, configPath, enabled, "")
	return configPath
}

func writeTestConfigValue(t *testing.T, enabledValue string, extraTopLevel string) string {
	configPath := filepath.Join(t.TempDir(), "datadog.yaml")
	writeConfigValueAt(t, configPath, enabledValue, extraTopLevel)
	return configPath
}

func writeConfigAt(t *testing.T, configPath string, enabled bool, extraTopLevel string) {
	writeConfigValueAt(t, configPath, strconv.FormatBool(enabled), extraTopLevel)
}

func writeConfigValueAt(t *testing.T, configPath string, enabledValue string, extraTopLevel string) {
	configContent := `
hostname: test
private_action_runner:
  enabled: %s
  private_key: test_private_key
  urn: test_urn
api_key: test_key
%s`
	err := os.WriteFile(configPath, []byte(fmt.Sprintf(configContent, enabledValue, extraTopLevel)), 0644)
	require.NoError(t, err)
}

func newConfigWithFleetDir(t *testing.T, mainEnabled bool) (string, string) {
	tmpDir := t.TempDir()
	mainPath := filepath.Join(tmpDir, "datadog.yaml")
	fleetDir := filepath.Join(tmpDir, "fleet")
	require.NoError(t, os.Mkdir(fleetDir, 0755))
	writeConfigAt(t, mainPath, mainEnabled, fmt.Sprintf("fleet_policies_dir: %s\n", fleetDir))
	return mainPath, fleetDir
}
