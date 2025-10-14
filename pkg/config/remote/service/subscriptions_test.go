// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/DataDog/go-tuf/data"

	"github.com/DataDog/datadog-agent/pkg/config/remote/uptane"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// mockSubscriptionStream implements pbgo.AgentSecure_CreateConfigSubscriptionServer
type mockSubscriptionStream struct {
	ctx context.Context

	// Channels for simulating bidirectional communication
	recvCh chan *pbgo.ConfigSubscriptionRequest
	sendCh chan *pbgo.ConfigSubscriptionResponse
	errCh  chan error

	mu     sync.Mutex
	closed bool
}

func newMockSubscriptionStream(
	ctx context.Context,
) *mockSubscriptionStream {
	return &mockSubscriptionStream{
		ctx:    ctx,
		recvCh: make(chan *pbgo.ConfigSubscriptionRequest),
		sendCh: make(chan *pbgo.ConfigSubscriptionResponse),
		errCh:  make(chan error, 1),
	}
}

func (m *mockSubscriptionStream) Send(
	response *pbgo.ConfigSubscriptionResponse,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return io.EOF
	}

	select {
	case m.sendCh <- response:
		return nil
	case <-m.ctx.Done():
		return m.ctx.Err()
	}
}

func (m *mockSubscriptionStream) Recv() (
	*pbgo.ConfigSubscriptionRequest,
	error,
) {
	select {
	case req := <-m.recvCh:
		return req, nil
	case err := <-m.errCh:
		return nil, err
	case <-m.ctx.Done():
		return nil, m.ctx.Err()
	}
}

func (m *mockSubscriptionStream) SetHeader(metadata.MD) error {
	return nil
}

func (m *mockSubscriptionStream) SendHeader(metadata.MD) error {
	return nil
}

func (m *mockSubscriptionStream) SetTrailer(metadata.MD) {
}

func (m *mockSubscriptionStream) Context() context.Context {
	return m.ctx
}

func (m *mockSubscriptionStream) SendMsg(interface{}) error {
	return nil
}

func (m *mockSubscriptionStream) RecvMsg(interface{}) error {
	return nil
}

// clientSendRequest sends a request from the client to the stream
func (m *mockSubscriptionStream) clientSendRequest(
	req *pbgo.ConfigSubscriptionRequest,
) {
	m.recvCh <- req
}

// clientCloseStream closes the stream from the client side
func (m *mockSubscriptionStream) clientCloseStream() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.errCh <- io.EOF
}

// clientReceiveResponse receives a response from the stream (non-blocking)
func (m *mockSubscriptionStream) clientReceiveResponse(
	timeout time.Duration,
) (*pbgo.ConfigSubscriptionResponse, error) {
	select {
	case resp := <-m.sendCh:
		return resp, nil
	case <-time.After(timeout):
		return nil, errors.New("timeout waiting for response")
	}
}

// clientReceiveNoResponse ensures no response is sent (non-blocking)
func (m *mockSubscriptionStream) clientReceiveNoResponse(
	timeout time.Duration,
) error {
	select {
	case v := <-m.sendCh:
		return errors.New("unexpected response received: " + v.String())
	case <-time.After(timeout):
		return nil
	}
}

func TestSubscriptionTrackAndUntrack(t *testing.T) {
	service, _ := newTestServiceWithOpts(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := newMockSubscriptionStream(ctx)

	// Start the subscription handler in a goroutine
	subscriptionDone := make(chan error, 1)
	go func() {
		subscriptionDone <- service.CreateConfigSubscription(stream)
	}()

	const runtimeID = "test-runtime-1"

	stream.clientSendRequest(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: runtimeID,
		Products:  []string{"APM_TRACING"},
	})
	require.Eventually(t, func() bool {
		return subscriptionIsRegistered(service, runtimeID)
	}, 1*time.Second, 10*time.Millisecond)

	stream.clientSendRequest(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_UNTRACK,
		RuntimeId: "test-runtime-1",
	})
	require.Eventually(t, func() bool {
		return !subscriptionIsRegistered(service, runtimeID)
	}, 1*time.Second, 10*time.Millisecond)
	stream.clientCloseStream()
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

	service, uptaneClient := newTestServiceWithOpts(t)
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

	stream := newMockSubscriptionStream(ctx)

	// Start the subscription handler
	subErrCh := make(chan error, 1)
	go func() {
		subErrCh <- service.CreateConfigSubscription(stream)
	}()

	stream.clientSendRequest(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-123",
		Products:  []string{"APM_TRACING"},
	})

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
	streamResp, err := stream.clientReceiveResponse(1 * time.Second)
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

	streamResp2, err := stream.clientReceiveResponse(1 * time.Second)
	require.NoError(t, err)
	require.NotNil(t, streamResp2)
	require.ElementsMatch(
		t, fileNames(streamResp2.TargetFiles), []string{apmConfigPath},
	)
	require.ElementsMatch(
		t, streamResp2.MatchedConfigs, []string{apmCachedPath, apmConfigPath},
	)

	cancel()
	select {
	case err := <-subErrCh:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(1 * time.Second):
		t.Fatal("subscription handler did not exit")
	}
}

// Ensures that a subscription receives an update even when no configs match its
// products so it can clear previously applied configs.
func TestSubscriptionGetsEmptyMatchedConfigsUpdate(t *testing.T) {
	service, uptaneClient := newTestServiceWithOpts(t)

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

	stream := newMockSubscriptionStream(ctx)

	subscriptionDone := make(chan error, 1)
	go func() {
		subscriptionDone <- service.CreateConfigSubscription(stream)
	}()

	// Track a product that the client doesn't have
	stream.clientSendRequest(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-123",
		Products:  []string{"LIVE_DEBUGGING"},
	})
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

	_, err := service.ClientGetConfigs(
		context.Background(),
		&pbgo.ClientGetConfigsRequest{
			Client:            tracerClient,
			CachedTargetFiles: nil,
		},
	)
	require.NoError(t, err)

	// Subscription should receive an update with no matched configs/files.
	update, err := stream.clientReceiveResponse(1 * time.Second)
	require.NoError(t, err)
	require.NotNil(t, update)
	require.Empty(t, update.TargetFiles)
	require.Empty(t, update.MatchedConfigs)

	// Clean up
	cancel()
	select {
	case <-subscriptionDone:
	case <-time.After(1 * time.Second):
		t.Fatal("subscription handler did not exit")
	}
}

// Ensure a subscription gets an explicit update when previously matched configs
// disappear so clients can clean up.
func TestSubscriptionGetsConfigRemovalUpdate(t *testing.T) {
	service, uptaneClient := newTestServiceWithOpts(t)

	const configPath = "datadog/2/APM_TRACING/config-123/config.json"
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

	uptaneClient.On("TargetFiles", mock.Anything).
		Run(func(args mock.Arguments) {
			require.ElementsMatch(
				t,
				[]string{configPath},
				args.Get(0).([]string),
			)
		}).
		Return(map[string][]byte{
			configPath: []byte("config"),
		}, nil).
		Once()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := newMockSubscriptionStream(ctx)
	subscriptionDone := make(chan error, 1)
	go func() {
		subscriptionDone <- service.CreateConfigSubscription(stream)
	}()

	stream.clientSendRequest(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-123",
		Products:  []string{"APM_TRACING"},
	})
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
		Products: []string{"APM_TRACING"},
	}
	service.clients.seen(tracerClient)

	_, err := service.ClientGetConfigs(
		context.Background(),
		&pbgo.ClientGetConfigsRequest{
			Client:            tracerClient,
			CachedTargetFiles: nil,
		},
	)
	require.NoError(t, err)

	firstUpdate, err := stream.clientReceiveResponse(1 * time.Second)
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

	removalUpdate, err := stream.clientReceiveResponse(1 * time.Second)
	require.NoError(t, err)
	require.NotNil(t, removalUpdate)
	require.Empty(t, removalUpdate.TargetFiles)
	require.Empty(t, removalUpdate.MatchedConfigs)

	cancel()
	select {
	case <-subscriptionDone:
	case <-time.After(1 * time.Second):
		t.Fatal("subscription handler did not exit")
	}
}

// Ensures subscriptions get initial data even when the polling client has no
// new configs.
func TestSubscriptionReceivesCachedFilesWhenClientUpToDate(t *testing.T) {
	service, uptaneClient := newTestServiceWithOpts(t)

	const configPath = "datadog/2/APM_TRACING/config-123/config.json"
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

	stream := newMockSubscriptionStream(ctx)
	subscriptionDone := make(chan error, 1)
	go func() {
		subscriptionDone <- service.CreateConfigSubscription(stream)
	}()

	stream.clientSendRequest(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-123",
		Products:  []string{"APM_TRACING"},
	})
	require.Eventually(t, func() bool {
		return subscriptionIsRegistered(service, "runtime-123")
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
		Products: []string{"APM_TRACING"},
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

	streamResp, err := stream.clientReceiveResponse(1 * time.Second)
	require.NoError(t, err)
	require.NotNil(t, streamResp)
	require.ElementsMatch(t, fileNames(streamResp.TargetFiles), []string{configPath})
	require.ElementsMatch(t, streamResp.MatchedConfigs, []string{configPath})

	cancel()
	select {
	case <-subscriptionDone:
	case <-time.After(1 * time.Second):
		t.Fatal("subscription handler did not exit")
	}
}

// Ensures that once a subscription has already seen a client, a subsequent poll
// without any new configs does not enqueue another update.
func TestSubscriptionNoUpdateDoesNotNotify(t *testing.T) {
	service, uptaneClient := newTestServiceWithOpts(t)

	const configPath = "datadog/2/APM_TRACING/config-123/config.json"
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

	stream := newMockSubscriptionStream(ctx)
	subscriptionDone := make(chan error, 1)
	go func() {
		subscriptionDone <- service.CreateConfigSubscription(stream)
	}()

	stream.clientSendRequest(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-123",
		Products:  []string{"APM_TRACING"},
	})
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
		Products: []string{"APM_TRACING"},
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

	firstUpdate, err := stream.clientReceiveResponse(1 * time.Second)
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

	// The subscription should not observe a redundant update.
	require.NoError(t, stream.clientReceiveNoResponse(100*time.Millisecond))

	cancel()
	select {
	case <-subscriptionDone:
	case <-time.After(1 * time.Second):
		t.Fatal("subscription handler did not exit")
	}
}

// Ensures that multiple subscriptions can be created for the same runtime_id
// with different products, and that each subscription will receive the files
// that it is interested in.
func TestSubscriptionCanTrackSameRuntimeIDWithDifferentProducts(t *testing.T) {
	service, uptaneClient := newTestServiceWithOpts(t)

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

	stream1 := newMockSubscriptionStream(ctx)
	stream2 := newMockSubscriptionStream(ctx)

	// Start two subscriptions
	sub1Done := make(chan error, 1)
	sub2Done := make(chan error, 1)
	go func() {
		sub1Done <- service.CreateConfigSubscription(stream1)
	}()
	go func() {
		sub2Done <- service.CreateConfigSubscription(stream2)
	}()

	// Both track the same runtime_id with different products
	stream1.clientSendRequest(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-123",
		Products:  []string{"APM_TRACING"},
	})
	stream2.clientSendRequest(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-123",
		Products:  []string{"LIVE_DEBUGGING"},
	})
	require.Eventually(t, func() bool {
		return numSubscriptionsRegisteredForRuntimeID(service, "runtime-123") == 2
	}, 1*time.Second, 10*time.Millisecond)

	// Client polls
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

	// Mark client as active to avoid cache bypass
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

	// Both subscriptions should receive updates
	streamResp1, err := stream1.clientReceiveResponse(1 * time.Second)
	require.NoError(t, err)
	require.NotNil(t, streamResp1)
	require.ElementsMatch(t, fileNames(streamResp1.TargetFiles), []string{apmPath})
	require.ElementsMatch(t, streamResp1.MatchedConfigs, []string{apmPath})

	streamResp2, err := stream2.clientReceiveResponse(1 * time.Second)
	require.NoError(t, err)
	require.NotNil(t, streamResp2)
	require.ElementsMatch(t, fileNames(streamResp2.TargetFiles), []string{liveDebuggingPath})
	require.ElementsMatch(t, streamResp2.MatchedConfigs, []string{liveDebuggingPath})
}

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
		if sub.trackedClients[runtimeID] != nil {
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

func TestSubscriptionInvalidRequests(t *testing.T) {
	service, _ := newTestServiceWithOpts(t)

	tests := []struct {
		name    string
		request *pbgo.ConfigSubscriptionRequest
	}{
		{
			name:    "nil request",
			request: nil,
		},
		{
			name: "empty runtime_id",
			request: &pbgo.ConfigSubscriptionRequest{
				Action:    pbgo.ConfigSubscriptionRequest_TRACK,
				RuntimeId: "",
				Products:  []string{"APM_TRACING"},
			},
		},
		{
			name: "invalid action",
			request: &pbgo.ConfigSubscriptionRequest{
				Action:    pbgo.ConfigSubscriptionRequest_INVALID,
				RuntimeId: "runtime-123",
			},
		},
		{
			name: "TRACK with no products",
			request: &pbgo.ConfigSubscriptionRequest{
				Action:    pbgo.ConfigSubscriptionRequest_TRACK,
				RuntimeId: "runtime-123",
				Products:  []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			stream := newMockSubscriptionStream(ctx)

			subscriptionDone := make(chan error, 1)
			go func() {
				subscriptionDone <- service.CreateConfigSubscription(stream)
			}()

			// Send invalid request
			if tt.request != nil {
				stream.clientSendRequest(tt.request)
			} else {
				// Simulate nil request by closing recvCh and sending error
				stream.errCh <- status.Error(
					codes.InvalidArgument,
					"request cannot be nil",
				)
			}

			// Should get an error back
			select {
			case err := <-subscriptionDone:
				assert.Error(t, err)
			case <-time.After(1 * time.Second):
				t.Fatal("subscription handler did not exit")
			}
		})
	}
}

// Ensures that only tracked clients receive updates.
func TestSubscriptionOnlyTrackedClientsGetUpdates(t *testing.T) {
	service, uptaneClient := newTestServiceWithOpts(t)

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

	stream := newMockSubscriptionStream(ctx)

	subscriptionDone := make(chan error, 1)
	go func() {
		subscriptionDone <- service.CreateConfigSubscription(stream)
	}()

	// Track only runtime-123
	stream.clientSendRequest(&pbgo.ConfigSubscriptionRequest{
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		RuntimeId: "runtime-123",
		Products:  []string{"APM_TRACING"},
	})

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

	_, err := service.ClientGetConfigs(
		context.Background(),
		&pbgo.ClientGetConfigsRequest{
			Client:            otherClient,
			CachedTargetFiles: nil,
		},
	)
	require.NoError(t, err)

	// Subscription should NOT receive an update (different runtime_id)
	err = stream.clientReceiveNoResponse(100 * time.Millisecond)
	assert.NoError(
		t,
		err,
		"Should not receive update for non-tracked client",
	)
}

// Helper function to create a test service with minimal setup
func newTestServiceWithOpts(
	t *testing.T,
) (*CoreAgentService, *mockCoreAgentUptane) {
	api := &mockAPI{}
	uptaneClient := &mockCoreAgentUptane{}
	clock := clock.NewMock()

	service := newTestService(t, api, uptaneClient, clock)

	return service, uptaneClient
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
