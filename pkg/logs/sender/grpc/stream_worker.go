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
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TODO For PoC Stage 1
// - telemetries (send/recv, failure, rotations)

// TODO for PoC Stage 2
// - better handle unrecoverable errors - auth/perm, protocol, stream-level gRPC status
// - implement more graceful shutdown, the current version we could lose some acks
// - implement jitter is stream lifetime, otherwise all streams will rotate at the same time
// - implement proper "stream/ordered" backpressure

// TODO for production
// - implement stream neotiation (state size, etc), able to downgrade to HTTP transport
// - Testing plan

// Notes on failure handling and backoff strategy:
// Unlike HTTP transport, stateful transport is stream-based. Once the stream is established,
// it's unlikely to fail sporadically at request level. The failure mode is likely to be:
// - Proxy is not available
//   fail in asyncCreateNewStream after connectionTimeout (10s)
// - Proxy is available, but Intake is not available
//   succeed in asyncCreateNewStream, but currentStream fails to send or receive any message
// - Stream is functioning, but agent has wrong credentials configuration
//   succeed in asyncCreateNewStream, but recv will fail with auth/perm error consistently
// Note: again unlike HTTP transport where a Intake back-pressure shows up a rejection at request
// level, in stateful transport, the back-pressure is more likely to show up as blocking send on
// a functioning stream.
// Due to reasons above, we will track the failures at stream level, and implement backoff
// for stream creation only. We use the same backoff policy as HTTP transport (defined for endpoint)
// with 1 tweak.
//   - nbErrors is incremented for each send/recv/protocol-error/stream-creation failures
//   - first time an valid ack is received on a new stream, we consider the stream established
//     and reset nbErrors to 0 (via RecoveryReset=true)

const (
	// Various constants - may become configurable
	ioChanBufferSize  = 10
	maxInflight       = 10000
	connectionTimeout = 10 * time.Second
	drainTimeout      = 5 * time.Second
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

// streamWorker manages a single gRPC bidirectional stream with Master - 2 Slave threading model
// Architecture: One supervisor goroutine + one sender goroutine + one receiver goroutine per stream
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
	currentStream   *streamInfo
	streamState     streamState
	streamFailureCh chan *streamInfo          // Signal sender/receiver failure with stream identity
	streamReadyCh   chan streamCreationResult // Signal when async stream creation completes
	streamLifetime  time.Duration
	streamTimer     *clock.Timer // Timer for stream lifetime, trigger soft rotation
	drainTimer      *clock.Timer // In case of unacked payloads, drain/wait before soft rotation
	backoffTimer    *clock.Timer // In case of stream creation failure, backoff before retrying

	// Channels for communication between supervisor and sender/receiver goroutines
	batchToSendCh chan *statefulpb.StatefulBatch // Signal batch to send to sender goroutine
	batchAckCh    chan *batchAck                 // Signal batch acknowledgments with stream identity

	// Inflight tracking - tracks sent (awaiting ack) and buffered (not sent) payloads
	inflight *inflightTracker

	// Retry backoff
	backoffPolicy backoff.Policy
	nbErrors      int

	// Compression for snapshot state
	compression compression.Compressor

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
	compressor compression.Compressor,
) *streamWorker {
	return newStreamWorkerWithClock(workerID, inputChan, destinationsCtx, conn, client, sink,
		endpoint, streamLifetime, compressor, clock.New(), nil)
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
	compressor compression.Compressor,
	clock clock.Clock,
	inflightTracker *inflightTracker,
) *streamWorker {
	backoffPolicy := backoff.NewExpBackoffPolicy(
		endpoint.BackoffFactor,
		endpoint.BackoffBase,
		endpoint.BackoffMax,
		endpoint.RecoveryInterval,
		true, // RecoveryReset = true (see stream failure comment above)
	)

	// Use provided inflightTracker (testing) or create default one
	if inflightTracker == nil {
		inflightTracker = newInflightTracker(workerID, maxInflight)
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
		streamFailureCh:     make(chan *streamInfo),
		batchAckCh:          make(chan *batchAck, ioChanBufferSize),
		streamReadyCh:       make(chan streamCreationResult),
		streamLifetime:      streamLifetime,
		inflight:            inflightTracker,
		backoffPolicy:       backoffPolicy,
		nbErrors:            0,
		compression:         compressor,
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

	// Start supervisor goroutine
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

// supervisorLoop is the main goroutine:
// - it receives input from upstream and manages the inflight tracking
// - it manages send/recv batches over the stream via sender/receiver goroutines
// - it handles stream lifecycle, rotation and backoff
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

		// Conditional send - only enabled when in active state with unsent payloads
		// This allows supervisor to progress on sending or block, but be woken by other events
		// Important: getNextBatch call is idempotent, it doesn't change inflight tracker state,
		// so if write to sendChan is blocked, next iteration will try again with the same batch.
		var nextBatch *statefulpb.StatefulBatch
		var sendChan chan<- *statefulpb.StatefulBatch
		if s.streamState == active && s.inflight.hasUnSent() {
			sendChan = s.batchToSendCh // Enable sending
			nextBatch = s.getNextBatch()
		} else {
			sendChan = nil // Disable sending
		}

		select {
		case payload := <-inputChan:
			// Fires in any state (gated only by inflight capacity), payload is always
			// added to the inflight tracker
			s.inflight.append(payload)

		case sendChan <- nextBatch:
			// Only happens if inflight hasUnSent() and streamState is active
			// Successfully queued batch to sender goroutine
			s.inflight.markSent()

		case ack := <-s.batchAckCh:
			// Fires in any state
			s.handleBatchAck(ack)

		case failedStream := <-s.streamFailureCh:
			// Fires in active/draining/connecting states
			s.handleStreamFailure(failedStream)

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

// handleBatchAck processes a BatchStatus acknowledgment from the server
func (s *streamWorker) handleBatchAck(ack *batchAck) {
	// Ignore stale acks from old streams
	if ack.stream != s.currentStream {
		return
	}

	receivedBatchID := uint32(ack.status.BatchId)

	// Handle snapshot/state ack (batch 0) - no payload to pop
	if receivedBatchID == 0 {
		return
	}

	// The two errors below should never happen if Intake is implemented
	// correctly, but we are being defensive.

	// Verify we have "sent payloads" awaiting ack
	if !s.inflight.hasUnacked() {
		log.Errorf("Worker %s: Received ack for batch %d but no sent payloads "+
			"in inflight tracker, terminating stream", s.workerID, receivedBatchID)
		tlmWorkerStreamErrors.Inc(s.workerID, "received_ack_but_no_sent_payloads")
		s.tryBeginStreamRotation(true)
		return
	}

	// Verify batchID matches expected sequence
	expectedBatchID := s.inflight.getHeadBatchID()
	if receivedBatchID != expectedBatchID {
		log.Errorf("Worker %s: BatchID mismatch! Expected %d, received %d. "+
			"out-of-order or duplicate ack, terminating stream",
			s.workerID, expectedBatchID, receivedBatchID)
		tlmWorkerStreamErrors.Inc(s.workerID, "batch_id_mismatch")
		s.tryBeginStreamRotation(true)
		return
	}

	if receivedBatchID == 1 {
		// Here we receive the ack of "first" real (non-snapshot) batch, so we can
		// assume that the stream is really operational. decrement the nb error count.
		s.nbErrors = s.backoffPolicy.DecError(s.nbErrors)
	}

	// Pop the acknowledged payload and send to auditor
	payload := s.inflight.pop()

	// Update shared agent-level metrics so gRPC sends are reflected in agent status
	metrics.LogsSent.Add(payload.Count())
	metrics.TlmLogsSent.Add(float64(payload.Count()))
	metrics.BytesSent.Add(int64(payload.UnencodedSize))
	metrics.TlmBytesSent.Add(float64(payload.UnencodedSize), "logs")
	metrics.EncodedBytesSent.Add(int64(len(payload.Encoded)))
	metrics.TlmEncodedBytesSent.Add(float64(len(payload.Encoded)), "logs", "grpc")

	if s.outputChan != nil {
		select {
		case s.outputChan <- payload:
			// Successfully sent to auditor
		default:
			log.Warnf("Worker %s: Auditor channel full, dropping ack for batch %d",
				s.workerID, receivedBatchID)

			// TODO: is this the only possible drop?
			tlmWorkerBytesDropped.Add(float64(len(payload.Encoded)), s.workerID)
			// TODO: update this metric with # logs (requires parsing payload)
			// metrics.DestinationLogsDropped.Set(s.endpoint.Host, &expvar.Int{})
			// metrics.LogsDropped.Inc(s.workerID, 1)
			// TODO: other general metrics to update?
		}
	}

	// If in Draining state and all acks received, transition to Connecting
	if s.streamState == draining && !s.inflight.hasUnacked() {
		log.Infof("Worker %s: All acks received in draining state, proceeding with rotation", s.workerID)
		s.drainTimer.Stop()
		s.tryBeginStreamRotation(false)
	}
}

// handleStreamFailure processes sender/receiver failure signals
func (s *streamWorker) handleStreamFailure(failedStream *streamInfo) {
	// Ignore if: stale signal OR not in active/draining state
	if failedStream != s.currentStream || (s.streamState != active && s.streamState != draining) {
		return
	}

	log.Infof("Worker %s: Sender or Receiver reported failure (state: %v), terminating stream",
		s.workerID, s.streamState)
	s.tryBeginStreamRotation(true)
}

// handleStreamReady processes async stream creation results
func (s *streamWorker) handleStreamReady(result streamCreationResult) {
	if s.streamState != connecting {
		return
	}

	if result.err != nil {
		log.Warnf("Worker %s: Stream creation failed: %v", s.workerID, result.err)
		tlmWorkerStreamErrors.Inc(s.workerID, "stream_creation_failed")

		s.tryBeginStreamRotation(true)
	} else {
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
		s.tryBeginStreamRotation(false)
	}
}

// handleDrainTimeout handles drain timer expiration
func (s *streamWorker) handleDrainTimeout() {
	if s.streamState != draining {
		return
	}

	log.Warnf("Worker %s: Drain timer expired in draining state, proceeding with rotation "+
		"(may lose some acks)", s.workerID)
	s.tryBeginStreamRotation(false)
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
	if s.batchToSendCh != nil {
		close(s.batchToSendCh)
	}
	s.closeStream(s.currentStream)
}

// tryBeginStreamRotation try to initiates stream rotation
// If dueToFailure is false (from soft rotation due stream lifetime expiration, or drain timeout),
// the stream rotation is initiated immediately, otherwise (due to failure scenario), it transitions
// to disconnected state and wait for the backoff duration.
func (s *streamWorker) tryBeginStreamRotation(dueToFailure bool) {

	// Close current stream if it exists
	if s.currentStream != nil {
		close(s.batchToSendCh)
		s.closeStream(s.currentStream)
		s.currentStream = nil
		s.streamTimer.Stop()
		s.drainTimer.Stop()
		s.backoffTimer.Stop()
	}

	var backoffDuration time.Duration
	if dueToFailure {
		s.nbErrors = s.backoffPolicy.IncError(s.nbErrors)
		backoffDuration = s.backoffPolicy.GetBackoffDuration(s.nbErrors)
	}

	if !dueToFailure || backoffDuration == 0 {
		log.Infof("Worker %s: Beginning stream creation (state: %v → connecting)", s.workerID, s.streamState)
		s.streamState = connecting
		s.asyncCreateNewStream()
	} else {
		log.Infof("Worker %s: Backing off stream creation for %v (error count: %d)", s.workerID, backoffDuration, s.nbErrors)
		s.streamState = disconnected
		s.backoffTimer.Reset(backoffDuration)
	}
}

// finishStreamRotation completes stream rotation (Connecting → Active transition)
// Activates the newly created stream and starts the receiver
// Transmits the snapshot state first, then (if any) the buffered payloads
func (s *streamWorker) finishStreamRotation(streamInfo *streamInfo) {
	log.Infof("Worker %s: Finishing stream rotation (state: connecting → active)", s.workerID)

	batchToSendCh := make(chan *statefulpb.StatefulBatch, ioChanBufferSize)
	s.currentStream = streamInfo
	s.streamState = active
	s.batchToSendCh = batchToSendCh

	// Start sender and receiver goroutines for this stream
	go s.senderLoop(streamInfo, batchToSendCh)
	go s.receiverLoop(streamInfo)

	s.streamTimer.Reset(s.streamLifetime)

	// Convert all the unacked items to buffered/unsent items by resetting inflight
	// tracker. Supervisor loop will pick them up automatically and send them.
	s.inflight.resetOnRotation()

	log.Infof("Worker %s: Stream rotation complete, now active", s.workerID)

	// Send snapshot state first (batch 0)
	serialized := s.inflight.getSnapshot()
	if serialized != nil {
		// Compress snapshot like regular batches
		compressed, err := s.compression.Compress(serialized)
		if err != nil {
			log.Errorf("Worker %s: Failed to compress snapshot: %v", s.workerID, err)
		} else {
			// Send compressed snapshot to sender goroutine via channel
			// This call won't block because it's buffered channel's first write
			s.batchToSendCh <- createBatch(compressed, 0)
		}
	}
}

// asyncCreateNewStream creates a new gRPC stream asynchronously
// Signals completion (success or failure) via streamReadyCh
func (s *streamWorker) asyncCreateNewStream() {
	go func() {
		log.Infof("Worker %s: Starting async stream creation", s.workerID)
		tlmWorkerStreamsOpened.Inc(s.workerID)

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

// ensureConnectionReady ensures the gRPC connection is ready
// It triggers the connection establishment to the remove server (if not already done).
// It blocks until either the connection is ready or connectionTimeout is reached.
// This function can block, since should be called in asyncCreateNewStream context.
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

// closeStream safely closes a stream by cancelling its context
func (s *streamWorker) closeStream(streamInfo *streamInfo) {
	if streamInfo != nil {
		streamInfo.cancel()
	}
}

// senderLoop runs in the sender goroutine to send batches to the server
// The sender is stateless - it only sends batches to the server and signals stream failure
// This goroutine exits when the stream fails/terminates
func (s *streamWorker) senderLoop(streamInfo *streamInfo, batchToSendCh chan *statefulpb.StatefulBatch) {
	for batch := range batchToSendCh {
		if err := streamInfo.stream.Send(batch); err != nil {
			// Check if it's due to context cancellation (clean shutdown/rotation)
			ctxErr := streamInfo.ctx.Err()
			if errors.Is(ctxErr, context.Canceled) || errors.Is(ctxErr, context.DeadlineExceeded) {
				log.Infof("Worker %s: Stream context cancelled, sender exiting", s.workerID)
				return
			}

			// Real send failure, signal to supervisor
			log.Warnf("Worker %s: Send failed: %v, terminating stream", s.workerID, err)
			s.signalStreamFailure(streamInfo, "send_err_"+status.Code(err).String())
			return
		}
		tlmWorkerBytesSent.Add(float64(proto.Size(batch)), s.workerID)
	}
	log.Infof("Worker %s: Sender channel closed, sender exiting", s.workerID)
}

// receiverLoop runs in the receiver goroutine to process server responses for a specific stream
// The receiver is stateless - it only forwards acks/errors to the supervisor
// This goroutine exits when the stream fails/terminates
func (s *streamWorker) receiverLoop(streamInfo *streamInfo) {
	for {
		msg, err := streamInfo.stream.Recv()
		if err == nil {
			// Normal message (batch acknowledgment) - forward to supervisor
			s.signalBatchAck(streamInfo, msg)
			continue
		}

		// Clean inbound close (server OK in trailers): signal stream failure
		if errors.Is(err, io.EOF) {
			log.Warnf("Worker %s: Stream closed by server", s.workerID)
			s.signalStreamFailure(streamInfo, "server_eof")
			return
		}

		// Local cancel/deadline (supervisor rotated, worker shutdown): just exit
		ctxErr := streamInfo.ctx.Err()
		if errors.Is(ctxErr, context.Canceled) || errors.Is(ctxErr, context.DeadlineExceeded) {
			log.Infof("Worker %s: Stream context cancelled, receiver exiting", s.workerID)
			return
		}

		// Stream-level gRPC status (non-OK):
		// RPC is over → signal stream failure or handle as irrecoverable
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.Unauthenticated, codes.PermissionDenied:
				s.handleIrrecoverableError("auth/perm: "+st.Message(), streamInfo)
				return
			case codes.InvalidArgument, codes.FailedPrecondition, codes.OutOfRange, codes.Unimplemented:
				s.handleIrrecoverableError("protocol: "+st.Message(), streamInfo)
				return
			default:
				// All other non-OK statuses: signal stream failure
				log.Warnf("Worker %s: gRPC error (code %v): %v", s.workerID, st.Code(), err)
				s.signalStreamFailure(streamInfo, "recv_error_"+st.Code().String())
				return
			}
		}

		// Transport error without status (RST/GOAWAY/TLS, socket close): signal stream failure
		log.Warnf("Worker %s: Transport error: %v", s.workerID, err)
		s.signalStreamFailure(streamInfo, "transport_error")
		return
	}
}

// signalStreamFailure signals the supervisor to rotate the stream
func (s *streamWorker) signalStreamFailure(streamInfo *streamInfo, reason string) {
	tlmWorkerStreamErrors.Inc(s.workerID, reason)

	// This signaling is blocking by design, it's okay to block the sender/receiver,
	// since the only way we get here is through a stream error.
	select {
	case s.streamFailureCh <- streamInfo:
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
func (s *streamWorker) handleIrrecoverableError(reason string, streamInfo *streamInfo) {
	// Currently this is treated as stream error, which will trigger a stream rotation
	// and retry of the same payload, which loops on. this IS NOT the desired behavior.
	// TODO: Implement proper handling of irrecoverable errors, by blocking the ingestion
	log.Infof("Worker %s: irrecoverable error detected: %s", s.workerID, reason)
	s.signalStreamFailure(streamInfo, "irrecoverable_error")
}

// getNextBatch crafts a StatefulBatch with the next batch to send from the
// inflight tracker. It doesn't change inflight tracker state
func (s *streamWorker) getNextBatch() *statefulpb.StatefulBatch {
	return createBatch(s.inflight.nextToSend().Encoded, s.inflight.nextBatchID())
}

// createBatch creates a StatefulBatch from serialized data and batch ID
func createBatch(data []byte, batchID uint32) *statefulpb.StatefulBatch {
	return &statefulpb.StatefulBatch{
		BatchId: batchID,
		Data:    data,
	}
}

// createStoppedTimer creates a timer that is stopped and has its channel drained
func createStoppedTimer(clk clock.Clock, d time.Duration) *clock.Timer {
	t := clk.Timer(d)
	if !t.Stop() {
		<-t.C
	}
	return t
}
