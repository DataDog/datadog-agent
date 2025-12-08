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

// telemetryBreakdown groups datum sizes for telemetry reporting on a stream.
type telemetryBreakdown struct {
	PatternLogsCount  int
	PatternLogBytes   int
	StateChangesCount int
	StateChangeBytes  int
	DatumCount        int
}

// StatefulExtra holds state changes (non-Log datums) from a batch and telemetry
// metadata used when emitting per-stream metrics.
// Used by inflight tracker to maintain snapshot state for stream rotation.
type StatefulExtra struct {
	StateChanges []*statefulpb.Datum
	Telemetry    telemetryBreakdown
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

	// Try to add to buffer
	if s.buffer.AddMessageWithSize(m.Metadata, m.Metadata.RawDataLen) {
		s.grpcDatums = append(s.grpcDatums, m.Datum)
		return true
	}

	// Buffer full (not an error)
	return false
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

	// Use the collected Datums and clear them
	grpcDatums := s.grpcDatums
	s.grpcDatums = make([]*statefulpb.Datum, 0)

	s.sendMessagesWithDatums(messagesMetadata, grpcDatums, outputChan)
}

func (s *batchStrategy) sendMessagesWithDatums(messagesMetadata []*message.MessageMetadata, grpcDatums []*statefulpb.Datum, outputChan chan *message.Payload) {
	defer s.utilization.Stop()

	unencodedSize := 0
	for _, msgMeta := range messagesMetadata {
		unencodedSize += msgMeta.RawDataLen
	}

	// Extract all state changes and telemetry from this batch for snapshot management and metrics
	var (
		stateChanges []*statefulpb.Datum
		tlmData      telemetryBreakdown
	)
	for _, datum := range grpcDatums {
		tlmData.DatumCount++
		switch d := datum.Data.(type) {
		case *statefulpb.Datum_PatternDefine:
			stateChanges = append(stateChanges, datum)
			s.recordStateChangeTelemetry(datum, &tlmData)
		case *statefulpb.Datum_PatternDelete, *statefulpb.Datum_DictEntryDelete:
			stateChanges = append(stateChanges, datum)
			s.recordStateChangeTelemetry(datum, &tlmData)
		case *statefulpb.Datum_DictEntryDefine:
			stateChanges = append(stateChanges, datum)
			s.recordStateChangeTelemetry(datum, &tlmData)
		case *statefulpb.Datum_Logs:
			if d.Logs != nil {
				s.recordLogTelemetry(d.Logs, proto.Size(datum), &tlmData)
			}
		}
	}

	// Create DatumSequence and marshal to bytes
	datumSeq := &statefulpb.DatumSequence{
		Data: grpcDatums,
	}

	serialized, err := proto.Marshal(datumSeq)
	if err != nil {
		log.Errorf("Failed to marshal DatumSequence: %v", err)
		return
	}

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
	}

	// Store batch-level state changes in payload
	p.StatefulExtra = &StatefulExtra{
		StateChanges: stateChanges,
		Telemetry:    tlmData,
	}

	outputChan <- p
	s.pipelineMonitor.ReportComponentEgress(p, metrics.StrategyTlmName, s.instanceID)
	s.pipelineMonitor.ReportComponentIngress(p, metrics.SenderTlmName, metrics.SenderTlmInstanceID)
}

// recordStateChangeTelemetry updates pipeline telemetry and per-batch counters for state mutations.
func (s *batchStrategy) recordStateChangeTelemetry(datum *statefulpb.Datum, tlmData *telemetryBreakdown) {
	size := proto.Size(datum)
	tlmData.StateChangesCount++
	tlmData.StateChangeBytes += size
	tlmPipelineStateChangeBytes.Add(float64(size), s.pipelineName)

	switch datum.Data.(type) {
	case *statefulpb.Datum_PatternDefine:
		tlmPipelinePatternsAdded.Inc(s.pipelineName)
		tlmPipelinePatternBytesAdded.Add(float64(size), s.pipelineName)
	case *statefulpb.Datum_PatternDelete:
		tlmPipelinePatternsRemoved.Inc(s.pipelineName)
		tlmPipelinePatternBytesRemoved.Add(float64(size), s.pipelineName)
	case *statefulpb.Datum_DictEntryDefine:
		tlmPipelineTokensAdded.Inc(s.pipelineName)
		tlmPipelineTokenBytesAdded.Add(float64(size), s.pipelineName)
	case *statefulpb.Datum_DictEntryDelete:
		tlmPipelineTokensRemoved.Inc(s.pipelineName)
		tlmPipelineTokenBytesRemoved.Add(float64(size), s.pipelineName)
	}
}

// recordLogTelemetry updates pipeline and per-batch counters for log datums.
func (s *batchStrategy) recordLogTelemetry(log *statefulpb.Log, encodedSize int, tlmData *telemetryBreakdown) {
	switch log.Content.(type) {
	case *statefulpb.Log_Raw:
		tlmPipelineRawLogBytes.Add(float64(encodedSize), s.pipelineName)
	case *statefulpb.Log_Structured:
		tlmData.PatternLogsCount++
		tlmData.PatternLogBytes += encodedSize
		tlmPipelinePatternLogBytes.Add(float64(encodedSize), s.pipelineName)
	}
}
