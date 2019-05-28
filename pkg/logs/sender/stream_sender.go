// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package sender

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

// StreamSender sends one log at a time to different destinations.
type StreamSender struct {
	inputChan    chan *message.Message
	outputChan   chan *message.Message
	destinations *client.Destinations
	done         chan struct{}
}

// NewStreamSender returns an new StreamSender.
func NewStreamSender(inputChan, outputChan chan *message.Message, destinations *client.Destinations) *StreamSender {
	return &StreamSender{
		inputChan:    inputChan,
		outputChan:   outputChan,
		destinations: destinations,
		done:         make(chan struct{}),
	}
}

// Start starts the StreamSender
func (s *StreamSender) Start() {
	go s.run()
}

// Stop stops the StreamSender,
// this call blocks until inputChan is flushed
func (s *StreamSender) Stop() {
	close(s.inputChan)
	<-s.done
}

// run lets the StreamSender send messages.
func (s *StreamSender) run() {
	defer func() {
		s.done <- struct{}{}
	}()

	for payload := range s.inputChan {
		s.send(payload)
	}
}

// send keeps trying to send the message to the main destination until it succeeds
// and try to send the message to the additional destinations only once.
func (s *StreamSender) send(payload *message.Message) {
	for {
		// this call is blocking until payload is sent (or the connection destination context cancelled)
		err := s.destinations.Main.Send(payload.Content)
		if err != nil {
			if err == context.Canceled {
				metrics.DestinationErrors.Add(1)
				// the context was cancelled, agent is stopping non-gracefully.
				// drop the message
				break
			}
			switch err.(type) {
			case *client.FramingError:
				metrics.DestinationErrors.Add(1)
				// the message can not be framed properly,
				// drop the message
				break
			default:
				metrics.DestinationErrors.Add(1)
				// retry as the error can be related to network issues
				continue
			}
		}
		for _, destination := range s.destinations.Additionals {
			// send to a queue then send asynchronously for additional endpoints,
			// it will drop messages if the queue is full
			destination.SendAsync(payload.Content)
		}

		metrics.LogsSent.Add(1)
		break
	}
	s.outputChan <- payload
}
