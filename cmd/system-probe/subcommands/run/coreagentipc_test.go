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

const coreAgentIPCKey = "system_probe_config.core_agent_ipc.enabled"

// TestCoreAgentIPCMirrorsSchema guards the hand-maintained default and env var name
// against drift from the system-probe config they duplicate.
func TestCoreAgentIPCMirrorsSchema(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := configmock.NewSystemProbe(t)
		require.Equal(t, cfg.GetBool(coreAgentIPCKey), defaultCoreAgentIPCEnabled,
			"defaultCoreAgentIPCEnabled no longer mirrors "+coreAgentIPCKey+" in system-probe_schema.yaml")
	})

	t.Run("env var", func(t *testing.T) {
		t.Setenv(coreAgentIPCEnvVar, "false")
		cfg := configmock.NewSystemProbe(t)
		require.False(t, cfg.GetBool(coreAgentIPCKey),
			coreAgentIPCEnvVar+" is not the env var the system-probe config binds to "+coreAgentIPCKey)
	})
}

// unsetCoreAgentIPCEnv unsets the env var for the duration of the test; t.Setenv
// registers the restore of any value inherited from the environment.
func unsetCoreAgentIPCEnv(t *testing.T) {
	t.Setenv(coreAgentIPCEnvVar, "")
	os.Unsetenv(coreAgentIPCEnvVar)
}

func TestCoreAgentIPCEnabled(t *testing.T) {
	writeConf := func(t *testing.T, name, content string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), name)
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
		return path
	}
	disabledYAML := "system_probe_config:\n  core_agent_ipc:\n    enabled: false\n"

	t.Run("defaults to enabled when nothing is set", func(t *testing.T) {
		unsetCoreAgentIPCEnv(t)
		require.Equal(t, defaultCoreAgentIPCEnabled, coreAgentIPCEnabled(writeConf(t, "system-probe.yaml", "")))
	})

	t.Run("missing file falls back to the default", func(t *testing.T) {
		unsetCoreAgentIPCEnv(t)
		require.Equal(t, defaultCoreAgentIPCEnabled, coreAgentIPCEnabled("/does/not/exist/system-probe.yaml"))
	})

	t.Run("yaml file path", func(t *testing.T) {
		unsetCoreAgentIPCEnv(t)
		require.False(t, coreAgentIPCEnabled(writeConf(t, "custom.yaml", disabledYAML)))
	})

	t.Run("directory gets system-probe.yaml appended", func(t *testing.T) {
		unsetCoreAgentIPCEnv(t)
		path := writeConf(t, "system-probe.yaml", disabledYAML)
		require.False(t, coreAgentIPCEnabled(filepath.Dir(path)))
	})

	t.Run("env var wins over yaml", func(t *testing.T) {
		t.Setenv(coreAgentIPCEnvVar, "true")
		require.True(t, coreAgentIPCEnabled(writeConf(t, "system-probe.yaml", disabledYAML)))
	})

	t.Run("invalid env var falls through to yaml", func(t *testing.T) {
		t.Setenv(coreAgentIPCEnvVar, "not-a-bool")
		require.False(t, coreAgentIPCEnabled(writeConf(t, "system-probe.yaml", disabledYAML)))
	})
}
