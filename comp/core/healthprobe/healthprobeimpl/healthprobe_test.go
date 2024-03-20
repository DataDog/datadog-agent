// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package healthprobeimpl implements the healthprobe component interface
package healthprobeimpl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
	"gotest.tools/assert"
)

func TestServer(t *testing.T) {
	lc := fxtest.NewLifecycle(t)

	port := "7869"
	overrides := map[string]any{
		"health_port": port,
	}

	_, err := newHealthProbe(lc,
		fxutil.Test[dependencies](
			t,
			logimpl.MockModule(),
			config.MockModule(),
			fx.Replace(config.MockParams{Overrides: overrides}),
		))

	assert.NilError(t, err)

	ctx := context.Background()
	assert.NilError(t, lc.Start(ctx))

	assert.NilError(t, lc.Stop(ctx))
}

func TestLiveHandler(t *testing.T) {
	configComponent := fxutil.Test[config.Component](t, config.MockModule())
	logComponent := fxutil.Test[log.Component](t, logimpl.MockModule())

	request := httptest.NewRequest(http.MethodGet, "/live", nil)
	responseRecorder := httptest.NewRecorder()

	liveHandler{config: configComponent, log: logComponent}.ServeHTTP(responseRecorder, request)

	assert.Equal(t, http.StatusOK, responseRecorder.Code)

	assert.Equal(t, "{\"Healthy\":null,\"Unhealthy\":null}", responseRecorder.Body.String())
}

func TestLiveHandlerUnhealthy(t *testing.T) {
	configComponent := fxutil.Test[config.Component](t, config.MockModule())
	logComponent := fxutil.Test[log.Component](t, logimpl.MockModule())

	request := httptest.NewRequest(http.MethodGet, "/live", nil)
	responseRecorder := httptest.NewRecorder()

	handler := health.RegisterLiveness("fake")
	defer func() {
		health.Deregister(handler)
	}()

	liveHandler{config: configComponent, log: logComponent}.ServeHTTP(responseRecorder, request)

	assert.Equal(t, http.StatusInternalServerError, responseRecorder.Code)

	assert.Equal(t, "{\"Healthy\":[\"healthcheck\"],\"Unhealthy\":[\"fake\"]}", responseRecorder.Body.String())
}

func TestReadyHandler(t *testing.T) {
	configComponent := fxutil.Test[config.Component](t, config.MockModule())
	logComponent := fxutil.Test[log.Component](t, logimpl.MockModule())

	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	responseRecorder := httptest.NewRecorder()

	readyHandler{config: configComponent, log: logComponent}.ServeHTTP(responseRecorder, request)

	assert.Equal(t, http.StatusOK, responseRecorder.Code)

	assert.Equal(t, "{\"Healthy\":null,\"Unhealthy\":null}", responseRecorder.Body.String())
}

func TestReadyHandlerUnhealthy(t *testing.T) {
	configComponent := fxutil.Test[config.Component](t, config.MockModule())
	logComponent := fxutil.Test[log.Component](t, logimpl.MockModule())

	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	responseRecorder := httptest.NewRecorder()

	handler := health.RegisterReadiness("fake")
	defer func() {
		health.Deregister(handler)
	}()

	readyHandler{config: configComponent, log: logComponent}.ServeHTTP(responseRecorder, request)

	assert.Equal(t, http.StatusInternalServerError, responseRecorder.Code)

	assert.Equal(t, "{\"Healthy\":[\"healthcheck\"],\"Unhealthy\":[\"fake\"]}", responseRecorder.Body.String())
}
