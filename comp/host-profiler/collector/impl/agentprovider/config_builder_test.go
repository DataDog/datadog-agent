// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && test

package agentprovider

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildExportersAdditionalHeaders(t *testing.T) {
	t.Setenv("DD_HOST_PROFILER_ADDITIONAL_HEADERS", "workspace:peterg17,env:staging")

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
	assert.Equal(t, "workspace:peterg17,env:staging", headers["x-datadog-additional-headers"])
}

func TestBuildExportersNoAdditionalHeaders(t *testing.T) {
	t.Setenv("DD_HOST_PROFILER_ADDITIONAL_HEADERS", "")

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

	_, exists := headers["x-datadog-additional-headers"]
	assert.False(t, exists, "header should not be present when env var is empty")

	// Unset should also not produce the header
	os.Unsetenv("DD_HOST_PROFILER_ADDITIONAL_HEADERS")
	conf2 := make(confMap)
	buildExporters(conf2, agent)
	exporters2, _ := conf2["exporters"].(confMap)
	exporter2, _ := exporters2["otlphttp/datadoghq.com_0"].(confMap)
	headers2, _ := exporter2["headers"].(confMap)
	_, exists = headers2["x-datadog-additional-headers"]
	assert.False(t, exists, "header should not be present when env var is unset")
}
