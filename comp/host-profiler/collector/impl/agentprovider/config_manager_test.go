// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

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

func TestNewConfigManagerDDProfilingEnabledFromYAML(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
hostprofiler:
  ddprofiling:
    enabled: true
`)
	mgr := newConfigManager(cfg)

	assert.True(t, mgr.hostProfilerConfig.DDProfiling.Enabled)
}

func TestNewConfigManagerDDProfilingEnabledFromEnvVar(t *testing.T) {
	t.Setenv("DD_HOSTPROFILER_DDPROFILING_ENABLED", "true")

	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
`)
	mgr := newConfigManager(cfg)

	assert.True(t, mgr.hostProfilerConfig.DDProfiling.Enabled)
}

func TestNewConfigManagerDDProfilingEnabledDefault(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
`)
	mgr := newConfigManager(cfg)

	assert.False(t, mgr.hostProfilerConfig.DDProfiling.Enabled)
}

func TestNewConfigManagerDDProfilingPeriodFromYAML(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
hostprofiler:
  ddprofiling:
    period: 30
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, 30, mgr.hostProfilerConfig.DDProfiling.Period)
}

func TestNewConfigManagerDDProfilingPeriodFromEnvVar(t *testing.T) {
	t.Setenv("DD_HOSTPROFILER_DDPROFILING_PERIOD", "45")

	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, 45, mgr.hostProfilerConfig.DDProfiling.Period)
}

func TestNewConfigManagerDDProfilingPeriodDefault(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, 0, mgr.hostProfilerConfig.DDProfiling.Period)
}

func TestNewConfigManagerDDProfilingPortFromYAML(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
hostprofiler:
  ddprofiling:
    port: 1234
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, 1234, mgr.hostProfilerConfig.DDProfiling.Port)
}

func TestNewConfigManagerDDProfilingPortFromEnvVar(t *testing.T) {
	t.Setenv("DD_HOSTPROFILER_DDPROFILING_PORT", "1234")

	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, 1234, mgr.hostProfilerConfig.DDProfiling.Port)
}

func TestNewConfigManagerDDProfilingPortDefault(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, 0, mgr.hostProfilerConfig.DDProfiling.Port)
}

func TestNewConfigManagerExperimentalProfilerControlsFromYAML(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
hostprofiler:
  heap_profiling: true
  live_heap_profiling: true
  tracers: native,python
`)
	mgr := newConfigManager(cfg)

	assert.True(t, mgr.hostProfilerConfig.HeapProfiling)
	assert.True(t, mgr.hostProfilerConfig.LiveHeapProfiling)
	assert.Equal(t, "native,python", mgr.hostProfilerConfig.Tracers)
}

func TestNewConfigManagerExperimentalProfilerControlsFromEnvVar(t *testing.T) {
	t.Setenv("DD_HOSTPROFILER_HEAP_PROFILING", "true")
	t.Setenv("DD_HOSTPROFILER_LIVE_HEAP_PROFILING", "true")
	t.Setenv("DD_HOSTPROFILER_TRACERS", "native,python")

	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
`)
	mgr := newConfigManager(cfg)

	assert.True(t, mgr.hostProfilerConfig.HeapProfiling)
	assert.True(t, mgr.hostProfilerConfig.LiveHeapProfiling)
	assert.Equal(t, "native,python", mgr.hostProfilerConfig.Tracers)
}

func TestNewConfigManagerHPFlarePortFromYAML(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
hostprofiler:
  hpflare:
    port: 9999
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, 9999, mgr.hostProfilerConfig.HPFlare.Port)
}

func TestNewConfigManagerHPFlarePortFromEnvVar(t *testing.T) {
	t.Setenv("DD_HOSTPROFILER_HPFLARE_PORT", "9999")

	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, 9999, mgr.hostProfilerConfig.HPFlare.Port)
}

func TestNewConfigManagerHPFlarePortDefault(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, 7778, mgr.hostProfilerConfig.HPFlare.Port)
}

func TestNewConfigManagerProfilingSendToMainEndpointDefault(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: main-key
site: datadoghq.com
apm_config:
  profiling_additional_endpoints:
    https://intake.profile.datadoghq.eu/api/v2/profile:
      - eu-key
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, 2, mgr.endpointsTotalLength)
	assert.ElementsMatch(t, []endpoint{
		{site: "datadoghq.eu", apiKeys: []string{"eu-key"}},
		{site: "datadoghq.com", apiKeys: []string{"main-key"}},
	}, mgr.endpoints)
}

func TestNewConfigManagerProfilingSendToMainEndpointDisabled(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: main-key
site: datadoghq.com
apm_config:
  profiling_dd_url: ://invalid-main-url
  profiling_send_to_main_endpoint: false
  profiling_additional_endpoints:
    https://intake.profile.datadoghq.eu/api/v2/profile:
      - eu-key
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, 1, mgr.endpointsTotalLength)
	assert.Equal(t, []endpoint{
		{site: "datadoghq.eu", apiKeys: []string{"eu-key"}},
	}, mgr.endpoints)
}
