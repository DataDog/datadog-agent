// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package controlconfig

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	configsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/stretchr/testify/require"
)

func TestResolveUsesEffectiveAgentValues(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "datadog.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte("{}\n"), 0600))
	cfg := configmock.NewFromFile(t, configFile)
	cfg.SetInTest("dd_url", "https://resolved.example")
	cfg.SetInTest("private_action_runner.task_concurrency", 7)
	cfg.SetInTest("private_action_runner.executor.socket_path", "/run/resolved-executor.sock")
	cfg.SetInTest("private_action_runner.procmgr_socket_path", "/run/resolved-procmgr.sock")
	cfg.SetInTest("private_action_runner.executor_process_name", "resolved-executor")
	cfg.SetInTest("private_action_runner.idle_timeout_seconds", 71)
	cfg.SetInTest("private_action_runner.heartbeat_interval_seconds", 19)
	cfg.SetInTest("private_action_runner.opms_extra_headers", map[string]string{"X-Test": "resolved"})
	cfg.SetInTest("proxy.http", "http://resolved-http-proxy")
	cfg.SetInTest("proxy.https", "http://resolved-https-proxy")
	cfg.SetInTest("proxy.no_proxy", []string{"api.resolved.example"})
	cfg.SetInTest("no_proxy_nonexact_match", true)
	cfg.SetInTest("skip_ssl_validation", true)
	cfg.SetInTest("min_tls_version", "tlsv1.3")
	cfg.SetInTest("auth_token_file_path", "/run/datadog/auth_token")
	cfg.SetInTest("private_action_runner.urn", "resolved-urn")
	cfg.SetInTest("private_action_runner.private_key", "resolved-private-key")

	got := Resolve(cfg)

	require.Equal(t, SchemaVersion, got.SchemaVersion)
	require.Equal(t, "https://resolved.example", got.MainEndpoint)
	require.Equal(t, 7, got.TaskConcurrency)
	require.Equal(t, "/run/resolved-executor.sock", got.ExecutorSocket)
	require.Equal(t, "/run/resolved-procmgr.sock", got.ProcmgrSocket)
	require.Equal(t, "resolved-executor", got.ExecutorProcessName)
	require.Equal(t, 71, got.IdleTimeoutSeconds)
	require.Equal(t, 19, got.HeartbeatIntervalSeconds)
	require.Equal(t, map[string]string{"X-Test": "resolved"}, got.OPMSExtraHeaders)
	require.Equal(t, ProxyConfig{
		HTTP:    "http://resolved-http-proxy",
		HTTPS:   "http://resolved-https-proxy",
		NoProxy: []string{"api.resolved.example"},
	}, got.Proxy)
	require.True(t, got.NoProxyNonexactMatch)
	require.True(t, got.SkipSSLValidation)
	require.Equal(t, "tlsv1.3", got.MinTLSVersion)
	require.Equal(t, filepath.Join("/run/datadog", "ipc_cert.pem"), got.IPCCertFile)
	require.Equal(t, "resolved-urn", got.URN)
	require.Equal(t, "resolved-private-key", got.PrivateKey)
}

func TestResolveReceivesSecretBackendValues(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "datadog.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(`
secret_backend_command: test-resolver
private_action_runner:
  urn: urn:dd:apps:on-prem-runner:us1:42:resolved-runner
  private_key: ENC[private-key]
`), 0600))
	t.Setenv("DD_PROXY_HTTPS", "ENC[https-proxy]")

	cfg := configmock.New(t)
	cfg.SetConfigFile(configFile)
	resolver := secretsmock.New(t)
	resolver.SetSecrets(map[string]string{
		"private-key": "resolved-private-key",
		"https-proxy": "http://resolved-user:resolved-pass@proxy.example:3128",
	})
	require.NoError(t, configsetup.LoadDatadog(cfg, resolver, delegatedauthmock.New(t), nil))

	got := Resolve(cfg)

	require.Equal(t, "resolved-private-key", got.PrivateKey)
	require.Equal(t, "http://resolved-user:resolved-pass@proxy.example:3128", got.Proxy.HTTPS)
}

func TestResolveWithIdentityMatchesMonolithSelection(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "datadog.yaml")
	identityFile := filepath.Join(dir, "identity.json")
	require.NoError(t, os.WriteFile(configFile, []byte("{}\n"), 0600))
	require.NoError(t, os.WriteFile(identityFile, []byte(`{
		"private_key": "persisted-private-key",
		"urn": "urn:dd:apps:on-prem-runner:us1:42:persisted-runner",
		"hostname": "original-host"
	}`), 0600))
	cfg := configmock.NewFromFile(t, configFile)
	cfg.SetInTest("private_action_runner.identity_file_path", identityFile)

	got, err := ResolveWithIdentity(context.Background(), cfg, &enrollment.AgentIdentifier{Hostname: "original-host"})
	require.NoError(t, err)
	require.Equal(t, "persisted-private-key", got.PrivateKey)
	require.Equal(t, "urn:dd:apps:on-prem-runner:us1:42:persisted-runner", got.URN)

	stale, err := ResolveWithIdentity(context.Background(), cfg, &enrollment.AgentIdentifier{Hostname: "replacement-host"})
	require.NoError(t, err)
	require.Empty(t, stale.PrivateKey)
	require.Empty(t, stale.URN)
}

func TestResolvePrefersExplicitIPCCertPath(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "datadog.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte("{}\n"), 0600))
	cfg := configmock.NewFromFile(t, configFile)
	cfg.SetInTest("ipc_cert_file_path", "/custom/ipc.pem")

	got := Resolve(cfg)

	require.Equal(t, "/custom/ipc.pem", got.IPCCertFile)
}
