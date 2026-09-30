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
	agentcache "github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
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

func TestStreamKubeMetadata_ReplayAndResync(t *testing.T) {
	for _, overflow := range []bool{false, true} {
		t.Run(strconv.FormatBool(overflow), func(t *testing.T) {
			store := controllers.GetGlobalMetaBundleStore()
			srv := NewKubeMetadataStreamServer(store, nil)
			srv.processWmetaEvents([]workloadmeta.Event{testKueueWorkloadEvent("old", workloadmeta.EventTypeSet)})
			nodeName := t.Name()
			cacheKey := agentcache.BuildAgentKey(apiserver.MetadataMapperCachePrefix, nodeName)
			t.Cleanup(func() { agentcache.Cache.Delete(cacheKey) })
			bundle := apiserver.NewMetadataMapperBundle()
			bundle.Services.Set("ns1", "pod1", "svc1")
			agentcache.Cache.Set(cacheKey, bundle, time.Minute)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			updateCount := 2
			expectedSends := 1 + 1 + updateCount // initial state, deletion, updates
			if overflow {
				updateCount = metadataHistorySize
				expectedSends = 2 // initial state, resync
			}
			var sent []*pb.KubeMetadataStreamResponse
			stream := &mockStream{ctx: ctx}
			stream.sendFunc = func(resp *pb.KubeMetadataStreamResponse) error {
				sent = append(sent, resp)
				if len(sent) == 1 {
					// Updates arrive while the initial state is in flight; all
					// notifications coalesce into a single wakeup.
					srv.processWmetaEvents([]workloadmeta.Event{testKueueWorkloadEvent("old", workloadmeta.EventTypeUnset)})
					for i := range updateCount {
						events := testNamespaceSetEvents()
						events[0].Entity.(*workloadmeta.KubernetesMetadata).Labels = map[string]string{"revision": strconv.Itoa(i)}
						srv.processWmetaEvents(events)
					}
					newBundle := apiserver.NewMetadataMapperBundle()
					newBundle.Services.Set("ns1", "pod1", "svc2")
					agentcache.Cache.Set(cacheKey, newBundle, time.Minute)
				} else if len(sent) == expectedSends {
					cancel()
				}
				return nil
			}
			require.NoError(t, srv.StreamKubeMetadata(&pb.KubeMetadataStreamRequest{NodeName: nodeName}, stream))
			require.Len(t, sent, expectedSends)
			assert.True(t, sent[0].IsFullState)
			require.Len(t, sent[0].KueueWorkloads, 1)
			assert.Equal(t, "old", sent[0].KueueWorkloads[0].Uid)
			last := sent[len(sent)-1]
			require.Len(t, last.NamespaceMetadata, 1)
			assert.Equal(t, strconv.Itoa(updateCount-1), last.NamespaceMetadata[0].Labels["revision"])
			if overflow {
				assert.True(t, last.IsFullState)
				assert.Empty(t, last.KueueWorkloads, "resync removes the deleted workload")
				require.Len(t, last.Mappings, 1)
				assert.Equal(t, []string{"svc2"}, last.Mappings[0].ServiceNames, "resync includes current node mappings")
			} else {
				for _, resp := range sent[1:] {
					assert.False(t, resp.IsFullState)
				}
				require.Len(t, sent[1].KueueWorkloads, 1)
				assert.Equal(t, pb.KubeMetadataEventType_UNSET, sent[1].KueueWorkloads[0].Type)
			}
			assert.Empty(t, srv.namespaceSubscribers)
		})
	}
}

func TestStreamKubeMetadata_UpdateSendError(t *testing.T) {
	srv := NewKubeMetadataStreamServer(controllers.GetGlobalMetaBundleStore(), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sendErr := errors.New("update send failed")
	stream := &mockStream{ctx: ctx}
	stream.sendFunc = func(resp *pb.KubeMetadataStreamResponse) error {
		if resp.IsFullState {
			srv.processWmetaEvents(testNamespaceSetEvents())
			return nil
		}
		return sendErr
	}
	assert.ErrorIs(t, srv.StreamKubeMetadata(&pb.KubeMetadataStreamRequest{NodeName: t.Name()}, stream), sendErr)
	assert.Empty(t, srv.namespaceSubscribers)
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
