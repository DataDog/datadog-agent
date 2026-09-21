// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package executor

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"net/http"
	"testing"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/stretchr/testify/require"
)

func TestControlPlaneConfigMatchesMonolithRuntime(t *testing.T) {
	for _, tc := range []struct {
		name, ddURL, bypass, wantURL, wantProxy string
		internalHTTP                            bool
	}{
		{name: "PAR-local", wantURL: "https://api.us3.datadoghq.com", wantProxy: "http://user:password@proxy:3128"},
		{name: "endpoint override", ddURL: "https://app.datadoghq.eu", wantURL: "https://api.datadoghq.eu", wantProxy: "http://user:password@proxy:3128"},
		{name: "HTTP fakeintake", ddURL: "http://fake-intake:8080", internalHTTP: true, wantURL: "http://fake-intake:8080", wantProxy: "http://user:password@proxy:3128"},
		{name: "exact bypass", bypass: "api.us3.datadoghq.com", wantURL: "https://api.us3.datadoghq.com"},
		{name: "domain bypass", bypass: ".datadoghq.com", wantURL: "https://api.us3.datadoghq.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.internalHTTP {
				t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "true")
			} else {
				t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "false")
			}
			cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
				"site": "us3.datadoghq.com", "dd_url": tc.ddURL,
				"private_action_runner.task_concurrency":   9,
				"private_action_runner.opms_extra_headers": map[string]string{"X-PAR-Only": "header"},
				"proxy.http": "http://user:password@proxy:3128", "proxy.https": "http://user:password@proxy:3128",
				"proxy.no_proxy": []string{tc.bypass}, "no_proxy_nonexact_match": true,
				"skip_ssl_validation": true, "min_tls_version": "tlsv1.3", "log_level": "debug",
			})
			runner, err := parconfig.FromDDConfig(cfg, nil)
			require.NoError(t, err)
			// Use the resolved identity, not stale values in the underlying configuration.
			runner.PrivateKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			require.NoError(t, err)
			runner.Urn = util.MakeRunnerURN("us3", 42, "runner")
			runner.OrgId, runner.RunnerId = 42, "runner"
			snapshot, err := ControlPlaneConfig(cfg, runner)
			require.NoError(t, err)
			require.True(t, snapshot.SplitMode)
			require.EqualValues(t, ControlPlaneProtocolVersion, snapshot.ProtocolVersion)
			require.Equal(t, "debug", snapshot.LogLevel)
			require.Equal(t, tc.wantURL, snapshot.Runtime.OpmsBaseUrl)
			require.Equal(t, runner.RunnerPoolSize, snapshot.Runtime.TaskConcurrency)
			require.Equal(t, runner.OpmsExtraHeaders, snapshot.Runtime.OpmsExtraHeaders)
			require.Equal(t, tc.wantProxy, snapshot.Runtime.OpmsProxyUrl)
			transport := httputils.CreateHTTPTransport(cfg)
			defer transport.CloseIdleConnections()
			request, err := http.NewRequest(http.MethodPost, opms.EndpointURL(runner, ""), nil)
			require.NoError(t, err)
			proxy, err := transport.Proxy(request)
			require.NoError(t, err)
			if proxy != nil {
				require.Equal(t, proxy.String(), snapshot.Runtime.OpmsProxyUrl)
			} else {
				require.Empty(t, snapshot.Runtime.OpmsProxyUrl)
			}
			require.Equal(t, transport.TLSClientConfig.InsecureSkipVerify, snapshot.Runtime.SkipSslValidation)
			require.Equal(t, "tlsv1.3", snapshot.Runtime.MinTlsVersion)
			key, err := util.Base64ToJWK(snapshot.Identity.PrivateKey)
			require.NoError(t, err)
			require.True(t, runner.PrivateKey.Equal(key.Key))
			require.Equal(t, runner.Urn, snapshot.Identity.Urn)
		})
	}
}

func TestDisabledControlPlaneConfig(t *testing.T) {
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{"private_action_runner.private_key": "invalid-key"})
	snapshot, err := ControlPlaneConfig(cfg, nil)
	require.NoError(t, err)
	require.False(t, snapshot.SplitMode)
	require.Nil(t, snapshot.Identity)
	require.Nil(t, snapshot.Runtime)
}
