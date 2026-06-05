// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package configstreamconsumerimpl

import (
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	configstreamconsumer "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// mockConfigStreamServer is a mock gRPC server for testing
type mockConfigStreamServer struct {
	pb.UnimplementedAgentSecureServer
	events chan *pb.ConfigEvent
	closed bool
}

func (m *mockConfigStreamServer) StreamConfigEvents(_ *pb.ConfigStreamRequest, stream pb.AgentSecure_StreamConfigEventsServer) error {
	// Extract session_id from gRPC metadata
	md, ok := metadata.FromIncomingContext(stream.Context())
	if !ok {
		return status.Error(codes.Unauthenticated, "missing gRPC metadata")
	}

	sessionIDs := md.Get("session_id")
	if len(sessionIDs) == 0 || sessionIDs[0] == "" {
		return status.Error(codes.Unauthenticated, "session_id required in metadata")
	}

	for event := range m.events {
		if m.closed {
			return io.EOF
		}
		if err := stream.Send(event); err != nil {
			return err
		}
	}
	return nil
}

// setupTestServer creates a test gRPC server and returns the server, address, and event channel
func setupTestServer(t *testing.T, ipcComp *ipcmock.IPCMock) (*grpc.Server, string, chan *pb.ConfigEvent, func()) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	opts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(ipcComp.GetTLSServerConfig())),
	}
	grpcServer := grpc.NewServer(opts...)
	mockServer := &mockConfigStreamServer{
		events: make(chan *pb.ConfigEvent, 100),
	}
	pb.RegisterAgentSecureServer(grpcServer, mockServer)

	go func() {
		_ = grpcServer.Serve(listener)
	}()

	cleanup := func() {
		mockServer.closed = true
		close(mockServer.events)
		grpcServer.Stop()
		listener.Close()
	}

	return grpcServer, listener.Addr().String(), mockServer.events, cleanup
}

// createTestConsumer creates a consumer for testing
func createTestConsumer(t *testing.T, serverAddr string, ipcComp *ipcmock.IPCMock) (*consumer, func()) {
	log := logmock.New(t)
	telemetryComp := telemetryimpl.NewMock(t)

	c := &consumer{
		log:       log,
		ipc:       ipcComp,
		telemetry: telemetryComp,
		params: configstreamconsumer.Params{
			ClientName:       "test-client",
			CoreAgentAddress: serverAddr,
			SessionID:        "test-session-123",
		},
		effectiveConfig: make(map[string]interface{}),
		readyCh:         make(chan struct{}),
		startTime:       time.Now(),
	}
	c.initMetrics()

	cleanup := func() {
		if c.cancel != nil {
			c.cancel()
		}
		if c.stream != nil {
			_ = c.stream.CloseSend()
		}
		if c.conn != nil {
			_ = c.conn.Close()
		}
	}

	return c, cleanup
}

func TestConsumerSnapshot(t *testing.T) {
	ipcComp := ipcmock.New(t)
	_, serverAddr, events, cleanup := setupTestServer(t, ipcComp)
	defer cleanup()

	consumer, cleanupConsumer := createTestConsumer(t, serverAddr, ipcComp)
	defer cleanupConsumer()

	// Send a snapshot
	settings := []*pb.ConfigSetting{
		{Key: "test_string", Value: mustNewValue(t, "hello")},
		{Key: "test_int", Value: mustNewValue(t, int64(42))},
		{Key: "test_bool", Value: mustNewValue(t, true)},
	}

	events <- &pb.ConfigEvent{
		Event: &pb.ConfigEvent_Snapshot{
			Snapshot: &pb.ConfigSnapshot{
				SequenceId: 1,
				Settings:   settings,
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start streaming in a goroutine
	consumer.ctx, consumer.cancel = context.WithCancel(context.Background())
	go func() {
		_ = consumer.connectAndStream()
	}()

	err := consumer.waitReady(ctx)
	require.NoError(t, err)

	// Verify config was applied to the effective config map
	consumer.configLock.RLock()
	defer consumer.configLock.RUnlock()
	assert.Equal(t, "hello", consumer.effectiveConfig["test_string"])
	assert.Equal(t, int64(42), consumer.effectiveConfig["test_int"])
	assert.Equal(t, true, consumer.effectiveConfig["test_bool"])
	assert.Equal(t, int32(1), consumer.lastSeqID)
}

func TestConsumerUpdates(t *testing.T) {
	ipcComp := ipcmock.New(t)
	_, serverAddr, events, cleanup := setupTestServer(t, ipcComp)
	defer cleanup()

	consumer, cleanupConsumer := createTestConsumer(t, serverAddr, ipcComp)
	defer cleanupConsumer()

	// Send initial snapshot
	events <- &pb.ConfigEvent{
		Event: &pb.ConfigEvent_Snapshot{
			Snapshot: &pb.ConfigSnapshot{
				SequenceId: 1,
				Settings: []*pb.ConfigSetting{
					{Key: "test_key", Value: mustNewValue(t, "initial")},
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start streaming
	consumer.ctx, consumer.cancel = context.WithCancel(context.Background())
	go func() {
		_ = consumer.connectAndStream()
	}()

	err := consumer.waitReady(ctx)
	require.NoError(t, err)

	// Send an update
	events <- &pb.ConfigEvent{
		Event: &pb.ConfigEvent_Update{
			Update: &pb.ConfigUpdate{
				SequenceId: 2,
				Setting: &pb.ConfigSetting{
					Key:   "test_key",
					Value: mustNewValue(t, "updated"),
				},
			},
		},
	}

	// Wait a bit for the update to be processed
	time.Sleep(100 * time.Millisecond)

	consumer.configLock.RLock()
	defer consumer.configLock.RUnlock()
	assert.Equal(t, "updated", consumer.effectiveConfig["test_key"])
	assert.Equal(t, int32(2), consumer.lastSeqID)
}

func TestConsumerStaleUpdates(t *testing.T) {
	ipcComp := ipcmock.New(t)
	_, serverAddr, events, cleanup := setupTestServer(t, ipcComp)
	defer cleanup()

	consumer, cleanupConsumer := createTestConsumer(t, serverAddr, ipcComp)
	defer cleanupConsumer()

	// Send initial snapshot
	events <- &pb.ConfigEvent{
		Event: &pb.ConfigEvent_Snapshot{
			Snapshot: &pb.ConfigSnapshot{
				SequenceId: 5,
				Settings: []*pb.ConfigSetting{
					{Key: "test_key", Value: mustNewValue(t, "current")},
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start streaming
	consumer.ctx, consumer.cancel = context.WithCancel(context.Background())
	go func() {
		_ = consumer.connectAndStream()
	}()

	err := consumer.waitReady(ctx)
	require.NoError(t, err)

	// Send a stale update (seq_id <= current)
	events <- &pb.ConfigEvent{
		Event: &pb.ConfigEvent_Update{
			Update: &pb.ConfigUpdate{
				SequenceId: 3,
				Setting: &pb.ConfigSetting{
					Key:   "test_key",
					Value: mustNewValue(t, "stale"),
				},
			},
		},
	}

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Verify stale update was NOT applied
	consumer.configLock.RLock()
	defer consumer.configLock.RUnlock()
	assert.Equal(t, "current", consumer.effectiveConfig["test_key"])
	assert.Equal(t, int32(5), consumer.lastSeqID)
}

// mustNewValue creates a structpb.Value or fails the test
func mustNewValue(t *testing.T, v interface{}) *structpb.Value {
	val, err := structpb.NewValue(v)
	require.NoError(t, err, fmt.Sprintf("failed to create Value from %v", v))
	return val
}

func TestConsumerAppliesUpdatesInOrder(t *testing.T) {
	t.Run("Consumer can start and block until snapshot", func(t *testing.T) {
		ipcComp := ipcmock.New(t)
		_, serverAddr, events, cleanup := setupTestServer(t, ipcComp)
		defer cleanup()

		consumer, cleanupConsumer := createTestConsumer(t, serverAddr, ipcComp)
		defer cleanupConsumer()

		// Start streaming in background
		consumer.ctx, consumer.cancel = context.WithCancel(context.Background())
		go func() {
			_ = consumer.connectAndStream()
		}()

		// WaitReady should block
		readyCtx, readyCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		err := consumer.waitReady(readyCtx)
		readyCancel()
		assert.Error(t, err, "should timeout before snapshot")

		// Send snapshot
		events <- &pb.ConfigEvent{
			Event: &pb.ConfigEvent_Snapshot{
				Snapshot: &pb.ConfigSnapshot{
					SequenceId: 1,
					Settings: []*pb.ConfigSetting{
						{Key: "ready", Value: mustNewValue(t, true)},
					},
				},
			},
		}

		// Now WaitReady should succeed
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err = consumer.waitReady(ctx)
		assert.NoError(t, err, "should unblock after snapshot")
	})

	t.Run("Consumer applies updates in order", func(t *testing.T) {
		ipcComp := ipcmock.New(t)
		_, serverAddr, events, cleanup := setupTestServer(t, ipcComp)
		defer cleanup()

		consumer, cleanupConsumer := createTestConsumer(t, serverAddr, ipcComp)
		defer cleanupConsumer()

		// Start streaming
		consumer.ctx, consumer.cancel = context.WithCancel(context.Background())
		go func() {
			_ = consumer.connectAndStream()
		}()

		// Send snapshot and ordered updates
		events <- &pb.ConfigEvent{
			Event: &pb.ConfigEvent_Snapshot{
				Snapshot: &pb.ConfigSnapshot{
					SequenceId: 1,
					Settings:   []*pb.ConfigSetting{},
				},
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := consumer.waitReady(ctx)
		require.NoError(t, err)

		// Send ordered updates
		for i := 2; i <= 5; i++ {
			events <- &pb.ConfigEvent{
				Event: &pb.ConfigEvent_Update{
					Update: &pb.ConfigUpdate{
						SequenceId: int32(i),
						Setting: &pb.ConfigSetting{
							Key:   "counter",
							Value: mustNewValue(t, int64(i)),
						},
					},
				},
			}
			time.Sleep(50 * time.Millisecond)
		}

		// Verify final state
		consumer.configLock.RLock()
		defer consumer.configLock.RUnlock()
		assert.Equal(t, int64(5), consumer.effectiveConfig["counter"])
		assert.Equal(t, int32(5), consumer.lastSeqID)
	})
}

// TestStartBlocksUntilSnapshot verifies that start blocks until the first snapshot is received,
// so the binary's run function sees a fully-populated config without calling WaitReady.
func TestStartBlocksUntilSnapshot(t *testing.T) {
	ipcComp := ipcmock.New(t)
	_, serverAddr, events, cleanup := setupTestServer(t, ipcComp)
	defer cleanup()

	consumer, cleanupConsumer := createTestConsumer(t, serverAddr, ipcComp)
	defer cleanupConsumer()

	// Send snapshot after a short delay to verify Start blocks.
	go func() {
		time.Sleep(50 * time.Millisecond)
		events <- &pb.ConfigEvent{
			Event: &pb.ConfigEvent_Snapshot{
				Snapshot: &pb.ConfigSnapshot{
					SequenceId: 1,
					Settings: []*pb.ConfigSetting{
						{Key: "server.port", Value: mustNewValue(t, int64(8080))},
						{Key: "feature.enabled", Value: mustNewValue(t, true)},
					},
				},
			},
		}
	}()

	startTime := time.Now()
	err := consumer.start(context.Background())
	startDuration := time.Since(startTime)

	require.NoError(t, err, "Start should succeed once snapshot is received")
	assert.GreaterOrEqual(t, startDuration, 50*time.Millisecond, "Start should have blocked until snapshot arrived")

	// Config is guaranteed to be fully populated when Start returns.
	consumer.configLock.RLock()
	defer consumer.configLock.RUnlock()
	assert.Equal(t, int64(8080), consumer.effectiveConfig["server.port"])
	assert.Equal(t, true, consumer.effectiveConfig["feature.enabled"])

	consumer.stop(context.Background())
}

// TestStartTimeoutFailsStartup verifies that start returns an error when the first snapshot
// is not received within ReadyTimeout, aborting FX startup.
func TestStartTimeoutFailsStartup(t *testing.T) {
	ipcComp := ipcmock.New(t)
	_, serverAddr, _, cleanup := setupTestServer(t, ipcComp)
	defer cleanup()

	consumer, cleanupConsumer := createTestConsumer(t, serverAddr, ipcComp)
	defer cleanupConsumer()

	// Short timeout so the test doesn't take 60s.
	consumer.params.ReadyTimeout = 200 * time.Millisecond

	startTime := time.Now()
	err := consumer.start(context.Background())
	startDuration := time.Since(startTime)

	require.Error(t, err, "Start should fail when no snapshot received within timeout")
	assert.Contains(t, err.Error(), "waiting for initial config snapshot")
	assert.GreaterOrEqual(t, startDuration, 200*time.Millisecond, "should respect ReadyTimeout")
	assert.Less(t, startDuration, 500*time.Millisecond, "should not wait longer than needed")
}
