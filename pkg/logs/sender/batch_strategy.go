// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"context"
	"expvar"
	"fmt"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	tlmDroppedTooLarge   = telemetry.NewCounter("logs_sender_batch_strategy", "dropped_too_large", []string{"pipeline"}, "Number of payloads dropped due to being too large")
	batchStrategyExpVars = expvar.NewMap("batch_strategy")
	expVarIdleMsMapKey   = "idleMs"
	expVarInUseMapKey    = "inUseMs"
)

// batchStrategy contains all the logic to send logs in batch.
type batchStrategy struct {
	buffer *MessageBuffer
	// pipelineName provides a name for the strategy to differentiate it from other instances in other internal pipelines
	pipelineName     string
	serializer       Serializer
	batchWait        time.Duration
	contentEncoding  ContentEncoding
	syncFlushTrigger chan struct{} // trigger a synchronous flush
	expVars          *expvar.Map
	clock            clock.Clock
}

// NewBatchStrategy returns a new batch concurrent strategy with the specified batch & content size limits
// If `maxConcurrent` > 0, then at most that many payloads will be sent concurrently, else there is no concurrency
// and the pipeline will block while sending each payload.
func NewBatchStrategy(serializer Serializer, batchWait time.Duration, maxConcurrent int, maxBatchSize int, maxContentSize int, pipelineName string, pipelineID int, contentEncoding ContentEncoding) Strategy {
	return newBatchStrategyWithClock(serializer, batchWait, maxConcurrent, maxBatchSize, maxContentSize, pipelineName, pipelineID, clock.New(), contentEncoding)
}

func newBatchStrategyWithClock(serializer Serializer, batchWait time.Duration, maxConcurrent int, maxBatchSize int, maxContentSize int, pipelineName string, pipelineID int, clock clock.Clock, contentEncoding ContentEncoding) Strategy {
	if maxConcurrent < 0 {
		maxConcurrent = 0
	}
	expVars := &expvar.Map{}
	expVars.AddFloat(expVarIdleMsMapKey, 0)
	expVars.AddFloat(expVarInUseMapKey, 0)

	batchStrategyExpVars.Set(fmt.Sprintf("%s_%d", pipelineName, pipelineID), expVars)

	return &batchStrategy{
		buffer:           NewMessageBuffer(maxBatchSize, maxContentSize),
		serializer:       serializer,
		batchWait:        batchWait,
		contentEncoding:  contentEncoding,
		syncFlushTrigger: make(chan struct{}),
		pipelineName:     pipelineName,
		expVars:          expVars,
		clock:            clock,
	}
}

func (s *batchStrategy) Flush(ctx context.Context) {
	s.syncFlushTrigger <- struct{}{}
}

// Send accumulates messages to a buffer and sends them when the buffer is full or outdated.
func (s *batchStrategy) Start(inputChan chan *message.Message, outputChan chan *Payload) {

	go func() {
		flushTicker := s.clock.Ticker(s.batchWait)
		defer func() {
			s.flushBuffer(outputChan)
			flushTicker.Stop()
		}()
		var startIdle = time.Now()
		for {
			select {
			case m, isOpen := <-inputChan:

				if !isOpen {
					// inputChan has been closed, no more payloads are expected
					return
				}
				// TODo: move this telemetry
				s.expVars.AddFloat(expVarIdleMsMapKey, float64(time.Since(startIdle)/time.Millisecond))
				var startInUse = time.Now()

				s.processMessage(m, outputChan)

				s.expVars.AddFloat(expVarInUseMapKey, float64(time.Since(startInUse)/time.Millisecond))
				startIdle = time.Now()

			case <-flushTicker.C:
				// the first message that was added to the buffer has been here for too long, send the payload now
				s.flushBuffer(outputChan)
			case <-s.syncFlushTrigger:
				s.flushBuffer(outputChan)
			}
		}
	}()
}

func (s *batchStrategy) processMessage(m *message.Message, outputChan chan *Payload) {
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
func (s *batchStrategy) flushBuffer(outputChan chan *Payload) {
	if s.buffer.IsEmpty() {
		return
	}
	messages := s.buffer.GetMessages()
	s.buffer.Clear()
	s.sendMessages(messages, outputChan)
}

func (s *batchStrategy) sendMessages(messages []*message.Message, outputChan chan *Payload) {
	// TODO: move telemetry to sender?
	metrics.LogsSent.Add(int64(len(messages)))
	metrics.TlmLogsSent.Add(float64(len(messages)))

	serializedMessage := s.serializer.Serialize(messages)
	encodedPayload, err := s.contentEncoding.encode(serializedMessage)
	if err != nil {
		log.Warn("Encoding failed - dropping payload", err)
		return
	}

	outputChan <- &Payload{messages: messages, payload: encodedPayload}
}
