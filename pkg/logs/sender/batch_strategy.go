// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package sender

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

const (
	maxBatchSize   = 200
	maxContentSize = 1000000
)

// batchStrategy contains all the logic to send logs in batch.
type batchStrategy struct {
	buffer     *MessageBuffer
	serializer Serializer
	batchWait  time.Duration
}

// NewBatchStrategy returns a new batchStrategy.
func NewBatchStrategy(serializer Serializer, batchWait time.Duration) Strategy {
	return &batchStrategy{
		buffer:     NewMessageBuffer(maxBatchSize, maxContentSize),
		serializer: serializer,
		batchWait:  batchWait,
	}
}

// Send accumulates messages to a buffer and sends them when the buffer is full or outdated.
func (s *batchStrategy) Send(inputChan chan *message.Message, outputChan chan *message.Message, send func([]byte) error) {
	flushTimer := time.NewTimer(s.batchWait)
	defer func() {
		flushTimer.Stop()
	}()

	for {
		select {
		case message, isOpen := <-inputChan:
			if !isOpen {
				// inputChan has been closed, no more payload are expected
				s.sendBuffer(outputChan, send)
				return
			}
			added := s.buffer.AddMessage(message)
			if !added || s.buffer.IsFull() {
				// message buffer is full, either reaching max batch size or max content size,
				// send the payload now
				if !flushTimer.Stop() {
					// make sure the timer won't tick concurrently
					select {
					case <-flushTimer.C:
					default:
					}
				}
				s.sendBuffer(outputChan, send)
				flushTimer.Reset(s.batchWait)
			}
			if !added {
				// it's possible that the message could not be added because the buffer was full
				// so we need to retry once again
				s.buffer.AddMessage(message)
			}
		case <-flushTimer.C:
			// the first message that was added to the buffer has been here for too long,
			// send the payload now
			s.sendBuffer(outputChan, send)
			flushTimer.Reset(s.batchWait)
		}
	}
}

// sendBuffer sends all the messages that are stored in the buffer and forwards them
// to the next stage of the pipeline.
func (s *batchStrategy) sendBuffer(outputChan chan *message.Message, send func([]byte) error) {
	if s.buffer.IsEmpty() {
		return
	}

	messages := s.buffer.GetMessages()
	defer s.buffer.Clear()

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
