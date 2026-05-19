// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configstreambootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func envFunc(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func writeYAML(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "datadog.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0600))
	return path
}

func TestIsEnabled(t *testing.T) {
	t.Run("false when env and yaml are empty", func(t *testing.T) {
		path := writeYAML(t, "")
		require.False(t, IsEnabled(path, envFunc(nil)))
	})

	t.Run("yaml enables the consumer", func(t *testing.T) {
		path := writeYAML(t, `
remote_agent:
  configstream:
    consumer:
      enabled: true
`)
		require.True(t, IsEnabled(path, envFunc(nil)))
	})

	t.Run("env var overrides yaml", func(t *testing.T) {
		path := writeYAML(t, `
remote_agent:
  configstream:
    consumer:
      enabled: false
`)
		env := envFunc(map[string]string{EnabledEnvVar: "true"})
		require.True(t, IsEnabled(path, env))
	})

	t.Run("missing yaml returns false", func(t *testing.T) {
		require.False(t, IsEnabled("/does/not/exist/datadog.yaml", envFunc(nil)))
	})
}

func TestReadSettings(t *testing.T) {
	t.Run("defaults when env and yaml are empty", func(t *testing.T) {
		path := writeYAML(t, "")
		got := readSettings(path, envFunc(nil))
		require.Equal(t, defaultCmdHost, got.CmdHost)
		require.Equal(t, defaultCmdPort, got.CmdPort)
		require.Empty(t, got.AuthTokenFilePath)
		require.Empty(t, got.IPCCertFilePath)
		require.True(t, got.RARRegistryEnabled)
	})

	t.Run("yaml supplies all five values", func(t *testing.T) {
		path := writeYAML(t, `
auth_token_file_path: /etc/dd/auth_token
ipc_cert_file_path: /etc/dd/ipc_cert.pem
cmd_host: 10.0.0.5
cmd_port: 9000
remote_agent:
  registry:
    enabled: true
`)
		got := readSettings(path, envFunc(nil))
		require.Equal(t, "/etc/dd/auth_token", got.AuthTokenFilePath)
		require.Equal(t, "/etc/dd/ipc_cert.pem", got.IPCCertFilePath)
		require.Equal(t, "10.0.0.5", got.CmdHost)
		require.Equal(t, 9000, got.CmdPort)
		require.True(t, got.RARRegistryEnabled)
	})

	t.Run("env vars override yaml", func(t *testing.T) {
		path := writeYAML(t, `
auth_token_file_path: /etc/dd/auth_token
ipc_cert_file_path: /etc/dd/ipc_cert.pem
cmd_host: 10.0.0.5
cmd_port: 9000
remote_agent:
  registry:
    enabled: false
`)
		env := envFunc(map[string]string{
			envAuthTokenFilePath:  "/env/auth",
			envIPCCertFilePath:    "/env/cert.pem",
			envCmdHost:            "192.168.1.1",
			envCmdPort:            "7000",
			envRARRegistryEnabled: "true",
		})
		got := readSettings(path, env)
		require.Equal(t, "/env/auth", got.AuthTokenFilePath)
		require.Equal(t, "/env/cert.pem", got.IPCCertFilePath)
		require.Equal(t, "192.168.1.1", got.CmdHost)
		require.Equal(t, 7000, got.CmdPort)
		require.True(t, got.RARRegistryEnabled)
	})

	t.Run("malformed yaml outside our keys is tolerated", func(t *testing.T) {
		path := writeYAML(t, `
cmd_host: 10.0.0.7
some_other_block:
  - this
  - is
  - fine
remote_agent:
  registry:
    enabled: true
`)
		got := readSettings(path, envFunc(nil))
		require.Equal(t, "10.0.0.7", got.CmdHost)
		require.True(t, got.RARRegistryEnabled)
	})
}

func TestRunFailsFastWhenRARDisabled(t *testing.T) {
	path := writeYAML(t, `
remote_agent:
  configstream:
    consumer:
      enabled: true
  registry:
    enabled: false
`)
	err := Run(context.Background(), Params{
		ClientName:    "test-agent",
		CLIConfigPath: path,
		LookupEnv:     envFunc(nil),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "remote_agent.registry.enabled=true")
}

func TestRunRequiresClientName(t *testing.T) {
	err := Run(context.Background(), Params{
		CLIConfigPath: writeYAML(t, ""),
		LookupEnv:     envFunc(nil),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "ClientName is required")
}
