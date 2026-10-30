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
    exporter:
      verbosity: detailed
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, "detailed", mgr.hostProfilerConfig.DebugVerbosity)
}

func TestNewConfigManagerDebugFromEnvVar(t *testing.T) {
	t.Setenv("DD_HOSTPROFILER_DEBUG_EXPORTER_VERBOSITY", "detailed")

	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, "detailed", mgr.hostProfilerConfig.DebugVerbosity)
}

func TestSendErrorFrames(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		settings := ""
		if enabled {
			settings = "hostprofiler:\n  send_error_frames: true"
		}
		cfg := config.NewMockFromYAML(t, settings)
		conf := confMap{}
		buildReceivers(conf, newConfigManager(cfg))

		receivers := conf["receivers"].(map[string]any)
		profiling := receivers["profiling"].(map[string]any)
		assert.Equal(t, enabled, profiling["send_error_frames"])
	}
}

func TestFilterMinProcessAge(t *testing.T) {
	for _, tc := range []struct {
		settings string
		expected string
	}{
		{"", "0s"},
		{"hostprofiler:\n  filter_min_process_age: 5s", "5s"},
	} {
		cfg := config.NewMockFromYAML(t, tc.settings)
		conf := confMap{}
		buildReceivers(conf, newConfigManager(cfg))

		receivers := conf["receivers"].(map[string]any)
		profiling := receivers["profiling"].(map[string]any)
		assert.Equal(t, tc.expected, profiling["filter_min_process_age"])
	}
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

func TestNewConfigManagerSymbolUploaderReportIntervalFromYAML(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
hostprofiler:
  symbol_uploader:
    report:
      interval: 600
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, 600, mgr.hostProfilerConfig.SymbolUploaderReport.Interval)
}

func TestNewConfigManagerSymbolUploaderReportIntervalFromEnvVar(t *testing.T) {
	t.Setenv("DD_HOSTPROFILER_SYMBOL_UPLOADER_REPORT_INTERVAL", "900")

	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, 900, mgr.hostProfilerConfig.SymbolUploaderReport.Interval)
}

func TestNewConfigManagerSymbolUploaderReportIntervalUnsetFallsBackToSchemaDefault(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
`)
	mgr := newConfigManager(cfg)
	assert.Equal(t, 300, mgr.hostProfilerConfig.SymbolUploaderReport.Interval)
}

func TestNewConfigManagerSymbolUploaderReportMaxFromYAML(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
hostprofiler:
  symbol_uploader:
    report:
      max_paths_per_report: 42
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, 42, mgr.hostProfilerConfig.SymbolUploaderReport.MaxPathsPerReport)
}

func TestNewConfigManagerSymbolUploaderReportMaxFromEnvVar(t *testing.T) {
	t.Setenv("DD_HOSTPROFILER_SYMBOL_UPLOADER_REPORT_MAX_PATHS_PER_REPORT", "77")

	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
`)
	mgr := newConfigManager(cfg)

	assert.Equal(t, 77, mgr.hostProfilerConfig.SymbolUploaderReport.MaxPathsPerReport)
}

func TestNewConfigManagerSymbolUploaderReportMaxUnsetFallsBackToSchemaDefault(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
`)
	mgr := newConfigManager(cfg)
	assert.Equal(t, 100, mgr.hostProfilerConfig.SymbolUploaderReport.MaxPathsPerReport)
}

func TestNewConfigManagerSymbolUploaderReportInvalidValuesCoercedToZero(t *testing.T) {
	cfg := config.NewMockFromYAML(t, `
api_key: test-key
site: datadoghq.com
hostprofiler:
  symbol_uploader:
    report:
      interval: abc
      max_paths_per_report: abc
`)
	mgr := newConfigManager(cfg)
	assert.Equal(t, 0, mgr.hostProfilerConfig.SymbolUploaderReport.Interval)
	assert.Equal(t, 0, mgr.hostProfilerConfig.SymbolUploaderReport.MaxPathsPerReport)
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
