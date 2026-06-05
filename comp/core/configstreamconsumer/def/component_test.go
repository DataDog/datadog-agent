// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package configstreamconsumer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeYAML(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "datadog.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0600))
	return path
}

func TestIsEnabled(t *testing.T) {
	t.Run("false when env and yaml are empty", func(t *testing.T) {
		t.Setenv(enabledEnvVar, "")
		os.Unsetenv(enabledEnvVar)
		path := writeYAML(t, "")
		require.False(t, IsEnabled(path))
	})

	t.Run("yaml enables the consumer", func(t *testing.T) {
		os.Unsetenv(enabledEnvVar)
		path := writeYAML(t, `
remote_agent:
  configstream:
    consumer:
      enabled: true
`)
		require.True(t, IsEnabled(path))
	})

	t.Run("env var overrides yaml", func(t *testing.T) {
		t.Setenv(enabledEnvVar, "true")
		path := writeYAML(t, `
remote_agent:
  configstream:
    consumer:
      enabled: false
`)
		require.True(t, IsEnabled(path))
	})

	t.Run("missing yaml returns false", func(t *testing.T) {
		os.Unsetenv(enabledEnvVar)
		require.False(t, IsEnabled("/does/not/exist/datadog.yaml"))
	})
}
