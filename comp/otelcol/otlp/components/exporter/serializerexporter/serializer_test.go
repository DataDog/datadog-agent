// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package serializerexporter

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/config/mock"

	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestInitSerializer(t *testing.T) {
	logger := zap.NewNop()
	var sourceProvider SourceProviderFunc = func(_ context.Context) (string, error) {
		return "test", nil
	}
	cfg := &ExporterConfig{}
	s, fw, err := initSerializer(logger, cfg, sourceProvider)
	assert.Nil(t, err)
	assert.IsType(t, &defaultforwarder.DefaultForwarder{}, fw)
	assert.NotNil(t, fw)
	assert.NotNil(t, s)
}

func TestProxyConfiguration(t *testing.T) {
	tests := []struct {
		name            string
		envVars         map[string]string
		proxyURL        string
		expectedHTTP    string
		expectedHTTPS   string
		expectedNoProxy []string
	}{
		{
			name: "both HTTP_PROXY and HTTPS_PROXY set",
			envVars: map[string]string{
				"HTTP_PROXY":  "http://proxy.example.com:8080",
				"HTTPS_PROXY": "https://secure-proxy.example.com:8443",
				"NO_PROXY":    "localhost,127.0.0.1,.local",
			},
			expectedHTTP:    "http://proxy.example.com:8080",
			expectedHTTPS:   "https://secure-proxy.example.com:8443",
			expectedNoProxy: []string{"localhost", "127.0.0.1", ".local"},
			proxyURL:        "",
		},
		{
			name: "only HTTP_PROXY set",
			envVars: map[string]string{
				"HTTP_PROXY": "http://proxy.example.com:3128",
			},
			expectedHTTP:    "http://proxy.example.com:3128",
			expectedHTTPS:   "",
			expectedNoProxy: []string{""},
			proxyURL:        "",
		},
		{
			name:            "no proxy environment variables",
			envVars:         map[string]string{},
			expectedHTTP:    "",
			expectedHTTPS:   "",
			expectedNoProxy: []string{""},
			proxyURL:        "",
		},
		{
			name: "single NO_PROXY entry",
			envVars: map[string]string{
				"NO_PROXY": "internal.company.com,localhost",
			},
			expectedHTTP:    "",
			expectedHTTPS:   "",
			expectedNoProxy: []string{"internal.company.com", "localhost"},
			proxyURL:        "",
		},
		{
			name:            "only proxy_url set",
			envVars:         map[string]string{},
			expectedHTTP:    "http://proxyurl.example.com:3128",
			expectedHTTPS:   "http://proxyurl.example.com:3128",
			expectedNoProxy: []string{""},
			proxyURL:        "http://proxyurl.example.com:3128",
		},
		{
			name: "both proxy_url and proxy env vars set - proxy_url takes precedence",
			envVars: map[string]string{
				"HTTP_PROXY":  "http://proxy.example.com:8080",
				"HTTPS_PROXY": "https://secure-proxy.example.com:8443",
			},
			expectedHTTP:    "http://proxyurl.example.com:3128",
			expectedHTTPS:   "http://proxyurl.example.com:3128",
			expectedNoProxy: []string{""},
			proxyURL:        "http://proxyurl.example.com:3128",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables for the test
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Create a test config with proxy URL if specified
			cfg := &ExporterConfig{
				API: datadogconfig.APIConfig{
					Key:  "test-key",
					Site: "datadoghq.com",
				},
			}
			if tt.proxyURL != "" {
				cfg.HTTPConfig.ProxyURL = tt.proxyURL
			}

			pkgconfig := mock.New(t)

			setupSerializer(pkgconfig, cfg)

			// Verify settings match
			assert.Equal(t, tt.expectedHTTP, pkgconfig.GetString("proxy.http"))
			assert.Equal(t, tt.expectedHTTPS, pkgconfig.GetString("proxy.https"))
			assert.Equal(t, tt.expectedNoProxy, pkgconfig.GetStringSlice("proxy.no_proxy"))
		})
	}
}
