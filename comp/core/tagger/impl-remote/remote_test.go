// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteimpl

import (
	"context"
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

func TestStart(t *testing.T) {
	if os.Getenv("CI") != "true" {
		t.Skip("Not run this test locally because it fails when there is already a running Agent")
	}

	if runtime.GOOS == "darwin" {
		t.Skip("TestStart is known to fail on the macOS Gitlab runners because of the already running Agent")
	}
	grpcServer, authToken, err := grpc.NewMockGrpcSecureServer("5001")
	require.NoError(t, err)
	defer grpcServer.Stop()

	params := tagger.RemoteParams{
		RemoteFilter: types.NewMatchAllFilter(),
		RemoteTarget: func(config.Component) (string, error) { return ":5001", nil },
		RemoteTokenFetcher: func(config.Component) func() (string, error) {
			return func() (string, error) {
				return authToken, nil
			}
		},
	}

	cfg := configmock.New(t)
	log := logmock.New(t)
	telemetry := nooptelemetry.GetCompatComponent()

	remoteTagger, err := newRemoteTagger(params, cfg, log, telemetry)
	require.NoError(t, err)
	err = remoteTagger.Start(context.TODO())
	require.NoError(t, err)
	remoteTagger.Stop()
}

func TestStartDoNotBlockIfServerIsNotAvailable(t *testing.T) {
	params := tagger.RemoteParams{
		RemoteFilter: types.NewMatchAllFilter(),
		RemoteTarget: func(config.Component) (string, error) { return ":5001", nil },
		RemoteTokenFetcher: func(config.Component) func() (string, error) {
			return func() (string, error) {
				return "something", nil
			}
		},
	}

	cfg := configmock.New(t)
	log := logmock.New(t)
	telemetry := nooptelemetry.GetCompatComponent()

	remoteTagger, err := newRemoteTagger(params, cfg, log, telemetry)
	require.NoError(t, err)
	err = remoteTagger.Start(context.TODO())
	require.NoError(t, err)
	remoteTagger.Stop()
}

func TestNewComponentSetsTaggerListEndpoint(t *testing.T) {
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
