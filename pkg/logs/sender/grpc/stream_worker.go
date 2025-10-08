// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TODO For PoC Stage 1
// - handle unrecoverable errors - auth/perm, protocol, stream-level gRPC status
// - check snapshot's generationID, only act on the current generation
// - implementback-off from stream creation on successive failures
// - handle createNewStream failures
// - telemetries (send/recv, failure, rotations)

// TODO for PoC Stage 2
// - implement backpressure

// RotationType represents the type of stream rotation
type RotationType int

const (
	// RotationTypeNone is the initial state, no rotation has happened yet
	RotationTypeNone RotationType = iota
	// RotationTypeHard is used when a stream rotation is started due to an error
	RotationTypeHard
	// RotationTypeGraceful is used when stream rotation is started due to stream lifetime expiration
	RotationTypeGraceful
)

// StreamRotateSignal represents a signal to upstream about stream rotation
type StreamRotateSignal struct {
	Type         RotationType
	GenerationID uint64
}

// ReceiverSignal represents a signal from receiver to supervisor
type ReceiverSignal struct {
	GenerationID uint64
	Error        error
}

// StreamInfo holds all stream-related information
type StreamInfo struct {
	Stream statefulpb.StatefulLogsService_LogsStreamClient
	Ctx    context.Context
	Cancel context.CancelFunc
}

// StreamWorker manages a single gRPC bidirectional stream with Master-Slave threading model
// Architecture: One supervisor/sender goroutine + one receiver goroutine per worker
type StreamWorker struct {
	// Configuration
	workerID            string
	destinationsContext *client.DestinationsContext

	// Pipeline integration
	inputChan  chan *message.Payload
	outputChan chan *message.Payload // For auditor acknowledgments
	sink       sender.Sink           // For getting auditor channel

	// gRPC connection management (shared with other streams)
	client statefulpb.StatefulLogsServiceClient

	// Stream management
	currentStream  *StreamInfo
	generationID   uint64
	generationMu   sync.RWMutex        // Protects generationID
	recvFailureCh  chan ReceiverSignal // Signal receiver failure with generationID
	streamLifetime time.Duration
	batchIDCounter uint32

	// Rotation management
	inRotation    bool
	rotationType  RotationType
	drainedStream *StreamInfo // Old stream being drained after graceful rotation

	// Upstream signaling
	signalStreamRotate chan StreamRotateSignal

	// Auditor acknowledgment tracking
	pendingPayloads   map[uint32]*message.Payload // batchID -> payload
	pendingPayloadsMu sync.Mutex                  // Protects pendingPayloads map

	// Control
	stopChan chan struct{}
	done     chan struct{}
}

// NewStreamWorker creates a new gRPC stream worker
func NewStreamWorker(
	workerID string,
	destinationsCtx *client.DestinationsContext,
	client statefulpb.StatefulLogsServiceClient,
	sink sender.Sink,
	streamLifetime time.Duration,
) *StreamWorker {
	worker := &StreamWorker{
		workerID:            workerID,
		destinationsContext: destinationsCtx,
		inputChan:           make(chan *message.Payload, 100), // Buffered input
		outputChan:          nil,                              // Will be set in Start()
		sink:                sink,                             // For getting auditor channel
		client:              client,
		recvFailureCh:       make(chan ReceiverSignal), // Unbuffered receiver failure signal
		streamLifetime:      streamLifetime,            // Stream recreation interval
		inRotation:          false,
		rotationType:        RotationTypeNone,
		signalStreamRotate:  make(chan StreamRotateSignal, 1),  // Size-1 buffer for drop-old semantics
		pendingPayloads:     make(map[uint32]*message.Payload), // Initialize batch tracking map
		stopChan:            make(chan struct{}),
		done:                make(chan struct{}),
	}

	return worker
}

// Start begins the supervisor goroutine
func (s *StreamWorker) Start() {
	log.Infof("Starting gRPC stream worker %s", s.workerID)
	s.outputChan = s.sink.Channel()

	// Create initial stream
	if stream, err := s.createNewStream(); err == nil {
		s.currentStream = stream
		currentGen := s.GetGenerationID()
		log.Infof("Worker %s: Created initial stream (generation %d)", s.workerID, currentGen)
		go s.receiverLoop(stream, currentGen)
	} else {
		log.Errorf("Worker %s: Failed to create initial stream: %v", s.workerID, err)
	}

	// Start supervisor/sender goroutine (master)
	go s.supervisorLoop()
}

// Stop gracefully shuts down the stream
func (s *StreamWorker) Stop() {
	log.Infof("Stopping gRPC stream worker %s", s.workerID)
	close(s.stopChan)
	<-s.done
	log.Infof("Worker %s: Stopped", s.workerID)
}

// supervisorLoop is the master goroutine that handles sending and stream lifecycle
func (s *StreamWorker) supervisorLoop() {
	defer close(s.done)

	streamTimer := time.NewTimer(s.streamLifetime)
	defer streamTimer.Stop()

	for {
		select {
		case payload := <-s.inputChan:
			if s.inRotation && s.rotationType == RotationTypeHard {
				// In hard rotation state
				if payload.IsSnapshot {
					// Received full state snapshot from upstream, ready to
					// start using the new stream now, with the snapshot as
					// the first message
					s.finishHardRotate(streamTimer)
				} else {
					// These are the messages that's written into channel
					// buffer but encoded with previous state, we can't send them
					// as-is into the new stream. Dropping them and rely on
					// upstream to re-encode and resend them
					continue
				}
			} else if s.inRotation && s.rotationType == RotationTypeGraceful {
				// In graceful rotation state
				if payload.IsSnapshot {
					// Received full state snapshot from upstream, ready to
					// switch to the new stream
					s.finishGracefulRotate(streamTimer)
				}
				// If payload is not a snapshot, we continuously send them to
				// the old stream.
			}

			// Send payload
			if err := s.sendPayload(payload); err != nil {
				// Send failed, hard rotate stream
				log.Warnf("Worker %s: Send failed, initiating hard rotation: %v", s.workerID, err)
				s.beginHardRotate()
			}

		case signal := <-s.recvFailureCh:
			currentGen := s.GetGenerationID()
			if signal.GenerationID != currentGen {
				// Signal from old stream generation, we must have rotated.
				// - In case of hard rotation, this is timing thing, old receiver is reporting
				//   the same transport failure as previously detected by the supervisor. since
				//   we have hard rotated, we can ignore the signal.
				// - In case of graceful rotation, this is the drained stream reporting the failure.
				//   since we've already switched to functioning new stream, we will ignore the signal.
				//   If there really were acks that we missed because drained stream died, we rely on
				//   the upstream to detect and resend them
				log.Infof("Worker %s: Ignoring signal from old generation %d (current: %d)",
					s.workerID, signal.GenerationID, currentGen)
				continue
			}

			// Receiver reported failure, hard rotate stream
			log.Warnf("Worker %s: Receiver reported failure, initiating hard rotation: %v", s.workerID, signal.Error)
			s.beginHardRotate()

		case <-streamTimer.C:
			// Life time expired, graceful rotate stream
			if !s.inRotation {
				log.Infof("Worker %s: Stream lifetime expired, initiating graceful rotation", s.workerID)
				s.beginGracefulRotate()
			}

		case <-s.stopChan:
			// Graceful shutdown
			if s.currentStream != nil {
				s.closeStream(s.currentStream)
			}
			s.closeStream(s.drainedStream)
			return
		}
	}
}

// sendStreamRotateSignal sends a rotation signal to upstream with size-1 drop-old semantics
// This ensures the supervisor never blocks and the upstream always gets the latest signal
func (s *StreamWorker) sendStreamRotateSignal(rt RotationType) {
	v := StreamRotateSignal{
		Type:         rt,
		GenerationID: s.GetGenerationID(),
	}
	select {
	case s.signalStreamRotate <- v:
		return // queued immediately
	default:
		// drop one old value if present
		select {
		case <-s.signalStreamRotate:
			// dropped old
		default:
			// nothing to drop (likely consumer grabbed it); try send again
		}
		s.signalStreamRotate <- v
	}
}

// beginHardRotate immediately closes and recreates the stream
func (s *StreamWorker) beginHardRotate() {
	currentGen := s.GetGenerationID()
	log.Infof("Worker %s: Beginning hard rotation (generation %d)", s.workerID, currentGen)

	// Signal "hard rotate" to upstream
	s.sendStreamRotateSignal(RotationTypeHard)

	// Close current stream
	s.closeStream(s.currentStream)
	s.currentStream = nil

	// Create new stream
	if streamInfo, err := s.createNewStream(); err == nil {
		s.currentStream = streamInfo
		// Start new receiver goroutine with new stream
		newGen := s.GetGenerationID()
		go s.receiverLoop(streamInfo, newGen)
	} else {
		log.Errorf("Worker %s: Failed to create new stream during hard rotation: %v", s.workerID, err)
	}

	// Set rotation state
	s.inRotation = true
	s.rotationType = RotationTypeHard
}

// finishHardRotate completes the hard rotation process
func (s *StreamWorker) finishHardRotate(streamTimer *time.Timer) {
	log.Infof("Worker %s: Hard rotation finished, resuming normal operation", s.workerID)
	s.inRotation = false
	s.rotationType = RotationTypeNone
	// Reset timer after successful rotation
	streamTimer.Reset(s.streamLifetime)
}

// beginGracefulRotate starts graceful rotation by signaling upstream
func (s *StreamWorker) beginGracefulRotate() {
	currentGen := s.GetGenerationID()
	log.Infof("Worker %s: Beginning graceful rotation (generation %d)", s.workerID, currentGen)

	// Signal "graceful rotate" to upstream
	s.sendStreamRotateSignal(RotationTypeGraceful)

	// Set rotation state
	s.inRotation = true
	s.rotationType = RotationTypeGraceful
}

// finishGracefulRotate completes graceful rotation by switching to new stream
func (s *StreamWorker) finishGracefulRotate(streamTimer *time.Timer) {
	log.Infof("Worker %s: Finishing graceful rotation", s.workerID)

	// Move current stream to drained
	s.drainedStream = s.currentStream
	s.currentStream = nil

	// Create new stream
	if streamInfo, err := s.createNewStream(); err == nil {
		s.currentStream = streamInfo
		newGen := s.GetGenerationID()
		log.Infof("Worker %s: Graceful rotation completed, new stream created (generation %d)", s.workerID, newGen)
		// Start new receiver goroutine with new stream
		go s.receiverLoop(streamInfo, newGen)
	} else {
		log.Errorf("Worker %s: Failed to create new stream during graceful rotation: %v", s.workerID, err)
	}

	// Start drain timer (10 seconds) - automatically closes drained stream when it expires
	drainedStreamToClose := s.drainedStream
	time.AfterFunc(10*time.Second, func() {
		log.Infof("Worker %s: Closing drained stream after 10 second grace period", s.workerID)
		s.closeStream(drainedStreamToClose)
	})

	// Reset rotation state
	s.inRotation = false
	s.rotationType = RotationTypeNone

	// Reset timer after successful rotation
	streamTimer.Reset(s.streamLifetime)
}

// GetGenerationID safely returns the current generation ID
func (s *StreamWorker) GetGenerationID() uint64 {
	s.generationMu.RLock()
	defer s.generationMu.RUnlock()
	return s.generationID
}

// createNewStream creates a new gRPC stream and returns StreamInfo
func (s *StreamWorker) createNewStream() (*StreamInfo, error) {
	// Increment generation for new stream
	s.generationMu.Lock()
	s.generationID++
	currentGen := s.generationID
	s.generationMu.Unlock()

	log.Infof("Worker %s: Creating new stream (generation %d)", s.workerID, currentGen)

	// Create per-stream context derived from destinations context
	ctx, cancel := context.WithCancel(s.destinationsContext.Context())

	// Create the stream (headers are added automatically via PerRPCCredentials)
	stream, err := s.client.LogsStream(ctx)
	if err != nil {
		cancel() // Clean up context on error
		log.Errorf("Worker %s: Failed to create gRPC stream (generation %d): %v", s.workerID, currentGen, err)
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	log.Infof("Worker %s: Successfully created gRPC stream (generation %d)", s.workerID, currentGen)
	return &StreamInfo{
		Stream: stream,
		Ctx:    ctx,
		Cancel: cancel,
	}, nil
}

// closeStream safely closes a stream and cancels its context
func (s *StreamWorker) closeStream(streamInfo *StreamInfo) {
	if streamInfo != nil {
		_ = streamInfo.Stream.CloseSend() // Per docs, this always returns nil
		streamInfo.Cancel()
	}
}

// sendPayload sends a payload through the current stream
func (s *StreamWorker) sendPayload(payload *message.Payload) error {
	if s.currentStream == nil {
		return fmt.Errorf("no active stream")
	}

	batch := s.payloadToBatch(payload)

	// Send the batch (headers were sent at stream creation time)
	if err := s.currentStream.Stream.Send(batch); err != nil {
		return fmt.Errorf("failed to send batch: %w", err)
	}

	// Track payload by batch ID for auditor acknowledgment when we receive BatchStatus
	s.pendingPayloadsMu.Lock()
	s.pendingPayloads[batch.BatchId] = payload
	s.pendingPayloadsMu.Unlock()

	return nil
}

// receiverLoop runs in the receiver goroutine to process server responses for a specific stream
// This goroutine exits when the stream fails and signals the supervisor
func (s *StreamWorker) receiverLoop(streamInfo *StreamInfo, generationID uint64) {
	stream := streamInfo.Stream
	for {
		msg, err := stream.Recv()
		if err == nil {
			// Normal message (e.g., BatchStatus)
			s.handleBatchStatus(msg)
			continue
		}

		// Clean inbound close (server OK in trailers): policy = signal receiver failure
		if errors.Is(err, io.EOF) {
			log.Warnf("Worker %s: Stream closed by server (generation %d)", s.workerID, generationID)
			s.signalRecvFailure(generationID, err)
			return // Exit this receiver goroutine
		}

		// Local cancel/deadline (supervisor rotated, worker shutdown): just exit
		if errors.Is(streamInfo.Ctx.Err(), context.Canceled) || errors.Is(streamInfo.Ctx.Err(), context.DeadlineExceeded) {
			log.Infof("Worker %s: Stream context cancelled, receiver exiting (generation %d)", s.workerID, generationID)
			return // Exit this receiver goroutine
		}

		// Stream-level gRPC status (non-OK): RPC is over → signal receiver failure or block terminal
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.Unauthenticated, codes.PermissionDenied:
				// Terminal until fixed; do not signal receiver failure here
				s.handleIrrecoverableError("auth/perm: " + st.Message())
				return // Exit this receiver goroutine
			case codes.InvalidArgument, codes.FailedPrecondition, codes.OutOfRange, codes.Unimplemented:
				// Terminal protocol/semantic issue; do not signal receiver failure
				s.handleIrrecoverableError("protocol: " + st.Message())
				return // Exit this receiver goroutine
			default:
				// All other non-OK statuses: signal receiver failure
				s.signalRecvFailure(generationID, err)
				return // Exit this receiver goroutine
			}
		}

		// Transport error without status (RST/GOAWAY/TLS, socket close): signal receiver failure
		log.Warnf("Worker %s: Transport error (generation %d): %v", s.workerID, generationID, err)
		s.signalRecvFailure(generationID, err)
		return // Exit this receiver goroutine
	}
}

// signalRecvFailure signals the supervisor to rotate the stream
func (s *StreamWorker) signalRecvFailure(generationID uint64, err error) {
	// Always signal with generation ID - supervisor will decide whether to act
	signal := ReceiverSignal{
		GenerationID: generationID,
		Error:        err,
	}

	// This signaling is blocking by design, it's okey to block the receiver,
	// since the only way we get here is through an irrecoverable error.
	// The stopChan is used to unblock the receiver when the worker is shutting down.
	select {
	case s.recvFailureCh <- signal:
	case <-s.stopChan:
	}
}

// handleIrrecoverableError blocks the receiver when encountering terminal errors
func (s *StreamWorker) handleIrrecoverableError(_ string) {
	// TODO: Implement proper blocking logic with exponential backoff and cancellable sleep
}

// handleBatchStatus processes a normal BatchStatus response
func (s *StreamWorker) handleBatchStatus(response *statefulpb.BatchStatus) {
	batchID := uint32(response.BatchId)

	// Find the specific payload for this batch ID
	s.pendingPayloadsMu.Lock()
	payload, exists := s.pendingPayloads[batchID]
	if exists {
		delete(s.pendingPayloads, batchID) // Clean up immediately while holding lock
	}
	s.pendingPayloadsMu.Unlock()

	if exists {
		if response.Status == statefulpb.BatchStatus_OK {
			// Handle acknowledgments - send successful payloads to auditor
			if s.outputChan != nil {
				select {
				case s.outputChan <- payload:
					// Successfully sent to auditor
				default:
					log.Warnf("Worker %s: Auditor channel full, dropping ack for batch %d", s.workerID, batchID)
				}
			}
		} else {
			log.Warnf("Worker %s: Received non-OK status for batch %d: %v", s.workerID, batchID, response.Status)
		}
	} else {
		log.Warnf("Worker %s: Received BatchStatus for unknown batch %d", s.workerID, batchID)
	}
}

// payloadToBatch converts a message payload to a StatefulBatch
// The payload.GRPCDatums contains array of *grpc.Datum objects
func (s *StreamWorker) payloadToBatch(payload *message.Payload) *statefulpb.StatefulBatch {
	s.batchIDCounter++
	batchID := s.batchIDCounter

	batch := &statefulpb.StatefulBatch{
		BatchId: batchID,
		Data:    make([]*statefulpb.Datum, 0, payload.Count()),
	}

	// Use the GRPCData array from the payload (much cleaner!)
	if payload.GRPCData != nil {
		batch.Data = append(batch.Data, payload.GRPCData...)
	}

	return batch
}
