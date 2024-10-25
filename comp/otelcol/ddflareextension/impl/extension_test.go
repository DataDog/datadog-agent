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
	"testing"

	ddflareextension "github.com/DataDog/datadog-agent/comp/otelcol/ddflareextension/def"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/confighttp"

	"go.uber.org/zap"
)

func getExtensionTestConfig(t *testing.T) *Config {
	factories, err := components()
	assert.NoError(t, err)
	return &Config{
		HTTPConfig: &confighttp.ServerConfig{
			Endpoint: "localhost:0",
		},
		configProviderSettings: newConfigProviderSettings(uriFromFile("config.yaml"), false),
		factories:              &factories,
	}
}

func getTestExtension(t *testing.T) (ddflareextension.Component, error) {
	c := context.Background()
	telemetry := component.TelemetrySettings{}
	info := component.NewDefaultBuildInfo()
	cfg := getExtensionTestConfig(t)

	return NewExtension(c, cfg, telemetry, info, false)
}

func getResponseToHandlerRequest(t *testing.T, tokenOverride string) *httptest.ResponseRecorder {

	// Create a request
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	token := apiutil.GetAuthToken()
	if tokenOverride != "" {
		token = tokenOverride
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

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

	conf := confmapFromResolverSettings(t, newResolverSettings(uriFromFile("config.yaml"), false))
	ddExt.NotifyConfig(context.TODO(), conf)
	assert.NoError(t, err)

	handler := ddExt.server.srv.Handler

	// Call the handler's ServeHTTP method
	handler.ServeHTTP(rr, req)

	return rr
}

func TestNewExtension(t *testing.T) {
	ext, err := getTestExtension(t)
	assert.NoError(t, err)
	assert.NotNil(t, ext)

	_, ok := ext.(*ddExtension)
	assert.True(t, ok)
}

func TestExtensionHTTPHandler(t *testing.T) {
	conf := configmock.New(t)
	err := apiutil.CreateAndSetAuthToken(conf)
	if err != nil {
		t.Fatal(err)
	}

	rr := getResponseToHandlerRequest(t, "")

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

func TestExtensionHTTPHandlerBadToken(t *testing.T) {
	conf := configmock.New(t)
	err := apiutil.CreateAndSetAuthToken(conf)
	if err != nil {
		t.Fatal(err)
	}

	rr := getResponseToHandlerRequest(t, "badtoken")

	// Check the response status code
	assert.Equalf(t, http.StatusForbidden, rr.Code,
		"handler returned wrong status code: got %v want %v", rr.Code, http.StatusForbidden)

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
