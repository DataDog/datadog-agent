// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package grpc

import (
	"time"

	"github.com/benbjohnson/clock"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	tlmDroppedTooLarge = telemetry.NewCounter("logs_sender_grpc_batch_strategy", "dropped_too_large", []string{"pipeline"}, "Number of payloads dropped due to being too large")
)

// enableDeltaEncoding controls whether the agent applies delta encoding to Log datums.
// Set to false until the server implements delta reconstruction (see PR #118018).
// When false: patternId, tags, and timestamp are sent as absolute values on every Log datum.
// TODO: re-enable once server-side delta decoding is implemented and protocol-version-gated.
const enableDeltaEncoding = true

// StatefulExtra holds state changes (non-Log datums) from a batch
// Used by inflight tracker to maintain snapshot state for stream rotation
type StatefulExtra struct {
	StateChanges        []*statefulpb.Datum
	PreCompressionBytes int
}

// isStateDatum returns true if the datum represents a state change
// (pattern/dict define/delete operations)
func isStateDatum(datum *statefulpb.Datum) bool {
	switch datum.Data.(type) {
	case *statefulpb.Datum_PatternDefine, *statefulpb.Datum_PatternDelete,
		*statefulpb.Datum_DictEntryDefine, *statefulpb.Datum_DictEntryDelete:
		return true
	default:
		return false
	}
}

// batchStrategy contains batching logic for gRPC sender without serializer
// It collects Datum objects from StatefulMessages and creates Payload with serialized DatumSequence
// Note: Serverless logs are not supported in this PoC implementation
type batchStrategy struct {
	inputChan    chan *message.StatefulMessage
	outputChan   chan *message.Payload
	flushChan    chan struct{}
	buffer       *sender.MessageBuffer
	pipelineName string
	batchWait    time.Duration
	compression  compression.Compressor
	stopChan     chan struct{} // closed when the goroutine has finished
	clock        clock.Clock

	// For gRPC: store Datums separately since MessageBuffer only stores metadata
	grpcDatums []*statefulpb.Datum

	// marshalBuf is reused across flushes for proto.Marshal output to reduce per-flush allocations.
	marshalBuf []byte

	// Delta encoding state - tracks previous values within current batch
	lastTimestamp     int64  // milliseconds since epoch
	lastPatternID     uint64 // pattern identifier
	lastTagsDictIndex uint64 // dictionary index of tag string

	// Telemetry
	pipelineMonitor metrics.PipelineMonitor
	utilization     metrics.UtilizationMonitor
	instanceID      string
}

// NewBatchStrategy returns a new gRPC batch strategy
func NewBatchStrategy(inputChan chan *message.StatefulMessage,
	outputChan chan *message.Payload,
	flushChan chan struct{},
	batchWait time.Duration,
	maxBatchSize int,
	maxContentSize int,
	pipelineName string,
	compression compression.Compressor,
	pipelineMonitor metrics.PipelineMonitor,
	instanceID string,
) sender.Strategy {
	return newBatchStrategyWithClock(inputChan, outputChan, flushChan, batchWait, maxBatchSize, maxContentSize, pipelineName, clock.New(), compression, pipelineMonitor, instanceID)
}

func newBatchStrategyWithClock(inputChan chan *message.StatefulMessage,
	outputChan chan *message.Payload,
	flushChan chan struct{},
	batchWait time.Duration,
	maxBatchSize int,
	maxContentSize int,
	pipelineName string,
	clock clock.Clock,
	compression compression.Compressor,
	pipelineMonitor metrics.PipelineMonitor,
	instanceID string,
) sender.Strategy {

	return &batchStrategy{
		inputChan:       inputChan,
		outputChan:      outputChan,
		flushChan:       flushChan,
		buffer:          sender.NewMessageBuffer(maxBatchSize, maxContentSize),
		batchWait:       batchWait,
		compression:     compression,
		stopChan:        make(chan struct{}),
		pipelineName:    pipelineName,
		clock:           clock,
		grpcDatums:      make([]*statefulpb.Datum, 0),
		pipelineMonitor: pipelineMonitor,
		utilization:     pipelineMonitor.MakeUtilizationMonitor(metrics.StrategyTlmName, instanceID),
		instanceID:      instanceID,
	}
}

// Mostly copy/pasted from sender/bactch_strategy.go
func (s *batchStrategy) Stop() {
	close(s.inputChan)
	<-s.stopChan
}

// Mostly copy/pasted from sender/bactch_strategy.go
func (s *batchStrategy) Start() {
	go func() {
		flushTicker := s.clock.Ticker(s.batchWait)
		defer func() {
			s.flushBuffer(s.outputChan)
			flushTicker.Stop()
			close(s.stopChan)
		}()
		for {
			select {
			case m, isOpen := <-s.inputChan:
				if !isOpen {
					// inputChan has been closed, no more payloads are expected
					return
				}
				s.processMessage(m, s.outputChan)
			case <-flushTicker.C:
				// flush the payloads at a regular interval so pending messages don't wait here for too long.
				s.flushBuffer(s.outputChan)
			case <-s.flushChan:
				// flush payloads on demand, used for infrequently running serverless functions
				s.flushBuffer(s.outputChan)
			}
		}
	}()
}

func (s *batchStrategy) addMessage(m *message.StatefulMessage) bool {
	// No utilization tracking here - just trivial slice operations
	// Real work (proto marshaling) is tracked in sendMessagesWithDatums()

	// Defensive check - should never happen with proper message construction
	if m.Datum == nil {
		return false
	}

	// Update delta state when PatternDefine passes through (only when delta encoding is active)
	if enableDeltaEncoding {
		if patternDefine := m.Datum.GetPatternDefine(); patternDefine != nil {
			s.lastPatternID = patternDefine.PatternId
		}
	}

	// Apply delta encoding to Log datums before adding to batch
	if logDatum := m.Datum.GetLogs(); logDatum != nil {
		s.applyDeltaEncoding(logDatum)
	}

	// Try to add to buffer
	if s.buffer.AddMessageWithSize(m.Metadata, m.Metadata.RawDataLen) {
		s.grpcDatums = append(s.grpcDatums, m.Datum)
		return true
	}

	// Buffer full (not an error)
	return false
}

// applyDeltaEncoding applies delta encoding to a Log datum within the current batch.
// Currently gated behind enableDeltaEncoding=false: the server does not implement delta
// reconstruction for any field (patternId, tags, timestamp), so this is a no-op until
// server-side support is added and protocol-version-negotiated.
func (s *batchStrategy) applyDeltaEncoding(logDatum *statefulpb.Log) {
	if !enableDeltaEncoding {
		return
	}
	// Timestamp delta encoding
	currentTimestamp := logDatum.Timestamp

	// First message in batch: send absolute timestamp
	if s.lastTimestamp == 0 {
		s.lastTimestamp = currentTimestamp
		// Keep absolute value in logDatum.Timestamp
	} else {
		// Normal case: compute and send delta
		delta := currentTimestamp - s.lastTimestamp
		s.lastTimestamp = currentTimestamp
		logDatum.Timestamp = delta // Note that when delta is 0, proto3 omits the timestamp field
	}

	// Pattern ID delta encoding (for structured logs only)
	if structured := logDatum.GetStructured(); structured != nil {
		if structured.PatternId == s.lastPatternID {
			structured.PatternId = 0 // proto3 omits zero values
		} else {
			s.lastPatternID = structured.PatternId
		}
	}

	// Tag delta encoding (extract dict index from TagSet)
	if tagSet := logDatum.Tags; tagSet != nil {
		if tagSetValue := tagSet.Tagset; tagSetValue != nil {
			if dictIndex := tagSetValue.GetDictIndex(); dictIndex != 0 {
				if dictIndex == s.lastTagsDictIndex {
					logDatum.Tags = nil // omit unchanged tags
				} else {
					s.lastTagsDictIndex = dictIndex
				}
			}
		}
	}
}

// Mostly copy/pasted from batch.go
func (s *batchStrategy) processMessage(m *message.StatefulMessage, outputChan chan *message.Payload) {
	// Track latency stats from metadata
	if m.Metadata.Origin != nil {
		m.Metadata.Origin.LogSource.LatencyStats.Add(m.Metadata.GetLatency())
	}

	added := s.addMessage(m)
	if !added || s.buffer.IsFull() {
		s.flushBuffer(outputChan)
	}
	if !added {
		// it's possible that the m could not be added because the buffer was full
		// so we need to retry once again
		added = s.addMessage(m)
		if !added {
			log.Warnf("Dropped message in pipeline=%s reason=too-large ContentLength=%d ContentSizeLimit=%d", s.pipelineName, m.Metadata.RawDataLen, s.buffer.ContentSizeLimit())
			tlmDroppedTooLarge.Inc(s.pipelineName)
		}
	}
}

// flushBuffer sends all the messages that are stored in the buffer and forwards them
// to the next stage of the pipeline.
func (s *batchStrategy) flushBuffer(outputChan chan *message.Payload) {
	if s.buffer.IsEmpty() {
		return
	}

	s.utilization.Start()

	messagesMetadata := s.buffer.GetMessages()
	s.buffer.Clear()

	// Use the collected Datums and clear them, reusing the backing array
	grpcDatums := s.grpcDatums
	s.grpcDatums = s.grpcDatums[:0]

	// Reset delta encoding state for next batch
	s.lastTimestamp = 0
	s.lastPatternID = 0
	s.lastTagsDictIndex = 0

	s.sendMessagesWithDatums(messagesMetadata, grpcDatums, outputChan)
}

func (s *batchStrategy) sendMessagesWithDatums(messagesMetadata []*message.MessageMetadata, grpcDatums []*statefulpb.Datum, outputChan chan *message.Payload) {
	defer s.utilization.Stop()

	unencodedSize := 0
	for _, msgMeta := range messagesMetadata {
		unencodedSize += msgMeta.RawDataLen
	}

	// Extract all state changes from this batch for snapshot management
	var stateChanges []*statefulpb.Datum
	for _, datum := range grpcDatums {
		if isStateDatum(datum) {
			stateChanges = append(stateChanges, datum)
		}
	}

	// Track per-datum-type counts and sizes
	for _, datum := range grpcDatums {
		var datumType string
		switch datum.Data.(type) {
		case *statefulpb.Datum_PatternDefine:
			datumType = "pattern_define"
		case *statefulpb.Datum_PatternDelete:
			datumType = "pattern_delete"
		case *statefulpb.Datum_Logs:
			datumType = "logs"
		case *statefulpb.Datum_DictEntryDefine:
			datumType = "dict_entry_define"
		case *statefulpb.Datum_DictEntryDelete:
			datumType = "dict_entry_delete"
		case *statefulpb.Datum_DeltaEncodingSync:
			datumType = "delta_encoding_sync"
		default:
			datumType = "unknown"
		}
		metrics.TlmDatumCount.Add(1, datumType)
		metrics.TlmDatumBytes.Add(float64(proto.Size(datum)), datumType)
	}

	// Create DatumSequence and marshal to bytes
	datumSeq := &statefulpb.DatumSequence{
		Data: grpcDatums,
	}

	var err error
	s.marshalBuf, err = proto.MarshalOptions{}.MarshalAppend(s.marshalBuf[:0], datumSeq)
	if err != nil {
		log.Errorf("Failed to marshal DatumSequence: %v", err)
		return
	}
	serialized := s.marshalBuf
	preCompressionBytes := len(serialized)

	// Compress the serialized protobuf data
	compressed, err := s.compression.Compress(serialized)
	if err != nil {
		log.Errorf("Failed to compress DatumSequence: %v", err)
		return
	}

	// Create payload with compressed data
	p := &message.Payload{
		MessageMetas:  messagesMetadata,
		Encoded:       compressed,
		Encoding:      s.compression.ContentEncoding(),
		UnencodedSize: unencodedSize,
		StatefulExtra: &StatefulExtra{
			PreCompressionBytes: preCompressionBytes,
		},
	}

	// Store batch-level state changes in payload
	if len(stateChanges) > 0 {
		p.StatefulExtra.(*StatefulExtra).StateChanges = stateChanges
	}

	outputChan <- p
	s.pipelineMonitor.ReportComponentEgress(p, metrics.StrategyTlmName, s.instanceID)
	s.pipelineMonitor.ReportComponentIngress(p, metrics.SenderTlmName, metrics.SenderTlmInstanceID)
}
