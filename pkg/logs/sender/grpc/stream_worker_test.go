// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package grpc

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
)

const (
	testTimeout      = 100 * time.Millisecond
	testTickInterval = 10 * time.Millisecond
	testShortWait    = 50 * time.Millisecond
)

// mockSink implements sender.Sink for testing
type mockSink struct {
	outputChan chan *message.Payload
}

func newMockSink() *mockSink {
	return &mockSink{
		outputChan: make(chan *message.Payload, 100),
	}
}

func (m *mockSink) Channel() chan *message.Payload {
	return m.outputChan
}

// mockLogsStream implements StatefulLogsService_LogsStreamClient for testing
type mockLogsStream struct {
	grpc.ClientStream

	mu sync.Mutex

	// Channels for communication
	sendCh chan *statefulpb.StatefulBatch // Batches sent by client
	recvCh chan *statefulpb.BatchStatus   // Acks to send to client
	errCh  chan error                     // To inject immediate errors in Recv()

	// Error control
	sendErr error // If set, next Send() will return this error
	recvErr error // If set, next Recv() will return this error

	// Track sent batches
	sentBatches []*statefulpb.StatefulBatch

	// Context
	ctx context.Context
}

func newMockLogsStream(ctx context.Context) *mockLogsStream {
	return &mockLogsStream{
		sendCh:      make(chan *statefulpb.StatefulBatch, 10),
		recvCh:      make(chan *statefulpb.BatchStatus, 10),
		errCh:       make(chan error, 1),
		sentBatches: make([]*statefulpb.StatefulBatch, 0),
		ctx:         ctx,
	}
}

func (m *mockLogsStream) Send(batch *statefulpb.StatefulBatch) error {
	m.mu.Lock()
	if m.sendErr != nil {
		err := m.sendErr
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()

	select {
	case m.sendCh <- batch:
		m.mu.Lock()
		m.sentBatches = append(m.sentBatches, batch)
		m.mu.Unlock()
		return nil
	case <-m.ctx.Done():
		return m.ctx.Err()
	}
}

func (m *mockLogsStream) Recv() (*statefulpb.BatchStatus, error) {
	m.mu.Lock()
	if m.recvErr != nil {
		err := m.recvErr
		m.mu.Unlock()
		return nil, err
	}
	m.mu.Unlock()

	select {
	case ack := <-m.recvCh:
		return ack, nil
	case err := <-m.errCh:
		return nil, err
	case <-m.ctx.Done():
		return nil, m.ctx.Err()
	}
}

func (m *mockLogsStream) CloseSend() error {
	return nil
}

// Helper to set send error
func (m *mockLogsStream) setSendError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendErr = err
}

// Helper to send an ack to the client
func (m *mockLogsStream) sendAck(batchID uint32) {
	m.recvCh <- &statefulpb.BatchStatus{
		BatchId: batchID,
	}
}

// Helper to inject an error immediately (unblocks Recv())
func (m *mockLogsStream) injectRecvError(err error) {
	m.errCh <- err
}

// Helper to get sent batch count
func (m *mockLogsStream) getSentBatchCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sentBatches)
}

// Helper to get a specific sent batch by index
func (m *mockLogsStream) getSentBatch(index int) *statefulpb.StatefulBatch {
	m.mu.Lock()
	defer m.mu.Unlock()
	if index < 0 || index >= len(m.sentBatches) {
		return nil
	}
	return m.sentBatches[index]
}

// mockLogsClient implements StatefulLogsServiceClient for testing
type mockLogsClient struct {
	mu sync.Mutex

	// Control stream creation
	createStreamErr       error // If set, LogsStream() will return this error
	failStreamCreationFor int   // Fail the next N stream creation attempts
	currentStream         *mockLogsStream
	streamCtx             context.Context
	streamCancel          context.CancelFunc
}

func newMockLogsClient() *mockLogsClient {
	return &mockLogsClient{}
}

func (m *mockLogsClient) LogsStream(ctx context.Context, _ ...grpc.CallOption) (statefulpb.StatefulLogsService_LogsStreamClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check counter-based failure first
	if m.failStreamCreationFor > 0 {
		m.failStreamCreationFor--
		err := m.createStreamErr
		// Clear error when counter reaches 0
		if m.failStreamCreationFor == 0 {
			m.createStreamErr = nil
		}
		return nil, err
	}

	// Check error-based failure (only if counter is not in use)
	if m.createStreamErr != nil {
		return nil, m.createStreamErr
	}

	// Create a new stream with a child context
	m.streamCtx, m.streamCancel = context.WithCancel(ctx)
	m.currentStream = newMockLogsStream(m.streamCtx)
	return m.currentStream, nil
}

// Helper to fail the next N stream creation attempts
func (m *mockLogsClient) failNextStreamCreations(count int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failStreamCreationFor = count
	m.createStreamErr = err
}

// Helper to get current stream
func (m *mockLogsClient) getCurrentStream() *mockLogsStream {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentStream
}

// testFixture holds all the components needed for testing
type testFixture struct {
	t              *testing.T
	mockClock      *clock.Mock
	mockClient     *mockLogsClient
	mockSink       *mockSink
	inputChan      chan *message.Payload
	outputChan     chan *message.Payload
	destCtx        *client.DestinationsContext
	endpoint       config.Endpoint
	streamLifetime time.Duration
	worker         *streamWorker
}

// newTestFixture creates all the test infrastructure
func newTestFixture(t *testing.T) *testFixture {
	// Create mock client
	mockClient := newMockLogsClient()

	// Create mock sink
	mockSink := newMockSink()

	// Create input channel
	inputChan := make(chan *message.Payload, 100)

	// Create mock destination context
	destCtx := client.NewDestinationsContext()
	destCtx.Start()

	// Create endpoint config with test backoff settings
	endpoint := config.Endpoint{
		BackoffFactor:    2.0,
		BackoffBase:      1.0,
		BackoffMax:       10.0,
		RecoveryInterval: 2,
		RecoveryReset:    false,
	}

	// Create mock clock
	mockClock := clock.NewMock()

	fixture := &testFixture{
		t:              t,
		mockClock:      mockClock,
		mockClient:     mockClient,
		mockSink:       mockSink,
		inputChan:      inputChan,
		outputChan:     mockSink.outputChan,
		destCtx:        destCtx,
		endpoint:       endpoint,
		streamLifetime: 10 * time.Second,
	}

	return fixture
}

// createWorker creates a streamWorker with the fixture's components
func (f *testFixture) createWorker() *streamWorker {
	return f.createWorkerWithInflight(nil) // nil = use default maxInflight
}

// createWorkerWithInflight creates a streamWorker with custom inflight capacity for testing
func (f *testFixture) createWorkerWithInflight(inflight *inflightTracker) *streamWorker {
	worker := newStreamWorkerWithClock(
		"test-worker",
		f.inputChan,
		f.destCtx,
		nil, // conn not needed with mock client
		f.mockClient,
		f.mockSink,
		f.endpoint,
		f.streamLifetime,
		f.mockClock,
		inflight,
		"logs",
	)
	f.worker = worker
	return worker
}

// cleanup shuts down all resources
func (f *testFixture) cleanup() {
	if f.worker != nil {
		// Check if worker is still running before stopping
		select {
		case <-f.worker.done:
			// Already stopped
		default:
			f.worker.stop()
		}
	}
	if f.destCtx != nil {
		f.destCtx.Stop()
	}
}

// Helper to create test payload for stream worker tests
func createWorkerTestPayload(content string) *message.Payload {
	return &message.Payload{
		Encoded: []byte(content),
		MessageMetas: []*message.MessageMetadata{
			{
				RawDataLen: len(content),
			},
		},
	}
}

// TestStreamWorkerBasicStartStop tests the basic lifecycle
func TestStreamWorkerBasicStartStop(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	worker := fixture.createWorker()

	// Start the worker
	worker.start()

	// Wait for stream to become active (mocked stream creation should be quick)
	require.Eventually(t, func() bool {
		return worker.streamState == active
	}, testTimeout, testTickInterval, "Worker should transition to active state")

	// Verify stream was created
	stream := fixture.mockClient.getCurrentStream()
	require.NotNil(t, stream, "Stream should be created")

	// Stop the worker
	worker.stop()

	// Verify clean shutdown
	select {
	case <-worker.done:
		// Success
	case <-time.After(testTimeout):
		t.Fatal("Worker did not shut down in time")
	}
}

// TestStreamWorkerSendReceive tests basic message flow from input to output
func TestStreamWorkerSendReceive(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	worker := fixture.createWorker()
	worker.start()

	// Wait for active state
	require.Eventually(t, func() bool {
		return worker.streamState == active
	}, testTimeout, testTickInterval)

	stream := fixture.mockClient.getCurrentStream()
	require.NotNil(t, stream)

	// Send one message
	payload := createWorkerTestPayload("test message")
	fixture.inputChan <- payload

	// Wait for message to be sent to stream
	require.Eventually(t, func() bool {
		return stream.getSentBatchCount() == 1
	}, testTimeout, testTickInterval)

	// Send ack for batch 1
	stream.sendAck(1)

	// Verify message appears in output channel
	select {
	case output := <-fixture.outputChan:
		require.Equal(t, payload, output)
	case <-time.After(testTimeout):
		t.Fatal("Message should appear in outputChan after ack")
	}
}

// TestStreamWorkerReceiverFailureRotation tests stream rotation on receiver failure
// with an inflight message that gets re-sent on the new stream
func TestStreamWorkerReceiverFailureRotation(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	worker := fixture.createWorker()
	worker.start()

	// Wait for active state
	require.Eventually(t, func() bool {
		return worker.streamState == active
	}, testTimeout, testTickInterval)

	stream1 := fixture.mockClient.getCurrentStream()
	require.NotNil(t, stream1)

	// Send 1 message
	payload := createWorkerTestPayload("test message")
	fixture.inputChan <- payload

	// Wait for message to be sent to stream1
	require.Eventually(t, func() bool {
		return stream1.getSentBatchCount() == 1
	}, testTimeout, testTickInterval)

	// Give receiverLoop time to enter Recv() and block
	time.Sleep(testShortWait)

	// Inject receiver error immediately (this unblocks Recv() and triggers stream rotation)
	// Note: We do NOT send an ack, so the message stays inflight
	stream1.injectRecvError(io.EOF)

	// Wait for rotation to complete (stream changes and state is active again)
	// Recv failure triggers backoff, so advance clock to allow rotation
	var stream2 *mockLogsStream
	require.Eventually(t, func() bool {
		fixture.mockClock.Add(500 * time.Millisecond) // Advance past backoff period
		stream2 = fixture.mockClient.getCurrentStream()
		return stream2 != nil && stream2 != stream1 && worker.streamState == active
	}, testTimeout, testTickInterval, "Should complete stream rotation with new stream")

	// The inflight message should be re-sent on the new stream (after rotation reset, it's batch 1 again)
	require.Eventually(t, func() bool {
		return stream2.getSentBatchCount() == 1
	}, testTimeout, testTickInterval, "Inflight message should be re-sent on new stream")

	// Send ack for batch 1 on new stream
	stream2.sendAck(1)

	// Verify message appears in output channel
	select {
	case output := <-fixture.outputChan:
		require.Equal(t, payload, output)
	case <-time.After(testTimeout):
		t.Fatal("Message should appear in outputChan after ack on new stream")
	}
}

// TestStreamWorkerStreamTimeout tests stream rotation triggered by stream timer expiration
func TestStreamWorkerStreamTimeout(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	worker := fixture.createWorker()
	worker.start()

	// Wait for active state
	require.Eventually(t, func() bool {
		return worker.streamState == active
	}, testTimeout, testTickInterval)

	stream1 := fixture.mockClient.getCurrentStream()
	require.NotNil(t, stream1)

	// Advance clock past stream lifetime to trigger stream timeout
	fixture.mockClock.Add(fixture.streamLifetime + time.Second)

	// Wait for rotation to complete (new stream created and active)
	var stream2 *mockLogsStream
	require.Eventually(t, func() bool {
		stream2 = fixture.mockClient.getCurrentStream()
		return stream2 != nil && stream2 != stream1 && worker.streamState == active
	}, testTimeout, testTickInterval, "Should rotate to new stream after timer expires")

	// Send a message on the new stream
	payload := createWorkerTestPayload("test on stream2")
	fixture.inputChan <- payload

	// Wait for message to be sent on stream2
	require.Eventually(t, func() bool {
		return stream2.getSentBatchCount() == 1
	}, testTimeout, testTickInterval, "Message should be sent on new stream")

	// Send ack
	stream2.sendAck(1)

	// Verify message appears in output
	select {
	case output := <-fixture.outputChan:
		require.Equal(t, payload, output)
	case <-time.After(testTimeout):
		t.Fatal("Message should appear in outputChan after ack")
	}
}

// TestStreamWorkerStreamTimeoutWithDrain tests graceful rotation when stream timer expires with inflight messages
func TestStreamWorkerStreamTimeoutWithDrain(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	worker := fixture.createWorker()
	worker.start()

	// Wait for active state
	require.Eventually(t, func() bool {
		return worker.streamState == active
	}, testTimeout, testTickInterval)

	stream1 := fixture.mockClient.getCurrentStream()
	require.NotNil(t, stream1)

	// Step 1: Send 1 message on stream1, don't send ack
	payload1 := createWorkerTestPayload("message 1")
	fixture.inputChan <- payload1

	// Wait for message to be sent on stream1
	require.Eventually(t, func() bool {
		return stream1.getSentBatchCount() == 1
	}, testTimeout, testTickInterval)

	// Step 2 & 3: Advance clock to trigger stream timeout, verify draining state
	fixture.mockClock.Add(fixture.streamLifetime + time.Second)

	// Should transition to draining (not connecting) because there's an unacked message
	require.Eventually(t, func() bool {
		return worker.streamState == draining
	}, testTimeout, testTickInterval, "Should transition to draining state with unacked messages")

	// Step 4: Send another message, verify it's buffered (NOT sent on stream1)
	payload2 := createWorkerTestPayload("message 2")
	fixture.inputChan <- payload2

	// Give time for message to be processed if it was going to be sent
	time.Sleep(testShortWait)

	// stream1 should still only have 1 batch sent
	require.Equal(t, 1, stream1.getSentBatchCount(), "Message 2 should be buffered, not sent on stream1")

	// Step 5 & 6 & 7: Send ack for batch 1, verify it appears in output
	stream1.sendAck(1)

	select {
	case output := <-fixture.outputChan:
		require.Equal(t, payload1, output, "First message should appear in output")
	case <-time.After(testTimeout):
		t.Fatal("Message 1 should appear in outputChan after ack")
	}

	// Step 8: Verify stream2 is created (draining → connecting → active)
	var stream2 *mockLogsStream
	require.Eventually(t, func() bool {
		stream2 = fixture.mockClient.getCurrentStream()
		return stream2 != nil && stream2 != stream1 && worker.streamState == active
	}, testTimeout, testTickInterval, "Should complete rotation to new stream after ack received")

	// Step 9: Verify message 2 is sent on stream2 (batch ID resets to 1 after rotation)
	require.Eventually(t, func() bool {
		return stream2.getSentBatchCount() == 1
	}, testTimeout, testTickInterval, "Buffered message 2 should be sent on new stream")

	// Send ack for batch 1 on stream2 to verify it's the second message
	stream2.sendAck(1)

	select {
	case output := <-fixture.outputChan:
		require.Equal(t, payload2, output, "Second message should appear in output")
	case <-time.After(testTimeout):
		t.Fatal("Message 2 should appear in outputChan after ack on stream2")
	}
}

// TestStreamWorkerDrainTimeout tests forced rotation when drain timer expires without receiving all acks
func TestStreamWorkerDrainTimeout(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	worker := fixture.createWorker()
	worker.start()

	// Wait for active state
	require.Eventually(t, func() bool {
		return worker.streamState == active
	}, testTimeout, testTickInterval)

	stream1 := fixture.mockClient.getCurrentStream()
	require.NotNil(t, stream1)

	// Step 1: Send message on stream1, don't send ack (stays inflight)
	payload := createWorkerTestPayload("message 1")
	fixture.inputChan <- payload

	// Wait for message to be sent on stream1
	require.Eventually(t, func() bool {
		return stream1.getSentBatchCount() == 1
	}, testTimeout, testTickInterval)

	// Step 2: Advance clock to trigger stream timeout → enter draining
	fixture.mockClock.Add(fixture.streamLifetime + time.Second)

	require.Eventually(t, func() bool {
		return worker.streamState == draining
	}, testTimeout, testTickInterval, "Should transition to draining state")

	// Step 3: Advance clock to trigger drain timeout (without sending ack) → force rotation
	fixture.mockClock.Add(drainTimeout + time.Second)

	// Step 4: Verify stream2 is created (draining → connecting → active)
	var stream2 *mockLogsStream
	require.Eventually(t, func() bool {
		stream2 = fixture.mockClient.getCurrentStream()
		return stream2 != nil && stream2 != stream1 && worker.streamState == active
	}, testTimeout, testTickInterval, "Should complete rotation to new stream after drain timeout")

	// Step 5: Verify batch 1 is re-sent on stream2 (inflight message replayed)
	require.Eventually(t, func() bool {
		return stream2.getSentBatchCount() == 1
	}, testTimeout, testTickInterval, "Inflight message should be re-sent on new stream")

	// Send ack for batch 1 on stream2
	stream2.sendAck(1)

	// Verify message appears in output
	select {
	case output := <-fixture.outputChan:
		require.Equal(t, payload, output)
	case <-time.After(testTimeout):
		t.Fatal("Message should appear in outputChan after ack on new stream")
	}
}

// TestStreamWorkerBackoff tests exponential backoff on stream creation failure
func TestStreamWorkerBackoff(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	worker := fixture.createWorker()

	// Configure mock to fail stream creation once, then succeed
	testErr := errors.New("simulated stream creation failure")
	fixture.mockClient.failNextStreamCreations(1, testErr)

	// Start worker (will attempt to create stream and should fail)
	worker.start()

	// Should fail to create stream and enter disconnected state
	require.Eventually(t, func() bool {
		return worker.streamState == disconnected
	}, testTimeout, testTickInterval, "Should transition to disconnected state after stream creation failure")

	// Verify no stream was created
	require.Nil(t, fixture.mockClient.getCurrentStream(), "No stream should be created on error")

	// Advance clock gradually to trigger backoff timer and verify stream is established
	// For first error, backoff is between 1-2 seconds (base=1s, factor=2, jitter)
	var stream *mockLogsStream
	require.Eventually(t, func() bool {
		fixture.mockClock.Add(500 * time.Millisecond)
		stream = fixture.mockClient.getCurrentStream()
		return stream != nil && worker.streamState == active
	}, testTimeout, testTickInterval, "Should transition to active state after backoff expires")

	// Verify we can send a message on the new stream
	payload := createWorkerTestPayload("test message")
	fixture.inputChan <- payload

	require.Eventually(t, func() bool {
		return stream.getSentBatchCount() == 1
	}, testTimeout, testTickInterval, "Message should be sent on new stream")

	stream.sendAck(1)

	select {
	case output := <-fixture.outputChan:
		require.Equal(t, payload, output)
	case <-time.After(testTimeout):
		t.Fatal("Message should appear in outputChan after ack")
	}
}

// TestStreamWorkerBackpressure verifies that inputChan blocks when inflight is full
func TestStreamWorkerBackpressure(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	// Use small inflight capacity for fast test
	smallInflight := newInflightTracker(5, "logs")
	worker := fixture.createWorkerWithInflight(smallInflight)
	worker.start()

	// Wait for active state
	require.Eventually(t, func() bool {
		return worker.streamState == active
	}, testTimeout, testTickInterval)

	stream := fixture.mockClient.getCurrentStream()
	require.NotNil(t, stream)

	// Send 5 messages (don't send acks, so they stay in "sent" state and fill inflight)
	for i := 0; i < 5; i++ {
		fixture.inputChan <- createWorkerTestPayload("test")
	}

	// Wait for inflight to be full
	require.Eventually(t, func() bool {
		return !worker.inflight.hasSpace()
	}, testTimeout, testTickInterval, "Inflight should be full")

	// Verify backpressure: send one more message, it should NOT be consumed
	fixture.inputChan <- createWorkerTestPayload("blocked")
	time.Sleep(testShortWait)
	require.Equal(t, 1, len(fixture.inputChan), "Message should remain in inputChan due to backpressure")

	// Send ack for batch 1 to free up space
	stream.sendAck(1)

	// Verify backpressure released: the blocked message should now be consumed
	require.Eventually(t, func() bool {
		return len(fixture.inputChan) == 0
	}, testTimeout, testTickInterval, "Message should be consumed after ack frees space")
}

// TestStreamWorkerErrorRecovery tests that Send() and Recv() failures trigger rotation and retry
func TestStreamWorkerErrorRecovery(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	worker := fixture.createWorker()
	worker.start()

	// Wait for initial stream to be active
	var stream1 *mockLogsStream
	require.Eventually(t, func() bool {
		stream1 = fixture.mockClient.getCurrentStream()
		return stream1 != nil && worker.streamState == active
	}, testTimeout, testTickInterval, "Worker should reach active state")

	// Inject send error BEFORE sending message
	stream1.setSendError(errors.New("simulated send failure"))

	// Send message - this will trigger Send() failure and rotation
	payload := createWorkerTestPayload("test message")
	fixture.inputChan <- payload

	// Wait for stream rotation (new stream created)
	// Send failure triggers backoff, so advance clock to allow rotation
	var stream2 *mockLogsStream
	require.Eventually(t, func() bool {
		fixture.mockClock.Add(500 * time.Millisecond) // Advance past backoff period
		stream2 = fixture.mockClient.getCurrentStream()
		return stream2 != nil && stream2 != stream1 && worker.streamState == active
	}, testTimeout, testTickInterval, "Worker should rotate to new stream after send error")

	// New stream should have retried the message (batch 1)
	require.Eventually(t, func() bool {
		return stream2.getSentBatchCount() == 1
	}, testTimeout, testTickInterval, "Message should be retried on new stream")

	// Send ack on new stream
	stream2.sendAck(1)

	// Verify message appears in outputChan
	select {
	case output := <-fixture.outputChan:
		require.Equal(t, payload, output)
	case <-time.After(testTimeout):
		t.Fatal("Message should appear in outputChan after rotation and ack")
	}

	// Part 2: Test injectRecvError with retriable gRPC error
	// Inject recv error (codes.Unavailable falls into default case -> rotation)
	stream2.injectRecvError(status.Error(codes.Unavailable, "simulated unavailable error"))

	// Send another message
	payload2 := createWorkerTestPayload("test message 2")
	fixture.inputChan <- payload2

	// Wait for stream rotation (new stream created)
	// Recv failure triggers backoff, so advance clock to allow rotation
	var stream3 *mockLogsStream
	require.Eventually(t, func() bool {
		fixture.mockClock.Add(500 * time.Millisecond) // Advance past backoff period
		stream3 = fixture.mockClient.getCurrentStream()
		return stream3 != nil && stream3 != stream2 && worker.streamState == active
	}, testTimeout, testTickInterval, "Worker should rotate to new stream after recv error")

	// New stream should have retried the message (batch 1 - reset after rotation)
	require.Eventually(t, func() bool {
		return stream3.getSentBatchCount() == 1
	}, testTimeout, testTickInterval, "Message should be retried on new stream after recv error")

	// Send ack on new stream
	stream3.sendAck(1)

	// Verify message appears in outputChan
	select {
	case output := <-fixture.outputChan:
		require.Equal(t, payload2, output)
	case <-time.After(testTimeout):
		t.Fatal("Message should appear in outputChan after recv error rotation and ack")
	}
}

// Helper functions to create Datum objects for testing
func createPatternDefine(id uint64, template string) *statefulpb.Datum {
	return &statefulpb.Datum{
		Data: &statefulpb.Datum_PatternDefine{
			PatternDefine: &statefulpb.PatternDefine{
				PatternId: id,
				Template:  template,
			},
		},
	}
}

func createPatternDelete(id uint64) *statefulpb.Datum {
	return &statefulpb.Datum{
		Data: &statefulpb.Datum_PatternDelete{
			PatternDelete: &statefulpb.PatternDelete{
				PatternId: id,
			},
		},
	}
}

func createDictEntryDefine(id uint64, value string) *statefulpb.Datum {
	return &statefulpb.Datum{
		Data: &statefulpb.Datum_DictEntryDefine{
			DictEntryDefine: &statefulpb.DictEntryDefine{
				Id:    id,
				Value: value,
			},
		},
	}
}

func createDictEntryDelete(id uint64) *statefulpb.Datum {
	return &statefulpb.Datum{
		Data: &statefulpb.Datum_DictEntryDelete{
			DictEntryDelete: &statefulpb.DictEntryDelete{
				Id: id,
			},
		},
	}
}

// createPayloadWithState creates a payload with state changes in StatefulExtra
func createPayloadWithState(content string, stateChanges []*statefulpb.Datum) *message.Payload {
	payload := createWorkerTestPayload(content)
	if len(stateChanges) > 0 {
		payload.StatefulExtra = &StatefulExtra{
			StateChanges: stateChanges,
		}
	}
	return payload
}

// verifySnapshotContents checks if a snapshot batch contains the expected state
func verifySnapshotContents(t *testing.T, batch *statefulpb.StatefulBatch, expectedPatterns map[uint64]string, expectedDictEntries map[uint64]string) {
	require.NotNil(t, batch)
	require.Equal(t, uint32(0), batch.BatchId, "Snapshot should have batch ID 0")

	// Deserialize the snapshot data (it's a DatumSequence)
	var datumSeq statefulpb.DatumSequence
	err := proto.Unmarshal(batch.Data, &datumSeq)
	require.NoError(t, err)

	// Count what we find
	foundPatterns := make(map[uint64]string)
	foundDictEntries := make(map[uint64]string)

	for _, datum := range datumSeq.Data {
		switch d := datum.Data.(type) {
		case *statefulpb.Datum_PatternDefine:
			foundPatterns[d.PatternDefine.PatternId] = d.PatternDefine.Template
		case *statefulpb.Datum_DictEntryDefine:
			foundDictEntries[d.DictEntryDefine.Id] = d.DictEntryDefine.Value
		default:
			t.Fatalf("Snapshot should only contain PatternDefine and DictEntryDefine, got: %T", datum.Data)
		}
	}

	require.Equal(t, expectedPatterns, foundPatterns, "Snapshot patterns mismatch")
	require.Equal(t, expectedDictEntries, foundDictEntries, "Snapshot dict entries mismatch")
}

// TestStreamWorkerSnapshot tests the snapshot functionality across stream rotations
func TestStreamWorkerSnapshot(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	// Override stream lifetime for this test
	fixture.streamLifetime = time.Second
	worker := fixture.createWorker()
	worker.start()

	// Wait for initial stream to be ready
	var stream1 *mockLogsStream
	require.Eventually(t, func() bool {
		stream1 = fixture.mockClient.getCurrentStream()
		return stream1 != nil && worker.streamState == active
	}, testTimeout, testTickInterval, "Initial stream should be established")

	// === Step 1: Send Batch 1 (5 entries) ===
	batch1StateChanges := []*statefulpb.Datum{
		createPatternDefine(1, "pattern1"),
		createDictEntryDefine(1, "value1"),
		createPatternDefine(2, "pattern2"),
		createDictEntryDefine(2, "value2"),
	}
	batch1Payload := createPayloadWithState("log with p1/d1", batch1StateChanges)
	fixture.inputChan <- batch1Payload

	// Wait for batch 1 to be sent
	require.Eventually(t, func() bool {
		return stream1.getSentBatchCount() == 1
	}, testTimeout, testTickInterval, "Batch 1 should be sent")

	// === Step 2: Ack Batch 1 ===
	stream1.sendAck(1)

	// Verify batch 1 appears in outputChan
	select {
	case output := <-fixture.outputChan:
		require.Equal(t, batch1Payload, output)
	case <-time.After(testTimeout):
		t.Fatal("Batch 1 should appear in outputChan")
	}

	// === Step 3: Send Batch 2 (6 entries) ===
	batch2StateChanges := []*statefulpb.Datum{
		createPatternDelete(1),
		createDictEntryDelete(1),
		createPatternDefine(3, "pattern3"),
		createDictEntryDefine(3, "value3"),
	}
	batch2Payload := createPayloadWithState("log with p2/d2 and p3/d3", batch2StateChanges)
	fixture.inputChan <- batch2Payload

	// Wait for batch 2 to be sent
	require.Eventually(t, func() bool {
		return stream1.getSentBatchCount() == 2
	}, testTimeout, testTickInterval, "Batch 2 should be sent")

	// === Step 4: Cut stream with recv failure (before acking batch 2) ===
	stream1.injectRecvError(io.EOF)

	// Wait for stream rotation
	// Recv failure triggers backoff, so advance clock to allow rotation
	var stream2 *mockLogsStream
	require.Eventually(t, func() bool {
		fixture.mockClock.Add(500 * time.Millisecond) // Advance past backoff period
		stream2 = fixture.mockClient.getCurrentStream()
		return stream2 != nil && stream2 != stream1 && worker.streamState == active
	}, testTimeout, testTickInterval, "Stream should rotate after recv failure")

	// === Step 5: Verify snapshot on new stream ===
	// Snapshot should contain state BEFORE batch 2: {p1, p2, d1, d2}
	require.Eventually(t, func() bool {
		return stream2.getSentBatchCount() >= 1
	}, testTimeout, testTickInterval, "Snapshot should be sent on new stream")

	snapshotBatch := stream2.getSentBatch(0)
	expectedPatterns1 := map[uint64]string{
		1: "pattern1",
		2: "pattern2",
	}
	expectedDictEntries1 := map[uint64]string{
		1: "value1",
		2: "value2",
	}
	verifySnapshotContents(t, snapshotBatch, expectedPatterns1, expectedDictEntries1)

	// === Step 6: Ack snapshot (batch 0) ===
	stream2.sendAck(0)

	// === Step 7: Verify Batch 2 is retransmitted ===
	require.Eventually(t, func() bool {
		return stream2.getSentBatchCount() == 2
	}, testTimeout, testTickInterval, "Batch 2 should be retransmitted")

	batch2Retransmitted := stream2.getSentBatch(1)
	require.Equal(t, uint32(1), batch2Retransmitted.BatchId)

	// === Step 8: Ack Batch 2 ===
	stream2.sendAck(1)

	// Verify batch 2 appears in outputChan
	select {
	case output := <-fixture.outputChan:
		require.Equal(t, batch2Payload, output)
	case <-time.After(testTimeout):
		t.Fatal("Batch 2 should appear in outputChan")
	}

	// === Step 9: Send Batch 3 (3 entries) ===
	batch3StateChanges := []*statefulpb.Datum{
		createPatternDefine(4, "pattern4"),
		createDictEntryDefine(4, "value4"),
	}
	batch3Payload := createPayloadWithState("log with p4/d4", batch3StateChanges)
	fixture.inputChan <- batch3Payload

	// Wait for batch 3 to be sent
	require.Eventually(t, func() bool {
		return stream2.getSentBatchCount() == 3
	}, testTimeout, testTickInterval, "Batch 3 should be sent")

	// === Step 10: Stream timer expires ===
	fixture.mockClock.Add(time.Second)

	// Worker should enter draining state (batch 3 is still inflight)
	require.Eventually(t, func() bool {
		return worker.streamState == draining
	}, testTimeout, testTickInterval, "Worker should enter draining state")

	// === Step 11: Drain timer expires (force rotation) ===
	fixture.mockClock.Add(5 * time.Second) // drainTimeout is 5 seconds

	// Wait for new stream to be created
	var stream3 *mockLogsStream
	require.Eventually(t, func() bool {
		stream3 = fixture.mockClient.getCurrentStream()
		return stream3 != nil && stream3 != stream2 && worker.streamState == active
	}, testTimeout, testTickInterval, "Stream should rotate after drain timeout")

	// === Step 12: Verify snapshot on new stream ===
	// Snapshot should contain state AFTER batch 2, BEFORE batch 3: {p2, p3, d2, d3}
	// (p1/d1 were deleted in batch 2)
	require.Eventually(t, func() bool {
		return stream3.getSentBatchCount() >= 1
	}, testTimeout, testTickInterval, "Snapshot should be sent on new stream")

	snapshotBatch2 := stream3.getSentBatch(0)
	expectedPatterns2 := map[uint64]string{
		2: "pattern2",
		3: "pattern3",
	}
	expectedDictEntries2 := map[uint64]string{
		2: "value2",
		3: "value3",
	}
	verifySnapshotContents(t, snapshotBatch2, expectedPatterns2, expectedDictEntries2)

	// Ack snapshot and batch 3
	stream3.sendAck(0)
	stream3.sendAck(1)

	// Verify batch 3 appears in outputChan
	select {
	case output := <-fixture.outputChan:
		require.Equal(t, batch3Payload, output)
	case <-time.After(testTimeout):
		t.Fatal("Batch 3 should appear in outputChan")
	}
}

// TestStreamWorkerSupervisorResponsiveWhileSenderBlocked verifies that the supervisor
// remains responsive to other events even when the sender goroutine is blocked on Send()
func TestStreamWorkerSupervisorResponsiveWhileSenderBlocked(t *testing.T) {
	fixture := newTestFixture(t)
	defer fixture.cleanup()

	worker := fixture.createWorker()
	worker.start()

	// Wait for active state
	require.Eventually(t, func() bool {
		return worker.streamState == active
	}, testTimeout, testTickInterval)

	stream := fixture.mockClient.getCurrentStream()
	require.NotNil(t, stream)

	// Step 1: Send a message normally to verify baseline functionality
	payload1 := createWorkerTestPayload("message 1")
	fixture.inputChan <- payload1

	// Wait for it to be sent (should go through batchToSendCh to senderLoop to stream.sendCh)
	require.Eventually(t, func() bool {
		return stream.getSentBatchCount() >= 1
	}, testTimeout, testTickInterval, "First message should be sent")

	// Consume from stream.sendCh to confirm sender is working
	<-stream.sendCh

	// Step 2: Block the sender by filling stream.sendCh (capacity 10)
	// Strategy: Fill stream.sendCh so senderLoop blocks, then verify supervisor is still responsive

	// Send messages to fill:
	// - stream.sendCh: 10 batches
	// - batch being sent by senderLoop: 1 batch (blocked on stream.Send)
	// - batchToSendCh: 10 batches
	// Total = 21 messages needed
	for i := 0; i < 21; i++ {
		fixture.inputChan <- createWorkerTestPayload(string(rune('A' + (i % 26))))
	}

	// Give time for all messages to be consumed from inputChan by supervisor
	// and queued through the pipeline
	time.Sleep(testShortWait)

	// At this point (since we're not consuming from stream.sendCh):
	// - stream.sendCh should have ~10 batches
	// - batchToSendCh should have ~10 batches
	// - senderLoop is blocked on stream.Send() trying to write to full stream.sendCh
	// - supervisor's select is blocked on the send case trying to write to full batchToSendCh

	// Step 3: THE KEY TEST - Verify supervisor remains responsive while blocked on send

	// Test 1: Verify we can still write to inputChan without blocking
	// Even though supervisor is blocked on send case, it should wake up to accept input
	payloadDuringBlock := createWorkerTestPayload("message while blocked")

	// This write should not block because supervisor can wake from send to handle inputChan
	done := make(chan bool, 1)
	go func() {
		fixture.inputChan <- payloadDuringBlock
		done <- true
	}()

	// Verify the write completes quickly (doesn't block indefinitely)
	select {
	case <-done:
		// Success - inputChan write didn't block
	case <-time.After(testTimeout):
		t.Fatal("Write to inputChan should not block even when supervisor is blocked on send")
	}

	// Give supervisor time to process the payload and attempt to send batch 23
	time.Sleep(testShortWait)

	// At this point, supervisor should have received the payload and be stuck trying to send batch 23
	// because all channels are full

	// Test 2: Verify supervisor can still handle acks while blocked on send
	// This proves select can be woken by multiple different cases
	stream.sendAck(1)

	// Verify the ack is processed and payload1 appears in outputChan
	select {
	case output := <-fixture.outputChan:
		require.Equal(t, payload1, output, "Ack should be processed even while supervisor is blocked on send")
	case <-time.After(testTimeout):
		t.Fatal("Supervisor should remain responsive to acks even while blocked on send")
	}
}
