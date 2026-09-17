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
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	parutil "github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
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
	assert.Nil(t, resolved.Runtime)
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
		"private_action_runner.urn":         validURN(),
		"private_action_runner.private_key": validPrivateKey(t),
		"auth_token_file_path":              "/tmp/auth_token",
	})

	resolved, err := runBootstrap(t, cfg, failIfEnrolled(t))

	require.NoError(t, err)
	assert.NotEmpty(t, resolved.IPCCertFilePath)
	require.NotNil(t, resolved.Runtime)
	assert.NotNil(t, resolved.Runtime.OPMSExtraHeaders, "Rust expects an object, not JSON null")
	assert.NotEmpty(t, resolved.Runtime.ExecutorSocketPath)
	assert.Positive(t, resolved.Runtime.TaskConcurrency)
}

func TestBootstrapRuntimeMatchesMonolith(t *testing.T) {
	for _, tc := range []struct {
		name      string
		overrides map[string]interface{}
		useDDURL  bool
		wantURL   string
		wantProxy string
	}{
		{
			name:      "PAR-local settings",
			wantURL:   "https://api.us3.datadoghq.com",
			wantProxy: "http://user:password@https-proxy:3128",
		},
		{
			name:      "explicit dd_url overrides site",
			overrides: map[string]interface{}{"dd_url": "https://app.datadoghq.eu"},
			wantURL:   "https://api.datadoghq.eu",
			wantProxy: "http://user:password@https-proxy:3128",
		},
		{
			name:      "internal HTTP fake intake",
			overrides: map[string]interface{}{"dd_url": "http://fake-intake:8080"},
			useDDURL:  true,
			wantURL:   "http://fake-intake:8080",
			wantProxy: "http://http-proxy:3128",
		},
		{
			name:      "exact proxy bypass",
			overrides: map[string]interface{}{"proxy.no_proxy": []string{"api.us3.datadoghq.com"}},
			wantURL:   "https://api.us3.datadoghq.com",
		},
		{
			name: "domain proxy bypass",
			overrides: map[string]interface{}{
				"proxy.no_proxy":          []string{".datadoghq.com"},
				"no_proxy_nonexact_match": true,
			},
			wantURL: "https://api.us3.datadoghq.com",
		},
		{
			name:      "direct connection",
			overrides: map[string]interface{}{"proxy.http": "", "proxy.https": ""},
			wantURL:   "https://api.us3.datadoghq.com",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.useDDURL {
				t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "true")
			} else {
				t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "false")
			}
			values := map[string]interface{}{
				"private_action_runner.urn":                  validURN(),
				"private_action_runner.private_key":          validPrivateKey(t),
				"private_action_runner.task_concurrency":     9,
				"private_action_runner.executor.socket_path": filepath.Join(t.TempDir(), "par.sock"),
				"private_action_runner.opms_extra_headers":   map[string]string{"X-PAR-Only": "header-value"},
				"log_level":               "debug",
				"site":                    "us3.datadoghq.com",
				"proxy.http":              "http://http-proxy:3128",
				"proxy.https":             "http://user:password@https-proxy:3128",
				"proxy.no_proxy":          []string{},
				"no_proxy_nonexact_match": false,
				"skip_ssl_validation":     true,
				"min_tls_version":         "tlsv1.3",
			}
			for key, value := range tc.overrides {
				values[key] = value
			}
			cfg := splitConfig(t, values)
			resolved, err := runBootstrap(t, cfg, failIfEnrolled(t))
			require.NoError(t, err)
			require.NotNil(t, resolved.Runtime)

			monolith, err := parconfig.FromDDConfig(cfg, nil)
			require.NoError(t, err)
			transport := httputils.CreateHTTPTransport(cfg)
			defer transport.CloseIdleConnections()
			request, err := http.NewRequest(http.MethodPost, opms.EndpointURL(monolith, "/api/unstable/on-prem-management/runner/dequeue"), nil)
			require.NoError(t, err)
			proxyURL := ""
			if transport.Proxy != nil {
				proxy, err := transport.Proxy(request)
				require.NoError(t, err)
				if proxy != nil {
					proxyURL = proxy.String()
				}
			}

			assert.Equal(t, "debug", resolved.LogLevel)
			assert.Equal(t, tc.wantURL, resolved.Runtime.OPMSBaseURL)
			assert.Equal(t, opms.EndpointURL(monolith, ""), resolved.Runtime.OPMSBaseURL)
			assert.Equal(t, tc.wantProxy, resolved.Runtime.OPMSProxyURL)
			assert.Equal(t, proxyURL, resolved.Runtime.OPMSProxyURL)
			assert.Equal(t, monolith.RunnerPoolSize, resolved.Runtime.TaskConcurrency)
			assert.Equal(t, monolith.OpmsExtraHeaders, resolved.Runtime.OPMSExtraHeaders)
			assert.Equal(t, values["private_action_runner.executor.socket_path"], resolved.Runtime.ExecutorSocketPath)
			assert.Equal(t, transport.TLSClientConfig.InsecureSkipVerify, resolved.Runtime.SkipSSLValidation)
			assert.Equal(t, "tlsv1.3", resolved.Runtime.MinTLSVersion)
		})
	}
}

func TestBootstrapRejectsInvalidIdentity(t *testing.T) {
	cfg := splitConfig(t, map[string]interface{}{
		"private_action_runner.urn":         "invalid",
		"private_action_runner.private_key": validPrivateKey(t),
	})

	_, err := runBootstrap(t, cfg, failIfEnrolled(t))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
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
