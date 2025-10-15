// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package sender

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type PayloadStatus struct {
	BatchID uint32
	Payload *message.Payload

	isOk      bool
	grpcError error
}

func (p *PayloadStatus) IsOk() bool {
	return p.isOk
}

func (p *PayloadStatus) Error() error {
	return p.grpcError
}

type pendingMessage struct {
	in      *message.Message
	encoded *message.Payload
}

// grpcStreamStrategy is a naive strategy that doesn't encode state on any logs, it simply encodes
// logs to gRPC as they are come in and sends them downstream, handling retries as needed.
type grpcStreamStrategy struct {
	inputChan          chan *message.Message // FROM processor
	outputChan         chan *message.Payload // TO gRPC sender
	senderResponseChan chan *PayloadStatus   // FROM gRPC Sender

	forwarderStopChan chan struct{} // Closed when this strategy is done forwarding messages from the inputChan once Stop() is called
	receiverStopChan  chan struct{} // Closed when this strategy is done receiving responses from the gRPC sender once Stop() is called

	pendingMessagesMu sync.Mutex
	pendingMessages   map[uint32]*pendingMessage

	serverlessMeta ServerlessMeta

	nextBatchId uint32

	// Telemetry
	pipelineName    string
	pipelineMonitor metrics.PipelineMonitor
	utilization     metrics.UtilizationMonitor
	instanceID      string
}

// NewGRPCStreamStrategy returns a new gRPC stream strategy
func NewGRPCStreamStrategy(inputChan chan *message.Message,
	outputChan chan *message.Payload,
	senderResponseChan chan *PayloadStatus,
	serverlessMeta ServerlessMeta,
	maxBatchSize int,
	maxContentSize int,
	pipelineName string,
	pipelineMonitor metrics.PipelineMonitor,
	instanceID string,
) Strategy {
	return &grpcStreamStrategy{
		inputChan:          inputChan,
		outputChan:         outputChan,
		senderResponseChan: senderResponseChan,
		forwarderStopChan:  make(chan struct{}),
		receiverStopChan:   make(chan struct{}),
		pendingMessages:    make(map[uint32]*pendingMessage),
		serverlessMeta:     serverlessMeta,
		nextBatchId:        0,
		pipelineName:       pipelineName,
		pipelineMonitor:    pipelineMonitor,
		utilization:        pipelineMonitor.MakeUtilizationMonitor(metrics.StrategyTlmName, instanceID),
		instanceID:         instanceID,
	}
}

// Stop closes the input channel and finishes sending any pending messages.
func (s *grpcStreamStrategy) Stop() {
	close(s.inputChan)
	// Close the senderResponseChan after 1 second to give the receiver time to finish
	time.AfterFunc(1*time.Second, func() {
		close(s.senderResponseChan)
	})
	<-s.forwarderStopChan
	<-s.receiverStopChan

	for batchID, pendingMessage := range s.pendingMessages {
		log.Warnf("Dropping pending message for pipeline %s (batch_id=%d): %v", s.pipelineName, batchID, pendingMessage.in)
	}
}

// Start reads the incoming messages and accumulates them to a buffer
func (s *grpcStreamStrategy) Start() {
	go s.forwarderLoop()
	go s.receiverLoop()
}

func (s *grpcStreamStrategy) forwarderLoop() {
	defer func() {
		close(s.forwarderStopChan)
	}()

	for {
		m, isOpen := <-s.inputChan
		if !isOpen {
			// inputChan has been closed, no more payloads are expected
			return
		}
		s.sendMessage(m)
	}
}

func (s *grpcStreamStrategy) receiverLoop() {
	defer func() {
		close(s.receiverStopChan)
	}()

	for {
		payloadStatus, isOpen := <-s.senderResponseChan
		if !isOpen {
			return
		}
		s.pendingMessagesMu.Lock()

		if payloadStatus.IsOk() {
			log.Infof("Got ack back for pipeline=%s batch_id=%d", s.pipelineName, payloadStatus.BatchID)
			s.outputChan <- s.pendingMessages[payloadStatus.BatchID].encoded
		} else {
			// TODO: implement smarter retry logic. For now just requeue the message.
			log.Warnf("Got error back for pipeline=%s batch_id=%d: %v, retrying", s.pipelineName, payloadStatus.BatchID, payloadStatus.Error())
			s.sendMessage(s.pendingMessages[payloadStatus.BatchID].in)
		}
		// Even if we requeued the message delete it here since sendMessage will add it back
		delete(s.pendingMessages, payloadStatus.BatchID)
		s.pendingMessagesMu.Unlock()
	}
}

func (s *grpcStreamStrategy) sendMessage(m *message.Message) {
	s.utilization.Start()
	defer s.utilization.Stop()

	if m.Origin != nil {
		m.Origin.LogSource.LatencyStats.Add(m.GetLatency())
	}

	unencodedSize := m.MessageMetadata.RawDataLen
	log.Debugf("Sending gRPC message for pipeline %s (content_size=%d)",
		s.pipelineName, unencodedSize)

	if s.serverlessMeta.IsEnabled() {
		s.serverlessMeta.Lock()
		s.serverlessMeta.WaitGroup().Add(1)
		s.serverlessMeta.Unlock()
	}

	// Check if any message in this batch is a snapshot
	isSnapshot := false
	if m.IsSnapshot {
		isSnapshot = true
	}

	// Create payload with GRPCDatums array instead of encoded bytes
	p := &message.Payload{
		MessageMetas:  []*message.MessageMetadata{&m.MessageMetadata},
		Encoded:       nil, // No encoded bytes for gRPC
		Encoding:      "",  // No encoding for gRPC
		UnencodedSize: unencodedSize,
		IsSnapshot:    isSnapshot, // Mark payload as snapshot if any message is snapshot
		GRPCEncoded: &statefulpb.StatefulBatch{
			BatchId: s.nextBatchId,
			Data:    []*statefulpb.Datum{m.GetGRPCDatum()},
		},
	}

	s.pendingMessagesMu.Lock()
	s.pendingMessages[s.nextBatchId] = &pendingMessage{
		in:      m,
		encoded: p,
	}
	s.pendingMessagesMu.Unlock()

	s.outputChan <- p
	s.pipelineMonitor.ReportComponentEgress(p, metrics.StrategyTlmName, s.instanceID)
	s.pipelineMonitor.ReportComponentIngress(p, metrics.SenderTlmName, metrics.SenderTlmInstanceID)
	s.nextBatchId++
}
