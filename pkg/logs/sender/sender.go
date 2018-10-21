// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package sender

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
)

// Sender is responsible for sending logs to different destinations.
type Sender struct {
	inputChan    chan *message.Message
	outputChan   chan *message.Message
	destinations *client.Destinations
	done         chan struct{}
}

// NewSender returns an new sender.
func NewSender(inputChan, outputChan chan *message.Message, destinations *client.Destinations) *Sender {
	return &Sender{
		inputChan:    inputChan,
		outputChan:   outputChan,
		destinations: destinations,
		done:         make(chan struct{}),
	}
}

// Start starts the Sender
func (s *Sender) Start() {
	go s.run()
}

// Stop stops the Sender,
// this call blocks until inputChan is flushed
func (s *Sender) Stop() {
	close(s.inputChan)
	<-s.done
}

// run lets the sender send messages.
func (s *Sender) run() {
	defer func() {
		s.done <- struct{}{}
	}()
	for payload := range s.inputChan {
		s.send(payload)
	}
}

// send keeps trying to send the message to the main destination until it succeeds
// and try to send the message to the additional destinations only once.
func (s *Sender) send(payload *message.Message) {
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
			// try and forget strategy for additional endpoints
			// this call is also blocking when the connection is not established yet
			// FIXME: run all `Send` in parallel to avoid the effect on a slow
			// destination on the others. Potentially add a buffer for secondary
			// destinations.
			destination.SendAsync(payload.Content)
		}

		metrics.LogsSent.Add(1)
		break
	}
	s.outputChan <- payload
}
