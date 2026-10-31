// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

const (
	coreAgentIPCEnvVar     = "DD_SYSTEM_PROBE_CONFIG_CORE_AGENT_IPC_ENABLED"
	fleetPoliciesDirEnvVar = "DD_FLEET_POLICIES_DIR"
)

// TestCoreAgentIPCMirrorsSchema guards the hand-maintained default against drift from
// the system-probe schema it duplicates.
func TestCoreAgentIPCMirrorsSchema(t *testing.T) {
	cfg := configmock.NewSystemProbe(t)
	require.Equal(t, cfg.GetBool(coreAgentIPCKey), defaultCoreAgentIPCEnabled,
		"defaultCoreAgentIPCEnabled no longer mirrors "+coreAgentIPCKey+" in system-probe_schema.yaml")
}

// unsetCoreAgentIPCEnv unsets the env vars for the duration of the test; t.Setenv
// registers the restore of any value inherited from the environment.
func unsetCoreAgentIPCEnv(t *testing.T) {
	for _, envVar := range []string{coreAgentIPCEnvVar, fleetPoliciesDirEnvVar} {
		t.Setenv(envVar, "")
		os.Unsetenv(envVar)
	}
}

func writeConf(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

const (
	disabledYAML       = "system_probe_config:\n  core_agent_ipc:\n    enabled: false\n"
	disabledDottedYAML = "system_probe_config.core_agent_ipc.enabled: false\n"
	disabledMixedYAML  = "system_probe_config:\n  core_agent_ipc.enabled: false\n"
)

func TestCoreAgentIPCEnabled(t *testing.T) {
	t.Run("defaults to enabled when nothing is set", func(t *testing.T) {
		unsetCoreAgentIPCEnv(t)
		require.Equal(t, defaultCoreAgentIPCEnabled, coreAgentIPCEnabled(writeConf(t, "system-probe.yaml", ""), ""))
	})

	t.Run("missing file falls back to the default", func(t *testing.T) {
		unsetCoreAgentIPCEnv(t)
		require.Equal(t, defaultCoreAgentIPCEnabled, coreAgentIPCEnabled("/does/not/exist/system-probe.yaml", ""))
	})

	for name, content := range map[string]string{
		"nested keys": disabledYAML,
		"dotted key":  disabledDottedYAML,
		"mixed keys":  disabledMixedYAML,
	} {
		t.Run("yaml file path with "+name, func(t *testing.T) {
			unsetCoreAgentIPCEnv(t)
			require.False(t, coreAgentIPCEnabled(writeConf(t, "custom.yaml", content), ""))
		})
	}

	t.Run("directory gets system-probe.yaml appended", func(t *testing.T) {
		unsetCoreAgentIPCEnv(t)
		path := writeConf(t, "system-probe.yaml", disabledYAML)
		require.False(t, coreAgentIPCEnabled(filepath.Dir(path), ""))
	})

	t.Run("env var wins over yaml", func(t *testing.T) {
		unsetCoreAgentIPCEnv(t)
		t.Setenv(coreAgentIPCEnvVar, "true")
		require.True(t, coreAgentIPCEnabled(writeConf(t, "system-probe.yaml", disabledYAML), ""))
	})

	t.Run("env var alone", func(t *testing.T) {
		unsetCoreAgentIPCEnv(t)
		t.Setenv(coreAgentIPCEnvVar, "false")
		require.False(t, coreAgentIPCEnabled(writeConf(t, "system-probe.yaml", ""), ""))
	})

	t.Run("fleet policy from the flag wins over env and yaml", func(t *testing.T) {
		unsetCoreAgentIPCEnv(t)
		t.Setenv(coreAgentIPCEnvVar, "true")
		fleetDir := filepath.Dir(writeConf(t, "system-probe.yaml", disabledYAML))
		local := writeConf(t, "system-probe.yaml", "system_probe_config:\n  core_agent_ipc:\n    enabled: true\n")
		require.False(t, coreAgentIPCEnabled(local, fleetDir))
	})

	t.Run("fleet policy with a dotted key", func(t *testing.T) {
		unsetCoreAgentIPCEnv(t)
		fleetDir := filepath.Dir(writeConf(t, "system-probe.yaml", disabledDottedYAML))
		require.False(t, coreAgentIPCEnabled(writeConf(t, "system-probe.yaml", ""), fleetDir))
	})

	t.Run("fleet policy dir from the env var", func(t *testing.T) {
		unsetCoreAgentIPCEnv(t)
		t.Setenv(fleetPoliciesDirEnvVar, filepath.Dir(writeConf(t, "system-probe.yaml", disabledYAML)))
		require.False(t, coreAgentIPCEnabled(writeConf(t, "system-probe.yaml", ""), ""))
	})

	t.Run("fleet policy dir from system-probe.yaml", func(t *testing.T) {
		unsetCoreAgentIPCEnv(t)
		fleetDir := filepath.Dir(writeConf(t, "system-probe.yaml", disabledYAML))
		require.False(t, coreAgentIPCEnabled(writeConf(t, "system-probe.yaml", "fleet_policies_dir: "+fleetDir+"\n"), ""))
	})

	t.Run("fleet policy without the key falls through", func(t *testing.T) {
		unsetCoreAgentIPCEnv(t)
		fleetDir := filepath.Dir(writeConf(t, "system-probe.yaml", "log_level: debug\n"))
		require.False(t, coreAgentIPCEnabled(writeConf(t, "system-probe.yaml", disabledYAML), fleetDir))
	})
}

// TestCoreAgentIPCEnabledMatchesConfigComponent checks the early read agrees with the
// value the system-probe config component ends up with, for every way of spelling it.
func TestCoreAgentIPCEnabledMatchesConfigComponent(t *testing.T) {
	for name, content := range map[string]string{
		"unset":       "",
		"nested keys": disabledYAML,
		"dotted key":  disabledDottedYAML,
		"mixed keys":  disabledMixedYAML,
	} {
		t.Run(name, func(t *testing.T) {
			unsetCoreAgentIPCEnv(t)
			path := writeConf(t, "system-probe.yaml", content)

			cfg := configmock.NewSystemProbe(t)
			cfg.SetConfigFile(path)
			require.NoError(t, cfg.ReadInConfig())

			require.Equal(t, cfg.GetBool(coreAgentIPCKey), coreAgentIPCEnabled(path, ""))
		})
	}
}
