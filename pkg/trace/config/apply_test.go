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

func TestTelemetryEndpointsConfig(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cfg := New()
		err := cfg.applyDatadogConfig()

		assert := assert.New(t)
		assert.NoError(err)
		assert.True(cfg.TelemetryConfig.Enabled)
		assert.Len(cfg.TelemetryConfig.Endpoints, 1)
		assert.Equal("https://instrumentation-telemetry-intake.datadoghq.com", cfg.TelemetryConfig.Endpoints[0].Host)
	})

	t.Run("dd_url", func(t *testing.T) {
		defer cleanConfig()
		config.Datadog.Set("apm_config.telemetry.dd_url", "http://example.com/")

		cfg := New()
		err := cfg.applyDatadogConfig()

		assert := assert.New(t)
		assert.NoError(err)
		assert.True(cfg.TelemetryConfig.Enabled)
		assert.Equal("http://example.com/", cfg.TelemetryConfig.Endpoints[0].Host)
	})

	t.Run("dd_url-malformed", func(t *testing.T) {
		defer cleanConfig()
		config.Datadog.Set("apm_config.telemetry.dd_url", "111://abc.com")

		cfg := New()
		err := cfg.applyDatadogConfig()

		assert := assert.New(t)
		assert.NoError(err)
		assert.True(cfg.TelemetryConfig.Enabled)
		assert.Equal(cfg.TelemetryConfig.Endpoints[0].Host, "111://abc.com")
	})

	t.Run("site", func(t *testing.T) {
		defer cleanConfig()
		config.Datadog.Set("site", "new_site.example.com")

		cfg := New()
		err := cfg.applyDatadogConfig()
		assert := assert.New(t)
		assert.NoError(err)
		assert.True(cfg.TelemetryConfig.Enabled)
		assert.Len(cfg.TelemetryConfig.Endpoints, 1)
		assert.Equal("https://instrumentation-telemetry-intake.new_site.example.com", cfg.TelemetryConfig.Endpoints[0].Host)
	})

	t.Run("additional-hosts", func(t *testing.T) {
		defer cleanConfig()
		additionalEndpoints := map[string]string{
			"http://test_backend_2.example.com": "test_apikey_2",
			"http://test_backend_3.example.com": "test_apikey_3",
		}
		config.Datadog.Set("apm_config.telemetry.additional_endpoints", additionalEndpoints)

		cfg := New()
		err := cfg.applyDatadogConfig()

		assert := assert.New(t)
		assert.NoError(err)
		assert.Equal("https://instrumentation-telemetry-intake.datadoghq.com", cfg.TelemetryConfig.Endpoints[0].Host)

		assert.True(cfg.TelemetryConfig.Enabled)
		assert.Len(cfg.TelemetryConfig.Endpoints, 3)

		for _, endpoint := range cfg.TelemetryConfig.Endpoints[1:] {
			assert.NotNil(additionalEndpoints[endpoint.Host])
			assert.Equal(endpoint.APIKey, additionalEndpoints[endpoint.Host])
		}
	})

	t.Run("additional-urls", func(t *testing.T) {
		defer cleanConfig()
		additionalEndpoints := map[string]string{
			"http://test_backend_2.example.com": "test_apikey_2",
			"http://test_backend_3.example.com": "test_apikey_3",
		}
		config.Datadog.Set("apm_config.telemetry.additional_endpoints", additionalEndpoints)

		cfg := New()
		err := cfg.applyDatadogConfig()

		assert := assert.New(t)
		assert.NoError(err)
		assert.Equal("https://instrumentation-telemetry-intake.datadoghq.com", cfg.TelemetryConfig.Endpoints[0].Host)
		assert.True(cfg.TelemetryConfig.Enabled)
		assert.Len(cfg.TelemetryConfig.Endpoints, 3)
		for _, endpoint := range cfg.TelemetryConfig.Endpoints[1:] {
			assert.NotNil(additionalEndpoints[endpoint.Host])
			assert.Equal(endpoint.APIKey, additionalEndpoints[endpoint.Host])
		}
	})

	t.Run("keep-malformed", func(t *testing.T) {
		defer cleanConfig()
		additionalEndpoints := map[string]string{
			"11://test_backend_2.example.com///": "test_apikey_2",
			"http://test_backend_3.example.com/": "test_apikey_3",
		}
		config.Datadog.Set("apm_config.telemetry.additional_endpoints", additionalEndpoints)

		cfg := New()
		err := cfg.applyDatadogConfig()
		assert := assert.New(t)
		assert.NoError(err)

		assert.True(cfg.TelemetryConfig.Enabled)
		assert.Len(cfg.TelemetryConfig.Endpoints, 3)
		assert.Equal("https://instrumentation-telemetry-intake.datadoghq.com", cfg.TelemetryConfig.Endpoints[0].Host)
		for _, endpoint := range cfg.TelemetryConfig.Endpoints[1:] {
			assert.Contains(additionalEndpoints, endpoint.Host)
		}
	})
}
