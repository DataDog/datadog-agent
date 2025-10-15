// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package sender

import (
	"time"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TODO(jsaf): this was written by Joy, and has smarter batching but does not yet handle errors correctly.

// grpcBatchStrategy contains batching logic for gRPC sender without serializer
// It collects Datum objects from messages and creates Payload with GRPCDatums array
type grpcBatchStrategy struct {
	inputChan      chan *message.Message
	outputChan     chan *message.Payload
	flushChan      chan struct{}
	serverlessMeta ServerlessMeta
	buffer         *MessageBuffer
	pipelineName   string
	batchWait      time.Duration
	compression    compression.Compressor
	stopChan       chan struct{} // closed when the goroutine has finished
	clock          clock.Clock

	// For gRPC: store Datums separately since MessageBuffer only stores metadata
	grpcDatums  []*statefulpb.Datum
	nextBatchID uint32

	// Telemetry
	pipelineMonitor metrics.PipelineMonitor
	utilization     metrics.UtilizationMonitor
	instanceID      string
}

// NewGRPCStreamStrategy returns a new gRPC stream strategy
func NewGRPCBatchStrategy(inputChan chan *message.Message,
	outputChan chan *message.Payload,
	flushChan chan struct{},
	serverlessMeta ServerlessMeta,
	batchWait time.Duration,
	maxBatchSize int,
	maxContentSize int,
	pipelineName string,
	compression compression.Compressor,
	pipelineMonitor metrics.PipelineMonitor,
	instanceID string,
) Strategy {
	return newGRPCBatchStrategyWithClock(inputChan, outputChan, flushChan, serverlessMeta, batchWait, maxBatchSize, maxContentSize, pipelineName, clock.New(), compression, pipelineMonitor, instanceID)
}

func newGRPCBatchStrategyWithClock(inputChan chan *message.Message,
	outputChan chan *message.Payload,
	flushChan chan struct{},
	serverlessMeta ServerlessMeta,
	batchWait time.Duration,
	maxBatchSize int,
	maxContentSize int,
	pipelineName string,
	clock clock.Clock,
	compression compression.Compressor,
	pipelineMonitor metrics.PipelineMonitor,
	instanceID string,
) Strategy {

	gs := &grpcBatchStrategy{
		inputChan:       inputChan,
		outputChan:      outputChan,
		flushChan:       flushChan,
		serverlessMeta:  serverlessMeta,
		buffer:          NewMessageBuffer(maxBatchSize, maxContentSize),
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
	return gs
}

// Stop flushes the buffer and stops the strategy
func (s *grpcBatchStrategy) Stop() {
	close(s.inputChan)
	<-s.stopChan
}

// Start reads the incoming messages and accumulates them to a buffer
func (s *grpcBatchStrategy) Start() {
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

func (s *grpcBatchStrategy) addMessage(m *message.Message) bool {
	s.utilization.Start()
	defer s.utilization.Stop()

	// For gRPC strategy: add to buffer and collect the GRPCDatum
	if s.buffer.AddMessage(m) {
		// Store the GRPCDatum separately
		if datum := m.GetGRPCDatum(); datum != nil {
			s.grpcDatums = append(s.grpcDatums, datum)
		}
		return true
	}
	return false
}

func (s *grpcBatchStrategy) processMessage(m *message.Message, outputChan chan *message.Payload) {
	if m.Origin != nil {
		m.Origin.LogSource.LatencyStats.Add(m.GetLatency())
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
			log.Warnf("Dropped message in pipeline=%s reason=too-large ContentLength=%d ContentSizeLimit=%d", s.pipelineName, len(m.GetContent()), s.buffer.ContentSizeLimit())
			tlmDroppedTooLarge.Inc(s.pipelineName)
		}
	}

	// Check if this message requires immediate flush (e.g., for stream rotation snapshots)
	if added && m.IsSnapshot {
		s.flushBuffer(outputChan)
	}
}

// flushBuffer sends all the messages that are stored in the buffer and forwards them
// to the next stage of the pipeline.
func (s *grpcBatchStrategy) flushBuffer(outputChan chan *message.Payload) {
	if s.buffer.IsEmpty() {
		return
	}

	s.utilization.Start()

	messagesMetadata := s.buffer.GetMessages()
	s.buffer.Clear()

	// Use the collected GRPCDatums and clear them
	grpcDatums := s.grpcDatums
	s.grpcDatums = make([]*statefulpb.Datum, 0)

	log.Debugf("Flushing gRPC buffer and sending %d messages for pipeline %s", len(messagesMetadata), s.pipelineName)
	s.sendMessagesWithData(messagesMetadata, grpcDatums, outputChan)
}

func (s *grpcBatchStrategy) sendMessagesWithData(messagesMetadata []*message.MessageMetadata, grpcDatums []*statefulpb.Datum, outputChan chan *message.Payload) {
	s.utilization.Stop()

	unencodedSize := 0
	for _, msgMeta := range messagesMetadata {
		unencodedSize += msgMeta.RawDataLen
	}

	log.Debugf("Send gRPC messages for pipeline %s (msg_count:%d, content_size=%d, datum_count=%d)",
		s.pipelineName, len(messagesMetadata), unencodedSize, len(grpcDatums))

	if s.serverlessMeta.IsEnabled() {
		s.serverlessMeta.Lock()
		s.serverlessMeta.WaitGroup().Add(1)
		s.serverlessMeta.Unlock()
	}

	// Check if any message in this batch is a snapshot
	isSnapshot := false
	for _, msgMeta := range messagesMetadata {
		if msgMeta.IsSnapshot {
			isSnapshot = true
			break
		}
	}

	// Create payload with GRPCDatums array instead of encoded bytes
	p := &message.Payload{
		MessageMetas:  messagesMetadata,
		Encoded:       nil, // No encoded bytes for gRPC
		Encoding:      "",  // No encoding for gRPC
		UnencodedSize: unencodedSize,
		IsSnapshot:    isSnapshot, // Mark payload as snapshot if any message is snapshot
		GRPCEncoded: &statefulpb.StatefulBatch{
			BatchId: s.nextBatchID,
			Data:    grpcDatums,
		},
	}

	outputChan <- p
	s.nextBatchID++
	s.pipelineMonitor.ReportComponentEgress(p, metrics.StrategyTlmName, s.instanceID)
	s.pipelineMonitor.ReportComponentIngress(p, metrics.SenderTlmName, metrics.SenderTlmInstanceID)
}
