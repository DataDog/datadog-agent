// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package dogtelextensionimpl

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

// startBareGRPCServer starts a real gRPC server on an ephemeral port and
// returns the server and the listener. The caller owns both.
func startBareGRPCServer(t *testing.T) (*grpc.Server, net.Listener) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	pb.RegisterAgentSecureServer(srv, &pb.UnimplementedAgentSecureServer{})
	go func() {
		_ = srv.Serve(lis)
	}()
	return srv, lis
}

// TestStopTaggerServer_NilServer verifies stopTaggerServer is a no-op and does
// not panic when neither taggerServer nor taggerListener are set.
func TestStopTaggerServer_NilServer(t *testing.T) {
	ext := &dogtelExtension{
		log:        logmock.New(t),
		coreConfig: configmock.NewMockWithOverrides(t, nil),
		serializer: serializermock.NewMetricSerializer(t),
		ipc:        ipcmock.New(t),
	}
	// Must not panic and must leave fields nil.
	assert.NotPanics(t, func() { ext.stopTaggerServer() })
	assert.Nil(t, ext.taggerServer)
	assert.Nil(t, ext.taggerListener)
}

// TestStopTaggerServer_CleanStop verifies that stopTaggerServer completes
// quickly when no clients are connected (GracefulStop returns immediately).
func TestStopTaggerServer_CleanStop(t *testing.T) {
	srv, lis := startBareGRPCServer(t)

	ext := &dogtelExtension{
		log:            logmock.New(t),
		coreConfig:     configmock.NewMockWithOverrides(t, nil),
		serializer:     serializermock.NewMetricSerializer(t),
		ipc:            ipcmock.New(t),
		taggerServer:   srv,
		taggerListener: lis,
	}

	start := time.Now()
	ext.stopTaggerServer()
	elapsed := time.Since(start)

	// Should finish well within the graceful-stop timeout.
	assert.Less(t, elapsed, taggerServerGracefulStopTimeout,
		"stopTaggerServer took longer than expected for an idle server")
	assert.Nil(t, ext.taggerServer)
	assert.Nil(t, ext.taggerListener)
}

// TestStopTaggerServer_ForcedStop verifies that stopTaggerServer does not block
// indefinitely when GracefulStop would stall (simulated by holding an open
// stream). It must fall back to Stop() and return within a reasonable bound
// (well under 2× the grace timeout).
func TestStopTaggerServer_ForcedStop(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	// blockingWrapper never finishes TaggerStreamEntities, simulating a
	// long-lived subscriber that would prevent GracefulStop from returning.
	blockingWrapper := &blockingTaggerServer{done: make(chan struct{})}
	srv := grpc.NewServer()
	pb.RegisterAgentSecureServer(srv, blockingWrapper)
	go func() { _ = srv.Serve(lis) }()

	// Dial and open a streaming RPC so GracefulStop will wait for it.
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	client := pb.NewAgentSecureClient(conn)
	stream, err := client.TaggerStreamEntities(ctx, &pb.StreamTagsRequest{})
	require.NoError(t, err)
	// Consume the stream in the background so the RPC is live.
	go func() { _, _ = stream.Recv() }()

	// Give the server a moment to register the in-flight stream.
	time.Sleep(50 * time.Millisecond)

	// Use a very short grace timeout so the test runs fast.
	origTimeout := taggerServerGracefulStopTimeout
	taggerServerGracefulStopTimeout = 200 * time.Millisecond
	t.Cleanup(func() { taggerServerGracefulStopTimeout = origTimeout })

	ext := &dogtelExtension{
		log:            logmock.New(t),
		coreConfig:     configmock.NewMockWithOverrides(t, nil),
		serializer:     serializermock.NewMetricSerializer(t),
		ipc:            ipcmock.New(t),
		taggerServer:   srv,
		taggerListener: lis,
	}

	// stopTaggerServer must return within grace timeout + a small margin.
	deadline := time.Now().Add(taggerServerGracefulStopTimeout + 500*time.Millisecond)
	ext.stopTaggerServer()

	assert.True(t, time.Now().Before(deadline),
		"stopTaggerServer blocked past the expected deadline")
	assert.Nil(t, ext.taggerServer)
	assert.Nil(t, ext.taggerListener)
}

// blockingTaggerServer is a pb.AgentSecureServer whose TaggerStreamEntities
// blocks until either the done channel is closed or the stream context is
// cancelled (which happens when the gRPC server calls Stop()).
type blockingTaggerServer struct {
	pb.UnimplementedAgentSecureServer
	done chan struct{}
}

func (b *blockingTaggerServer) TaggerStreamEntities(_ *pb.StreamTagsRequest, stream pb.AgentSecure_TaggerStreamEntitiesServer) error {
	select {
	case <-b.done:
	case <-stream.Context().Done():
	}
	return nil
}
