// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package hpflareextension defines the OpenTelemetry Extension implementation.
package hpflareextension

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/confmap"

	"go.uber.org/zap"
)

func getTestExtension(optIpc ipc.Component) (*DDExtension, error) {
	telemetry := component.TelemetrySettings{}
	cfg := &Config{
		HTTPConfig: &confighttp.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  "localhost:0",
				Transport: confignet.TransportTypeTCP,
			},
		},
	}

	return NewExtension(cfg, optIpc, telemetry)
}

func getResponseToHandlerRequest(t *testing.T, ipc ipc.Component, tokenOverride string) *httptest.ResponseRecorder {

	// Create a request
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	token := ipc.GetAuthToken()
	if tokenOverride != "" {
		token = tokenOverride
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	// Create a ResponseRecorder
	rr := httptest.NewRecorder()

	// Create an instance of your handler
	ddExt, err := getTestExtension(ipc)
	require.NoError(t, err)

	ddExt.telemetry.Logger = zap.New(zap.NewNop().Core())

	host := componenttest.NewNopHost()

	ddExt.Start(context.TODO(), host)

	conf := confmap.NewFromStringMap(map[string]any{
		"extensions": []any{"hpflare"},
	})
	ddExt.NotifyConfig(context.TODO(), conf)
	assert.NoError(t, err)

	handler := ddExt.server.srv.Handler

	// Call the handler's ServeHTTP method
	handler.ServeHTTP(rr, req)

	return rr
}

func TestExtensionHTTPHandler(t *testing.T) {
	ipc := ipcmock.New(t)

	rr := getResponseToHandlerRequest(t, ipc, "")

	// Check the response status code
	assert.Equalf(t, http.StatusOK, rr.Code,
		"handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)

	// Check the response body
	expectedKeys := []string{
		"config",
	}
	var response map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &response)

	for _, key := range expectedKeys {
		_, ok := response[key]
		assert.True(t, ok)
	}
}

func TestExtensionHTTPHandlerBadToken(t *testing.T) {
	ipc := ipcmock.New(t)

	rr := getResponseToHandlerRequest(t, ipc, "badtoken")

	// Check the response status code
	assert.Equalf(t, http.StatusForbidden, rr.Code,
		"handler returned wrong status code: got %v want %v", rr.Code, http.StatusForbidden)

}
