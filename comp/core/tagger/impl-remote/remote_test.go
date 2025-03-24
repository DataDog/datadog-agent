// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteimpl

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
)

// TestNewComponent tests that the Remote Tagger can be instantiated and started.
func TestNewComponent(t *testing.T) {
	// Skip this test if not running in CI, as it may conflict with another Agent.
	if os.Getenv("CI") != "true" {
		t.Skip("Skipping test as it is not running in CI.")
	}
	if runtime.GOOS == "darwin" {
		t.Skip("Skipping test on macOS runners with an existing Agent.")
	}

	// Start a mock gRPC server.
	grpcServer, authToken, grpcErr := grpc.NewMockGrpcSecureServer("5001")
	require.NoError(t, grpcErr)
	defer grpcServer.Stop()

	// Instantiate the component.
	req := Requires{
		Lc:     compdef.NewTestLifecycle(t),
		Config: configmock.New(t),
		Log:    logmock.New(t),
		Params: tagger.RemoteParams{
			RemoteTarget: func(config.Component) (string, error) { return ":5001", nil },
			RemoteTokenFetcher: func(config.Component) func() (string, error) {
				return func() (string, error) {
					return authToken, nil
				}
			},
		},
		Telemetry: nooptelemetry.GetCompatComponent(),
	}
	_, err := NewComponent(req)
	require.NoError(t, err)
}

// TestNewComponentNonBlocking tests that the Remote Tagger instantiation does not block when the gRPC server is not available.
func TestNewComponentNonBlocking(t *testing.T) {
	// Instantiate the component.
	req := Requires{
		Lc:     compdef.NewTestLifecycle(t),
		Config: configmock.New(t),
		Log:    logmock.New(t),
		Params: tagger.RemoteParams{
			RemoteTarget: func(config.Component) (string, error) { return ":5001", nil },
			RemoteTokenFetcher: func(config.Component) func() (string, error) {
				return func() (string, error) {
					return "", nil
				}
			},
		},
		Telemetry: nooptelemetry.GetCompatComponent(),
	}
	_, err := NewComponent(req)
	require.NoError(t, err)
}

// TestNewComponentSetsTaggerListEndpoint tests the Remote Tagger tagger-list endpoint.
func TestNewComponentSetsTaggerListEndpoint(t *testing.T) {
	// Instantiate the component.
	req := Requires{
		Lc:     compdef.NewTestLifecycle(t),
		Config: configmock.New(t),
		Log:    logmock.New(t),
		Params: tagger.RemoteParams{
			RemoteTarget: func(config.Component) (string, error) { return ":5001", nil },
			RemoteTokenFetcher: func(config.Component) func() (string, error) {
				return func() (string, error) {
					return "something", nil
				}
			},
		},
		Telemetry: nooptelemetry.GetCompatComponent(),
	}
	provides, err := NewComponent(req)
	require.NoError(t, err)

	endpointProvider := provides.Endpoint.Provider

	assert.Equal(t, []string{"GET"}, endpointProvider.Methods())
	assert.Equal(t, "/tagger-list", endpointProvider.Route())

	// Create a test server with the endpoint handler
	server := httptest.NewServer(endpointProvider.HandlerFunc())
	defer server.Close()

	// Make a request to the endpoint
	resp, err := http.Get(server.URL + "/tagger-list")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response types.TaggerListResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.NotNil(t, response.Entities)
}
