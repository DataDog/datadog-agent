// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// remoteQueryTestSite is the site the shared test config uses; fixtures' baseUrl values are
// on dd.datad0g.com.
const remoteQueryTestSite = "datad0g.com"

// remoteQueryTestConfig returns a mock Agent config whose site matches the fixtures' baseUrl.
func remoteQueryTestConfig(t testing.TB) config.Component {
	t.Helper()
	cfg := configmock.New(t)
	cfg.SetInTest("site", remoteQueryTestSite)
	return cfg
}

func TestValidateResultDeliveryBaseURL(t *testing.T) {
	const intake = "/api/unstable/its-agent-intake"
	tests := []struct {
		name    string
		baseURL string
		site    string
		wantErr string
	}{
		{name: "subdomain", baseURL: "https://dd.datad0g.com" + intake, site: "datad0g.com"},
		{name: "apex host", baseURL: "https://datad0g.com" + intake, site: "datad0g.com"},
		{name: "explicit 443", baseURL: "https://dd.datad0g.com:443" + intake, site: "datad0g.com"},
		{name: "host case-insensitive", baseURL: "https://DD.Datad0g.COM" + intake, site: "datad0g.com"},
		{name: "no path", baseURL: "https://dd.datad0g.com", site: "datad0g.com"},
		{name: "default site", baseURL: "https://app.datadoghq.com" + intake, site: "datadoghq.com"},
		{name: "http", baseURL: "http://dd.datad0g.com" + intake, site: "datad0g.com", wantErr: "https scheme"},
		{name: "other host", baseURL: "https://evil.com" + intake, site: "datad0g.com", wantErr: "Agent's site"},
		{name: "site as subdomain of attacker", baseURL: "https://datad0g.com.evil.com" + intake, site: "datad0g.com", wantErr: "Agent's site"},
		{name: "site as suffix without dot", baseURL: "https://evildatad0g.com" + intake, site: "datad0g.com", wantErr: "Agent's site"},
		{name: "trailing dot host", baseURL: "https://dd.datad0g.com." + intake, site: "datad0g.com", wantErr: "Agent's site"},
		{name: "userinfo", baseURL: "https://u:p@dd.datad0g.com" + intake, site: "datad0g.com", wantErr: "userinfo"},
		{name: "userinfo host confusion", baseURL: "https://dd.datad0g.com@evil.com" + intake, site: "datad0g.com", wantErr: "userinfo"},
		{name: "other port", baseURL: "https://dd.datad0g.com:8443" + intake, site: "datad0g.com", wantErr: "port"},
		{name: "query", baseURL: "https://dd.datad0g.com" + intake + "?a=b", site: "datad0g.com", wantErr: "query"},
		{name: "empty query marker", baseURL: "https://dd.datad0g.com" + intake + "?", site: "datad0g.com", wantErr: "query"},
		{name: "fragment", baseURL: "https://dd.datad0g.com" + intake + "#x", site: "datad0g.com", wantErr: "fragment"},
		{name: "backslash confusion", baseURL: `https://evil.com\@dd.datad0g.com` + intake, site: "datad0g.com", wantErr: "backslash"},
		{name: "whitespace", baseURL: "https://dd.datad0g.com" + intake + " x", site: "datad0g.com", wantErr: "whitespace"},
		{name: "relative", baseURL: "/api/unstable/its-agent-intake", site: "datad0g.com", wantErr: "absolute URL"},
		{name: "scheme-relative", baseURL: "//dd.datad0g.com" + intake, site: "datad0g.com", wantErr: "absolute URL"},
		{name: "opaque", baseURL: "https:dd.datad0g.com", site: "datad0g.com", wantErr: "absolute URL"},
		{name: "unparseable", baseURL: "https://[::1", site: "datad0g.com", wantErr: "absolute URL"},
		{name: "unset site fails closed", baseURL: "https://app.datadoghq.com" + intake, site: "", wantErr: "not configured"},
		{name: "default site rejects other site", baseURL: "https://dd.datad0g.com" + intake, site: "datadoghq.com", wantErr: "Agent's site"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateResultDeliveryBaseURL(tc.baseURL, tc.site)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "result_delivery.baseUrl")
			assert.Contains(t, err.Error(), tc.wantErr)
			// The handle is never echoed back.
			assert.NotContains(t, err.Error(), "datad0g.com"+intake)
			assert.NotContains(t, err.Error(), "evil.com")
		})
	}
}

func TestRemoteQueryExecuteServiceSite(t *testing.T) {
	cfg := configmock.New(t)

	// A config without an explicit site uses the config component's default; a missing
	// config or an explicitly empty site yields no site (callers fail closed).
	assert.Equal(t, "datadoghq.com", NewRemoteQueryExecuteService(nil, true, cfg).site())
	require.NoError(t, validateResultDeliveryBaseURL("https://app.datadoghq.com/api/unstable/its-agent-intake", NewRemoteQueryExecuteService(nil, true, cfg).site()))
	assert.Equal(t, "", NewRemoteQueryExecuteService(nil, true, nil).site())
	assert.Equal(t, "", (*RemoteQueryExecuteService)(nil).site())

	cfg.SetInTest("site", "  EU1.Datadoghq.com ")
	assert.Equal(t, "eu1.datadoghq.com", NewRemoteQueryExecuteService(nil, true, cfg).site())

	cfg.SetInTest("site", "  ")
	assert.Equal(t, "", NewRemoteQueryExecuteService(nil, true, cfg).site())
}

// TestExecuteStreamRejectsBaseURLOffSite proves a baseUrl off the Agent's site fails the
// run before the integration sees anything, that the config's default site applies when
// unset, and that a missing site fails closed.
func TestExecuteStreamRejectsBaseURLOffSite(t *testing.T) {
	newRequest := func(t *testing.T) RemoteQueryExecuteRequest {
		req, err := NewRemoteQueryExecuteRequest(
			"postgres",
			RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"},
			"SELECT 1 AS value",
			false,
			syntheticForwardDelivery(),
		)
		require.NoError(t, err)
		return req
	}
	emit := func(check.RemoteQueryStreamEvent) error { return nil }

	t.Run("configured site mismatch", func(t *testing.T) {
		runner := newSyntheticForwardRunner(nil)
		cfg := configmock.New(t)
		cfg.SetInTest("site", "datadoghq.com")
		service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, cfg)

		result := service.ExecuteStream(context.Background(), newRequest(t), emit)

		require.NotNil(t, result.Error)
		assert.Equal(t, statusInvalidRequest, result.Error.Code)
		assert.Equal(t, "result_delivery.baseUrl must be on the Agent's site", result.Error.Message)
		assert.NotContains(t, result.Error.Message, "datad0g.com")
		assert.Equal(t, 0, runner.resolveCalls)
		assert.Equal(t, 0, runner.executeCalls)
		assert.Equal(t, 0, runner.streamCalls)
	})

	t.Run("unset site uses config default site", func(t *testing.T) {
		runner := newSyntheticForwardRunner(nil)
		service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, configmock.New(t))

		result := service.ExecuteStream(context.Background(), newRequest(t), emit)

		require.NotNil(t, result.Error)
		assert.Equal(t, statusInvalidRequest, result.Error.Code)
		assert.Equal(t, "result_delivery.baseUrl must be on the Agent's site", result.Error.Message)
		assert.Equal(t, 0, runner.streamCalls)
	})

	t.Run("nil config fails closed", func(t *testing.T) {
		runner := newSyntheticForwardRunner(nil)
		service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{fakeWrappedCheck{Check: runner}}}, true, nil)

		result := service.ExecuteStream(context.Background(), newRequest(t), emit)

		require.NotNil(t, result.Error)
		assert.Equal(t, statusInvalidRequest, result.Error.Code)
		assert.Equal(t, "result_delivery.baseUrl cannot be checked: the Agent site is not configured", result.Error.Message)
		assert.Equal(t, 0, runner.streamCalls)
	})
}
