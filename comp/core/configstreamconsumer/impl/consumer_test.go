// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package configstreamconsumerimpl

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/pkg/api/security/cert"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/configstreambootstrap"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
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

func TestPbValueToGoKeepsWireType(t *testing.T) {
	// Narrowing is the declared default type's job, not this function's.
	require.Nil(t, pbValueToGo(nil))
	require.Equal(t, float64(5), pbValueToGo(structpb.NewNumberValue(5)))
	require.Equal(t, float64(5.5), pbValueToGo(structpb.NewNumberValue(5.5)))
	require.Equal(t, "s", pbValueToGo(structpb.NewStringValue("s")))
	require.Equal(t, true, pbValueToGo(structpb.NewBoolValue(true)))
}

// writeIPCCert produces a real IPC certificate file, in the cert+key PEM layout the loader expects.
func writeIPCCert(t *testing.T, path string) {
	t.Helper()
	cfg := configmock.New(t)
	cfg.SetInTest("ipc_cert_file_path", path)
	cfg.SetInTest("cluster_trust_chain.ca_cert_file_path", "")
	cfg.SetInTest("cluster_trust_chain.ca_key_file_path", "")
	cfg.SetInTest("cluster_trust_chain.enable_tls_verification", false)
	_, _, _, err := cert.FetchOrCreateIPCCert(context.Background(), cfg)
	require.NoError(t, err)
}

func TestLoadIPCCredentialsWaitsForCoreAgent(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "auth_token")
	certPath := filepath.Join(dir, "ipc_cert.pem")

	// The core agent writes both artifacts only after the consumer has started waiting, which is
	// the container case this retry exists for.
	writeIPCCert(t, filepath.Join(dir, "staged_cert.pem"))
	staged, err := os.ReadFile(filepath.Join(dir, "staged_cert.pem"))
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(100 * time.Millisecond)
		_ = os.WriteFile(tokenPath, []byte("0123456789abcdef0123456789abcdef"), 0600)
		_ = os.WriteFile(certPath, staged, 0600)
	}()

	authToken, clientTLS, err := loadIPCCredentials(tokenPath, certPath, 10*time.Second, 10*time.Millisecond, pkglog.NewWrapper(2))
	<-done
	require.NoError(t, err)
	require.Equal(t, "0123456789abcdef0123456789abcdef", authToken)
	require.NotNil(t, clientTLS)
}

func TestLoadIPCCredentialsTimesOut(t *testing.T) {
	dir := t.TempDir()
	timeout := 100 * time.Millisecond

	start := time.Now()
	_, _, err := loadIPCCredentials(
		filepath.Join(dir, "absent_token"), filepath.Join(dir, "absent_cert.pem"),
		timeout, 10*time.Millisecond, pkglog.NewWrapper(2),
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "load IPC credentials")
	// It waited rather than failing on the first read, which is what got containers restarted.
	require.GreaterOrEqual(t, time.Since(start), timeout)
}
