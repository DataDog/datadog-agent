// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package sender

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// A Sender sends messages from an inputChan to datadog's intake,
// handling connections and retries.
type Sender struct {
	inputChan    chan message.Message
	outputChan   chan message.Message
	destinations *Destinations
	done         chan struct{}
}

// New returns an initialized Sender
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

// run lets the sender wire messages
func (s *Sender) run() {
	defer func() {
		s.done <- struct{}{}
	}()
	for payload := range s.inputChan {
		s.send(payload)
	}
}

// wireMessage lets the Sender send a message to datadog's intake
func (s *Sender) send(payload message.Message) {
	for {
		err := s.destinations.Main.Write(payload)
		if err != nil {
			switch err.(type) {
			case *FramingError:
				break
			default:
				continue
			}
		}
		break
	}

	for _, destination := range s.destinations.Additonals {
		go destination.Write(payload)
	}

	s.outputChan <- payload
}
