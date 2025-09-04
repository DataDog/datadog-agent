// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/benbjohnson/clock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TODO For PoC Stage 1
// - implement snapshot state transmission
// - better handle unrecoverable errors - auth/perm, protocol, stream-level gRPC status
// - telemetries (send/recv, failure, rotations)

// TODO for PoC Stage 2
// - implement more graceful shutdown, the current version we could lose some acks
// - currently, s.currentStream.stream.Send(batch) can still block, especially
//   if we have a lot of buffered payloads to re-send after a stream rotation,
//   especially if we are flow controlled. This will block the supervisor loop
// 	 and potentially backpressure the input channel
// - implement proper "stream/ordered" backpressure

// TODO for production
// - implement stream neotiation (state size, etc), able to downgrade to HTTP transport
// - Testing plan

const (
	// Various constants - may become configurable
	batchAckChanBuffer = 10
	maxInflight        = 10000
	connectionTimeout  = 10 * time.Second
	drainTimeout       = 5 * time.Second
)

// streamState represents the current state of the stream worker
//
//go:generate stringer -type=streamState
type streamState int

const (
	// disconnected is the initial state or stream creation failure backoff state
	disconnected streamState = iota
	// connecting is the state while waiting for asyncCreateNewStream to complete or fail
	connecting
	// active is the normal operating state with a valid stream
	active
	// draining waits for all acks to arrive before rotating to a new stream
	draining
)

// streamInfo holds all stream-related information
type streamInfo struct {
	stream statefulpb.StatefulLogsService_LogsStreamClient
	ctx    context.Context
	cancel context.CancelFunc
}

// streamCreationResult represents the result of async stream creation
type streamCreationResult struct {
	info *streamInfo
	err  error
}

// batchAck wraps a batch acknowledgment with stream identity to prevent stale signals
type batchAck struct {
	stream *streamInfo
	status *statefulpb.BatchStatus
}

// streamWorker manages a single gRPC bidirectional stream with Master-Slave threading model
// Architecture: One supervisor/sender goroutine + one receiver goroutine per worker
type streamWorker struct {
	// Configuration
	workerID            string
	destinationsContext *client.DestinationsContext

	// Pipeline integration
	inputChan  chan *message.Payload
	outputChan chan *message.Payload // For auditor acknowledgments
	sink       sender.Sink           // For getting auditor channel

	// gRPC connection management (shared with other streams)
	conn   *grpc.ClientConn
	client statefulpb.StatefulLogsServiceClient

	// Stream management
	currentStream  *streamInfo
	streamState    streamState
	recvFailureCh  chan *streamInfo          // Signal receiver failure with stream identity
	batchAckCh     chan *batchAck            // Signal batch acknowledgments with stream identity
	streamReadyCh  chan streamCreationResult // Signal when async stream creation completes
	streamLifetime time.Duration
	streamTimer    *clock.Timer // Timer for stream lifetime, trigger soft rotation
	drainTimer     *clock.Timer // In case of unacked payloads, drain/wait before soft rotation
	backoffTimer   *clock.Timer // In case of stream creation failure, backoff before retrying

	// Inflight tracking - tracks sent (awaiting ack) and buffered (not sent) payloads
	inflight *inflightTracker

	// Retry backoff
	backoffPolicy backoff.Policy
	nbErrors      int

	// Control
	stopChan chan struct{}
	done     chan struct{}
	clock    clock.Clock
}

// newStreamWorker creates a new gRPC stream worker
func newStreamWorker(
	workerID string,
	inputChan chan *message.Payload,
	destinationsCtx *client.DestinationsContext,
	conn *grpc.ClientConn,
	client statefulpb.StatefulLogsServiceClient,
	sink sender.Sink,
	endpoint config.Endpoint,
	streamLifetime time.Duration,
) *streamWorker {
	return newStreamWorkerWithClock(workerID, inputChan, destinationsCtx, conn, client, sink,
		endpoint, streamLifetime, clock.New(), nil)
}

// newStreamWorkerWithClock creates a new gRPC stream worker with injectable clock for testing
func newStreamWorkerWithClock(
	workerID string,
	inputChan chan *message.Payload,
	destinationsCtx *client.DestinationsContext,
	conn *grpc.ClientConn,
	client statefulpb.StatefulLogsServiceClient,
	sink sender.Sink,
	endpoint config.Endpoint,
	streamLifetime time.Duration,
	clock clock.Clock,
	inflightTracker *inflightTracker,
) *streamWorker {
	backoffPolicy := backoff.NewExpBackoffPolicy(
		endpoint.BackoffFactor,
		endpoint.BackoffBase,
		endpoint.BackoffMax,
		endpoint.RecoveryInterval,
		endpoint.RecoveryReset,
	)

	// Use provided inflightTracker (testing) or create default one
	if inflightTracker == nil {
		inflightTracker = newInflightTracker(maxInflight)
	}

	worker := &streamWorker{
		workerID:            workerID,
		destinationsContext: destinationsCtx,
		inputChan:           inputChan,
		outputChan:          nil,
		sink:                sink,
		conn:                conn,
		client:              client,
		streamState:         disconnected,
		recvFailureCh:       make(chan *streamInfo),
		batchAckCh:          make(chan *batchAck, batchAckChanBuffer),
		streamReadyCh:       make(chan streamCreationResult),
		streamLifetime:      streamLifetime,
		inflight:            inflightTracker,
		backoffPolicy:       backoffPolicy,
		nbErrors:            0,
		stopChan:            make(chan struct{}),
		done:                make(chan struct{}),
		clock:               clock,
		streamTimer:         createStoppedTimer(clock, 0),
		backoffTimer:        createStoppedTimer(clock, 0),
		drainTimer:          createStoppedTimer(clock, 0),
	}

	return worker
}

// start begins the supervisor goroutine & creates a new stream asynchronously
func (s *streamWorker) start() {
	log.Infof("Starting gRPC stream worker %s", s.workerID)
	s.outputChan = s.sink.Channel()

	// Start supervisor/sender goroutine (master)
	go s.supervisorLoop()

	s.asyncCreateNewStream()

	log.Infof("Worker %s: Started", s.workerID)
}

// stop shuts down the stream worker
func (s *streamWorker) stop() {
	log.Infof("Stopping gRPC stream worker %s", s.workerID)
	close(s.stopChan)
	<-s.done
	log.Infof("Worker %s: Stopped", s.workerID)
}

// supervisorLoop is the master goroutine that handles sending and stream lifecycle
func (s *streamWorker) supervisorLoop() {
	defer close(s.done)

	// supervisor loop starts without a stream, but asyncCreateNewStream is called
	// right after in streamWorker's start(), so we are in connecting state right away
	s.streamState = connecting

	for {
		// Conditional inputChan - only enabled when inflight tracker has space
		// This backpressures to upstream when at capacity
		var inputChan <-chan *message.Payload
		if s.inflight.hasSpace() {
			inputChan = s.inputChan // Enable reading
		} else {
			inputChan = nil // Disable reading
		}

		select {
		case payload := <-inputChan:
			// Fires in any state (gated only by inflight capacity), payload is always
			// added to the inflight tracker. But we only proceed to send if we are
			// in the active state with a valid stream
			s.inflight.append(payload)
			s.sendPayloads()

		case ack := <-s.batchAckCh:
			// Fires in any state
			s.handleBatchAck(ack)

		case failedStream := <-s.recvFailureCh:
			// Fires in active/draining/connecting states
			s.handleRecvFailure(failedStream)

		case result := <-s.streamReadyCh:
			// Fires only in connecting state
			s.handleStreamReady(result)

		case <-s.streamTimer.C:
			// Fires only in active state (except rare timing race, it's in connecting)
			s.handleStreamTimeout()

		case <-s.drainTimer.C:
			// Fires in draining state or (rarely) in connecting/active state
			// If in non-draining state, it means acks arrival at the same time
			// as the drain timer expiration, so we will skip the signal
			s.handleDrainTimeout()

		case <-s.backoffTimer.C:
			// Fires only in disconnected state
			s.handleBackoffTimeout()

		case <-s.stopChan:
			// Fires in any state
			s.handleShutdown()
			return
		}
	}
}

// sendPayloads attempts to send all buffered payloads when in Active state
// the same function is used to send new payload in normal operation, and
// to send (or resend) buffered payloads after a stream rotation
func (s *streamWorker) sendPayloads() {
	if s.streamState != active {
		return
	}

	// Send all buffered payloads in order
	for {
		payload := s.inflight.nextToSend()
		if payload == nil {
			// No more buffered payloads to send
			break
		}

		batchID := s.inflight.nextBatchID()
		batch := s.payloadToBatch(payload, batchID)

		// TODO Send call can block, by TCP/HTTP2 flow controls
		if err := s.currentStream.stream.Send(batch); err != nil {
			log.Warnf("Worker %s: Send failed, initiating stream rotation: %v", s.workerID, err)
			s.beginStreamRotation()
			return // stop sending, payloads remain buffered for next rotation
		}

		// Successfully sent, mark as sent in the inflight tracker
		s.inflight.markSent()
	}
}

// handleBatchAck processes a BatchStatus acknowledgment from the server
func (s *streamWorker) handleBatchAck(ack *batchAck) {
	// Ignore stale acks from old streams
	if ack.stream != s.currentStream {
		return
	}

	receivedBatchID := uint32(ack.status.BatchId)

	// The two errors below should never happen if Intake is implemented
	// correctly, but we are being defensive.

	// Verify we have "sent payloads" awaiting ack
	if !s.inflight.hasUnacked() {
		log.Errorf("Worker %s: Received ack for batch %d but no sent payloads in inflight tracker, "+
			"irrecoverable error - initiating stream rotation", s.workerID, receivedBatchID)
		s.beginStreamRotation()
		return
	}

	// Verify batchID matches expected sequence
	expectedBatchID := s.inflight.getHeadBatchID()
	if receivedBatchID != expectedBatchID {
		log.Errorf("Worker %s: BatchID mismatch! Expected %d, received %d. "+
			"ut-of-order or duplicate ack, irrecoverable error - initiating stream rotation",
			s.workerID, expectedBatchID, receivedBatchID)
		s.beginStreamRotation()
		return
	}

	// Pop the acknowledged payload and send to auditor
	payload := s.inflight.pop()
	if s.outputChan != nil {
		select {
		case s.outputChan <- payload:
			// Successfully sent to auditor
		default:
			log.Warnf("Worker %s: Auditor channel full, dropping ack for batch %d", s.workerID, receivedBatchID)
		}
	}

	// If in Draining state and all acks received, transition to Connecting
	if s.streamState == draining && !s.inflight.hasUnacked() {
		log.Infof("Worker %s: All acks received in draining state, proceeding with rotation", s.workerID)
		s.drainTimer.Stop()
		s.beginStreamRotation()
	}
}

// handleRecvFailure processes receiver failure signals
func (s *streamWorker) handleRecvFailure(failedStream *streamInfo) {
	// Ignore if: stale signal OR not in active/draining state
	if failedStream != s.currentStream || (s.streamState != active && s.streamState != draining) {
		return
	}

	log.Infof("Worker %s: Receiver reported failure (state: %v), initiating stream rotation", s.workerID, s.streamState)
	s.beginStreamRotation()
}

// handleStreamReady processes async stream creation results
func (s *streamWorker) handleStreamReady(result streamCreationResult) {
	if s.streamState != connecting {
		return
	}

	if result.err != nil {
		s.nbErrors = s.backoffPolicy.IncError(s.nbErrors)
		s.handleStreamCreationFailure(result.err)
	} else {
		s.nbErrors = s.backoffPolicy.DecError(s.nbErrors)
		s.finishStreamRotation(result.info)
	}
}

// handleStreamTimeout processes stream lifetime expiration
func (s *streamWorker) handleStreamTimeout() {
	if s.streamState != active {
		return
	}

	if s.inflight.hasUnacked() {
		log.Infof("Worker %s: Stream lifetime expired with %d unacked payloads, entering Draining state",
			s.workerID, s.inflight.sentCount())
		s.streamState = draining
		s.drainTimer.Reset(drainTimeout)
	} else {
		log.Infof("Worker %s: Stream lifetime expired with no unacked payloads, rotating immediately",
			s.workerID)
		s.beginStreamRotation()
	}
}

// handleDrainTimeout handles drain timer expiration
func (s *streamWorker) handleDrainTimeout() {
	if s.streamState != draining {
		return
	}

	log.Warnf("Worker %s: Drain timer expired in draining state, proceeding with rotation (may lose some acks)",
		s.workerID)
	s.beginStreamRotation()
}

// handleBackoffTimeout processes backoff timer expiration and retries stream creation
func (s *streamWorker) handleBackoffTimeout() {
	if s.streamState != disconnected {
		return
	}

	log.Infof("Worker %s: Backoff timer expired, retrying stream creation (error count: %d)", s.workerID, s.nbErrors)
	s.streamState = connecting
	s.asyncCreateNewStream()
}

// handleShutdown performs graceful shutdown cleanup
func (s *streamWorker) handleShutdown() {
	log.Infof("Worker %s: Shutting down", s.workerID)
	s.streamTimer.Stop()
	s.backoffTimer.Stop()
	s.drainTimer.Stop()
	s.closeStream(s.currentStream)
}

// beginStreamRotation initiates stream rotation
// Closes current stream and starts async creation of a new stream
func (s *streamWorker) beginStreamRotation() {
	log.Infof("Worker %s: Beginning stream rotation (state: %v → connecting)", s.workerID, s.streamState)

	s.closeStream(s.currentStream)
	s.currentStream = nil
	s.streamTimer.Stop()
	s.drainTimer.Stop()
	s.backoffTimer.Stop()

	s.streamState = connecting
	s.asyncCreateNewStream()
}

// finishStreamRotation completes stream rotation (Connecting → Active transition)
// Activates the newly created stream and starts the receiver
// Transmits the snapshot state first, then (if any) the buffered payloads
func (s *streamWorker) finishStreamRotation(streamInfo *streamInfo) {
	log.Infof("Worker %s: Finishing stream rotation (state: connecting → active)", s.workerID)

	s.currentStream = streamInfo
	s.streamState = active

	go s.receiverLoop(streamInfo)

	s.streamTimer.Reset(s.streamLifetime)

	// Convert all the unacked items to buffered items by resetting inflight tracker
	// because we need to resent them.
	s.inflight.resetOnRotation()

	log.Infof("Worker %s: Stream rotation complete, now active", s.workerID)

	// TODO implement: transmit the snapshot state first
	// Then send the remaining buffered payloads
	if s.inflight.hasUnSent() {
		s.sendPayloads()
	}
}

// handleStreamCreationFailure processes stream creation failures with exponential backoff
func (s *streamWorker) handleStreamCreationFailure(err error) {
	backoffDuration := s.backoffPolicy.GetBackoffDuration(s.nbErrors)

	log.Warnf("Worker %s: Stream creation failed: %v. Backing off for %v (error count: %d)",
		s.workerID, err, backoffDuration, s.nbErrors)

	s.streamState = disconnected

	if backoffDuration > 0 {
		s.backoffTimer.Reset(backoffDuration)
	} else {
		// it shouldn't happen, but be defensive
		// retry immediately by transitioning directly to connecting
		log.Infof("Worker %s: Zero backoff duration, retrying immediately", s.workerID)
		s.streamState = connecting
		s.asyncCreateNewStream()
	}
}

// asyncCreateNewStream creates a new gRPC stream asynchronously
// Signals completion (success or failure) via streamReadyCh
func (s *streamWorker) asyncCreateNewStream() {
	go func() {
		log.Infof("Worker %s: Starting async stream creation", s.workerID)

		var result streamCreationResult

		// Ensure the connection is ready, can block up to connectionTimeout
		err := s.ensureConnectionReady()
		if err != nil {
			log.Errorf("Worker %s: Async stream creation failed (connection failure) %v", s.workerID, err)
			result = streamCreationResult{info: nil, err: err}
		} else {
			// Create per-stream context derived from destinations context
			streamCtx, streamCancel := context.WithCancel(s.destinationsContext.Context())

			// Create the stream, shouldn't block at this point.
			stream, err := s.client.LogsStream(streamCtx)

			if err != nil {
				streamCancel()
				log.Errorf("Worker %s: Async stream creation failed (post-connection): %v", s.workerID, err)
				result = streamCreationResult{info: nil, err: err}
			} else {
				log.Infof("Worker %s: Async stream creation succeeded", s.workerID)
				result = streamCreationResult{
					info: &streamInfo{
						stream: stream,
						ctx:    streamCtx,
						cancel: streamCancel,
					},
					err: nil,
				}
			}
		}

		// Signal result to supervisor (blocks until received or stopped)
		select {
		case s.streamReadyCh <- result:
		case <-s.stopChan:
			// Worker stopped before supervisor could receive result
			// We own cleanup since supervisor never got the stream
			if result.info != nil {
				s.closeStream(result.info)
			}
		}
	}()
}

func (s *streamWorker) ensureConnectionReady() error {
	// Skip connection check if conn is nil (for testing with mock clients)
	if s.conn == nil {
		return nil
	}

	connCtx, cancel := context.WithTimeout(s.destinationsContext.Context(), connectionTimeout)
	defer cancel()

	// Nudge dialing if idle; doesn't block
	s.conn.Connect()

	for {
		state := s.conn.GetState()
		switch state {
		case connectivity.Ready:
			return nil
		case connectivity.Shutdown:
			return errors.New("gRPC conn is shutdown")
		}
		// Wait for state change or timeout/cancel.
		if !s.conn.WaitForStateChange(connCtx, state) {
			// context done (timeout or cancellation)
			return connCtx.Err()
		}
	}
}

// closeStream safely closes a stream and cancels its context
func (s *streamWorker) closeStream(streamInfo *streamInfo) {
	if streamInfo != nil {
		if err := streamInfo.stream.CloseSend(); err != nil {
			log.Debugf("Worker %s: Error closing stream send: %v", s.workerID, err)
		}
		streamInfo.cancel()
	}
}

// receiverLoop runs in the receiver goroutine to process server responses for a specific stream
// The receiver is stateless - it only forwards acks/errors to the supervisor
// This goroutine exits when the stream fails (after signaling the supervisor)
func (s *streamWorker) receiverLoop(streamInfo *streamInfo) {
	stream := streamInfo.stream
	for {
		msg, err := stream.Recv()
		if err == nil {
			// Normal message (batch acknowledgment) - forward to supervisor
			s.signalBatchAck(streamInfo, msg)
			continue
		}

		// Clean inbound close (server OK in trailers): policy = signal receiver failure
		if errors.Is(err, io.EOF) {
			log.Warnf("Worker %s: Stream closed by server", s.workerID)
			s.signalRecvFailure(streamInfo)
			return
		}

		// Local cancel/deadline (supervisor rotated, worker shutdown): just exit
		ctxErr := streamInfo.ctx.Err()
		if errors.Is(ctxErr, context.Canceled) || errors.Is(ctxErr, context.DeadlineExceeded) {
			log.Infof("Worker %s: Stream context cancelled, receiver exiting", s.workerID)
			return
		}

		// Stream-level gRPC status (non-OK): RPC is over → signal receiver failure or block terminal
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.Unauthenticated, codes.PermissionDenied:
				// Terminal until fixed; do not signal receiver failure here
				s.handleIrrecoverableError("auth/perm: "+st.Message(), streamInfo)
				return
			case codes.InvalidArgument, codes.FailedPrecondition, codes.OutOfRange, codes.Unimplemented:
				// Terminal protocol/semantic issue; do not signal receiver failure
				s.handleIrrecoverableError("protocol: "+st.Message(), streamInfo)
				return
			default:
				// All other non-OK statuses: signal receiver failure
				log.Warnf("Worker %s: gRPC error (code %v): %v", s.workerID, st.Code(), err)
				s.signalRecvFailure(streamInfo)
				return
			}
		}

		// Transport error without status (RST/GOAWAY/TLS, socket close): signal receiver failure
		log.Warnf("Worker %s: Transport error: %v", s.workerID, err)
		s.signalRecvFailure(streamInfo)
		return
	}
}

// signalRecvFailure signals the supervisor to rotate the stream
func (s *streamWorker) signalRecvFailure(streamInfo *streamInfo) {
	// This signaling is blocking by design, it's okey to block the receiver,
	// since the only way we get here is through an irrecoverable error.
	select {
	case s.recvFailureCh <- streamInfo:
	case <-s.stopChan:
	}
}

// signalBatchAck forwards a batch acknowledgment to the supervisor
// If the worker is stopped, returns without delivering (shutdown is in progress anyway)
func (s *streamWorker) signalBatchAck(streamInfo *streamInfo, msg *statefulpb.BatchStatus) {
	select {
	case s.batchAckCh <- &batchAck{stream: streamInfo, status: msg}:
	case <-s.stopChan:
	}
}

// handleIrrecoverableError are errors that shouldn't be retried, and ideally
// should be block the ingestion, until the error is resolved.
func (s *streamWorker) handleIrrecoverableError(_ string, streamInfo *streamInfo) {
	// Currently this is treated as stream error, which will trigger a stream rotation
	// and retry of the same payload, which loops on. this IS NOT the desired behavior.
	// TODO: Implement proper handling of irrecoverable errors, by blocking the ingestion
	s.signalRecvFailure(streamInfo)
}

// payloadToBatch converts a message payload to a StatefulBatch
// The payload.Encoded contains serialized DatumSequence (from batch_strategy)
func (s *streamWorker) payloadToBatch(payload *message.Payload, batchID uint32) *statefulpb.StatefulBatch {
	batch := &statefulpb.StatefulBatch{
		BatchId: batchID,
		Data:    payload.Encoded,
	}

	return batch
}

// createStoppedTimer creates a timer that is stopped and has its channel drained
func createStoppedTimer(clk clock.Clock, d time.Duration) *clock.Timer {
	t := clk.Timer(d)
	if !t.Stop() {
		<-t.C
	}
	return t
}
