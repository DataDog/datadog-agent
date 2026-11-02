// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package v1

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/controllers"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"google.golang.org/grpc/metadata"
)

// mockStream implements pb.AgentSecure_StreamKubeMetadataServer for testing.
type mockStream struct {
	ctx      context.Context
	sent     []*pb.KubeMetadataStreamResponse
	sendErr  error
	sendFunc func(*pb.KubeMetadataStreamResponse) error // optional per-call control
}

func (m *mockStream) Send(resp *pb.KubeMetadataStreamResponse) error {
	if m.sendFunc != nil {
		return m.sendFunc(resp)
	}
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = append(m.sent, resp)
	return nil
}

func (m *mockStream) Context() context.Context     { return m.ctx }
func (m *mockStream) SetHeader(metadata.MD) error  { return nil }
func (m *mockStream) SendHeader(metadata.MD) error { return nil }
func (m *mockStream) SetTrailer(metadata.MD)       {}
func (m *mockStream) SendMsg(interface{}) error    { return nil }
func (m *mockStream) RecvMsg(interface{}) error    { return nil }

func newTestWmetaAndStore(t *testing.T) (workloadmetamock.Mock, *controllers.MetaBundleStore) {
	t.Helper()
	wmetaMock := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	store := controllers.GetGlobalMetaBundleStore()
	return wmetaMock, store
}

func TestStreamKubeMetadata_InitialFullStateSendSpan(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	wmetaMock, store := newTestWmetaAndStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	srv := NewKubeMetadataStreamServer(store, wmetaMock)
	srv.Start(ctx)

	sendCount := 0
	stream := &mockStream{
		ctx: ctx,
		sendFunc: func(_ *pb.KubeMetadataStreamResponse) error {
			sendCount++
			if sendCount == 1 {
				cancel()
			}
			return nil
		},
	}

	req := &pb.KubeMetadataStreamRequest{
		NodeName: "test-node",
	}

	err := srv.StreamKubeMetadata(req, stream)
	require.NoError(t, err)

	spans := mt.FinishedSpans()
	var fullStateSpan *mocktracer.Span
	for _, s := range spans {
		if s.OperationName() == "cluster_agent.metadata_stream.send_full_state" {
			fullStateSpan = s
			break
		}
	}

	require.NotNil(t, fullStateSpan, "expected send_full_state span to be created")
	assert.Equal(t, "sendFullState", fullStateSpan.Tag("resource.name"))
	assert.Equal(t, "test-node", fullStateSpan.Tag("node_name"))
	assert.Nil(t, fullStateSpan.Tag("error.message"))
}

func TestStreamKubeMetadata_InitialFullStateSendErrorSpan(t *testing.T) {
	mt := mocktracer.Start()
	defer mt.Stop()

	wmetaMock, store := newTestWmetaAndStore(t)

	ctx := context.Background()
	srv := NewKubeMetadataStreamServer(store, wmetaMock)
	srv.Start(ctx)

	sendErr := errors.New("send failed")
	stream := &mockStream{
		ctx:     ctx,
		sendErr: sendErr,
	}

	req := &pb.KubeMetadataStreamRequest{
		NodeName: "test-node",
	}

	err := srv.StreamKubeMetadata(req, stream)
	require.Error(t, err)

	spans := mt.FinishedSpans()
	var fullStateSpan *mocktracer.Span
	for _, s := range spans {
		if s.OperationName() == "cluster_agent.metadata_stream.send_full_state" {
			fullStateSpan = s
			break
		}
	}

	require.NotNil(t, fullStateSpan, "expected send_full_state span on error")
	assert.Equal(t, "sendFullState", fullStateSpan.Tag("resource.name"))
	assert.Equal(t, "test-node", fullStateSpan.Tag("node_name"))
	assert.NotNil(t, fullStateSpan.Tag("error.message"))
}

// newKAIStreamTestServer returns a server with the "local" PodGroup on node1
// and the "remote" PodGroup on node2.
func newKAIStreamTestServer() *KubeMetadataStreamServer {
	srv := NewKubeMetadataStreamServer(controllers.GetGlobalMetaBundleStore(), nil)
	srv.processWmetaEvents([]workloadmeta.Event{
		testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-a", "local", "queue-a"),
		testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-a", "remote", "queue-a"),
		testKAIPodEvent(workloadmeta.EventTypeSet, "p1", "team-a", "node1", "local"),
		testKAIPodEvent(workloadmeta.EventTypeSet, "p2", "team-a", "node2", "remote"),
	})
	return srv
}

// startTestStream streams to nodeName until the test ends and returns a
// function that waits for the next response. beforeSend, if set, is called
// with the number of the send before each one.
func startTestStream(t *testing.T, srv *KubeMetadataStreamServer, nodeName string, beforeSend func(send int)) func() *pb.KubeMetadataStreamResponse {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	sent := make(chan *pb.KubeMetadataStreamResponse, 16)
	sends := 0
	stream := &mockStream{
		ctx: ctx,
		sendFunc: func(resp *pb.KubeMetadataStreamResponse) error {
			sends++
			if beforeSend != nil {
				beforeSend(sends)
			}
			sent <- resp
			return nil
		},
	}
	done := make(chan error)
	go func() {
		done <- srv.StreamKubeMetadata(&pb.KubeMetadataStreamRequest{NodeName: nodeName}, stream)
	}()
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-done)
	})

	return func() *pb.KubeMetadataStreamResponse {
		t.Helper()
		select {
		case resp := <-sent:
			return resp
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for a stream response")
			return nil
		}
	}
}

func TestStreamKubeMetadata_KAIPodGroupsPerNode(t *testing.T) {
	srv := newKAIStreamTestServer()
	next := startTestStream(t, srv, "node1", nil)

	initial := next()
	assert.True(t, initial.IsFullState)
	require.Len(t, initial.KaiPodGroups, 1, "the full state only has the node's PodGroups")
	assert.Equal(t, "local", initial.KaiPodGroups[0].Name)

	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-a", "local", "queue-b")})
	update := next()
	assert.False(t, update.IsFullState)
	require.Len(t, update.KaiPodGroups, 1)
	assert.Equal(t, "queue-b", update.KaiPodGroups[0].Queue)
	assert.Equal(t, pb.KubeMetadataEventType_SET, update.KaiPodGroups[0].Type)

	// Another node's PodGroup is not sent: the next response is the UNSET of
	// the pod moving to another node.
	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-a", "remote", "queue-b")})
	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodEvent(workloadmeta.EventTypeSet, "p1", "team-a", "node2", "local")})
	update = next()
	require.Len(t, update.KaiPodGroups, 1)
	assert.Equal(t, "local", update.KaiPodGroups[0].Name)
	assert.Equal(t, pb.KubeMetadataEventType_UNSET, update.KaiPodGroups[0].Type)
}

func TestStreamKubeMetadata_KAIPodGroupsOnHistoryResync(t *testing.T) {
	srv := newKAIStreamTestServer()
	blocked := make(chan struct{})
	unblock := make(chan struct{})
	next := startTestStream(t, srv, "node1", func(send int) {
		if send == 2 {
			// Hold the first incremental update so the stream falls behind
			// the retained history.
			close(blocked)
			<-unblock
		}
	})

	require.True(t, next().IsFullState)

	srv.processWmetaEvents([]workloadmeta.Event{testKueueWorkloadEvent("0", workloadmeta.EventTypeSet)})
	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the stream to send version 1")
	}
	// Overflow the history while the stream is blocked sending version 1.
	for i := 1; i <= metadataHistorySize+1; i++ {
		srv.processWmetaEvents([]workloadmeta.Event{testKueueWorkloadEvent(strconv.Itoa(i), workloadmeta.EventTypeSet)})
	}
	// The PodGroup changes while the stream is behind.
	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-a", "local", "queue-b")})
	close(unblock)

	assert.False(t, next().IsFullState)
	resync := next()
	require.True(t, resync.IsFullState)
	require.Len(t, resync.KaiPodGroups, 1, "the resync only has the node's PodGroups")
	assert.Equal(t, "local", resync.KaiPodGroups[0].Name)
	assert.Equal(t, "queue-b", resync.KaiPodGroups[0].Queue)

	// The resync is the new baseline: the next change is incremental.
	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-a", "local", "queue-c")})
	update := next()
	assert.False(t, update.IsFullState)
	require.Len(t, update.KaiPodGroups, 1)
	assert.Equal(t, "queue-c", update.KaiPodGroups[0].Queue)
}
