// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && test

package agentprovider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildExportersAdditionalHTTPHeaders(t *testing.T) {
	agent := configManager{
		endpoints: []endpoint{{
			site:    "datadoghq.com",
			apiKeys: []string{"test_key"},
		}},
		endpointsTotalLength: 1,
		hostProfilerConfig: hostProfilerConfig{
			AdditionalHTTPHeaders: map[string]string{
				"x-datadog-profiling-metadata": "workspace:peterg17,env:staging",
				"x-custom-routing":             "some-value",
			},
		},
	}
	conf := make(confMap)
	buildExporters(conf, agent)

	exporters, ok := conf["exporters"].(confMap)
	require.True(t, ok)
	exporter, ok := exporters["otlphttp/datadoghq.com_0"].(confMap)
	require.True(t, ok)
	headers, ok := exporter["headers"].(confMap)
	require.True(t, ok)

	assert.Equal(t, "workspace:peterg17,env:staging", headers["x-datadog-profiling-metadata"])
	assert.Equal(t, "some-value", headers["x-custom-routing"])
	assert.Equal(t, "test_key", headers["dd-api-key"])
}

func TestBuildExportersNoAdditionalHTTPHeaders(t *testing.T) {
	agent := configManager{
		endpoints: []endpoint{{
			site:    "datadoghq.com",
			apiKeys: []string{"test_key"},
		}},
		endpointsTotalLength: 1,
	}
	conf := make(confMap)
	buildExporters(conf, agent)

	exporters, ok := conf["exporters"].(confMap)
	require.True(t, ok)
	exporter, ok := exporters["otlphttp/datadoghq.com_0"].(confMap)
	require.True(t, ok)
	headers, ok := exporter["headers"].(confMap)
	require.True(t, ok)

	assert.Equal(t, "test_key", headers["dd-api-key"])
	assert.Len(t, headers, 3) // dd-api-key, dd-evp-origin, dd-evp-origin-version
}

func TestBuildConfigDDProfilingEnabled(t *testing.T) {
	agent := configManager{
		endpoints: []endpoint{{
			site:    "datadoghq.com",
			apiKeys: []string{"test_key"},
		}},
		endpointsTotalLength: 1,
		hostProfilerConfig: hostProfilerConfig{
			DDProfilingEnabled: true,
		},
	}
	conf := buildConfig(agent, testCollectorParams{})

	extensions, ok := conf["extensions"].(confMap)
	require.True(t, ok)
	ddprofiling, ok := extensions["ddprofiling/default"].(confMap)
	require.True(t, ok)
	assert.Empty(t, ddprofiling, "ddprofiling config should be empty when period is not set")

	service, ok := conf["service"].(confMap)
	require.True(t, ok)
	svcExtensions, ok := service["extensions"].([]any)
	require.True(t, ok)
	assert.Contains(t, svcExtensions, "ddprofiling/default")
	assert.Contains(t, svcExtensions, "hpflare/default")
}

func TestBuildConfigDDProfilingEnabledWithPeriod(t *testing.T) {
	agent := configManager{
		endpoints: []endpoint{{
			site:    "datadoghq.com",
			apiKeys: []string{"test_key"},
		}},
		endpointsTotalLength: 1,
		hostProfilerConfig: hostProfilerConfig{
			DDProfilingEnabled: true,
			DDProfilingPeriod:  30,
		},
	}
	conf := buildConfig(agent, testCollectorParams{})

	extensions, ok := conf["extensions"].(confMap)
	require.True(t, ok)
	ddprofiling, ok := extensions["ddprofiling/default"].(confMap)
	require.True(t, ok)
	profilerOptions, ok := ddprofiling["profiler_options"].(confMap)
	require.True(t, ok)
	assert.Equal(t, 30, profilerOptions["period"])

	service, ok := conf["service"].(confMap)
	require.True(t, ok)
	svcExtensions, ok := service["extensions"].([]any)
	require.True(t, ok)
	assert.Contains(t, svcExtensions, "ddprofiling/default")
}

func TestBuildConfigDDProfilingDisabled(t *testing.T) {
	agent := configManager{
		endpoints: []endpoint{{
			site:    "datadoghq.com",
			apiKeys: []string{"test_key"},
		}},
		endpointsTotalLength: 1,
	}
	conf := buildConfig(agent, testCollectorParams{})

	extensions, ok := conf["extensions"].(confMap)
	require.True(t, ok)
	_, ok = extensions["ddprofiling/default"]
	assert.False(t, ok, "ddprofiling extension should not be present when disabled")

	service, ok := conf["service"].(confMap)
	require.True(t, ok)
	svcExtensions, ok := service["extensions"].([]any)
	require.True(t, ok)
	assert.NotContains(t, svcExtensions, "ddprofiling/default")
	assert.Contains(t, svcExtensions, "hpflare/default")
}

func TestBuildExportersAdditionalHTTPHeadersDoNotOverrideRequired(t *testing.T) {
	agent := configManager{
		endpoints: []endpoint{{
			site:    "datadoghq.com",
			apiKeys: []string{"test_key"},
		}},
		endpointsTotalLength: 1,
		hostProfilerConfig: hostProfilerConfig{
			AdditionalHTTPHeaders: map[string]string{
				"dd-api-key": "should-not-override",
			},
		},
	}
	conf := make(confMap)
	buildExporters(conf, agent)

	exporters, ok := conf["exporters"].(confMap)
	require.True(t, ok)
	exporter, ok := exporters["otlphttp/datadoghq.com_0"].(confMap)
	require.True(t, ok)
	headers, ok := exporter["headers"].(confMap)
	require.True(t, ok)

	assert.Equal(t, "test_key", headers["dd-api-key"], "required headers must not be overridden")
}
