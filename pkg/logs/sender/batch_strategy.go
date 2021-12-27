// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"time"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	tlmDroppedTooLarge = telemetry.NewCounter("logs_sender_batch_strategy", "dropped_too_large", []string{"pipeline"}, "Number of payloads dropped due to being too large")
)

// batchStrategy contains all the logic to send logs in batch.
type batchStrategy struct {
	inputChan  chan *message.Message
	outputChan chan *message.Payload
	buffer     *MessageBuffer
	// pipelineName provides a name for the strategy to differentiate it from other instances in other internal pipelines
	pipelineName     string
	serializer       Serializer
	batchWait        time.Duration
	contentEncoding  ContentEncoding
	syncFlushTrigger chan struct{} // trigger a synchronous flush
	clock            clock.Clock
}

// NewBatchStrategy returns a new batch concurrent strategy with the specified batch & content size limits
func NewBatchStrategy(inputChan chan *message.Message,
	outputChan chan *message.Payload,
	serializer Serializer,
	batchWait time.Duration,
	maxBatchSize int,
	maxContentSize int,
	pipelineName string,
	contentEncoding ContentEncoding) Strategy {
	return newBatchStrategyWithClock(inputChan, outputChan, serializer, batchWait, maxBatchSize, maxContentSize, pipelineName, clock.New(), contentEncoding)
}

func newBatchStrategyWithClock(inputChan chan *message.Message,
	outputChan chan *message.Payload,
	serializer Serializer,
	batchWait time.Duration,
	maxBatchSize int,
	maxContentSize int,
	pipelineName string,
	clock clock.Clock,
	contentEncoding ContentEncoding) Strategy {

	return &batchStrategy{
		inputChan:        inputChan,
		outputChan:       outputChan,
		buffer:           NewMessageBuffer(maxBatchSize, maxContentSize),
		serializer:       serializer,
		batchWait:        batchWait,
		contentEncoding:  contentEncoding,
		syncFlushTrigger: make(chan struct{}),
		pipelineName:     pipelineName,
		clock:            clock,
	}
}

// Stop flushes the buffer and stops the strategy
func (s *batchStrategy) Stop() {
	s.syncFlushTrigger <- struct{}{}
	close(s.inputChan)
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
			case <-s.syncFlushTrigger:
				s.flushBuffer(s.outputChan)
			}
		}
	}()
}

func (s *batchStrategy) processMessage(m *message.Message, outputChan chan *message.Payload) {
	if m.Origin != nil {
		m.Origin.LogSource.LatencyStats.Add(m.GetLatency())
	}
	added := s.buffer.AddMessage(m)
	if !added || s.buffer.IsFull() {
		s.flushBuffer(outputChan)
	}
	if !added {
		// it's possible that the m could not be added because the buffer was full
		// so we need to retry once again
		if !s.buffer.AddMessage(m) {
			log.Warnf("Dropped message in pipeline=%s reason=too-large ContentLength=%d ContentSizeLimit=%d", s.pipelineName, len(m.Content), s.buffer.ContentSizeLimit())
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
	messages := s.buffer.GetMessages()
	s.buffer.Clear()
	s.sendMessages(messages, outputChan)
}

func (s *batchStrategy) sendMessages(messages []*message.Message, outputChan chan *message.Payload) {
	serializedMessage := s.serializer.Serialize(messages)
	encodedPayload, err := s.contentEncoding.encode(serializedMessage)
	if err != nil {
		log.Warn("Encoding failed - dropping payload", err)
		return
	}

	outputChan <- &message.Payload{
		Messages:      messages,
		Encoded:       encodedPayload,
		Encoding:      s.contentEncoding.name(),
		UnencodedSize: len(serializedMessage),
	}
}
