// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && test

package agentprovider

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/stretchr/testify/assert"
)

func TestNewConfigManagerDebugFromYAML(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
hostprofiler:
  debug:
    verbosity: detailed
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, "detailed", mgr.hostProfilerConfig.DebugVerbosity)
}

func TestNewConfigManagerDebugFromEnvVar(t *testing.T) {
	t.Setenv("DD_HOSTPROFILER_DEBUG_VERBOSITY", "detailed")

	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, "detailed", mgr.hostProfilerConfig.DebugVerbosity)
}

func TestNewConfigManagerAdditionalHTTPHeadersFromYAML(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
hostprofiler:
  additional_http_headers:
    x-custom-header: custom-value
    x-another: another-value
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, map[string]string{
		"x-custom-header": "custom-value",
		"x-another":       "another-value",
	}, mgr.hostProfilerConfig.AdditionalHTTPHeaders)
}

func TestNewConfigManagerAdditionalHTTPHeadersFromEnvVar(t *testing.T) {
	t.Setenv("DD_HOSTPROFILER_ADDITIONAL_HTTP_HEADERS", `{"x-custom-header":"custom-value"}`)

	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, map[string]string{
		"x-custom-header": "custom-value",
	}, mgr.hostProfilerConfig.AdditionalHTTPHeaders)
}

func TestNewConfigManagerAdditionalHTTPHeadersEmpty(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
`)
	mgr := newConfigManager(cfg)

	assert.Empty(t, mgr.hostProfilerConfig.AdditionalHTTPHeaders)
}
