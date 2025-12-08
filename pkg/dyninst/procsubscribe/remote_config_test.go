// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procsubscribe_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procsubscribe"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procsubscribe/procscan"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestMain(m *testing.M) {
	dyninsttest.SetupLogging()
	os.Exit(m.Run())
}

const (
	runtimeID1 = "runtime-1"
	runtimeID2 = "runtime-2"
	pid1       = int32(4242)
	pid2       = int32(4243)
)

var (
	md1 = tracermetadata.TracerMetadata{
		RuntimeID:      runtimeID1,
		ServiceName:    "svc",
		ServiceEnv:     "prod",
		ServiceVersion: "1.0.0",
	}
	md2 = tracermetadata.TracerMetadata{
		RuntimeID:      runtimeID2,
		ServiceName:    "svc",
		ServiceEnv:     "prod",
		ServiceVersion: "1.0.0",
	}
	client1 = &pbgo.Client{
		ClientTracer: &pbgo.ClientTracer{
			RuntimeId:  runtimeID1,
			Service:    "svc",
			Env:        "prod",
			AppVersion: "1.0.0",
		},
	}
	client1WithExtraInfo = &pbgo.Client{
		ClientTracer: &pbgo.ClientTracer{
			RuntimeId:     runtimeID1,
			Service:       "svc",
			Env:           "prod",
			AppVersion:    "1.0.0",
			ContainerTags: []string{"container_id:container-42"},
			ProcessTags: []string{
				"process_tag:1234567890",
				"git.repository_url:https://github.com/org/repo",
				"git.commit.sha:deadbeef",
			},
			Tags: []string{
				"tag1:1234567890",
				"tag2:1234567890",
				"tag3:1234567890",
			},
		},
	}
	client2 = &pbgo.Client{
		ClientTracer: &pbgo.ClientTracer{
			RuntimeId:  runtimeID2,
			Service:    "svc",
			Env:        "prod",
			AppVersion: "1.0.0",
		},
	}
)

type waitRequest struct {
	duration time.Duration
	doneCh   chan struct{}
}

func (r waitRequest) Close() {
	close(r.doneCh)
}

type waitRequestChan chan waitRequest

func (c waitRequestChan) expect(t *testing.T, duration time.Duration) waitRequest {
	select {
	case req := <-c:
		require.Equal(t, duration, req.duration)
		return req
	case <-time.After(time.Second):
		t.Fatalf("expected request for %s", duration)
		panic("unreachable")
	}
}

func (c waitRequestChan) Wait(ctx context.Context, duration time.Duration) error {
	req := waitRequest{
		duration: duration,
		doneCh:   make(chan struct{}),
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case c <- req:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-req.doneCh:
		return nil
	}
}

func TestRemoteConfigProcessSubscriberTracksUpdatesAndRemovals(t *testing.T) {
	goleak.VerifyNone(t, goleak.IgnoreCurrent())

	mockClock := clock.NewMock()
	scanner := &stubScanner{
		results: []scanResult{
			{
				added: []procscan.DiscoveredProcess{
					{
						PID:            uint32(pid1),
						TracerMetadata: md1,
						Executable:     process.Executable{Path: "/exe1"},
					},
					{
						PID:            uint32(pid2),
						TracerMetadata: md2,
						Executable:     process.Executable{Path: "/exe2"},
					},
				},
			},
			{
				removed: []procscan.ProcessID{
					procscan.ProcessID(pid1),
					procscan.ProcessID(pid2),
				},
			},
		},
	}

	waitRequests := make(waitRequestChan)
	streams, remoteSub := runFakeAgentSecureServer(t)
	subscriber := procsubscribe.NewRemoteConfigProcessSubscriber(
		remoteSub,
		procsubscribe.WithProcessScanner(scanner),
		procsubscribe.WithClock(mockClock),
		procsubscribe.WithJitterFactor(0),
		procsubscribe.WithWaitFunc(waitRequests.Wait),
	)
	t.Cleanup(subscriber.Close)

	updatesCh := make(chan process.ProcessesUpdate, 100)
	subscriber.Subscribe(func(update process.ProcessesUpdate) {
		updatesCh <- update
	})
	subscriber.Start()

	// Wait for both the scan and the stream manager to start.
	w1, w2 := waitRequests.expect(t, 0), waitRequests.expect(t, 0)
	w1.Close()
	w2.Close()
	s := <-streams
	// Now only the scanner should come and wait.
	scanWait := waitRequests.expect(t, 3*time.Second)

	mockClock.Add(time.Millisecond)

	trackReq, err := s.stream.Recv()
	require.NoError(t, err)
	require.EqualExportedValues(t, &pbgo.ConfigSubscriptionRequest{
		RuntimeId: runtimeID1,
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	}, trackReq)
	trackReq, err = s.stream.Recv()
	require.NoError(t, err)
	require.EqualExportedValues(t, &pbgo.ConfigSubscriptionRequest{
		RuntimeId: runtimeID2,
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	}, trackReq)

	t.Logf("sending error to trigger a reconnect")
	s.errCh <- errors.New("boom")
	t.Logf("waiting for new stream")
	waitRequests.expect(t, 200*time.Millisecond).Close()
	s = <-streams
	t.Logf("new stream received")

	// Expect fresh subscriptions after a successful connection.
	trackReq, err = s.stream.Recv()
	require.NoError(t, err)
	require.EqualExportedValues(t, &pbgo.ConfigSubscriptionRequest{
		RuntimeId: runtimeID1,
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	}, trackReq)
	trackReq, err = s.stream.Recv()
	require.NoError(t, err)
	require.EqualExportedValues(t, &pbgo.ConfigSubscriptionRequest{
		RuntimeId: runtimeID2,
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	}, trackReq)

	const configPath = "datadog/1/LIVE_DEBUGGING/config-id/probe.json"
	probeJSON := `{"id":"probe-id","version":1,"type":"METRIC_PROBE","where":{"typeName":"pkg.Type","methodName":"Func"},"kind":"count","metricName":"test.metric"}`

	s.stream.Send(&pbgo.ConfigSubscriptionResponse{
		MatchedConfigs: []string{configPath},
		Client:         client1,
		TargetFiles: []*pbgo.File{
			{
				Path: configPath,
				Raw:  []byte(probeJSON + "\n"),
			},
			{
				Path: fmt.Sprintf("datadog/1/%s/foo/symdb.json", data.ProductLiveDebuggingSymbolDB),
				Raw:  []byte(`{"upload_symbols":true}`),
			},
		},
	})

	update := <-updatesCh
	require.Len(t, update.Updates, 1)
	require.Equal(t, runtimeID1, update.Updates[0].RuntimeID)

	s.stream.Send(&pbgo.ConfigSubscriptionResponse{
		MatchedConfigs: []string{configPath},
		Client:         client2,
		TargetFiles: []*pbgo.File{
			{
				Path: configPath,
				Raw:  []byte(probeJSON),
			},
		},
	})

	update = <-updatesCh
	require.Len(t, update.Updates, 1)
	require.Equal(t, runtimeID2, update.Updates[0].RuntimeID)

	log.Infof("adding default scan interval to trigger a scan update")
	scanWait.Close()

	untrackReq, err := s.stream.Recv()
	require.NoError(t, err)
	require.Equal(t, pbgo.ConfigSubscriptionRequest_UNTRACK, untrackReq.GetAction())
	require.Equal(t, runtimeID1, untrackReq.GetRuntimeId())
	untrackReq, err = s.stream.Recv()
	require.NoError(t, err)
	require.Equal(t, pbgo.ConfigSubscriptionRequest_UNTRACK, untrackReq.GetAction())
	require.Equal(t, runtimeID2, untrackReq.GetRuntimeId())

	removal := <-updatesCh
	require.Len(t, removal.Removals, 2)
	require.ElementsMatch(t, []process.ID{
		{PID: pid1},
		{PID: pid2},
	}, removal.Removals)
	subscriber.Close()
}

func TestRetrackAfterNewStream(t *testing.T) {
	goleak.VerifyNone(t, goleak.IgnoreCurrent())

	mockClock := clock.NewMock()
	scanner := &stubScanner{
		results: []scanResult{
			{
				added: []procscan.DiscoveredProcess{
					{
						PID:            uint32(pid1),
						TracerMetadata: md1,
						Executable:     process.Executable{Path: "/exe1"},
					},
					{
						PID:            uint32(pid2),
						TracerMetadata: md2,
						Executable:     process.Executable{Path: "/exe2"},
					},
				},
			},
			{
				removed: []procscan.ProcessID{
					procscan.ProcessID(pid1),
					procscan.ProcessID(pid2),
				},
			},
		},
	}

	streams, remoteSub := runFakeAgentSecureServer(t)
	subscriber := procsubscribe.NewRemoteConfigProcessSubscriber(
		remoteSub,
		procsubscribe.WithProcessScanner(scanner),
		procsubscribe.WithClock(mockClock),
		procsubscribe.WithJitterFactor(0),
	)
	t.Cleanup(subscriber.Close)

	updatesCh := make(chan process.ProcessesUpdate, 2)
	subscriber.Subscribe(func(update process.ProcessesUpdate) {
		updatesCh <- update
	})
	subscriber.Start()

	s := <-streams
	mockClock.Add(time.Millisecond)

	trackReq, err := s.stream.Recv()
	require.NoError(t, err)
	require.EqualExportedValues(t, &pbgo.ConfigSubscriptionRequest{
		RuntimeId: runtimeID1,
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	}, trackReq)
	trackReq, err = s.stream.Recv()
	require.NoError(t, err)
	require.EqualExportedValues(t, &pbgo.ConfigSubscriptionRequest{
		RuntimeId: runtimeID2,
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	}, trackReq)

	mockClock.Add(procsubscribe.DefaultScanInterval)

	untrackReq, err := s.stream.Recv()
	require.NoError(t, err)
	require.EqualExportedValues(t, &pbgo.ConfigSubscriptionRequest{
		RuntimeId: runtimeID1,
		Action:    pbgo.ConfigSubscriptionRequest_UNTRACK,
	}, untrackReq)
	untrackReq, err = s.stream.Recv()
	require.NoError(t, err)
	require.EqualExportedValues(t, &pbgo.ConfigSubscriptionRequest{
		RuntimeId: runtimeID2,
		Action:    pbgo.ConfigSubscriptionRequest_UNTRACK,
	}, untrackReq)

	removal := <-updatesCh
	require.Len(t, removal.Removals, 2)
	require.ElementsMatch(t, []process.ID{
		{PID: pid1},
		{PID: pid2},
	}, removal.Removals)
	subscriber.Close()
}

func TestRemoteConfigSymDBUpdates(t *testing.T) {
	goleak.VerifyNone(t, goleak.IgnoreCurrent())

	mockClock := clock.NewMock()
	scanner := &stubScanner{
		results: []scanResult{
			{
				added: []procscan.DiscoveredProcess{
					{
						PID:            uint32(pid1),
						TracerMetadata: md1,
						Executable:     process.Executable{Path: "/exe1"},
					},
				},
			},
		},
	}

	streams, remoteSub := runFakeAgentSecureServer(t)
	subscriber := procsubscribe.NewRemoteConfigProcessSubscriber(
		remoteSub,
		procsubscribe.WithProcessScanner(scanner),
		procsubscribe.WithClock(mockClock),
		procsubscribe.WithJitterFactor(0),
	)
	t.Cleanup(subscriber.Close)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	updatesCh := make(chan process.ProcessesUpdate, 2)
	subscriber.Subscribe(func(update process.ProcessesUpdate) {
		select {
		case updatesCh <- update:
		case <-ctx.Done():
		case <-time.After(time.Second):
			cancel()
			t.Error("expected send to succeed")
		}
	})
	subscriber.Start()

	s := receiveSoon(t, streams)

	trackReq, err := s.stream.Recv()
	require.NoError(t, err)
	require.EqualExportedValues(t, &pbgo.ConfigSubscriptionRequest{
		RuntimeId: runtimeID1,
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	}, trackReq)

	symdbPath := fmt.Sprintf("datadog/1/%s/foo/symdb.json", data.ProductLiveDebuggingSymbolDB)
	firstResp := &pbgo.ConfigSubscriptionResponse{
		Client:         client1,
		MatchedConfigs: []string{symdbPath},
		TargetFiles: []*pbgo.File{
			{
				Path: symdbPath,
				Raw:  []byte(`{"upload_symbols":true}`),
			},
		},
	}
	require.NoError(t, s.stream.Send(firstResp))
	getUpdate := func() process.ProcessesUpdate {
		t.Helper()
		select {
		case update := <-updatesCh:
			return update
		case <-time.After(time.Second):
			t.Fatal("expected update")
			panic("unreachable")
		}
	}

	update := getUpdate()
	require.Len(t, update.Updates, 1)
	require.Equal(t, runtimeID1, update.Updates[0].RuntimeID)
	require.True(t, update.Updates[0].ShouldUploadSymDB)

	require.NoError(t, s.stream.Send(firstResp))
	select {
	case <-time.After(20 * time.Millisecond):
	case <-updatesCh:
		t.Fatal("expected no update")
	}

	// Send an empty response to remove the SymDB config.
	require.NoError(t, s.stream.Send(&pbgo.ConfigSubscriptionResponse{
		Client:         client1,
		MatchedConfigs: []string{},
	}))

	update = getUpdate()
	require.Len(t, update.Updates, 1)
	require.Equal(t, runtimeID1, update.Updates[0].RuntimeID)
	require.False(t, update.Updates[0].ShouldUploadSymDB)

	// Send a response with the SymDB config again.
	require.NoError(t, s.stream.Send(firstResp))
	update = getUpdate()
	require.Len(t, update.Updates, 1)
	require.Equal(t, runtimeID1, update.Updates[0].RuntimeID)
	require.True(t, update.Updates[0].ShouldUploadSymDB)

	subscriber.Close()
}

func receiveSoon[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case val := <-ch:
		return val
	case <-time.After(time.Second):
		t.Fatal("expected value")
		panic("unreachable")
	}
}

func TestContainerAndGitInfoParsing(t *testing.T) {
	goleak.VerifyNone(t, goleak.IgnoreCurrent())

	mockClock := clock.NewMock()
	scanner := &stubScanner{
		results: []scanResult{
			{
				added: []procscan.DiscoveredProcess{
					{
						PID:            uint32(pid1),
						TracerMetadata: md1,
						Executable:     process.Executable{Path: "/exe1"},
					},
				},
			},
		},
	}

	streams, remoteSub := runFakeAgentSecureServer(t)
	subscriber := procsubscribe.NewRemoteConfigProcessSubscriber(
		remoteSub,
		procsubscribe.WithProcessScanner(scanner),
		procsubscribe.WithClock(mockClock),
		procsubscribe.WithJitterFactor(0),
	)
	t.Cleanup(subscriber.Close)

	updatesCh := make(chan process.ProcessesUpdate, 2)
	subscriber.Subscribe(func(update process.ProcessesUpdate) {
		updatesCh <- update
	})
	subscriber.Start()

	s := <-streams

	mockClock.Add(time.Millisecond)

	trackReq, err := s.stream.Recv()
	require.NoError(t, err)
	require.EqualExportedValues(t, &pbgo.ConfigSubscriptionRequest{
		RuntimeId: runtimeID1,
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	}, trackReq)

	probePath := "datadog/1/LIVE_DEBUGGING/config-id/probe.json"
	probeJSON := `{"id":"probe-id","version":1,"type":"METRIC_PROBE","where":{"typeName":"pkg.Type","methodName":"Func"},"kind":"count","metricName":"test.metric"}`
	firstResp := &pbgo.ConfigSubscriptionResponse{
		Client:         client1WithExtraInfo,
		MatchedConfigs: []string{probePath},
		TargetFiles: []*pbgo.File{
			{
				Path: probePath,
				Raw:  []byte(probeJSON),
			},
		},
	}
	require.NoError(t, s.stream.Send(firstResp))

	update := <-updatesCh
	require.Len(t, update.Updates, 1)
	require.Equal(t, runtimeID1, update.Updates[0].RuntimeID)
	require.Equal(t, "svc", update.Updates[0].Service)
	require.Equal(t, "prod", update.Updates[0].Environment)
	require.Equal(t, "1.0.0", update.Updates[0].Version)
	require.Equal(t, process.GitInfo{
		RepositoryURL: "https://github.com/org/repo",
		CommitSha:     "deadbeef",
	}, update.Updates[0].GitInfo)
	require.Equal(t, process.ContainerInfo{
		ContainerID: "container-42",
	}, update.Updates[0].Container)

	subscriber.Close()
}

func TestExponentialBackoffUpToMaxDelayForNewStream(t *testing.T) {
	goleak.VerifyNone(t, goleak.IgnoreCurrent())

	scanner := &stubScanner{}
	mockClock := clock.NewMock()
	streams, remoteSub := runFakeAgentSecureServer(t)
	waitRequests := make(waitRequestChan)
	subscriber := procsubscribe.NewRemoteConfigProcessSubscriber(
		remoteSub,
		procsubscribe.WithProcessScanner(scanner),
		procsubscribe.WithClock(mockClock),
		procsubscribe.WithJitterFactor(0),
		procsubscribe.WithWaitFunc(waitRequests.Wait),
	)
	t.Cleanup(subscriber.Close)
	subscriber.Start()

	w1, w2 := waitRequests.expect(t, 0), waitRequests.expect(t, 0)
	w1.Close()
	w2.Close()
	t.Logf("waiting for scanner to start")
	s := <-streams
	scanWait := waitRequests.expect(t, 3*time.Second)
	defer scanWait.Close()

	var durations []time.Duration
	for i := 0; i < 10; i++ {
		s.errCh <- errors.New("boom")
		req := <-waitRequests
		durations = append(durations, req.duration)
		req.Close()
		s = <-streams
	}
	require.Equal(t, []time.Duration{
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1600 * time.Millisecond,
		3200 * time.Millisecond,
		6400 * time.Millisecond,
		12800 * time.Millisecond,
		25600 * time.Millisecond,
		30 * time.Second,
		30 * time.Second, // max delay
	}, durations)
}

func TestSymDBStatePreservedWhenNotInMatchedConfigs(t *testing.T) {
	goleak.VerifyNone(t, goleak.IgnoreCurrent())

	mockClock := clock.NewMock()
	scanner := &stubScanner{
		results: []scanResult{
			{
				added: []procscan.DiscoveredProcess{
					{
						PID:            uint32(pid1),
						TracerMetadata: md1,
						Executable:     process.Executable{Path: "/exe1"},
					},
				},
			},
		},
	}

	streams, remoteSub := runFakeAgentSecureServer(t)
	subscriber := procsubscribe.NewRemoteConfigProcessSubscriber(
		remoteSub,
		procsubscribe.WithProcessScanner(scanner),
		procsubscribe.WithClock(mockClock),
		procsubscribe.WithJitterFactor(0),
	)
	t.Cleanup(subscriber.Close)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	updatesCh := make(chan process.ProcessesUpdate, 2)
	subscriber.Subscribe(func(update process.ProcessesUpdate) {
		select {
		case updatesCh <- update:
		case <-ctx.Done():
		case <-time.After(time.Second):
			cancel()
			t.Error("expected send to succeed")
		}
	})
	subscriber.Start()

	s := receiveSoon(t, streams)

	trackReq, err := s.stream.Recv()
	require.NoError(t, err)
	require.EqualExportedValues(t, &pbgo.ConfigSubscriptionRequest{
		RuntimeId: runtimeID1,
		Action:    pbgo.ConfigSubscriptionRequest_TRACK,
		Products:  pbgo.ConfigSubscriptionProducts_LIVE_DEBUGGING,
	}, trackReq)

	symdbPath := fmt.Sprintf("datadog/1/%s/foo/symdb.json", data.ProductLiveDebuggingSymbolDB)
	probePath := "datadog/1/LIVE_DEBUGGING/config-id/probe.json"
	probeJSON := `{"id":"probe-id","version":1,"type":"METRIC_PROBE","where":{"typeName":"pkg.Type","methodName":"Func"},"kind":"count","metricName":"test.metric"}`

	// Initial config with both symdb and probe.
	initialResp := &pbgo.ConfigSubscriptionResponse{
		Client:         client1,
		MatchedConfigs: []string{symdbPath, probePath},
		TargetFiles: []*pbgo.File{
			{
				Path: symdbPath,
				Raw:  []byte(`{"upload_symbols":true}`),
			},
			{
				Path: probePath,
				Raw:  []byte(probeJSON),
			},
		},
	}
	require.NoError(t, s.stream.Send(initialResp))

	getUpdate := func() process.ProcessesUpdate {
		t.Helper()
		select {
		case update := <-updatesCh:
			return update
		case <-time.After(time.Second):
			t.Fatal("expected update")
			panic("unreachable")
		}
	}

	update := getUpdate()
	require.Len(t, update.Updates, 1)
	require.Equal(t, runtimeID1, update.Updates[0].RuntimeID)
	require.True(t, update.Updates[0].ShouldUploadSymDB)

	// Send a config update with only probe config (symdb not in
	// MatchedConfigs). This tests the bug fix where symdb state should be
	// preserved rather than incorrectly disabled.
	probeOnlyResp := &pbgo.ConfigSubscriptionResponse{
		Client:         client1,
		MatchedConfigs: []string{probePath, symdbPath},
		TargetFiles:    []*pbgo.File{},
	}
	require.NoError(t, s.stream.Send(probeOnlyResp))

	// Should NOT receive an update because symdb state hasn't changed (it
	// should remain enabled from the initial config).
	select {
	case <-time.After(50 * time.Millisecond):
	case update := <-updatesCh:
		t.Fatalf("expected no update, got update with %d items: %#v", len(update.Updates), update)
	}

	// Now explicitly remove symdb to verify it can still be disabled.
	emptyResp := &pbgo.ConfigSubscriptionResponse{
		Client:         client1,
		MatchedConfigs: []string{},
	}
	require.NoError(t, s.stream.Send(emptyResp))

	update = getUpdate()
	require.Len(t, update.Updates, 1)
	require.Equal(t, runtimeID1, update.Updates[0].RuntimeID)
	require.False(t, update.Updates[0].ShouldUploadSymDB)

	subscriber.Close()
}

type scanResult struct {
	added   []procscan.DiscoveredProcess
	removed []procscan.ProcessID
	err     error
}

type stubScanner struct {
	mu      sync.Mutex
	results []scanResult
}

func (s *stubScanner) Scan() ([]procscan.DiscoveredProcess, []procscan.ProcessID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.results) == 0 {
		return nil, nil, nil
	}
	res := s.results[0]
	s.results = s.results[1:]
	added := append([]procscan.DiscoveredProcess(nil), res.added...)
	removed := append([]procscan.ProcessID(nil), res.removed...)
	return added, removed, res.err
}

type fakeAgentSecureServer struct {
	pbgo.UnimplementedAgentSecureServer
	streams chan<- configSubscription
}

type configSubscription struct {
	errCh  chan<- error
	stream pbgo.AgentSecure_CreateConfigSubscriptionServer
}

func (s *fakeAgentSecureServer) CreateConfigSubscription(stream pbgo.AgentSecure_CreateConfigSubscriptionServer) error {
	defer func() { log.Infof("CreateConfigSubscription done") }()
	log.Infof("CreateConfigSubscription started")
	if err := stream.SendHeader(metadata.MD{}); err != nil {
		log.Errorf("failed to send header: %v", err)
		return err
	}
	errCh := make(chan error)
	ctx := stream.Context()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.streams <- configSubscription{stream: stream, errCh: errCh}:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func runFakeAgentSecureServer(t *testing.T) (<-chan configSubscription, procsubscribe.RemoteConfigSubscriber) {
	streams := make(chan configSubscription)
	server := grpc.NewServer()
	bufconn := bufconn.Listen(1024)
	pbgo.RegisterAgentSecureServer(server, &fakeAgentSecureServer{streams: streams})
	go func() {
		_ = server.Serve(bufconn)
	}()
	t.Cleanup(server.Stop)
	c, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return bufconn.Dial()
		}),
	)
	require.NoError(t, err)
	client := pbgo.NewAgentSecureClient(c)
	return streams, client
}
