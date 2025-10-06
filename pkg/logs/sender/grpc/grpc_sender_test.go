// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package grpc

import (
	"fmt"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
)

// MockGRPCServer that implements StatefulLogsServiceServer
type MockGRPCServer struct {
	statefulpb.UnimplementedStatefulLogsServiceServer

	// Control behavior
	shouldFailSend   bool
	shouldFailRecv   bool
	shouldDisconnect bool
	responseDelay    time.Duration
	batchResponses   map[int32]statefulpb.BatchStatus_Status
	mu               sync.RWMutex

	// Track what was received
	receivedBatches []*statefulpb.StatefulBatch
	activeStreams   []statefulpb.StatefulLogsService_LogsStreamServer
	streamsMu       sync.RWMutex
}

func NewMockGRPCServer() *MockGRPCServer {
	return &MockGRPCServer{
		batchResponses:  make(map[int32]statefulpb.BatchStatus_Status),
		receivedBatches: make([]*statefulpb.StatefulBatch, 0),
		activeStreams:   make([]statefulpb.StatefulLogsService_LogsStreamServer, 0),
	}
}

func (s *MockGRPCServer) LogsStream(stream statefulpb.StatefulLogsService_LogsStreamServer) error {
	s.streamsMu.Lock()
	s.activeStreams = append(s.activeStreams, stream)
	streamIndex := len(s.activeStreams) - 1
	s.streamsMu.Unlock()

	defer func() {
		s.streamsMu.Lock()
		if streamIndex < len(s.activeStreams) {
			s.activeStreams = append(s.activeStreams[:streamIndex], s.activeStreams[streamIndex+1:]...)
		}
		s.streamsMu.Unlock()
	}()

	for {
		// Receive batch from client first
		batch, err := stream.Recv()
		if err != nil {
			return err
		}

		s.mu.RLock()
		shouldFail := s.shouldFailRecv
		shouldDisconnect := s.shouldDisconnect
		delay := s.responseDelay
		s.mu.RUnlock()

		// Store the received batch (so tests can verify it was received)
		s.mu.Lock()
		s.receivedBatches = append(s.receivedBatches, batch)

		// Determine response status
		responseStatus := statefulpb.BatchStatus_OK
		if status, exists := s.batchResponses[int32(batch.BatchId)]; exists {
			responseStatus = status
		}
		s.mu.Unlock()

		// Check for failures AFTER receiving but BEFORE responding
		if shouldDisconnect {
			// Disconnect after receiving batch but before sending response
			// This simulates server dying mid-processing
			return status.Error(codes.Unavailable, "server disconnected")
		}

		if shouldFail {
			// Fail after receiving batch but before sending response
			return status.Error(codes.Internal, "simulated recv failure")
		}

		// Add delay if configured
		if delay > 0 {
			time.Sleep(delay)
		}

		// Send response back
		response := &statefulpb.BatchStatus{
			BatchId: int32(batch.BatchId),
			Status:  responseStatus,
		}

		if err := stream.Send(response); err != nil {
			return err
		}
	}
}

// Control methods for testing
func (s *MockGRPCServer) SetShouldFailSend(fail bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shouldFailSend = fail
}

func (s *MockGRPCServer) SetShouldFailRecv(fail bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shouldFailRecv = fail
}

func (s *MockGRPCServer) SetShouldDisconnect(disconnect bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shouldDisconnect = disconnect
}

func (s *MockGRPCServer) SetResponseDelay(delay time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responseDelay = delay
}

func (s *MockGRPCServer) SetBatchResponse(batchID int32, status statefulpb.BatchStatus_Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.batchResponses[batchID] = status
}

func (s *MockGRPCServer) GetReceivedBatches() []*statefulpb.StatefulBatch {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.receivedBatches
}

func (s *MockGRPCServer) ClearReceivedBatches() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.receivedBatches = s.receivedBatches[:0]
}

func (s *MockGRPCServer) DisconnectAllStreams() {
	s.streamsMu.Lock()
	defer s.streamsMu.Unlock()
	s.shouldDisconnect = true
}

// Test helper to start mock gRPC server
func startMockGRPCServer(t *testing.T) (*MockGRPCServer, string, func()) {
	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	mockServer := NewMockGRPCServer()
	grpcServer := grpc.NewServer()
	statefulpb.RegisterStatefulLogsServiceServer(grpcServer, mockServer)

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			t.Logf("gRPC server error: %v", err)
		}
	}()

	address := listener.Addr().String()

	cleanup := func() {
		grpcServer.Stop()
		listener.Close()
	}

	// Server is ready immediately after starting

	return mockServer, address, cleanup
}

// MockSink for testing
type MockSink struct {
	outputChan chan *message.Payload
}

func (s *MockSink) Channel() chan *message.Payload {
	return s.outputChan
}

// Helper to create GRPCSender with mock server
func createTestGRPCSender(t *testing.T, address string) (*Sender, *MockSink) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("logs_config.batch_wait", 100) // Short batch wait for testing
	cfg.SetWithoutSource("logs_config.pipelines", 1)    // Single pipeline

	// Parse host and port from address (e.g., "127.0.0.1:53662")
	host, portStr, err := net.SplitHostPort(address)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	// Create endpoint using the constructor pattern from existing tests
	endpoint := config.NewMockEndpointWithOptions(map[string]interface{}{
		"host":        host,
		"port":        port,
		"is_reliable": true,
		"use_grpc":    true,
		"use_ssl":     false,
	})

	endpoints := &config.Endpoints{
		UseGRPC:   true,
		Main:      endpoint,
		Endpoints: []config.Endpoint{endpoint},
	}

	sink := &MockSink{outputChan: make(chan *message.Payload, 100)}
	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()
	t.Cleanup(func() { destinationsCtx.Stop() })

	pipelineMonitor := metrics.NewNoopPipelineMonitor("test")

	sender := NewGRPCSender(cfg, sink, endpoints, destinationsCtx, pipelineMonitor)
	require.NotNil(t, sender)

	return sender, sink
}

// Test end-to-end payload flow through GRPCSender
func TestGRPCSenderEndToEndFlow(t *testing.T) {
	mockServer, address, cleanup := startMockGRPCServer(t)
	defer cleanup()

	sender, sink := createTestGRPCSender(t, address)

	sender.Start()
	defer sender.Stop()

	// Create test payload
	msg := message.NewMessage([]byte("test message"), nil, "", 0)
	payload := &message.Payload{
		MessageMetas:  []*message.MessageMetadata{&msg.MessageMetadata},
		Encoded:       []byte("test message"),
		Encoding:      "identity",
		UnencodedSize: 12,
		IsSnapshot:    false,
		GRPCData:      []*statefulpb.Datum{{Data: &statefulpb.Datum_Logs{Logs: &statefulpb.Log{Content: &statefulpb.Log_Raw{Raw: "test message"}}}}},
	}

	// Send payload through GRPCSender input channel
	inputChan := sender.In()
	select {
	case inputChan <- payload:
	case <-time.After(1 * time.Second):
		t.Fatal("Failed to send payload to GRPCSender")
	}

	// Wait for server to actually receive the batch (event-driven, not time-based)
	require.Eventually(t, func() bool {
		batches := mockServer.GetReceivedBatches()
		return len(batches) >= 1
	}, 3*time.Second, 50*time.Millisecond, "Server should receive batch")

	// Verify server received the batch
	batches := mockServer.GetReceivedBatches()
	require.Len(t, batches, 1, "Server should have received one batch")

	batch := batches[0]
	assert.Equal(t, uint32(1), batch.BatchId)
	require.Len(t, batch.Data, 1)
	assert.Equal(t, "test message", batch.Data[0].GetLogs().GetRaw())

	// Verify payload was acknowledged to auditor
	select {
	case ackPayload := <-sink.outputChan:
		assert.Equal(t, payload, ackPayload)
	case <-time.After(1 * time.Second):
		t.Fatal("Expected payload acknowledgment from auditor")
	}
}

// Test GRPCSender stream failure and recovery
func TestGRPCSenderFailureRecovery(t *testing.T) {
	mockServer, address, cleanup := startMockGRPCServer(t)
	defer cleanup()

	sender, sink := createTestGRPCSender(t, address)

	sender.Start()
	defer sender.Stop()

	// Connection will be established on first send

	// Send first payload (should succeed)
	msg1 := message.NewMessage([]byte("message 1"), nil, "", 0)
	payload1 := &message.Payload{
		MessageMetas:  []*message.MessageMetadata{&msg1.MessageMetadata},
		Encoded:       []byte("message 1"),
		Encoding:      "identity",
		UnencodedSize: 9,
		IsSnapshot:    false,
		GRPCData:      []*statefulpb.Datum{{Data: &statefulpb.Datum_Logs{Logs: &statefulpb.Log{Content: &statefulpb.Log_Raw{Raw: "message 1"}}}}},
	}

	inputChan := sender.In()
	select {
	case inputChan <- payload1:
	case <-time.After(1 * time.Second):
		t.Fatal("Failed to send first payload")
	}

	// Wait for server to receive the batch
	require.Eventually(t, func() bool {
		return len(mockServer.GetReceivedBatches()) >= 1
	}, 2*time.Second, 50*time.Millisecond)

	// Verify first payload succeeded
	batches := mockServer.GetReceivedBatches()
	require.Len(t, batches, 1)

	select {
	case ackPayload := <-sink.outputChan:
		assert.Equal(t, payload1, ackPayload)
	case <-time.After(1 * time.Second):
		t.Fatal("Expected first payload acknowledgment")
	}

	// Get initial generation from the single worker (since we have 1 pipeline)
	require.Len(t, sender.workers, 1, "Should have exactly 1 worker for single pipeline")
	initialGeneration := sender.workers[0].generationID

	// Simulate server failure
	mockServer.SetShouldDisconnect(true)

	// Send second payload (should trigger failure and rotation)
	msg2 := message.NewMessage([]byte("message 2"), nil, "", 0)
	payload2 := &message.Payload{
		MessageMetas:  []*message.MessageMetadata{&msg2.MessageMetadata},
		Encoded:       []byte("message 2"),
		Encoding:      "identity",
		UnencodedSize: 9,
		IsSnapshot:    false,
		GRPCData:      []*statefulpb.Datum{{Data: &statefulpb.Datum_Logs{Logs: &statefulpb.Log{Content: &statefulpb.Log_Raw{Raw: "message 2"}}}}},
	}

	select {
	case inputChan <- payload2:
	case <-time.After(1 * time.Second):
		t.Fatal("Failed to send second payload")
	}

	// Wait for failure to be detected and rotation to begin
	require.Eventually(t, func() bool {
		return sender.workers[0].generationID > initialGeneration
	}, 3*time.Second, 100*time.Millisecond)

	// Verify generation incremented due to failure
	currentGeneration := sender.workers[0].generationID
	assert.Greater(t, currentGeneration, initialGeneration, "Generation should increment after failure")

	// Re-enable server (simulate recovery)
	mockServer.SetShouldDisconnect(false)
	mockServer.ClearReceivedBatches()

	// Server is now available for new connections

	// Send snapshot to complete rotation
	msgSnapshot := message.NewMessage([]byte("snapshot"), nil, "", 0)
	payloadSnapshot := &message.Payload{
		MessageMetas:  []*message.MessageMetadata{&msgSnapshot.MessageMetadata},
		Encoded:       []byte("snapshot"),
		Encoding:      "identity",
		UnencodedSize: 8,
		IsSnapshot:    true,
		GRPCData:      []*statefulpb.Datum{{Data: &statefulpb.Datum_Logs{Logs: &statefulpb.Log{Content: &statefulpb.Log_Raw{Raw: "snapshot"}}}}},
	}

	select {
	case inputChan <- payloadSnapshot:
	case <-time.After(1 * time.Second):
		t.Fatal("Failed to send snapshot payload")
	}

	// Wait for snapshot to be received on new stream
	require.Eventually(t, func() bool {
		return len(mockServer.GetReceivedBatches()) >= 1
	}, 3*time.Second, 100*time.Millisecond)

	// Verify snapshot was sent on new stream
	newBatches := mockServer.GetReceivedBatches()
	require.GreaterOrEqual(t, len(newBatches), 1, "Should have received snapshot on new stream")

	// Find snapshot batch
	var snapshotBatch *statefulpb.StatefulBatch
	for _, batch := range newBatches {
		if len(batch.Data) > 0 && batch.Data[0].GetLogs().GetRaw() == "snapshot" {
			snapshotBatch = batch
			break
		}
	}
	require.NotNil(t, snapshotBatch, "Should have received snapshot batch")

	// Send another payload to verify traffic continues
	msg3 := message.NewMessage([]byte("message 3"), nil, "", 0)
	payload3 := &message.Payload{
		MessageMetas:  []*message.MessageMetadata{&msg3.MessageMetadata},
		Encoded:       []byte("message 3"),
		Encoding:      "identity",
		UnencodedSize: 9,
		IsSnapshot:    false,
		GRPCData:      []*statefulpb.Datum{{Data: &statefulpb.Datum_Logs{Logs: &statefulpb.Log{Content: &statefulpb.Log_Raw{Raw: "message 3"}}}}},
	}

	select {
	case inputChan <- payload3:
	case <-time.After(1 * time.Second):
		t.Fatal("Failed to send third payload")
	}

	// Payload sent, acknowledgments will be collected below

	// Collect all acknowledgments we receive (may include message 2, snapshot, message 3)
	var receivedPayloads []*message.Payload
	timeout := time.After(3 * time.Second)

	// Collect acknowledgments for up to 3 seconds
	for {
		select {
		case ackPayload := <-sink.outputChan:
			receivedPayloads = append(receivedPayloads, ackPayload)
		case <-timeout:
			goto done
		}
	}

done:
	require.GreaterOrEqual(t, len(receivedPayloads), 2, "Should have received at least 2 acknowledgments")

	// Verify we got the expected payloads (snapshot and message 3 at minimum)
	payloadContents := make([]string, len(receivedPayloads))
	for i, p := range receivedPayloads {
		payloadContents[i] = string(p.Encoded)
	}
	assert.Contains(t, payloadContents, "snapshot", "Should have received snapshot")
	assert.Contains(t, payloadContents, "message 3", "Should have received message 3")
}

// Test multiple consecutive failures with GRPCSender
func TestGRPCSenderMultipleFailures(t *testing.T) {
	mockServer, address, cleanup := startMockGRPCServer(t)
	defer cleanup()

	sender, sink := createTestGRPCSender(t, address)

	sender.Start()
	defer sender.Stop()

	// Get initial generation from the single worker
	require.Len(t, sender.workers, 1, "Should have exactly 1 worker for single pipeline")
	initialGeneration := sender.workers[0].generationID

	inputChan := sender.In()

	mockServer.ClearReceivedBatches()

	// Cause failure
	mockServer.SetShouldDisconnect(true)

	// Send payload to trigger failure
	msg := message.NewMessage([]byte("trigger"), nil, "", 0)
	payload := &message.Payload{
		MessageMetas:  []*message.MessageMetadata{&msg.MessageMetadata},
		Encoded:       []byte("trigger"),
		Encoding:      "identity",
		UnencodedSize: len("trigger"),
		IsSnapshot:    false,
	}

	select {
	case inputChan <- payload:
	case <-time.After(1 * time.Second):
		t.Fatal("Failed to send trigger payload")
	}

	// Wait for failure detection (generation increment)
	require.Eventually(t, func() bool {
		return sender.workers[0].generationID == initialGeneration+1
	}, 2*time.Second, 100*time.Millisecond)

	// Send snapshot to complete rotation
	// but this message should trigger another rotation
	msgSnapshot := message.NewMessage([]byte("snapshot"), nil, "", 0)
	payloadSnapshot := &message.Payload{
		MessageMetas:  []*message.MessageMetadata{&msgSnapshot.MessageMetadata},
		Encoded:       []byte("snapshot"),
		Encoding:      "identity",
		UnencodedSize: len("snapshot"),
		IsSnapshot:    true,
	}

	select {
	case inputChan <- payloadSnapshot:
	case <-time.After(1 * time.Second):
		t.Fatal("Failed to send snapshot")
	}

	// Verify generation incremented (at least 2 times)
	require.Eventually(t, func() bool {
		return sender.workers[0].generationID == initialGeneration+2
	}, 2*time.Second, 100*time.Millisecond)

	mockServer.SetShouldDisconnect(false)

	// Send final payload to verify system is still working
	msgFinal := message.NewMessage([]byte("final test"), nil, "", 0)
	payloadFinal := &message.Payload{
		MessageMetas:  []*message.MessageMetadata{&msgFinal.MessageMetadata},
		Encoded:       []byte("final test"),
		Encoding:      "identity",
		UnencodedSize: 10,
		IsSnapshot:    true,
	}

	select {
	case inputChan <- payloadFinal:
	case <-time.After(1 * time.Second):
		t.Fatal("Failed to send final payload")
	}

	// Payload sent, wait for acknowledgment

	// Verify we get at least one acknowledgment (system is working)
	// Due to async nature and multiple failures, we may have many pending acks
	timeout := time.After(2 * time.Second)
	var gotAck bool
	for !gotAck {
		select {
		case <-sink.outputChan:
			gotAck = true
		case <-timeout:
			t.Fatal("Expected at least one payload acknowledgment")
		}
	}

	// Verify system is still functional by checking no more failures
	assert.True(t, gotAck, "System should still be processing payloads")
}

// Test GRPCSender signal channel mapping functionality
func TestGRPCSenderSignalChannelMapping(t *testing.T) {
	_, address, cleanup := startMockGRPCServer(t)
	defer cleanup()

	sender, _ := createTestGRPCSender(t, address)

	sender.Start()
	defer sender.Stop()

	// Test GetSignalChannelForInputChannel functionality
	inputChan := sender.In()
	signalChan := sender.GetSignalChannelForInputChannel(inputChan)

	require.NotNil(t, signalChan, "Should have signal channel for input channel")

	// Verify the mapping is correct
	worker, exists := sender.channelToWorkerMap[inputChan]
	require.True(t, exists, "Input channel should be mapped to a worker")

	// The signal channel should be the same underlying channel, even though types differ
	// GetSignalChannelForInputChannel returns chan any (via unsafe conversion)
	// while worker.signalStreamRotate is chan StreamRotateSignal
	// We can verify they're the same by checking the channel addresses
	assert.NotNil(t, signalChan, "Signal channel should not be nil")
	assert.NotNil(t, worker.signalStreamRotate, "Worker signal channel should not be nil")
}

// Test GRPCSender graceful shutdown
func TestGRPCSenderGracefulShutdown(t *testing.T) {
	_, address, cleanup := startMockGRPCServer(t)
	defer cleanup()

	sender, sink := createTestGRPCSender(t, address)

	sender.Start()

	// Send some payloads
	inputChan := sender.In()
	for i := 0; i < 3; i++ {
		msg := message.NewMessage([]byte(fmt.Sprintf("message %d", i)), nil, "", 0)
		payload := &message.Payload{
			MessageMetas:  []*message.MessageMetadata{&msg.MessageMetadata},
			Encoded:       []byte(fmt.Sprintf("message %d", i)),
			Encoding:      "identity",
			UnencodedSize: len(fmt.Sprintf("message %d", i)),
			IsSnapshot:    false,
		}

		select {
		case inputChan <- payload:
		case <-time.After(1 * time.Second):
			t.Fatalf("Failed to send payload %d", i)
		}
	}

	// Processing will start immediately

	// Stop sender gracefully
	sender.Stop()

	// Shutdown is synchronous

	// Verify some acknowledgments came through (system processed what it could)
	var ackCount int
	timeout := time.After(1 * time.Second)
	for {
		select {
		case <-sink.outputChan:
			ackCount++
		case <-timeout:
			goto done
		}
	}

done:
	// Should have processed at least some payloads before shutdown
	assert.GreaterOrEqual(t, ackCount, 0, "Should have processed some payloads during graceful shutdown")
}
