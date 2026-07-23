// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build test

package datadogexporter

import (
	"testing"
	"time"

	datadogconfig "github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/datadogconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
)

// TestBuildMetricsExporterConfig_HTTPPassThrough verifies that user-configured
// HTTP settings (timeout, and by extension proxy/TLS/headers carried in
// ClientConfig) are forwarded to the serializer exporter config.
func TestBuildMetricsExporterConfig_HTTPPassThrough(t *testing.T) {
	cfg, ok := CreateDefaultConfig().(*datadogconfig.Config)
	require.True(t, ok)
	cfg.ClientConfig.Timeout = 42 * time.Second

	ex := buildMetricsExporterConfig(cfg, nil)

	assert.Equal(t, 42*time.Second, ex.HTTPConfig.Timeout,
		"HTTPConfig.Timeout should reflect the user-configured value")
	assert.Equal(t, 42*time.Second, ex.TimeoutConfig.Timeout,
		"TimeoutConfig.Timeout should match HTTPConfig.Timeout")
}

// TestBuildMetricsExporterConfig_UpstreamDefaultHTTPTimeout verifies that the
// upstream datadogconfig default (15 s) is preserved when the user hasn't
// explicitly overridden http.timeout. Our 20 s fallback only activates when
// the timeout is programmatically zero.
func TestBuildMetricsExporterConfig_UpstreamDefaultHTTPTimeout(t *testing.T) {
	cfg, ok := CreateDefaultConfig().(*datadogconfig.Config)
	require.True(t, ok)
	// datadogconfig.CreateDefaultConfig() sets ClientConfig.Timeout = 15 s.

	ex := buildMetricsExporterConfig(cfg, nil)

	assert.Equal(t, 15*time.Second, ex.HTTPConfig.Timeout,
		"HTTPConfig.Timeout should preserve the upstream 15 s default")
	assert.Equal(t, 15*time.Second, ex.TimeoutConfig.Timeout)
}

// TestBuildMetricsExporterConfig_ZeroTimeoutFallback verifies that when
// ClientConfig.Timeout is explicitly zero (meaning "no timeout"), we apply
// a 20 s safety fallback so the HTTP client doesn't hang indefinitely.
func TestBuildMetricsExporterConfig_ZeroTimeoutFallback(t *testing.T) {
	cfg, ok := CreateDefaultConfig().(*datadogconfig.Config)
	require.True(t, ok)
	cfg.ClientConfig.Timeout = 0 // explicitly disable upstream default

	ex := buildMetricsExporterConfig(cfg, nil)

	assert.Equal(t, 20*time.Second, ex.HTTPConfig.Timeout,
		"HTTPConfig.Timeout should fall back to 20 s when explicitly set to zero")
	assert.Equal(t, 20*time.Second, ex.TimeoutConfig.Timeout)
}

// TestBuildMetricsExporterConfig_APIPassThrough verifies that the API key and
// site are forwarded from the datadogexporter config to the serializer exporter
// config so that the serializerexporter can create its own sync serializer in
// the DDOT path (needed when UseSyncForwarder is enabled).
func TestBuildMetricsExporterConfig_APIPassThrough(t *testing.T) {
	cfg, ok := CreateDefaultConfig().(*datadogconfig.Config)
	require.True(t, ok)
	cfg.API.Key = "secret-key"
	cfg.API.Site = "datadoghq.eu"

	ex := buildMetricsExporterConfig(cfg, nil)

	assert.Equal(t, datadogconfig.APIConfig{Key: "secret-key", Site: "datadoghq.eu"}, ex.API)
}

// TestBuildMetricsExporterConfig_RetryDefaultsToLegacyBudget verifies that
// when the user hasn't configured retry_on_failure, the serializer exporter
// uses the legacy forwarder retry budget (2-64s / 15 min) rather than the
// OTel defaults (5s / 30s / 5 min). This preserves the pre-sync-forwarder
// behavior for DDOT deployments.
func TestBuildMetricsExporterConfig_RetryDefaultsToLegacyBudget(t *testing.T) {
	cfg, ok := CreateDefaultConfig().(*datadogconfig.Config)
	require.True(t, ok)
	// Do not modify cfg.BackOffConfig — use whatever CreateDefaultConfig sets.

	ex := buildMetricsExporterConfig(cfg, nil)

	legacyDefaults := serializerexporter.DefaultAgentRetryConfig()
	assert.Equal(t, legacyDefaults.InitialInterval, ex.RetryConfig.InitialInterval,
		"InitialInterval should default to legacy 2s, not OTel 5s")
	assert.Equal(t, legacyDefaults.MaxInterval, ex.RetryConfig.MaxInterval,
		"MaxInterval should default to legacy 64s, not OTel 30s")
	assert.Equal(t, legacyDefaults.MaxElapsedTime, ex.RetryConfig.MaxElapsedTime,
		"MaxElapsedTime should default to legacy 15 min, not OTel 5 min")
}

// TestBuildMetricsExporterConfig_RetryPassThrough verifies that the
// retry_on_failure settings from the datadogexporter config are forwarded to
// the serializer exporter's RetryConfig when they differ from OTel defaults,
// so the OTel exporterhelper retry layer honours explicit user overrides.
func TestBuildMetricsExporterConfig_RetryPassThrough(t *testing.T) {
	cfg, ok := CreateDefaultConfig().(*datadogconfig.Config)
	require.True(t, ok)
	// Use values that differ from OTel defaults so they are treated as
	// explicit user overrides rather than "not configured".
	cfg.BackOffConfig.Enabled = true
	cfg.BackOffConfig.InitialInterval = 3 * time.Second
	cfg.BackOffConfig.MaxInterval = 60 * time.Second
	cfg.BackOffConfig.MaxElapsedTime = 10 * time.Minute // differs from OTel default (5m)

	ex := buildMetricsExporterConfig(cfg, nil)

	assert.True(t, ex.RetryConfig.Enabled)
	assert.Equal(t, 3*time.Second, ex.RetryConfig.InitialInterval)
	assert.Equal(t, 60*time.Second, ex.RetryConfig.MaxInterval)
	assert.Equal(t, 10*time.Minute, ex.RetryConfig.MaxElapsedTime)
}
