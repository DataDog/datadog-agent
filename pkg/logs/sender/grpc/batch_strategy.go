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

// Buckets for datum count per batch histogram
var datumCountBuckets = []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000}

var (
	tlmDroppedTooLarge = telemetry.NewCounter("logs_sender_grpc_batch_strategy", "dropped_too_large", []string{"pipeline"}, "Number of payloads dropped due to being too large")

	// =========================================================================
	// Per-Pipeline Metrics (tagged by "pipeline")
	// =========================================================================

	// TlmStateSize is the current size of the snapshot state in bytes (patterns + dict entries)
	TlmStateSize = telemetry.NewGauge("logs_sender_grpc", "state_size_bytes", []string{"pipeline"}, "Current size of the snapshot state in bytes")

	// Pattern state metrics
	TlmPatternsAdded       = telemetry.NewCounter("logs_sender_grpc", "patterns_added_total", []string{"pipeline"}, "Total number of patterns added to state")
	TlmPatternsRemoved     = telemetry.NewCounter("logs_sender_grpc", "patterns_removed_total", []string{"pipeline"}, "Total number of patterns removed from state")
	TlmPatternBytesAdded   = telemetry.NewCounter("logs_sender_grpc", "pattern_bytes_added_total", []string{"pipeline"}, "Total bytes of patterns added to state")
	TlmPatternBytesRemoved = telemetry.NewCounter("logs_sender_grpc", "pattern_bytes_removed_total", []string{"pipeline"}, "Total bytes of patterns removed from state")

	// Token (dict entry) state metrics
	TlmTokensAdded       = telemetry.NewCounter("logs_sender_grpc", "tokens_added_total", []string{"pipeline"}, "Total number of tokens (dict entries) added to state")
	TlmTokensRemoved     = telemetry.NewCounter("logs_sender_grpc", "tokens_removed_total", []string{"pipeline"}, "Total number of tokens (dict entries) removed from state")
	TlmTokenBytesAdded   = telemetry.NewCounter("logs_sender_grpc", "token_bytes_added_total", []string{"pipeline"}, "Total bytes of tokens (dict entries) added to state")
	TlmTokenBytesRemoved = telemetry.NewCounter("logs_sender_grpc", "token_bytes_removed_total", []string{"pipeline"}, "Total bytes of tokens (dict entries) removed from state")

	// Log bytes metrics (per pipeline)
	TlmPatternLogBytes  = telemetry.NewCounter("logs_sender_grpc", "pattern_log_bytes_total", []string{"pipeline"}, "Total bytes of pattern-based logs sent")
	TlmRawLogBytes      = telemetry.NewCounter("logs_sender_grpc", "raw_log_bytes_total", []string{"pipeline"}, "Total bytes of raw logs sent")
	TlmStateChangeBytes = telemetry.NewCounter("logs_sender_grpc", "state_change_bytes_total", []string{"pipeline"}, "Total bytes of state changes sent")

	// =========================================================================
	// Per-Stream Metrics (tagged by "stream")
	// =========================================================================

	TlmStreamPatternLogCount  = telemetry.NewCounter("logs_sender_grpc", "stream_pattern_logs_total", []string{"stream"}, "Total number of pattern-based logs sent per stream")
	TlmStreamPatternLogBytes  = telemetry.NewCounter("logs_sender_grpc", "stream_pattern_log_bytes_total", []string{"stream"}, "Total bytes of pattern-based logs sent per stream")
	TlmStreamStateChangeCount = telemetry.NewCounter("logs_sender_grpc", "stream_state_changes_total", []string{"stream"}, "Total number of state changes sent per stream")
	TlmStreamStateChangeBytes = telemetry.NewCounter("logs_sender_grpc", "stream_state_change_bytes_total", []string{"stream"}, "Total bytes of state changes sent per stream")
	TlmStreamBatchCount       = telemetry.NewCounter("logs_sender_grpc", "stream_batches_total", []string{"stream"}, "Total number of batches sent per stream")
	TlmStreamDatumsPerBatch   = telemetry.NewHistogram("logs_sender_grpc", "stream_datums_per_batch", []string{"stream"}, "Number of datums per batch sent per stream", datumCountBuckets)
)

// BatchStats holds statistics about a batch for telemetry purposes
type BatchStats struct {
	PatternLogCount  int // Number of pattern-based logs in the batch
	PatternLogBytes  int // Total bytes of pattern-based logs
	StateChangeCount int // Number of state changes (pattern/token add/remove)
	StateChangeBytes int // Total bytes of state changes
	TotalDatumCount  int // Total number of datums in the batch
}

// StatefulExtra holds state changes (non-Log datums) from a batch
// Used by inflight tracker to maintain snapshot state for stream rotation
type StatefulExtra struct {
	StateChanges []*statefulpb.Datum
	Stats        BatchStats // Batch statistics for per-stream telemetry
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

	// Extract state changes and track per-pipeline metrics
	var stateChanges []*statefulpb.Datum
	var stats BatchStats
	stats.TotalDatumCount = len(grpcDatums)

	for _, datum := range grpcDatums {
		if isStateDatum(datum) {
			stateChanges = append(stateChanges, datum)
		}
		// Track per-pipeline metrics and accumulate batch stats
		s.trackDatumMetrics(datum, &stats)
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

	// Store batch-level state changes and stats in payload
	p.StatefulExtra = &StatefulExtra{
		StateChanges: stateChanges,
		Stats:        stats,
	}

	outputChan <- p
	s.pipelineMonitor.ReportComponentEgress(p, metrics.StrategyTlmName, s.instanceID)
	s.pipelineMonitor.ReportComponentIngress(p, metrics.SenderTlmName, metrics.SenderTlmInstanceID)
}

// trackDatumMetrics records per-pipeline telemetry metrics and accumulates batch stats
func (s *batchStrategy) trackDatumMetrics(datum *statefulpb.Datum, stats *BatchStats) {
	switch d := datum.Data.(type) {
	case *statefulpb.Datum_DictEntryDefine:
		// Token (dict entry) added
		size := 8 + len(d.DictEntryDefine.Value) // ID (8 bytes) + value length
		TlmTokensAdded.Inc(s.pipelineName)
		TlmTokenBytesAdded.Add(float64(size), s.pipelineName)
		TlmStateChangeBytes.Add(float64(size), s.pipelineName)
		stats.StateChangeCount++
		stats.StateChangeBytes += size

	case *statefulpb.Datum_DictEntryDelete:
		// Token (dict entry) removed - size is just the ID
		size := 8
		TlmTokensRemoved.Inc(s.pipelineName)
		TlmTokenBytesRemoved.Add(float64(size), s.pipelineName)
		TlmStateChangeBytes.Add(float64(size), s.pipelineName)
		stats.StateChangeCount++
		stats.StateChangeBytes += size

	case *statefulpb.Datum_PatternDefine:
		// Pattern added
		size := 8 + len(d.PatternDefine.Template) + 4 + (len(d.PatternDefine.PosList) * 4)
		TlmPatternsAdded.Inc(s.pipelineName)
		TlmPatternBytesAdded.Add(float64(size), s.pipelineName)
		TlmStateChangeBytes.Add(float64(size), s.pipelineName)
		stats.StateChangeCount++
		stats.StateChangeBytes += size

	case *statefulpb.Datum_PatternDelete:
		// Pattern removed - size is just the ID
		size := 8
		TlmPatternsRemoved.Inc(s.pipelineName)
		TlmPatternBytesRemoved.Add(float64(size), s.pipelineName)
		TlmStateChangeBytes.Add(float64(size), s.pipelineName)
		stats.StateChangeCount++
		stats.StateChangeBytes += size

	case *statefulpb.Datum_Logs:
		logData := d.Logs
		switch content := logData.Content.(type) {
		case *statefulpb.Log_Raw:
			// Raw log
			size := len(content.Raw)
			TlmRawLogBytes.Add(float64(size), s.pipelineName)

		case *statefulpb.Log_Structured:
			// Pattern-based log
			size := 8 // pattern_id
			for _, dv := range content.Structured.DynamicValues {
				size += estimateDynamicValueSize(dv)
			}
			TlmPatternLogBytes.Add(float64(size), s.pipelineName)
			stats.PatternLogCount++
			stats.PatternLogBytes += size
		}
	}
}

// estimateDynamicValueSize returns the estimated size of a DynamicValue in bytes
func estimateDynamicValueSize(dv *statefulpb.DynamicValue) int {
	switch v := dv.Value.(type) {
	case *statefulpb.DynamicValue_IntValue:
		return 8
	case *statefulpb.DynamicValue_FloatValue:
		return 8
	case *statefulpb.DynamicValue_StringValue:
		return len(v.StringValue)
	case *statefulpb.DynamicValue_DictIndex:
		return 8
	default:
		return 0
	}
}
