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
	"sync"
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

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
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
		params: Params{
			ClientName:       "test-client",
			CoreAgentAddress: serverAddr,
			SessionID:        "test-session-123",
		},
		effectiveConfig: make(map[string]interface{}),
		readyCh:         make(chan struct{}),
	}

	c.reader = &configReader{consumer: c}

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
	consumer.initMetrics()
	go func() {
		startTime := time.Now()
		firstSnapshot := true
		_ = consumer.connectAndStream(startTime, &firstSnapshot)
	}()

	err := consumer.WaitReady(ctx)
	require.NoError(t, err)

	// Verify config was applied
	assert.Equal(t, "hello", consumer.Reader().GetString("test_string"))
	assert.Equal(t, 42, consumer.Reader().GetInt("test_int"))
	assert.True(t, consumer.Reader().GetBool("test_bool"))
	assert.Equal(t, uint64(1), consumer.Reader().GetSequenceID())
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
	consumer.initMetrics()
	go func() {
		startTime := time.Now()
		firstSnapshot := true
		_ = consumer.connectAndStream(startTime, &firstSnapshot)
	}()

	err := consumer.WaitReady(ctx)
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

	assert.Equal(t, "updated", consumer.Reader().GetString("test_key"))
	assert.Equal(t, uint64(2), consumer.Reader().GetSequenceID())
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
	consumer.initMetrics()
	go func() {
		startTime := time.Now()
		firstSnapshot := true
		_ = consumer.connectAndStream(startTime, &firstSnapshot)
	}()

	err := consumer.WaitReady(ctx)
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
	assert.Equal(t, "current", consumer.Reader().GetString("test_key"))
	assert.Equal(t, uint64(5), consumer.Reader().GetSequenceID())
}

func TestConsumerChangeSubscription(t *testing.T) {
	ipcComp := ipcmock.New(t)
	_, serverAddr, events, cleanup := setupTestServer(t, ipcComp)
	defer cleanup()

	consumer, cleanupConsumer := createTestConsumer(t, serverAddr, ipcComp)
	defer cleanupConsumer()

	// Subscribe to changes
	changeCh, unsubscribe := consumer.Subscribe()
	defer unsubscribe()

	// Send initial snapshot
	events <- &pb.ConfigEvent{
		Event: &pb.ConfigEvent_Snapshot{
			Snapshot: &pb.ConfigSnapshot{
				SequenceId: 1,
				Settings: []*pb.ConfigSetting{
					{Key: "key1", Value: mustNewValue(t, "value1")},
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start streaming
	consumer.ctx, consumer.cancel = context.WithCancel(context.Background())
	consumer.initMetrics()
	go func() {
		startTime := time.Now()
		firstSnapshot := true
		_ = consumer.connectAndStream(startTime, &firstSnapshot)
	}()

	err := consumer.WaitReady(ctx)
	require.NoError(t, err)

	// Collect initial snapshot changes
	changeCount := 0
	timeout := time.After(500 * time.Millisecond)
drainInitial:
	for {
		select {
		case <-changeCh:
			changeCount++
		case <-timeout:
			break drainInitial
		}
	}

	// Should have received at least one change for the initial snapshot
	assert.Greater(t, changeCount, 0)

	// Send an update
	events <- &pb.ConfigEvent{
		Event: &pb.ConfigEvent_Update{
			Update: &pb.ConfigUpdate{
				SequenceId: 2,
				Setting: &pb.ConfigSetting{
					Key:   "key1",
					Value: mustNewValue(t, "value2"),
				},
			},
		},
	}

	// Wait for the change event
	select {
	case change := <-changeCh:
		assert.Equal(t, "key1", change.Key)
		assert.Equal(t, "value1", change.OldValue)
		assert.Equal(t, "value2", change.NewValue)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for change event")
	}
}

func TestConsumerReader(t *testing.T) {
	log := logmock.New(t)
	ipcComp := ipcmock.New(t)
	telemetryComp := telemetryimpl.NewMock(t)

	consumer := &consumer{
		log:       log,
		ipc:       ipcComp,
		telemetry: telemetryComp,
		params: Params{
			ClientName:       "test",
			CoreAgentAddress: "localhost:1234",
			SessionID:        "test-session",
		},
		effectiveConfig: map[string]interface{}{
			"string_key": "hello",
			"int_key":    int64(42),
			"bool_key":   true,
			"float_key":  3.14,
			"slice_key":  []interface{}{"a", "b", "c"},
			"map_key":    map[string]interface{}{"nested": "value"},
		},
		lastSeqID: 10,
	}

	reader := &configReader{consumer: consumer}

	// Test various reader methods
	assert.Equal(t, "hello", reader.GetString("string_key"))
	assert.Equal(t, 42, reader.GetInt("int_key"))
	assert.Equal(t, int32(42), reader.GetInt32("int_key"))
	assert.Equal(t, int64(42), reader.GetInt64("int_key"))
	assert.True(t, reader.GetBool("bool_key"))
	assert.Equal(t, 3.14, reader.GetFloat64("float_key"))
	assert.Equal(t, []string{"a", "b", "c"}, reader.GetStringSlice("slice_key"))
	assert.Equal(t, uint64(10), reader.GetSequenceID())

	// Test map access
	stringMap := reader.GetStringMap("map_key")
	require.NotNil(t, stringMap)
	assert.Equal(t, "value", stringMap["nested"])

	// Test AllSettings
	allSettings := reader.AllSettings()
	assert.Equal(t, 6, len(allSettings))

	// Test IsSet
	assert.True(t, reader.IsSet("string_key"))
	assert.False(t, reader.IsSet("nonexistent_key"))
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
		consumer.initMetrics()
		go func() {
			startTime := time.Now()
			firstSnapshot := true
			_ = consumer.connectAndStream(startTime, &firstSnapshot)
		}()

		// WaitReady should block
		readyCtx, readyCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		err := consumer.WaitReady(readyCtx)
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
		err = consumer.WaitReady(ctx)
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
		consumer.initMetrics()
		go func() {
			startTime := time.Now()
			firstSnapshot := true
			_ = consumer.connectAndStream(startTime, &firstSnapshot)
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
		err := consumer.WaitReady(ctx)
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
		assert.Equal(t, 5, consumer.Reader().GetInt("counter"))
		assert.Equal(t, uint64(5), consumer.Reader().GetSequenceID())
	})
}

// TestUsagePatternBlocking tests blocking until the config ready
func TestUsagePatternBlocking(t *testing.T) {
	ipcComp := ipcmock.New(t)
	_, serverAddr, events, cleanup := setupTestServer(t, ipcComp)
	defer cleanup()

	consumer, cleanupConsumer := createTestConsumer(t, serverAddr, ipcComp)
	defer cleanupConsumer()

	// Simulate: agent needs config values before it can start
	var agentStarted bool
	var configuredPort int

	// Start the config stream (non-blocking - runs in background)
	err := consumer.Start(context.Background())
	require.NoError(t, err)

	// Agent is NOT started yet - we need config first
	assert.False(t, agentStarted, "agent should not start before config ready")

	// Send snapshot asynchronously (simulates server response)
	go func() {
		time.Sleep(100 * time.Millisecond) // Simulate network delay
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

	// Block until config is ready
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	startTime := time.Now()
	err = consumer.WaitReady(ctx)
	waitDuration := time.Since(startTime)

	require.NoError(t, err, "config should be ready within timeout")
	assert.GreaterOrEqual(t, waitDuration, 100*time.Millisecond, "should have waited for snapshot")
	assert.Less(t, waitDuration, 1*time.Second, "should not timeout")

	// Config is GUARANTEED to be fully populated here
	cfg := consumer.Reader()
	configuredPort = cfg.GetInt("server.port")
	featureEnabled := cfg.GetBool("feature.enabled")

	assert.Equal(t, 8080, configuredPort, "config should be populated")
	assert.True(t, featureEnabled, "config should be populated")

	// Now it's safe to start the agent with config
	agentStarted = true
	assert.True(t, agentStarted, "agent can now start with complete config")

	consumer.stop()
}

// TestUsagePatternNonBlocking tests whether the consumer can start immediately with eventual consistency
func TestUsagePatternNonBlocking(t *testing.T) {
	ipcComp := ipcmock.New(t)
	_, serverAddr, events, cleanup := setupTestServer(t, ipcComp)
	defer cleanup()

	consumer, cleanupConsumer := createTestConsumer(t, serverAddr, ipcComp)
	defer cleanupConsumer()

	// Start the config stream (non-blocking - runs in background)
	err := consumer.Start(context.Background())
	require.NoError(t, err)

	// DON'T call WaitReady() - proceed immediately

	// Agent starts IMMEDIATELY without waiting
	cfg := consumer.Reader()
	agentStarted := true
	assert.True(t, agentStarted, "agent starts immediately")

	// Config may be empty initially (returns zero values)
	initialPort := cfg.GetInt("server.port")
	assert.Equal(t, 0, initialPort, "config not yet populated - zero value returned")

	// Subscribe to config changes to react when config arrives
	changes, unsubscribe := consumer.Subscribe()
	defer unsubscribe()

	// Track when config updates arrive (mutex protects map from concurrent read/write)
	var configUpdatesMu sync.Mutex
	configUpdates := make(map[string]interface{})
	changeReceived := make(chan bool, 1)

	go func() {
		for change := range changes {
			configUpdatesMu.Lock()
			configUpdates[change.Key] = change.NewValue
			configUpdatesMu.Unlock()
			if change.Key == "server.port" {
				changeReceived <- true
			}
		}
	}()

	// Now send snapshot (simulates server sending config)
	events <- &pb.ConfigEvent{
		Event: &pb.ConfigEvent_Snapshot{
			Snapshot: &pb.ConfigSnapshot{
				SequenceId: 1,
				Settings: []*pb.ConfigSetting{
					{Key: "server.port", Value: mustNewValue(t, int64(9090))},
					{Key: "feature.experimental", Value: mustNewValue(t, true)},
				},
			},
		},
	}

	// Wait for change notification
	select {
	case <-changeReceived:
		// Config has arrived
	case <-time.After(2 * time.Second):
		t.Fatal("config snapshot not received")
	}

	// Config is now eventually consistent
	updatedPort := cfg.GetInt("server.port")
	assert.Equal(t, 9090, updatedPort, "config should be updated")

	// Verify change events were received (lock to avoid race with subscriber goroutine)
	configUpdatesMu.Lock()
	portVal, hasPort := configUpdates["server.port"]
	configUpdatesMu.Unlock()
	assert.True(t, hasPort, "configUpdates should contain server.port")
	assert.Equal(t, int64(9090), portVal)

	// Agent can dynamically reconfigure based on updates
	t.Logf("Agent dynamically reconfigured: port changed from %d to %d", initialPort, updatedPort)

	consumer.stop()
}

// TestUsagePatternBlockingTimeout tests that blocking pattern fails fast if server unavailable
func TestUsagePatternBlockingTimeout(t *testing.T) {
	ipcComp := ipcmock.New(t)
	_, serverAddr, _, cleanup := setupTestServer(t, ipcComp)
	defer cleanup()

	consumer, cleanupConsumer := createTestConsumer(t, serverAddr, ipcComp)
	defer cleanupConsumer()

	// Start the config stream
	err := consumer.Start(context.Background())
	require.NoError(t, err)

	// DON'T send snapshot - simulate server not responding

	// Try to wait with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	startTime := time.Now()
	err = consumer.WaitReady(ctx)
	waitDuration := time.Since(startTime)

	// Should fail with timeout error
	require.Error(t, err, "should timeout when no snapshot received")
	assert.Contains(t, err.Error(), "timed out waiting for config snapshot")
	assert.GreaterOrEqual(t, waitDuration, 200*time.Millisecond, "should respect timeout")
	assert.Less(t, waitDuration, 500*time.Millisecond, "should not wait too long")

	// Agent can decide how to handle: retry, use defaults, or fail
	t.Log("Agent can handle timeout gracefully: retry, use defaults, or exit")

	consumer.stop()
}

// TestUsagePatternMultipleWaiters tests that multiple goroutines can wait on the same readiness
func TestUsagePatternMultipleWaiters(t *testing.T) {
	ipcComp := ipcmock.New(t)
	_, serverAddr, events, cleanup := setupTestServer(t, ipcComp)
	defer cleanup()

	consumer, cleanupConsumer := createTestConsumer(t, serverAddr, ipcComp)
	defer cleanupConsumer()

	// Start the config stream
	err := consumer.Start(context.Background())
	require.NoError(t, err)

	// Simulate multiple components waiting for config
	const numWaiters = 5
	waitersReady := make(chan int, numWaiters)

	for i := 0; i < numWaiters; i++ {
		go func(id int) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			if err := consumer.WaitReady(ctx); err == nil {
				waitersReady <- id
			}
		}(i)
	}

	// Give waiters time to start
	time.Sleep(100 * time.Millisecond)

	// Send snapshot - should unblock ALL waiters simultaneously
	events <- &pb.ConfigEvent{
		Event: &pb.ConfigEvent_Snapshot{
			Snapshot: &pb.ConfigSnapshot{
				SequenceId: 1,
				Settings: []*pb.ConfigSetting{
					{Key: "shared.config", Value: mustNewValue(t, "available")},
				},
			},
		},
	}

	// All waiters should unblock
	for i := 0; i < numWaiters; i++ {
		select {
		case waiterID := <-waitersReady:
			t.Logf("Waiter %d unblocked", waiterID)
		case <-time.After(2 * time.Second):
			t.Fatalf("Waiter %d did not unblock", i)
		}
	}

	// All can read config
	cfg := consumer.Reader()
	assert.Equal(t, "available", cfg.GetString("shared.config"))

	consumer.stop()
}
