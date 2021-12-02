// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

// TestParseReplaceRules tests the compileReplaceRules helper function.
func TestParseRepaceRules(t *testing.T) {
	assert := assert.New(t)
	rules := []*ReplaceRule{
		{Name: "http.url", Pattern: "(token/)([^/]*)", Repl: "${1}?"},
		{Name: "http.url", Pattern: "guid", Repl: "[REDACTED]"},
		{Name: "custom.tag", Pattern: "(/foo/bar/).*", Repl: "${1}extra"},
	}
	err := compileReplaceRules(rules)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		assert.Equal(r.Pattern, r.Re.String())
	}
}

func TestSplitTag(t *testing.T) {
	for _, tt := range []struct {
		tag string
		kv  *Tag
	}{
		{
			tag: "",
			kv:  &Tag{K: ""},
		},
		{
			tag: "key:value",
			kv:  &Tag{K: "key", V: "value"},
		},
		{
			tag: "env:prod",
			kv:  &Tag{K: "env", V: "prod"},
		},
		{
			tag: "env:staging:east",
			kv:  &Tag{K: "env", V: "staging:east"},
		},
		{
			tag: "key",
			kv:  &Tag{K: "key"},
		},
	} {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, splitTag(tt.tag), tt.kv)
		})
	}
}

// mockConfig is a single config entry mocking util with automatic reset
//
// Usage: defer mockConfig(key, new_value)()
// will automatically revert previous once current scope exits
func mockConfig(k string, v interface{}) func() {
	oldConfig := config.Datadog
	config.Mock().Set(k, v)
	return func() { config.Datadog = oldConfig }
}

func TestTelemetryEndpointsConfig(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := New()
		err := cfg.applyDatadogConfig()

		assert.NoError(t, err)
		assert.True(t, cfg.TelemetryConfig.Enabled)
		assert.Len(t, cfg.TelemetryConfig.Endpoints, 1)
		assert.Equal(t, "instrumentation-telemetry-intake.datadoghq.com", cfg.TelemetryConfig.Endpoints[0].Host)
	})

	t.Run("dd_url", func(t *testing.T) {
		defer mockConfig("apm_config.telemetry.dd_url", "http://example.com")()

		cfg := New()
		err := cfg.applyDatadogConfig()

		assert.NoError(t, err)
		assert.True(t, cfg.TelemetryConfig.Enabled)
		assert.Equal(t, "example.com", cfg.TelemetryConfig.Endpoints[0].Host)
	})

	t.Run("dd_url-fail", func(t *testing.T) {
		defer mockConfig("apm_config.telemetry.dd_url", "111://abc.com")()

		cfg := New()
		err := cfg.applyDatadogConfig()

		assert.NoError(t, err)
		assert.False(t, cfg.TelemetryConfig.Enabled)
	})

	t.Run("site", func(t *testing.T) {
		defer mockConfig("site", "new_site.example.com")()

		cfg := New()
		err := cfg.applyDatadogConfig()

		assert.NoError(t, err)
		assert.True(t, cfg.TelemetryConfig.Enabled)
		assert.Len(t, cfg.TelemetryConfig.Endpoints, 1)
		assert.Equal(t, "instrumentation-telemetry-intake.new_site.example.com", cfg.TelemetryConfig.Endpoints[0].Host)
	})

	t.Run("additional-hosts", func(t *testing.T) {
		additionalEndpoints := make(map[string]string)
		additionalEndpoints["test_backend_2.example.com"] = "test_apikey_2"
		additionalEndpoints["test_backend_3.example.com"] = "test_apikey_3"

		defer mockConfig("apm_config.telemetry.additional_endpoints", additionalEndpoints)()

		cfg := New()
		err := cfg.applyDatadogConfig()

		assert.NoError(t, err)
		assert.Equal(t, "instrumentation-telemetry-intake.datadoghq.com", cfg.TelemetryConfig.Endpoints[0].Host)

		assert.True(t, cfg.TelemetryConfig.Enabled)
		assert.Len(t, cfg.TelemetryConfig.Endpoints, 3)

		for _, endpoint := range cfg.TelemetryConfig.Endpoints[1:] {
			assert.NotNil(t, additionalEndpoints[endpoint.Host])
			assert.Equal(t, endpoint.APIKey, additionalEndpoints[endpoint.Host])
		}
	})

	t.Run("additional-urls", func(t *testing.T) {
		additionalEndpoints := make(map[string]string)
		additionalEndpoints["http://test_backend_2.example.com"] = "test_apikey_2"
		additionalEndpoints["http://test_backend_3.example.com"] = "test_apikey_3"

		defer mockConfig("apm_config.telemetry.additional_endpoints", additionalEndpoints)()

		cfg := New()
		err := cfg.applyDatadogConfig()

		assert.NoError(t, err)
		assert.Equal(t, "instrumentation-telemetry-intake.datadoghq.com", cfg.TelemetryConfig.Endpoints[0].Host)

		assert.True(t, cfg.TelemetryConfig.Enabled)
		assert.Len(t, cfg.TelemetryConfig.Endpoints, 3)

		for _, endpoint := range cfg.TelemetryConfig.Endpoints[1:] {
			assert.NotNil(t, additionalEndpoints["http://"+endpoint.Host])
			assert.Equal(t, endpoint.APIKey, additionalEndpoints["http://"+endpoint.Host])
		}
	})

	t.Run("skip-additional", func(t *testing.T) {
		additionalEndpoints := make(map[string]string)
		additionalEndpoints["11://test_backend_2.example.com///"] = "test_apikey_2"
		additionalEndpoints["http://test_backend_3.example.com/"] = "test_apikey_3"

		defer mockConfig("apm_config.telemetry.additional_endpoints", additionalEndpoints)()
		cfg := New()
		err := cfg.applyDatadogConfig()
		assert.NoError(t, err)

		assert.True(t, cfg.TelemetryConfig.Enabled)
		assert.Len(t, cfg.TelemetryConfig.Endpoints, 2)
		assert.Equal(t, "instrumentation-telemetry-intake.datadoghq.com", cfg.TelemetryConfig.Endpoints[0].Host)
		assert.Equal(t, "test_backend_3.example.com", cfg.TelemetryConfig.Endpoints[1].Host)
	})
}
