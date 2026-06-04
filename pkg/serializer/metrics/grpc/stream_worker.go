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

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// streamWorker implements the gRPC bidirectional streaming protocol for one
// stateful metrics stream using a master-2-slave threading model: one
// supervisor goroutine + one sender goroutine + one receiver goroutine.
//
// This is a metric-side adaptation of pkg/logs/sender/grpc/stream_worker.go.
// The protocol mechanics (state machine, stream lifetime, drain semantics,
// backoff, stale-signal handling) are identical because the gRPC layer is
// payload-agnostic. The differences from the logs version are:
//
//   - Payload type:    *Payload (metric)         vs *message.Payload (logs)
//   - Batch envelope:  MetricStatefulBatch       vs StatefulBatch
//   - Batch status:    MetricBatchStatus         vs BatchStatus
//   - Service client:  StatefulMetricsServiceClient
//   - No auditor sink — on ack we just bump telemetry counters and let the
//     payload be GC'd. There is no equivalent of logs' auditor for metrics
//     in the PoC.
//   - No lazy snapshot — getSnapshot() always dumps the full dict. The
//     sender path is just `payload.Encoded`, no per-payload prefix injection.

const (
	ioChanBufferSize  = 10
	connectionTimeout = 10 * time.Second
	drainTimeout      = 5 * time.Second
)

// streamState represents the current state of the stream worker.
//
//go:generate stringer -type=streamState
type streamState int

const (
	disconnected streamState = iota
	connecting
	active
	draining
)

// String returns a human-readable name for the streamState (stringer-equivalent,
// manually written to avoid the go:generate dependency for this small enum).
func (s streamState) String() string {
	switch s {
	case disconnected:
		return "disconnected"
	case connecting:
		return "connecting"
	case active:
		return "active"
	case draining:
		return "draining"
	default:
		return "unknown"
	}
}

// streamInfo bundles a gRPC stream with its lifetime context. Stale signals
// from old streams are filtered by comparing pointer identity (s.currentStream
// must equal the signal's streamInfo).
type streamInfo struct {
	stream statefulpb.StatefulMetricsService_MetricsStreamClient
	ctx    context.Context
	cancel context.CancelFunc
}

// streamCreationResult is the result of an async stream-creation attempt.
type streamCreationResult struct {
	info *streamInfo
	err  error
}

// batchAck wraps a batch acknowledgment with stream identity to prevent
// stale signals from old streams being processed as if for the current one.
type batchAck struct {
	stream *streamInfo
	status *statefulpb.MetricBatchStatus
}

// streamWorker manages a single gRPC stream's lifecycle. One supervisor
// goroutine owns all state; sender + receiver goroutines are stateless
// I/O wrappers.
type streamWorker struct {
	laneID string

	inputChan chan *Payload // buffered payloads to send (from encoder)

	// gRPC connection and per-stream state
	parentCtx       context.Context
	conn            *grpc.ClientConn
	client          statefulpb.StatefulMetricsServiceClient
	currentStream   *streamInfo
	streamState     streamState
	streamFailureCh chan *streamInfo
	streamReadyCh   chan streamCreationResult
	streamLifetime  time.Duration
	streamTimer     *clock.Timer
	drainTimer      *clock.Timer
	backoffTimer    *clock.Timer

	// IO goroutines
	batchToSendCh chan *statefulpb.MetricStatefulBatch
	batchAckCh    chan *batchAck

	// Inflight bookkeeping
	inflight *inflightTracker

	// Retry backoff
	backoffPolicy backoff.Policy
	nbErrors      int

	// Compression for the snapshot batch sent on every stream rotation.
	// Steady-state batches are compressed by the encoder (the outer
	// MetricDatumSequence layer in payloadsBuilderV3Stateful.submit) before
	// they hit the sender, so payload.Encoded already matches the stream's
	// dd-content-encoding header. The snapshot is built inside this
	// streamWorker on rotation (from inflight.getSnapshot) and bypasses the
	// encoder — so it must be compressed here to match the same header,
	// otherwise intake's per-stream decompressor sees garbage and log+drops
	// the snapshot, leaving the new stream's dict empty.
	compression compression.Compressor

	// Shutdown
	stopChan chan struct{}
	done     chan struct{}

	clock clock.Clock
}

// streamWorkerConfig bundles the (small) per-worker configuration.
type streamWorkerConfig struct {
	LaneID         string
	InputChan      chan *Payload
	ParentContext  context.Context
	Conn           *grpc.ClientConn
	Client         statefulpb.StatefulMetricsServiceClient
	StreamLifetime time.Duration
	MaxInflight    int
	Backoff        backoff.Policy
	// Compression must match the stream's dd-content-encoding (see comment
	// on streamWorker.compression). Required; nil is a programming error.
	Compression compression.Compressor
}

// newStreamWorker constructs a streamWorker. Caller must subsequently call
// start() to spin up the supervisor goroutine.
func newStreamWorker(cfg streamWorkerConfig) *streamWorker {
	return newStreamWorkerWithClock(cfg, clock.New(), nil)
}

// newStreamWorkerWithClock is the testing-friendly variant.
func newStreamWorkerWithClock(cfg streamWorkerConfig, clk clock.Clock, infl *inflightTracker) *streamWorker {
	if infl == nil {
		infl = newInflightTracker(cfg.LaneID, cfg.MaxInflight)
	}
	return &streamWorker{
		laneID:          cfg.LaneID,
		inputChan:       cfg.InputChan,
		parentCtx:       cfg.ParentContext,
		conn:            cfg.Conn,
		client:          cfg.Client,
		streamState:     disconnected,
		streamFailureCh: make(chan *streamInfo),
		batchAckCh:      make(chan *batchAck, ioChanBufferSize),
		streamReadyCh:   make(chan streamCreationResult),
		streamLifetime:  cfg.StreamLifetime,
		inflight:        infl,
		backoffPolicy:   cfg.Backoff,
		compression:     cfg.Compression,
		stopChan:        make(chan struct{}),
		done:            make(chan struct{}),
		clock:           clk,
		streamTimer:     createStoppedTimer(clk, 0),
		backoffTimer:    createStoppedTimer(clk, 0),
		drainTimer:      createStoppedTimer(clk, 0),
	}
}

// start kicks off the supervisor goroutine and triggers an async stream
// creation. Non-blocking.
func (s *streamWorker) start() {
	log.Infof("Starting stateful metrics stream worker %s", s.laneID)
	go s.supervisorLoop()
	s.asyncCreateNewStream()
}

// stop shuts down the worker. Blocks until the supervisor exits.
func (s *streamWorker) stop() {
	log.Infof("Stopping stateful metrics stream worker %s", s.laneID)
	close(s.stopChan)
	<-s.done
	log.Infof("Stateful metrics stream worker %s stopped", s.laneID)
}

// supervisorLoop is the sole owner of state for this worker. All decisions
// happen here; sender + receiver goroutines only do I/O and report results.
func (s *streamWorker) supervisorLoop() {
	defer close(s.done)

	// supervisorLoop starts disconnected; start() calls asyncCreateNewStream
	// concurrently so we'll transition to connecting shortly.
	s.streamState = connecting

	for {
		// Backpressure: only read input when inflight has capacity.
		var inputChan <-chan *Payload
		if s.inflight.hasSpace() {
			inputChan = s.inputChan
		}

		// Sending: only enabled when active with unsent payloads.
		var nextBatch *statefulpb.MetricStatefulBatch
		var sendChan chan<- *statefulpb.MetricStatefulBatch
		if s.streamState == active && s.inflight.hasUnSent() {
			sendChan = s.batchToSendCh
			nextBatch = s.getNextBatch()
		}

		select {
		case payload := <-inputChan:
			s.inflight.append(payload)

		case sendChan <- nextBatch:
			s.inflight.markSent()

		case ack := <-s.batchAckCh:
			s.handleBatchAck(ack)

		case failedStream := <-s.streamFailureCh:
			s.handleStreamFailure(failedStream)

		case result := <-s.streamReadyCh:
			s.handleStreamReady(result)

		case <-s.streamTimer.C:
			s.handleStreamTimeout()

		case <-s.drainTimer.C:
			s.handleDrainTimeout()

		case <-s.backoffTimer.C:
			s.handleBackoffTimeout()

		case <-s.stopChan:
			s.handleShutdown()
			return
		}
	}
}

func (s *streamWorker) handleBatchAck(ack *batchAck) {
	// Ignore stale acks from old streams.
	if ack.stream != s.currentStream {
		return
	}
	receivedBatchID := ack.status.BatchId

	// batch_id=0 is the snapshot ack — no payload to pop. Receiver has
	// reset its state machine; we're good to proceed with batch 1+.
	if receivedBatchID == snapshotBatchID {
		return
	}

	if !s.inflight.hasUnacked() {
		log.Errorf("Lane %s: received ack for batch %d but no sent payloads in inflight; terminating stream",
			s.laneID, receivedBatchID)
		tlmStreamErrors.Inc(s.laneID, "received_ack_but_no_sent_payloads")
		s.tryBeginStreamRotation(true)
		return
	}

	expectedBatchID := s.inflight.getHeadBatchID()
	if receivedBatchID != expectedBatchID {
		log.Errorf("Lane %s: batchID mismatch (expected %d, got %d); out-of-order or duplicate ack, terminating stream",
			s.laneID, expectedBatchID, receivedBatchID)
		tlmStreamErrors.Inc(s.laneID, "batch_id_mismatch")
		s.tryBeginStreamRotation(true)
		return
	}

	if receivedBatchID == firstRealBatchID {
		// First real batch acked → stream is proven operational; reset backoff.
		s.nbErrors = s.backoffPolicy.DecError(s.nbErrors)
	}

	// Pop the acked payload. inflight.pop applies StateChanges to snapshot.
	payload := s.inflight.pop()
	if payload != nil {
		// Telemetry on successful delivery.
		tlmBytesSent.Add(float64(len(payload.Encoded)), s.laneID)
		tlmPreCompressionBytesSent.Add(float64(payload.PreCompressionBytes), s.laneID)
	}

	// If draining and all unacked drained, rotate now.
	if s.streamState == draining && !s.inflight.hasUnacked() {
		log.Infof("Lane %s: all acks received during drain, rotating", s.laneID)
		s.drainTimer.Stop()
		s.tryBeginStreamRotation(false)
	}
}

func (s *streamWorker) handleStreamFailure(failedStream *streamInfo) {
	if failedStream != s.currentStream || (s.streamState != active && s.streamState != draining) {
		return
	}
	log.Infof("Lane %s: sender/receiver reported failure (state: %v), terminating stream",
		s.laneID, s.streamState)
	s.tryBeginStreamRotation(true)
}

func (s *streamWorker) handleStreamReady(result streamCreationResult) {
	if s.streamState != connecting {
		return
	}
	if result.err != nil {
		log.Warnf("Lane %s: stream creation failed: %v", s.laneID, result.err)
		tlmStreamErrors.Inc(s.laneID, "stream_creation_failed")
		s.tryBeginStreamRotation(true)
		return
	}
	s.finishStreamRotation(result.info)
}

func (s *streamWorker) handleStreamTimeout() {
	if s.streamState != active {
		return
	}
	if s.inflight.hasUnacked() {
		log.Infof("Lane %s: stream lifetime expired with %d unacked; draining",
			s.laneID, s.inflight.sentCount())
		s.streamState = draining
		s.drainTimer.Reset(drainTimeout)
	} else {
		log.Infof("Lane %s: stream lifetime expired with no unacked; rotating", s.laneID)
		tlmRotationCount.Inc(s.laneID, "lifetime_no_unacked")
		s.tryBeginStreamRotation(false)
	}
}

func (s *streamWorker) handleDrainTimeout() {
	if s.streamState != draining {
		return
	}
	log.Warnf("Lane %s: drain timeout expired, rotating (may retransmit unacked)", s.laneID)
	tlmRotationCount.Inc(s.laneID, "drain_timeout")
	s.tryBeginStreamRotation(false)
}

func (s *streamWorker) handleBackoffTimeout() {
	if s.streamState != disconnected {
		return
	}
	log.Infof("Lane %s: backoff expired, retrying stream (error count: %d)",
		s.laneID, s.nbErrors)
	s.streamState = connecting
	s.asyncCreateNewStream()
}

func (s *streamWorker) handleShutdown() {
	log.Infof("Lane %s: shutting down", s.laneID)
	s.streamTimer.Stop()
	s.backoffTimer.Stop()
	s.drainTimer.Stop()
	if s.batchToSendCh != nil {
		close(s.batchToSendCh)
	}
	s.closeStream(s.currentStream)
}

// tryBeginStreamRotation closes the current stream (if any) and either kicks
// off a new connection immediately (if not a failure-driven rotation, or
// backoff is 0) or sets up the disconnected→backoffTimer state.
func (s *streamWorker) tryBeginStreamRotation(dueToFailure bool) {
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
		tlmRotationCount.Inc(s.laneID, "failure")
	}

	if !dueToFailure || backoffDuration == 0 {
		log.Infof("Lane %s: beginning stream creation (state: %v → connecting)",
			s.laneID, s.streamState)
		s.streamState = connecting
		s.asyncCreateNewStream()
	} else {
		log.Infof("Lane %s: backing off stream creation for %v (error count: %d)",
			s.laneID, backoffDuration, s.nbErrors)
		s.streamState = disconnected
		s.backoffTimer.Reset(backoffDuration)
	}
}

// finishStreamRotation activates the newly-created stream:
//  1. Stand up the per-stream send channel and spin up sender/receiver goroutines.
//  2. Reset inflight (un-acked → un-sent; batch IDs reset to 0/1).
//  3. Send batch_id=0 with the full snapshot (non-lazy per contract.md D6).
func (s *streamWorker) finishStreamRotation(streamInfo *streamInfo) {
	log.Infof("Lane %s: finishing stream rotation (state: connecting → active)", s.laneID)

	batchToSendCh := make(chan *statefulpb.MetricStatefulBatch, ioChanBufferSize)
	s.currentStream = streamInfo
	s.streamState = active
	s.batchToSendCh = batchToSendCh

	go s.senderLoop(streamInfo, batchToSendCh)
	go s.receiverLoop(streamInfo)

	s.streamTimer.Reset(s.streamLifetime)
	s.inflight.resetOnRotation()

	// Send the full snapshot as batch_id=0. The supervisor loop's
	// sendChan-gated branch will pick up subsequent buffered payloads
	// with batch_id starting from 1.
	serialized, err := s.inflight.getSnapshot()
	if err != nil {
		log.Errorf("Lane %s: failed to serialize snapshot: %v", s.laneID, err)
	} else if serialized != nil {
		// The stream's dd-content-encoding header (set once at stream open)
		// tells intake to unconditionally decompress every message on the
		// stream. Steady-state batches arrive at the sender already
		// compressed by the encoder; the snapshot is built here and bypasses
		// the encoder, so we must compress it ourselves to match the header.
		compressed, cerr := s.compression.Compress(serialized)
		if cerr != nil {
			log.Errorf("Lane %s: failed to compress snapshot: %v — skipping snapshot send (new stream's dict will be empty until refs resolve)",
				s.laneID, cerr)
		} else {
			log.Infof("Lane %s: sending snapshot batch_id=%d (uncompressed=%d B, compressed=%d B)",
				s.laneID, snapshotBatchID, len(serialized), len(compressed))
			s.batchToSendCh <- &statefulpb.MetricStatefulBatch{
				BatchId: snapshotBatchID,
				Data:    compressed,
			}
		}
	} else {
		log.Infof("Lane %s: skipping snapshot batch_id=0 (dict is empty — first stream)",
			s.laneID)
	}
}

// asyncCreateNewStream creates a new gRPC stream in a goroutine. Reports
// outcome via streamReadyCh. Can block up to connectionTimeout while
// waiting for the underlying gRPC conn to become Ready.
func (s *streamWorker) asyncCreateNewStream() {
	go func() {
		log.Infof("Lane %s: async stream creation start", s.laneID)
		tlmStreamsOpened.Inc(s.laneID)

		var result streamCreationResult
		err := s.ensureConnectionReady()
		if err != nil {
			log.Errorf("Lane %s: stream creation failed (connection): %v", s.laneID, err)
			result = streamCreationResult{err: err}
		} else {
			streamCtx, streamCancel := context.WithCancel(s.parentCtx)
			stream, err := s.client.MetricsStream(streamCtx)
			if err != nil {
				streamCancel()
				log.Errorf("Lane %s: stream creation failed: %v", s.laneID, err)
				result = streamCreationResult{err: err}
			} else {
				result = streamCreationResult{info: &streamInfo{
					stream: stream,
					ctx:    streamCtx,
					cancel: streamCancel,
				}}
			}
		}

		select {
		case s.streamReadyCh <- result:
		case <-s.stopChan:
			if result.info != nil {
				s.closeStream(result.info)
			}
		}
	}()
}

func (s *streamWorker) ensureConnectionReady() error {
	if s.conn == nil {
		return nil // testing path with mock client
	}
	connCtx, cancel := context.WithTimeout(s.parentCtx, connectionTimeout)
	defer cancel()
	s.conn.Connect()
	for {
		state := s.conn.GetState()
		switch state {
		case connectivity.Ready:
			return nil
		case connectivity.Shutdown:
			return errors.New("gRPC conn is shutdown")
		}
		if !s.conn.WaitForStateChange(connCtx, state) {
			return connCtx.Err()
		}
	}
}

func (s *streamWorker) closeStream(info *streamInfo) {
	if info != nil {
		info.cancel()
	}
}

// senderLoop pulls batches off batchToSendCh and pushes them onto the gRPC
// stream. Exits when the channel is closed (clean shutdown) or Send returns
// an error (signals failure to supervisor).
func (s *streamWorker) senderLoop(info *streamInfo, batchToSendCh chan *statefulpb.MetricStatefulBatch) {
	for batch := range batchToSendCh {
		if err := info.stream.Send(batch); err != nil {
			ctxErr := info.ctx.Err()
			if errors.Is(ctxErr, context.Canceled) || errors.Is(ctxErr, context.DeadlineExceeded) {
				log.Infof("Lane %s: stream context cancelled, sender exiting", s.laneID)
				return
			}
			log.Warnf("Lane %s: Send failed: %v, terminating stream", s.laneID, err)
			s.signalStreamFailure(info, "send_err_"+status.Code(err).String())
			return
		}
		tlmBytesSent.Add(float64(proto.Size(batch)), s.laneID)
	}
	log.Infof("Lane %s: send channel closed, sender exiting", s.laneID)
}

// receiverLoop pulls acks off the gRPC stream and forwards them to the
// supervisor via batchAckCh. Maps gRPC stream errors to either irrecoverable
// (auth/perm/protocol) or recoverable (retry) failure signals.
func (s *streamWorker) receiverLoop(info *streamInfo) {
	for {
		msg, err := info.stream.Recv()
		if err == nil {
			s.signalBatchAck(info, msg)
			continue
		}
		if errors.Is(err, io.EOF) {
			log.Warnf("Lane %s: stream closed by server", s.laneID)
			s.signalStreamFailure(info, "server_eof")
			return
		}
		ctxErr := info.ctx.Err()
		if errors.Is(ctxErr, context.Canceled) || errors.Is(ctxErr, context.DeadlineExceeded) {
			log.Infof("Lane %s: stream context cancelled, receiver exiting", s.laneID)
			return
		}
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.Unauthenticated, codes.PermissionDenied:
				s.handleIrrecoverableError("auth/perm: "+st.Message(), info)
				return
			case codes.InvalidArgument, codes.FailedPrecondition, codes.OutOfRange, codes.Unimplemented:
				s.handleIrrecoverableError("protocol: "+st.Message(), info)
				return
			default:
				log.Warnf("Lane %s: gRPC error (code %v): %v", s.laneID, st.Code(), err)
				s.signalStreamFailure(info, "recv_error_"+st.Code().String())
				return
			}
		}
		log.Warnf("Lane %s: transport error: %v", s.laneID, err)
		s.signalStreamFailure(info, "transport_error")
		return
	}
}

func (s *streamWorker) signalStreamFailure(info *streamInfo, reason string) {
	tlmStreamErrors.Inc(s.laneID, reason)
	select {
	case s.streamFailureCh <- info:
	case <-s.stopChan:
	}
}

func (s *streamWorker) signalBatchAck(info *streamInfo, msg *statefulpb.MetricBatchStatus) {
	select {
	case s.batchAckCh <- &batchAck{stream: info, status: msg}:
	case <-s.stopChan:
	}
}

// handleIrrecoverableError logs and signals a stream failure. PoC treats
// auth/protocol errors as stream errors (retry loop). Future work: block
// ingestion until config is fixed.
func (s *streamWorker) handleIrrecoverableError(reason string, info *streamInfo) {
	log.Infof("Lane %s: irrecoverable error: %s", s.laneID, reason)
	s.signalStreamFailure(info, "irrecoverable_error")
}

// getNextBatch builds a MetricStatefulBatch wrapping the next-to-send
// payload's encoded bytes. PoC has no lazy prefix injection — bytes go
// straight onto the wire.
func (s *streamWorker) getNextBatch() *statefulpb.MetricStatefulBatch {
	payload := s.inflight.nextToSend()
	if payload == nil {
		return nil
	}
	bid := s.inflight.nextBatchID()
	// Diagnostic: log the first real batch_id per stream lifetime so we can
	// verify resetOnRotation is correctly resetting the counter. Once
	// Bug C is fully diagnosed this can be downgraded to Debug.
	if bid == firstRealBatchID {
		log.Infof("Lane %s: sending first real batch on this stream — batch_id=%d, encoded=%d B",
			s.laneID, bid, len(payload.Encoded))
	}
	return &statefulpb.MetricStatefulBatch{
		BatchId: bid,
		Data:    payload.Encoded,
	}
}

func createStoppedTimer(clk clock.Clock, d time.Duration) *clock.Timer {
	t := clk.Timer(d)
	if !t.Stop() {
		<-t.C
	}
	return t
}
