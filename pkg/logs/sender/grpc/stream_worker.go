// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
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
	RotationTypeNone RotationType = iota
	RotationTypeHard
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
	Stream StatefulLogsService_LogsStreamClient
	Ctx    context.Context
	Cancel context.CancelFunc
}

// StreamWorker manages a single gRPC bidirectional stream with Master-Slave threading model
// Architecture: One supervisor/sender goroutine + one persistent receiver goroutine per worker
type StreamWorker struct {
	// Configuration
	workerID            string
	destinationsContext *client.DestinationsContext

	// Pipeline integration
	inputChan  chan *message.Payload
	outputChan chan *message.Payload // For auditor acknowledgments
	sink       sender.Sink           // For getting auditor channel

	// gRPC connection management (shared with other streams)
	client StatefulLogsServiceClient

	// Stream management
	currentStream  *StreamInfo
	generationID   uint64
	recvFailureCh  chan ReceiverSignal // Signal receiver failure with generationID
	streamLifetime time.Duration
	batchIDCounter *atomic.Uint32 // Shared across all workers for global uniqueness

	// Rotation management
	inRotation    bool
	rotationType  RotationType
	drainedStream *StreamInfo // Old stream being drained after graceful rotation

	// Upstream signaling
	signalStreamRotate chan StreamRotateSignal

	// Auditor acknowledgment tracking
	pendingPayloads   map[uint32]*message.Payload // batchID -> payload
	pendingPayloadsMu sync.Mutex                  // Protects pendingPayloads map

	// Poor man's snapshot: cache pattern payloads for re-sending after rotation
	patternCache   []*message.Payload // Recently sent pattern define/update payloads
	patternCacheMu sync.Mutex         // Protects patternCache

	// Control
	stopChan chan struct{}
	done     chan struct{}
}

// NewStreamWorker creates a new gRPC stream worker
func NewStreamWorker(
	workerID string,
	destinationsCtx *client.DestinationsContext,
	client StatefulLogsServiceClient,
	sink sender.Sink,
	streamLifetime time.Duration,
	batchIDCounter *atomic.Uint32,
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
		batchIDCounter:      batchIDCounter,            // Shared counter for globally unique batch IDs
		inRotation:          false,
		rotationType:        RotationTypeNone,
		signalStreamRotate:  make(chan StreamRotateSignal, 1),  // Size-1 buffer for drop-old semantics
		pendingPayloads:     make(map[uint32]*message.Payload), // Initialize batch tracking map
		patternCache:        make([]*message.Payload, 0, 100),  // Cache for pattern payloads
		stopChan:            make(chan struct{}),
		done:                make(chan struct{}),
	}

	return worker
}

// Start begins the supervisor goroutine
func (s *StreamWorker) Start() {
	log.Infof("🚀 ========== Starting gRPC stream worker %s ==========", s.workerID)
	s.outputChan = s.sink.Channel()
	log.Infof("🔌 Worker %s: outputChan configured: %v", s.workerID, s.outputChan != nil)

	// Create initial stream
	log.Infof("🔧 Worker %s: Creating initial gRPC stream...", s.workerID)
	if stream, err := s.createNewStream(); err == nil {
		s.currentStream = stream
		log.Infof("✅ Worker %s: Created initial stream (generation %d)", s.workerID, s.generationID)
		log.Infof("🔄 Worker %s: Starting receiverLoop goroutine (generation %d)", s.workerID, s.generationID)
		go s.receiverLoop(stream, s.generationID)
	} else {
		log.Errorf("❌ Worker %s: Failed to create initial stream: %v", s.workerID, err)
	}

	// Start supervisor/sender goroutine (master)
	log.Infof("👷 Worker %s: Starting supervisorLoop goroutine", s.workerID)
	go s.supervisorLoop()
	log.Infof("🚀 Worker %s: Start() complete", s.workerID)
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
				// If payload is not a snapshot, we continously send them to
				// the old stream.
			}

			// Send payload
			if err := s.sendPayload(payload); err != nil {
				// Send failed, hard rotate stream
				log.Warnf("Worker %s: Send failed, initiating hard rotation: %v", s.workerID, err)
				s.beginHardRotate()
			}

		case signal := <-s.recvFailureCh:
			if signal.GenerationID != s.generationID {
				// Signal from old stream generation, we must have rotated.
				// - In case of hard rotation, this is timing thing, old receiver is reporting
				//   the same transport failure as previously detected by the supervisor. since
				//   we have hard rotated, we can ignore the signal.
				// - In case of graceful rotation, this is the drained stream reporting the failure.
				//   since we've already switched to functioning new stream, we will ignore the signal.
				//   If there really were acks that we missed because drained stream died, we rely on
				//   the upstream to detect and resend them
				log.Infof("Worker %s: Ignoring signal from old generation %d (current: %d)",
					s.workerID, signal.GenerationID, s.generationID)
				continue
			}

			// Receiver reported failure, hard rotate stream
			log.Warnf("Worker %s: Receiver reported failure, initiating hard rotation: %v", s.workerID, signal.Error)
			s.beginHardRotate()

		case <-streamTimer.C:
			// Life time expired, graceful rotate stream
			if !s.inRotation {
				log.Infof("⏰ ========== Worker %s: STREAM LIFETIME EXPIRED - GRACEFUL ROTATION ==========", s.workerID)
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
		GenerationID: s.generationID,
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
	log.Infof("Worker %s: Beginning hard rotation (generation %d)", s.workerID, s.generationID)

	// Signal "hard rotate" to upstream
	s.sendStreamRotateSignal(RotationTypeHard)

	// Close current stream
	s.closeStream(s.currentStream)
	s.currentStream = nil

	// Create new stream
	if streamInfo, err := s.createNewStream(); err == nil {
		s.currentStream = streamInfo
		// Start new receiver goroutine with new stream
		go s.receiverLoop(streamInfo, s.generationID)
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
	log.Infof("🔄 Worker %s: BEGIN GRACEFUL ROTATION (generation %d)", s.workerID, s.generationID)

	// Signal "graceful rotate" to upstream
	s.sendStreamRotateSignal(RotationTypeGraceful)
	log.Infof("📡 Worker %s: Sent graceful rotation signal to upstream", s.workerID)

	// Set rotation state
	s.inRotation = true
	s.rotationType = RotationTypeGraceful
	log.Infof("🔄 Worker %s: In graceful rotation mode, waiting for snapshot...", s.workerID)
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
		log.Infof("Worker %s: Graceful rotation completed, new stream created (generation %d)", s.workerID, s.generationID)
		// Start new receiver goroutine with new stream
		go s.receiverLoop(streamInfo, s.generationID)
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

// createNewStream creates a new gRPC stream and returns StreamInfo
func (s *StreamWorker) createNewStream() (*StreamInfo, error) {
	// Increment generation for new stream
	s.generationID++
	log.Infof("Worker %s: Creating new stream (generation %d)", s.workerID, s.generationID)

	// Create per-stream context derived from destinations context
	ctx, cancel := context.WithCancel(s.destinationsContext.Context())

	// Create the stream (headers are added automatically via PerRPCCredentials)
	stream, err := s.client.LogsStream(ctx)
	if err != nil {
		cancel() // Clean up context on error
		log.Errorf("Worker %s: Failed to create gRPC stream (generation %d): %v", s.workerID, s.generationID, err)
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	log.Infof("Worker %s: Successfully created gRPC stream (generation %d)", s.workerID, s.generationID)
	return &StreamInfo{
		Stream: stream,
		Ctx:    ctx,
		Cancel: cancel,
	}, nil
}

// closeStream safely closes a stream and cancels its context
func (s *StreamWorker) closeStream(streamInfo *StreamInfo) {
	if streamInfo != nil {
		streamInfo.Stream.CloseSend()
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
	// Removed debug log to reduce noise
	s.pendingPayloadsMu.Unlock()

	return nil
}

// receiverLoop runs in the receiver goroutine to process server responses for a specific stream
// This goroutine exits when the stream fails and signals the supervisor
func (s *StreamWorker) receiverLoop(streamInfo *StreamInfo, generationID uint64) {
	stream := streamInfo.Stream
	log.Infof("🔄 Worker %s: receiverLoop STARTED (generation %d)", s.workerID, generationID)
	recvCount := 0
	for {
		msg, err := stream.Recv()
		if err == nil {
			// Normal message (e.g., BatchStatus)
			recvCount++
			if recvCount%100 == 1 {
				log.Infof("✅ Worker %s: Received %d BatchStatus messages so far (latest: batch_id=%d)", s.workerID, recvCount, msg.BatchId)
			}
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
func (s *StreamWorker) handleIrrecoverableError(reason string) {
	// TODO: Implement proper blocking logic with exponential backoff and cancellable sleep
}

// handleBatchStatus processes a normal BatchStatus response
func (s *StreamWorker) handleBatchStatus(response *BatchStatus) {
	batchID := uint32(response.BatchId)

	// Debug: Print the full server response
	log.Debugf("🔵 Worker %s: SERVER RESPONSE: batch_id=%d, status=%v (enum=%d), full_response=%+v",
		s.workerID, response.BatchId, response.Status, int32(response.Status), response)

	// Find the specific payload for this batch ID
	s.pendingPayloadsMu.Lock()
	payload, exists := s.pendingPayloads[batchID]
	if exists {
		delete(s.pendingPayloads, batchID) // Clean up immediately while holding lock
	} else {
		log.Warnf("❌ Worker %s: Payload for batch_id=%d NOT FOUND in pendingPayloads (total pending: %d)", s.workerID, batchID, len(s.pendingPayloads))
	}
	s.pendingPayloadsMu.Unlock()

	if exists {
		if response.Status == BatchStatus_OK {
			// Update metrics for successful send
			metrics.LogsSent.Add(int64(payload.Count()))
			metrics.TlmLogsSent.Add(float64(payload.Count()))

			// Handle acknowledgments - send successful payloads to auditor
			if s.outputChan != nil {
				select {
				case s.outputChan <- payload:
					// Success - no log to reduce noise
				default:
					log.Warnf("Worker %s: Auditor channel full, dropping ack for batch %d", s.workerID, batchID)
				}
			} else {
				log.Warnf("❌ Worker %s: outputChan is nil, cannot send ack for batch_id=%d", s.workerID, batchID)
			}
		} else {
			log.Warnf("Worker %s: Received non-OK status for batch %d: %v", s.workerID, batchID, response.Status)
		}
	}
}

// payloadToBatch converts a message payload to a StatefulBatch
func (s *StreamWorker) payloadToBatch(payload *message.Payload) *StatefulBatch {
	batchID := s.batchIDCounter.Inc()

	batch := &StatefulBatch{
		BatchId: batchID,
		Data:    make([]*Datum, 0, payload.Count()),
	}

	// Check payload type by looking at metadata tags
	payloadType := ""
	for _, meta := range payload.MessageMetas {
		for _, tag := range meta.ProcessingTags {
			if tag == "data_type:pattern_define" || tag == "data_type:pattern_update" || tag == "data_type:log_with_pattern" {
				payloadType = tag
				break
			}
		}
		if payloadType != "" {
			break
		}
	}

	switch payloadType {
	case "data_type:pattern_define", "data_type:pattern_update":
		// Handle pattern change (define or update)
		datum, err := s.decodePatternDatum(payload)
		if err != nil {
			log.Errorf("Worker %s: Failed to decode pattern: %v", s.workerID, err)
		} else if datum != nil {
			batch.Data = append(batch.Data, datum)
			if payloadType == "data_type:pattern_define" {
				log.Infof("📤 Worker %s: Sending PatternDefine (ID=%d, template='%s')",
					s.workerID, datum.GetPatternDefine().PatternId, datum.GetPatternDefine().Template)
			} else {
				log.Infof("📤 Worker %s: Sending PatternUpdate (ID=%d, template='%s')",
					s.workerID, datum.GetPatternUpdate().PatternId, datum.GetPatternUpdate().NewTemplate)
			}
		}

	case "data_type:log_with_pattern":
		// Handle log with pattern reference + wildcard values
		datum, err := s.decodeLogDatum(payload)
		if err != nil {
			log.Errorf("Worker %s: Failed to decode log: %v", s.workerID, err)
		} else if datum != nil {
			batch.Data = append(batch.Data, datum)
			log.Debugf("📤 Worker %s: Sending Log with pattern_id=%d, %d wildcard values",
				s.workerID, datum.GetLogs().GetStructured().PatternId, len(datum.GetLogs().GetStructured().DynamicValues))
		}

	default:
		// Handle regular log payload (no pattern)
		datum := &Datum{
			Data: &Datum_Logs{
				Logs: &Log{
					Content: &Log_Raw{
						Raw: string(payload.Encoded), // Send compressed data as-is
					},
				},
			},
		}
		batch.Data = append(batch.Data, datum)
	}

	return batch
}

// PatternData matches the structure from dumb_strategy (sender package)
// We define it here to avoid import cycles
type PatternData struct {
	PatternID  uint64
	Template   string
	ParamCount uint32
	PosList    []uint32
	IsUpdate   bool
}

// LogData matches the structure from dumb_strategy (sender package)
type LogData struct {
	PatternID      uint64
	WildcardValues []string
	Timestamp      uint64
}

// decodePatternDatum decodes a pattern payload from gob format to protobuf (PatternDefine or PatternUpdate)
func (s *StreamWorker) decodePatternDatum(payload *message.Payload) (*Datum, error) {
	// Decode gob-encoded PatternData
	var patternData PatternData
	decoder := gob.NewDecoder(bytes.NewReader(payload.Encoded))

	if err := decoder.Decode(&patternData); err != nil {
		return nil, fmt.Errorf("failed to decode pattern data: %w", err)
	}

	// Convert to protobuf PatternDefine or PatternUpdate
	if patternData.IsUpdate {
		patternUpdate := &PatternUpdate{
			PatternId:   patternData.PatternID,
			NewTemplate: patternData.Template,
			ParamCount:  patternData.ParamCount,
			PosList:     patternData.PosList,
		}
		return &Datum{
			Data: &Datum_PatternUpdate{
				PatternUpdate: patternUpdate,
			},
		}, nil
	}

	patternDefine := &PatternDefine{
		PatternId:  patternData.PatternID,
		Template:   patternData.Template,
		ParamCount: patternData.ParamCount,
		PosList:    patternData.PosList,
	}
	return &Datum{
		Data: &Datum_PatternDefine{
			PatternDefine: patternDefine,
		},
	}, nil
}

// decodeLogDatum decodes a log payload from gob format to protobuf Log with StructuredLog
func (s *StreamWorker) decodeLogDatum(payload *message.Payload) (*Datum, error) {
	// Decode gob-encoded LogData
	var logData LogData
	decoder := gob.NewDecoder(bytes.NewReader(payload.Encoded))

	if err := decoder.Decode(&logData); err != nil {
		return nil, fmt.Errorf("failed to decode log data: %w", err)
	}

	// Convert wildcard values to DynamicValue protobuf
	dynamicValues := make([]*DynamicValue, len(logData.WildcardValues))
	for i, val := range logData.WildcardValues {
		dynamicValues[i] = &DynamicValue{
			Value: &DynamicValue_StringValue{
				StringValue: val,
			},
		}
	}

	// Create StructuredLog
	structuredLog := &StructuredLog{
		PatternId:     logData.PatternID,
		DynamicValues: dynamicValues,
	}

	// Wrap in Log and Datum
	return &Datum{
		Data: &Datum_Logs{
			Logs: &Log{
				Timestamp: logData.Timestamp,
				Content: &Log_Structured{
					Structured: structuredLog,
				},
			},
		},
	}, nil
}
