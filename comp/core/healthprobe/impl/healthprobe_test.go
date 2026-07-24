// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package impl implements the healthprobe component interface
package healthprobeimpl

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	healthprobeComponent "github.com/DataDog/datadog-agent/comp/core/healthprobe/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

func TestServer(t *testing.T) {
	reserved, err := net.Listen("tcp", "0.0.0.0:0")
	require.NoError(t, err)
	port := reserved.Addr().(*net.TCPAddr).Port
	require.NoError(t, reserved.Close())

	lc := compdef.NewTestLifecycle(t)
	logComponent := logmock.New(t)

	requires := Requires{
		Lc:  lc,
		Log: logComponent,
		Options: healthprobeComponent.Options{
			Port: port,
		},
	}

	provides, err := NewComponent(requires)

	require.NoError(t, err)

	require.NotNil(t, provides.Comp)

	beforeStart, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	require.NoError(t, err, "constructing the component must not claim the health port")
	require.NoError(t, beforeStart.Close())

	ctx := context.Background()

	lc.AssertHooksNumber(1)
	require.NoError(t, lc.Start(ctx))
	conflicting, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	require.Error(t, err, "starting the component must claim the health port")
	if conflicting != nil {
		require.NoError(t, conflicting.Close())
	}
	require.NoError(t, lc.Stop(ctx))
}

func TestServerNoHealthPort(t *testing.T) {
	lc := compdef.NewTestLifecycle(t)
	logComponent := logmock.New(t)

	requires := Requires{
		Lc:  lc,
		Log: logComponent,
		Options: healthprobeComponent.Options{
			Port: 0,
		},
	}

	provides, err := NewComponent(requires)

	assert.NoError(t, err)

	assert.Nil(t, provides.Comp)
}

func TestLiveHandler(t *testing.T) {
	logComponent := logmock.New(t)

	request := httptest.NewRequest(http.MethodGet, "/live", nil)
	responseRecorder := httptest.NewRecorder()

	liveHandler{logsGoroutines: false, log: logComponent}.ServeHTTP(responseRecorder, request)

	assert.Equal(t, http.StatusOK, responseRecorder.Code)

	assert.Equal(t, "{\"Healthy\":null,\"Unhealthy\":null}", responseRecorder.Body.String())
}

func TestLiveHandlerUnhealthy(t *testing.T) {
	logComponent := logmock.New(t)

	request := httptest.NewRequest(http.MethodGet, "/live", nil)
	responseRecorder := httptest.NewRecorder()

	handler := health.RegisterLiveness("fake")
	defer func() {
		health.Deregister(handler)
	}()

	liveHandler{logsGoroutines: false, log: logComponent}.ServeHTTP(responseRecorder, request)

	assert.Equal(t, http.StatusInternalServerError, responseRecorder.Code)

	assert.Equal(t, "{\"Healthy\":[\"healthcheck\"],\"Unhealthy\":[\"fake\"]}", responseRecorder.Body.String())
}

func TestReadyHandler(t *testing.T) {
	logComponent := logmock.New(t)

	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	responseRecorder := httptest.NewRecorder()

	readyHandler{logsGoroutines: false, log: logComponent}.ServeHTTP(responseRecorder, request)

	assert.Equal(t, http.StatusOK, responseRecorder.Code)

	assert.Equal(t, "{\"Healthy\":null,\"Unhealthy\":null}", responseRecorder.Body.String())
}

func TestReadyHandlerUnhealthy(t *testing.T) {
	logComponent := logmock.New(t)

	request := httptest.NewRequest(http.MethodGet, "/ready", nil)
	responseRecorder := httptest.NewRecorder()

	handler := health.RegisterReadiness("fake")
	defer func() {
		health.Deregister(handler)
	}()

	readyHandler{logsGoroutines: false, log: logComponent}.ServeHTTP(responseRecorder, request)

	assert.Equal(t, http.StatusInternalServerError, responseRecorder.Code)

	assert.Equal(t, "{\"Healthy\":[\"healthcheck\"],\"Unhealthy\":[\"fake\"]}", responseRecorder.Body.String())
}

func TestHealthHandlerFails(t *testing.T) {
	logComponent := logmock.New(t)

	request := httptest.NewRequest(http.MethodGet, "/live", nil)
	responseRecorder := httptest.NewRecorder()

	healthHandler(false, logComponent, func() (health.Status, error) {
		return health.Status{}, errors.New("fail to extract status")
	}, responseRecorder, request)

	assert.Equal(t, http.StatusInternalServerError, responseRecorder.Code)
	assert.Equal(t, "{\"error\":\"fail to extract status\"}\n", responseRecorder.Body.String())
}
