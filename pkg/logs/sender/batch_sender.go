// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package sender

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

const (
	batchTimeout   = 5 * time.Second
	maxBatchSize   = 20
	maxContentSize = 1000000
)

// BatchSender is responsible for sending a batch of logs to different destinations.
type BatchSender struct {
	inputChan     chan *message.Message
	outputChan    chan *message.Message
	destinations  *client.Destinations
	done          chan struct{}
	batchTimeout  time.Duration
	messageBuffer *MessageBuffer
}

// NewBatchSender returns an new BatchSender.
func NewBatchSender(inputChan, outputChan chan *message.Message, destinations *client.Destinations) *BatchSender {
	return &BatchSender{
		inputChan:     inputChan,
		outputChan:    outputChan,
		destinations:  destinations,
		done:          make(chan struct{}),
		batchTimeout:  batchTimeout,
		messageBuffer: NewMessageBuffer(maxBatchSize, maxContentSize),
	}
}

// Start starts the BatchSender
func (b *BatchSender) Start() {
	go b.run()
}

// Stop stops the BatchSender,
// this call blocks until inputChan is flushed
func (b *BatchSender) Stop() {
	close(b.inputChan)
	<-b.done
}

// run lets the BatchSender send messages.
func (b *BatchSender) run() {
	flushTimer := time.NewTimer(b.batchTimeout)
	defer func() {
		flushTimer.Stop()
		b.done <- struct{}{}
	}()

	for {
		select {
		case payload, isOpen := <-b.inputChan:
			if !isOpen {
				// inputChan has been closed, no more payload are expected
				b.sendBuffer()
				return
			}
			success := b.messageBuffer.TryAddMessage(payload)
			if !success || b.messageBuffer.IsFull() {
				// message buffer is full, either reaching maxBatchCount of maxRequestSize
				// send request now. reset the timer
				if !flushTimer.Stop() {
					<-flushTimer.C
				}
				b.sendBuffer()
				flushTimer.Reset(b.batchTimeout)
			}
			if !success {
				// it's possible we didn't append last try because maxRequestSize is reached
				// append it again after the sendbuffer is flushed
				b.messageBuffer.TryAddMessage(payload)
			}
		case <-flushTimer.C:
			// the timout expired, the content is ready to be sent
			b.sendBuffer()
			flushTimer.Reset(b.batchTimeout)
		}
	}
}

// send keeps trying to send the message to the main destination until it succeeds
// and try to send the message to the additional destinations only once.
func (b *BatchSender) sendBuffer() {
	if b.messageBuffer.IsEmpty() {
		return
	}

	batchedContent := b.messageBuffer.GetPayload()
	defer b.messageBuffer.Clear()

	for {
		// this call is blocking until payload is sent (or the connection destination context cancelled)
		err := b.destinations.Main.Send(batchedContent)
		if err != nil {
			metrics.DestinationErrors.Add(1)
			if err == context.Canceled {
				// the context was cancelled, agent is stopping non-gracefully.
				// drop the message
				return
			}

			switch err.(type) {
			case *client.RetryableError:
				// could not send the payload because of a transport issue,
				// let's retry.
				continue
			}

			log.Warnf("Could not send payload, dropping it: %v", err)
			break
		}

		for _, destination := range b.destinations.Additionals {
			// send to a queue then send asynchronously for additional endpoints,
			// it will drop messages if the queue is full
			destination.SendAsync(batchedContent)
		}

		metrics.LogsSent.Add(1)
		break
	}

	for _, m := range b.messageBuffer.GetMessages() {
		b.outputChan <- m
	}
}
