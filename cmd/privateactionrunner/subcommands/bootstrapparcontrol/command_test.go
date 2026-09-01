// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package bootstrapparcontrol

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	parutil "github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBootstrapCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t, Commands(&command.GlobalParams{}), []string{"bootstrap-par-control"}, run, func() {})
}

func runBootstrap(t *testing.T, cfg coreconfig.Component, enroll enrollAndPersistFunc) (*ControlPlaneConfig, error) {
	t.Helper()
	hostnameComp, _ := hostnamemock.NewMock("test-host")
	var out bytes.Buffer
	if err := bootstrap(context.Background(), cfg, hostnameComp, enroll, &out); err != nil {
		return nil, err
	}
	var resolved ControlPlaneConfig
	require.NoError(t, json.Unmarshal(out.Bytes(), &resolved))
	return &resolved, nil
}

func splitConfig(t *testing.T, overrides map[string]interface{}) coreconfig.Component {
	values := map[string]interface{}{
		"private_action_runner.enabled":            true,
		"private_action_runner.split_enabled":      true,
		"private_action_runner.identity_file_path": filepath.Join(t.TempDir(), "identity.json"),
	}
	for key, value := range overrides {
		values[key] = value
	}
	return coreconfig.NewMockWithOverrides(t, values)
}

func TestControlPlaneConfigMatchesGoldenFixture(t *testing.T) {
	got, err := json.Marshal(&ControlPlaneConfig{
		SplitMode: true,
		LogLevel:  "debug",
		Identity: &Identity{
			URN:        "urn:dd:apps:on-prem-runner:us1:42:runner-1",
			PrivateKey: "encoded-jwk",
			OrgID:      42,
			RunnerID:   "runner-1",
		},
		OPMSBaseURL:      "https://api.datadoghq.com",
		OPMSProxyURL:     "http://secure-proxy.example:8443",
		AgentVersion:     "7.83.0",
		Modes:            []string{"pull"},
		TaskConcurrency:  5,
		ExecutorSocket:   "/opt/datadog-agent/run/par-executor.sock",
		IPCCertFilePath:  "/etc/datadog-agent/ipc_cert.pem",
		OPMSExtraHeaders: map[string]string{"X-Test-Routing": "canary"},
		TLS: &TLS{
			SkipSSLValidation: true,
			MinTLSVersion:     "tlsv1.3",
		},
		LoopIntervalMilliseconds:       1_000,
		HeartbeatIntervalMilliseconds:  20_000,
		HealthCheckIntervalMillisecond: 30_000,
		OPMSRequestTimeoutMilliseconds: 30_000,
		MinBackoffMilliseconds:         1_000,
		MaxBackoffMilliseconds:         180_000,
		WaitBeforeRetryMilliseconds:    300_000,
		MaxAttempts:                    20,
	})
	require.NoError(t, err)
	want, err := os.ReadFile("testdata/bootstrap_config.json")
	require.NoError(t, err)

	assert.Equal(t, string(want), string(got)+"\n")
}

func TestBootstrapSplitModeDisabled(t *testing.T) {
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"private_action_runner.enabled":       true,
		"private_action_runner.split_enabled": false,
		"log_level":                           "debug",
	})

	resolved, err := runBootstrap(t, cfg, failIfEnrolled(t))

	require.NoError(t, err)
	assert.False(t, resolved.SplitMode)
	assert.Equal(t, "debug", resolved.LogLevel)
	assert.Nil(t, resolved.Identity)
}

func TestBootstrapPersistedIdentityWins(t *testing.T) {
	cfg := splitConfig(t, map[string]interface{}{
		"private_action_runner.urn":         parutil.MakeRunnerURN("us5", 999, "inline-runner"),
		"private_action_runner.private_key": validPrivateKey(t),
	})
	writeIdentity(t, cfg, validURN(), validPrivateKey(t), "test-host")

	resolved, err := runBootstrap(t, cfg, failIfEnrolled(t))

	require.NoError(t, err)
	assert.Equal(t, validURN(), resolved.Identity.URN)
	assert.Equal(t, int64(123), resolved.Identity.OrgID)
}

func TestBootstrapUsesInlineIdentity(t *testing.T) {
	urn := parutil.MakeRunnerURN("us5", 999, "inline-runner")
	key := validPrivateKey(t)
	cfg := splitConfig(t, map[string]interface{}{
		"private_action_runner.urn":         urn,
		"private_action_runner.private_key": key,
	})

	resolved, err := runBootstrap(t, cfg, failIfEnrolled(t))

	require.NoError(t, err)
	assert.Equal(t, urn, resolved.Identity.URN)
	assert.Equal(t, key, resolved.Identity.PrivateKey)
}

func TestBootstrapSelfEnrolls(t *testing.T) {
	cfg := splitConfig(t, map[string]interface{}{"private_action_runner.self_enroll": true})
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	resolved, err := runBootstrap(t, cfg, func(ctx context.Context, cfg coreconfig.Component, _ *enrollment.AgentIdentifier) (*enrollment.Result, error) {
		result := &enrollment.Result{URN: validURN(), PrivateKey: privateKey, Hostname: "test-host"}
		return result, enrollment.PersistIdentity(ctx, cfg, result)
	})

	require.NoError(t, err)
	assert.Equal(t, validURN(), resolved.Identity.URN)
	assert.NotEmpty(t, resolved.Identity.PrivateKey)
}

func TestBootstrapResolvesConfig(t *testing.T) {
	cfg := splitConfig(t, map[string]interface{}{
		"private_action_runner.urn":                  validURN(),
		"private_action_runner.private_key":          validPrivateKey(t),
		"private_action_runner.executor.socket_path": "/tmp/executor.sock",
		"private_action_runner.task_concurrency":     7,
		"proxy.https":                                "http://proxy.example:8443",
		"skip_ssl_validation":                        true,
		"min_tls_version":                            "tlsv1.3",
	})

	resolved, err := runBootstrap(t, cfg, failIfEnrolled(t))

	require.NoError(t, err)
	assert.Equal(t, "http://proxy.example:8443", resolved.OPMSProxyURL)
	assert.Equal(t, "/tmp/executor.sock", resolved.ExecutorSocket)
	assert.Equal(t, int32(7), resolved.TaskConcurrency)
	assert.True(t, resolved.TLS.SkipSSLValidation)
	assert.Equal(t, "tlsv1.3", resolved.TLS.MinTLSVersion)
	assert.NotZero(t, resolved.LoopIntervalMilliseconds)
}

func TestBootstrapRejectsFIPS(t *testing.T) {
	cfg := splitConfig(t, map[string]interface{}{
		"private_action_runner.urn":         validURN(),
		"private_action_runner.private_key": validPrivateKey(t),
		"fips.enabled":                      true,
	})

	_, err := runBootstrap(t, cfg, failIfEnrolled(t))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "fips.enabled")
}

func TestBootstrapUsesDDURLForFakeintake(t *testing.T) {
	t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")
	cfg := splitConfig(t, map[string]interface{}{
		"private_action_runner.urn":         validURN(),
		"private_action_runner.private_key": validPrivateKey(t),
		"dd_url":                            "http://fakeintake.test:8080",
	})

	resolved, err := runBootstrap(t, cfg, failIfEnrolled(t))

	require.NoError(t, err)
	assert.Equal(t, "http://fakeintake.test:8080", resolved.OPMSBaseURL)
}

func writeIdentity(t *testing.T, cfg coreconfig.Component, urn, key, hostname string) {
	t.Helper()
	result := &enrollment.Result{URN: urn, Hostname: hostname}
	jwk, err := parutil.Base64ToJWK(key)
	require.NoError(t, err)
	result.PrivateKey = jwk.Key.(*ecdsa.PrivateKey)
	require.NoError(t, enrollment.PersistIdentity(context.Background(), cfg, result))
}

func validURN() string {
	return parutil.MakeRunnerURN("us1", 123, "test-runner")
}

func validPrivateKey(t *testing.T) string {
	t.Helper()
	privateJWK, _, err := parutil.GenerateKeys()
	require.NoError(t, err)
	encoded, err := privateJWK.MarshalJSON()
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func failIfEnrolled(t *testing.T) enrollAndPersistFunc {
	return func(context.Context, coreconfig.Component, *enrollment.AgentIdentifier) (*enrollment.Result, error) {
		t.Fatal("unexpected enrollment")
		return nil, nil
	}
}
