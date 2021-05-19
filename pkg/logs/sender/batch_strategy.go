// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

var (
	tlmDroppedTooLarge = telemetry.NewCounter("logs_sender_batch_strategy", "dropped_too_large", []string{"pipeline"}, "Number of payloads dropped due to being too large")
)

// batchStrategy contains all the logic to send logs in batch.
type batchStrategy struct {
	buffer *MessageBuffer
	// pipelineName provides a name for the strategy to differentiate it from other instances in other internal pipelines
	pipelineName     string
	serializer       Serializer
	batchWait        time.Duration
	climit           chan struct{}  // semaphore for limiting concurrent sends
	pendingSends     sync.WaitGroup // waitgroup for concurrent sends
	syncFlushTrigger chan struct{}  // trigger a synchronous flush
	syncFlushDone    chan struct{}  // wait for a synchronous flush to finish
}

// NewBatchStrategy returns a new batch concurrent strategy with the specified batch & content size limits
// If `maxConcurrent` > 0, then at most that many payloads will be sent concurrently, else there is no concurrency
// and the pipeline will block while sending each payload.
func NewBatchStrategy(serializer Serializer, batchWait time.Duration, maxConcurrent int, maxBatchSize int, maxContentSize int, pipelineName string) Strategy {
	if maxConcurrent < 0 {
		maxConcurrent = 0
	}
	return &batchStrategy{
		buffer:           NewMessageBuffer(maxBatchSize, maxContentSize),
		serializer:       serializer,
		batchWait:        batchWait,
		climit:           make(chan struct{}, maxConcurrent),
		syncFlushTrigger: make(chan struct{}),
		syncFlushDone:    make(chan struct{}),
		pipelineName:     pipelineName,
	}

}

func (s *batchStrategy) Flush(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	default:
		s.syncFlushTrigger <- struct{}{}
		<-s.syncFlushDone
	}
}

func (s *batchStrategy) syncFlush(inputChan chan *message.Message, outputChan chan *message.Message, send func([]byte) error) {
	defer func() {
		s.flushBuffer(outputChan, send)
		s.pendingSends.Wait()
	}()
	for {
		select {
		case m, isOpen := <-inputChan:
			if !isOpen {
				return
			}
			s.processMessage(m, outputChan, send)
		default:
			return
		}
	}
}

// Send accumulates messages to a buffer and sends them when the buffer is full or outdated.
func (s *batchStrategy) Send(inputChan chan *message.Message, outputChan chan *message.Message, send func([]byte) error) {
	flushTicker := time.NewTicker(s.batchWait)
	defer func() {
		s.flushBuffer(outputChan, send)
		flushTicker.Stop()
		s.pendingSends.Wait()
	}()
	for {
		select {
		case m, isOpen := <-inputChan:
			if !isOpen {
				// inputChan has been closed, no more payloads are expected
				return
			}
			s.processMessage(m, outputChan, send)
		case <-flushTicker.C:
			// the first message that was added to the buffer has been here for too long, send the payload now
			s.flushBuffer(outputChan, send)
		case <-s.syncFlushTrigger:
			s.syncFlush(inputChan, outputChan, send)
			s.syncFlushDone <- struct{}{}
		}
	}
}

func (s *batchStrategy) processMessage(m *message.Message, outputChan chan *message.Message, send func([]byte) error) {
	if m.Origin != nil {
		m.Origin.LogSource.LatencyStats.Add(m.GetLatency())
	}
	added := s.buffer.AddMessage(m)
	if !added || s.buffer.IsFull() {
		s.flushBuffer(outputChan, send)
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
func (s *batchStrategy) flushBuffer(outputChan chan *message.Message, send func([]byte) error) {
	if s.buffer.IsEmpty() {
		return
	}
	messages := s.buffer.GetMessages()
	s.buffer.Clear()
	// if the channel is non-buffered then there is no concurrency and we block on sending each payload
	if cap(s.climit) == 0 {
		s.sendMessages(messages, outputChan, send)
		return
	}
	s.climit <- struct{}{}
	s.pendingSends.Add(1)
	go func() {
		s.sendMessages(messages, outputChan, send)
		s.pendingSends.Done()
		<-s.climit
	}()
}

func (s *batchStrategy) sendMessages(messages []*message.Message, outputChan chan *message.Message, send func([]byte) error) {
	err := send(s.serializer.Serialize(messages))
	if err != nil {
		if shouldStopSending(err) {
			return
		}
		log.Warnf("Could not send payload: %v", err)
	}

	metrics.LogsSent.Add(int64(len(messages)))
	metrics.TlmLogsSent.Add(float64(len(messages)))

	for _, message := range messages {
		outputChan <- message
	}
}
