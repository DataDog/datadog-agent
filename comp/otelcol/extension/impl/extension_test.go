// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package extensionimpl defines the OpenTelemetry Extension implementation.
package extensionimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	collectorcontribimpl "github.com/DataDog/datadog-agent/comp/otelcol/collector-contrib/impl"
	configstore "github.com/DataDog/datadog-agent/comp/otelcol/configstore/impl"
	extension "github.com/DataDog/datadog-agent/comp/otelcol/extension/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/otelcol"
)

var cpSettings = otelcol.ConfigProviderSettings{
	ResolverSettings: confmap.ResolverSettings{
		URIs: []string{filepath.Join("testdata", "config.yaml")},
		ProviderFactories: []confmap.ProviderFactory{
			fileprovider.NewFactory(),
			yamlprovider.NewFactory(),
		},
	},
}

func getExtensionTestConfig(t *testing.T) *Config {
	cf, err := configstore.NewConfigStore()
	assert.NoError(t, err)

	factories, err := collectorcontribimpl.NewComponent().OTelComponentFactories()
	assert.NoError(t, err)

	cf.AddConfigs(cpSettings, cpSettings, factories)
	return &Config{
		HTTPConfig: &confighttp.ServerConfig{
			Endpoint: "localhost:0",
		},
		ConfigStore: cf,
	}
}

func getTestExtension(t *testing.T) (extension.Component, error) {
	c := context.Background()
	telemetry := component.TelemetrySettings{}
	info := component.NewDefaultBuildInfo()
	cfg := getExtensionTestConfig(t)

	return NewExtension(c, cfg, telemetry, info)
}

func TestNewExtension(t *testing.T) {
	ext, err := getTestExtension(t)
	assert.NoError(t, err)
	assert.NotNil(t, ext)

	_, ok := ext.(*ddExtension)
	assert.True(t, ok)
}

func TestExtensionHTTPHandler(t *testing.T) {
	// Create a request
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create a ResponseRecorder
	rr := httptest.NewRecorder()

	// Create an instance of your handler
	ext, err := getTestExtension(t)
	require.NoError(t, err)

	ddExt := ext.(*ddExtension)

	// Call the handler's ServeHTTP method
	ddExt.ServeHTTP(rr, req)

	// Check the response status code
	assert.Equalf(t, http.StatusOK, rr.Code,
		"handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)

	// Check the response body
	expectedKeys := []string{
		"version",
		"command",
		"description",
		"extension_version",
		"provided_configuration",
		"full_configuration",
		"runtime_override_configuration",
		"environment_variable_configuration",
		"environment",
	}
	var response map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &response)

	for _, key := range expectedKeys {
		_, ok := response[key]
		assert.True(t, ok)
	}

	// There will be no sources configured and thus, that key
	// should not be present
	_, ok := response["sources"]
	assert.False(t, ok)
}
