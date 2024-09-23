// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddflareextensionimpl defines the OpenTelemetry Extension implementation.
package ddflareextensionimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	ddflareextension "github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configstore "github.com/DataDog/datadog-agent/comp/otelcol/configstore/impl"
	spanmetricsconnector "github.com/open-telemetry/opentelemetry-collector-contrib/connector/spanmetricsconnector"
	healthcheckextension "github.com/open-telemetry/opentelemetry-collector-contrib/extension/healthcheckextension"
	pprofextension "github.com/open-telemetry/opentelemetry-collector-contrib/extension/pprofextension"
	transformprocessor "github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/connector"
	"go.opentelemetry.io/collector/exporter"
	otlpexporter "go.opentelemetry.io/collector/exporter/otlpexporter"
	otlphttpexporter "go.opentelemetry.io/collector/exporter/otlphttpexporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	batchprocessor "go.opentelemetry.io/collector/processor/batchprocessor"
	"go.opentelemetry.io/collector/receiver"
	nopreceiver "go.opentelemetry.io/collector/receiver/nopreceiver"
	otlpreceiver "go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.uber.org/zap"
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

func components() (otelcol.Factories, error) {
	var err error
	factories := otelcol.Factories{}

	factories.Extensions, err = extension.MakeFactoryMap(
		healthcheckextension.NewFactory(),
		pprofextension.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	factories.Receivers, err = receiver.MakeFactoryMap(
		nopreceiver.NewFactory(),
		otlpreceiver.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	factories.Exporters, err = exporter.MakeFactoryMap(
		otlpexporter.NewFactory(),
		otlphttpexporter.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	factories.Processors, err = processor.MakeFactoryMap(
		batchprocessor.NewFactory(),
		transformprocessor.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	factories.Connectors, err = connector.MakeFactoryMap(
		spanmetricsconnector.NewFactory(),
	)
	if err != nil {
		return otelcol.Factories{}, err
	}

	return factories, nil
}

func getExtensionTestConfig(t *testing.T) *Config {
	cf, err := configstore.NewConfigStore()
	assert.NoError(t, err)

	factories, err := components()
	assert.NoError(t, err)

	cf.AddConfigs(cpSettings, cpSettings, factories)
	return &Config{
		HTTPConfig: &confighttp.ServerConfig{
			Endpoint: "localhost:0",
		},
		ConfigStore: cf,
	}
}

func getTestExtension(t *testing.T) (ddflareextension.Component, error) {
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
	ddExt.telemetry.Logger = zap.New(zap.NewNop().Core())

	host := newHostWithExtensions(
		map[component.ID]component.Component{
			component.MustNewIDWithName("pprof", "custom"): nil,
		},
	)

	ddExt.Start(context.TODO(), host)

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
		"sources",
	}
	var response map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &response)

	for _, key := range expectedKeys {
		_, ok := response[key]
		assert.True(t, ok)
	}
}

type hostWithExtensions struct {
	component.Host
	exts map[component.ID]component.Component
}

func newHostWithExtensions(exts map[component.ID]component.Component) component.Host {
	return &hostWithExtensions{
		Host: componenttest.NewNopHost(),
		exts: exts,
	}
}

func (h *hostWithExtensions) GetExtensions() map[component.ID]component.Component {
	return h.exts
}
