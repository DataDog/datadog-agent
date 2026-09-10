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

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// TestBootstrapDefaultsMirrorSchema guards the hand-maintained constants against
// drift. Bootstrap runs before the schema-backed defaults are loaded, so it cannot
// read them at runtime and has to duplicate them -- but a test can compare the two.
func TestBootstrapDefaultsMirrorSchema(t *testing.T) {
	cfg := configmock.New(t)

	require.Equal(t, cfg.GetBool("remote_agent.configstream.consumer.enabled"), defaultEnabled,
		"defaultEnabled no longer mirrors remote_agent.configstream.consumer.enabled in core_schema.yaml")
	require.Equal(t, cfg.GetBool("remote_agent.core_agent_ipc.enabled"), defaultCoreAgentIPCEnabled,
		"defaultCoreAgentIPCEnabled no longer mirrors remote_agent.core_agent_ipc.enabled in core_schema.yaml")
}

func TestIsEnabled(t *testing.T) {
	t.Run("false when env and yaml are empty", func(t *testing.T) {
		t.Setenv(enabledEnvVar, "")
		os.Unsetenv(enabledEnvVar)
		path := writeYAML(t, "")
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

	t.Run("missing yaml returns false", func(t *testing.T) {
		os.Unsetenv(enabledEnvVar)
		require.False(t, isEnabled("/does/not/exist/datadog.yaml"))
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

	t.Run("core agent IPC enabled falls through to the consumer default", func(t *testing.T) {
		os.Unsetenv(enabledEnvVar)
		os.Unsetenv(coreAgentIPCEnvVar)
		path := writeYAML(t, `
remote_agent:
  core_agent_ipc:
    enabled: true
`)
		// Enabling core agent IPC must not enable the consumer by itself: with
		// nothing else set it stays at its own default.
		require.False(t, isEnabled(path))
	})

	t.Run("core agent IPC enabled leaves an explicitly enabled consumer alone", func(t *testing.T) {
		os.Unsetenv(enabledEnvVar)
		os.Unsetenv(coreAgentIPCEnvVar)
		path := writeYAML(t, `
remote_agent:
  core_agent_ipc:
    enabled: true
  configstream:
    consumer:
      enabled: true
`)
		require.True(t, isEnabled(path))
	})
}
