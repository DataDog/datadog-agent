// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

// buildLogsAgentEndpoints builds endpoints the way the core logs agent does.
func buildLogsAgentEndpoints(t *testing.T, cfg config.Component, httpConnectivity HTTPConnectivity) *Endpoints {
	t.Helper()
	endpoints, err := BuildEndpointsWithVectorOverride(cfg, httpConnectivity, "logs", AgentJSONIntakeProtocol, DefaultIntakeOrigin)
	require.NoError(t, err)
	return endpoints
}

func TestLogsIntakeRoutingCandidates(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]interface{}
		expected  []int
	}{
		{
			name:     "default site",
			expected: []int{0},
		},
		{
			name:      "datacenter site",
			overrides: map[string]interface{}{"site": "us3.datadoghq.com"},
			expected:  []int{0},
		},
		{
			name:      "gov site",
			overrides: map[string]interface{}{"site": "ddog-gov.com"},
			expected:  []int{0},
		},
		{
			name:      "staging site",
			overrides: map[string]interface{}{"site": "datad0g.com"},
			expected:  []int{0},
		},
		{
			name:      "explicit official URL",
			overrides: map[string]interface{}{"logs_config.logs_dd_url": "https://agent-http-intake.logs.datadoghq.eu"},
			expected:  []int{0},
		},
		{
			name:      "explicit official host and port 443",
			overrides: map[string]interface{}{"logs_config.logs_dd_url": "agent-http-intake.logs.us5.datadoghq.com:443"},
			expected:  []int{0},
		},
		{
			name:      "explicit official URL with FQDN dot",
			overrides: map[string]interface{}{"logs_config.logs_dd_url": "https://agent-http-intake.logs.datadoghq.com."},
			expected:  []int{0},
		},
		{
			name:      "custom port",
			overrides: map[string]interface{}{"logs_config.logs_dd_url": "https://agent-http-intake.logs.datadoghq.com:8443"},
		},
		{
			name:      "path prefix",
			overrides: map[string]interface{}{"logs_config.logs_dd_url": "https://agent-http-intake.logs.datadoghq.com/prefix"},
		},
		{
			name:      "plain HTTP",
			overrides: map[string]interface{}{"logs_config.logs_dd_url": "http://agent-http-intake.logs.datadoghq.com:80"},
		},
		{
			name: "logs_no_ssl",
			overrides: map[string]interface{}{
				"logs_config.logs_dd_url": "agent-http-intake.logs.datadoghq.com:443",
				"logs_config.logs_no_ssl": true,
			},
		},
		{
			name:      "PrivateLink",
			overrides: map[string]interface{}{"logs_config.logs_dd_url": "https://agent-http-intake-pl.logs.us5.datadoghq.com"},
		},
		{
			name:      "proxy",
			overrides: map[string]interface{}{"logs_config.logs_dd_url": "https://logs-proxy.example.com"},
		},
		{
			name:      "v1 API",
			overrides: map[string]interface{}{"logs_config.use_v2_api": false},
		},
		{
			name: "OPW replaces the Datadog endpoint",
			overrides: map[string]interface{}{
				"observability_pipelines_worker.logs.enabled": true,
				// Even an OPW URL that looks like the official intake is never routed.
				"observability_pipelines_worker.logs.url": "https://agent-http-intake.logs.datadoghq.com",
			},
		},
		{
			name: "OPW dual-ship",
			overrides: map[string]interface{}{
				"observability_pipelines_worker.logs.enabled":   true,
				"observability_pipelines_worker.logs.url":       "https://agent-http-intake.logs.datadoghq.com",
				"observability_pipelines_worker.logs.dual_ship": true,
			},
			expected: []int{0},
		},
		{
			name: "MRF",
			overrides: map[string]interface{}{
				"multi_region_failover.enabled": true,
				"multi_region_failover.site":    "us3.datadoghq.com",
				"multi_region_failover.api_key": "mrfkey",
			},
			expected: []int{0},
		},
		{
			name: "additional endpoints",
			overrides: map[string]interface{}{
				// Additional endpoints are sent over TCP unless HTTP is forced.
				"logs_config.force_use_http": true,
				"logs_config.additional_endpoints": `[
					{"api_key": "key2", "host": "agent-http-intake.logs.datadoghq.eu"},
					{"api_key": "key3", "host": "logs.example.com"},
					{"api_key": "key4", "host": "agent-http-intake.logs.datadoghq.com", "port": 8443}
				]`,
			},
			expected: []int{0, 1},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			endpoints := buildLogsAgentEndpoints(t, config.NewMockWithOverrides(t, tc.overrides), HTTPConnectivitySuccess)
			require.True(t, endpoints.UseHTTP)
			assert.Equal(t, tc.expected, endpoints.LogsIntakeRoutingCandidates())
		})
	}

	t.Run("TCP", func(t *testing.T) {
		endpoints := buildLogsAgentEndpoints(t, config.NewMock(t), HTTPConnectivityFailure)
		require.False(t, endpoints.UseHTTP)
		assert.Nil(t, endpoints.LogsIntakeRoutingCandidates())
	})
}

func TestWithHostOverrides(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"api_key":                    "mainkey",
		"logs_config.force_use_http": true,
		"logs_config.additional_endpoints": `[
			{"api_key": "key2", "host": "agent-http-intake.logs.datadoghq.eu"},
			{"api_key": "key3", "host": "logs.example.com", "port": 10516}
		]`,
	})
	base := buildLogsAgentEndpoints(t, cfg, HTTPConnectivitySuccess)
	require.True(t, base.UseHTTP)
	baseMainHost := base.Main.Host
	require.Len(t, base.Endpoints, 3)

	assert.Same(t, base, base.WithHostOverrides(nil))

	routed := base.WithHostOverrides(map[int]HostOverride{
		0: {Host: "acme.agent-http-intake.logs.datadoghq.com"},
		1: {Host: "acme.agent-http-intake.logs.datadoghq.eu", Port: 8443},
		7: {Host: "ignored.datadoghq.com"},
	})

	// The base is left untouched.
	assert.Equal(t, baseMainHost, base.Main.Host)
	assert.Equal(t, baseMainHost, base.Endpoints[0].Host)
	assert.Equal(t, "agent-http-intake.logs.datadoghq.eu", base.Endpoints[1].Host)
	assert.Equal(t, 0, base.Endpoints[1].Port)

	// Main follows Endpoints[0]; other settings are kept.
	assert.Equal(t, "acme.agent-http-intake.logs.datadoghq.com", routed.Main.Host)
	assert.Equal(t, "acme.agent-http-intake.logs.datadoghq.com", routed.Endpoints[0].Host)
	assert.Equal(t, "acme.agent-http-intake.logs.datadoghq.eu", routed.Endpoints[1].Host)
	assert.Equal(t, 8443, routed.Endpoints[1].Port)
	assert.Equal(t, base.Endpoints[2], routed.Endpoints[2])
	assert.Equal(t, base.UseHTTP, routed.UseHTTP)
	assert.Equal(t, base.BatchMaxSize, routed.BatchMaxSize)
	assert.Equal(t, base.Main.Version, routed.Main.Version)
	assert.Equal(t, base.Main.UseCompression, routed.Main.UseCompression)

	// Overriding a routed copy keeps the configured host in the status.
	rerouted := routed.WithHostOverrides(map[int]HostOverride{0: {Host: "shard2.agent-http-intake.logs.datadoghq.com"}})
	assert.Contains(t, rerouted.Main.GetStatus("", true), "(intake routing; default host: "+baseMainHost+")")
}

func TestWithHostOverridesKeepsAPIKeyRotation(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"api_key":                          "mainkey",
		"logs_config.force_use_http":       true,
		"logs_config.additional_endpoints": `[{"api_key": "key2", "host": "agent-http-intake.logs.datadoghq.eu"}]`,
	})
	base := buildLogsAgentEndpoints(t, cfg, HTTPConnectivitySuccess)
	routed := base.WithHostOverrides(map[int]HostOverride{
		0: {Host: "acme.agent-http-intake.logs.datadoghq.com"},
		1: {Host: "acme.agent-http-intake.logs.datadoghq.eu"},
	})

	cfg.SetInTest("api_key", "mainkey2")
	key, ready := routed.Main.GetAPIKeyIfReady()
	assert.Equal(t, "mainkey2", key)
	assert.True(t, ready)

	// The additional endpoint is reconciled against its configured host, not the routed one, so the
	// rotation reaches the routed copy instead of disabling it.
	cfg.SetInTest("logs_config.additional_endpoints", `[{"api_key": "key2-rotated", "host": "agent-http-intake.logs.datadoghq.eu"}]`)
	key, ready = routed.Endpoints[1].GetAPIKeyIfReady()
	assert.Equal(t, "key2-rotated", key)
	assert.True(t, ready)
	assert.Equal(t, "key2-rotated", base.Endpoints[1].GetAPIKey())
}

func TestIntakeRoutingStatus(t *testing.T) {
	base := buildLogsAgentEndpoints(t, config.NewMock(t), HTTPConnectivitySuccess)
	routed := base.WithHostOverrides(map[int]HostOverride{0: {Host: "acme.agent-http-intake.logs.datadoghq.com"}})

	for _, line := range base.GetStatus() {
		assert.NotContains(t, line, "intake routing")
	}
	status := routed.GetStatus()
	require.Len(t, status, 1)
	assert.Contains(t, status[0], "to acme.agent-http-intake.logs.datadoghq.com on port 443")
	assert.Contains(t, status[0], "(intake routing; default host: agent-http-intake.logs.datadoghq.com.)")
}
