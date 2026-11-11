// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package daemon

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestRestrictedLocalAPIHandler(t *testing.T) {
	cfg := configmock.New(t)
	d := &testDaemon{}

	d.On("GetPackage", "datadog-agent-integration-deps", "").Return(Package{
		Name:    "datadog-agent-integration-deps",
		Version: "7.99.0-1",
		URL:     "oci://registry/datadog-agent-integration-deps:7.99.0-1",
	}, nil).Once()
	d.On("Install", mock.Anything, "oci://registry/datadog-agent-integration-deps:7.99.0-1", mock.Anything).Return(nil).Once()
	d.On("Remove", mock.Anything, "datadog-agent-integration-deps").Return(nil).Once()

	impl := &restrictedLocalAPIImpl{daemon: d, config: cfg}
	server := httptest.NewServer(impl.handler())
	defer server.Close()

	post := func(path string) int {
		resp, err := http.Post(server.URL+path, "application/json", strings.NewReader(`{}`))
		require.NoError(t, err)
		defer resp.Body.Close()
		return resp.StatusCode
	}

	assert.Equal(t, http.StatusOK, post("/datadog-agent-integration-deps/install"))
	assert.Equal(t, http.StatusOK, post("/datadog-agent-integration-deps/remove"))

	assert.Equal(t, http.StatusForbidden, post("/datadog-agent/remove"))
	assert.Equal(t, http.StatusForbidden, post("/datadog-agent/install"))
	d.AssertExpectations(t)
}

func TestRestrictedLocalAPIDefaultPackageWhenUnset(t *testing.T) {
	cfg := configmock.New(t)
	d := &testDaemon{}
	d.On("GetPackage", "datadog-agent-integration-deps", "").Return(Package{
		Name:    "datadog-agent-integration-deps",
		Version: "7.99.0-1",
		URL:     "oci://registry/datadog-agent-integration-deps:7.99.0-1",
	}, nil).Once()
	d.On("Install", mock.Anything, "oci://registry/datadog-agent-integration-deps:7.99.0-1", mock.Anything).Return(nil).Once()

	impl := &restrictedLocalAPIImpl{daemon: d, config: cfg}
	server := httptest.NewServer(impl.handler())
	defer server.Close()

	resp, err := http.Post(server.URL+"/datadog-agent-integration-deps/install", "application/json", strings.NewReader(`{}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	d.AssertExpectations(t)
}
