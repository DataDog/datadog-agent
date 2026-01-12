// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"maps"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/DataDog/go-tuf/data"

	rdata "github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestSubscriptionTrackAndUntrack(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()

	service := newTestService(t, api, uptaneClient, clock)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := runTestingService(t, service)
	stream, err := client.CreateConfigSubscription(ctx)
	require.NoError(t, err)

	const runtimeID = "test-runtime-1"

	require.NoError(t, stream.Send(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: runtimeID,
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	}))
	require.Eventually(t, func() bool {
		return subscriptionIsRegistered(service, runtimeID)
	}, 1*time.Second, 10*time.Millisecond)

	require.NoError(t, stream.Send(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_UNTRACK,
		RuntimeId: "test-runtime-1",
	}))
	require.Eventually(t, func() bool {
		return !subscriptionIsRegistered(service, runtimeID)
	}, 1*time.Second, 10*time.Millisecond)
	require.NoError(t, stream.CloseSend())
}

// Ensures that a subscription will receive all files, including client cached
// files on the first poll, but will not receive those files on subsequent
// polls.
func TestSubscriptionGetsInitialUpdate(t *testing.T) {
	apmCachedPath := "datadog/2/APM_TRACING/config-abc/cached.json"
	apmConfigPath := "datadog/2/APM_TRACING/config-123/config.json"
	liveDebuggingPath := "datadog/2/LIVE_DEBUGGING/config-456/debugging.json"
	targetFileData := map[string][]byte{
		apmCachedPath:     []byte("cached"),
		apmConfigPath:     []byte("config"),
		liveDebuggingPath: []byte("debugging"),
	}
	files := newTargetFiles(targetFileData)

	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock,
		withSubscriptionProductMappings(productMappingsWithApmTracingProducts))
	uptaneClient.On("TUFVersionState").
		Return(uptane.TUFVersions{
			DirectorRoot:    1,
			DirectorTargets: 1,
		}, nil)
	uptaneClient.On("DirectorRoot", uint64(1)).
		Return([]byte("root1"), nil)
	uptaneClient.On("Targets").
		Return(files, nil)
	uptaneClient.On("TargetsMeta").
		Return([]byte("targets-meta"), nil)
	mock.InOrder(
		uptaneClient.On("TargetFiles", []string{apmConfigPath, apmCachedPath}).
			Return(map[string][]byte{
				apmCachedPath: []byte("cached"),
				apmConfigPath: []byte("config"),
			}, nil),
		uptaneClient.On("TargetFiles", []string{apmConfigPath}).
			Return(map[string][]byte{
				apmConfigPath: []byte("config"),
			}, nil),
	)
	uptaneClient.On("TimestampExpires").
		Return(time.Now().Add(1*time.Hour), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := runTestingService(t, service)
	stream, err := client.CreateConfigSubscription(ctx)
	require.NoError(t, err)

	require.NoError(t, stream.Send(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-123",
		Products:  apmTracingProducts,
	}))

	require.Eventually(t, func() bool {
		return subscriptionIsRegistered(service, "runtime-123")
	}, 1*time.Second, 10*time.Millisecond)

	// Simulate a client polling for configs (with cached files)
	tracerClient := &pbgo.Client{
		Id: "client-456",
		State: &pbgo.ClientState{
			RootVersion:    1,
			TargetsVersion: 0,
		},
		IsTracer: true,
		ClientTracer: &pbgo.ClientTracer{
			RuntimeId: "runtime-123",
			Language:  "go",
		},
		Products: []string{"APM_TRACING"},
	}

	// Mark client as active to avoid cache bypass
	service.clients.seen(tracerClient)

	resp1, err := service.ClientGetConfigs(
		context.Background(),
		&pbgo.ClientGetConfigsRequest{
			Client: tracerClient,
			CachedTargetFiles: []*pbgo.TargetFileMeta{
				uptaneToCore(apmCachedPath, files[apmCachedPath]),
			},
		},
	)
	require.NoError(t, err)
	require.ElementsMatch(
		t, fileNames(resp1.TargetFiles), []string{apmConfigPath},
	)

	// Subscription should receive an update with cached files
	streamResp, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, streamResp)
	require.ElementsMatch(
		t, fileNames(streamResp.TargetFiles), []string{apmCachedPath, apmConfigPath},
	)
	require.ElementsMatch(
		t, streamResp.MatchedConfigs, []string{apmCachedPath, apmConfigPath},
	)

	resp2, err := service.ClientGetConfigs(
		context.Background(),
		&pbgo.ClientGetConfigsRequest{
			Client: tracerClient,
			CachedTargetFiles: []*pbgo.TargetFileMeta{
				uptaneToCore(apmCachedPath, files[apmCachedPath]),
			},
		},
	)
	require.NoError(t, err)
	require.ElementsMatch(
		t, fileNames(resp2.TargetFiles), []string{apmConfigPath},
	)

	streamResp2, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, streamResp2)
	require.ElementsMatch(
		t, fileNames(streamResp2.TargetFiles), []string{apmConfigPath},
	)
	require.ElementsMatch(
		t, streamResp2.MatchedConfigs, []string{apmCachedPath, apmConfigPath},
	)
}

// Ensures that a subscription receives an update even when no configs match its
// products so it can clear previously applied configs.
func TestSubscriptionGetsEmptyMatchedConfigsUpdate(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	const cachedPath = "datadog/2/APM_TRACING/config-abc/cached.json"
	const uncachedPath = "datadog/2/APM_TRACING/config-123/config.json"
	const differentProductPath = "datadog/2/LIVE_DEBUGGING/config-456/debugging.json"
	targetFileData := map[string][]byte{
		cachedPath:           []byte("cached"),
		uncachedPath:         []byte("config"),
		differentProductPath: []byte("debugging"),
	}
	files := newTargetFiles(targetFileData)
	targetFiles := map[string][]byte{}
	apmKeys := []string{cachedPath, uncachedPath}
	for _, path := range apmKeys {
		targetFiles[path] = []byte(targetFileData[path])
	}
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{
		DirectorRoot:    1,
		DirectorTargets: 1,
	}, nil)
	uptaneClient.On("DirectorRoot", uint64(1)).Return([]byte("root1"), nil)
	uptaneClient.On("Targets").Return(files, nil)
	uptaneClient.On("TargetsMeta").Return([]byte("targets-meta"), nil)
	uptaneClient.On("TargetFiles", mock.Anything).Return(targetFiles, nil)
	uptaneClient.On("TimestampExpires").Return(time.Now().Add(1*time.Hour), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := runTestingService(t, service)
	stream, err := client.CreateConfigSubscription(ctx)
	require.NoError(t, err)

	// Track a product that the client doesn't have
	require.NoError(t, stream.Send(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-123",
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	}))
	require.Eventually(t, func() bool {
		return subscriptionIsRegistered(service, "runtime-123")
	}, 1*time.Second, 10*time.Millisecond)

	// Client polls with different product
	tracerClient := &pbgo.Client{
		Id: "client-456",
		State: &pbgo.ClientState{
			RootVersion:    1,
			TargetsVersion: 0,
		},
		IsTracer: true,
		ClientTracer: &pbgo.ClientTracer{
			RuntimeId: "runtime-123",
			Language:  "go",
		},
		Products: []string{"APM_TRACING"},
	}

	// Mark client as active to avoid cache bypass
	service.clients.seen(tracerClient)

	_, err = service.ClientGetConfigs(
		context.Background(),
		&pbgo.ClientGetConfigsRequest{
			Client:            tracerClient,
			CachedTargetFiles: nil,
		},
	)
	require.NoError(t, err)

	// Subscription should receive an update with no matched configs/files.
	update, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, update)
	require.Empty(t, update.TargetFiles)
	require.Empty(t, update.MatchedConfigs)
}

// Ensure a subscription gets an explicit update when previously matched configs
// disappear so clients can clean up.
func TestSubscriptionGetsConfigRemovalUpdate(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	const configPath = "datadog/2/LIVE_DEBUGGING/config-123/debugging.json"
	initialTargets := newTargetFiles(map[string][]byte{
		configPath: []byte("config"),
	})
	emptyTargets := newTargetFiles(map[string][]byte{})

	uptaneClient.On("TUFVersionState").
		Return(uptane.TUFVersions{
			DirectorRoot:    1,
			DirectorTargets: 1,
		}, nil).Once()
	uptaneClient.On("TUFVersionState").
		Return(uptane.TUFVersions{
			DirectorRoot:    1,
			DirectorTargets: 2,
		}, nil).Once()

	uptaneClient.On("TargetsMeta").
		Return([]byte("targets-meta"), nil).
		Twice()
	uptaneClient.On("TimestampExpires").
		Return(time.Now().Add(1*time.Hour), nil).
		Twice()

	mock.InOrder(
		uptaneClient.On("Targets").
			Return(initialTargets, nil).
			Once(),
		uptaneClient.On("Targets").
			Return(emptyTargets, nil).
			Once(),
	)

	mock.InOrder(
		uptaneClient.
			On("TargetFiles", []string{configPath}).
			Return(map[string][]byte{
				configPath: []byte("config"),
			}, nil).
			Once(),
		uptaneClient.
			On("TargetFiles", []string(nil)).
			Return(map[string][]byte(nil), nil).
			Once(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := runTestingService(t, service)
	stream, err := client.CreateConfigSubscription(ctx)
	require.NoError(t, err)

	require.NoError(t, stream.Send(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-123",
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	}))
	require.Eventually(t, func() bool {
		return subscriptionIsRegistered(service, "runtime-123")
	}, 1*time.Second, 10*time.Millisecond)

	tracerClient := &pbgo.Client{
		Id: "client-456",
		State: &pbgo.ClientState{
			RootVersion:    1,
			TargetsVersion: 0,
		},
		IsTracer: true,
		ClientTracer: &pbgo.ClientTracer{
			RuntimeId: "runtime-123",
			Language:  "go",
		},
		Products: []string{"LIVE_DEBUGGING"},
	}
	service.clients.seen(tracerClient)

	_, err = service.ClientGetConfigs(
		context.Background(),
		&pbgo.ClientGetConfigsRequest{
			Client:            tracerClient,
			CachedTargetFiles: nil,
		},
	)
	require.NoError(t, err)

	firstUpdate, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, firstUpdate)
	require.ElementsMatch(t, firstUpdate.MatchedConfigs, []string{configPath})
	require.ElementsMatch(t, fileNames(firstUpdate.TargetFiles), []string{configPath})

	// Simulate the client updating to the latest targets version.
	tracerClient.State.TargetsVersion = 1

	_, err = service.ClientGetConfigs(
		context.Background(),
		&pbgo.ClientGetConfigsRequest{
			Client:            tracerClient,
			CachedTargetFiles: nil,
		},
	)
	require.NoError(t, err)

	removalUpdate, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, removalUpdate)
	require.Empty(t, removalUpdate.TargetFiles)
	require.Empty(t, removalUpdate.MatchedConfigs)
}

// Ensures subscriptions get initial data even when the polling client has no
// new configs.
func TestSubscriptionReceivesCachedFilesWhenClientUpToDate(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)
	counters := service.telemetryReporter.(*telemetryReporter)

	const configPath = "datadog/2/LIVE_DEBUGGING/config-123/debugging.json"
	targetFileData := map[string][]byte{
		configPath: []byte("config"),
	}
	files := newTargetFiles(targetFileData)

	uptaneClient.On("TUFVersionState").
		Return(uptane.TUFVersions{
			DirectorRoot:    1,
			DirectorTargets: 1,
		}, nil)
	uptaneClient.On("Targets").
		Return(files, nil)
	uptaneClient.On("TargetFiles", []string{configPath}).
		Return(map[string][]byte{
			configPath: []byte("config"),
		}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := runTestingService(t, service)
	stream, err := client.CreateConfigSubscription(ctx)
	require.NoError(t, err)

	require.NoError(t, stream.Send(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-123",
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	}))

	_, err = stream.Header()
	require.NoError(t, err)
	require.Equal(t, counters.subscriptionsActiveGauge.Load(), int64(1))
	require.Equal(t, counters.subscriptionsConnected.Load(), int64(1))
	require.Equal(t, counters.subscriptionsDisconnected.Load(), int64(0))
	require.Eventually(t, func() bool {
		return counters.subscriptionClientsTrackedGauge.Load() == 1
	}, 1*time.Second, 10*time.Millisecond)

	tracerClient := &pbgo.Client{
		Id: "client-456",
		State: &pbgo.ClientState{
			RootVersion:    1,
			TargetsVersion: 1,
		},
		IsTracer: true,
		ClientTracer: &pbgo.ClientTracer{
			RuntimeId: "runtime-123",
			Language:  "go",
		},
		Products: []string{"LIVE_DEBUGGING", "APM_TRACING"},
	}
	service.clients.seen(tracerClient)

	resp, err := service.ClientGetConfigs(
		context.Background(),
		&pbgo.ClientGetConfigsRequest{
			Client: tracerClient,
			CachedTargetFiles: []*pbgo.TargetFileMeta{
				uptaneToCore(configPath, files[configPath]),
			},
		},
	)
	require.NoError(t, err)
	require.Empty(t, resp.TargetFiles)
	require.Empty(t, resp.ClientConfigs)

	streamResp, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, streamResp)
	require.ElementsMatch(t, fileNames(streamResp.TargetFiles), []string{configPath})
	require.ElementsMatch(t, streamResp.MatchedConfigs, []string{configPath})
	cancel()

	require.Eventually(t, func() bool {
		return counters.subscriptionsActiveGauge.Load() == 0 &&
			counters.subscriptionsDisconnected.Load() == 1
	}, 1*time.Second, 10*time.Millisecond)
}

// Ensures that once a subscription has already seen a client, a subsequent poll
// without any new configs does not enqueue another update.
func TestSubscriptionNoUpdateDoesNotNotify(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	const configPath = "datadog/2/LIVE_DEBUGGING/config-123/debugging.json"
	targetFileData := map[string][]byte{
		configPath: []byte("config"),
	}
	files := newTargetFiles(targetFileData)

	uptaneClient.On("TUFVersionState").
		Return(uptane.TUFVersions{
			DirectorRoot:    1,
			DirectorTargets: 1,
		}, nil).
		Twice()
	uptaneClient.On("Targets").
		Return(files, nil).
		Once()
	uptaneClient.On("TargetsMeta").
		Return([]byte("targets-meta"), nil).
		Once()
	uptaneClient.On("TargetFiles", []string{configPath}).
		Return(map[string][]byte{
			configPath: []byte("config"),
		}, nil).
		Once()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := runTestingService(t, service)
	stream, err := client.CreateConfigSubscription(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-123",
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	}))
	require.Eventually(t, func() bool {
		return subscriptionIsRegistered(service, "runtime-123")
	}, 1*time.Second, 10*time.Millisecond)

	tracerClient := &pbgo.Client{
		Id: "client-456",
		State: &pbgo.ClientState{
			RootVersion:    1,
			TargetsVersion: 0,
		},
		IsTracer: true,
		ClientTracer: &pbgo.ClientTracer{
			RuntimeId: "runtime-123",
			Language:  "go",
		},
		Products: []string{"LIVE_DEBUGGING", "APM_TRACING"},
	}
	service.clients.seen(tracerClient)

	resp1, err := service.ClientGetConfigs(
		context.Background(),
		&pbgo.ClientGetConfigsRequest{
			Client:            tracerClient,
			CachedTargetFiles: nil,
		},
	)
	require.NoError(t, err)
	require.ElementsMatch(
		t, fileNames(resp1.TargetFiles), []string{configPath},
	)

	firstUpdate, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, firstUpdate)
	require.ElementsMatch(
		t, fileNames(firstUpdate.TargetFiles), []string{configPath},
	)
	require.ElementsMatch(
		t, firstUpdate.MatchedConfigs, []string{configPath},
	)

	// Client is now aligned with the server targets version and caches.
	tracerClient.State.TargetsVersion = 1

	resp2, err := service.ClientGetConfigs(
		context.Background(),
		&pbgo.ClientGetConfigsRequest{
			Client: tracerClient,
			CachedTargetFiles: []*pbgo.TargetFileMeta{
				uptaneToCore(configPath, files[configPath]),
			},
		},
	)
	require.NoError(t, err)
	require.Empty(t, resp2.TargetFiles)
	require.Empty(t, resp2.ClientConfigs)

	respCh := make(chan *pbgo.ConfigSubscriptionResponse, 1)
	go func() {
		resp, _ := stream.Recv()
		respCh <- resp
	}()
	select {
	case resp := <-respCh:
		t.Fatalf("should not receive update: %v", resp)
	case <-time.After(100 * time.Millisecond):
	}
}

// Ensures that multiple subscriptions can be created for the same runtime_id
// with different products, and that each subscription will receive the files
// that it is interested in.
func TestSubscriptionCanTrackSameRuntimeIDWithDifferentProducts(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock,
		withSubscriptionProductMappings(productMappingsWithApmTracingProducts))

	const apmPath = "datadog/2/APM_TRACING/config-abc/cached.json"
	const liveDebuggingPath = "datadog/2/LIVE_DEBUGGING/config-456/debugging.json"
	targetFileData := map[string][]byte{
		apmPath:           []byte("apm"),
		liveDebuggingPath: []byte("debugging"),
	}
	files := newTargetFiles(targetFileData)
	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{
		DirectorRoot:    1,
		DirectorTargets: 1,
	}, nil)
	uptaneClient.On("DirectorRoot", uint64(1)).Return([]byte("root1"), nil)
	uptaneClient.On("Targets").Return(files, nil)
	uptaneClient.On("TargetsMeta").Return([]byte("targets-meta"), nil)
	targetFiles := map[string][]byte{
		apmPath:           []byte(targetFileData[apmPath]),
		liveDebuggingPath: []byte(targetFileData[liveDebuggingPath]),
	}
	uptaneClient.On("TargetFiles", mock.Anything).Return(targetFiles, nil)
	uptaneClient.On("TimestampExpires").Return(time.Now().Add(1*time.Hour), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := runTestingService(t, service)
	stream1, err := client.CreateConfigSubscription(ctx)
	require.NoError(t, err)
	stream2, err := client.CreateConfigSubscription(ctx)
	require.NoError(t, err)

	// Both track the same runtime_id with different products.
	require.NoError(t, stream1.Send(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-123",
		Products:  apmTracingProducts,
	}))
	require.NoError(t, stream2.Send(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-123",
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	}))
	require.Eventually(t, func() bool {
		return numSubscriptionsRegisteredForRuntimeID(service, "runtime-123") == 2
	}, 1*time.Second, 10*time.Millisecond)

	// Client polls.
	tracerClient := &pbgo.Client{
		Id: "client-456",
		State: &pbgo.ClientState{
			RootVersion:    1,
			TargetsVersion: 0,
		},
		IsTracer: true,
		ClientTracer: &pbgo.ClientTracer{
			RuntimeId: "runtime-123",
			Language:  "go",
		},
		Products: []string{"APM_TRACING", "LIVE_DEBUGGING"},
	}

	// Mark client as active to avoid cache bypass.
	service.clients.seen(tracerClient)

	resp1, err := service.ClientGetConfigs(
		context.Background(),
		&pbgo.ClientGetConfigsRequest{
			Client:            tracerClient,
			CachedTargetFiles: nil,
		},
	)
	require.NoError(t, err)
	require.ElementsMatch(t, fileNames(resp1.TargetFiles), []string{apmPath, liveDebuggingPath})

	// Both subscriptions should receive updates.
	streamResp1, err := stream1.Recv()
	require.NoError(t, err)
	require.NotNil(t, streamResp1)
	require.ElementsMatch(t, fileNames(streamResp1.TargetFiles), []string{apmPath})
	require.ElementsMatch(t, streamResp1.MatchedConfigs, []string{apmPath})

	streamResp2, err := stream2.Recv()
	require.NoError(t, err)
	require.NotNil(t, streamResp2)
	require.ElementsMatch(t, fileNames(streamResp2.TargetFiles), []string{liveDebuggingPath})
	require.ElementsMatch(t, streamResp2.MatchedConfigs, []string{liveDebuggingPath})
}

func TestSubscriptionInvalidRequests(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock)

	tests := []struct {
		name    string
		request *pbgo.ConfigSubscriptionRequest
	}{
		{
			name: "track with empty runtime_id",
			request: &pbgo.ConfigSubscriptionRequest{
				Action:    pbgo.ConfigSubscriptionRequest_TRACK,
				RuntimeId: "",
				Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
			},
		},
		{
			name: "untrack with empty runtime_id",
			request: &pbgo.ConfigSubscriptionRequest{
				Action:    pbgo.ConfigSubscriptionRequest_UNTRACK,
				RuntimeId: "",
				Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
			},
		},
		{
			name: "invalid action",
			request: &pbgo.ConfigSubscriptionRequest{
				Action:    42,
				RuntimeId: "runtime-123",
				Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
			},
		},
		{
			name: "TRACK with no products",
			request: &pbgo.ConfigSubscriptionRequest{
				Action:    pbgo.ConfigSubscriptionRequest_TRACK,
				RuntimeId: "runtime-123",
				Products:  0,
			},
		},
		{
			name: "TRACK with no invalid products",
			request: &pbgo.ConfigSubscriptionRequest{
				Action:    pbgo.ConfigSubscriptionRequest_TRACK,
				RuntimeId: "runtime-123",
				Products:  42,
			},
		},
		{
			name:    "nil request",
			request: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := runTestingService(t, service)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stream, err := client.CreateConfigSubscription(ctx)
			require.NoError(t, err)

			require.NoError(t, stream.Send(tt.request))
			_, err = stream.Recv()
			require.Error(t, err)
			require.Equal(t, codes.InvalidArgument, status.Code(err))
		})
	}
}

// Ensures that only tracked clients receive updates.
func TestSubscriptionOnlyTrackedClientsGetUpdates(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock,
		withSubscriptionProductMappings(productMappingsWithApmTracingProducts))

	targetFileData := map[string][]byte{
		"datadog/2/APM_TRACING/config-abc/cached.json":       []byte("cached"),
		"datadog/2/APM_TRACING/config-123/config.json":       []byte("config"),
		"datadog/2/LIVE_DEBUGGING/config-456/debugging.json": []byte("debugging"),
	}
	files := newTargetFiles(targetFileData)

	uptaneClient.On("TUFVersionState").Return(uptane.TUFVersions{
		DirectorRoot:    1,
		DirectorTargets: 1,
	}, nil)
	uptaneClient.On("DirectorRoot", uint64(1)).Return([]byte("root1"), nil)
	uptaneClient.On("Targets").Return(files, nil)
	uptaneClient.On("TargetsMeta").Return([]byte("targets-meta"), nil)
	targetsFiles := make(map[string][]byte)
	for path, content := range targetFileData {
		targetsFiles[path] = []byte(content)
	}
	uptaneClient.On(
		"TargetFiles",
		mock.Anything,
	).Return(map[string][]byte{
		"datadog/2/APM_TRACING/config-abc/cached.json": []byte("cached"),
		"datadog/2/APM_TRACING/config-123/config.json": []byte("config"),
	}, nil)
	uptaneClient.On("TimestampExpires").Return(
		time.Now().Add(1*time.Hour),
		nil,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := runTestingService(t, service)
	stream, err := client.CreateConfigSubscription(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-123",
		Products:  apmTracingProducts,
	}))

	require.Eventually(t, func() bool {
		return subscriptionIsRegistered(service, "runtime-123")
	}, 1*time.Second, 10*time.Millisecond)

	// Different client polls (runtime-456)
	otherClient := &pbgo.Client{
		Id: "client-789",
		State: &pbgo.ClientState{
			RootVersion:    1,
			TargetsVersion: 0,
		},
		IsTracer: true,
		ClientTracer: &pbgo.ClientTracer{
			RuntimeId: "runtime-456",
			Language:  "go",
		},
		Products: []string{"APM_TRACING"},
	}

	// Mark client as active to avoid cache bypass
	service.clients.seen(otherClient)

	_, err = client.ClientGetConfigs(
		context.Background(),
		&pbgo.ClientGetConfigsRequest{
			Client:            otherClient,
			CachedTargetFiles: nil,
		},
	)
	require.NoError(t, err)

	respCh := make(chan *pbgo.ConfigSubscriptionResponse, 1)
	go func() {
		resp, _ := stream.Recv()
		respCh <- resp
	}()

	select {
	case resp := <-respCh:
		t.Fatalf("should not receive update for non-tracked client: %v", resp)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestSubscriptionsLimits(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock, func(o *options) {
		o.maxConcurrentSubscriptions = 1
	})
	client := runTestingService(t, service)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := client.CreateConfigSubscription(ctx)
	require.NoError(t, err)
	_, err = stream.Header()
	require.NoError(t, err)

	stream2, err2 := client.CreateConfigSubscription(ctx)
	require.NoError(t, err2) // this is about dialing
	_, err2 = stream2.Recv()
	require.Equal(t, codes.ResourceExhausted, status.Code(err2))
	require.ErrorContains(t, err2, "maximum number of subscriptions reached (1)")

	require.NoError(t, stream.CloseSend())
	require.NoError(t, err)
	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF)

	stream3, err3 := client.CreateConfigSubscription(ctx)
	require.NoError(t, err3)
	_, err3 = stream3.Header()
	require.NoError(t, err3)
}

func TestTrackLimitPerSubscription(t *testing.T) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock,
		func(o *options) {
			o.maxTrackedRuntimeIDsPerSubscription = 2
		})
	client := runTestingService(t, service)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := client.CreateConfigSubscription(ctx)
	require.NoError(t, err)
	_, err = stream.Header()
	require.NoError(t, err)
	require.NoError(t, stream.Send(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-123",
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	}))
	require.NoError(t, stream.Send(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-456",
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	}))
	require.NoError(t, stream.Send(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-789",
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	}))
	_, err = stream.Recv()
	require.Error(t, err)
	require.ErrorContains(t, err, "maximum number of runtime IDs per subscription reached (2)")
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
}

type statsHandlerFunc func(ctx context.Context, stats stats.RPCStats)

func (statsHandlerFunc) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context {
	return ctx
}

func (f statsHandlerFunc) HandleRPC(ctx context.Context, stats stats.RPCStats) {
	f(ctx, stats)
}

func (statsHandlerFunc) HandleConn(context.Context, stats.ConnStats) {
}

func (statsHandlerFunc) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}

// TestSlowReceiverOverwritesPendingUpdatesAndResendsAllFiles tests that in
// the situation where our client connection is so slow that we see two
// differnet polls and by the time the second poll arrives we still haven't
// sent the first poll's response, we overwrite the pending updates with the
// second poll's response and resend all the files.
//
// We don't really expect this to happen in real life, but the logic is
// important for correctness while bounding memory usage, so it needs to be
// tested.
func TestSlowReceiver(t *testing.T) {
	config1Path := "datadog/2/LIVE_DEBUGGING/config-1/config1.json"
	config2Path := "datadog/2/LIVE_DEBUGGING/config-2/config2.json"
	config3Path := "datadog/2/LIVE_DEBUGGING/config-3/config3.json"
	config4Path := "datadog/2/LIVE_DEBUGGING/config-4/config4.json"
	targetFileData := map[string][]byte{
		config1Path: bytes.Repeat([]byte("a"), 128<<10),
		config2Path: bytes.Repeat([]byte("b"), 128<<10),
		config3Path: bytes.Repeat([]byte("c"), 128<<10),
		config4Path: bytes.Repeat([]byte("d"), 128<<10),
	}
	filesData := func(names ...string) map[string][]byte {
		files := make(map[string][]byte)
		for _, name := range names {
			files[name] = targetFileData[name]
		}
		return files
	}
	files1 := newTargetFiles(filesData(config1Path))
	files2 := newTargetFiles(filesData(config1Path, config2Path))
	files3 := newTargetFiles(filesData(config1Path, config2Path, config3Path))
	files4 := newTargetFiles(filesData(config1Path, config2Path, config3Path, config4Path))

	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()
	service := newTestService(t, api, uptaneClient, clock, func(o *options) {
		o.maxSubscriptionQueueSize = 1
	})
	tufVersions := func(v int) uptane.TUFVersions {
		return uptane.TUFVersions{DirectorRoot: 1, DirectorTargets: uint64(v)}
	}
	mock.InOrder(
		uptaneClient.On("TUFVersionState").Return(tufVersions(1), nil).Once(),
		uptaneClient.On("TUFVersionState").Return(tufVersions(2), nil).Once(),
		uptaneClient.On("TUFVersionState").Return(tufVersions(3), nil).Once(),
		uptaneClient.On("TUFVersionState").Return(tufVersions(4), nil).Twice(),
	)
	uptaneClient.On("DirectorRoot", uint64(1)).
		Return([]byte("root1"), nil)
	mock.InOrder(
		uptaneClient.On("Targets").Return(files1, nil).Once(),
		uptaneClient.On("Targets").Return(files2, nil).Once(),
		uptaneClient.On("Targets").Return(files3, nil).Once(),
		uptaneClient.On("Targets").Return(files4, nil).Twice(),
	)
	uptaneClient.On("TargetsMeta").
		Return([]byte("targets-meta"), nil)
	mock.InOrder(
		uptaneClient.On("TargetFiles", []string{config1Path}).
			Return(filesData(config1Path), nil),
		uptaneClient.On("TargetFiles", []string{config2Path}).
			Return(filesData(config2Path), nil),
		uptaneClient.On("TargetFiles", []string{config3Path}).
			Return(filesData(config3Path), nil),
		uptaneClient.On("TargetFiles", []string{config4Path}).
			Return(filesData(config4Path), nil),
		uptaneClient.On("TargetFiles", []string{config1Path, config2Path, config3Path, config4Path}).
			Return(filesData(config1Path, config2Path, config3Path, config4Path), nil),
	)
	uptaneClient.On("TimestampExpires").
		Return(time.Now().Add(1*time.Hour), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var responsesSent atomic.Int32
	client := runTestingService(t, service, grpc.StatsHandler(statsHandlerFunc(func(
		_ context.Context, s stats.RPCStats,
	) {
		out, ok := s.(*stats.OutPayload)
		if !ok {
			return
		}
		if _, ok := out.Payload.(*pbgo.ConfigSubscriptionResponse); ok {
			responsesSent.Add(1)
		}
	})))
	stream, err := client.CreateConfigSubscription(ctx)
	require.NoError(t, err)

	require.NoError(t, stream.Send(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-123",
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	}))

	_, err = stream.Header()
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return subscriptionIsRegistered(service, "runtime-123")
	}, 1*time.Second, 10*time.Millisecond)

	// Simulate a client polling for configs (with cached files)
	tracerClient := &pbgo.Client{
		Id: "client-456",
		State: &pbgo.ClientState{
			RootVersion:    1,
			TargetsVersion: 0,
		},
		IsTracer: true,
		ClientTracer: &pbgo.ClientTracer{
			RuntimeId: "runtime-123",
			Language:  "go",
		},
		Products: []string{"LIVE_DEBUGGING"},
	}

	// Mark client as active to avoid cache bypass
	service.clients.seen(tracerClient)

	// The first poll won't result in any files being returned because the
	// client is up-to-date, but should send a message to the subscription.
	resp1, err := client.ClientGetConfigs(
		context.Background(),
		&pbgo.ClientGetConfigsRequest{
			Client: tracerClient,
			CachedTargetFiles: []*pbgo.TargetFileMeta{
				uptaneToCore(config1Path, files1[config1Path]),
			},
		},
	)
	require.NoError(t, err)
	require.Empty(t, resp1.TargetFiles)
	tracerClient.State.TargetsVersion = 1

	require.Eventually(t, func() bool {
		return responsesSent.Load() == 1
	}, 1*time.Second, 10*time.Millisecond)

	// Client intentionally doesn't call Recv.
	resp2, err := client.ClientGetConfigs(ctx, &pbgo.ClientGetConfigsRequest{
		Client: tracerClient,
		CachedTargetFiles: []*pbgo.TargetFileMeta{
			uptaneToCore(config1Path, files1[config1Path]),
		},
	})
	require.NoError(t, err)
	require.ElementsMatch(t, fileNames(resp2.TargetFiles), []string{config2Path})

	// Make sure that the second response gets blocked in sending by ensuring it
	// is no longer in the pending queue (it would have been added before resp2
	// was sent).
	require.Eventually(t, func() bool {
		service.mu.Lock()
		defer service.mu.Unlock()
		return len(service.mu.subscriptions.subs[1].pendingQueue) == 0
	}, 1*time.Second, 10*time.Millisecond)

	tracerClient.State.TargetsVersion = 2
	resp3, err := client.ClientGetConfigs(ctx, &pbgo.ClientGetConfigsRequest{
		Client: tracerClient,
		CachedTargetFiles: []*pbgo.TargetFileMeta{
			uptaneToCore(config1Path, files1[config1Path]),
			uptaneToCore(config2Path, files2[config2Path]),
		},
	})
	require.NoError(t, err)
	require.ElementsMatch(t, fileNames(resp3.TargetFiles), []string{config3Path})

	tracerClient.State.TargetsVersion = 3
	resp4, err := client.ClientGetConfigs(ctx, &pbgo.ClientGetConfigsRequest{
		Client: tracerClient,
		CachedTargetFiles: []*pbgo.TargetFileMeta{
			uptaneToCore(config1Path, files1[config1Path]),
			uptaneToCore(config2Path, files2[config2Path]),
			uptaneToCore(config3Path, files3[config3Path]),
		},
	})
	require.NoError(t, err)
	require.ElementsMatch(t, fileNames(resp4.TargetFiles), []string{config4Path})

	streamResp, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, streamResp)
	require.ElementsMatch(t, fileNames(streamResp.TargetFiles), []string{config1Path})

	streamResp2, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, streamResp2)
	require.ElementsMatch(t, fileNames(streamResp2.TargetFiles), []string{config2Path})

	tracerClient.State.TargetsVersion = 4
	resp5, err := client.ClientGetConfigs(ctx, &pbgo.ClientGetConfigsRequest{
		Client: tracerClient,
		CachedTargetFiles: []*pbgo.TargetFileMeta{
			uptaneToCore(config1Path, files1[config1Path]),
			uptaneToCore(config2Path, files2[config2Path]),
			uptaneToCore(config3Path, files3[config3Path]),
			uptaneToCore(config4Path, files4[config4Path]),
		},
	})
	require.NoError(t, err)
	require.Empty(t, resp5.TargetFiles)

	streamResp3, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, streamResp3)
	require.ElementsMatch(t, fileNames(streamResp3.TargetFiles), []string{
		config1Path, config2Path, config3Path, config4Path,
	})
}

func runTestingService(t *testing.T, service *CoreAgentService, opts ...grpc.ServerOption) pbgo.AgentSecureClient {
	const bufSize = 1 << 10 // 1KiB
	l := bufconn.Listen(bufSize)
	s := grpc.NewServer(opts...)
	type embeddedUnimplementedAgentSecureServer struct {
		pbgo.UnimplementedAgentSecureServer
	}
	var v struct {
		*CoreAgentService
		embeddedUnimplementedAgentSecureServer
	}
	v.CoreAgentService = service
	pbgo.RegisterAgentSecureServer(s, &v)
	t.Cleanup(func() { s.Stop() })
	go s.Serve(l)

	dialOpts := []grpc.DialOption{
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return l.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithReadBufferSize(1 << 10),
		grpc.WithInitialWindowSize(64 << 10),
		grpc.WithStaticConnWindowSize(64 << 10),
		grpc.WithStaticStreamWindowSize(64 << 10),
	}
	conn, err := grpc.NewClient("passthrough://bufnet", dialOpts...)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	client := pbgo.NewAgentSecureClient(conn)
	return client
}

func newTargetFiles(tf map[string][]byte) data.TargetFiles {
	targets := make(data.TargetFiles)
	for path, content := range tf {
		sha256Hash := sha256.Sum256([]byte(content))
		targets[path] = data.TargetFileMeta{
			FileMeta: data.FileMeta{
				Length: int64(len(content)),
				Hashes: data.Hashes{
					"sha256": sha256Hash[:],
				},
			},
		}
	}
	return targets
}

func withSubscriptionProductMappings(productsMappings productsMappings) Option {
	return func(o *options) {
		o.subscriptionProductMappings = productsMappings
	}
}

// A fake enum value that we'll use for testing to ensure that all the
// infrastructure supports working with multiple sets of products.
const apmTracingProducts pbgo.ConfigSubscriptionProducts = 2

var productMappingsWithApmTracingProducts = func() productsMappings {
	mappings := maps.Clone(defaultSubscriptionProductMappings)
	mappings[apmTracingProducts] = map[rdata.Product]struct{}{
		rdata.ProductAPMTracing: {},
	}
	return mappings
}()

func uptaneToCore(path string, m data.TargetFileMeta) *pbgo.TargetFileMeta {
	hashes := make([]*pbgo.TargetFileHash, 0, len(m.Hashes))
	for algorithm, hash := range m.Hashes {
		hashes = append(hashes, &pbgo.TargetFileHash{
			Algorithm: algorithm,
			Hash:      hex.EncodeToString(hash),
		})
	}

	return &pbgo.TargetFileMeta{
		Path:   path,
		Length: m.Length,
		Hashes: hashes,
	}
}

func subscriptionIsRegistered(service *CoreAgentService, runtimeID string) bool {
	return numSubscriptionsRegisteredForRuntimeID(service, runtimeID) > 0
}

func numSubscriptionsRegisteredForRuntimeID(service *CoreAgentService, runtimeID string) int {
	service.mu.Lock()
	defer service.mu.Unlock()
	count := 0
	for _, sub := range service.mu.subscriptions.subs {
		if _, ok := sub.trackedClients[runtimeID]; ok {
			count++
		}
	}
	return count
}

func fileNames(files []*pbgo.File) (fileNames []string) {
	for _, file := range files {
		fileNames = append(fileNames, file.Path)
	}
	return fileNames
}
