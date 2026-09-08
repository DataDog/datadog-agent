// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package configstreamconsumerimpl

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsEnabled(t *testing.T) {
	t.Run("defaults to enabled when env and yaml are empty", func(t *testing.T) {
		t.Setenv(enabledEnvVar, "")
		os.Unsetenv(enabledEnvVar)
		path := writeYAML(t, "")
		require.True(t, isEnabled(path))
	})

	t.Run("yaml disables the consumer", func(t *testing.T) {
		os.Unsetenv(enabledEnvVar)
		path := writeYAML(t, `
remote_agent:
  configstream:
    consumer:
      enabled: false
`)
		require.False(t, isEnabled(path))
	})

	t.Run("yaml enables the consumer", func(t *testing.T) {
		os.Unsetenv(enabledEnvVar)
		path := writeYAML(t, `
remote_agent:
  configstream:
    consumer:
      enabled: true
`)
		require.True(t, isEnabled(path))
	})

	t.Run("env var overrides yaml", func(t *testing.T) {
		t.Setenv(enabledEnvVar, "true")
		path := writeYAML(t, `
remote_agent:
  configstream:
    consumer:
      enabled: false
`)
		require.True(t, isEnabled(path))
	})

	t.Run("env var disables the default", func(t *testing.T) {
		t.Setenv(enabledEnvVar, "false")
		path := writeYAML(t, "")
		require.False(t, isEnabled(path))
	})

	t.Run("missing yaml falls back to the default", func(t *testing.T) {
		os.Unsetenv(enabledEnvVar)
		require.True(t, isEnabled("/does/not/exist/datadog.yaml"))
	})
}

func TestIsEnabledCoreAgentIPCDisabled(t *testing.T) {
	t.Run("yaml disabling core agent IPC disables the consumer", func(t *testing.T) {
		os.Unsetenv(enabledEnvVar)
		os.Unsetenv(coreAgentIPCEnvVar)
		path := writeYAML(t, `
remote_agent:
  core_agent_ipc:
    enabled: false
`)
		require.False(t, isEnabled(path))
	})

	t.Run("core agent IPC wins over an explicitly enabled consumer", func(t *testing.T) {
		os.Unsetenv(enabledEnvVar)
		os.Unsetenv(coreAgentIPCEnvVar)
		path := writeYAML(t, `
remote_agent:
  core_agent_ipc:
    enabled: false
  configstream:
    consumer:
      enabled: true
`)
		require.False(t, isEnabled(path))
	})

	t.Run("env var disabling core agent IPC disables the consumer", func(t *testing.T) {
		t.Setenv(enabledEnvVar, "true")
		t.Setenv(coreAgentIPCEnvVar, "false")
		path := writeYAML(t, "")
		require.False(t, isEnabled(path))
	})

	t.Run("core agent IPC enabled leaves the consumer default untouched", func(t *testing.T) {
		os.Unsetenv(enabledEnvVar)
		os.Unsetenv(coreAgentIPCEnvVar)
		path := writeYAML(t, `
remote_agent:
  core_agent_ipc:
    enabled: true
`)
		require.True(t, isEnabled(path))
	})
}
