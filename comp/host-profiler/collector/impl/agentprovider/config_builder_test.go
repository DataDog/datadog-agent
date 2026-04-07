// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

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
