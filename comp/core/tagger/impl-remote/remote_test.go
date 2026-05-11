// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteimpl

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerTelemetry "github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	mocktelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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

	// Instantiate the component.
	req := Requires{
		Lc:     compdef.NewTestLifecycle(t),
		Config: configmock.New(t),
		Log:    logmock.New(t),
		Params: tagger.NewRemoteParams(
			tagger.WithRemoteTarget(func(config.Component) (string, error) { return ":5001", nil }),
		),
		Telemetry: nooptelemetry.GetCompatComponent(),
		IPC:       ipcmock.New(t),
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
		Params: tagger.NewRemoteParams(
			tagger.WithRemoteTarget(func(config.Component) (string, error) { return ":5001", nil }),
		),
		Telemetry: nooptelemetry.GetCompatComponent(),
		IPC:       ipcmock.New(t),
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
		Params: tagger.NewRemoteParams(
			tagger.WithRemoteTarget(func(config.Component) (string, error) { return ":5001", nil }),
		),
		Telemetry: nooptelemetry.GetCompatComponent(),
		IPC:       ipcmock.New(t),
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

// TestNewComponentWithOverride tests the Remote Tagger initialization with overrides for TLS and auth token.
func TestNewComponentWithOverride(t *testing.T) {
	// Create a mock IPC component
	ipcComp := ipcmock.New(t)

	// Create a test server with the endpoint handler
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	t.Run("auth token getter blocks 2s and succeeds", func(t *testing.T) {
		start := time.Now()
		req := Requires{
			Lc:     compdef.NewTestLifecycle(t),
			Config: configmock.New(t),
			Log:    logmock.New(t),
			Params: tagger.NewRemoteParams(
				tagger.WithRemoteTarget(func(config.Component) (string, error) { return server.URL, nil }),
				tagger.WithOverrideTLSConfigGetter(func() (*tls.Config, error) {
					return &tls.Config{
						InsecureSkipVerify: true,
					}, nil
				}),
				tagger.WithOverrideAuthTokenGetter(func(_ configmodel.Reader) (string, error) {
					time.Sleep(2 * time.Second)
					return "test-token", nil
				}),
			),
			Telemetry: nooptelemetry.GetCompatComponent(),
			IPC:       ipcComp,
		}
		_, err := NewComponent(req)
		elapsed := time.Since(start)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, elapsed, 2*time.Second, "NewComponent should wait for auth token getter")
	})

	t.Run("auth token getter blocks >10s and fails", func(t *testing.T) {
		start := time.Now()
		req := Requires{
			Lc:     compdef.NewTestLifecycle(t),
			Config: configmock.New(t),
			Log:    logmock.New(t),
			Params: tagger.NewRemoteParams(
				tagger.WithRemoteTarget(func(config.Component) (string, error) { return server.URL, nil }),
				tagger.WithOverrideTLSConfigGetter(func() (*tls.Config, error) {
					return &tls.Config{
						InsecureSkipVerify: true,
					}, nil
				}),

				tagger.WithOverrideAuthTokenGetter(func(_ configmodel.Reader) (string, error) {
					return "", errors.New("auth token getter always fails")
				})),
			Telemetry: nooptelemetry.GetCompatComponent(),
			IPC:       ipcComp,
		}
		_, err := NewComponent(req)
		elapsed := time.Since(start)
		assert.Error(t, err, "NewComponent should fail if auth token getter blocks too long")
		assert.GreaterOrEqual(t, elapsed, 10*time.Second, "Should wait at least 10s before failing")
		assert.Less(t, elapsed, 15*time.Second, "Should not wait excessively long")
	})
}

func TestProcessResponseLegacyServerMarksReadyAfterFirstUnmarkedSnapshot(t *testing.T) {
	tagger := newTestRemoteTagger(t)

	err := tagger.processResponse(newStreamTagsResponse("container-1", []string{"pod_name:one"}, false))
	require.NoError(t, err)

	assert.True(t, tagger.ready)
	assert.Empty(t, tagger.resyncEvents)
	entity := tagger.store.getEntity(types.NewEntityID(types.ContainerID, "container-1"))
	require.NotNil(t, entity)
	assert.Equal(t, []string{"pod_name:one"}, entity.LowCardinalityTags)
}

func TestProcessResponseSupportedChunkedSnapshotWaitsForCompletion(t *testing.T) {
	tagger := newTestRemoteTagger(t)
	tagger.initialSnapshotCompleteSupportedByServer = true

	err := tagger.processResponse(newStreamTagsResponse("container-1", []string{"pod_name:one"}, false))
	require.NoError(t, err)

	assert.False(t, tagger.ready)
	assert.Len(t, tagger.resyncEvents, 1)
	assert.Nil(t, tagger.store.getEntity(types.NewEntityID(types.ContainerID, "container-1")))

	err = tagger.processResponse(newStreamTagsResponse("container-2", []string{"pod_name:two"}, true))
	require.NoError(t, err)

	assert.True(t, tagger.ready)
	assert.Empty(t, tagger.resyncEvents)
	assert.NotNil(t, tagger.store.getEntity(types.NewEntityID(types.ContainerID, "container-1")))
	assert.NotNil(t, tagger.store.getEntity(types.NewEntityID(types.ContainerID, "container-2")))
}

func newTestRemoteTagger(t testing.TB) *remoteTagger {
	t.Helper()
	tel := fxutil.Test[coretelemetry.Component](t, mocktelemetry.Module())
	return &remoteTagger{
		store: newTagStore(taggerTelemetry.NewStore(tel)),
		log:   logmock.New(t),
	}
}

func newStreamTagsResponse(id string, lowCardinalityTags []string, initialSnapshotComplete bool) *pb.StreamTagsResponse {
	return &pb.StreamTagsResponse{
		Events: []*pb.StreamTagsEvent{
			{
				Type: pb.EventType_ADDED,
				Entity: &pb.Entity{
					Id: &pb.EntityId{
						Prefix: string(types.ContainerID),
						Uid:    id,
					},
					LowCardinalityTags: lowCardinalityTags,
				},
			},
		},
		InitialSnapshotComplete: initialSnapshotComplete,
	}
}
