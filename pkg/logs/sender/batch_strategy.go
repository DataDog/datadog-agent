// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package sender

import (
	"bytes"
	"io"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	tlmDroppedTooLarge = telemetry.NewCounter("logs_sender_batch_strategy", "dropped_too_large", []string{"pipeline"}, "Number of payloads dropped due to being too large")
)

// batchStrategy contains all the logic to send logs in batch.
type batchStrategy struct {
	inputChan      chan *message.Message
	outputChan     chan *message.Payload
	flushChan      chan struct{}
	serverlessMeta ServerlessMeta
	buffer         *MessageBuffer
	// pipelineName provides a name for the strategy to differentiate it from other instances in other internal pipelines
	pipelineName string
	serializer   Serializer
	batchWait    time.Duration
	compression  compression.Compressor
	stopChan     chan struct{} // closed when the goroutine has finished
	clock        clock.Clock

	// Telemtry
	pipelineMonitor metrics.PipelineMonitor
	utilization     metrics.UtilizationMonitor
	compressor      compression.StreamCompressor
	writeCounter    *writerCounter
	encodedPayload  *bytes.Buffer
	instanceID      string
}

// NewBatchStrategy returns a new batch concurrent strategy with the specified batch & content size limits
func NewBatchStrategy(inputChan chan *message.Message,
	outputChan chan *message.Payload,
	flushChan chan struct{},
	serverlessMeta ServerlessMeta,
	serializer Serializer,
	batchWait time.Duration,
	maxBatchSize int,
	maxContentSize int,
	pipelineName string,
	compression compression.Compressor,
	pipelineMonitor metrics.PipelineMonitor,
	instanceID string,
) Strategy {
	return newBatchStrategyWithClock(inputChan, outputChan, flushChan, serverlessMeta, serializer, batchWait, maxBatchSize, maxContentSize, pipelineName, clock.New(), compression, pipelineMonitor, instanceID)
}

func newBatchStrategyWithClock(inputChan chan *message.Message,
	outputChan chan *message.Payload,
	flushChan chan struct{},
	serverlessMeta ServerlessMeta,
	serializer Serializer,
	batchWait time.Duration,
	maxBatchSize int,
	maxContentSize int,
	pipelineName string,
	clock clock.Clock,
	compression compression.Compressor,
	pipelineMonitor metrics.PipelineMonitor,
	instanceID string,
) Strategy {

	bs := &batchStrategy{
		inputChan:       inputChan,
		outputChan:      outputChan,
		flushChan:       flushChan,
		serverlessMeta:  serverlessMeta,
		buffer:          NewMessageBuffer(maxBatchSize, maxContentSize),
		serializer:      serializer,
		batchWait:       batchWait,
		compression:     compression,
		stopChan:        make(chan struct{}),
		pipelineName:    pipelineName,
		clock:           clock,
		pipelineMonitor: pipelineMonitor,
		utilization:     pipelineMonitor.MakeUtilizationMonitor(metrics.StrategyTlmName, instanceID),
		instanceID:      instanceID,
	}
	bs.MakeCompressor()
	return bs
}

func (s *batchStrategy) MakeCompressor() {
	s.buffer.Clear()
	s.serializer.Reset()
	var encodedPayload bytes.Buffer
	compressor := s.compression.NewStreamCompressor(&encodedPayload)
	if compressor == nil {
		compressor = &compression.NoopStreamCompressor{Writer: &encodedPayload}
	}

	wc := newWriterWithCounter(compressor)
	s.writeCounter = wc
	s.compressor = compressor
	s.encodedPayload = &encodedPayload
}

// Stop flushes the buffer and stops the strategy
func (s *batchStrategy) Stop() {
	close(s.inputChan)
	<-s.stopChan
}

// Start reads the incoming messages and accumulates them to a buffer. The buffer is
// encoded (optionally compressed) and written to a Payload which goes to the next
// step in the pipeline.
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

func (s *batchStrategy) addMessage(m *message.Message) (bool, error) {
	s.utilization.Start()
	defer s.utilization.Stop()

	if s.buffer.AddMessage(m) {
		err := s.serializer.Serialize(m, s.writeCounter)
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func (s *batchStrategy) processMessage(m *message.Message, outputChan chan *message.Payload) {
	if m.Origin != nil {
		m.Origin.LogSource.LatencyStats.Add(m.GetLatency())
	}
	added, err := s.addMessage(m)
	if err != nil {
		log.Warn("Encoding failed - dropping payload", err)
		s.MakeCompressor()
		return
	}
	if !added || s.buffer.IsFull() {
		s.flushBuffer(outputChan)
	}
	if !added {
		// it's possible that the m could not be added because the buffer was full
		// so we need to retry once again
		added, err = s.addMessage(m)
		if err != nil {
			log.Warn("Encoding failed - dropping payload", err)
			s.MakeCompressor()
			return
		}
		if !added {
			log.Warnf("Dropped message in pipeline=%s reason=too-large ContentLength=%d ContentSizeLimit=%d", s.pipelineName, len(m.GetContent()), s.buffer.ContentSizeLimit())
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
	if err := s.serializer.Finish(s.writeCounter); err != nil {
		log.Warn("Encoding failed - dropping payload", err)
		s.MakeCompressor()
		s.utilization.Stop()
		return
	}

	messagesMetadata := s.buffer.GetMessages()
	s.buffer.Clear()
	// Logging specifically for DBM pipelines, which seem to fail to send more often than other pipelines.
	// pipelineName comes from epforwarder.passthroughPipelineDescs.eventType, and these names are constants in the epforwarder package.
	if s.pipelineName == "dbm-samples" || s.pipelineName == "dbm-metrics" || s.pipelineName == "dbm-activity" {
		log.Debugf("Flushing buffer and sending %d messages for pipeline %s", len(messagesMetadata), s.pipelineName)
	}
	s.sendMessages(messagesMetadata, outputChan)
}

func (s *batchStrategy) sendMessages(messagesMetadata []*message.MessageMetadata, outputChan chan *message.Payload) {
	defer s.MakeCompressor()

	if err := s.compressor.Close(); err != nil {
		log.Warn("Encoding failed - dropping payload", err)
		s.utilization.Stop()
		return
	}

	unencodedSize := s.writeCounter.getWrittenBytes()
	log.Debugf("Send messages for pipeline %s (msg_count:%d, content_size=%d, avg_msg_size=%.2f)", s.pipelineName, len(messagesMetadata), unencodedSize, float64(unencodedSize)/float64(len(messagesMetadata)))

	if s.serverlessMeta.IsEnabled() {
		// Increment the wait group so the flush doesn't finish until all payloads are sent to all destinations
		// The lock is needed to ensure that the wait group is not incremented while the flush is in progress
		s.serverlessMeta.Lock()
		s.serverlessMeta.WaitGroup().Add(1)
		s.serverlessMeta.Unlock()
	}

	p := message.NewPayload(messagesMetadata, s.encodedPayload.Bytes(), s.compression.ContentEncoding(), unencodedSize)

	s.utilization.Stop()
	outputChan <- p
	s.pipelineMonitor.ReportComponentEgress(p, metrics.StrategyTlmName, s.instanceID)
	s.pipelineMonitor.ReportComponentIngress(p, metrics.SenderTlmName, metrics.SenderTlmInstanceID)
}

// writerCounter is a simple io.Writer that counts the number of bytes written to it
type writerCounter struct {
	io.Writer
	counter int
}

func newWriterWithCounter(w io.Writer) *writerCounter {
	return &writerCounter{Writer: w}
}

// Write writes the given bytes and increments the counter
func (wc *writerCounter) Write(b []byte) (int, error) {
	n, err := wc.Writer.Write(b)
	wc.counter += n
	return n, err
}

// getWrittenBytes returns the number of bytes written to the writer
func (wc *writerCounter) getWrittenBytes() int {
	return wc.counter
}
