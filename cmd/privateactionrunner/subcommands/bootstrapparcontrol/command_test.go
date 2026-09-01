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
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	parutil "github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestBootstrapCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"bootstrap-par-control"},
		run,
		func() {})
}

// splitConfig builds an enrolled split-mode config with a persisted identity.
func splitConfig(t *testing.T, overrides map[string]interface{}) coreconfig.Component {
	identityPath := filepath.Join(t.TempDir(), "identity.json")
	writeIdentity(t, identityPath, "test-host")
	values := map[string]interface{}{
		"private_action_runner.enabled":            true,
		"private_action_runner.split_enabled":      true,
		"private_action_runner.identity_file_path": identityPath,
	}
	for key, value := range overrides {
		values[key] = value
	}
	return coreconfig.NewMockWithOverrides(t, values)
}

func runBootstrap(t *testing.T, cfg coreconfig.Component) (*ControlPlaneConfig, string, error) {
	t.Helper()
	hostnameComp, _ := hostnamemock.NewMock("test-host")
	var out bytes.Buffer
	err := bootstrap(context.Background(), logmock.New(t), cfg, hostnameComp, failIfEnrolled(t), &out)
	if err != nil {
		return nil, out.String(), err
	}
	return parseEmitted(t, out.String()), out.String(), nil
}

func parseEmitted(t *testing.T, payload string) *ControlPlaneConfig {
	t.Helper()
	var resolved ControlPlaneConfig
	require.NoError(t, json.Unmarshal([]byte(payload), &resolved))
	return &resolved
}

func TestBootstrapSplitModeDisabled(t *testing.T) {
	// Every disabling combination must return the gate without enrolling, and
	// without tripping over FIPS mode or a missing identity.
	for name, overrides := range map[string]map[string]interface{}{
		"par disabled":         {"private_action_runner.enabled": false, "private_action_runner.split_enabled": true},
		"split disabled":       {"private_action_runner.enabled": true, "private_action_runner.split_enabled": false},
		"both disabled":        {"private_action_runner.enabled": false, "private_action_runner.split_enabled": false},
		"fips does not matter": {"private_action_runner.enabled": true, "private_action_runner.split_enabled": false, "fips.enabled": true},
		"no identity anywhere": {"private_action_runner.enabled": true, "private_action_runner.split_enabled": false},
		"self enroll disabled": {"private_action_runner.enabled": true, "private_action_runner.split_enabled": false, "private_action_runner.self_enroll": false},
	} {
		t.Run(name, func(t *testing.T) {
			overrides["log_level"] = "debug"
			cfg := coreconfig.NewMockWithOverrides(t, overrides)

			resolved, _, err := runBootstrap(t, cfg)

			require.NoError(t, err)
			assert.False(t, resolved.SplitMode)
			assert.Equal(t, "debug", resolved.LogLevel)
			// Nothing beyond the gate is resolved or leaked.
			assert.Nil(t, resolved.Identity)
			assert.Empty(t, resolved.OPMSProxyURL)
			assert.Nil(t, resolved.TLS)
			assert.Empty(t, resolved.OPMSBaseURL)
		})
	}
}

// The gate-only payload is a contract with par-control: it must carry the gate
// and nothing else, so a disabled runner never emits an identity object.
func TestBootstrapGateOnlyPayloadIsMinimal(t *testing.T) {
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"private_action_runner.enabled":       true,
		"private_action_runner.split_enabled": false,
		"log_level":                           "warn",
	})
	hostnameComp, _ := hostnamemock.NewMock("test-host")
	var out bytes.Buffer
	require.NoError(t, bootstrap(context.Background(), logmock.New(t), cfg, hostnameComp, failIfEnrolled(t), &out))

	var keys map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out.Bytes(), &keys))

	assert.ElementsMatch(t, []string{"split_mode", "log_level"}, mapKeys(keys))
}

func mapKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

func TestBootstrapRejectsFIPSMode(t *testing.T) {
	cfg := splitConfig(t, map[string]interface{}{"fips.enabled": true})

	_, _, err := runBootstrap(t, cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "fips.enabled")
}

// A Gov site is not FIPS mode and must keep working in split mode.
func TestBootstrapAllowsGovSite(t *testing.T) {
	cfg := splitConfig(t, map[string]interface{}{"site": "ddog-gov.com"})

	resolved, _, err := runBootstrap(t, cfg)

	require.NoError(t, err)
	assert.True(t, resolved.SplitMode)
	assert.Equal(t, "https://api.ddog-gov.com", resolved.OPMSBaseURL)
}

func TestBootstrapPersistedIdentityWins(t *testing.T) {
	identityPath := filepath.Join(t.TempDir(), "identity.json")
	writeIdentity(t, identityPath, "test-host")
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"private_action_runner.enabled":            true,
		"private_action_runner.split_enabled":      true,
		"private_action_runner.identity_file_path": identityPath,
		"private_action_runner.urn":                parutil.MakeRunnerURN("us5", 999, "inline-runner"),
		"private_action_runner.private_key":        validPrivateKey(t),
	})

	resolved, _, err := runBootstrap(t, cfg)

	require.NoError(t, err)
	assert.Equal(t, validURN(), resolved.Identity.URN)
	assert.Equal(t, int64(123), resolved.Identity.OrgID)
	assert.Equal(t, "test-runner", resolved.Identity.RunnerID)
	assert.NotEmpty(t, resolved.Identity.PrivateKey)
}

func TestBootstrapUsesInlineIdentityWithoutPersistedOne(t *testing.T) {
	inlineURN := parutil.MakeRunnerURN("us5", 999, "inline-runner")
	inlineKey := validPrivateKey(t)
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"private_action_runner.enabled":            true,
		"private_action_runner.split_enabled":      true,
		"private_action_runner.identity_file_path": filepath.Join(t.TempDir(), "missing.json"),
		"private_action_runner.urn":                inlineURN,
		"private_action_runner.private_key":        inlineKey,
	})

	resolved, _, err := runBootstrap(t, cfg)

	require.NoError(t, err)
	assert.Equal(t, inlineURN, resolved.Identity.URN)
	assert.Equal(t, int64(999), resolved.Identity.OrgID)
	assert.Equal(t, "inline-runner", resolved.Identity.RunnerID)
	assert.Equal(t, inlineKey, resolved.Identity.PrivateKey)
}

func TestBootstrapReturnsNewlyEnrolledIdentity(t *testing.T) {
	identityPath := filepath.Join(t.TempDir(), "identity.json")
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"private_action_runner.enabled":            true,
		"private_action_runner.split_enabled":      true,
		"private_action_runner.self_enroll":        true,
		"private_action_runner.identity_file_path": identityPath,
	})
	hostnameComp, _ := hostnamemock.NewMock("test-host")
	enrolledKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	var out bytes.Buffer
	err = bootstrap(context.Background(), logmock.New(t), cfg, hostnameComp,
		func(_ context.Context, _ log.Component, cfg coreconfig.Component, _ *enrollment.AgentIdentifier) (*enrollment.Result, error) {
			result := &enrollment.Result{URN: validURN(), PrivateKey: enrolledKey, Hostname: "test-host"}
			// Real enrollment persists before returning.
			require.NoError(t, enrollment.PersistIdentity(context.Background(), cfg, result))
			return result, nil
		}, &out)

	require.NoError(t, err)
	resolved := parseEmitted(t, out.String())
	assert.Equal(t, validURN(), resolved.Identity.URN)
	// The returned key is the one enrollment just persisted.
	persisted, err := enrollment.GetIdentityFromPreviousEnrollment(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, persisted.PrivateKey, resolved.Identity.PrivateKey)
}

func TestBootstrapExportsProxyTLSAndPaths(t *testing.T) {
	cfg := splitConfig(t, map[string]interface{}{
		"proxy.https":         "http://secure-proxy.example:8443",
		"proxy.no_proxy":      []string{"localhost"},
		"skip_ssl_validation": true,
		"min_tls_version":     "tlsv1.3",
		"ipc_cert_file_path":  "/custom/ipc_cert.pem",
		"private_action_runner.executor.socket_path": "/tmp/custom-executor.sock",
		"private_action_runner.task_concurrency":     7,
		"private_action_runner.opms_extra_headers":   map[string]string{"X-Test-Routing": "canary"},
	})

	resolved, _, err := runBootstrap(t, cfg)

	require.NoError(t, err)
	assert.Equal(t, "http://secure-proxy.example:8443", resolved.OPMSProxyURL)
	assert.True(t, resolved.TLS.SkipSSLValidation)
	assert.Equal(t, "tlsv1.3", resolved.TLS.MinTLSVersion)
	assert.Equal(t, "/custom/ipc_cert.pem", resolved.IPCCertFilePath)
	assert.Equal(t, "/tmp/custom-executor.sock", resolved.ExecutorSocket)
	assert.Equal(t, int32(7), resolved.TaskConcurrency)
	assert.Equal(t, map[string]string{"X-Test-Routing": "canary"}, resolved.OPMSExtraHeaders)
}

func TestResolveProxyUsesCanonicalAgentMatching(t *testing.T) {
	for name, test := range map[string]struct {
		target  string
		noProxy string
		want    string
	}{
		"explicit default port matches": {
			target:  "https://opms.internal:443",
			noProxy: "opms.internal:443",
		},
		"omitted default port does not match": {
			target:  "https://opms.internal",
			noProxy: "opms.internal:443",
			want:    "http://proxy.example:3128",
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
				"proxy.https":    "http://proxy.example:3128",
				"proxy.no_proxy": []string{test.noProxy},
			})

			got, err := resolveProxy(cfg, test.target)

			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestResolveProxyErrorDoesNotExposeCredentials(t *testing.T) {
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"proxy.https": "http://user:hunter2@%",
	})

	_, err := resolveProxy(cfg, "https://opms.internal")

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "hunter2")
}

// par-control has no Agent config, so an unset ipc_cert_file_path has to be
// resolved to a concrete path here.
func TestBootstrapResolvesDefaultIPCCertPath(t *testing.T) {
	authToken := filepath.Join(t.TempDir(), "auth_token")
	cfg := splitConfig(t, map[string]interface{}{"auth_token_file_path": authToken})

	resolved, _, err := runBootstrap(t, cfg)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(filepath.Dir(authToken), "ipc_cert.pem"), resolved.IPCCertFilePath)
}

func TestBootstrapOPMSEndpointMatchesTheGoClient(t *testing.T) {
	t.Run("site", func(t *testing.T) {
		cfg := splitConfig(t, map[string]interface{}{"site": "us3.datadoghq.com"})

		resolved, _, err := runBootstrap(t, cfg)

		require.NoError(t, err)
		assert.Equal(t, "https://api.us3.datadoghq.com", resolved.OPMSBaseURL)
	})

	// Fakeintake E2Es route OPMS at dd_url through this switch; Go and Rust must
	// resolve it identically.
	t.Run("fakeintake dd_url", func(t *testing.T) {
		t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")
		cfg := splitConfig(t, map[string]interface{}{"dd_url": "http://fakeintake.test:8080"})

		resolved, _, err := runBootstrap(t, cfg)

		require.NoError(t, err)
		assert.Equal(t, "http://fakeintake.test:8080", resolved.OPMSBaseURL)
	})
}

func TestBootstrapUsesDocumentedDurationUnits(t *testing.T) {
	cfg := splitConfig(t, nil)

	resolved, _, err := runBootstrap(t, cfg)

	require.NoError(t, err)
	assert.Equal(t, int64(1_000), resolved.LoopIntervalMilliseconds)
	assert.Equal(t, int64(20_000), resolved.HeartbeatIntervalMilliseconds)
	assert.Equal(t, int64(30_000), resolved.HealthCheckIntervalMillisecond)
	assert.Equal(t, int64(30_000), resolved.OPMSRequestTimeoutMilliseconds)
	assert.Equal(t, int64(1_000), resolved.MinBackoffMilliseconds)
	assert.Equal(t, int64(180_000), resolved.MaxBackoffMilliseconds)
	assert.Equal(t, int64(300_000), resolved.WaitBeforeRetryMilliseconds)
	assert.Equal(t, int32(20), resolved.MaxAttempts)
	assert.Equal(t, []string{"pull"}, resolved.Modes)
	assert.NotEmpty(t, resolved.AgentVersion)
}

// The payload carries the private key and may carry proxy credentials, so it
// must never reach an error string.
func TestBootstrapErrorsNeverCarrySecrets(t *testing.T) {
	secretKey := validPrivateKey(t)
	cfg := splitConfig(t, map[string]interface{}{
		"fips.enabled":                      true,
		"private_action_runner.private_key": secretKey,
		"proxy.https":                       "http://user:hunter2@proxy.example:8443",
	})

	_, stdout, err := runBootstrap(t, cfg)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), secretKey)
	assert.NotContains(t, err.Error(), "hunter2")
	// A failed bootstrap emits no configuration at all.
	assert.Empty(t, stdout)
}

func TestBootstrapPropagatesEnrollmentFailure(t *testing.T) {
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"private_action_runner.enabled":            true,
		"private_action_runner.split_enabled":      true,
		"private_action_runner.self_enroll":        false,
		"private_action_runner.identity_file_path": filepath.Join(t.TempDir(), "missing.json"),
	})

	_, stdout, err := runBootstrap(t, cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "private_action_runner.self_enroll is false")
	assert.Empty(t, stdout)
}
