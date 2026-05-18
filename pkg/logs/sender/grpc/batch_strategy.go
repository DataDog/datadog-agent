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

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs-library/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	tlmDroppedTooLarge = telemetryimpl.GetCompatComponent().NewCounter("logs_sender_grpc_batch_strategy", "dropped_too_large", []string{"pipeline"}, "Number of payloads dropped due to being too large")
)

// enableDeltaEncoding controls whether the agent applies delta encoding to Log datums.
// When false: patternId, tags, and timestamp are sent as absolute values on every Log datum.
const enableDeltaEncoding = true

const datumBytesTelemetrySampleRate = 16

// StatefulExtra holds state changes (non-Log datums) from a batch
// Used by inflight tracker to maintain snapshot state for stream rotation
type StatefulExtra struct {
	StateChanges []*statefulpb.Datum
	// WireDatums are the canonical, pre-delta datums represented by the encoded
	// payload. Inflight uses them to find state references and rebuild replay
	// batches with lazy snapshot state before final wire delta encoding.
	WireDatums          []*statefulpb.Datum
	PreCompressionBytes int
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
	// datumBytesTelemetryCount keeps sampling unbiased across batch boundaries.
	datumBytesTelemetryCount uint64

	// Delta encoding state - tracks previous values within current batch
	lastTimestamp      int64  // milliseconds since epoch
	lastPatternID      uint64 // pattern identifier
	lastTagsDictIndex  uint64 // dictionary index of tag string
	lastServiceDictID  uint64 // dictionary index of the service field
	lastStatusDictID   uint64 // dictionary index of the status field
	lastJSONSchemaDict uint64 // dictionary index of the JSON schema field

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

	if !s.buffer.AddMessageWithSize(m.Metadata, m.Metadata.RawDataLen) {
		// Buffer full (not an error)
		return false
	}

	s.grpcDatums = append(s.grpcDatums, m.Datum)
	return true
}

func deltaEncodeDatumsForWire(datums []*statefulpb.Datum) []*statefulpb.Datum {
	if !enableDeltaEncoding {
		return datums
	}

	encoded := make([]*statefulpb.Datum, 0, len(datums))
	state := batchStrategy{}
	for _, datum := range datums {
		switch data := datum.GetData().(type) {
		case *statefulpb.Datum_PatternDefine:
			if data.PatternDefine != nil {
				state.lastPatternID = data.PatternDefine.PatternId
			}
			encoded = append(encoded, datum)
		case *statefulpb.Datum_Logs:
			if data.Logs == nil {
				encoded = append(encoded, datum)
				continue
			}
			cloned := cloneLogForDeltaEncoding(data.Logs)
			state.applyDeltaEncoding(cloned)
			encoded = append(encoded, &statefulpb.Datum{
				Data: &statefulpb.Datum_Logs{Logs: cloned},
			})
		case *statefulpb.Datum_FlatLog:
			if data.FlatLog == nil {
				encoded = append(encoded, datum)
				continue
			}
			cloned := cloneFlatLogForDeltaEncoding(data.FlatLog)
			state.applyFlatLogDeltaEncoding(cloned)
			encoded = append(encoded, &statefulpb.Datum{
				Data: &statefulpb.Datum_FlatLog{FlatLog: cloned},
			})
		default:
			encoded = append(encoded, datum)
		}
	}
	return encoded
}

func cloneLogForDeltaEncoding(logDatum *statefulpb.Log) *statefulpb.Log {
	cloned := &statefulpb.Log{
		Timestamp: logDatum.Timestamp,
		Tags:      logDatum.Tags,
		Uuid:      logDatum.Uuid,
		Status:    logDatum.Status,
		Service:   logDatum.Service,
	}
	switch content := logDatum.Content.(type) {
	case *statefulpb.Log_Structured:
		clonedStructured := cloneStructuredLogForDeltaEncoding(content.Structured)
		cloned.Content = &statefulpb.Log_Structured{Structured: clonedStructured}
	case *statefulpb.Log_Raw:
		cloned.Content = &statefulpb.Log_Raw{Raw: content.Raw}
	}
	return cloned
}

func cloneStructuredLogForDeltaEncoding(logDatum *statefulpb.StructuredLog) *statefulpb.StructuredLog {
	if logDatum == nil {
		return nil
	}
	return &statefulpb.StructuredLog{
		PatternId:           logDatum.PatternId,
		DynamicValues:       logDatum.DynamicValues,
		JsonMessageKey:      logDatum.JsonMessageKey,
		JsonContextSchemaId: logDatum.JsonContextSchemaId,
		JsonContextValues:   logDatum.JsonContextValues,
		JsonContext:         logDatum.JsonContext,
	}
}

func cloneFlatLogForDeltaEncoding(logDatum *statefulpb.FlatLog) *statefulpb.FlatLog {
	return &statefulpb.FlatLog{
		Timestamp:               logDatum.Timestamp,
		Status:                  logDatum.Status,
		Service:                 logDatum.Service,
		Tags:                    logDatum.Tags,
		PatternId:               logDatum.PatternId,
		DynamicValues:           logDatum.DynamicValues,
		RawLog:                  logDatum.RawLog,
		JsonSchemaId:            logDatum.JsonSchemaId,
		JsonContextValues:       logDatum.JsonContextValues,
		JsonContextValueKinds:   logDatum.JsonContextValueKinds,
		JsonContextIntValues:    logDatum.JsonContextIntValues,
		JsonContextFloatValues:  logDatum.JsonContextFloatValues,
		JsonContextDictValues:   logDatum.JsonContextDictValues,
		JsonContextRawValues:    logDatum.JsonContextRawValues,
		JsonContextStringValues: logDatum.JsonContextStringValues,
		Uuid:                    logDatum.Uuid,
	}
}

func (s *batchStrategy) applyTimestampDeltaEncoding(timestamp *int64) {
	currentTimestamp := *timestamp
	if s.lastTimestamp == 0 {
		s.lastTimestamp = currentTimestamp
		return
	}

	delta := currentTimestamp - s.lastTimestamp
	s.lastTimestamp = currentTimestamp
	*timestamp = delta // Note that when delta is 0, proto3 omits the timestamp field.
}

// applyDeltaEncoding applies delta encoding to a Log datum within the current batch.
func (s *batchStrategy) applyDeltaEncoding(logDatum *statefulpb.Log) {
	if !enableDeltaEncoding {
		return
	}
	// Timestamp delta encoding
	s.applyTimestampDeltaEncoding(&logDatum.Timestamp)

	// Pattern ID delta encoding (for structured logs only)
	if structured := logDatum.GetStructured(); structured != nil {
		if structured.PatternId == s.lastPatternID {
			structured.PatternId = 0
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

	// Service delta encoding (only for dict-index encoded services)
	if service := logDatum.Service; service != nil {
		if dictIndex := service.GetDictIndex(); dictIndex != 0 {
			if dictIndex == s.lastServiceDictID {
				logDatum.Service = nil // omit unchanged service
			} else {
				s.lastServiceDictID = dictIndex
			}
		}
	}
}

// applyFlatLogDeltaEncoding applies delta encoding to a FlatLog datum within the current batch.
func (s *batchStrategy) applyFlatLogDeltaEncoding(logDatum *statefulpb.FlatLog) {
	if !enableDeltaEncoding {
		return
	}

	s.applyTimestampDeltaEncoding(&logDatum.Timestamp)

	logDatum.Status = s.deltaFlatLogDictIndex(logDatum.Status, &s.lastStatusDictID)
	logDatum.Service = s.deltaFlatLogDictIndex(logDatum.Service, &s.lastServiceDictID)
	logDatum.Tags = s.deltaFlatLogDictIndex(logDatum.Tags, &s.lastTagsDictIndex)
	logDatum.JsonSchemaId = s.deltaFlatLogDictIndex(logDatum.JsonSchemaId, &s.lastJSONSchemaDict)

	if logDatum.RawLog == "" {
		if logDatum.PatternId == s.lastPatternID {
			logDatum.PatternId = 0
		} else {
			s.lastPatternID = logDatum.PatternId
		}
	}
}

func (s *batchStrategy) deltaFlatLogDictIndex(current uint64, last *uint64) uint64 {
	if current == 0 {
		current = flatLogEmptyDictIndex
	}
	if current == *last {
		return 0
	}
	*last = current
	return current
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
	s.lastServiceDictID = 0
	s.lastStatusDictID = 0
	s.lastJSONSchemaDict = 0

	s.sendMessagesWithDatums(messagesMetadata, grpcDatums, outputChan)
}

func (s *batchStrategy) sendMessagesWithDatums(messagesMetadata []*message.MessageMetadata, grpcDatums []*statefulpb.Datum, outputChan chan *message.Payload) {
	defer s.utilization.Stop()

	unencodedSize := 0
	for _, msgMeta := range messagesMetadata {
		unencodedSize += msgMeta.RawDataLen
	}

	stateChanges, wireDatums := splitStateAndWireDatums(grpcDatums)

	for _, datum := range grpcDatums {
		datumType := datumTelemetryType(datum)
		metrics.TlmDatumCount.Add(1, datumType)
		if s.datumBytesTelemetryCount%datumBytesTelemetrySampleRate == 0 {
			metrics.TlmDatumBytes.Add(float64(proto.Size(datum)*datumBytesTelemetrySampleRate), datumType)
		}
		s.datumBytesTelemetryCount++
	}

	encodedDatums := deltaEncodeDatumsForWire(wireDatums)

	// Create DatumSequence and marshal to bytes
	datumSeq := &statefulpb.DatumSequence{
		Data: encodedDatums,
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
	if len(compressed) > 0 && &compressed[0] == &serialized[0] {
		compressed = append([]byte(nil), compressed...)
	}

	// Create payload with compressed data
	p := &message.Payload{
		MessageMetas:  messagesMetadata,
		Encoded:       compressed,
		Encoding:      s.compression.ContentEncoding(),
		UnencodedSize: unencodedSize,
		StatefulExtra: &StatefulExtra{
			WireDatums:          wireDatums,
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

func datumTelemetryType(datum *statefulpb.Datum) string {
	switch datum.Data.(type) {
	case *statefulpb.Datum_PatternDefine:
		return "pattern_define"
	case *statefulpb.Datum_PatternDelete:
		return "pattern_delete"
	case *statefulpb.Datum_Logs, *statefulpb.Datum_FlatLog:
		return "logs"
	case *statefulpb.Datum_DictEntryDefine:
		return "dict_entry_define"
	case *statefulpb.Datum_DictEntryDelete:
		return "dict_entry_delete"
	case *statefulpb.Datum_DeltaEncodingSync:
		return "delta_encoding_sync"
	case *statefulpb.Datum_JsonSchemaDefine:
		return "json_schema_define"
	case *statefulpb.Datum_JsonSchemaDelete:
		return "json_schema_delete"
	default:
		return "unknown"
	}
}
