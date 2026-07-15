// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package configstreamconsumerimpl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/configstreambootstrap"
)

const (
	envAuthTokenFilePath  = "DD_AUTH_TOKEN_FILE_PATH"
	envIPCCertFilePath    = "DD_IPC_CERT_FILE_PATH"
	envCmdHost            = "DD_CMD_HOST"
	envCmdPort            = "DD_CMD_PORT"
	envVSockAddr          = "DD_VSOCK_ADDR"
	envRARRegistryEnabled = "DD_REMOTE_AGENT_REGISTRY_ENABLED"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{envAuthTokenFilePath, envIPCCertFilePath, envCmdHost, envCmdPort, envVSockAddr, envRARRegistryEnabled} {
		t.Setenv(k, "")
		require.NoError(t, os.Unsetenv(k))
	}
}

func writeYAML(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "datadog.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0600))
	return path
}

func TestReadSettings(t *testing.T) {
	t.Run("defaults from common_settings when env+yaml empty", func(t *testing.T) {
		clearEnv(t)
		got := readSettings(writeYAML(t, ""))
		require.Equal(t, "localhost", got.CmdHost)
		require.Equal(t, 5001, got.CmdPort)
		require.Empty(t, got.AuthTokenFilePath)
		require.Empty(t, got.IPCCertFilePath)
		require.True(t, got.RARRegistryEnabled)
	})

	t.Run("yaml supplies all values", func(t *testing.T) {
		clearEnv(t)
		path := writeYAML(t, `
auth_token_file_path: /etc/dd/auth_token
ipc_cert_file_path: /etc/dd/ipc_cert.pem
cmd_host: 10.0.0.5
cmd_port: 9000
remote_agent:
  registry:
    enabled: true
`)
		got := readSettings(path)
		require.Equal(t, "/etc/dd/auth_token", got.AuthTokenFilePath)
		require.Equal(t, "/etc/dd/ipc_cert.pem", got.IPCCertFilePath)
		require.Equal(t, "10.0.0.5", got.CmdHost)
		require.Equal(t, 9000, got.CmdPort)
		require.True(t, got.RARRegistryEnabled)
	})

	t.Run("yaml overrides env", func(t *testing.T) {
		t.Setenv(envCmdHost, "192.168.1.1")
		t.Setenv(envCmdPort, "7000")
		configstreambootstrap.UseDynamicSchema(t)
		path := writeYAML(t, `
cmd_host: 10.0.0.5
cmd_port: 9000
remote_agent:
  registry:
    enabled: false
`)
		got := readSettings(path)
		require.Equal(t, "10.0.0.5", got.CmdHost)
		require.Equal(t, 9000, got.CmdPort)
		require.False(t, got.RARRegistryEnabled)
	})

	t.Run("env supplies value when yaml omits it", func(t *testing.T) {
		clearEnv(t)
		t.Setenv(envAuthTokenFilePath, "/env/auth")
		t.Setenv(envCmdHost, "192.168.1.1")
		configstreambootstrap.UseDynamicSchema(t)
		got := readSettings(writeYAML(t, ""))
		require.Equal(t, "/env/auth", got.AuthTokenFilePath)
		require.Equal(t, "192.168.1.1", got.CmdHost)
	})

	t.Run("malformed yaml outside our keys is tolerated", func(t *testing.T) {
		clearEnv(t)
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
		got := readSettings(path)
		require.Equal(t, "10.0.0.7", got.CmdHost)
		require.True(t, got.RARRegistryEnabled)
	})

	t.Run("yaml supplies vsock_addr", func(t *testing.T) {
		clearEnv(t)
		path := writeYAML(t, `
vsock_addr: vsock:2:5001
remote_agent:
  registry:
    enabled: true
`)
		got := readSettings(path)
		require.Equal(t, "vsock:2:5001", got.VSockAddr)
	})
}
