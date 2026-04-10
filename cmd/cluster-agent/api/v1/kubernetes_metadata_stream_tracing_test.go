// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package v1

import (
	"context"
	"errors"
	"testing"

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

func TestStreamKubeMetadata_SessionSpan(t *testing.T) {
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
			// Cancel context after initial full state send
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
	var sessionSpan mocktracer.Span
	for _, s := range spans {
		if s.OperationName() == "cluster_agent.metadata_stream.session" {
			sessionSpan = s
			break
		}
	}

	require.NotNil(t, sessionSpan, "expected session span to be created")
	assert.Equal(t, "cluster_agent.metadata_stream.session", sessionSpan.OperationName())
	assert.Equal(t, "session", sessionSpan.Tag("resource.name"))
	assert.Equal(t, "test-node", sessionSpan.Tag("node_name"))
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
	var fullStateSpan mocktracer.Span
	for _, s := range spans {
		if s.OperationName() == "cluster_agent.metadata_stream.send_full_state" {
			fullStateSpan = s
			break
		}
	}

	require.NotNil(t, fullStateSpan, "expected send_full_state span to be created")
	assert.Equal(t, "sendFullState", fullStateSpan.Tag("resource.name"))
	assert.Nil(t, fullStateSpan.Tag("error"))
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
	var fullStateSpan mocktracer.Span
	for _, s := range spans {
		if s.OperationName() == "cluster_agent.metadata_stream.send_full_state" {
			fullStateSpan = s
			break
		}
	}

	require.NotNil(t, fullStateSpan, "expected send_full_state span on error")
	assert.Equal(t, "sendFullState", fullStateSpan.Tag("resource.name"))
	assert.NotNil(t, fullStateSpan.Tag("error"))
}

func TestStreamKubeMetadata_InitialSendErrorSpan(t *testing.T) {
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
	var sessionSpan mocktracer.Span
	for _, s := range spans {
		if s.OperationName() == "cluster_agent.metadata_stream.session" {
			sessionSpan = s
			break
		}
	}

	require.NotNil(t, sessionSpan, "expected session span to be created on error")
	assert.Equal(t, "test-node", sessionSpan.Tag("node_name"))
	// The session span should have the error tag set
	assert.NotNil(t, sessionSpan.Tag("error"))
}
