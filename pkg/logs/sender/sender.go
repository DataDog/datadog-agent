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

// Strategy should contain all logic to send logs to a remote destination
// and forward them the next stage of the pipeline.
type Strategy interface {
	Send(inputChan chan *message.Message, outputChan chan *message.Message, send func([]byte) error)
}

// Sender sends logs to different destinations.
type Sender struct {
	inputChan    chan *message.Message
	outputChan   chan *message.Message
	destinations *client.Destinations
	strategy     Strategy
	done         chan struct{}
}

// NewSender returns a new sender.
func NewSender(inputChan chan *message.Message, outputChan chan *message.Message, destinations *client.Destinations, strategy Strategy) *Sender {
	return &Sender{
		inputChan:    inputChan,
		outputChan:   outputChan,
		destinations: destinations,
		strategy:     strategy,
		done:         make(chan struct{}),
	}
}

// Start starts the sender.
func (s *Sender) Start() {
	go s.run()
}

// Stop stops the sender,
// this call blocks until inputChan is flushed
func (s *Sender) Stop() {
	close(s.inputChan)
	<-s.done
}

func (s *Sender) run() {
	defer func() {
		s.done <- struct{}{}
	}()
	s.strategy.Send(s.inputChan, s.outputChan, s.send)
}

// send sends a payload to multiple destinations,
// it will forever retry for the main destination unless the error is not retryable
// and only try once for additionnal destinations.
func (s *Sender) send(payload []byte) error {
	for {
		err := s.destinations.Main.Send(payload)
		if err != nil {
			metrics.DestinationErrors.Add(1)
			metrics.TlmDestinationErrors.Inc()
			if _, ok := err.(*client.RetryableError); ok {
				// could not send the payload because of a client issue,
				// let's retry
				continue
			}
			return err
		}
		break
	}

	for _, destination := range s.destinations.Additionals {
		// send in the background so that the agent does not fall behind
		// for the main destination
		destination.SendAsync(payload)
	}

	return nil
}

// shouldStopSending returns true if a component should stop sending logs.
func shouldStopSending(err error) bool {
	return err == context.Canceled
}
