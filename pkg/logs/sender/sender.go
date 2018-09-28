// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package sender

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Sender is responsible for sending logs to different destinations.
type Sender struct {
	inputChan    chan message.Message
	outputChan   chan message.Message
	destinations *Destinations
	done         chan struct{}
}

// NewSender returns an new sender.
func NewSender(inputChan, outputChan chan message.Message, destinations *Destinations) *Sender {
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
func (s *Sender) send(payload message.Message) {
	for {
		err := s.destinations.Main.Write(payload)
		if err != nil {
			switch err.(type) {
			case *FramingError:
				// the message can not be framed properly,
				// drop the message
				break
			default:
				// retry as the error can be related to network issues
				continue
			}
		}
		for _, destination := range s.destinations.Additonals {
			// try and forget strategy for additional endpoints
			go destination.Write(payload)
		}
		break
	}
	s.outputChan <- payload
}
